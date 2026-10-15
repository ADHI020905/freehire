package sources

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// The kaspi fixtures are real pages captured from job.kaspi.kz on 2026-10-14: the vacancy
// sitemap cut down to four entries, and each entry's page with the Nuxt state script and
// stylesheets removed (the head's ld+json blocks and the server-rendered body are verbatim).
const (
	kaspiDBA     = "https://job.kaspi.kz/vacancy/junior-database-administrator-postgresql"
	kaspiGid     = "https://job.kaspi.kz/vacancy/kaspi-gid"
	kaspiJava    = "https://job.kaspi.kz/vacancy/middle-java-developer"
	kaspiPartner = "https://job.kaspi.kz/vacancy/menedzher-po-privlecheniyu-partnerov"
)

func kaspiFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/kaspi/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// kaspiFake routes the sitemap and every page fixture except the slugs named in skip.
func kaspiFake(t *testing.T, skip ...string) *routedHTTP {
	t.Helper()
	fake := (&routedHTTP{}).route("/sitemap/vacancies.xml", kaspiFixture(t, "sitemap.xml"))
	for _, u := range []string{kaspiDBA, kaspiGid, kaspiJava, kaspiPartner} {
		slug := kaspiVacancyID(u)
		if !slices.Contains(skip, slug) {
			fake.route(u, kaspiFixture(t, slug+".html"))
		}
	}
	return fake
}

func kaspiEntry() CompanyEntry { return CompanyEntry{Company: "Kaspi.kz", Provider: "kaspi"} }

func kaspiFetch(t *testing.T, fake *routedHTTP) map[string]Job {
	t.Helper()
	jobs, err := NewKaspi(fake).Fetch(context.Background(), kaspiEntry())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	byID := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	return byID
}

func TestKaspiProvider(t *testing.T) {
	if got := NewKaspi(nil).Provider(); got != "kaspi" {
		t.Errorf("Provider() = %q, want %q", got, "kaspi")
	}
}

func TestKaspiIsBoardless(t *testing.T) {
	if _, ok := NewKaspi(nil).(boardless); !ok {
		t.Error("kaspi should implement the boardless marker")
	}
}

func TestKaspiVacancyID(t *testing.T) {
	cases := map[string]string{
		kaspiDBA:                          "junior-database-administrator-postgresql",
		kaspiGid + "/":                    "kaspi-gid",
		kaspiGid + "?utm_source=x":        "kaspi-gid",
		kaspiGid + "/respond":             "", // the apply form, not a vacancy
		"https://job.kaspi.kz/":           "",
		"https://job.kaspi.kz/kk/vacancy": "",
	}
	for loc, want := range cases {
		if got := kaspiVacancyID(loc); got != want {
			t.Errorf("kaspiVacancyID(%q) = %q, want %q", loc, got, want)
		}
	}
}

