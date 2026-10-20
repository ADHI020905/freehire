package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// harriFake answers the slug lookup and the posting details from files keyed by URL, and the
// search POST from responses keyed by the request's `start`, the way the platform pages: the
// requested size does not change which posting a start offset begins at, so the fixtures
// (captured at size=4) serve the adapter's own page size.
type harriFake struct {
	mu       sync.Mutex
	gets     map[string]string // url -> raw body
	getErr   map[string]error  // url -> error
	pages    map[int]string    // start -> raw search body
	searches []harriSearchRequest
	asked    []string
}

func (f *harriFake) GetJSON(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.asked = append(f.asked, u)
	f.mu.Unlock()
	if err := f.getErr[u]; err != nil {
		return err
	}
	raw, ok := f.gets[u]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return json.Unmarshal([]byte(raw), v)
}

func (f *harriFake) PostJSON(_ context.Context, _ string, body, v any) error {
	req := body.(harriSearchRequest)
	f.mu.Lock()
	f.searches = append(f.searches, req)
	f.mu.Unlock()
	raw, ok := f.pages[req.Start]
	if !ok {
		raw = `{"data":{"hits":0,"results":[]}}`
	}
	return json.Unmarshal([]byte(raw), v)
}

func harriFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/harri/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

var harriBrindisaIDs = []string{"2843559", "2836139", "2836137", "2835987", "2829995", "2827291", "2825947"}

func harriDetailURL(id string) string {
	return "https://gateway.harri.com/core-reader/api/v1/profile/job/" + id
}

// harriBrindisa is the Brindisa Kitchens career page (brand 4018620, harri.com/
// brindisakitchenscareers) as captured 2026-10-20: seven postings across its restaurants, paged
// at size 4.
func harriBrindisa(t *testing.T) *harriFake {
	f := &harriFake{
		gets: map[string]string{
			"https://gateway.harri.com/core/api/v1/profile/slug/brindisakitchenscareers": harriFixture(t, "brindisakitchenscareers_slug.json"),
		},
		getErr: map[string]error{},
		pages: map[int]string{
			0: harriFixture(t, "brindisakitchenscareers_start0.json"),
			4: harriFixture(t, "brindisakitchenscareers_start4.json"),
		},
	}
	for _, id := range harriBrindisaIDs {
		f.gets[harriDetailURL(id)] = harriFixture(t, "job_"+id+".json")
	}
	return f
}

var harriBrindisaEntry = CompanyEntry{Provider: "harri", Company: "Brindisa Kitchens", Board: "BrindisaKitchensCareers"}

func TestHarriProviderAndMarkers(t *testing.T) {
	s := NewHarri(nil)
	if s.Provider() != "harri" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("harri must be a HydratingSource: the listing carries no body")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("harri proves its walk against the search's hits and must carry fullBoardListing")
	}
	if _, ok := All(nil)["harri"]; !ok {
		t.Error("harri is not registered in All")
	}
}

