package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
)

// kalibrrFakeHTTP serves the trimmed real search responses (2026-10-15) by the country the
// request names, after mutate has had its way with the decoded body.
type kalibrrFakeHTTP struct {
	t      *testing.T
	mutate func(body map[string]any)
	asked  []url.Values
}

func (f *kalibrrFakeHTTP) GetJSON(_ context.Context, u string, v any) error {
	parsed, err := url.Parse(u)
	if err != nil || !strings.HasPrefix(u, kalibrrSearchURL+"?") {
		return fmt.Errorf("unexpected GetJSON %s", u)
	}
	q := parsed.Query()
	f.asked = append(f.asked, q)
	raw, err := os.ReadFile("testdata/kalibrr/" + strings.ToLower(q.Get("country")) + "_it.json")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		f.t.Fatal(err)
	}
	if f.mutate != nil {
		f.mutate(body)
	}
	out, _ := json.Marshal(body)
	return json.Unmarshal(out, v)
}

func kalibrrFetch(t *testing.T, board string, mutate func(map[string]any)) ([]Job, *kalibrrFakeHTTP, error) {
	t.Helper()
	f := &kalibrrFakeHTTP{t: t, mutate: mutate}
	jobs, err := NewKalibrr(f).Fetch(context.Background(), CompanyEntry{Board: board})
	return jobs, f, err
}

func kalibrrByID(jobs []Job) map[string]Job {
	out := map[string]Job{}
	for _, j := range jobs {
		out[j.ExternalID] = j
	}
	return out
}

func TestKalibrrFetchMapsTheBoard(t *testing.T) {
	jobs, f, err := kalibrrFetch(t, "Indonesia", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(jobs))
	}
	q := f.asked[0]
	if q.Get("country") != "Indonesia" || q.Get("function") != "IT and Software" ||
		q.Get("limit") != "2000" || q.Get("offset") != "0" {
		t.Errorf("query: %v", q)
	}
	byID := kalibrrByID(jobs)

	bca := byID["196084"]
	if bca.Title != "IT ENGINEER" || bca.Company != "PT Bank Central Asia Tbk" || !bca.IsTechHint {
		t.Errorf("identity: %q / %q tech=%v", bca.Title, bca.Company, bca.IsTechHint)
	}
	if !strings.HasPrefix(bca.URL, "https://karir.bca.co.id/candidate/id/career/it-engineer") {
		t.Errorf("employer apply link not used: %s", bca.URL)
	}
	if len(bca.Countries) != 1 || bca.Countries[0] != "id" || !strings.HasSuffix(bca.Location, "Indonesia") {
		t.Errorf("place: %q %v", bca.Location, bca.Countries)
	}
	if bca.WorkMode != "" || bca.EmploymentType != "full_time" || bca.PostedAt == nil {
		t.Errorf("facets: mode=%q type=%q posted=%v", bca.WorkMode, bca.EmploymentType, bca.PostedAt)
	}

	if j := byID["272230"]; j.WorkMode != "hybrid" || j.Remote {
		t.Errorf("hybrid: %q remote=%v", j.WorkMode, j.Remote)
	}
	astro := byID["265065"]
	if astro.WorkMode != "remote" || !astro.Remote || astro.EmploymentType != "contract" {
		t.Errorf("remote contract: %q %v %q", astro.WorkMode, astro.Remote, astro.EmploymentType)
	}
	if astro.URL != "https://www.kalibrr.com/c/astro-technologies-indonesia/jobs/265065/product-designer" {
		t.Errorf("posting page: %s", astro.URL)
	}
	// A salary the employer hides still carries its figures; it is not published.
	if j := byID["273215"]; j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" {
		t.Errorf("hidden salary published: %v %v %q", j.SalaryMin, j.SalaryMax, j.SalaryCurrency)
	}
	for _, j := range jobs {
		if strings.TrimSpace(j.Description) == "" {
			t.Errorf("%s has no body", j.ExternalID)
		}
	}
}

func TestKalibrrSalaryAndSharedLinks(t *testing.T) {
	jobs, _, err := kalibrrFetch(t, "Philippines", nil)
	if err != nil {
		t.Fatal(err)
	}
	byID := kalibrrByID(jobs)
	m := byID["263258"]
	if m.SalaryMin == nil || *m.SalaryMin != 100000 || m.SalaryMax == nil || *m.SalaryMax != 150000 ||
		m.SalaryCurrency != "PHP" || m.SalaryPeriod != "month" {
		t.Errorf("salary: %v-%v %s/%s", m.SalaryMin, m.SalaryMax, m.SalaryCurrency, m.SalaryPeriod)
	}
	// One Google Form behind two postings is neither posting's own page, and a redirector.
	for _, id := range []string{"271382", "273103"} {
		if u := byID[id].URL; !strings.HasPrefix(u, "https://www.kalibrr.com/c/") {
			t.Errorf("%s: shared form used: %s", id, u)
		}
	}
}

func TestKalibrrFailsAnUntrustworthyBoard(t *testing.T) {
	first := func(body map[string]any) map[string]any { return body["jobs"].([]any)[0].(map[string]any) }
	cases := map[string]struct {
		board  string
		mutate func(map[string]any)
	}{
		"not a country":          {"indonesia; drop", nil},
		"fewer rows than listed": {"Indonesia", func(b map[string]any) { b["count"] = 6.0 }},
		"no count":               {"Indonesia", func(b map[string]any) { delete(b, "count") }},
		"empty board":            {"Indonesia", func(b map[string]any) { b["count"], b["jobs"] = 0.0, []any{} }},
		"over the limit":         {"Indonesia", func(b map[string]any) { b["count"] = 2001.0 }},
		"function ignored":       {"Indonesia", func(b map[string]any) { first(b)["function"] = "Arts and Sports" }},
		"country ignored": {"Indonesia", func(b map[string]any) {
			first(b)["google_location"] = map[string]any{"address_components": map[string]any{"country": "Philippines"}}
		}},
		"a posting twice": {"Indonesia", func(b map[string]any) {
			jobs := b["jobs"].([]any)
			jobs[1].(map[string]any)["id"] = jobs[0].(map[string]any)["id"]
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, _, err := kalibrrFetch(t, c.board, c.mutate); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestKalibrrIsAFullBoardListingAggregator(t *testing.T) {
	src := NewKalibrr(nil)
	if _, ok := src.(aggregator); !ok {
		t.Error("kalibrr is not an aggregator")
	}
	if _, ok := src.(boardless); ok {
		t.Error("kalibrr's board is a country market, it is not boardless")
	}
	if _, ok := src.(fullBoardListing); !ok {
		t.Error("kalibrr does not claim fullBoardListing")
	}
	if _, ok := Taxonomy()["kalibrr"]; !ok {
		t.Error("kalibrr is not registered")
	}
}
