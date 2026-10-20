package sources

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// highermeBrand is the brand the recorded fixture lists (app.higherme.com/careers/<brand>).
const highermeBrand = "6078977d8dfec"

func TestHigherMeProvider(t *testing.T) {
	if got := NewHigherMe(nil).Provider(); got != "higherme" {
		t.Errorf("Provider() = %q, want %q", got, "higherme")
	}
}

// TestHigherMeFetchRecordedBrand runs the adapter over a live /classic/jobs response recorded
// 2026-10-20 for one brand: seven published postings on one page, two of them with a salary.
func TestHigherMeFetchRecordedBrand(t *testing.T) {
	raw, err := os.ReadFile("testdata/higherme/brand_jobs.json")
	if err != nil {
		t.Fatal(err)
	}
	fake := (&routedHTTP{}).route("/classic/jobs?", string(raw))

	jobs, err := NewHigherMe(fake).Fetch(context.Background(), CompanyEntry{Company: "Camp", Board: highermeBrand})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 7 {
		t.Fatalf("jobs = %d, want 7", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	runner := byID["607aff1be9251"]
	if runner.Title != "Food Runner" {
		t.Errorf("title = %q", runner.Title)
	}
	if runner.URL != "https://app.higherme.com/jobs/607aff1be9251" {
		t.Errorf("url = %q", runner.URL)
	}
	if runner.Company != "Camp" {
		t.Errorf("company = %q, want the configured company", runner.Company)
	}
	if runner.Location != "Greenville, South Carolina, United States" {
		t.Errorf("location = %q", runner.Location)
	}
	if len(runner.Countries) != 1 || runner.Countries[0] != "us" {
		t.Errorf("countries = %v, want [us]", runner.Countries)
	}
	if runner.Remote || runner.PostedAt != nil {
		t.Errorf("remote = %v, posted = %v; want false, nil", runner.Remote, runner.PostedAt)
	}
	if runner.EmploymentType != "" {
		t.Errorf("employment type = %q, want empty for a posting open to both schedules", runner.EmploymentType)
	}
	if runner.SalaryMin != nil || runner.SalaryMax != nil || runner.SalaryCurrency != "" {
		t.Errorf("salary set on a posting that states none: %v %v %q", runner.SalaryMin, runner.SalaryMax, runner.SalaryCurrency)
	}
	if !strings.Contains(runner.Description, "food runner") || !strings.Contains(runner.Description, "<h3>About</h3>") {
		t.Errorf("description missing the summary or the about section: %q", runner.Description)
	}

	cook := byID["607b00d34131b"]
	if cook.SalaryMin == nil || *cook.SalaryMin != 10 || cook.SalaryMax == nil || *cook.SalaryMax != 15 ||
		cook.SalaryCurrency != "USD" || cook.SalaryPeriod != "hour" {
		t.Errorf("hourly salary = %v-%v %q/%q, want 10-15 USD/hour", cook.SalaryMin, cook.SalaryMax, cook.SalaryCurrency, cook.SalaryPeriod)
	}

	manager := byID["6632a68d3ffb5"]
	if manager.SalaryMin == nil || *manager.SalaryMin != 45000 || manager.SalaryPeriod != "year" {
		t.Errorf("annual salary = %v %q, want 45000/year", manager.SalaryMin, manager.SalaryPeriod)
	}
	if manager.EmploymentType != "full_time" {
		t.Errorf("employment type = %q, want full_time", manager.EmploymentType)
	}
}

// highermePage builds a /classic/jobs page of the given posting ids, all under brand.
func highermePage(brand string, lastPage, total int, ids ...string) string {
	var data []string
	for _, id := range ids {
		data = append(data, fmt.Sprintf(`{"id":%q,"attributes":{"title":"Cook %s","full_time":false,"part_time":true,
			"summary":"<p>Cook.</p><script>x()</script>","requirements":["Lift 30 lbs","  "],"salary":{"min":null,"max":null,"type":"hourly"},
			"status":"published"},"relations":{"location":{"attributes":{"city":"Toronto","state":{"name":"Ontario"},
			"country":{"name":"Canada","short":"CA"}},"relations":{"brand":{"id":%q}}}}}`, id, id, brand))
	}
	return fmt.Sprintf(`{"data":[%s],"meta":{"last_page":%d,"total":%d}}`, strings.Join(data, ","), lastPage, total)
}

func TestHigherMeFetchPagesToTheEnd(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=1", highermePage("b1", 2, 3, "a", "b")).
		route("page=2", highermePage("b1", 2, 3, "c"))

	jobs, err := NewHigherMe(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Board: "b1"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	j := jobs[0]
	if j.EmploymentType != "part_time" {
		t.Errorf("employment type = %q, want part_time", j.EmploymentType)
	}
	if strings.Contains(j.Description, "x()") || !strings.Contains(j.Description, "<li>Lift 30 lbs</li>") {
		t.Errorf("description not sanitized or requirements missing: %q", j.Description)
	}
	if strings.Count(j.Description, "<li>") != 1 {
		t.Errorf("a blank requirement must not render a bullet: %q", j.Description)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "ca" {
		t.Errorf("countries = %v, want [ca]", j.Countries)
	}
}

// A listing that ends short of its own total is a truncated read, and a partial board must not
// pass for a whole one — the sweep would close the unread remainder.
func TestHigherMeFetchFailsShortOfTheTotal(t *testing.T) {
	fake := (&routedHTTP{}).route("page=1", highermePage("b1", 1, 5, "a", "b"))
	if _, err := NewHigherMe(fake).Fetch(context.Background(), CompanyEntry{Board: "b1"}); err == nil {
		t.Fatal("fetch succeeded on a listing that read 2 of a reported 5")
	}
}

// An ignored brand filter answers with the whole platform; a posting of another brand must fail
// the crawl rather than be filed under this board's employer.
func TestHigherMeFetchRejectsAnotherBrandsPosting(t *testing.T) {
	fake := (&routedHTTP{}).route("page=1", highermePage("someone-else", 1, 1, "a"))
	if _, err := NewHigherMe(fake).Fetch(context.Background(), CompanyEntry{Board: "b1"}); err == nil {
		t.Fatal("fetch accepted a posting of a different brand")
	}
}

func TestHigherMeFetchEmptyBoard(t *testing.T) {
	empty := highermePage("b1", 1, 0)

	live := (&routedHTTP{}).route("/classic/jobs?", empty).route("/classic/brands/b1", `{"data":{"id":"b1"}}`)
	jobs, err := NewHigherMe(live).Fetch(context.Background(), CompanyEntry{Board: "b1"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("live empty brand: jobs = %d, err = %v; want 0, nil", len(jobs), err)
	}

	gone := (&routedHTTP{}).route("/classic/jobs?", empty).
		routeErr("/classic/brands/b1", &StatusError{Method: "GET", Code: 404, URL: "x"})
	if _, err := NewHigherMe(gone).Fetch(context.Background(), CompanyEntry{Board: "b1"}); !errors.Is(err, ErrBoardGone) {
		t.Fatalf("unknown brand: err = %v, want ErrBoardGone", err)
	}
}

func TestHigherMeSalaryOutsideItsCountriesIsDropped(t *testing.T) {
	lo := 12.0
	if minPay, maxPay, cur, _ := highermeSalary(&lo, nil, "hourly", "MX"); minPay != nil || maxPay != nil || cur != "" {
		t.Errorf("salary for a location outside US/CA = %v %v %q, want none", minPay, maxPay, cur)
	}
}
