package sources

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// The fixtures are real ptsecurity.com pages captured 2026-10-14, trimmed to what the adapter
// reads: the listing keeps its ld+json and the ONE flight row carrying the vacancy-list
// payload (verbatim); each vacancy page keeps its ld+json and its article-content element.

func ptsecurityFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

// ptsecurityListingPage builds a listing page whose flight carries the given vacancy-list
// payload, in the shape the site renders it.
func ptsecurityListingPage(data string) string {
	row := `4b:["$","$L4c",null,{"testId":"vacancy-list","skeletonCount":20,"data":` + data + `}]` + "\n"
	return `<html><body><script>self.__next_f.push([1,` + jsString(row) + `])</script></body></html>`
}

func jsString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

func TestPtsecurityProviderAndBoardless(t *testing.T) {
	s := NewPtsecurity(nil)
	if got := s.Provider(); got != "ptsecurity" {
		t.Errorf("Provider() = %q, want ptsecurity", got)
	}
	if _, ok := s.(boardless); !ok {
		t.Error("ptsecurity should implement the boardless marker")
	}
}

func TestPtsecurityFetchMapsListingAndDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/about/vacancy/391799/", ptsecurityFixture(t, "ptsecurity_vacancy_391799.html")).
		route("/about/vacancy/394319/", ptsecurityFixture(t, "ptsecurity_vacancy_394319.html")).
		route("/about/vacancy/vacancy/", ptsecurityFixture(t, "ptsecurity_vacancy_vacancy.html")).
		// Every other vacancy the real listing names answers with a page carrying no
		// article, so it surfaces as an unreadable marker rather than vanishing.
		route("/about/vacancy/?page=1", ptsecurityFixture(t, "ptsecurity_list.html")).
		route("/about/vacancy/", `<html><body></body></html>`)

	jobs, err := NewPtsecurity(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Positive Technologies", Provider: "ptsecurity"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 17 {
		t.Fatalf("got %d jobs, want the listing's 17", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	red := byID["391799"]
	if red.Unreadable {
		t.Fatal("391799 has a full page and must be read")
	}
	if red.Title != "Red team developer" {
		t.Errorf("Title = %q", red.Title)
	}
	if red.URL != "https://ptsecurity.com/about/vacancy/391799/" {
		t.Errorf("URL = %q", red.URL)
	}
	if red.Company != "Positive Technologies" {
		t.Errorf("Company = %q", red.Company)
	}
	if red.Location != "Удаленная работа" {
		t.Errorf("Location = %q", red.Location)
	}
	if !red.Remote || red.WorkMode != "remote" {
		t.Errorf("Remote/WorkMode = %v/%q, want true/remote (jobLocationType TELECOMMUTE)", red.Remote, red.WorkMode)
	}
	if red.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", red.EmploymentType)
	}
	if red.Category != "security" {
		t.Errorf("Category = %q, want security (the site files it under Хакеры)", red.Category)
	}
	// The stack is spelled with a CYRILLIC С ("С++", "С#") on the site.
	for _, want := range []string{"cpp", "go", "dotnet", "csharp"} {
		if !slices.Contains(red.Skills, want) {
			t.Errorf("Skills = %v, missing %q", red.Skills, want)
		}
	}
	if !strings.Contains(red.Description, "Требования:") || !strings.Contains(red.Description, "Обфускация исходного кода") {
		t.Errorf("Description lacks the article body: %.200q", red.Description)
	}
	if red.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil (datePosted is empty on every page)", red.PostedAt)
	}

	labs := byID["394319"]
	if labs.Location != "Москва, Санкт-Петербург, Новосибирск, Томск" {
		t.Errorf("Location = %q", labs.Location)
	}
	if labs.Remote || labs.WorkMode != "" {
		t.Errorf("Remote/WorkMode = %v/%q, want false/\"\" (no remote signal)", labs.Remote, labs.WorkMode)
	}
	if !slices.Equal(labs.Countries, []string{"ru"}) {
		t.Errorf("Countries = %v, want [ru] from the ld+json addresses", labs.Countries)
	}
	if !strings.Contains(labs.Description, "Positive Labs") {
		t.Errorf("Description = %.200q", labs.Description)
	}

	// "vacancy" is a generic slug, but it is the vacancy's real route: the page it opens is
	// Руководитель консалтинговых проектов (fixture captured from that URL).
	consult := byID["vacancy"]
	if consult.Unreadable || consult.URL != "https://ptsecurity.com/about/vacancy/vacancy/" ||
		consult.Title != "Руководитель консалтинговых проектов" ||
		!strings.Contains(consult.Description, "консалтинг") {
		t.Errorf("vacancy = %+v, want Руководитель консалтинговых проектов read from /about/vacancy/vacancy/", consult)
	}

	// A vacancy whose page carries no article is named by the listing but unread.
	erp := byID["1c-analitik--erp"]
	if !erp.Unreadable || erp.URL != "https://ptsecurity.com/about/vacancy/1c-analitik--erp/" {
		t.Errorf("1c-analitik--erp = %+v, want an unreadable marker", erp)
	}
}

