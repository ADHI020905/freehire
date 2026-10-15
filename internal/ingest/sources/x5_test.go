package sources

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// The x5 fixtures are real responses captured from rabota.x5.ru on 2026-10-14: the list is
// two pages built from five live items (with the page metadata set to two pages so the walk
// is exercised), and each detail is that vacancy's own detail response, unedited.
const (
	x5GoDevID     = "76dc9436-816f-4583-98a3-c04ac13b32e7" // IT / X5 Tech, Markdown body
	x5KemerovoID  = "6a8424f4c63e073bff896508"             // schedule, travel, education, "•" bullets
	x5NoCityID    = "953b09a0-cebe-42ae-9bc0-bf1c8f00cb6d" // city null, office address only
	x5RemoteID    = "76529cde-d025-4e83-9050-ff8e9e07d2b2" // work_format remote, no city
	x5MassVacancy = "79ce08af-b7a5-4cf3-a306-aba78c082a85" // mass funnel, plain-line body
)

func x5Fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/x5/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// x5Fake routes the two list pages and every detail fixture except those named in skip.
func x5Fake(t *testing.T, skip ...string) *routedHTTP {
	t.Helper()
	fake := (&routedHTTP{}).
		route("page=1&", x5Fixture(t, "list_p1.json")).
		route("page=2&", x5Fixture(t, "list_p2.json"))
	for _, id := range []string{x5GoDevID, x5KemerovoID, x5NoCityID, x5RemoteID, x5MassVacancy} {
		if !slices.Contains(skip, id) {
			fake.route("/vacancies/"+id, x5Fixture(t, "detail_"+id+".json"))
		}
	}
	return fake
}

func x5Entry() CompanyEntry { return CompanyEntry{Company: "X5 Group", Provider: "x5"} }

func x5ByID(jobs []Job) map[string]Job {
	byID := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	return byID
}

func TestX5Provider(t *testing.T) {
	if got := NewX5(nil).Provider(); got != "x5" {
		t.Errorf("Provider() = %q, want %q", got, "x5")
	}
}

func TestX5IsBoardlessAndHydrating(t *testing.T) {
	src := NewX5(nil)
	if _, ok := src.(boardless); !ok {
		t.Error("x5 should implement the boardless marker")
	}
	if _, ok := src.(HydratingSource); !ok {
		t.Error("x5 should be a HydratingSource")
	}
}

func TestX5FetchWalksEveryPage(t *testing.T) {
	jobs, err := NewX5(x5Fake(t)).Fetch(context.Background(), x5Entry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("len(jobs) = %d, want 5 (three on page 1, two on page 2)", len(jobs))
	}
}

