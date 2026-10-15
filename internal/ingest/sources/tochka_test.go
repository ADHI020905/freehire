package sources

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The fixtures are real hr.tochka.com pages captured 2026-10-14. The sitemap is trimmed to a
// few of its entries; each vacancy page keeps its ld+json and, verbatim, the flight row
// holding the vacancy object plus every text row its "$N" references point into.

func tochkaFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func TestTochkaProviderAndBoardless(t *testing.T) {
	s := NewTochka(nil)
	if got := s.Provider(); got != "tochka" {
		t.Errorf("Provider() = %q, want tochka", got)
	}
	if _, ok := s.(boardless); !ok {
		t.Error("tochka should implement the boardless marker")
	}
}

func tochkaFake(t *testing.T) *routedHTTP {
	return (&routedHTTP{}).
		route("sitemap.xml", tochkaFixture(t, "tochka_sitemap.xml")).
		route("/vacancies/catalog/analyst-dnk/", tochkaFixture(t, "tochka_vacancy_analyst.html")).
		route("/vacancies/catalog/menedzher-po-prodazham-vyezdnye-vstrechi-kaluga/", tochkaFixture(t, "tochka_vacancy_kaluga.html"))
}

func TestTochkaFetchMapsSitemapVacancies(t *testing.T) {
	jobs, err := NewTochka(tochkaFake(t)).Fetch(context.Background(),
		CompanyEntry{Company: "Точка Банк", Provider: "tochka"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2 (only /vacancies/catalog/<slug>/ entries)", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	a := byID["analyst-dnk"]
	if a.Title != "Senior продуктовый аналитик" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.URL != "https://hr.tochka.com/vacancies/catalog/analyst-dnk/" {
		t.Errorf("URL = %q", a.URL)
	}
	if a.Company != "Точка Банк" {
		t.Errorf("Company = %q", a.Company)
	}
	if !a.Remote || a.WorkMode != "remote" || a.Location != "" {
		t.Errorf("Remote/WorkMode/Location = %v/%q/%q, want true/remote/empty", a.Remote, a.WorkMode, a.Location)
	}
	if !a.IsTechHint || a.Category != "data_analytics" {
		t.Errorf("IsTechHint/Category = %v/%q, want true/data_analytics", a.IsTechHint, a.Category)
	}
	if a.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", a.EmploymentType)
	}
	if a.ExperienceYearsMin == nil || *a.ExperienceYearsMin != 5 {
		t.Errorf("ExperienceYearsMin = %v, want 5 (over_five_years)", a.ExperienceYearsMin)
	}
	// "до 500 000 ₽": an upper bound only.
	if a.SalaryMin != nil || a.SalaryMax == nil || *a.SalaryMax != 500000 ||
		a.SalaryCurrency != "RUB" || a.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %q/%q, want -500000 RUB/month", a.SalaryMin, a.SalaryMax, a.SalaryCurrency, a.SalaryPeriod)
	}
	if a.PostedAt == nil || a.PostedAt.Format("2006-01-02") != "2025-10-29" {
		t.Errorf("PostedAt = %v, want 2025-10-29", a.PostedAt)
	}
	// Every section the page renders, including the ones held as "$N" text rows.
	for _, want := range []string{
		"Ищем опытных продуктовых аналитиков", // detail
		"Команда Sales-ML",                        // aboutTeam
		"Анализировать потенциал гипотез",         // requirement ($ref)
		"Кросс-функциональные команды",            // blocks
		"Есть опыт работы продуктовым аналитиком", // responsibility ($ref)
		"Официальная зарплата",                    // expectation ($ref)
		"Ты подойдёшь, если",
	} {
		if !strings.Contains(a.Description, want) {
			t.Errorf("Description lacks %q", want)
		}
	}
	if strings.Contains(a.Description, "$1f") || strings.Contains(a.Description, "$2") {
		t.Errorf("Description carries an unresolved flight reference: %.300q", a.Description)
	}

	k := byID["menedzher-po-prodazham-vyezdnye-vstrechi-kaluga"]
	if k.Location != "Калуга" {
		t.Errorf("Location = %q, want Калуга", k.Location)
	}
	if k.Remote || k.WorkMode != "onsite" {
		t.Errorf("Remote/WorkMode = %v/%q, want false/onsite (field sales trips)", k.Remote, k.WorkMode)
	}
	if k.IsTechHint || k.Category != "sales" {
		t.Errorf("IsTechHint/Category = %v/%q, want false/sales", k.IsTechHint, k.Category)
	}
	if k.SalaryMin == nil || *k.SalaryMin != 150000 || k.SalaryMax != nil {
		t.Errorf("salary = %v-%v, want 150000-", k.SalaryMin, k.SalaryMax)
	}
	if k.ExperienceYearsMin == nil || *k.ExperienceYearsMin != 1 {
		t.Errorf("ExperienceYearsMin = %v, want 1 (up_to_three_years)", k.ExperienceYearsMin)
	}
}

