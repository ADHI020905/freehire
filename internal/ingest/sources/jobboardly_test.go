package sources

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestJobboardlyMarkers(t *testing.T) {
	s := NewJobboardly(nil)
	if s.Provider() != "jobboardly" {
		t.Errorf("Provider() = %q, want jobboardly", s.Provider())
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("jobboardly lists many employers and must be an aggregator")
	}
	if _, ok := s.(boardless); ok {
		t.Error("jobboardly is addressed by tenant subdomain and must not be boardless")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("jobboardly reads one unpaginated catalogue and should be fullBoardListing")
	}
	if _, ok := All(nil)["jobboardly"]; !ok {
		t.Error("All() should register provider jobboardly")
	}
	if !slices.Contains(FilterableProviders(), "jobboardly") {
		t.Error("FilterableProviders() should include jobboardly")
	}
}

// jobboardlyFixture is four postings taken verbatim from two live boards' /jobs.json
// (kubernetes-jobs, hire-mamas; 2026-10-17), bodies trimmed.
func jobboardlyFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/jobboardly/jobs.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestJobboardlyFetchMapsPostings(t *testing.T) {
	fake := (&routedHTTP{}).route("https://kubernetes-jobs.jobboardly.com/jobs.json", jobboardlyFixture(t))
	jobs, err := NewJobboardly(fake).Fetch(context.Background(), CompanyEntry{Provider: "jobboardly", Board: "Kubernetes-Jobs", Company: "Kubernetes Jobs"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4", len(jobs))
	}

	onsite := jobs[0]
	if onsite.ExternalID != "kubernetes-platform-engineer-1da9413c" ||
		onsite.URL != "https://kubernetesjobs.com/jobs/kubernetes-platform-engineer-1da9413c" {
		t.Errorf("identity = %q / %q", onsite.ExternalID, onsite.URL)
	}
	if onsite.Company != "Booz Allen Hamilton" || onsite.Title != "Kubernetes Platform Engineer" {
		t.Errorf("company/title = %q / %q", onsite.Company, onsite.Title)
	}
	if onsite.Location != "Fort Bragg, North Carolina, United States" || onsite.WorkMode != "onsite" || onsite.Remote {
		t.Errorf("place = %q mode=%q remote=%v", onsite.Location, onsite.WorkMode, onsite.Remote)
	}
	if onsite.Countries != nil {
		t.Errorf("Countries = %v, want nil (no location limits stated)", onsite.Countries)
	}
	if onsite.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", onsite.EmploymentType)
	}
	if onsite.SalaryMin == nil || *onsite.SalaryMin != 69400 || onsite.SalaryMax == nil || *onsite.SalaryMax != 158000 ||
		onsite.SalaryCurrency != "USD" || onsite.SalaryPeriod != "year" {
		t.Errorf("salary = %v-%v %q %q", onsite.SalaryMin, onsite.SalaryMax, onsite.SalaryCurrency, onsite.SalaryPeriod)
	}
	if onsite.PostedAt == nil || onsite.PostedAt.UTC().Format("2006-01-02") != "2026-10-16" {
		t.Errorf("PostedAt = %v", onsite.PostedAt)
	}
	if !strings.Contains(onsite.Description, "<b>The Opportunity:</b>") {
		t.Errorf("Description lost markup: %q", onsite.Description)
	}

	remoteUS := jobs[1]
	if remoteUS.WorkMode != "remote" || !remoteUS.Remote || !slices.Equal(remoteUS.Countries, []string{"us"}) || remoteUS.Location != "US" {
		t.Errorf("remote US = mode %q countries %v location %q", remoteUS.WorkMode, remoteUS.Countries, remoteUS.Location)
	}

	// "Worldwide" names no country, so it reaches the location dictionary as text only.
	worldwide := jobs[2]
	if worldwide.Countries != nil || worldwide.Location != "Worldwide" || worldwide.EmploymentType != "contract" {
		t.Errorf("worldwide = countries %v location %q type %q", worldwide.Countries, worldwide.Location, worldwide.EmploymentType)
	}
	if worldwide.SalaryMin == nil || *worldwide.SalaryMin != 80 || worldwide.SalaryPeriod != "hour" {
		t.Errorf("hourly salary = %v %q", worldwide.SalaryMin, worldwide.SalaryPeriod)
	}

	// This posting's application_link is an email address; the URL stays the posting page.
	email := jobs[3]
	if email.URL != "https://hire-mamas.jobboardly.com/jobs/business-development-podcast-sponsorship-coordinator-fe01dc06" {
		t.Errorf("URL = %q, want the board's posting page", email.URL)
	}
}

func TestJobboardlyRejectsNonLabelBoard(t *testing.T) {
	for _, board := range []string{"", "a.b", "https://x.jobboardly.com", "-x", "x_y"} {
		if _, err := NewJobboardly(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: board}); err == nil {
			t.Errorf("board %q accepted", board)
		}
	}
}

func TestJobboardlyListFailureFailsBoard(t *testing.T) {
	if _, err := NewJobboardly(&routedHTTP{}).Fetch(context.Background(), CompanyEntry{Board: "hire-mamas"}); err == nil {
		t.Error("a failed listing must fail the board")
	}
}

func TestJobboardlySalaryEdges(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"no bounds", `{"schedule":"yearly","minimum":null,"maximum":null}`},
		{"unmapped schedule", `{"schedule":"weekly","minimum":{"cents":100000,"currency_iso":"USD"},"maximum":null}`},
		{"two currencies", `{"schedule":"yearly","minimum":{"cents":100000,"currency_iso":"USD"},"maximum":{"cents":200000,"currency_iso":"EUR"}}`},
	}
	for _, c := range cases {
		body := `[{"title":"T","company":{"name":"C"},"links":{"self":"https://b.jobboardly.com/jobs/t-0a1b2c3d"},"salary":` + c.body + `}]`
		jobs, err := NewJobboardly((&routedHTTP{}).route("jobs.json", body)).Fetch(context.Background(), CompanyEntry{Board: "b"})
		if err != nil || len(jobs) != 1 {
			t.Fatalf("%s: jobs=%d err=%v", c.name, len(jobs), err)
		}
		if j := jobs[0]; j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" || j.SalaryPeriod != "" {
			t.Errorf("%s: salary = %v-%v %q %q, want none", c.name, j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
		}
	}
}

func TestJobboardlyDropsPostingWithoutIdentityOrCompany(t *testing.T) {
	body := `[
{"title":"no page","company":{"name":"C"},"links":{"self":""}},
{"title":"not a posting","company":{"name":"C"},"links":{"self":"https://b.jobboardly.com/about"}},
{"title":"no company","company":{"name":""},"links":{"self":"https://b.jobboardly.com/jobs/x-0a1b2c3d"}}
]`
	jobs, err := NewJobboardly((&routedHTTP{}).route("jobs.json", body)).Fetch(context.Background(), CompanyEntry{Board: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %d jobs, want 0: %+v", len(jobs), jobs)
	}
}
