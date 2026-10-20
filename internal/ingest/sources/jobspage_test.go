package sources

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

// jobspageListing is a tenant page's __NEXT_DATA__, trimmed to the fields the adapter reads
// plus a Draft.js description it must NOT use as the body.
func jobspageListing(postings ...string) string {
	return `<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"companyData":{"companyName":"Acme"},"jobsData":[` +
		strings.Join(postings, ",") + `]}},"page":"/_sites/[company]"}</script>`
}

func jobspageListingPosting(id, title, remote, employment string, expired bool) string {
	exp := "false"
	if expired {
		exp = "true"
	}
	return `{"id":"` + id + `","jobTitle":"` + title + `","city":"Limassol","country":"CY",` +
		`"remoteJob":"` + remote + `","employmentType":"` + employment + `","isExpired":` + exp + `,` +
		`"description":"{\"blocks\":[{\"text\":\"draft body\"}]}"}`
}

// jobspageDetail is a posting page's schema.org JobPosting block, the only HTML body. Like the
// live page it escapes "<" so a tag in the body cannot close the surrounding script element.
func jobspageDetail(description string) string {
	description = strings.ReplaceAll(description, "<", `\u003c`)
	return `<html><head><script type="application/ld+json">{"@context":"https://schema.org/",` +
		`"@type":"JobPosting","title":"x","description":"` + description + `",` +
		`"datePosted":"2026-10-01T10:23:34.000Z"}</script></head><body></body></html>`
}

// jobspageNotFoundErr is what the client returns for the platform's /404 redirect loop.
func jobspageNotFoundErr(host string) error {
	return &url.Error{Op: "Get", URL: "https://" + host + "/404", Err: errors.New("stopped after 10 redirects")}
}

func TestJobspageProvider(t *testing.T) {
	if got := NewJobspage(nil).Provider(); got != "jobspage" {
		t.Errorf("Provider() = %q, want jobspage", got)
	}
}

func TestJobspageFetchMapsListingAndDetail(t *testing.T) {
	fake := (&routedHTTP{}).
		route("acme.jobspage.co/ab12cd34-backend-developer", jobspageDetail("<p>Own the backend.</p><script>alert(1)</script>")).
		route("acme.jobspage.co", jobspageListing(
			jobspageListingPosting("ab12cd34-backend-developer", "Backend Developer", "Remote only", "Full-time", false),
			jobspageListingPosting("zz99yy88-old-role", "Old Role", "No remote", "Part-time", true),
		))

	jobs, err := NewJobspage(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "jobspage", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (the expired posting is skipped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "ab12cd34-backend-developer" {
		t.Errorf("ExternalID = %q", j.ExternalID)
	}
	if j.URL != "https://acme.jobspage.co/ab12cd34-backend-developer" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "Backend Developer" || j.Company != "Acme" || j.Location != "Limassol, CY" {
		t.Errorf("Title/Company/Location = %q/%q/%q", j.Title, j.Company, j.Location)
	}
	if !strings.Contains(j.Description, "Own the backend.") || strings.Contains(j.Description, "alert") ||
		strings.Contains(j.Description, "draft body") {
		t.Errorf("Description = %q, want the sanitized JobPosting body", j.Description)
	}
	if j.WorkMode != "remote" || !j.Remote {
		t.Errorf("WorkMode/Remote = %q/%v, want remote/true", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time", j.EmploymentType)
	}
	want := time.Date(2026, 10, 1, 10, 23, 34, 0, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
	}
}

func TestJobspageGoneBoardIsErrBoardGone(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("gone.jobspage.co", jobspageNotFoundErr("gone.jobspage.co"))

	_, err := NewJobspage(fake).Fetch(context.Background(), CompanyEntry{Company: "Gone", Provider: "jobspage", Board: "gone"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

// A posting the platform answers /404 for is gone and dropped; one whose page could not be
// read is an unreadable marker, so the sweep does not close a posting that is still listed.
func TestJobspageDetailGoneVersusUnreadable(t *testing.T) {
	fake := (&routedHTTP{}).
		routeErr("acme.jobspage.co/gone0000-role", jobspageNotFoundErr("acme.jobspage.co")).
		routeErr("acme.jobspage.co/flaky000-role", &StatusError{Method: "GET", Code: 503, URL: "x"}).
		route("acme.jobspage.co/nobody00-role", jobspageDetail("")).
		route("acme.jobspage.co", jobspageListing(
			jobspageListingPosting("gone0000-role", "Gone", "No remote", "Other", false),
			jobspageListingPosting("flaky000-role", "Flaky", "No remote", "Other", false),
			jobspageListingPosting("nobody00-role", "No body", "No remote", "Other", false),
		))

	jobs, err := NewJobspage(fake).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "jobspage", Board: "acme"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got := map[string]bool{}
	for _, j := range jobs {
		if !j.Unreadable {
			t.Errorf("%s: want an Unreadable marker, got a posting", j.ExternalID)
		}
		got[j.ExternalID] = true
	}
	if len(jobs) != 2 || !got["flaky000-role"] || !got["nobody00-role"] {
		t.Errorf("got %v, want markers for flaky000-role and nobody00-role only", got)
	}
}

func TestJobspageWorkModeAndEmployment(t *testing.T) {
	for in, want := range map[string]string{"Remote only": "remote", "No remote": "onsite", "Remote friendly": "", "": ""} {
		if got := jobspageWorkMode(in); got != want {
			t.Errorf("jobspageWorkMode(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Full-time": "full_time", "Part-time": "part_time", "Contract": "contract", "Other": ""} {
		if got := jobspageEmploymentType(in); got != want {
			t.Errorf("jobspageEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

// A board with no openings is an empty array; a page without the key is a reshaped page, and
// reading it as empty would close every posting the board still lists.
func TestJobspageListingWithoutJobsDataFails(t *testing.T) {
	empty := (&routedHTTP{}).route("acme.jobspage.co", jobspageListing())
	jobs, err := NewJobspage(empty).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "jobspage", Board: "acme"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("empty board: jobs=%d err=%v, want 0 and no error", len(jobs), err)
	}

	reshaped := (&routedHTTP{}).route("acme.jobspage.co",
		`<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{}}}</script>`)
	if _, err := NewJobspage(reshaped).Fetch(context.Background(), CompanyEntry{Company: "Acme", Provider: "jobspage", Board: "acme"}); err == nil {
		t.Fatal("listing without jobsData: want an error, got none")
	}
}