func TestTochkaVacancyMappings(t *testing.T) {
	modes := map[string]string{"remotely": "remote", "hybrid": "hybrid", "office": "onsite", "trips": "onsite", "new": ""}
	for in, want := range modes {
		if got := tochkaWorkMode(in); got != want {
			t.Errorf("tochkaWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
	if got := tochkaSeniority("lead"); got != "lead" {
		t.Errorf("tochkaSeniority(lead) = %q", got)
	}
	if got := tochkaSeniority(""); got != "" {
		t.Errorf("tochkaSeniority(\"\") = %q", got)
	}
	if got := tochkaExperienceYears("up_to_five_years"); got == nil || *got != 3 {
		t.Errorf("tochkaExperienceYears(up_to_five_years) = %v, want 3", got)
	}
	if got := tochkaExperienceYears("whatever"); got != nil {
		t.Errorf("tochkaExperienceYears(unknown) = %v, want nil", *got)
	}
	if got := tochkaCategory(tochkaVacancy{MainCategory: &tochkaFacet{Slug: "infosec"}}); got != "security" {
		t.Errorf("infosec category = %q, want security", got)
	}
}

// A vacancy page whose flight names only OTHER vacancies (the "more vacancies" cards) is not
// read as one of them.
func TestTochkaPicksTheVacancyByItsOwnSlug(t *testing.T) {
	row := `5:["$","$L1",null,{"vacancy":{"title":"Другая","slug":"other"}}]` + "\n"
	page := `<html><body><script>self.__next_f.push([1,` + jsString(row) + `])</script></body></html>`
	fake := tochkaFake(t)
	fake.routes = append([]struct{ match, body string }{{"/analyst-dnk/", page}}, fake.routes...)

	jobs, err := NewTochka(fake).Fetch(context.Background(), CompanyEntry{Company: "Точка Банк"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	for _, j := range jobs {
		if j.ExternalID == "analyst-dnk" && !j.Unreadable {
			t.Fatalf("analyst-dnk read from another vacancy's object: %+v", j)
		}
		if j.ExternalID == "other" {
			t.Fatal("a related vacancy card must not become a posting")
		}
	}
}

func TestTochkaGoneVacancyIsDropped(t *testing.T) {
	fake := tochkaFake(t).routeErr("/analyst-dnk/", &StatusError{Code: 404})
	jobs, err := NewTochka(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ExternalID == "analyst-dnk" {
		t.Fatalf("jobs = %+v, want only the kaluga vacancy", jobs)
	}
}

func TestTochkaReadingNoneFails(t *testing.T) {
	fake := tochkaFake(t).routeErr("/vacancies/catalog/", &StatusError{Code: 503})
	if _, err := NewTochka(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a crawl that listed vacancies and read none of them must fail")
	}
}

// If the sitemap becomes an index, every sub-sitemap is followed — and one that fails fails the
// run rather than truncating the enumeration.
func TestTochkaFollowsASitemapIndex(t *testing.T) {
	index := `<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` +
		`<sitemap><loc>https://hr.tochka.com/sitemap-vacancies.xml</loc></sitemap></sitemapindex>`
	newFake := func() *routedHTTP {
		return (&routedHTTP{}).
			route("/vacancies/catalog/analyst-dnk/", tochkaFixture(t, "tochka_vacancy_analyst.html")).
			route("/vacancies/catalog/menedzher-po-prodazham-vyezdnye-vstrechi-kaluga/", tochkaFixture(t, "tochka_vacancy_kaluga.html"))
	}

	fake := newFake().route("sitemap-vacancies.xml", tochkaFixture(t, "tochka_sitemap.xml")).route("sitemap.xml", index)
	jobs, err := NewTochka(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs, err = %d, %v; want both vacancies through the index", len(jobs), err)
	}

	broken := newFake().routeErr("sitemap-vacancies.xml", &StatusError{Code: 503}).route("sitemap.xml", index)
	if _, err := NewTochka(broken).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a sub-sitemap that fails must fail the run, not truncate it")
	}
}

func TestTochkaSitemapWithoutVacanciesFails(t *testing.T) {
	fake := (&routedHTTP{}).route("sitemap.xml",
		`<?xml version="1.0" encoding="UTF-8"?><urlset><url><loc>https://hr.tochka.com/</loc></url></urlset>`)
	if _, err := NewTochka(fake).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a sitemap naming no vacancy pages must fail")
	}
}
