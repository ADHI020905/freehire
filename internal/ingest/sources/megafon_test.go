package sources

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

// megafonFixture reads a trimmed real response captured from job.megafon.ru.
func megafonFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/megafon/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// megafonFake routes the four cities of the fixture set: the two pseudo-cities (remote, "all
// cities"), Moscow over two pages, and Volgograd, which re-lists one of Moscow's postings.
// Details exist for 4798 and 4763 only; every other detail request finds no route and fails.
func megafonFake(t *testing.T) *routedHTTP {
	t.Helper()
	return (&routedHTTP{}).
		route("/cities/available", megafonFixture(t, "cities.json")).
		route("cities=2374&page=1", megafonFixture(t, "vacancies_remote.json")).
		route("cities=2375&page=1", megafonFixture(t, "vacancies_all_cities.json")).
		route("cities=1&page=1", megafonFixture(t, "vacancies_moscow_p1.json")).
		route("cities=1&page=2", megafonFixture(t, "vacancies_moscow_p2.json")).
		route("cities=16&page=1", megafonFixture(t, "vacancies_volgograd.json")).
		route("/vacancies/java-razrabotchik-informatsionnyh-sistem-sobstvennoj-roznitsy-4798?cityId=1", megafonFixture(t, "detail_4798.json")).
		route("/vacancies/android-razrabotchik-4763?cityId=2374", megafonFixture(t, "detail_4763.json"))
}

func megafonByID(jobs []Job) map[string]Job {
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	return byID
}

func TestMegafonProvider(t *testing.T) {
	if got := NewMegafon(nil).Provider(); got != "megafon" {
		t.Errorf("Provider() = %q, want megafon", got)
	}
}

func TestMegafonIsBoardlessHydrating(t *testing.T) {
	s := NewMegafon(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("megafon should implement the boardless marker")
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("megafon should be a HydratingSource (detail is a per-posting request)")
	}
}

func TestMegafonRegisteredInAll(t *testing.T) {
	if _, ok := Taxonomy()["megafon"]; !ok {
		t.Error("megafon is not registered in sources.All")
	}
}

// The unfiltered listing pages on a publication timestamp shared by hundreds of postings, so it
// repeats some and never serves others. The crawl walks the listing city by city instead and
// merges by id: a posting listed in two cities is one job carrying both.
func TestMegafonWalksEveryCityAndMergesByID(t *testing.T) {
	jobs, err := NewMegafon(megafonFake(t)).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "MegaFon", Provider: "megafon"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// remote 4 + all-cities 1 + Moscow 4+3 + Volgograd 2, minus 3876 listed twice.
	if len(jobs) != 13 {
		t.Fatalf("len(jobs) = %d, want 13 unique postings", len(jobs))
	}
	byID := megafonByID(jobs)
	if got := byID["3876"].Location; got != "Москва; Волгоград" {
		t.Errorf("3876 Location = %q, want both cities it is listed in", got)
	}
	if got := byID["3875"].Location; got != "Волгоград" {
		t.Errorf("3875 Location = %q, want Волгоград", got)
	}
}

func TestMegafonHydratesUnseenPosting(t *testing.T) {
	jobs, err := NewMegafon(megafonFake(t)).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "MegaFon", Provider: "megafon"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	j, ok := megafonByID(jobs)["4798"]
	if !ok {
		t.Fatal("posting 4798 missing")
	}
	if j.Title != "Java разработчик информационных систем собственной розницы" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "МегаТех" {
		t.Errorf("Company = %q, want the posting's own organisation", j.Company)
	}
	if want := "https://job.megafon.ru/vacancy/moskva/it-reshenia/java-razrabotchik-informatsionnyh-sistem-sobstvennoj-roznitsy-4798"; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	if j.Location != "Москва" {
		t.Errorf("Location = %q, want Москва", j.Location)
	}
	for _, want := range []string{"Анализ требований к информационным системам", "Опыт разработки ПО на языках программирования Java"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("Description missing %q", want)
		}
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode = %q Remote = %v, want remote from the posting's remote tag", j.WorkMode, j.Remote)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 3 {
		t.Errorf("ExperienceYearsMin = %v, want 3 from \"от 3 до 6 лет\"", j.ExperienceYearsMin)
	}
	for _, want := range []string{"java", "kafka", "postgresql"} {
		if !slices.Contains(j.Skills, want) {
			t.Errorf("Skills = %v, missing %q", j.Skills, want)
		}
	}
	if j.Category != "backend" {
		t.Errorf("Category = %q, want backend from the Backend specialty", j.Category)
	}
	if !j.IsTechHint {
		t.Error("IsTechHint = false, want true for the ИТ-решения sector")
	}
	if j.PostedAt == nil || j.PostedAt.Unix() != 1791896670 {
		t.Errorf("PostedAt = %v, want publishedAt 1791896670", j.PostedAt)
	}
	if j.SeenRefresh {
		t.Error("an unseen posting must not be a SeenRefresh")
	}
}

