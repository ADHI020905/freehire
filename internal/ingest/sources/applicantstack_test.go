package sources

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// The fixtures under testdata/applicantstack were recorded from the live site on 2026-10-20:
// ozarkhealth's single-page listing and one of its posting pages, northernohiofieldservicesinc's
// three listing pages (282 postings) plus the "Invalid page number" page past its end, sunbelt's
// listing rendered with no rows (a title filter matching nothing — the same markup a board with
// no openings serves), and a gis posting page whose ld+json leaves title and address null.
func applicantstackFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/applicantstack/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// testApplicantStack is the adapter without the per-board crawl delay.
func testApplicantStack(c HTMLGetter) applicantstack { return applicantstack{http: c} }

func TestApplicantStackProvider(t *testing.T) {
	if got := NewApplicantStack(nil).Provider(); got != "applicantstack" {
		t.Errorf("Provider() = %q, want applicantstack", got)
	}
	if _, ok := NewApplicantStack(nil).(HydratingSource); !ok {
		t.Error("applicantstack must be a HydratingSource")
	}
	if _, ok := NewApplicantStack(nil).(fullBoardListing); !ok {
		t.Error("applicantstack must carry the fullBoardListing marker")
	}
}

func TestApplicantStackFetchNewHydratesOnlyUnseenPostings(t *testing.T) {
	const hydrated = "a23rqqe2fctl"
	const gone = "a23rqqeo6x57"
	fake := (&routedHTTP{}).
		routeErr("/x/detail/"+gone, &StatusError{Method: "GET", Code: 404, URL: "https://ozarkhealth.applicantstack.com/x/detail/" + gone}).
		route("/x/detail/"+hydrated, applicantstackFixture(t, "ozarkhealth_detail_a23rqqe2fctl.html")).
		route("ozarkhealth.applicantstack.com/x/openings", applicantstackFixture(t, "ozarkhealth_openings.html"))

	jobs, err := testApplicantStack(fake).FetchNew(context.Background(),
		CompanyEntry{Company: "Ozark Health", Provider: "applicantstack", Board: "ozarkhealth"},
		func(id string) bool { return id != hydrated && id != gone })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	// 16 listed: 14 seen refreshes, one hydrated, and the posting answering 404 deferred.
	if len(jobs) != 15 {
		t.Fatalf("got %d jobs, want 15", len(jobs))
	}
	if fake.calls != 1+2 {
		t.Errorf("made %d requests, want one listing and two posting pages", fake.calls)
	}
	var got *Job
	refreshes := 0
	for i := range jobs {
		j := &jobs[i]
		if j.SeenRefresh {
			refreshes++
			if j.Title == "" || j.Company != "Ozark Health" || !strings.HasPrefix(j.URL, "https://ozarkhealth.applicantstack.com/x/detail/") {
				t.Errorf("refresh row incomplete: %+v", *j)
			}
			continue
		}
		got = j
	}
	if refreshes != 14 || got == nil {
		t.Fatalf("got %d refreshes and hydrated=%v, want 14 and one", refreshes, got != nil)
	}
	if got.ExternalID != hydrated || got.URL != "https://ozarkhealth.applicantstack.com/x/detail/"+hydrated {
		t.Errorf("identity = %q %q", got.ExternalID, got.URL)
	}
	// The listing's title (whitespace collapsed), not the ld+json's separately configured one.
	if got.Title != "EMERGENCY ROOM RN" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Company != "Ozark Health" {
		t.Errorf("Company = %q, want the configured employer", got.Company)
	}
	if got.Location != "Clinton, AR, United States" || got.Remote {
		t.Errorf("Location = %q remote=%v", got.Location, got.Remote)
	}
	if !strings.Contains(got.Description, "<li>") || !strings.Contains(got.Description, "Proof of the legal right to work") {
		t.Errorf("Description not the sanitized body: %q", got.Description)
	}
	want := time.Date(2026, 10, 7, 20, 9, 22, 0, time.UTC)
	if got.PostedAt == nil || !got.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", got.PostedAt, want)
	}
}

func TestApplicantStackWalksEveryListingPage(t *testing.T) {
	fake := (&routedHTTP{}).
		route("openings?pge=2", applicantstackFixture(t, "northernohio_openings_p2.html")).
		route("openings?pge=3", applicantstackFixture(t, "northernohio_openings_p3.html")).
		route("northernohiofieldservicesinc.applicantstack.com/x/openings", applicantstackFixture(t, "northernohio_openings_p1.html"))

	jobs, err := testApplicantStack(fake).FetchNew(context.Background(),
		CompanyEntry{Company: "Northern Ohio Field Services", Provider: "applicantstack", Board: "northernohiofieldservicesinc"},
		func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 282 || fake.calls != 3 {
		t.Fatalf("got %d jobs in %d requests, want the pager's 282 in 3", len(jobs), fake.calls)
	}
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.ExternalID] = true
	}
	if len(ids) != 282 {
		t.Errorf("got %d distinct ids, want 282", len(ids))
	}
}

