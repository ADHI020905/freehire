package sources

import (
	"context"
	"encoding/xml"
	"os"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live work180.com responses fetched from the crawl host on
// 2026-10-18: the AU jobs sitemap's header with its first search entry and three posting entries
// verbatim, and each posting page's ld+json blocks verbatim (the JobPosting body cut short) with
// the one RSC payload line that carries the posting's own record.

const (
	work180AUSitemap = "https://work180.com/en-au/jobs-sitemap.xml"
	work180AUPage    = "https://work180.com/en-au/for-women/employer/transgrid/job/1083968/manager-business-improvement-and-strategic-in"
	work180USPage    = "https://work180.com/en-us/for-women/employer/comparative/job/535122/senior-software-developer-full-stack"
)

func work180Fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func work180Parse(t *testing.T, body string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// work180Fake serves fixed sitemaps and pages by URL; an unknown URL is a 404.
type work180Fake struct {
	mu    sync.Mutex
	xml   map[string]string
	pages map[string]string
	errs  map[string]error
	got   []string
}

func (f *work180Fake) GetXML(_ context.Context, url string, v any) error {
	body, ok := f.xml[url]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: url}
	}
	return xml.Unmarshal([]byte(body), v)
}

func (f *work180Fake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
	if err, ok := f.errs[url]; ok {
		return nil, err
	}
	body, ok := f.pages[url]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: url}
	}
	return html.Parse(strings.NewReader(body))
}

func newWORK180Fake(t *testing.T) *work180Fake {
	return &work180Fake{
		xml:   map[string]string{work180AUSitemap: work180Fixture(t, "work180_jobs_sitemap_au.xml")},
		pages: map[string]string{work180AUPage: work180Fixture(t, "work180_detail_au.html")},
	}
}

func TestWORK180ListReadsPostingURLsNewestFirst(t *testing.T) {
	f := newWORK180Fake(t)
	list, err := work180{sitemap: f, pages: f}.list(context.Background(), "au")
	if err != nil {
		t.Fatal(err)
	}
	// The sitemap's search category entry is not a posting.
	want := []work180Listing{
		{ID: 1083968, URL: work180AUPage, EmployerSlug: "transgrid"},
		{ID: 1071913, URL: "https://work180.com/en-au/for-women/employer/cummins-australia/job/1071913/technician---field-service", EmployerSlug: "cummins-australia"},
		{ID: 21552, URL: "https://work180.com/en-au/for-women/employer/queensland-fire-department/job/21552/auxiliary-firefighter---boulia", EmployerSlug: "queensland-fire-department"},
	}
	if len(list) != len(want) {
		t.Fatalf("list = %+v, want %+v", list, want)
	}
	for i := range want {
		if list[i] != want[i] {
			t.Errorf("list[%d] = %+v, want %+v", i, list[i], want[i])
		}
	}
}

func TestWORK180ListRejectsAnUnknownBoardAndAnEmptySitemap(t *testing.T) {
	f := newWORK180Fake(t)
	s := work180{sitemap: f, pages: f}
	if _, err := s.list(context.Background(), "nz"); err == nil {
		t.Error("an unknown board was crawled")
	}
	f.xml["https://work180.com/en-gb/jobs-sitemap.xml"] = `<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` +
		`<url><loc>https://work180.com/en-gb/search/engineering</loc></url></urlset>`
	if _, err := s.list(context.Background(), "uk"); err == nil {
		t.Error("a sitemap naming no posting read as an empty region")
	}
}

func TestWORK180JobURLKeepsOnlyTheBoardsOwnLocale(t *testing.T) {
	if _, ok := parseWORK180JobURL(work180USPage, "en-au"); ok {
		t.Error("an en-us posting was accepted on the au board")
	}
	if l, ok := parseWORK180JobURL(work180USPage, "en-us"); !ok || l.ID != 535122 || l.EmployerSlug != "comparative" {
		t.Errorf("en-us posting = %+v, %v", l, ok)
	}
	for _, raw := range []string{
		"https://work180.com/en-us/for-women/employer/comparative",
		"https://work180.com/en-us/search/engineering",
		"http://work180.com/en-us/for-women/employer/comparative/job/535122/x",
		"https://example.com/en-us/for-women/employer/comparative/job/535122/x",
	} {
		if l, ok := parseWORK180JobURL(raw, "en-us"); ok {
			t.Errorf("%s read as posting %+v", raw, l)
		}
	}
}

func TestWORK180PostingMapsTheJobPostingAndTheApplicationLink(t *testing.T) {
	l := work180Listing{ID: 1083968, URL: work180AUPage, EmployerSlug: "transgrid"}
	job, ok := parseWORK180Posting(work180Parse(t, work180Fixture(t, "work180_detail_au.html")), l)
	if !ok {
		t.Fatal("posting not parsed")
	}
	if job.ExternalID != "1083968" || job.Title != "Manager, Business Improvement and Strategic Initiatives" || job.Company != "Transgrid" {
		t.Errorf("identity = %q %q %q", job.ExternalID, job.Title, job.Company)
	}
	// An EXTERNAL application names the employer's own ATS posting.
	if job.URL != "https://careers.transgrid.com.au/job/Sydney-Manager%2C-Business-Improvement-and-Strategic-Initiatives/1368929566/" {
		t.Errorf("URL = %q", job.URL)
	}
	if job.Location != "Sydney, NSW, Australia" {
		t.Errorf("Location = %q", job.Location)
	}
	if job.EmploymentType != "full_time" || job.WorkMode != "" || job.Remote {
		t.Errorf("facets = %q %q %v", job.EmploymentType, job.WorkMode, job.Remote)
	}
	if job.PostedAt == nil || job.PostedAt.UTC().Format("2006-01-02T15:04") != "2026-10-16T01:30" {
		t.Errorf("PostedAt = %v", job.PostedAt)
	}
	if !strings.Contains(job.Description, "LUMEA") || len(job.Countries) != 0 {
		t.Errorf("description/countries = %q %v", job.Description, job.Countries)
	}
}

