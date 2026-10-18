package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // wuzzuf dates are Cairo wall time, and Egypt observes DST

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// wuzzuf adapts WUZZUF (wuzzuf.net), Egypt's largest general job board, whose postings each
// name their own employer — mostly in Egypt, with a minority in Saudi Arabia, the UAE and
// further afield. Board-based aggregator: a board is one of the platform's own job ROLES
// ("IT/Software Development"), which is what keeps a general board on its technical slice —
// 436 of 5,266 active postings on 2026-10-18.
//
// wuzzuf.net itself sits behind an interactive Cloudflare challenge that refuses every client
// from every IP we have, but the JSON:API the site's own front end calls is served from
// api.wuzzuf.net, an AWS load balancer with no bot wall. The search names a role's posting ids
// and employers; the job documents carry everything else, so the crawl is a search walk plus
// one detail request per wuzzufDetailBatch postings. The search ignores a filter key it does
// not know and answers the whole catalogue instead, so every document is checked against the
// board's role before it is believed.
//
// fullBoardListing: the search walk reads to a short page and fails unless the distinct ids
// read equal the search's own count, on any failed page, on a count that moves mid-walk and on
// reaching wuzzufMaxPages. See the WUZZUF traps in AGENTS.md.
type wuzzuf struct {
	http JSONAPIClient
	// pageSize and detailBatch are wuzzufPageSize and wuzzufDetailBatch outside tests.
	pageSize, detailBatch int
}

const (
	wuzzufSearchURL = "https://api.wuzzuf.net/api/search/job"
	wuzzufJobsURL   = "https://api.wuzzuf.net/api/job"
	wuzzufSiteURL   = "https://wuzzuf.net/"
	// wuzzufPageSize is how many search rows one request asks for.
	wuzzufPageSize = 100
	// wuzzufMaxPages is a ceiling twenty times above the IT role's size, never the expected
	// end: a role whose count does not fit it FAILS rather than being read in part.
	wuzzufMaxPages = 100
	// wuzzufDetailBatch is how many job documents one request asks for. A document is ~30 KB
	// and the API spends 0.3-1.2 s on each, so ten keeps a request well inside the client's
	// 15 s timeout; a batch of 25 took up to 30 s.
	wuzzufDetailBatch = 10
)