func TestMegafonRemotePseudoCity(t *testing.T) {
	jobs, err := NewMegafon(megafonFake(t)).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "MegaFon", Provider: "megafon"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := megafonByID(jobs)
	j := byID["4763"]
	if j.WorkMode != "remote" {
		t.Errorf("4763 WorkMode = %q, want remote from the remote pseudo-city", j.WorkMode)
	}
	if j.Location != "" {
		t.Errorf("4763 Location = %q, want empty (the pseudo-city is not a place)", j.Location)
	}
	if j.Category != "mobile" {
		t.Errorf("4763 Category = %q, want mobile", j.Category)
	}
	if !strings.Contains(j.Description, "Android") {
		t.Errorf("4763 Description not hydrated: %q", j.Description)
	}
	if want := "https://job.megafon.ru/vacancy/udalennyj-format/it-reshenia/android-razrabotchik-4763"; j.URL != want {
		t.Errorf("4763 URL = %q, want %q", j.URL, want)
	}
	// 4511 is listed only under the remote pseudo-city and carries no tag: still remote.
	if byID["4511"].WorkMode != "remote" {
		t.Errorf("4511 WorkMode = %q, want remote", byID["4511"].WorkMode)
	}
	// 4794 carries the hybrid-schedule tag, which outranks the remote listing it sits in.
	if byID["4794"].WorkMode != "hybrid" {
		t.Errorf("4794 WorkMode = %q, want hybrid from its tag", byID["4794"].WorkMode)
	}
	if byID["4794"].Company != "ЕдиныйФактор" {
		t.Errorf("4794 Company = %q, want ЕдиныйФактор", byID["4794"].Company)
	}
}

func TestMegafonAllCitiesPseudoCityIsNationwide(t *testing.T) {
	jobs, err := NewMegafon(megafonFake(t)).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "MegaFon", Provider: "megafon"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	j := megafonByID(jobs)["3284"]
	if j.Location != "Россия" {
		t.Errorf("3284 Location = %q, want Россия", j.Location)
	}
	if !slices.Equal(j.Countries, []string{"ru"}) {
		t.Errorf("3284 Countries = %v, want [ru]", j.Countries)
	}
	if j.Company != "МегаФон Ритейл" {
		t.Errorf("3284 Company = %q", j.Company)
	}
}

func TestMegafonSeenPostingSkipsDetail(t *testing.T) {
	fake := megafonFake(t)
	jobs, err := NewMegafon(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "MegaFon", Provider: "megafon"}, func(id string) bool { return id == "4798" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	j := megafonByID(jobs)["4798"]
	if !j.SeenRefresh {
		t.Error("4798 is seen: want a SeenRefresh")
	}
	if j.Description != "" {
		t.Errorf("a seen posting must carry no description, got %q", j.Description)
	}
	if j.Title == "" || j.URL == "" {
		t.Error("a SeenRefresh still needs its identity (title, url)")
	}
}

// A failed detail keeps the posting, list-only: dropping it would close a live posting, and a
// hydrating source re-offers a body-less row within the hydration-retry window.
func TestMegafonFailedDetailIngestsListOnly(t *testing.T) {
	jobs, err := NewMegafon(megafonFake(t)).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "MegaFon", Provider: "megafon"}, func(string) bool { return false })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	j, ok := megafonByID(jobs)["4800"]
	if !ok {
		t.Fatal("4800 dropped on a failed detail; want it ingested list-only")
	}
	if j.Description != "" || j.Title == "" {
		t.Errorf("4800 = %+v, want title without description", j)
	}
	if j.PostedAt == nil {
		t.Error("4800 PostedAt nil: the listing carries publishedAt")
	}
}