func TestHarriFetchHydratesTheWholeBrandTree(t *testing.T) {
	f := harriBrindisa(t)
	jobs, err := NewHarri(f).Fetch(context.Background(), harriBrindisaEntry)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != len(harriBrindisaIDs) {
		t.Fatalf("got %d jobs, want %d", len(jobs), len(harriBrindisaIDs))
	}
	// Two search pages, walked by start until the declared hits were read.
	if len(f.searches) != 2 || f.searches[0].Start != 0 || f.searches[1].Start != 4 {
		t.Fatalf("searches = %+v, want start 0 then 4", f.searches)
	}
	if got := f.searches[0].BrandLevelIDs; len(got) != 1 || got[0] != 4018620 {
		t.Errorf("searched brand ids %v, want the slug's brand 4018620", got)
	}

	var waiter Job
	for _, j := range jobs {
		if j.ExternalID == "2829995" {
			waiter = j
		}
		if j.Company != "Brindisa Kitchens" {
			t.Errorf("%s: company %q, want the board's employer, not the store brand", j.ExternalID, j.Company)
		}
		if strings.TrimSpace(j.Description) == "" {
			t.Errorf("%s: empty description", j.ExternalID)
		}
	}
	if waiter.Title != "Waiter" {
		t.Fatalf("posting 2829995 not mapped: %+v", waiter)
	}
	if want := "https://harri.com/TapasBrindisaKitchens-LondonBridge/job/2829995-waiter"; waiter.URL != want {
		t.Errorf("URL = %q, want %q (the posting's own store slug)", waiter.URL, want)
	}
	if waiter.Location != "London, United Kingdom" {
		t.Errorf("Location = %q, want the repeated state folded", waiter.Location)
	}
	if len(waiter.Countries) != 1 || waiter.Countries[0] != "gb" {
		t.Errorf("Countries = %v, want [gb] from the non-ISO \"UK\"", waiter.Countries)
	}
	if want := time.Date(2026, 9, 30, 12, 41, 21, 0, time.UTC); waiter.PostedAt == nil || !waiter.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", waiter.PostedAt, want)
	}
	if waiter.WorkMode != "" || waiter.Remote {
		t.Errorf("an in-restaurant posting got WorkMode %q Remote %v", waiter.WorkMode, waiter.Remote)
	}
}

func TestHarriEmploymentTypeOnlyFromALoneSchedule(t *testing.T) {
	jobs, err := NewHarri(harriBrindisa(t)).Fetch(context.Background(), harriBrindisaEntry)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, j := range jobs {
		got[j.ExternalID] = j.EmploymentType
	}
	// 2836139 states FULL_TIME alone; 2843559 states PART_TIME and FULL_TIME, which says nothing.
	if got["2836139"] != "full_time" {
		t.Errorf("2836139 EmploymentType = %q, want full_time", got["2836139"])
	}
	if got["2843559"] != "" {
		t.Errorf("2843559 EmploymentType = %q, want empty for two schedules", got["2843559"])
	}
}

func TestHarriFetchNewSkipsTheDetailOfSeenPostings(t *testing.T) {
	f := harriBrindisa(t)
	jobs, err := harri{http: f}.FetchNew(context.Background(), harriBrindisaEntry, func(id string) bool { return id != "2829995" })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != len(harriBrindisaIDs) {
		t.Fatalf("got %d jobs, want %d", len(jobs), len(harriBrindisaIDs))
	}
	details := 0
	for _, u := range f.asked {
		if strings.Contains(u, "/profile/job/") {
			details++
		}
	}
	if details != 1 {
		t.Errorf("made %d detail requests, want 1 (only the unseen posting)", details)
	}
	for _, j := range jobs {
		if j.ExternalID == "2829995" {
			if j.SeenRefresh || j.Description == "" {
				t.Errorf("unseen posting not hydrated: %+v", j)
			}
			continue
		}
		if !j.SeenRefresh || j.Description != "" || j.Title == "" {
			t.Errorf("%s: want a body-less SeenRefresh carrying its title, got %+v", j.ExternalID, j)
		}
	}
}

