package sources

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

// potokFixture reads a captured Potok page from testdata.
func potokFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestPotokProvider(t *testing.T) {
	if got := NewPotok(nil).Provider(); got != "potok" {
		t.Errorf("Provider() = %q, want potok", got)
	}
	src := NewPotok(nil)
	if _, ok := src.(HydratingSource); !ok {
		t.Error("potok must be a HydratingSource: the listing carries no body")
	}
	if _, ok := src.(fullBoardListing); !ok {
		t.Error("potok must be fullBoardListing: the listing is one unpaginated page")
	}
	if _, ok := src.(boardless); ok {
		t.Error("potok is board-keyed (board = tenant subdomain)")
	}
}

func TestPotokListingReadsEveryPosting(t *testing.T) {
	fake := (&routedHTTP{}).route("skyengteam.potok.io/open/jobs/", potokFixture(t, "potok_list_skyengteam.html"))

	postings, err := potok{http: fake}.list(context.Background(), CompanyEntry{Board: "skyengteam"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(postings) != 16 {
		t.Fatalf("len(postings) = %d, want 16 (the captured page lists 16)", len(postings))
	}
	var got *potokPosting
	for i := range postings {
		if postings[i].id == "2747" {
			got = &postings[i]
		}
	}
	if got == nil {
		t.Fatal("posting 2747 not listed")
	}
	if got.title != "Senior Full Stack Developer (PHP, Angular)" || got.location != "Москва" || got.team != "IT" {
		t.Errorf("posting 2747 = %+v", *got)
	}
}

func TestPotokFetchHydratesFromDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/open/jobs/2747", potokFixture(t, "potok_detail_2747.html")).
		route("skyengteam.potok.io/open/jobs/", potokFixture(t, "potok_list_skyengteam.html"))

	jobs, err := NewPotok(fake).Fetch(context.Background(), CompanyEntry{Company: "Skyeng", Provider: "potok", Board: "skyengteam"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// Only 2747's detail is routed; every other detail fails and is dropped.
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "2747" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != "https://skyengteam.potok.io/open/jobs/2747" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Senior Full Stack Developer (PHP, Angular)" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.Company != "Skyeng" {
		t.Errorf("Company = %q, want the configured company", j.Company)
	}
	if j.Location != "Москва" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode = %q Remote = %v, want remote (schedule \"Удаленная работа\")", j.WorkMode, j.Remote)
	}
	if !strings.Contains(j.Description, "PHP") || strings.Contains(j.Description, "Откликнуться") {
		t.Errorf("Description should be the .plain-list body only, got %.200q", j.Description)
	}
	if j.PostedAt != nil {
		t.Errorf("PostedAt = %v, want nil (neither page states one)", j.PostedAt)
	}
}

func TestPotokFetchNewSkipsDetailForSeenPostings(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/open/jobs/2747", potokFixture(t, "potok_detail_2747.html")).
		route("skyengteam.potok.io/open/jobs/", potokFixture(t, "potok_list_skyengteam.html"))

	seen := func(id string) bool { return id != "2747" }
	jobs, err := NewPotok(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Skyeng", Provider: "potok", Board: "skyengteam"}, seen)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 16 {
		t.Fatalf("len(jobs) = %d, want 16 (15 refreshes + 1 hydrated)", len(jobs))
	}
	refreshes := 0
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshes++
			if j.Title == "" || j.Company != "Skyeng" || j.URL == "" {
				t.Errorf("a refresh must carry its identity and title, got %+v", j)
			}
			continue
		}
		if j.ExternalID != "2747" || j.Description == "" {
			t.Errorf("hydrated job = %+v", j)
		}
	}
	if refreshes != 15 {
		t.Errorf("refreshes = %d, want 15", refreshes)
	}
	// One listing request plus one detail request: seen postings cost nothing.
	if fake.calls != 2 {
		t.Errorf("requests = %d, want 2", fake.calls)
	}
}

func TestPotokEmptyTenantIsAnEmptyBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("/open/jobs/", potokFixture(t, "potok_list_empty.html"))

	jobs, err := NewPotok(fake).Fetch(context.Background(), CompanyEntry{Company: "Золотое Яблоко", Board: "zolotoe-yabloko"})
	if err != nil {
		t.Fatalf("Fetch: %v (a tenant with no postings is an empty board, not a failure)", err)
	}
	if len(jobs) != 0 {
		t.Errorf("len(jobs) = %d, want 0", len(jobs))
	}
}

func TestPotokPageWithoutAListingFails(t *testing.T) {
	// A 200 that is not the career page (the recruiter SPA shell, a changed template) must not
	// read as an empty board: the board-scoped sweep would close every posting on it.
	fake := (&routedHTTP{}).route("/open/jobs/", `<html><head><title>Potok</title></head><body><div id="app"></div></body></html>`)

	if _, err := NewPotok(fake).Fetch(context.Background(), CompanyEntry{Board: "acme"}); err == nil {
		t.Fatal("Fetch succeeded on a page with no postings wrapper; want an error")
	}
}

func TestPotokListingFailureFailsTheBoard(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("/open/jobs/", &StatusError{Method: "GET", Code: http.StatusNotFound, URL: "x"})

	if _, err := NewPotok(fake).Fetch(context.Background(), CompanyEntry{Board: "gone"}); err == nil {
		t.Fatal("Fetch succeeded on a 404 listing; want an error")
	}
}

// potokHubListing is the shape of a hub tenant (tehnopark-skolkovo): every group is one
// resident company.
const potokHubListing = `<html><head><title>Technopark Skolkovo</title></head><body>
<div class="postings-wrapper">
  <div class="postings-group">
    <div class="posting-category-title large-category-label">
      ООО Вандер Технолоджис
    </div>
    <div class="posting">
      <a class="posting-title" target="_blank" href="/open/jobs/357052">
        <h5>Backend   Developer</h5>
        <div class="posting-categories">
          <span href="#" class="sort-by-location posting-category small-category-label">Москва</span>
        </div>
      </a>
    </div>
  </div>
  <div class="postings-group">
    <div class="posting-category-title large-category-label"> </div>
    <div class="posting">
      <a class="posting-title" target="_blank" href="/open/jobs/1">
        <h5>Office Manager</h5>
      </a>
    </div>
  </div>
</div></body></html>`

func TestPotokHubBoardAttributesEachGroupsCompany(t *testing.T) {
	fake := (&routedHTTP{}).route("/open/jobs/", potokHubListing)
	seenAll := func(string) bool { return true }

	hub := CompanyEntry{Company: "Технопарк Сколково", Board: "tehnopark-skolkovo", Hub: true}
	jobs, err := NewPotok(fake).(HydratingSource).FetchNew(context.Background(), hub, seenAll)
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	if jobs[0].Company != "ООО Вандер Технолоджис" {
		t.Errorf("hub job company = %q, want the group's company", jobs[0].Company)
	}
	if jobs[0].Title != "Backend Developer" {
		t.Errorf("Title = %q, want whitespace collapsed", jobs[0].Title)
	}
	if jobs[1].Company != "Технопарк Сколково" {
		t.Errorf("a blank group falls back to the configured company, got %q", jobs[1].Company)
	}

	plain := CompanyEntry{Company: "Технопарк Сколково", Board: "tehnopark-skolkovo"}
	jobs, _ = NewPotok(fake).(HydratingSource).FetchNew(context.Background(), plain, seenAll)
	if len(jobs) == 0 || jobs[0].Company != "Технопарк Сколково" {
		t.Errorf("a non-hub board keeps the configured company, got %+v", jobs)
	}
}
