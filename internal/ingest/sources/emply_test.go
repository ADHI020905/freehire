package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// emplyFake answers the listing POST from real responses keyed by the request's offset, the
// way the platform pages: the requested count does not change which postings an offset starts
// at, so the fixtures (captured at count=4) serve any page size the adapter asks for.
type emplyFake struct {
	pages map[int]string // offset -> raw response body
	fail  map[int]error  // offset -> transport error

	urls   []string
	bodies []emplyPageRequest
}

func (f *emplyFake) PostJSON(_ context.Context, u string, body, v any) error {
	req := body.(emplyPageRequest)
	f.urls = append(f.urls, u)
	f.bodies = append(f.bodies, req)
	if err := f.fail[req.Offset]; err != nil {
		return err
	}
	raw, ok := f.pages[req.Offset]
	if !ok {
		raw = `{"vacancies":[],"count":0}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func emplyFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/emply/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// emplyPaychex is job.paychex.eu (an Emply career site on a customer domain) as captured
// 2026-10-19: six vacancies over two pages, one of them the talent-pool "Unsolicited
// applications" entry.
func emplyPaychex(t *testing.T) *emplyFake {
	return &emplyFake{pages: map[int]string{
		0: emplyFixture(t, "paychex_offset0.json"),
		4: emplyFixture(t, "paychex_offset4.json"),
	}}
}

func TestEmplyProviderAndMarkers(t *testing.T) {
	s := NewEmply(nil)
	if s.Provider() != "emply" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(boardless); ok {
		t.Error("emply is board-keyed, not boardless")
	}
	if !FullBoardListingProviders(All(nil))["emply"] {
		t.Error("FullBoardListingProviders(All(nil)) should include emply")
	}
}

func TestEmplyHost(t *testing.T) {
	cases := map[string]string{
		"molslinjen":                  "molslinjen.career.emply.com",
		"Molslinjen":                  "molslinjen.career.emply.com",
		"molslinjen.career.emply.com": "molslinjen.career.emply.com",
		"job.paychex.eu":              "job.paychex.eu",
		"Midtjob.dk":                  "midtjob.dk",
	}
	for board, want := range cases {
		if got := emplyHost(board); got != want {
			t.Errorf("emplyHost(%q) = %q, want %q", board, got, want)
		}
	}
	// The two spellings of one career site are one crawl target.
	a, _ := BoardDedupeKey(CompanyEntry{Provider: "emply", Board: "molslinjen"})
	b, _ := BoardDedupeKey(CompanyEntry{Provider: "emply", Board: "molslinjen.career.emply.com"})
	if a != b {
		t.Errorf("BoardDedupeKey differs for a slug and its host: %q vs %q", a, b)
	}
}

func TestEmplyFetchMapsRealResponses(t *testing.T) {
	f := emplyPaychex(t)
	jobs, err := NewEmply(f).Fetch(context.Background(), CompanyEntry{Board: "job.paychex.eu", Company: "Paychex"})
	if err != nil {
		t.Fatal(err)
	}
	// Six listed, the talent-pool entry is not a vacancy.
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(jobs))
	}
	for _, u := range f.urls {
		if u != "https://job.paychex.eu/api/integration/vacancy/get-page" {
			t.Errorf("listing URL = %q", u)
		}
	}
	if len(f.bodies) != 2 || f.bodies[0].Offset != 0 || f.bodies[1].Offset != 4 {
		t.Errorf("page requests = %+v, want offsets 0 then 4", f.bodies)
	}
	if f.bodies[0].Filters == nil {
		t.Error("filters must be sent as an empty array: the endpoint answers 500 without it")
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if _, ok := byID["moxy5t"]; ok {
		t.Error("the talent-pool entry (unsolicited applications) must not be yielded")
	}

	lead := byID["hmds58"]
	if lead.Title != "Lead Integration Developer (.NET / APIs / Cloud) for Emply" {
		t.Errorf("title = %q", lead.Title)
	}
	if lead.URL != "https://job.paychex.eu/ad/lead-integration-developer-net-apis-cloud-for-emply/hmds58/en" {
		t.Errorf("url = %q", lead.URL)
	}
	if lead.Company != "Paychex" {
		t.Errorf("company = %q", lead.Company)
	}
	if lead.Location != "Østerbro / Lyngbyvej 2, 2100 København, Denmark, 2100, Østerbro, Denmark" {
		t.Errorf("location = %q", lead.Location)
	}
	if lead.PostedAt == nil || lead.PostedAt.Format("2006-01-02") != "2026-08-21" {
		t.Errorf("posted = %v", lead.PostedAt)
	}
	if !strings.Contains(lead.Description, "<") || len(lead.Description) < 1000 {
		t.Errorf("description should be the posting's HTML body, got %d bytes", len(lead.Description))
	}

	// A posting carrying a German and an English text is read in English, and its URL names
	// the language read.
	ae := byID["t8sgwl"]
	if !strings.HasSuffix(ae.URL, "/t8sgwl/en") {
		t.Errorf("url = %q, want the English page", ae.URL)
	}
	if strings.Contains(ae.Description, "Deine Aufgaben") || strings.Contains(ae.Description, " und ") {
		t.Errorf("description should be the English text: %.200q", ae.Description)
	}

	// A posting with no location states none.
	if l := byID["tzfpvg"].Location; l != "" {
		t.Errorf("location = %q, want empty", l)
	}
}

// Without an English text the posting is read in its first (primary) language.
func TestEmplyPicksFirstLanguageWithoutEnglish(t *testing.T) {
	var page emplyPage
	if err := json.Unmarshal([]byte(emplyFixture(t, "molslinjen_first.json")), &page); err != nil {
		t.Fatal(err)
	}
	v := (*page.Vacancies)[0]
	tr := v.translation()
	if tr.LanguageKey != "da-DK" || tr.Title != "ØRESUNDSLINJEN søger passioneret kok" {
		t.Errorf("translation = %s %q, want the da-DK one", tr.LanguageKey, tr.Title)
	}
	job, ok := emplyJob("molslinjen.career.emply.com", "Molslinjen", v)
	if !ok {
		t.Fatal("emplyJob declined a real vacancy")
	}
	if job.URL != "https://molslinjen.career.emply.com/ad/oresundslinjen-soger-passioneret-kok/2exc7k/da" {
		t.Errorf("url = %q", job.URL)
	}
}

// A later page failing must fail the whole crawl: the listing is a full-board listing, and
// returning the first page alone would let the sweep close every posting past it.
func TestEmplyLaterPageFailureFailsCrawl(t *testing.T) {
	f := emplyPaychex(t)
	f.fail = map[int]error{4: &StatusError{Code: 503}}
	if _, err := NewEmply(f).Fetch(context.Background(), CompanyEntry{Board: "job.paychex.eu", Company: "Paychex"}); err == nil {
		t.Fatal("a failed later page must fail the crawl")
	}
}

func TestEmplyFirstPageFailureFailsCrawl(t *testing.T) {
	f := emplyPaychex(t)
	f.fail = map[int]error{0: errors.New("boom")}
	if _, err := NewEmply(f).Fetch(context.Background(), CompanyEntry{Board: "job.paychex.eu", Company: "Paychex"}); err == nil {
		t.Fatal("a failed first page must fail the crawl")
	}
}

// A walk that ends short of the listing's own count did not read the whole board.
func TestEmplyShortWalkFailsCrawl(t *testing.T) {
	f := emplyPaychex(t)
	delete(f.pages, 4) // the second page answers empty while count still says 6
	if _, err := NewEmply(f).Fetch(context.Background(), CompanyEntry{Board: "job.paychex.eu", Company: "Paychex"}); err == nil {
		t.Fatal("an empty page before the declared count must fail the crawl")
	}
}

// A page that repeats postings already read (the listing shifted under the walk) leaves the
// distinct ids short of the count, which is not a full listing either.
func TestEmplyRepeatedPageFailsCrawl(t *testing.T) {
	f := emplyPaychex(t)
	f.pages[4] = f.pages[0]
	if _, err := NewEmply(f).Fetch(context.Background(), CompanyEntry{Board: "job.paychex.eu", Company: "Paychex"}); err == nil {
		t.Fatal("distinct postings short of the declared count must fail the crawl")
	}
}

// A response without a vacancies array is not an empty board.
func TestEmplyMissingVacanciesFailsCrawl(t *testing.T) {
	f := &emplyFake{pages: map[int]string{0: `{"status":500,"title":"Internal Server Error"}`}}
	if _, err := NewEmply(f).Fetch(context.Background(), CompanyEntry{Board: "x", Company: "X"}); err == nil {
		t.Fatal("a response without vacancies must fail the crawl")
	}
}

func TestEmplyEmptyBoardSucceeds(t *testing.T) {
	f := &emplyFake{pages: map[int]string{0: `{"vacancies":[],"count":0}`}}
	jobs, err := NewEmply(f).Fetch(context.Background(), CompanyEntry{Board: "x", Company: "X"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("empty board: jobs=%d err=%v", len(jobs), err)
	}
}
