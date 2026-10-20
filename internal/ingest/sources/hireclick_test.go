package sources

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// The fixtures under testdata/hireclick were recorded from a live tenant (williamsnd) with the
// two things that make these pages megabytes cut out: the base64 payloads of the inlined logo
// images and the ASP.NET __VIEWSTATE value. Neither carries anything the adapter reads.
func hireclickFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/hireclick/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(raw)
}

// hireclickFake serves the recorded board: its job board page, its four-posting list, and two
// posting pages. Of the other two postings, 260964 redirects back to the job board (the
// platform's answer for a removed posting) and 263839 fails with a 5xx (a page this crawl
// could not read).
func hireclickFake(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).
		routeRedirect("/view/260964", "https://williamsnd.hireclick.com/jobboard/").
		routeErr("/view/263839", &StatusError{Method: http.MethodGet, Code: http.StatusBadGateway}).
		route("/view/254235", hireclickFixture(t, "williamsnd_view_254235.html")).
		route("/view/260782", hireclickFixture(t, "williamsnd_view_260782.html")).
		route("GetSettingsAndJobs?orgGUID=FC454F65-3845-46EA-AB2F-9D91956D90AF", hireclickFixture(t, "williamsnd_jobs.json")).
		route("/jobboard/jobboard.aspx", hireclickFixture(t, "williamsnd_jobboard.html"))
}

var hireclickEntry = CompanyEntry{Company: "Williams County", Provider: "hireclick", Board: "williamsnd"}

func TestHireClickProvider(t *testing.T) {
	if got := NewHireClick(nil).Provider(); got != "hireclick" {
		t.Errorf("Provider() = %q, want %q", got, "hireclick")
	}
}

func TestHireClickJobID(t *testing.T) {
	cases := map[string]string{
		"https://williamsnd.hireclick.com/jb/corrections-deputy/view/263839":                    "263839",
		"https://senca.hireclick.com/jb/family-development-associate-nebraska-city/view/264587": "264587",
		"https://williamsnd.hireclick.com/jobboard/":                                            "",
		"https://williamsnd.hireclick.com/jb/generalApplication":                                "",
	}
	for u, want := range cases {
		if got := hireclickJobID(u); got != want {
			t.Errorf("hireclickJobID(%q) = %q, want %q", u, got, want)
		}
	}
}

func TestHireClickFetchMapsPostings(t *testing.T) {
	jobs, err := NewHireClick(hireclickFake(t)).Fetch(context.Background(), hireclickEntry)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs (%v), want 3: two read, one unreadable, the redirected one dropped", len(jobs), byID)
	}

	a, ok := byID["254235"]
	if !ok {
		t.Fatalf("missing job 254235; got %v", byID)
	}
	if a.Title != "Adolescent Care Attendant (2pm - midnight)" {
		t.Errorf("title = %q", a.Title)
	}
	if a.Company != "Williams County" {
		t.Errorf("company = %q, want the configured company", a.Company)
	}
	if a.URL != "https://williamsnd.hireclick.com/jb/adolescent-care-attendant-2pm-midnight/view/254235" {
		t.Errorf("url = %q", a.URL)
	}
	// The address's locality arrives with a trailing space ("WILLISTON "), which must not
	// survive into the stored location.
	if a.Location != "WILLISTON, ND, US" {
		t.Errorf("location = %q, want %q", a.Location, "WILLISTON, ND, US")
	}
	if len(a.Countries) != 1 || a.Countries[0] != "us" {
		t.Errorf("countries = %v, want [us]", a.Countries)
	}
	if a.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time", a.EmploymentType)
	}
	want := time.Date(2026, 7, 17, 12, 36, 49, 0, time.UTC)
	if a.PostedAt == nil || !a.PostedAt.Equal(want) {
		t.Errorf("posted = %v, want %v", a.PostedAt, want)
	}
	if a.Remote || a.WorkMode != "" {
		t.Errorf("remote = %v work mode = %q, want an on-site posting left unknown", a.Remote, a.WorkMode)
	}
	if len(a.Description) < 500 || strings.Contains(a.Description, "<script") {
		t.Errorf("description is not the sanitized full body: %d bytes", len(a.Description))
	}

	if b := byID["260782"]; b.Title != "Human Resources Specialist" || b.Description == "" {
		t.Errorf("job 260782 = title %q, %d-byte description", b.Title, len(b.Description))
	}
	if u := byID["263839"]; !u.Unreadable || u.Company != "Williams County" {
		t.Errorf("job 263839 = %+v, want an unreadable marker for the 5xx page", u)
	}
	if _, ok := byID["260964"]; ok {
		t.Error("job 260964 redirected to the job board and must be dropped as gone")
	}
}

