package sources

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

func uzumFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// uzumSite routes the real sitemap and the three real vacancy pages it names; the fourth
// vacancy the sitemap lists (3700) answers 404, as it did live.
func uzumSite(t *testing.T) *routedHTTP {
	t.Helper()
	return (&routedHTTP{}).
		route("sitemap.xml", uzumFixture(t, "uzum_sitemap.xml")).
		routeErr("/ru/vacancies/3700", &StatusError{Method: "GET", Code: 404, URL: "https://people.uzum.com/career/ru/vacancies/3700"}).
		route("/ru/vacancies/3384", uzumFixture(t, "uzum_vacancy_3384.html")).
		route("/ru/vacancies/3692", uzumFixture(t, "uzum_vacancy_3692.html")).
		route("/ru/vacancies/3770", uzumFixture(t, "uzum_vacancy_3770.html"))
}

func uzumFetch(t *testing.T, fake *routedHTTP) []Job {
	t.Helper()
	jobs, err := NewUzum(fake).Fetch(context.Background(), CompanyEntry{Company: "Uzum", Provider: "uzum"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	slices.SortFunc(jobs, func(a, b Job) int { return strings.Compare(a.ExternalID, b.ExternalID) })
	return jobs
}

func TestUzumFetchReadsTheRussianVacanciesTheSitemapLists(t *testing.T) {
	jobs := uzumFetch(t, uzumSite(t))

	// The /uz/ copies of the same ids and the site's non-vacancy pages are not postings, and
	// 3700 answered 404 — the site's own word that it is gone.
	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ExternalID)
	}
	if want := []string{"3384", "3692", "3770"}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

func TestUzumMapsARemoteVacancy(t *testing.T) {
	// 3384 is a remote role, and remote roles are exactly the ones the site publishes NO
	// JobPosting ld+json for — so this is the case the RSC payload exists to cover.
	j := uzumFetch(t, uzumSite(t))[0]

	if j.URL != "https://people.uzum.com/career/ru/vacancies/3384" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != `Senior System Analyst \ Cистемный Аналитик` {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Uzum Bank" {
		t.Errorf("Company = %q, want the business unit Uzum Bank", j.Company)
	}
	if j.Location != "Ташкент, Узбекистан" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode = %q, Remote = %v, want remote/true", j.WorkMode, j.Remote)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 6 {
		t.Errorf("ExperienceYearsMin = %v, want 6", j.ExperienceYearsMin)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-06-12" {
		t.Errorf("PostedAt = %v, want 2026-06-12 (publication_date is MM/DD/YYYY)", j.PostedAt)
	}
	if j.EmploymentType != "" || j.Seniority != "" {
		t.Errorf("EmploymentType = %q, Seniority = %q, want both empty for a non-trainee role", j.EmploymentType, j.Seniority)
	}
	if !strings.Contains(j.Description, "Чем предстоит заниматься") {
		t.Errorf("Description lacks the body text: %q", j.Description)
	}
	if strings.Contains(j.Description, "components") || strings.Contains(j.Description, "displayType") {
		t.Errorf("Description leaks the page-builder JSON: %q", j.Description)
	}
}

func TestUzumMapsAnOfficeVacancy(t *testing.T) {
	j := uzumFetch(t, uzumSite(t))[1]

	// title_ru carries a trailing newline on the site.
	if j.Title != "Менеджер по работе с логистическими партнерами" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Uzum Tezkor" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.WorkMode != "onsite" || j.Remote {
		t.Errorf("WorkMode = %q, Remote = %v, want onsite/false", j.WorkMode, j.Remote)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 1 {
		t.Errorf("ExperienceYearsMin = %v, want 1", j.ExperienceYearsMin)
	}
	if j.PostedAt == nil || j.PostedAt.Format("2006-01-02") != "2026-10-06" {
		t.Errorf("PostedAt = %v, want 2026-10-06", j.PostedAt)
	}
}

