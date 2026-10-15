package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// ibmFake serves canned search responses keyed by the request's `from` offset; an offset with
// no page answers an empty hit list carrying the same total, which is what the live index
// returns past its end.
type ibmFake struct {
	pages  map[int]string
	fail   map[int]bool
	bodies []map[string]any
}

func (f *ibmFake) PostJSON(_ context.Context, _ string, body, v any) error {
	b := body.(map[string]any)
	f.bodies = append(f.bodies, b)
	from := b["from"].(int)
	if f.fail[from] {
		return errors.New("ibmFake: boom")
	}
	raw, ok := f.pages[from]
	if !ok {
		raw = `{"hits":{"total":{"value":0,"relation":"eq"},"hits":[]}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func ibmFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// ibmSyntheticPage builds a page of n hits with ids starting at first, stating total.
func ibmSyntheticPage(first, n, total int) string {
	hits := make([]string, n)
	for i := range hits {
		hits[i] = fmt.Sprintf(`{"_source":{"field_text_01":%d,"title":"Job %d","body":"Body %d"}}`, first+i, first+i, first+i)
	}
	return fmt.Sprintf(`{"hits":{"total":{"value":%d,"relation":"eq"},"hits":[%s]}}`, total, strings.Join(hits, ","))
}

func TestIBMProviderAndMarkers(t *testing.T) {
	s := NewIBM(nil)
	if got := s.Provider(); got != "ibm" {
		t.Errorf("Provider() = %q, want ibm", got)
	}
	if _, ok := s.(boardless); !ok {
		t.Error("ibm should implement the boardless marker")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("ibm should implement the fullBoardListing marker")
	}
	if !FullBoardListingProviders(All(nil))["ibm"] {
		t.Error("FullBoardListingProviders(All(nil)) should include ibm")
	}
}

// The fixture is a real response (the Italy slice of the index, 2026-10-15): an internship and
// a professional role under "Multiple Cities", and one with a real city.
func TestIBMFetchMapsRealResponse(t *testing.T) {
	f := &ibmFake{pages: map[int]string{0: ibmFixture(t, "ibm_search_italy.json")}}
	jobs, err := NewIBM(f).Fetch(context.Background(), CompanyEntry{Company: "IBM"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	intern := byID["136943"]
	if intern.Title != "Internship Cloud FullStack Developer" || intern.Company != "IBM" {
		t.Errorf("intern = %q / %q", intern.Title, intern.Company)
	}
	if intern.URL != "https://careers.ibm.com/careers/JobDetail?jobId=136943" {
		t.Errorf("URL = %q", intern.URL)
	}
	if intern.EmploymentType != "internship" || intern.Seniority != "intern" {
		t.Errorf("intern employment/seniority = %q/%q", intern.EmploymentType, intern.Seniority)
	}
	if intern.Location != "Italy" || len(intern.Countries) != 1 || intern.Countries[0] != "it" {
		t.Errorf("Multiple Cities location = %q countries %v, want the country alone", intern.Location, intern.Countries)
	}
	if intern.WorkMode != "hybrid" || intern.Remote {
		t.Errorf("work mode = %q remote=%v", intern.WorkMode, intern.Remote)
	}
	if intern.PostedAt == nil || intern.PostedAt.Format("2006-01-02") != "2026-10-13" {
		t.Errorf("PostedAt = %v", intern.PostedAt)
	}
	d := intern.Description
	if strings.Contains(d, "Milano S.r.l") || strings.Contains(d, "Multiple Cities") {
		t.Errorf("facet suffix not stripped: ...%s", d[max(0, len(d)-200):])
	}
	if strings.Contains(d, "â€™") || !strings.Contains(d, "You’ll") {
		t.Error("double-encoded apostrophe not repaired")
	}
	if !strings.Contains(d, "<li>SQL</li>") {
		t.Errorf("inline bullets not rebuilt into a list: ...%s", d[max(0, len(d)-300):])
	}

	if pro := byID["136940"]; pro.Seniority != "" || pro.EmploymentType != "" {
		t.Errorf("Professional must state no seniority, got %q/%q", pro.Seniority, pro.EmploymentType)
	}
	if bari := byID["136151"]; bari.Location != "Bari, Italy" {
		t.Errorf("city location = %q, want %q", bari.Location, "Bari, Italy")
	}
}

// Real response (Netherlands slice): one Remote posting and one stating no work mode, which
// must stay unknown rather than become onsite.
func TestIBMFetchWorkModes(t *testing.T) {
	f := &ibmFake{pages: map[int]string{0: ibmFixture(t, "ibm_search_netherlands.json")}}
	jobs, err := NewIBM(f).Fetch(context.Background(), CompanyEntry{Company: "IBM"})
	if err != nil {
		t.Fatal(err)
	}
	modes := map[string]string{}
	for _, j := range jobs {
		modes[j.ExternalID] = j.WorkMode
		if j.Location != "AMSTERDAM, Netherlands" {
			t.Errorf("%s location = %q", j.ExternalID, j.Location)
		}
	}
	if modes["134069"] != "remote" || modes["131951"] != "" {
		t.Errorf("work modes = %v", modes)
	}
}

func TestIBMSearchBodySortsByUniqueID(t *testing.T) {
	f := &ibmFake{pages: map[int]string{0: ibmFixture(t, "ibm_search_netherlands.json")}}
	if _, err := NewIBM(f).Fetch(context.Background(), CompanyEntry{Company: "IBM"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(f.bodies[0])
	for _, want := range []string{`"sort":[{"id":"asc"}]`, `"size":100`, `"field_text_01"`, `"body"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("request body lacks %s: %s", want, raw)
		}
	}
}