// Every way the walk can stop short of the pager's total must fail the crawl: this adapter
// carries fullBoardListing, so a partial listing returned as a success would close the rest.
func TestApplicantStackListingFailsShortOfTheStatedTotal(t *testing.T) {
	p1 := applicantstackFixture(t, "northernohio_openings_p1.html")
	cases := map[string]*routedHTTP{
		// page 2 repeats page 1, adding nothing at 100 of 282
		"page adds nothing": (&routedHTTP{}).route("openings", p1),
		// page 2 is past the end, as the platform renders an out-of-range page
		"page has no table": (&routedHTTP{}).
			route("openings?pge=2", applicantstackFixture(t, "northernohio_openings_p4_invalid.html")).
			route("openings", p1),
		// page 2 fails outright
		"page errors": (&routedHTTP{}).
			routeErr("openings?pge=2", &StatusError{Method: "GET", Code: 503}).
			route("openings", p1),
		// the total moves between pages
		"total changes": (&routedHTTP{}).
			route("openings?pge=2", strings.Replace(applicantstackFixture(t, "northernohio_openings_p2.html"), "of 282", "of 283", 1)).
			route("openings", p1),
	}
	for name, fake := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := testApplicantStack(fake).FetchNew(context.Background(),
				CompanyEntry{Company: "N", Provider: "applicantstack", Board: "northernohiofieldservicesinc"},
				func(string) bool { return true })
			if err == nil {
				t.Fatal("FetchNew succeeded with a partial listing")
			}
		})
	}
}

func TestApplicantStackEmptyBoard(t *testing.T) {
	fake := (&routedHTTP{}).route("openings", applicantstackFixture(t, "sunbelt_openings_empty.html"))
	jobs, err := testApplicantStack(fake).Fetch(context.Background(),
		CompanyEntry{Company: "Sunbelt", Provider: "applicantstack", Board: "sunbelt"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("got %d jobs, err %v; want an empty board read as empty", len(jobs), err)
	}
}

// A page with no openings table is not an empty board: an unknown tenant redirects to the
// vendor's marketing site, and reading that as "no postings" would close the board's catalogue.
func TestApplicantStackPageWithoutTableIsAnError(t *testing.T) {
	fake := (&routedHTTP{}).route("openings", applicantstackFixture(t, "northernohio_openings_p4_invalid.html"))
	if _, err := testApplicantStack(fake).Fetch(context.Background(),
		CompanyEntry{Company: "N", Provider: "applicantstack", Board: "northernohiofieldservicesinc"}); err == nil {
		t.Fatal("Fetch read a page without the openings table as a listing")
	}
}

// The gis posting's ld+json carries a null title and an address with nothing but a null country:
// the listing title stands, and the location stays empty rather than guessed.
func TestApplicantStackDetailWithSparseJSONLD(t *testing.T) {
	fake := (&routedHTTP{}).route("/x/detail/a2w7lo7j4ogo", applicantstackFixture(t, "gis_detail_a2w7lo7j4ogo.html"))
	j, ok := applicantstackDetail(context.Background(), fake,
		CompanyEntry{Company: "GIS Field Services", Board: "gis"},
		applicantstackPosting{id: "a2w7lo7j4ogo", title: "Mortgage Field Inspector"})
	if !ok {
		t.Fatal("detail deferred a posting that has a body")
	}
	if j.Title != "Mortgage Field Inspector" || j.Location != "" || j.Description == "" || j.PostedAt == nil {
		t.Errorf("got %+v", j)
	}
}

func TestApplicantStackDetailWithoutJobPostingIsDeferred(t *testing.T) {
	fake := (&routedHTTP{}).route("/x/detail/", "<html><body>Maintenance</body></html>")
	if _, ok := applicantstackDetail(context.Background(), fake,
		CompanyEntry{Company: "A", Board: "a"}, applicantstackPosting{id: "x1", title: "T"}); ok {
		t.Fatal("a page without a JobPosting must defer the posting, not store it body-less")
	}
}

func TestApplicantStackListingURL(t *testing.T) {
	if got := applicantstackListingURL("m30", 1); got != "https://m30.applicantstack.com/x/openings" {
		t.Errorf("page 1 = %q", got)
	}
	if got := applicantstackListingURL("m30", 3); got != "https://m30.applicantstack.com/x/openings?pge=3" {
		t.Errorf("page 3 = %q", got)
	}
}
