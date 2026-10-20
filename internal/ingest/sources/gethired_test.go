package sources

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// gethiredFake serves one body for the feed request and reports final as the URL the
// request ended on — the requested one unless a test declares a redirect.
type gethiredFake struct {
	body   string
	final  string
	gotURL string
}

func (f *gethiredFake) GetXMLResolved(_ context.Context, url string, v any) (string, error) {
	f.gotURL = url
	final := url
	if f.final != "" {
		final = f.final
	}
	return final, xml.Unmarshal([]byte(f.body), v)
}

func gethiredFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/gethired/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGetHiredProvider(t *testing.T) {
	if got := NewGetHired(nil).Provider(); got != "gethired" {
		t.Errorf("Provider() = %q, want gethired", got)
	}
}

// The fixtures are live feeds recorded 2026-10-20, byte for byte.
func TestGetHiredFetch(t *testing.T) {
	fake := &gethiredFake{body: gethiredFixture(t, "millersmarketnorwalk.xml")}
	jobs, err := NewGetHired(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "gethired", Board: "millersmarketnorwalk", Company: "Miller's Market",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fake.gotURL != "https://millersmarketnorwalk.easyapply.co/rss" {
		t.Errorf("requested %q, want the tenant's feed", fake.gotURL)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	j := jobs[0]
	if j.ExternalID != "produce-clerk-120" {
		t.Errorf("ExternalID = %q, want the guid's slug", j.ExternalID)
	}
	if j.URL != "https://easyapply.co/job/produce-clerk-120" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Title != "produce clerk" || j.Company != "Miller's Market" || j.Location != "Norwalk, OH" {
		t.Errorf("title/company/location = %q / %q / %q", j.Title, j.Company, j.Location)
	}
	if j.Remote {
		t.Error("an onsite location must not read as remote")
	}
	if !strings.Contains(j.Description, "<p>part time produce clerk") {
		t.Errorf("Description = %q, want the CDATA body as HTML", j.Description)
	}
	want := time.Date(2026, 9, 29, 10, 48, 27, 0, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
	}
	if jobs[1].Title != "meat dept" {
		t.Errorf("Title = %q, want the feed's trailing space trimmed", jobs[1].Title)
	}
}

// The slug is minted once and kept through a title edit, so the feed can carry a posting
// titled "...East" under a "...west-1" slug; the slug, not the title, is the identity.
func TestGetHiredFetchRemoteAndStableSlug(t *testing.T) {
	fake := &gethiredFake{body: gethiredFixture(t, "conturavetusainc.xml")}
	jobs, err := NewGetHired(fake).Fetch(context.Background(), CompanyEntry{
		Provider: "gethired", Board: "conturavetusainc", Company: "Contura Vet USA Inc",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 6 {
		t.Fatalf("len(jobs) = %d, want 6", len(jobs))
	}
	ids := map[string]bool{}
	remote := 0
	for _, j := range jobs {
		ids[j.ExternalID] = true
		if j.Remote {
			remote++
			if j.Location != "Remote" {
				t.Errorf("remote job %q has location %q", j.ExternalID, j.Location)
			}
		}
	}
	if len(ids) != 6 {
		t.Errorf("distinct ids = %d, want 6", len(ids))
	}
	if remote != 4 {
		t.Errorf("remote = %d, want 4", remote)
	}
	if !ids["professional-field-services-veterinarian-west-1"] {
		t.Error("missing the slug kept through a title edit")
	}
}

// A subdomain the platform does not serve redirects to its sign-in page; that must be
// reported as the board being gone, not as an XML syntax error.
func TestGetHiredFetchBoardGone(t *testing.T) {
	fake := &gethiredFake{
		body:  "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"></head></html>",
		final: "https://nosuchtenant.easyapply.co/login",
	}
	_, err := NewGetHired(fake).Fetch(context.Background(), CompanyEntry{Board: "nosuchtenant"})
	if !errors.Is(err, ErrBoardGone) {
		t.Fatalf("err = %v, want ErrBoardGone", err)
	}
}

func TestGetHiredFetchEmptyBoard(t *testing.T) {
	fake := &gethiredFake{body: `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>API</title><description>Current Job Openings</description></channel></rss>`}
	jobs, err := NewGetHired(fake).Fetch(context.Background(), CompanyEntry{Board: "api"})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs, err = %d, %v; want an empty, successful crawl", len(jobs), err)
	}
}