func TestHireClickFetchNewRefreshesSeenPostings(t *testing.T) {
	fake := hireclickFake(t)
	seen := func(id string) bool { return id != "254235" }
	jobs, err := NewHireClick(fake).(HydratingSource).FetchNew(context.Background(), hireclickEntry, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4 (three refreshes, one hydrated)", len(jobs))
	}
	for _, j := range jobs {
		if j.ExternalID == "254235" {
			if j.SeenRefresh || j.Description == "" {
				t.Errorf("unseen posting was not hydrated: %+v", j)
			}
			continue
		}
		if !j.SeenRefresh || j.Description != "" || j.Title == "" {
			t.Errorf("seen posting %s = %+v, want a titled liveness refresh with no body", j.ExternalID, j)
		}
	}
	// Board page + list + the one unseen posting's page.
	if fake.calls != 3 {
		t.Errorf("made %d requests, want 3", fake.calls)
	}
}

func TestHireClickUnknownTenantFails(t *testing.T) {
	// A subdomain no tenant owns lands on the platform's maintenance page, which carries no
	// organisation id; that must fail the board, not read as an empty one.
	fake := (&routedHTTP{}).route("/jobboard/jobboard.aspx", "<html><body><h1>Maintenance</h1></body></html>")
	_, err := NewHireClick(fake).Fetch(context.Background(), CompanyEntry{Board: "nosuchtenant"})
	if err == nil {
		t.Fatal("Fetch succeeded for a board page without an organisation id")
	}
}

func TestHireClickListFailureFailsTheBoard(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("GetSettingsAndJobs", errors.New("connection reset")).
		route("/jobboard/jobboard.aspx", hireclickFixture(t, "williamsnd_jobboard.html"))
	if _, err := NewHireClick(fake).Fetch(context.Background(), hireclickEntry); err == nil {
		t.Fatal("Fetch succeeded although the job list could not be read")
	}
}

// The cap covers every request kind the adapter makes, through ONE pool: with both slots held,
// a board page, a job list and a posting page must all wait rather than reach the origin.
func TestLimitedHireClickSharesOneCapAcrossRequestKinds(t *testing.T) {
	fake := hireclickFake(t)
	g := limitedHireClick(fake).(limitedHireClickGetter)
	if cap(g.sem) != hireclickMaxInFlight {
		t.Fatalf("cap = %d, want %d", cap(g.sem), hireclickMaxInFlight)
	}
	for range hireclickMaxInFlight {
		g.sem <- struct{}{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.GetHTML(ctx, "https://williamsnd.hireclick.com/jobboard/jobboard.aspx"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetHTML error = %v, want context.Canceled", err)
	}
	var raw string
	if err := g.GetJSON(ctx, "https://williamsnd.hireclick.com/api/Controllers/JobBoard/GetSettingsAndJobs?orgGUID=x", &raw); !errors.Is(err, context.Canceled) {
		t.Errorf("GetJSON error = %v, want context.Canceled", err)
	}
	if _, _, err := g.GetHTMLResolved(ctx, "https://williamsnd.hireclick.com/jb/x/view/254235"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetHTMLResolved error = %v, want context.Canceled", err)
	}
	if fake.calls != 0 {
		t.Errorf("origin reached %d times with no free slot", fake.calls)
	}

	// Released slots let a request through, and it releases its own afterwards.
	<-g.sem
	<-g.sem
	if _, err := g.GetHTML(context.Background(), "https://williamsnd.hireclick.com/jobboard/jobboard.aspx"); err != nil {
		t.Fatalf("GetHTML with a free slot: %v", err)
	}
	if len(g.sem) != 0 {
		t.Errorf("slot not released: %d held", len(g.sem))
	}
}
