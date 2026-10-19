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
	"time"
)

// The home page and app bundle excerpts are real (2026-10-18), cut down to the script tag and
// the Supabase client's construction; the anon key is replaced so a secret scanner has nothing
// to flag. The rows in testdata/golangjobs/jobs.json are a real PostgREST answer to the
// adapter's own select, for nine ids chosen to cover each mapping branch.
const (
	golangJobsTestPage = `<html><head><script async type="module" crossorigin src="/assets/index-BjPsTFwM.js"></script>` +
		`<link rel="stylesheet" crossorigin href="/assets/index-DY-9RXtM.css"></head><body><div id="root"></div></body></html>`
	golangJobsTestBundle = `this.changedAccessToken=s}}const jn=new vn("https://mvjyjzestmcxxmmmakec.supabase.co",` +
		`"eyJTEST.eyJTEST.TEST");function bn(e){`
	golangJobsTestBase = "https://mvjyjzestmcxxmmmakec.supabase.co/rest/v1/jobs?"
)

// golangJobsTestNow is the clock the fixture is read against: the day after its newest posting,
// which puts the age cutoff between the 2026-08-10 row (kept) and the 2026-08-01 one (dropped).
var golangJobsTestNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

// golangJobsFakeHTTP plays the home page, the app bundle and the PostgREST endpoint. pageSize
// overrides the server's max-rows so a multi-page walk can be exercised with the fixture.
type golangJobsFakeHTTP struct {
	rows     []json.RawMessage
	pageSize int
	// counts are the counts answered in turn (the last repeats); nil answers len(rows).
	counts      []int
	failOffset  int // fail the page read at this offset; -1 never
	bundle      string
	headers     map[string]string
	offsetsRead []int
}