func TestHarriUnknownSlugIsBoardGone(t *testing.T) {
	f := &harriFake{getErr: map[string]error{
		"https://gateway.harri.com/core/api/v1/profile/slug/no-such-brand": &StatusError{Method: "GET", Code: 400},
	}}
	_, err := NewHarri(f).Fetch(context.Background(), CompanyEntry{Company: "X", Board: "no-such-brand"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestHarriRejectsABoardThatIsNotASlug(t *testing.T) {
	for _, board := range []string{"", "harri.com/acme", "acme/job/1"} {
		if _, err := NewHarri(&harriFake{}).Fetch(context.Background(), CompanyEntry{Company: "X", Board: board}); err == nil {
			t.Errorf("board %q accepted", board)
		}
	}
}

func TestHarriFailsAWalkShortOfTheDeclaredHits(t *testing.T) {
	f := harriBrindisa(t)
	delete(f.pages, 4) // the second page answers empty before the seven declared hits are read
	if _, err := NewHarri(f).Fetch(context.Background(), harriBrindisaEntry); err == nil {
		t.Fatal("a walk that read 4 of 7 declared postings succeeded")
	}
}

func TestHarriDetailOutcomes(t *testing.T) {
	t.Run("a gone posting is dropped, the rest kept", func(t *testing.T) {
		f := harriBrindisa(t)
		delete(f.gets, harriDetailURL("2829995")) // the fake answers 404
		jobs, err := NewHarri(f).Fetch(context.Background(), harriBrindisaEntry)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != len(harriBrindisaIDs)-1 {
			t.Errorf("got %d jobs, want %d", len(jobs), len(harriBrindisaIDs)-1)
		}
	})
	t.Run("a posting the platform reports expired is dropped", func(t *testing.T) {
		f := harriBrindisa(t)
		f.gets[harriDetailURL("2829995")] = strings.Replace(f.gets[harriDetailURL("2829995")], `"status": "PUBLISHED"`, `"status": "EXPIRED"`, 1)
		jobs, err := NewHarri(f).Fetch(context.Background(), harriBrindisaEntry)
		if err != nil {
			t.Fatal(err)
		}
		for _, j := range jobs {
			if j.ExternalID == "2829995" {
				t.Error("an EXPIRED posting was yielded")
			}
		}
	})
	t.Run("listing postings and reading none of them fails the crawl", func(t *testing.T) {
		f := harriBrindisa(t)
		for _, id := range harriBrindisaIDs {
			f.getErr[harriDetailURL(id)] = &StatusError{Method: "GET", Code: 503}
		}
		_, err := NewHarri(f).Fetch(context.Background(), harriBrindisaEntry)
		if err == nil || !strings.Contains(err.Error(), "read none") {
			t.Fatalf("err = %v, want the read-none failure", err)
		}
	})
}

// TestHarriRemotePosting hydrates a Wendy's UK district-manager posting whose Timing states
// FULL_TIME + FULLY_REMOTE (captured 2026-10-20).
func TestHarriRemotePosting(t *testing.T) {
	row := harriFixture(t, "remote_row_2845499.json")
	f := &harriFake{
		gets: map[string]string{
			"https://gateway.harri.com/core/api/v1/profile/slug/wendysukcareers": `{"data": {"id": 4358924, "type": "brand", "is_archived": false}, "status_code": 200, "status": "SUCCESS"}`,
			harriDetailURL("2845499"): harriFixture(t, "job_2845499.json"),
		},
		pages: map[int]string{0: fmt.Sprintf(`{"data":{"hits":1,"results":[%s]}}`, row)},
	}
	jobs, err := NewHarri(f).Fetch(context.Background(), CompanyEntry{Company: "Wendy's", Board: "wendysukcareers"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs", len(jobs))
	}
	j := jobs[0]
	if j.WorkMode != "remote" || !j.Remote || j.EmploymentType != "full_time" {
		t.Errorf("WorkMode %q Remote %v EmploymentType %q, want remote/true/full_time", j.WorkMode, j.Remote, j.EmploymentType)
	}
}

func TestHarriTiming(t *testing.T) {
	cases := []struct {
		codes          []string
		mode, schedule string
	}{
		{nil, "", ""},
		{[]string{"FULL_TIME"}, "", "full_time"},
		{[]string{"PART_TIME"}, "", "part_time"},
		{[]string{"PART_TIME", "FULL_TIME"}, "", ""},
		{[]string{"FULL_TIME", "HYBRID_REMOTE"}, "hybrid", "full_time"},
		{[]string{"FULLY_REMOTE", "HYBRID_REMOTE"}, "remote", ""},
		{[]string{"HYBRID_REMOTE", "FULLY_REMOTE"}, "remote", ""},
	}
	for _, c := range cases {
		mode, schedule := harriTiming(c.codes)
		if mode != c.mode || schedule != c.schedule {
			t.Errorf("harriTiming(%v) = (%q, %q), want (%q, %q)", c.codes, mode, schedule, c.mode, c.schedule)
		}
	}
}