func TestWORK180RemotePostingStatesNoPlace(t *testing.T) {
	l := work180Listing{ID: 535122, URL: work180USPage, EmployerSlug: "comparative"}
	job, ok := parseWORK180Posting(work180Parse(t, work180Fixture(t, "work180_detail_us_remote.html")), l)
	if !ok {
		t.Fatal("posting not parsed")
	}
	if !job.Remote || job.WorkMode != "remote" || job.Location != "" {
		t.Errorf("remote = %v %q, location %q", job.Remote, job.WorkMode, job.Location)
	}
	if job.URL != "https://careers.comparative.ai/jobs/1544181-senior-software-developer-full-stack" {
		t.Errorf("URL = %q", job.URL)
	}
}

func TestWORK180PostingRefusesAnotherPostingsPageAndAClosedOne(t *testing.T) {
	page := work180Fixture(t, "work180_detail_au.html")
	other := work180Listing{ID: 1071913, URL: work180AUPage, EmployerSlug: "transgrid"}
	if _, ok := parseWORK180Posting(work180Parse(t, page), other); ok {
		t.Error("a page rendering another posting was stored under the sitemap's id")
	}
	l := work180Listing{ID: 1083968, URL: work180AUPage, EmployerSlug: "transgrid"}
	expired := strings.Replace(page, `\"expired\":null`, `\"expired\":\"2026-10-17T00:00:00Z\"`, 1)
	if expired == page {
		t.Fatal("fixture lost its expired field")
	}
	if _, ok := parseWORK180Posting(work180Parse(t, expired), l); ok {
		t.Error("an expired posting was yielded")
	}
}

func TestWORK180EmailApplicationKeepsTheWORK180Page(t *testing.T) {
	page := strings.Replace(work180Fixture(t, "work180_detail_au.html"), `\"applicationType\":\"EXTERNAL\"`, `\"applicationType\":\"EMAIL\"`, 1)
	l := work180Listing{ID: 1083968, URL: work180AUPage, EmployerSlug: "transgrid"}
	job, ok := parseWORK180Posting(work180Parse(t, page), l)
	if !ok {
		t.Fatal("posting not parsed")
	}
	if job.URL != work180AUPage {
		t.Errorf("URL = %q, want the WORK180 page", job.URL)
	}
}

func TestWORK180GatedCrawlSpendsPagesOnlyOnUncoveredNewPostings(t *testing.T) {
	f := newWORK180Fake(t)
	s := work180{sitemap: f, pages: f, maxNewPerBoard: 10}
	seen := func(id string) bool { return id == "21552" }
	var asked []string
	gate := DetailGate{Covered: func(c []string) map[string]bool {
		asked = c
		return map[string]bool{"cummins-australia": true}
	}}
	jobs, err := s.FetchNewGated(context.Background(), CompanyEntry{Provider: "work180", Board: "au"}, seen, gate)
	if err != nil {
		t.Fatal(err)
	}
	// The coverage probe is asked about the new postings' employers only.
	if strings.Join(asked, ",") != "transgrid,cummins-australia" {
		t.Errorf("coverage asked about %v", asked)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if j := byID["21552"]; !j.SeenRefresh {
		t.Errorf("seen posting = %+v, want a refresh", j)
	}
	if j := byID["1071913"]; j.SeenRefresh || j.Description != "" || j.Company != "cummins-australia" {
		t.Errorf("covered posting = %+v, want body-less under the slug the probe was asked about", j)
	}
	if j := byID["1083968"]; j.Company != "Transgrid" || j.Description == "" {
		t.Errorf("new posting = %+v, want hydrated", j)
	}
	if len(f.got) != 1 || f.got[0] != work180AUPage {
		t.Errorf("pages fetched = %v, want only the uncovered new posting", f.got)
	}
}

func TestWORK180CapHydratesTheNewestPostingsAndDefersTheRest(t *testing.T) {
	f := newWORK180Fake(t)
	s := work180{sitemap: f, pages: f, maxNewPerBoard: 1}
	jobs, err := s.FetchNew(context.Background(), CompanyEntry{Board: "au"}, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 1 || f.got[0] != work180AUPage {
		t.Errorf("pages fetched = %v, want only the newest", f.got)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "1083968" {
		t.Errorf("jobs = %+v, want the newest posting only (the rest deferred, not stored body-less)", jobs)
	}
}

func TestWORK180CrawlThatReadsNoPageFails(t *testing.T) {
	f := newWORK180Fake(t)
	f.errs = map[string]error{work180AUPage: &StatusError{Method: "GET", Code: 403, URL: work180AUPage}}
	s := work180{sitemap: f, pages: f, maxNewPerBoard: 10}
	if _, err := s.FetchNew(context.Background(), CompanyEntry{Board: "au"}, func(string) bool { return false }); err == nil {
		t.Error("a crawl that read none of its new postings succeeded")
	}
}

func TestWORK180IsARegisteredCoverageGatedAggregatorWithBoards(t *testing.T) {
	src, ok := Taxonomy()["work180"]
	if !ok {
		t.Fatal("work180 is not registered")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("work180 is not an aggregator")
	}
	if _, ok := src.(boardless); ok {
		t.Error("work180 is boardless; its board selects the regional site")
	}
	if _, ok := src.(CoverageGated); !ok {
		t.Error("work180 is not CoverageGated")
	}
	if _, ok := src.(fullBoardListing); ok {
		t.Error("work180 claims fullBoardListing without proof")
	}
}