func newGolangJobsFake(t *testing.T) *golangJobsFakeHTTP {
	t.Helper()
	raw, err := os.ReadFile("testdata/golangjobs/jobs.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return &golangJobsFakeHTTP{rows: rows, pageSize: golangJobsPageSize, failOffset: -1, bundle: golangJobsTestBundle}
}

func (f *golangJobsFakeHTTP) GetText(_ context.Context, u string) (string, error) {
	switch u {
	case golangJobsHomeURL:
		return golangJobsTestPage, nil
	case "https://www.golangjobs.tech/assets/index-BjPsTFwM.js":
		return f.bundle, nil
	}
	return "", fmt.Errorf("unexpected GetText %s", u)
}

func (f *golangJobsFakeHTTP) GetJSONWithHeaders(_ context.Context, u string, headers map[string]string, v any) error {
	if !strings.HasPrefix(u, golangJobsTestBase) {
		return fmt.Errorf("unexpected URL %s", u)
	}
	f.headers = headers
	q, err := url.ParseQuery(strings.TrimPrefix(u, golangJobsTestBase))
	if err != nil {
		return err
	}
	if q.Get("is_archived") != "eq.false" {
		return fmt.Errorf("archived postings not excluded: %s", u)
	}
	var out any
	if q.Get("select") == "count" {
		n := len(f.rows)
		if len(f.counts) > 0 {
			n = f.counts[0]
			if len(f.counts) > 1 {
				f.counts = f.counts[1:]
			}
		}
		out = []map[string]int{{"count": n}}
	} else {
		offset, _ := strconv.Atoi(q.Get("offset"))
		f.offsetsRead = append(f.offsetsRead, offset)
		if offset == f.failOffset {
			return &StatusError{Method: http.MethodGet, Code: http.StatusBadGateway, URL: u}
		}
		lo := min(offset, len(f.rows))
		hi := min(lo+f.pageSize, len(f.rows))
		out = f.rows[lo:hi]
	}
	b, _ := json.Marshal(out)
	return json.Unmarshal(b, v)
}

func newGolangJobsAt(f golangJobsHTTP) Source {
	return golangJobs{http: f, now: func() time.Time { return golangJobsTestNow }}
}

func TestGolangJobsFetchMapsTheTable(t *testing.T) {
	f := newGolangJobsFake(t)
	jobs, err := newGolangJobsAt(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if f.headers["apikey"] != "eyJTEST.eyJTEST.TEST" || f.headers["Authorization"] != "Bearer eyJTEST.eyJTEST.TEST" {
		t.Errorf("anon key not sent from the bundle: %v", f.headers)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	// The two 2025 postings and the 2026-08-01 one are past golangJobsMaxAge; six are kept.
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs, want 6", len(jobs))
	}
	for _, old := range []string{
		"e9116aca-ae9e-42ac-aaa8-a5f28619c3b6", "7f298f27-d34d-4ec4-8455-b3360083c8a5", "166c806f-f75e-4f60-b35a-40bb43687f5d",
	} {
		if _, ok := byID[old]; ok {
			t.Errorf("stale posting %s kept", old)
		}
	}

	c := byID["1454bf7a-21e4-49f3-8127-c389ee5b3074"]
	if c.Title != "Senior/Staff Platform Engineer" || c.Company != "CodeRabbit" {
		t.Errorf("identity: %q / %q", c.Title, c.Company)
	}
	if c.URL != "https://jobs.ashbyhq.com/coderabbit/2dcba427-d10e-42c4-9a3d-028aaca82977" {
		t.Errorf("the posting's own link not used: %s", c.URL)
	}
	if c.Location != "San Francisco, United States" || len(c.Countries) != 1 || c.Countries[0] != "us" {
		t.Errorf("place: %q %v", c.Location, c.Countries)
	}
	if c.WorkMode != "" || c.Remote || c.EmploymentType != "full_time" || !c.IsTechHint {
		t.Errorf("facets: mode=%q remote=%v type=%q tech=%v", c.WorkMode, c.Remote, c.EmploymentType, c.IsTechHint)
	}
	if len(c.Skills) != 1 || c.Skills[0] != "go" {
		t.Errorf("skills: %v", c.Skills)
	}
	if c.SalaryMin == nil || *c.SalaryMin != 220000 || c.SalaryMax == nil || *c.SalaryMax != 280000 ||
		c.SalaryCurrency != "USD" || c.SalaryPeriod != "year" {
		t.Errorf("salary: %v-%v %s/%s", c.SalaryMin, c.SalaryMax, c.SalaryCurrency, c.SalaryPeriod)
	}
	if c.PostedAt == nil || c.PostedAt.UTC().Format(time.RFC3339) != "2026-09-09T15:08:49Z" {
		t.Errorf("posted: %v", c.PostedAt)
	}
	if !strings.Contains(c.Description, "CodeRabbit") || strings.Contains(c.Description, "<script") {
		t.Errorf("description: %.200s", c.Description)
	}

	// A remote posting with a global scope states no country.
	g := byID["f0aa72be-a6e0-4c01-9c27-998f1c88a439"]
	if g.Location != "Remote" || g.Countries != nil || g.WorkMode != "remote" || !g.Remote {
		t.Errorf("global remote: %q %v %q %v", g.Location, g.Countries, g.WorkMode, g.Remote)
	}
	// A remote posting scoped to a country keeps it.
	r := byID["cc26513b-9089-4971-9fe4-176f2d91e8bf"]
	if r.Location != "Remote, India" || len(r.Countries) != 1 || r.Countries[0] != "in" || r.WorkMode != "remote" {
		t.Errorf("country remote: %q %v %q", r.Location, r.Countries, r.WorkMode)
	}
	if r.SalaryMin == nil || *r.SalaryMin != 200000 || r.SalaryCurrency != "INR" {
		t.Errorf("INR salary: %v %s", r.SalaryMin, r.SalaryCurrency)
	}
	// A row with no salary bound carries no currency, even though salary_currency defaults to
	// USD on every row.
	if n := byID["0d9933eb-8c0b-4a66-87b8-426bf4e84528"]; n.SalaryMin != nil || n.SalaryMax != nil || n.SalaryCurrency != "" {
		t.Errorf("salary without a bound: %v %v %q", n.SalaryMin, n.SalaryMax, n.SalaryCurrency)
	}
	// One job re-posted as two rows under one link: neither row claims the link.
	for id, slug := range map[string]string{
		"e495c526-8315-48cd-a08e-97834d7c6f03": "golang-jobs-at-happl-e495c526",
		"6e68367a-59a9-4e46-8b6d-eb87c06b05fb": "golang-jobs-at-happl-6e68367a",
	} {
		if u := byID[id].URL; u != golangJobsPostingURL+slug {
			t.Errorf("shared link used for %s: %s", id, u)
		}
	}
}

func TestGolangJobsWalksEveryPage(t *testing.T) {
	// A server whose max-rows is below the adapter's page size answers short pages; the walk
	// advances by the rows it received, so a smaller clamp costs pages, never rows.
	f := newGolangJobsFake(t)
	f.pageSize = 4
	jobs, err := newGolangJobsAt(f).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 || fmt.Sprint(f.offsetsRead) != "[0 4 8]" {
		t.Errorf("got %d jobs over offsets %v, want 6 over [0 4 8]", len(jobs), f.offsetsRead)
	}
}

func TestGolangJobsEmploymentType(t *testing.T) {
	// The board's posting form offers exactly these four types.
	for in, want := range map[string]string{
		"full-time": "full_time", "part-time": "part_time", "contract": "contract", "internship": "internship", "": "",
	} {
		if got := golangJobsEmploymentType(in); got != want {
			t.Errorf("golangJobsEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGolangJobsFailsAnIncompleteRead(t *testing.T) {
	cases := map[string]func(f *golangJobsFakeHTTP){
		"a later page fails":     func(f *golangJobsFakeHTTP) { f.pageSize, f.failOffset = 4, 4 },
		"the first page fails":   func(f *golangJobsFakeHTTP) { f.failOffset = 0 },
		"fewer read than listed": func(f *golangJobsFakeHTTP) { f.counts = []int{10} },
		"the table changed":      func(f *golangJobsFakeHTTP) { f.counts = []int{9, 10} },
		"an empty table":         func(f *golangJobsFakeHTTP) { f.rows, f.counts = nil, []int{0} },
		"no Supabase client":     func(f *golangJobsFakeHTTP) { f.bundle = "var d={}" },
		"a non-Supabase project": func(f *golangJobsFakeHTTP) { f.bundle = strings.Replace(f.bundle, "supabase.co", "example.com", 1) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newGolangJobsFake(t)
			mutate(f)
			if jobs, err := newGolangJobsAt(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestGolangJobsIsAFullCatalogAggregator(t *testing.T) {
	src := NewGolangJobs(nil)
	if src.Provider() != "golangjobs" {
		t.Errorf("Provider() = %q", src.Provider())
	}
	if _, ok := src.(boardless); !ok {
		t.Error("golangjobs is not boardless")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("golangjobs is not an aggregator")
	}
	if _, ok := src.(fullCatalog); !ok {
		t.Error("golangjobs does not claim fullCatalog")
	}
	if _, ok := Taxonomy()["golangjobs"]; !ok {
		t.Error("golangjobs is not registered")
	}
}