// A city whose listing cannot be read fails the crawl: its postings would otherwise read as
// unseen and be closed by the sweep.
func TestMegafonCityFailureFailsCrawl(t *testing.T) {
	fake := megafonFake(t).routeErr("cities=16&page=1", errors.New("boom"))
	if _, err := NewMegafon(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "MegaFon", Provider: "megafon"}, func(string) bool { return false }); err == nil {
		t.Fatal("FetchNew succeeded with a city listing failing")
	}
}

func TestMegafonEmptyCityEndsWalk(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/cities/available", `{"cities":[{"id":7,"title":"Тверь","slug":"tver"}]}`).
		route("cities=7&page=1", `{"page":1,"pages":0,"total":0,"vacancies":[]}`)
	jobs, err := NewMegafon(fake).Fetch(context.Background(), CompanyEntry{Company: "MegaFon", Provider: "megafon"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("got %d jobs, want 0", len(jobs))
	}
	if fake.calls != 2 {
		t.Errorf("calls = %d, want 2 (cities + one empty page)", fake.calls)
	}
}

func TestMegafonExperienceYears(t *testing.T) {
	cases := map[string]int{"Без опыта": 0, "До 1 года": 0, "1-3 года": 1, "от 3 до 6 лет": 3, "более 6 лет": 6}
	for in, want := range cases {
		got := megafonExperienceYears(in)
		if got == nil || *got != want {
			t.Errorf("megafonExperienceYears(%q) = %v, want %d", in, got, want)
		}
	}
	if got := megafonExperienceYears("что-то"); got != nil {
		t.Errorf("unknown experience = %v, want nil", *got)
	}
}

func TestMegafonCategoryNeedsOneUnambiguousSpecialty(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"Backend"}, "backend"},
		{[]string{"Тестирование "}, "qa"},
		{[]string{"Информационная безопасность"}, "security"},
		{[]string{"DevOps/SRE"}, ""},                      // spans two categories
		{[]string{"Backend", "Мобильная разработка"}, ""}, // two specialties disagree
		{[]string{"Архитектура", "Системный анализ"}, "architecture"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := megafonCategory(c.in); got != c.want {
			t.Errorf("megafonCategory(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// No sampled posting states a salary, but the detail carries the bounds; a stated one is read
// as roubles per month, and an absent one states nothing.
func TestMegafonDetailSalary(t *testing.T) {
	lo, hi := 80000.0, 120000.0
	j := megafonDetail{MinSalary: &lo, MaxSalary: &hi}.apply(Job{})
	if j.SalaryMin == nil || *j.SalaryMin != 80000 || j.SalaryMax == nil || *j.SalaryMax != 120000 {
		t.Errorf("Salary = %v..%v, want 80000..120000", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "RUB" || j.SalaryPeriod != "month" {
		t.Errorf("currency/period = %q/%q, want RUB/month", j.SalaryCurrency, j.SalaryPeriod)
	}
	if none := (megafonDetail{}).apply(Job{}); none.SalaryMin != nil || none.SalaryCurrency != "" {
		t.Errorf("no salary stated, got %v %q", none.SalaryMin, none.SalaryCurrency)
	}
}

// A city whose listing outruns the page cap fails the crawl rather than return the pages read.
func TestMegafonPageCapFailsCrawl(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/cities/available", `{"cities":[{"id":7,"title":"Тверь","slug":"tver"}]}`).
		route("cities=7&page=", `{"page":1,"pages":1000,"total":10000,"vacancies":[{"id":1,"slug":"x-1","title":"x","city":{"id":7,"title":"Тверь","slug":"tver"}}]}`)
	_, err := NewMegafon(fake).Fetch(context.Background(), CompanyEntry{Company: "MegaFon"})
	if err == nil || !strings.Contains(err.Error(), "page cap") {
		t.Fatalf("err = %v, want a page-cap failure", err)
	}
}
