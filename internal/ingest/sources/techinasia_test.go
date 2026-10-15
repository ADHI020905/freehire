package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The search page and app bundle excerpts are real (2026-10-15), cut down to the script tag and
// the config object the adapter reads; the public search key is replaced so a secret scanner
// has nothing to flag.
const (
	techInAsiaTestPage = `<html><head><script src="//static.techinasia.com/assets/v5/runtime.a2e951c6.js" defer="defer" crossorigin></script>` +
		`<script src="//static.techinasia.com/assets/v5/app.0d3c45a9.js" defer="defer" crossorigin></script></head><body></body></html>`
	techInAsiaTestBundle = `ppa:{price:"155,500"}}}};var g=[];var d={algolia:{apiKey:"00000000000000000000000000000000",appId:"219WX3MPV4",trendingQue`
	techInAsiaTestQuery  = "https://219wx3mpv4-dsn.algolia.net/1/indexes/job_postings/query"
)

// techInAsiaFakeHTTP plays the search page, the app bundle and the Algolia index. pageSize
// overrides the adapter's hitsPerPage so a multi-page walk can be exercised with the fixture.
type techInAsiaFakeHTTP struct {
	hits     []json.RawMessage
	pageSize int
	// nbHits overrides the count the index reports; -1 reports the real one.
	nbHits     int
	estimated  bool
	failPage   int // fail this 0-based page; -1 never
	bundle     string
	headers    map[string]string
	pagesAsked []int
}