func TestPtsecurityPaginatesUntilPageCount(t *testing.T) {
	page := func(n int, slug string) string {
		return ptsecurityListingPage(`{"vacancies":[{"id":"1","title":"Информационная безопасность","vacancies":[` +
			`{"title":"Analyst ` + slug + `","slug":"` + slug + `","isRemoteVacancy":false,"activity":"Информационная безопасность","locations":["Москва"],"stack":{"data":[]}}]}],` +
			`"pagination":{"page":` + itoa(n) + `,"pageSize":1,"pageCount":2,"total":2}}`)
	}
	article := `<html><body><div data-testid="article-content"><p>Body</p></div></body></html>`
	fake := (&routedHTTP{}).
		route("/about/vacancy/a/", article).
		route("/about/vacancy/b/", article).
		route("?page=1", page(1, "a")).
		route("?page=2", page(2, "b"))

	jobs, err := NewPtsecurity(fake).Fetch(context.Background(), CompanyEntry{Company: "Positive Technologies"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 || jobs[0].ExternalID != "a" || jobs[1].ExternalID != "b" {
		t.Fatalf("jobs = %+v, want a then b", jobs)
	}
	if jobs[0].Category != "security" {
		t.Errorf("Category = %q, want security", jobs[0].Category)
	}
}

// A listing that comes up short of its own total is a crawl failure: closing the vacancies it
// did not deliver would retire live postings.
func TestPtsecurityShortListingFails(t *testing.T) {
	fake := (&routedHTTP{}).route("?page=1", ptsecurityListingPage(
		`{"vacancies":[{"id":"1","title":null,"vacancies":[{"title":"A","slug":"a","locations":[]}]}],`+
			`"pagination":{"page":1,"pageSize":20,"pageCount":1,"total":5}}`))
	if _, err := NewPtsecurity(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when the listing delivers fewer vacancies than its total")
	}
}

func TestPtsecurityMissingPayloadFails(t *testing.T) {
	fake := (&routedHTTP{}).route("?page=1", `<html><body><script>self.__next_f.push([1,"0:[]\n"])</script></body></html>`)
	if _, err := NewPtsecurity(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when the listing carries no vacancy-list payload")
	}
}

func TestPtsecurityEmptyListingIsNotAnError(t *testing.T) {
	fake := (&routedHTTP{}).route("?page=1", ptsecurityListingPage(
		`{"vacancies":[],"pagination":{"page":1,"pageSize":20,"pageCount":0,"total":0}}`))
	jobs, err := NewPtsecurity(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs, err = %v, %v; want none and no error (the site's own total says zero)", jobs, err)
	}
}

func TestPtsecurityReadingNoneFails(t *testing.T) {
	fake := (&routedHTTP{}).
		route("?page=1", ptsecurityFixture(t, "ptsecurity_list.html")).
		route("/about/vacancy/", `<html><body></body></html>`)
	if _, err := NewPtsecurity(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a crawl that listed vacancies and read none of them must fail")
	}
}

func TestPtsecurityGoneVacancyIsDropped(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("/about/vacancy/394319/", &StatusError{Code: 404}).
		route("/about/vacancy/391799/", ptsecurityFixture(t, "ptsecurity_vacancy_391799.html")).
		route("?page=1", ptsecurityFixture(t, "ptsecurity_list.html")).
		route("/about/vacancy/", `<html><body></body></html>`)
	jobs, err := NewPtsecurity(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, j := range jobs {
		if j.ExternalID == "394319" {
			t.Fatalf("a 404 vacancy must be dropped, got %+v", j)
		}
	}
}

// A listing that repeats an entry still delivers its total: the repeat is counted as
// delivered, then collapsed onto the one page its slug opens.
func TestPtsecurityRepeatedSlugDoesNotFailTheCrawl(t *testing.T) {
	entry := `{"title":"A","slug":"vacancy","locations":["Москва"]}`
	fake := (&routedHTTP{}).
		route("/about/vacancy/vacancy/", `<html><body><div data-testid="article-content"><p>Body</p></div></body></html>`).
		route("?page=1", ptsecurityListingPage(`{"vacancies":[{"id":"1","title":null,"vacancies":[`+entry+`,`+entry+`]}],`+
			`"pagination":{"page":1,"pageSize":20,"pageCount":1,"total":2}}`))
	jobs, err := NewPtsecurity(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID != "vacancy" {
		t.Fatalf("jobs = %+v, want the one vacancy", jobs)
	}
}

func TestPtsecurityNullAddressAddsNoCountry(t *testing.T) {
	var p ptsecurityPosting
	if err := json.Unmarshal([]byte(`{"jobLocation":{"address":null}}`), &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(p.JobLocation.Address) != 0 {
		t.Errorf("Address = %+v, want none for null", p.JobLocation.Address)
	}
}