func TestIBMFetchPagesToTheStatedTotal(t *testing.T) {
	f := &ibmFake{pages: map[int]string{
		0:   ibmSyntheticPage(1, 100, 103),
		100: ibmSyntheticPage(101, 3, 103),
	}}
	jobs, err := NewIBM(f).Fetch(context.Background(), CompanyEntry{Company: "IBM"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 103 || len(f.bodies) != 2 {
		t.Errorf("got %d jobs in %d requests, want 103 in 2", len(jobs), len(f.bodies))
	}
}

// A full last page is not proof of the end: the walk asks for the next offset, which comes back
// empty.
func TestIBMFetchExactMultipleOfPageSize(t *testing.T) {
	f := &ibmFake{pages: map[int]string{0: ibmSyntheticPage(1, 100, 100)}}
	f.pages[100] = `{"hits":{"total":{"value":100,"relation":"eq"},"hits":[]}}`
	jobs, err := NewIBM(f).Fetch(context.Background(), CompanyEntry{Company: "IBM"})
	if err != nil || len(jobs) != 100 {
		t.Fatalf("got %d jobs, err %v", len(jobs), err)
	}
}

func TestIBMFetchFailsWhenNotWhole(t *testing.T) {
	cases := map[string]*ibmFake{
		"first page fails": {fail: map[int]bool{0: true}},
		"later page fails": {pages: map[int]string{0: ibmSyntheticPage(1, 100, 150)}, fail: map[int]bool{100: true}},
		// A posting published mid-walk shifts the offsets and re-serves a row already read.
		"count short of total": {pages: map[int]string{
			0:   ibmSyntheticPage(1, 100, 104),
			100: ibmSyntheticPage(100, 4, 104),
		}},
		"inexact total": {pages: map[int]string{0: strings.Replace(ibmSyntheticPage(1, 3, 3), `"eq"`, `"gte"`, 1)}},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := NewIBM(f).Fetch(context.Background(), CompanyEntry{Company: "IBM"}); err == nil {
				t.Errorf("want an error, got %d jobs", len(jobs))
			}
		})
	}
}

func TestIBMLocation(t *testing.T) {
	for _, c := range []struct{ city, country, want string }{
		{"Bangalore, IN", "India", "Bangalore, India"},
		{"Multiple Cities", "United States", "United States"},
		{"", "Canada", "Canada"},
		{"Yorktown Heights, US", "", "Yorktown Heights, US"},
		{"Seoul, KR", "Korea, Republic of", "Seoul, KR"},
	} {
		if got := ibmLocation(c.city, c.country); got != c.want {
			t.Errorf("ibmLocation(%q, %q) = %q, want %q", c.city, c.country, got, c.want)
		}
	}
	if got := ibmCountries("Seoul, KR", "Korea, Republic of"); len(got) != 1 || got[0] != "kr" {
		t.Errorf("ibmCountries must fall back to the city's code, got %v", got)
	}
	if got := ibmCountries("Multiple Cities", "India"); len(got) != 1 || got[0] != "in" {
		t.Errorf("ibmCountries(India) = %v", got)
	}
}
