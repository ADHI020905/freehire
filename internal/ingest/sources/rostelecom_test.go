package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// rostelecomFixture reads a trimmed real response captured from job.rt.ru.
func rostelecomFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/rostelecom/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func rostelecomFake(t *testing.T) *routedHTTP {
	t.Helper()
	return (&routedHTTP{}).
		route("page=0&", rostelecomFixture(t, "vacancies_p0.json")).
		route("page=1&", rostelecomFixture(t, "vacancies_p1.json")).
		route("page=2&", rostelecomFixture(t, "vacancies_p2.json"))
}

func rostelecomFetch(t *testing.T, fake *routedHTTP) map[string]Job {
	t.Helper()
	jobs, err := NewRostelecom(fake).Fetch(context.Background(), CompanyEntry{Company: "Rostelecom", Provider: "rostelecom"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if len(byID) != len(jobs) {
		t.Fatalf("duplicate external ids in %d jobs", len(jobs))
	}
	return byID
}

func TestRostelecomProvider(t *testing.T) {
	if got := NewRostelecom(nil).Provider(); got != "rostelecom" {
		t.Errorf("Provider() = %q, want rostelecom", got)
	}
	if _, ok := NewRostelecom(nil).(boardless); !ok {
		t.Error("rostelecom should implement the boardless marker")
	}
	if _, ok := Taxonomy()["rostelecom"]; !ok {
		t.Error("rostelecom is not registered in sources.All")
	}
}

func TestRostelecomWalksEveryPage(t *testing.T) {
	fake := rostelecomFake(t)
	byID := rostelecomFetch(t, fake)
	if len(byID) != 6 {
		t.Fatalf("len(jobs) = %d, want 6 across three pages", len(byID))
	}
	if fake.calls != 3 {
		t.Errorf("calls = %d, want 3 (totalPages)", fake.calls)
	}
}

func TestRostelecomMapsInlinePosting(t *testing.T) {
	j := rostelecomFetch(t, rostelecomFake(t))["14629"]
	if j.Title != "Машинист экскаватора" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://job.rt.ru/vacancy/14629" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "Rostelecom" {
		t.Errorf("Company = %q, want the configured entry (the API names no employer)", j.Company)
	}
	if j.Location != "Пушкино г" {
		t.Errorf("Location = %q, want the city", j.Location)
	}
	for _, want := range []string{"Управление мини-экскаватором", "Удостоверение машиниста", "официальное оформление"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q", want)
		}
	}
	if j.SalaryMin == nil || *j.SalaryMin != 70190 || j.SalaryMax == nil || *j.SalaryMax != 87738 {
		t.Errorf("Salary = %v..%v, want 70190..87738", j.SalaryMin, j.SalaryMax)
	}
	// salaryCurrency is null on this posting; every figure on the site is in roubles.
	if j.SalaryCurrency != "RUB" || j.SalaryPeriod != "month" {
		t.Errorf("Salary currency/period = %q/%q, want RUB/month", j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil (the API states no date)", j.PostedAt)
	}
}

func TestRostelecomOpenEndedSalaryAndNoExperience(t *testing.T) {
	j := rostelecomFetch(t, rostelecomFake(t))["8555"]
	if j.SalaryMin == nil || *j.SalaryMin != 40000 || j.SalaryMax != nil {
		t.Errorf("Salary = %v..%v, want 40000..nil", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "RUB" {
		t.Errorf("SalaryCurrency = %q, want RUB from \"руб\"", j.SalaryCurrency)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 0 {
		t.Errorf("ExperienceYearsMin = %v, want 0 from the \"Без опыта\" direction", j.ExperienceYearsMin)
	}
}

func TestRostelecomSecurityDirection(t *testing.T) {
	j := rostelecomFetch(t, rostelecomFake(t))["14502"]
	if j.Category != "security" || !j.IsTechHint {
		t.Errorf("Category = %q IsTechHint = %v, want security/true from the ИБ direction", j.Category, j.IsTechHint)
	}
	if j.SalaryMin != nil || j.SalaryCurrency != "" {
		t.Errorf("a posting with no salary must state none, got %v %q", j.SalaryMin, j.SalaryCurrency)
	}
	if other := rostelecomFetch(t, rostelecomFake(t))["14629"]; other.IsTechHint || other.Category != "" {
		t.Errorf("Технический блок must not assert IT: IsTechHint=%v Category=%q", other.IsTechHint, other.Category)
	}
}

func TestRostelecomTags(t *testing.T) {
	v := rostelecomVacancy{ID: 1, Name: "x", Tags: []rostelecomNamed{{ID: 11, Name: "Гибрид"}, {ID: 10, Name: "Частичная занятость"}}}
	j := v.toJob(CompanyEntry{Company: "Rostelecom"})
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j.WorkMode)
	}
	if j.EmploymentType != "part_time" {
		t.Errorf("EmploymentType = %q, want part_time", j.EmploymentType)
	}
}

func TestRostelecomPageFailureFailsCrawl(t *testing.T) {
	fake := rostelecomFake(t).routeErr("page=1&", errors.New("boom"))
	if _, err := NewRostelecom(fake).Fetch(context.Background(), CompanyEntry{Company: "Rostelecom"}); err == nil {
		t.Fatal("Fetch succeeded with a page failing")
	}
}

func TestRostelecomEmptyListYieldsNoJobs(t *testing.T) {
	fake := (&routedHTTP{}).route("page=0&", `{"totalPages":0,"totalCount":0,"vacancies":[]}`)
	jobs, err := NewRostelecom(fake).Fetch(context.Background(), CompanyEntry{Company: "Rostelecom"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
}

// An empty page before totalPages leaves the listing short of totalCount. Returning what was
// gathered would let the unseen sweep close the postings the empty page should have held.
func TestRostelecomShortListingFailsCrawl(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=0&", rostelecomFixture(t, "vacancies_p0.json")).
		route("page=1&", `{"totalPages":3,"totalCount":6,"vacancies":[]}`)
	if _, err := NewRostelecom(fake).Fetch(context.Background(), CompanyEntry{Company: "Rostelecom"}); err == nil {
		t.Fatal("Fetch succeeded with a listing short of totalCount")
	}
}

// A page repeating ids already read does not count toward totalCount: distinct ids do.
func TestRostelecomRepeatedPageFailsCrawl(t *testing.T) {
	fake := (&routedHTTP{}).
		route("page=0&", rostelecomFixture(t, "vacancies_p0.json")).
		route("page=1&", rostelecomFixture(t, "vacancies_p0.json")).
		route("page=2&", rostelecomFixture(t, "vacancies_p0.json"))
	if _, err := NewRostelecom(fake).Fetch(context.Background(), CompanyEntry{Company: "Rostelecom"}); err == nil {
		t.Fatal("Fetch succeeded with 3 distinct ids against totalCount 6")
	}
}

func TestRostelecomPageCapFailsCrawl(t *testing.T) {
	fake := (&routedHTTP{}).route("page=",
		`{"totalPages":1000,"totalCount":1000,"vacancies":[{"id":1,"name":"x"}]}`)
	_, err := NewRostelecom(fake).Fetch(context.Background(), CompanyEntry{Company: "Rostelecom"})
	if err == nil || !strings.Contains(err.Error(), "page cap") {
		t.Fatalf("err = %v, want a page-cap failure", err)
	}
}