func TestX5MapsListAndDetailFields(t *testing.T) {
	jobs, err := NewX5(x5Fake(t)).Fetch(context.Background(), x5Entry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	j, ok := x5ByID(jobs)[x5GoDevID]
	if !ok {
		t.Fatal("Go developer vacancy missing")
	}
	if j.Title != "Senior Go developer" {
		t.Errorf("Title = %q", j.Title)
	}
	if want := "https://rabota.x5.ru/vacancies/" + x5GoDevID; j.URL != want {
		t.Errorf("URL = %q, want %q", j.URL, want)
	}
	// The business unit is the hiring employer; the configured entry is only the fallback.
	if j.Company != "X5 Tech" {
		t.Errorf("Company = %q, want the business unit X5 Tech", j.Company)
	}
	if j.Location != "Москва" {
		t.Errorf("Location = %q, want Москва", j.Location)
	}
	if j.WorkMode != "onsite" || j.Remote {
		t.Errorf("WorkMode = %q Remote = %v, want onsite/false for work_format office", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	if !j.IsTechHint {
		t.Error("IsTechHint = false, want true for a vacancy filed under the platform's IT category")
	}
	want := time.Date(2026, 9, 7, 11, 5, 9, 118552000, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
	}
	if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" {
		t.Errorf("salary = %v/%v/%q, want none (the posting states none)", j.SalaryMin, j.SalaryMax, j.SalaryCurrency)
	}

	// The body is the site's own section layout, rendered from the detail's Markdown.
	for _, frag := range []string{
		"<h3>Особенности</h3>", "<li>Белая зарплата</li>",
		"<h3>Обязанности</h3>", "<li>Разработка и поддержка микросервисов на языке Go</li>",
		"<h3>Требования</h3>", "<li>Опыт разработки на Go</li>",
		"<h3>Условия</h3>", "Гибкий график работы",
	} {
		if !strings.Contains(j.Description, frag) {
			t.Errorf("Description missing %q:\n%s", frag, j.Description)
		}
	}
	if strings.Contains(j.Description, "О вакансии") {
		t.Error("an empty content.description must not produce an empty section")
	}
}

func TestX5MapsConditionsAndRequirementFacts(t *testing.T) {
	jobs, err := NewX5(x5Fake(t)).Fetch(context.Background(), x5Entry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	j := x5ByID(jobs)[x5KemerovoID]
	if j.Company != "Чижик" {
		t.Errorf("Company = %q, want Чижик", j.Company)
	}
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid", j.WorkMode)
	}
	if j.EducationLevel != "bachelor" {
		t.Errorf("EducationLevel = %q, want bachelor for Высшее", j.EducationLevel)
	}
	if j.IsTechHint {
		t.Error("IsTechHint = true for an Офис-category vacancy")
	}
	for _, frag := range []string{
		"Образование:</strong> Высшее",
		"Компьютерные навыки:</strong> Уверенный пользователь",
		"График работы:</strong> Пятидневный",
		"Командировки:</strong> Кемеровская, Томская области",
		"Характер работы:</strong> Разъездной",
		// "•" lines are not Markdown list items; hard wraps keep them on their own lines.
		"Навыки владения ПК;<br",
	} {
		if !strings.Contains(j.Description, frag) {
			t.Errorf("Description missing %q:\n%s", frag, j.Description)
		}
	}
}

func TestX5LocationFallsBackToOfficeAddress(t *testing.T) {
	jobs, err := NewX5(x5Fake(t)).Fetch(context.Background(), x5Entry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := x5ByID(jobs)[x5NoCityID].Location, "г. Нижний Новгород, ул. Долгополова, д. 77"; got != want {
		t.Errorf("Location = %q, want the office's full address %q", got, want)
	}
}

func TestX5RemoteWorkFormat(t *testing.T) {
	jobs, err := NewX5(x5Fake(t)).Fetch(context.Background(), x5Entry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	j := x5ByID(jobs)[x5RemoteID]
	if !j.Remote || j.WorkMode != "remote" {
		t.Errorf("Remote = %v WorkMode = %q, want true/remote", j.Remote, j.WorkMode)
	}
}

func TestX5PlainLineBodyKeepsItsLines(t *testing.T) {
	jobs, err := NewX5(x5Fake(t)).Fetch(context.Background(), x5Entry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	d := x5ByID(jobs)[x5MassVacancy].Description
	if !strings.Contains(d, "Оформление по ТК РФ с возможностью профессионального и карьерного роста<br") {
		t.Errorf("mass-vacancy lines collapsed into one run:\n%s", d)
	}
}

func TestX5FetchNewSkipsDetailForSeenPostings(t *testing.T) {
	// The seen posting's detail is not routed: fetching it would fail and drop it.
	fake := x5Fake(t, x5GoDevID)
	jobs, err := NewX5(fake).(HydratingSource).FetchNew(context.Background(), x5Entry(),
		func(id string) bool { return id == x5GoDevID })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	byID := x5ByID(jobs)
	seen, ok := byID[x5GoDevID]
	if !ok {
		t.Fatal("the seen posting was not re-listed")
	}
	if !seen.SeenRefresh || seen.Description != "" || seen.Company != "X5 Tech" || seen.Title != "Senior Go developer" {
		t.Errorf("seen posting = %+v, want an identity-only refresh", seen)
	}
	if fresh := byID[x5KemerovoID]; fresh.SeenRefresh || fresh.Description == "" {
		t.Errorf("unseen posting = %+v, want it hydrated", fresh)
	}
}

func TestX5FetchDropsFailedDetail(t *testing.T) {
	jobs, err := NewX5(x5Fake(t, x5RemoteID)).Fetch(context.Background(), x5Entry())
	if err != nil {
		t.Fatalf("Fetch should not abort on one failed detail: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("len(jobs) = %d, want 4", len(jobs))
	}
	if _, ok := x5ByID(jobs)[x5RemoteID]; ok {
		t.Error("the posting whose detail failed should be dropped")
	}
}

func TestX5ListFailureIsBoardError(t *testing.T) {
	if _, err := NewX5(&routedHTTP{}).Fetch(context.Background(), x5Entry()); err == nil {
		t.Fatal("Fetch() error = nil, want a board error when the list fails")
	}
}

func TestX5Salary(t *testing.T) {
	cases := []struct {
		name               string
		exact, lo, hi      *float64
		wantMin, wantMax   *int
		wantCur, wantRange string
	}{
		{name: "none"},
		{name: "exact", exact: f64(90000), wantMin: intPtr(90000), wantMax: intPtr(90000), wantCur: "RUB"},
		{name: "range", lo: f64(80000), hi: f64(120000), wantMin: intPtr(80000), wantMax: intPtr(120000), wantCur: "RUB"},
		{name: "from only", lo: f64(80000), wantMin: intPtr(80000), wantCur: "RUB"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lo, hi, cur, period := x5Salary(x5Item{Salary: c.exact, SalaryMin: c.lo, SalaryMax: c.hi})
			if !eqIntPtr(lo, c.wantMin) || !eqIntPtr(hi, c.wantMax) || cur != c.wantCur {
				t.Errorf("x5Salary = %v/%v/%q, want %v/%v/%q", lo, hi, cur, c.wantMin, c.wantMax, c.wantCur)
			}
			if cur != "" && period != "month" {
				t.Errorf("period = %q, want month", period)
			}
		})
	}
}

func f64(v float64) *float64 { return &v }

func eqIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
