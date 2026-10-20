package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// infojobs adapts infojobs.net, Spain's largest job board. Boardless and multi-company: the
// whole-site listing (/ofertas-trabajo) is paged and each posting carries its own employer, so it
// stays in the source facet and takes the company from the offer.
// The pages are a server-rendered React app that embeds its data as a `window.__INITIAL_PROPS__`
// blob; the listing blob carries every field EXCEPT the full description (the list holds a
// truncated teaser). The full body lives on the offer's own page, and that page is not read:
// measured 2026-10-20 it answers a GeeTest bot challenge from a laptop, the prod host and the
// proxy alike, so a detail request costs one shared-proxy round trip per new offer (~63k on the
// first crawl) and yields nothing. Every offer therefore lands with its teaser.
//
// The crawler host's own IP is refused outright (405 on every page since 2026-10-08), which is why
// infojobs is in proxiedProviders.
type infojobs struct {
	http TextGetter
}

const (
	// infojobsListURL is the whole-site listing. It used to be the IT category's
	// (/ofertas-trabajo/informatica-telecomunicaciones), back when the catalogue was IT-only;
	// by 2026-10-20 that page answered a GeeTest bot challenge instead of the listing.
	infojobsListURL = "https://www.infojobs.net/ofertas-trabajo?page=%d"
	// infojobsMaxPages caps pagination as a backstop; the overview.totalElements count is the
	// primary stop. Measured 2026-10-20: 63.6k offers at 22 a page is ~2.9k pages, ~0.5 s each.
	infojobsMaxPages = 4000
)

// NewInfoJobs builds the infojobs.net adapter over the given HTTP client.
func NewInfoJobs(c TextGetter) Source { return infojobs{http: c} }

func (infojobs) Provider() string { return "infojobs" }

func (infojobs) boardless() {}

func (infojobs) aggregator() {}

// infojobsListProps is the subset of a listing page's __INITIAL_PROPS__ we read: the page of
// offers and the catalogue size used to stop pagination.
type infojobsListProps struct {
	Offers   []infojobsOffer `json:"offers"`
	Overview struct {
		TotalElements int `json:"totalElements"`
	} `json:"overview"`
}

// infojobsOffer is one listing offer. Description here is a truncated teaser (the full body lives
// on the detail page); Link is a protocol-relative URL with a tracking query. Teleworking is the
// structured work-mode label.
type infojobsOffer struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	CompanyName string `json:"companyName"`
	Link        string `json:"link"`
	City        string `json:"city"`
	PublishedAt string `json:"publishedAt"`
	Teleworking string `json:"teleworking"`
	Description string `json:"description"`
}

// Fetch is the crawl for callers that supply no seen set; FetchNew is the path ingest uses.
func (s infojobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew pages the listing and reports a seen offer as a liveness refresh only, so a
// stored row is not re-upserted (and its facets re-derived) on every crawl.
func (s infojobs) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	offers, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, o := range offers {
		job, ok := o.toJob()
		if !ok {
			continue
		}
		job.SeenRefresh = seen(o.Code)
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// crawl pages the whole-site listing and returns every raw offer — the shared list walk behind
// Fetch and FetchNew. It stops at the last page (an empty page or once the whole catalogue is
// collected), capped by infojobsMaxPages.
func (s infojobs) crawl(ctx context.Context) ([]infojobsOffer, error) {
	var offers []infojobsOffer
	for page := 1; page <= infojobsMaxPages; page++ {
		text, err := s.http.GetText(ctx, fmt.Sprintf(infojobsListURL, page))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("infojobs: listing page %d: %w", page, err)
			}
			break // a later page failing ends enumeration with the offers gathered so far
		}
		var props infojobsListProps
		if err := infojobsDecodeProps(text, &props); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("infojobs: listing page %d: %w", page, err)
			}
			break
		}
		if len(props.Offers) == 0 {
			break
		}
		offers = append(offers, props.Offers...)
		if props.Overview.TotalElements > 0 && len(offers) >= props.Overview.TotalElements {
			break
		}
	}
	return offers, nil
}

// infojobsPropsMarker matches the `window.__INITIAL_PROPS__ = JSON.parse(` assignment; the JSON
// string literal that follows is the (double-encoded) props blob.
var infojobsPropsMarker = regexp.MustCompile(`window\.__INITIAL_PROPS__\s*=\s*JSON\.parse\(`)

// infojobsDecodeProps extracts the __INITIAL_PROPS__ blob from a page and unmarshals it into v.
// The blob is double-encoded — a JSON string literal (produced for JSON.parse) whose content is
// itself the JSON document — so it is decoded in two steps: a json.Decoder reads the leading
// string token (correctly honouring the `\"` escapes and stopping at the true closing quote,
// which a lazy regex could not), then its unescaped content is unmarshalled into v.
func infojobsDecodeProps(pageText string, v any) error {
	loc := infojobsPropsMarker.FindStringIndex(pageText)
	if loc == nil {
		return errors.New("infojobs: __INITIAL_PROPS__ not found")
	}
	dec := json.NewDecoder(strings.NewReader(pageText[loc[1]:]))
	var inner string
	if err := dec.Decode(&inner); err != nil {
		return fmt.Errorf("infojobs: decode props string: %w", err)
	}
	if err := json.Unmarshal([]byte(inner), v); err != nil {
		return fmt.Errorf("infojobs: unmarshal props: %w", err)
	}
	return nil
}

// infojobsWorkMode maps the structured teleworking label to freehire's work-mode vocabulary.
func infojobsWorkMode(teleworking string) string {
	switch strings.TrimSpace(teleworking) {
	case "Solo teletrabajo":
		return "remote"
	case "Híbrido":
		return "hybrid"
	case "Presencial":
		return "onsite"
	default:
		return ""
	}
}

// toJob maps a listing offer to a Job, returning ok=false for an unusable offer (no code to key
// on, no link to build the URL, or no company). The URL is the offer's protocol-relative link
// with its tracking query stripped and the scheme restored.
func (o infojobsOffer) toJob() (Job, bool) {
	if o.Code == "" || o.Link == "" || o.CompanyName == "" {
		return Job{}, false
	}
	mode := infojobsWorkMode(o.Teleworking)
	return Job{
		ExternalID:  o.Code,
		URL:         "https:" + trimURLSuffix(o.Link),
		Title:       o.Title,
		Company:     o.CompanyName,
		Location:    o.City,
		Description: sanitizeHTML(o.Description),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    parseRFC3339(o.PublishedAt),
	}, true
}