func TestUzumMapsATraineeVacancy(t *testing.T) {
	j := uzumFetch(t, uzumSite(t))[2]

	if j.Title != "Стажёр в направление финансовой отчётности" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.EmploymentType != "internship" || j.Seniority != "intern" {
		t.Errorf("EmploymentType = %q, Seniority = %q, want internship/intern", j.EmploymentType, j.Seniority)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 0 {
		t.Errorf("ExperienceYearsMin = %v, want 0 (no_experience)", j.ExperienceYearsMin)
	}
}

func TestUzumUnreadableDetailIsAMarkerNotADrop(t *testing.T) {
	fake := (&routedHTTP{}).
		route("sitemap.xml", uzumFixture(t, "uzum_sitemap.xml")).
		routeErr("/ru/vacancies/3700", &StatusError{Method: "GET", Code: 404}).
		routeErr("/ru/vacancies/3384", &StatusError{Method: "GET", Code: 503}).
		route("/ru/vacancies/3692", `<html><body><p>re-templated</p></body></html>`).
		route("/ru/vacancies/3770", uzumFixture(t, "uzum_vacancy_3770.html"))

	jobs := uzumFetch(t, fake)
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	for _, j := range jobs[:2] {
		if !j.Unreadable || j.Company != "Uzum" {
			t.Errorf("job %s: Unreadable = %v, Company = %q, want an unreadable marker under the entry's company", j.ExternalID, j.Unreadable, j.Company)
		}
	}
	if jobs[2].Unreadable {
		t.Errorf("job 3770 read fine and must not be a marker")
	}
}

func TestUzumWithdrawnVacancyIsDropped(t *testing.T) {
	withdrawn := strings.Replace(uzumFixture(t, "uzum_vacancy_3692.html"), `\"disabled\":false`, `\"disabled\":true`, 1)
	fake := (&routedHTTP{}).
		route("sitemap.xml", uzumFixture(t, "uzum_sitemap.xml")).
		routeErr("/ru/vacancies/3700", &StatusError{Method: "GET", Code: 404}).
		route("/ru/vacancies/3384", uzumFixture(t, "uzum_vacancy_3384.html")).
		route("/ru/vacancies/3692", withdrawn).
		route("/ru/vacancies/3770", uzumFixture(t, "uzum_vacancy_3770.html"))

	for _, j := range uzumFetch(t, fake) {
		if j.ExternalID == "3692" {
			t.Fatalf("a disabled vacancy was yielded: %+v", j)
		}
	}
}

func TestUzumReadingNothingOfWhatWasListedFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).
		route("sitemap.xml", uzumFixture(t, "uzum_sitemap.xml")).
		routeErr("/ru/vacancies/", errors.New("connection reset"))

	if _, err := NewUzum(fake).Fetch(context.Background(), CompanyEntry{Company: "Uzum"}); err == nil {
		t.Fatal("Fetch succeeded with every vacancy unreadable, want an error")
	}
}

func TestUzumSitemapWithoutVacanciesFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).route("sitemap.xml",
		`<?xml version="1.0" encoding="UTF-8"?><urlset><url><loc>https://people.uzum.com/career/ru</loc></url></urlset>`)

	if _, err := NewUzum(fake).Fetch(context.Background(), CompanyEntry{Company: "Uzum"}); err == nil {
		t.Fatal("Fetch succeeded on a sitemap listing no vacancy, want an error")
	}
}

func TestUzumBrokenSitemapFailsTheCrawl(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("sitemap.xml", errors.New("origin down"))

	if _, err := NewUzum(fake).Fetch(context.Background(), CompanyEntry{Company: "Uzum"}); err == nil {
		t.Fatal("Fetch succeeded with the sitemap unreachable, want an error")
	}
}

func TestUzumBodyHTML(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			name: "text components are concatenated",
			in:   `{"components":[{"type":"text","value":{"body":"<p>a</p>"}},{"type":"text","value":{"body":"<p>b</p>"}}]}`,
			want: "<p>a</p><p>b</p>",
		},
		{
			name: "a component that is not text costs only itself",
			in:   `{"components":[{"type":"gallery","value":{"images":["x.png"]}},{"type":"text","value":{"body":"<p>b</p>"}}]}`,
			want: "<p>b</p>",
		},
		{
			name: "a body that is not page-builder JSON is the HTML itself",
			in:   "<p>plain</p>",
			want: "<p>plain</p>",
		},
		{name: "empty", in: "", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := uzumBodyHTML(c.in); got != c.want {
				t.Errorf("uzumBodyHTML = %q, want %q", got, c.want)
			}
		})
	}
}
