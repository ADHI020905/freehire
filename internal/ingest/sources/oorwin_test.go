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
)

// oorwinFixture reads an answer captured verbatim from api.oorwin.ai (2026-10-26): unicom's
// listing (nine postings), two of their details, the platform's answer for a posting taken down
// and for a portal label it does not know. The details are stored without arrFormData — the
// ~600 KB application form every detail carries, which the adapter does not read.
func oorwinFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/oorwin/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// oorwinFake answers listings through list and details from fixtures keyed by job id; an
// unknown id answers the platform's "no longer active".
type oorwinFake struct {
	t       *testing.T
	list    func(r oorwinListRequest) []byte
	details map[string]string
	errs    map[string]error
	mu      sync.Mutex
	lists   []oorwinListRequest
	viewed  []string
}

func (f *oorwinFake) PostJSON(_ context.Context, url string, body, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r := body.(type) {
	case oorwinListRequest:
		if !strings.HasSuffix(url, "/getJobList") {
			return fmt.Errorf("listing posted to %s", url)
		}
		f.lists = append(f.lists, r)
		return json.Unmarshal(f.list(r), v)
	case oorwinDetailRequest:
		if !strings.HasSuffix(url, "/job_view") {
			return fmt.Errorf("detail posted to %s", url)
		}
		f.viewed = append(f.viewed, r.JobID)
		if err := f.errs[r.JobID]; err != nil {
			return err
		}
		name, ok := f.details[r.JobID]
		if !ok {
			name = "detail_expired.json"
		}
		return json.Unmarshal(oorwinFixture(f.t, name), v)
	}
	return fmt.Errorf("unexpected body %T", body)
}

const (
	oorwinJava = "e648ca1680f191584fa35225228b65a12fc4d821"
	oorwinERP  = "981e59390935346eb112dde77196f36d1ec5d0fb"
)

func oorwinUnicom(t *testing.T) *oorwinFake {
	return &oorwinFake{
		t:    t,
		list: func(oorwinListRequest) []byte { return oorwinFixture(t, "list_unicom.json") },
		details: map[string]string{
			oorwinJava: "detail_" + oorwinJava + ".json",
			oorwinERP:  "detail_" + oorwinERP + ".json",
		},
	}
}

// The catalogue's company is the bare label, as a board seeded from a portal URL carries it;
// the portal's own display name ("Unicom Technologies INC") is what a posting must name.
var oorwinUnicomEntry = CompanyEntry{Company: "unicom", Provider: "oorwin", Board: "unicom"}

func TestOorwinProvider(t *testing.T) {
	s, ok := All(nil)["oorwin"]
	if !ok || s.Provider() != "oorwin" {
		t.Fatal("All() must register oorwin")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("oorwin must be a HydratingSource: the body costs a detail request")
	}
	if !FullBoardListingProviders(All(nil))["oorwin"] {
		t.Error("oorwin's walk is held to the listing's stated total: it must be fullBoardListing")
	}
}