func TestKaspiMapsPostingAndPageFields(t *testing.T) {
	byID := kaspiFetch(t, kaspiFake(t))
	if len(byID) != 4 {
		t.Fatalf("len(jobs) = %d, want 4", len(byID))
	}
	j, ok := byID["junior-database-administrator-postgresql"]
	if !ok {
		t.Fatal("DBA vacancy missing")
	}
	if j.Title != "Junior Database Administrator (PostgreSQL)" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != kaspiDBA {
		t.Errorf("URL = %q, want %q", j.URL, kaspiDBA)
	}
	if j.Company != "Kaspi.kz" {
		t.Errorf("Company = %q, want the configured Kaspi.kz", j.Company)
	}
	// The ld+json addressLocality is "KZ"; the page's city line is the real location, without
	// its "(Головной офис)" note.
	if j.Location != "Алматы" {
		t.Errorf("Location = %q, want Алматы", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "kz" {
		t.Errorf("Countries = %v, want [kz]", j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time for Полный день", j.EmploymentType)
	}
	// Tagged both "Нет опыта" and "От 1 года до 3 лет": the floor is the lower one.
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 0 {
		t.Errorf("ExperienceYearsMin = %v, want 0", j.ExperienceYearsMin)
	}
	if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" {
		t.Errorf("salary = %v/%v/%q, want none", j.SalaryMin, j.SalaryMax, j.SalaryCurrency)
	}
	// datePosted carries no zone; it matches the sitemap's UTC lastmod to the millisecond.
	want := time.Date(2026, 10, 13, 12, 54, 48, 962797000, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
	}
	if !strings.Contains(j.Description, "<li>Знание SQL (выборки, соединения, агрегации, подзапросы).</li>") {
		t.Errorf("Description lost its list markup:\n%s", j.Description)
	}
	if strings.Contains(j.Description, "&lt;") {
		t.Errorf("Description is still entity-escaped:\n%s", j.Description)
	}
	if j.Remote {
		t.Error("an Almaty office role should not be flagged Remote")
	}
}

func TestKaspiSalaryFromAndExperienceFloor(t *testing.T) {
	j := kaspiFetch(t, kaspiFake(t))["middle-java-developer"]
	if j.SalaryMin == nil || *j.SalaryMin != 800000 || j.SalaryMax != nil {
		t.Errorf("salary = %v/%v, want from 800000", j.SalaryMin, j.SalaryMax)
	}
	if j.SalaryCurrency != "KZT" || j.SalaryPeriod != "month" {
		t.Errorf("currency/period = %q/%q, want KZT/month", j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 2 {
		t.Errorf("ExperienceYearsMin = %v, want 2", j.ExperienceYearsMin)
	}
	// Гибкий график is a schedule, not an employment type.
	if j.EmploymentType != "" {
		t.Errorf("EmploymentType = %q, want empty", j.EmploymentType)
	}
}

func TestKaspiExactSalary(t *testing.T) {
	j := kaspiFetch(t, kaspiFake(t))["menedzher-po-privlecheniyu-partnerov"]
	if j.SalaryMin == nil || j.SalaryMax == nil || *j.SalaryMin != 300000 || *j.SalaryMax != 300000 {
		t.Errorf("salary = %v/%v, want exactly 300000", j.SalaryMin, j.SalaryMax)
	}
}

func TestKaspiMultiCityLocation(t *testing.T) {
	j := kaspiFetch(t, kaspiFake(t))["kaspi-gid"]
	if want := "Алматы, Астана, Жезказган, Усть-Каменогорск"; j.Location != want {
		t.Errorf("Location = %q, want %q", j.Location, want)
	}
}

func TestKaspiSalaryLabel(t *testing.T) {
	cases := []struct {
		label    string
		min, max *int
	}{
		{"от 800 000 тенге", intPtr(800000), nil},
		{"От 220 000 тенге", intPtr(220000), nil},
		{"от 1000 000", intPtr(1000000), nil},
		{"от 200.000", intPtr(200000), nil},
		{"300 000", intPtr(300000), intPtr(300000)},
		{"до 500 000 ₸", nil, intPtr(500000)},
		{"по договорённости", nil, nil},
		{"", nil, nil},
	}
	for _, c := range cases {
		lo, hi, cur, _ := kaspiSalary(c.label)
		if !eqIntPtr(lo, c.min) || !eqIntPtr(hi, c.max) {
			t.Errorf("kaspiSalary(%q) = %v/%v, want %v/%v", c.label, lo, hi, c.min, c.max)
		}
		if (lo != nil || hi != nil) != (cur == "KZT") {
			t.Errorf("kaspiSalary(%q) currency = %q", c.label, cur)
		}
	}
}

func TestKaspiUnreadablePageIsMarkedNotDropped(t *testing.T) {
	// The page has no route, so its fetch fails with a transport error: the crawl could not
	// read it, which is not the platform saying it is gone.
	byID := kaspiFetch(t, kaspiFake(t, "kaspi-gid"))
	j, ok := byID["kaspi-gid"]
	if !ok {
		t.Fatal("an unreadable page was dropped; want an Unreadable marker")
	}
	if !j.Unreadable || j.URL != kaspiGid || j.Company != "Kaspi.kz" {
		t.Errorf("marker = %+v, want Unreadable with identity", j)
	}
}

func TestKaspiGonePageIsDropped(t *testing.T) {
	fake := kaspiFake(t, "kaspi-gid").routeErr(kaspiGid, &StatusError{Code: 404})
	if _, ok := kaspiFetch(t, fake)["kaspi-gid"]; ok {
		t.Error("a 404 page should be dropped, not yielded")
	}
}

func TestKaspiSitemapFailureIsBoardError(t *testing.T) {
	if _, err := NewKaspi(&routedHTTP{}).Fetch(context.Background(), kaspiEntry()); err == nil {
		t.Fatal("Fetch() error = nil, want a board error when the sitemap fails")
	}
}