func newTechInAsiaFake(t *testing.T) *techInAsiaFakeHTTP {
	t.Helper()
	raw, err := os.ReadFile("testdata/techinasia/list.json")
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Hits []json.RawMessage `json:"hits"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	return &techInAsiaFakeHTTP{hits: r.Hits, pageSize: techInAsiaHitsPerPage, nbHits: -1, failPage: -1, bundle: techInAsiaTestBundle}
}

func (f *techInAsiaFakeHTTP) GetText(_ context.Context, u string) (string, error) {
	switch u {
	case techInAsiaSearchURL:
		return techInAsiaTestPage, nil
	case "https://static.techinasia.com/assets/v5/app.0d3c45a9.js":
		return f.bundle, nil
	}
	return "", fmt.Errorf("unexpected GetText %s", u)
}

func (f *techInAsiaFakeHTTP) PostJSONWithHeaders(_ context.Context, u string, headers map[string]string, body, v any) error {
	if u != techInAsiaTestQuery {
		return fmt.Errorf("unexpected query URL %s", u)
	}
	f.headers = headers
	p, err := url.ParseQuery(body.(map[string]string)["params"])
	if err != nil {
		return err
	}
	page, _ := strconv.Atoi(p.Get("page"))
	f.pagesAsked = append(f.pagesAsked, page)
	if page == f.failPage {
		return &StatusError{Method: http.MethodPost, Code: http.StatusBadGateway, URL: u}
	}
	lo := min(page*f.pageSize, len(f.hits))
	hi := min(lo+f.pageSize, len(f.hits))
	n := len(f.hits)
	if f.nbHits >= 0 {
		n = f.nbHits
	}
	out, _ := json.Marshal(map[string]any{
		"hits": f.hits[lo:hi], "nbHits": n, "page": page,
		"nbPages": (len(f.hits) + f.pageSize - 1) / f.pageSize, "exhaustiveNbHits": !f.estimated,
	})
	return json.Unmarshal(out, v)
}

func techInAsiaByID(t *testing.T, jobs []Job) map[string]Job {
	t.Helper()
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestTechInAsiaFetchMapsTheIndex(t *testing.T) {
	f := newTechInAsiaFake(t)
	jobs, err := NewTechInAsia(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 8 {
		t.Fatalf("got %d jobs, want 8", len(jobs))
	}
	if f.headers["X-Algolia-Application-Id"] != "219WX3MPV4" ||
		f.headers["X-Algolia-API-Key"] != "00000000000000000000000000000000" {
		t.Errorf("credentials not sent from the bundle: %v", f.headers)
	}
	byID := techInAsiaByID(t, jobs)

	g := byID["4448ac55-d7a0-488c-9348-05f4d615d8ce"]
	if g.Title != "Senior Golang Developer - Banking Industry" || g.Company != "SIGMATECH" {
		t.Errorf("identity: %q / %q", g.Title, g.Company)
	}
	if g.URL != "https://www.techinasia.com/jobs/4448ac55-d7a0-488c-9348-05f4d615d8ce" {
		t.Errorf("url without an external link: %s", g.URL)
	}
	if g.Location != "Tangerang, Indonesia" || len(g.Countries) != 1 || g.Countries[0] != "id" {
		t.Errorf("place: %q %v", g.Location, g.Countries)
	}
	if g.WorkMode != "onsite" || g.Remote || g.EmploymentType != "contract" || !g.IsTechHint {
		t.Errorf("facets: mode=%q remote=%v type=%q tech=%v", g.WorkMode, g.Remote, g.EmploymentType, g.IsTechHint)
	}
	if g.SalaryMin == nil || *g.SalaryMin != 14400000 || g.SalaryMax == nil || *g.SalaryMax != 15400000 ||
		g.SalaryCurrency != "IDR" || g.SalaryPeriod != "month" {
		t.Errorf("salary: %v-%v %s/%s", g.SalaryMin, g.SalaryMax, g.SalaryCurrency, g.SalaryPeriod)
	}
	if g.ExperienceYearsMin == nil || *g.ExperienceYearsMin != 5 {
		t.Errorf("experience: %v", g.ExperienceYearsMin)
	}
	if g.PostedAt == nil || g.PostedAt.UTC().Format("2006-01-02 15:04:05") != "2026-10-14 10:52:14" {
		t.Errorf("posted: %v", g.PostedAt)
	}
	if !strings.Contains(g.Description, "Proven experience") {
		t.Errorf("description: %.200s", g.Description)
	}
	if len(g.Skills) == 0 {
		t.Error("no skills mapped")
	}

	// The posting's own employer link wins.
	if u := byID["7e44d429-0d3e-47ee-8326-e5e35c4a5405"].URL; !strings.HasPrefix(u, "https://kumparan.darwinbox.com/") {
		t.Errorf("employer link not used: %s", u)
	}
	// One intake form shared by two postings is neither posting's own page.
	for _, id := range []string{"8522c4d6-fea4-41fc-8e7a-5ed70368a1cd", "94f2aee5-d773-42c4-83c3-b264b274abfd"} {
		if u := byID[id].URL; u != techInAsiaPostingURL+id {
			t.Errorf("shared link used for %s: %s", id, u)
		}
	}
	// A redirector is opaque.
	if u := byID["f166f634-94cd-49ea-8b7e-81f95e20c919"].URL; !strings.HasPrefix(u, techInAsiaPostingURL) {
		t.Errorf("shortener used: %s", u)
	}
	// A remote posting with no city states no country.
	r := byID["92bcfa64-807d-4d64-81fd-b1d97c46e7ff"]
	if r.Location != "Remote" || r.Countries != nil || r.WorkMode != "remote" || !r.Remote {
		t.Errorf("remote: %q %v %q", r.Location, r.Countries, r.WorkMode)
	}
	// An all-zero range is no salary; a non-technical position is no tech hint.
	m := byID["52efcfac-2cab-44ab-bfa4-ad254710ae83"]
	if m.SalaryMin != nil || m.SalaryMax != nil || m.SalaryCurrency != "" || m.IsTechHint {
		t.Errorf("zero salary or hint: %v %v %q %v", m.SalaryMin, m.SalaryMax, m.SalaryCurrency, m.IsTechHint)
	}
	if m.ExperienceYearsMin != nil {
		t.Errorf("zero experience mapped: %v", *m.ExperienceYearsMin)
	}
}

func TestTechInAsiaWalksEveryPage(t *testing.T) {
	f := newTechInAsiaFake(t)
	f.pageSize = 3
	jobs, err := NewTechInAsia(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 8 || len(f.pagesAsked) != 3 {
		t.Errorf("got %d jobs over pages %v, want 8 over 3", len(jobs), f.pagesAsked)
	}
}

func TestTechInAsiaFailsAnIncompleteRead(t *testing.T) {
	cases := map[string]func(f *techInAsiaFakeHTTP){
		"a later page fails":     func(f *techInAsiaFakeHTTP) { f.pageSize, f.failPage = 3, 2 },
		"the first page fails":   func(f *techInAsiaFakeHTTP) { f.failPage = 0 },
		"fewer read than listed": func(f *techInAsiaFakeHTTP) { f.nbHits = 9 },
		"an estimated count":     func(f *techInAsiaFakeHTTP) { f.estimated = true },
		"an empty index":         func(f *techInAsiaFakeHTTP) { f.hits, f.nbHits = nil, 0 },
		"no credentials":         func(f *techInAsiaFakeHTTP) { f.bundle = "var d={}" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newTechInAsiaFake(t)
			mutate(f)
			if jobs, err := NewTechInAsia(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestTechInAsiaIsAFullCatalogAggregator(t *testing.T) {
	src := NewTechInAsia(nil)
	if _, ok := src.(boardless); !ok {
		t.Error("techinasia is not boardless")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("techinasia is not an aggregator")
	}
	if _, ok := src.(fullCatalog); !ok {
		t.Error("techinasia does not claim fullCatalog")
	}
	if _, ok := Taxonomy()["techinasia"]; !ok {
		t.Error("techinasia is not registered")
	}
}