var (
	// wuzzufBoardRE is a role name as the platform spells it ("IT/Software Development",
	// "Engineering - Telecom/Technology").
	wuzzufBoardRE = regexp.MustCompile(`^[A-Z][A-Za-z&/ -]*[A-Za-z]$`)
	// wuzzufIDRE is a posting id, a UUID.
	wuzzufIDRE = regexp.MustCompile(`^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)
	// wuzzufURIRE is a posting page's path on wuzzuf.net: jobs/p/<slug>, internship/<slug>,
	// saudi/jobs/p/<slug>.
	wuzzufURIRE = regexp.MustCompile(`^(?:[a-z]+/)+[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// wuzzufCairo is the zone the API's zone-less posting dates are written in.
var wuzzufCairo = mustLoadLocation("Africa/Cairo")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// NewWuzzuf builds the WUZZUF adapter over the given HTTP client.
func NewWuzzuf(c JSONAPIClient) Source {
	return wuzzuf{http: c, pageSize: wuzzufPageSize, detailBatch: wuzzufDetailBatch}
}

func (wuzzuf) Provider() string { return "wuzzuf" }

// Every posting names its own employer, so wuzzuf stays in the source facet and in the
// cross-source duplicate suppression set.
func (wuzzuf) aggregator() {}

// fullBoardListing: Fetch walks the role's search to a short page and fails unless the ids
// read equal the search's count, all distinct, and every document is of the board's role. See
// the fullBoardListing interface (source.go) for the bar.
func (wuzzuf) fullBoardListing() {}

// wuzzufSearchBody is the search request the site's front end sends; the API refuses one
// without longitude/latitude ("Request is missing some parameters").
type wuzzufSearchBody struct {
	StartIndex    int                 `json:"startIndex"`
	PageSize      int                 `json:"pageSize"`
	Longitude     string              `json:"longitude"`
	Latitude      string              `json:"latitude"`
	Query         string              `json:"query"`
	SearchFilters map[string][]string `json:"searchFilters"`
}

type wuzzufSearchResult struct {
	Meta struct {
		TotalResultsCount *int `json:"totalResultsCount"`
	} `json:"meta"`
	Data []wuzzufHit `json:"data"`
}

// wuzzufHit is one search row: the posting id and, among its computed fields, the employer.
type wuzzufHit struct {
	ID         string `json:"id"`
	Attributes struct {
		ComputedFields []struct {
			Name  string `json:"name"`
			Value []any  `json:"value"`
		} `json:"computedFields"`
	} `json:"attributes"`
}

// company is the employer the search names, "" when the employer chose to hide it.
func (h wuzzufHit) company() string {
	for _, f := range h.Attributes.ComputedFields {
		if f.Name == "company_name" && len(f.Value) > 0 {
			s, _ := f.Value[0].(string)
			return strings.Join(strings.Fields(s), " ")
		}
	}
	return ""
}

type wuzzufNamed struct {
	Name string `json:"name"`
}

type wuzzufJobsResult struct {
	Data []wuzzufDoc `json:"data"`
}

// wuzzufDoc is one job document as the API states it. Nullable objects decode to nil.
type wuzzufDoc struct {
	ID         string `json:"id"`
	Attributes struct {
		Status       string `json:"status"`
		JobType      string `json:"jobType"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Requirements string `json:"requirements"`
		URI          string `json:"uri"`
		PostedAt     string `json:"postedAt"`
		HideSalary   bool   `json:"hideSalary"`
		Salary       struct {
			Min      *float64 `json:"min"`
			Max      *float64 `json:"max"`
			IsPaid   bool     `json:"isPaid"`
			Currency *struct {
				Code string `json:"code"`
			} `json:"currency"`
			Period *wuzzufNamed `json:"period"`
		} `json:"salary"`
		Keywords             []wuzzufNamed `json:"keywords"`
		CareerLevel          *wuzzufNamed  `json:"careerLevel"`
		WorkplaceArrangement *struct {
			DisplayedName string `json:"displayedName"`
		} `json:"workplaceArrangement"`
		Location struct {
			Country *struct {
				Name string `json:"name"`
				Code string `json:"code"`
			} `json:"country"`
			City *wuzzufNamed `json:"city"`
			Area *wuzzufNamed `json:"area"`
		} `json:"location"`
		WorkExperienceYears struct {
			Min *int `json:"min"`
		} `json:"workExperienceYears"`
		WorkRoles []wuzzufNamed `json:"workRoles"`
		WorkTypes []wuzzufNamed `json:"workTypes"`
	} `json:"attributes"`
}

func (s wuzzuf) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if !wuzzufBoardRE.MatchString(e.Board) {
		return nil, fmt.Errorf("wuzzuf: board %q is not a role name", e.Board)
	}
	hits, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	// A posting whose employer is hidden names no company, so it is not yielded and costs no
	// detail request.
	named := hits[:0:0]
	for _, h := range hits {
		if h.company() != "" {
			named = append(named, h)
		}
	}
	jobs := make([]Job, 0, len(named))
	for lo := 0; lo < len(named); lo += s.detailBatch {
		batch := named[lo:min(lo+s.detailBatch, len(named))]
		docs, err := s.details(ctx, batch)
		if err != nil {
			// A failed request is the absence of an answer about these postings, not the
			// platform saying they are gone.
			for _, h := range batch {
				jobs = append(jobs, unreadableDetail(h.ID, "", h.company()))
			}
			continue
		}
		for _, h := range batch {
			d, ok := docs[h.ID]
			if !ok {
				continue // the API no longer serves it: gone since the search was read
			}
			if !d.hasRole(e.Board) {
				return nil, fmt.Errorf("wuzzuf: board %q: posting %s is not of the role, the search ignored its filter",
					e.Board, h.ID)
			}
			if job, ok := d.toJob(h.company()); ok {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, nil
}

// list walks the role's search to its natural end, failing on any page error, on a count
// that moves mid-walk, on a repeated id and on a total the walk did not read in full.
func (s wuzzuf) list(ctx context.Context, role string) ([]wuzzufHit, error) {
	var hits []wuzzufHit
	seen := map[string]bool{}
	total := -1
	for page := 0; ; page++ {
		if page == wuzzufMaxPages {
			return nil, fmt.Errorf("wuzzuf: board %q: reached %d pages without the end", role, wuzzufMaxPages)
		}
		body := wuzzufSearchBody{
			StartIndex:    page * s.pageSize,
			PageSize:      s.pageSize,
			Longitude:     "0",
			Latitude:      "0",
			SearchFilters: map[string][]string{"roles": {role}},
		}
		var r wuzzufSearchResult
		if err := s.http.PostJSONAPI(ctx, wuzzufSearchURL, body, &r); err != nil {
			return nil, fmt.Errorf("wuzzuf: board %q: search page %d: %w", role, page, err)
		}
		if r.Meta.TotalResultsCount == nil {
			return nil, fmt.Errorf("wuzzuf: board %q: the search states no count", role)
		}
		count := *r.Meta.TotalResultsCount
		switch {
		case page > 0 && count != total:
			return nil, fmt.Errorf("wuzzuf: board %q: the count moved from %d to %d mid-walk", role, total, count)
		case count == 0:
			// The platform's roles always carry postings; nothing at all is a misspelled role
			// (an unknown value matches nothing), not a board whose postings all closed.
			return nil, fmt.Errorf("wuzzuf: board %q matches no postings", role)
		case count > wuzzufMaxPages*s.pageSize:
			return nil, fmt.Errorf("wuzzuf: board %q lists %d postings, over the %d the walk reads",
				role, count, wuzzufMaxPages*s.pageSize)
		}
		total = count
		for _, h := range r.Data {
			if !wuzzufIDRE.MatchString(h.ID) {
				return nil, fmt.Errorf("wuzzuf: board %q: unreadable posting id %q", role, h.ID)
			}
			if seen[h.ID] {
				return nil, fmt.Errorf("wuzzuf: board %q: posting %s listed twice", role, h.ID)
			}
			seen[h.ID] = true
			hits = append(hits, h)
		}
		if len(r.Data) < s.pageSize {
			break
		}
	}
	if len(hits) != total {
		return nil, fmt.Errorf("wuzzuf: board %q: read %d of %d postings", role, len(hits), total)
	}
	return hits, nil
}

// details fetches the job documents of one batch, keyed by id.
func (s wuzzuf) details(ctx context.Context, batch []wuzzufHit) (map[string]wuzzufDoc, error) {
	ids := make([]string, len(batch))
	for i, h := range batch {
		ids[i] = h.ID
	}
	q := url.Values{}
	q.Set("filter[other][ids]", strings.Join(ids, ","))
	q.Set("page[size]", strconv.Itoa(len(ids)))
	var r wuzzufJobsResult
	if err := s.http.GetJSONAPI(ctx, wuzzufJobsURL+"?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	out := make(map[string]wuzzufDoc, len(r.Data))
	for _, d := range r.Data {
		out[d.ID] = d
	}
	return out, nil
}

func (d wuzzufDoc) hasRole(role string) bool {
	for _, r := range d.Attributes.WorkRoles {
		if r.Name == role {
			return true
		}
	}
	return false
}

func (d wuzzufDoc) toJob(company string) (Job, bool) {
	a := d.Attributes
	title := strings.Join(strings.Fields(a.Title), " ")
	if a.Status != "active" || title == "" || !wuzzufURIRE.MatchString(a.URI) {
		return Job{}, false
	}
	body := a.Description
	if strings.TrimSpace(a.Requirements) != "" {
		body += "<h3>Requirements</h3>" + a.Requirements
	}
	job := Job{
		ExternalID:     d.ID,
		URL:            wuzzufSiteURL + a.URI,
		Title:          title,
		Company:        company,
		Description:    sanitizeHTML(body),
		PostedAt:       wuzzufPostedAt(a.PostedAt),
		EmploymentType: d.employmentType(),
		Seniority:      d.seniority(),
		IsTechHint:     true, // every board is a role, and the crawl reads the IT one
	}
	if a.WorkExperienceYears.Min != nil && *a.WorkExperienceYears.Min > 0 {
		years := *a.WorkExperienceYears.Min
		job.ExperienceYearsMin = &years
	}
	if w := a.WorkplaceArrangement; w != nil {
		job.WorkMode = wuzzufWorkModes[w.DisplayedName]
	}
	job.Remote = job.WorkMode == "remote"
	loc := a.Location
	var place []string
	for _, n := range []*wuzzufNamed{loc.Area, loc.City} {
		if n != nil {
			place = append(place, n.Name)
		}
	}
	if loc.Country != nil {
		place = append(place, loc.Country.Name)
		if c := location.NormalizeCountry(strings.TrimSpace(loc.Country.Code)); c != "" {
			job.Countries = []string{c}
		}
	}
	job.Location = distinctJoin(place, ", ", strings.TrimSpace)
	skills := make([]string, 0, len(a.Keywords))
	for _, k := range a.Keywords {
		skills = append(skills, k.Name)
	}
	job.Skills = skilltag.Canonicalize(skills)
	d.salary(&job)
	return job, true
}

// wuzzufPostedAt reads the API's "MM/DD/YYYY HH:MM:SS", which is Cairo wall time: a posting
// read at 18:00 UTC carried 20:19 as its posting time.
func wuzzufPostedAt(s string) *time.Time {
	t, err := time.ParseInLocation("01/02/2006 15:04:05", strings.TrimSpace(s), wuzzufCairo)
	if err != nil {
		return nil
	}
	return NotFuture(&t)
}

var wuzzufWorkModes = map[string]string{
	"On-site": "onsite",
	"Hybrid":  "hybrid",
	"Remote":  "remote",
}

// wuzzufEmploymentTypes maps the platform's work types onto vocab.EmploymentTypeValues.
// shift_based has no counterpart.
var wuzzufEmploymentTypes = map[string]string{
	"full_time":         "full_time",
	"part_time":         "part_time",
	"freelance_project": "contract",
	"internship":        "internship",
}

// employmentType is stated only when the posting names exactly one work type: 14 of 436 IT
// postings name several ("full_time, part_time, freelance_project"), which says nothing.
func (d wuzzufDoc) employmentType() string {
	if len(d.Attributes.WorkTypes) != 1 {
		return ""
	}
	return wuzzufEmploymentTypes[d.Attributes.WorkTypes[0].Name]
}

// seniority maps the career levels that name one: an internship or a student is an intern,
// "Entry Level" a junior. "Experienced" spans middle and senior and "Manager" / "Senior
// Management" a management track, so they are left to the title dictionary.
func (d wuzzufDoc) seniority() string {
	a := d.Attributes
	if a.JobType == "internship" {
		return "intern"
	}
	if a.CareerLevel == nil {
		return ""
	}
	switch a.CareerLevel.Name {
	case "Student":
		return "intern"
	case "Entry Level":
		return "junior"
	default:
		return ""
	}
}

var wuzzufSalaryPeriods = map[string]string{
	"Per Hour":  "hour",
	"Per Day":   "day",
	"Per Month": "month",
	"Per Year":  "year",
}

// salary takes the posting's range only when the employer shows it and it is paid: a hidden
// salary is withheld by the employer, and an unpaid internship carries a 0-0 range.
func (d wuzzufDoc) salary(job *Job) {
	sal := d.Attributes.Salary
	if d.Attributes.HideSalary || !sal.IsPaid || sal.Currency == nil || sal.Period == nil {
		return
	}
	period := wuzzufSalaryPeriods[sal.Period.Name]
	currency := strings.ToUpper(strings.TrimSpace(sal.Currency.Code))
	if period == "" || len(currency) != 3 {
		return
	}
	var lo, hi *int
	if sal.Min != nil {
		lo = roundSalaryPart(*sal.Min)
	}
	if sal.Max != nil {
		hi = roundSalaryPart(*sal.Max)
	}
	if lo == nil && hi == nil {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, period
}
