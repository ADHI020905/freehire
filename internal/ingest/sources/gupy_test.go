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

// gupyFixture reads a response captured verbatim from Gupy's live API (2026-10-19).
func gupyFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/gupy/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestGupyProvider(t *testing.T) {
	if got := NewGupy(nil).Provider(); got != "gupy" {
		t.Errorf("Provider() = %q, want %q", got, "gupy")
	}
}

func TestGupyIsAFullBoardListing(t *testing.T) {
	// The career page's listing is one unpaginated response carrying every published
	// posting, so a crawl that returns is the whole board — what licenses the board-scoped
	// sweep to close what disappeared while the old API was down.
	if _, ok := NewGupy(nil).(fullBoardListing); !ok {
		t.Error("gupy must implement fullBoardListing")
	}
}

func TestGupyFetchListsTheCareerPage(t *testing.T) {
	fake := (&routedHTTP{}).
		route("job-publication/public/jobs?subdomain=caju", gupyFixture(t, "list_caju.json")).
		route("job-publication/public/jobs/12691526", gupyFixture(t, "detail_12691526.json"))

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Caju", Provider: "gupy", Board: "caju",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "12691526" {
		t.Errorf("ExternalID = %q, want the Gupy job id", j.ExternalID)
	}
	if j.URL != "https://caju.gupy.io/jobs/12691526" {
		t.Errorf("URL = %q, want the career page's own job URL", j.URL)
	}
	if j.Title != "Pessoa Desenvolvedora FrontEnd Sênior" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Caju" {
		t.Errorf("Company = %q, want the board's company", j.Company)
	}
	if j.Location != "Brasil" {
		t.Errorf("Location = %q, want the country alone when city/state are blank", j.Location)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote/WorkMode = %v/%q, want true/remote", j.Remote, j.WorkMode)
	}
	want := time.Date(2026, 10, 16, 17, 28, 19, 254000000, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want the detail's publishedAt %v", j.PostedAt, want)
	}
	// Every section of the detail is assembled, in board order, with its markup.
	desc := j.Description
	for _, section := range []string{"Desenvolvimento de aplicações web", "Pessoa Engenheira Frontend Sênior", "engenheira de software sênior"} {
		if !strings.Contains(desc, section) {
			t.Errorf("Description missing %q", section)
		}
	}
	if !strings.Contains(desc, "<li>") {
		t.Errorf("Description dropped list markup")
	}
}

func TestGupyFetchMapsAnOnSiteAddress(t *testing.T) {
	fake := (&routedHTTP{}).
		route("subdomain=origoenergiainterno", gupyFixture(t, "list_origoenergiainterno.json")).
		route("job-publication/public/jobs/12679978", gupyFixture(t, "detail_12679978.json"))

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Órigo Energia", Provider: "gupy", Board: "origoenergiainterno",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.Location != "São Paulo, SP, Brasil" {
		t.Errorf("Location = %q, want city, state code, country", j.Location)
	}
	if j.Remote || j.WorkMode != "hybrid" {
		t.Errorf("Remote/WorkMode = %v/%q, want false/hybrid", j.Remote, j.WorkMode)
	}
	if j.URL != "https://origoenergiainterno.gupy.io/jobs/12679978" {
		t.Errorf("URL = %q", j.URL)
	}
}

func TestGupyFetchFailsWhenTheCareerPageIsGone(t *testing.T) {
	// Gupy answers an unknown subdomain with 404 "Career Page not Found". That is a
	// board-level failure, never an empty board: an empty board would license a sweep.
	fake := (&routedHTTP{}).routeErr("subdomain=", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound})

	if _, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Gone", Provider: "gupy", Board: "gone",
	}); err == nil {
		t.Fatal("Fetch succeeded on a missing career page, want an error")
	}
}

func TestGupyFetchDropsAPostingTheDetailSaysIsGone(t *testing.T) {
	// A 404 on the detail is Gupy's own answer that the posting no longer exists.
	fake := (&routedHTTP{}).
		routeErr("job-publication/public/jobs/12691526", &StatusError{Method: http.MethodGet, Code: http.StatusNotFound}).
		route("subdomain=caju", gupyFixture(t, "list_caju.json"))

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{Company: "Caju", Provider: "gupy", Board: "caju"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want the gone posting dropped", len(jobs))
	}
}

func TestGupyFetchMarksAnUnreadableDetail(t *testing.T) {
	// The detail is the posting's only body, so a detail this crawl could not read must not
	// read as "gone" — it becomes an Unreadable marker the pipeline counts.
	fake := (&routedHTTP{}).
		routeErr("job-publication/public/jobs/12691526", errors.New("timeout")).
		route("subdomain=caju", gupyFixture(t, "list_caju.json"))

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{Company: "Caju", Provider: "gupy", Board: "caju"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Unreadable {
		t.Fatalf("jobs = %+v, want one Unreadable marker", jobs)
	}
	if jobs[0].ExternalID != "12691526" || jobs[0].URL != "https://caju.gupy.io/jobs/12691526" {
		t.Errorf("marker identity = %q %q", jobs[0].ExternalID, jobs[0].URL)
	}
}

func TestGupyFetchMarksADetailWithNoBody(t *testing.T) {
	// A 200 whose sections are all placeholders carries no posting body; storing it
	// body-less would never be repaired, so it is a marker too.
	fake := (&routedHTTP{}).
		route("subdomain=caju", gupyFixture(t, "list_caju.json")).
		route("job-publication/public/jobs/12691526", `{"description":"<p>.</p>","responsibilities":"","prerequisites":"<p>&nbsp;</p>"}`)

	jobs, err := NewGupy(fake).Fetch(context.Background(), CompanyEntry{Company: "Caju", Provider: "gupy", Board: "caju"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].Unreadable {
		t.Fatalf("jobs = %+v, want one Unreadable marker", jobs)
	}
}