func TestOorwinListingRefreshesSeenPostings(t *testing.T) {
	fake := oorwinUnicom(t)
	jobs, err := NewOorwin(fake).(HydratingSource).FetchNew(context.Background(), oorwinUnicomEntry,
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 9 || len(fake.lists) != 1 || len(fake.viewed) != 0 {
		t.Fatalf("jobs/lists/details = %d/%d/%d, want 9 postings from one listing and no detail",
			len(jobs), len(fake.lists), len(fake.viewed))
	}
	if r := fake.lists[0]; r.SubDomain != "unicom" || r.Page != 1 || r.Limit != oorwinPageSize {
		t.Errorf("listing request = %+v", r)
	}
	for _, j := range jobs {
		if !j.SeenRefresh || len(j.ExternalID) != 40 || j.Company != "Unicom Technologies INC" ||
			j.URL != "https://unicom.oorwin.com/careers/index.html#/job/details/"+j.ExternalID {
			t.Errorf("job = %+v, want an identity-only SeenRefresh", j)
		}
	}
}

func TestOorwinMapsADetail(t *testing.T) {
	fake := oorwinUnicom(t)
	jobs, err := NewOorwin(fake).(HydratingSource).FetchNew(context.Background(), oorwinUnicomEntry,
		func(id string) bool { return id != oorwinJava && id != oorwinERP })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != 9 {
		t.Fatalf("got %d postings, want 9", len(byID))
	}
	j := byID[oorwinJava]
	if j.SeenRefresh || !strings.Contains(j.Description, "Senior Java Developer") {
		t.Fatalf("java = %+v, want a hydrated posting", j)
	}
	if j.Title != "senior Java Developer / Lead Java Developer" || j.Company != "Unicom Technologies INC" {
		t.Errorf("title/company = %q/%q", j.Title, j.Company)
	}
	if j.Location != "Chicago, IL, USA" || len(j.Countries) != 1 || j.Countries[0] != "us" {
		t.Errorf("location/countries = %q/%v", j.Location, j.Countries)
	}
	if j.WorkMode != "onsite" || j.Remote || j.EmploymentType != "contract" || j.ExperienceYearsMin != nil {
		t.Errorf("workMode/remote/type/exp = %q/%v/%q/%v", j.WorkMode, j.Remote, j.EmploymentType, j.ExperienceYearsMin)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02 15:04") != "2026-10-22 13:30" {
		t.Errorf("postedAt = %v", j.PostedAt)
	}
	// A posting that names no city keeps its country alone.
	if erp := byID[oorwinERP]; erp.Location != "USA" || erp.Description == "" {
		t.Errorf("erp = %q / body %d bytes", erp.Location, len(erp.Description))
	}
}

func TestOorwinDropsAPostingTakenDown(t *testing.T) {
	fake := oorwinUnicom(t)
	const gone = "f47df51ff80ec5044c45d5d466a9b5cdb230eb07" // no detail fixture: "no longer active"
	jobs, err := NewOorwin(fake).(HydratingSource).FetchNew(context.Background(), oorwinUnicomEntry,
		func(id string) bool { return id != gone })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	for _, j := range jobs {
		if j.ExternalID == gone {
			t.Fatalf("a posting the platform calls no longer active was yielded: %+v", j)
		}
	}
	if len(jobs) != 8 {
		t.Errorf("got %d postings, want the 8 others", len(jobs))
	}
}

func TestOorwinUnreadableDetailIsMarked(t *testing.T) {
	fake := oorwinUnicom(t)
	fake.errs = map[string]error{oorwinJava: &StatusError{Code: 503}}
	jobs, err := NewOorwin(fake).(HydratingSource).FetchNew(context.Background(), oorwinUnicomEntry,
		func(id string) bool { return id != oorwinJava })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	for _, j := range jobs {
		if j.ExternalID == oorwinJava {
			if !j.Unreadable {
				t.Fatalf("a 503 detail = %+v, want the unreadable marker", j)
			}
			return
		}
	}
	t.Fatal("a posting whose detail failed was dropped instead of marked unreadable")
}

func TestOorwinBoundsHydrationPerRun(t *testing.T) {
	fake := oorwinUnicom(t)
	s := NewOorwin(fake).(oorwin)
	s.maxNewPerRun = 2
	jobs, err := s.FetchNew(context.Background(), oorwinUnicomEntry, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(fake.viewed) != 2 {
		t.Errorf("details = %d, want the per-run bound of 2", len(fake.viewed))
	}
	for _, j := range jobs {
		if j.SeenRefresh {
			t.Errorf("an unseen posting past the bound was yielded as seen: %+v", j)
		}
	}
}

func TestOorwinUnknownBoardIsGone(t *testing.T) {
	fake := &oorwinFake{t: t, list: func(oorwinListRequest) []byte { return oorwinFixture(t, "list_gone.json") }}
	_, err := NewOorwin(fake).Fetch(context.Background(), CompanyEntry{Provider: "oorwin", Board: "accel"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestOorwinRejectsABoardThatIsNotALabel(t *testing.T) {
	fake := oorwinUnicom(t)
	if _, err := NewOorwin(fake).Fetch(context.Background(), CompanyEntry{Provider: "oorwin", Board: "unicom.oorwin.com/careers"}); err == nil {
		t.Fatal("a URL-shaped board was accepted")
	}
	if len(fake.lists) != 0 {
		t.Error("a malformed board reached the API")
	}
}

// A listing that ends short of its own stated total must fail the crawl, never report a
// truncated board: the sweep would read the missing postings as gone.
func TestOorwinShortListingFails(t *testing.T) {
	fake := &oorwinFake{t: t, list: func(r oorwinListRequest) []byte {
		if r.Page == 1 {
			return []byte(`{"success":1,"data":{"list_details":{"total":3,"data":[{"computed_sha1_job_id":"a"},{"computed_sha1_job_id":"b"}]}}}`)
		}
		return []byte(`{"success":1,"data":{"list_details":{"total":3,"data":[]}}}`)
	}}
	if _, err := NewOorwin(fake).Fetch(context.Background(), oorwinUnicomEntry); err == nil {
		t.Fatal("a listing short of its total was reported as a success")
	}
	if len(fake.lists) != 2 {
		t.Errorf("listing pages = %d, want the walk to try page 2 before failing", len(fake.lists))
	}
}

func TestOorwinExperienceMin(t *testing.T) {
	for in, want := range map[string]int{"8-10": 8, "5": 5, "3-3": 3, "0-30": 0, "": 0, "None": 0} {
		got := oorwinExperienceMin(in)
		if (want == 0) != (got == nil) || (got != nil && *got != want) {
			t.Errorf("oorwinExperienceMin(%q) = %v, want %d", in, got, want)
		}
	}
}
