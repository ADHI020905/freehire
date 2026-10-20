package sources

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ijPage wraps an inner __INITIAL_PROPS__ JSON document in the exact double-encoding the real
// pages use: JSON.parse("<the document as a JS string literal>"). json.Marshal of the document
// string produces that literal (quoted, with the inner quotes escaped), so the adapter's decoder
// is exercised end-to-end.
func ijPage(t *testing.T, innerJSON string) string {
	t.Helper()
	lit, err := json.Marshal(innerJSON)
	if err != nil {
		t.Fatalf("marshal literal: %v", err)
	}
	return `<html><head><script>window.__INITIAL_PROPS__ = JSON.parse(` + string(lit) + `);</script></head><body></body></html>`
}

// infojobsHTTP is a test TextGetter serving listing pages keyed by ?page=N; any other request
// fails, so a detail fetch is a test failure.
type infojobsHTTP struct {
	listPages map[int]string
	failList  bool
	got       []string
}

var infojobsPageRE = regexp.MustCompile(`[?&]page=(\d+)`)

func (f *infojobsHTTP) GetText(_ context.Context, url string) (string, error) {
	f.got = append(f.got, url)
	if strings.HasPrefix(url, "https://www.infojobs.net/ofertas-trabajo?") {
		if f.failList {
			return "", errors.New("infojobsHTTP: list boom")
		}
		page := 1
		if m := infojobsPageRE.FindStringSubmatch(url); m != nil {
			page, _ = strconv.Atoi(m[1])
		}
		if body, ok := f.listPages[page]; ok {
			return body, nil
		}
		return `<html>window.__INITIAL_PROPS__ = JSON.parse("{\"offers\":[]}");</html>`, nil
	}
	return "", errors.New("infojobsHTTP: unexpected request " + url)
}

func TestInfojobsFetchNewRefreshesSeenAndKeepsTeaser(t *testing.T) {
	// Two offers on page 1; totalElements=2 stops pagination after one page. Offer B's teaser
	// carries a description with an embedded quote and a `");` sequence to exercise the decoder.
	list := `{"overview":{"totalElements":2},"offers":[
	  {"code":"AAA","title":"Backend Engineer","companyName":"Acme","city":"Madrid",
	   "publishedAt":"2026-07-12T08:00:00Z","teleworking":"Presencial","link":"//www.infojobs.net/madrid/backend/of-iAAA?applicationOrigin=search"},
	  {"code":"BBB","title":"Data Scientist","companyName":"Beta","city":"Barcelona",
	   "description":"Teaser body. He said \"hi\"); and more text.",
	   "publishedAt":"2026-07-11T09:00:00Z","teleworking":"Solo teletrabajo","link":"//www.infojobs.net/barcelona/data/of-iBBB?applicationOrigin=search"}
	]}`
	http := &infojobsHTTP{listPages: map[int]string{1: ijPage(t, list)}}

	// AAA already ingested → seen; BBB new → its teaser, with no detail request.
	seen := func(id string) bool { return id == "AAA" }
	jobs, err := infojobs{http: http}.FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs, got %d: %+v", len(jobs), jobs)
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	aaa := byID["AAA"]
	if !aaa.SeenRefresh {
		t.Fatalf("seen offer AAA should be a liveness refresh: %+v", aaa)
	}
	if aaa.URL != "https://www.infojobs.net/madrid/backend/of-iAAA" {
		t.Fatalf("AAA url (query must be stripped, scheme restored): %q", aaa.URL)
	}
	if aaa.WorkMode != "onsite" {
		t.Fatalf("Presencial should be onsite, got %q", aaa.WorkMode)
	}

	bbb := byID["BBB"]
	if bbb.SeenRefresh {
		t.Fatalf("new offer BBB must not be a seen-refresh")
	}
	if bbb.WorkMode != "remote" || !bbb.Remote {
		t.Fatalf("Solo teletrabajo should be remote: mode=%q remote=%v", bbb.WorkMode, bbb.Remote)
	}
	if !strings.Contains(bbb.Description, "Teaser body") || !strings.Contains(bbb.Description, "more text") {
		t.Fatalf("BBB should carry the listing teaser: %q", bbb.Description)
	}
	if bbb.PostedAt == nil {
		t.Fatalf("BBB posted_at not parsed")
	}
}

// Fetch (list-only) yields the teaser description without any detail request.
func TestInfojobsFetchListOnly(t *testing.T) {
	list := `{"overview":{"totalElements":1},"offers":[
	  {"code":"Y","title":"QA","companyName":"Co","city":"Sevilla","teleworking":"",
	   "description":"Just the teaser.","link":"//www.infojobs.net/sevilla/qa/of-iY"}]}`
	http := &infojobsHTTP{listPages: map[int]string{1: ijPage(t, list)}}
	jobs, err := infojobs{http: http}.Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !strings.Contains(jobs[0].Description, "Just the teaser") {
		t.Fatalf("list-only teaser expected: %+v", jobs)
	}
	if jobs[0].WorkMode != "" {
		t.Fatalf("empty teleworking should yield empty work mode, got %q", jobs[0].WorkMode)
	}
	// No detail URL should have been requested.
	for _, u := range http.got {
		if !strings.HasPrefix(u, "https://www.infojobs.net/ofertas-trabajo?") {
			t.Fatalf("unexpected non-listing request: %q", u)
		}
	}
}

// A first-page listing failure is a board-level error.
func TestInfojobsListFailure(t *testing.T) {
	http := &infojobsHTTP{failList: true}
	if _, err := (infojobs{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want error on list failure")
	}
}

// The double-encoded __INITIAL_PROPS__ decodes even when the payload contains escaped quotes and
// a `");` sequence that would defeat a lazy regex.
func TestInfojobsDecodeProps(t *testing.T) {
	inner := `{"offers":[{"description":"a \"quote\" then ); and \"end\");"}]}`
	page := ijPage(t, inner)
	var props infojobsListProps
	if err := infojobsDecodeProps(page, &props); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(props.Offers) != 1 || props.Offers[0].Description != `a "quote" then ); and "end");` {
		t.Fatalf("decoded offers mismatch: %+v", props.Offers)
	}
}

func TestInfojobsWorkMode(t *testing.T) {
	for label, want := range map[string]string{
		"Solo teletrabajo": "remote",
		"Híbrido":          "hybrid",
		"Presencial":       "onsite",
		"":                 "",
	} {
		if got := infojobsWorkMode(label); got != want {
			t.Errorf("infojobsWorkMode(%q) = %q, want %q", label, got, want)
		}
	}
}

// A page that fails after the first — here the bot-challenge page, which carries no props blob —
// fails the whole crawl rather than returning the offers gathered so far, which the sweep would
// read as the full listing.
func TestInfojobsLaterPageFailureFailsCrawl(t *testing.T) {
	list := `{"overview":{"totalElements":3},"offers":[
	  {"code":"A","title":"Dev","companyName":"Co","link":"//www.infojobs.net/x/of-iA"}]}`
	http := &infojobsHTTP{listPages: map[int]string{
		1: ijPage(t, list),
		2: `<html><title>InfoJobs</title>No podemos identificar tu navegador</html>`,
	}}
	if _, err := (infojobs{http: http}).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when a later listing page fails, got a partial list")
	}
}
