package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// hcsFake answers GetJSON from real responses keyed by URL. An unknown URL is a 404, the
// platform's answer for a posting id it does not hold.
type hcsFake struct {
	bodies map[string]string
	errs   map[string]error

	mu   sync.Mutex
	urls []string
}

func (f *hcsFake) GetJSON(_ context.Context, u string, v any) error {
	f.mu.Lock()
	f.urls = append(f.urls, u)
	f.mu.Unlock()
	if err := f.errs[u]; err != nil {
		return err
	}
	raw, ok := f.bodies[u]
	if !ok {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	return json.Unmarshal([]byte(raw), v)
}

func hcsFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/hcshiring/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const hcsRegencyBase = "https://regencypark.hcshiring.com"

// hcsRegencyPark is regencypark.hcshiring.com as captured 2026-10-20: sixteen postings over
// two listing pages, and the body of one of them.
func hcsRegencyPark(t *testing.T) *hcsFake {
	return &hcsFake{bodies: map[string]string{
		hcsRegencyBase + "/api/jobs?page=1":                               hcsFixture(t, "regencypark_page1.json"),
		hcsRegencyBase + "/api/jobs?page=2":                               hcsFixture(t, "regencypark_page2.json"),
		hcsRegencyBase + "/api/jobDescriptions?id=69Vh-JzX6EaPPt-WP-GBFA": hcsFixture(t, "regencypark_detail_69Vh.json"),
	}, errs: map[string]error{}}
}

func TestHCSHiringProviderAndMarkers(t *testing.T) {
	s := NewHCSHiring(nil)
	if s.Provider() != "hcshiring" {
		t.Errorf("Provider() = %q", s.Provider())
	}
	if _, ok := s.(HydratingSource); !ok {
		t.Error("hcshiring must hydrate: the body lives only on the per-posting call")
	}
	if _, ok := s.(boardless); ok {
		t.Error("hcshiring is board-keyed, not boardless")
	}
	if !FullBoardListingProviders(All(nil))["hcshiring"] {
		t.Error("FullBoardListingProviders(All(nil)) should include hcshiring")
	}
}

func TestHCSHiringFetchNew(t *testing.T) {
	fake := hcsRegencyPark(t)
	// One unseen posting's body cannot be read this run (a 500 that survived the client's
	// retries); one unseen posting is gone (404, no fixture).
	const unreadable, gone, hydrated = "hs4dEISPR0ym2tqFezL11A", "nAlyDrD4IkWFViQk4_Rq7w", "69Vh-JzX6EaPPt-WP-GBFA"
	fake.errs[hcsRegencyBase+"/api/jobDescriptions?id="+unreadable] = &StatusError{Method: "GET", Code: 500}
	unseen := map[string]bool{unreadable: true, gone: true, hydrated: true}

	jobs, err := NewHCSHiring(fake).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Provider: "hcshiring", Board: "RegencyPark", Company: "Regency at the Park"},
		func(id string) bool { return !unseen[id] })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 15 {
		t.Fatalf("got %d jobs, want 15 (16 listed, one gone)", len(jobs))
	}

	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	if _, ok := byID[gone]; ok {
		t.Error("a posting the platform answers 404 for must be dropped")
	}

	h := byID[hydrated]
	if h.SeenRefresh {
		t.Error("an unseen posting must be hydrated, not refreshed")
	}
	if h.Title != "Certified Occupational Therapist Assistant - COTA" {
		t.Errorf("Title = %q", h.Title)
	}
	if h.URL != hcsRegencyBase+"/jobs/"+hydrated {
		t.Errorf("URL = %q", h.URL)
	}
	if h.Company != "Regency at the Park" {
		t.Errorf("Company = %q", h.Company)
	}
	if h.Location != "College Place, WA 99324" {
		t.Errorf("Location = %q", h.Location)
	}
	if len(h.Countries) != 1 || h.Countries[0] != "us" {
		t.Errorf("Countries = %v", h.Countries)
	}
	if h.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q, want full_time from Status \"Full Time\"", h.EmploymentType)
	}
	if h.SalaryMin == nil || *h.SalaryMin != 34 || h.SalaryMax == nil || *h.SalaryMax != 39 ||
		h.SalaryPeriod != "hour" || h.SalaryCurrency != "USD" {
		t.Errorf("salary = %v-%v %s/%s, want 34-39 USD/hour", h.SalaryMin, h.SalaryMax, h.SalaryCurrency, h.SalaryPeriod)
	}
	if h.PostedAt == nil || !h.PostedAt.Equal(time.Date(2026, 10, 19, 15, 17, 38, 560421000, time.UTC)) {
		t.Errorf("PostedAt = %v", h.PostedAt)
	}
	if !strings.Contains(h.Description, "Certified Occupational Therapist Assistant assumes clinical responsibility") {
		t.Errorf("Description lacks the posting body: %.200q", h.Description)
	}
	if strings.Contains(h.Description, "<div class='location'>") {
		t.Error("Description must be sanitized")
	}

	u := byID[unreadable]
	if u.SeenRefresh || u.Description != "" || u.Title == "" {
		t.Errorf("an unreadable body must ingest list-only: %+v", u)
	}
	if u.EmploymentType != "" {
		t.Errorf("EmploymentType = %q, want empty for Status \"PRN\"", u.EmploymentType)
	}

	refreshed := 0
	for _, j := range jobs {
		if j.SeenRefresh {
			refreshed++
		}
	}
	if refreshed != 13 {
		t.Errorf("got %d liveness refreshes, want 13", refreshed)
	}
	for _, u := range fake.urls {
		if strings.Contains(u, "jobDescriptions") && !unseen[strings.TrimPrefix(u, hcsRegencyBase+"/api/jobDescriptions?id=")] {
			t.Errorf("fetched a body for a seen posting: %s", u)
		}
	}
}

func TestHCSHiringLaterPageFailureFailsTheCrawl(t *testing.T) {
	fake := hcsRegencyPark(t)
	fake.errs[hcsRegencyBase+"/api/jobs?page=2"] = errors.New("timeout")
	if _, err := NewHCSHiring(fake).Fetch(context.Background(), CompanyEntry{Board: "regencypark"}); err == nil {
		t.Fatal("a failed later page must fail the crawl: the board is swept board-scoped")
	}
}

func TestHCSHiringShortReadFailsTheCrawl(t *testing.T) {
	fake := hcsRegencyPark(t)
	// Page 2 answering empty leaves 10 of the declared 16 read.
	fake.bodies[hcsRegencyBase+"/api/jobs?page=2"] = `{"jobs":[],"meta":{"page":2,"perPage":10,"totalJobs":16,"totalPages":2}}`
	_, err := NewHCSHiring(fake).Fetch(context.Background(), CompanyEntry{Board: "regencypark"})
	if err == nil || !strings.Contains(err.Error(), "read 10 distinct postings, listing declares 16") {
		t.Fatalf("err = %v, want the short read reported", err)
	}
}

func TestHCSHiringUnknownTenantFails(t *testing.T) {
	fake := &hcsFake{}
	if _, err := NewHCSHiring(fake).Fetch(context.Background(), CompanyEntry{Board: "nosuchtenant"}); err == nil {
		t.Fatal("an unknown tenant (404 on the listing) must fail the crawl")
	}
}

func TestParseHCSSalary(t *testing.T) {
	cases := []struct {
		in     string
		lo, hi int
		period string
		none   bool
	}{
		{in: "38.50 - 55.71 Hourly", lo: 39, hi: 56, period: "hour"},
		{in: "13.00 Hourly", lo: 13, hi: 13, period: "hour"},
		{in: "85000.00 - 95000.00 Annually", lo: 85000, hi: 95000, period: "year"},
		{in: "110000.00 Annually", lo: 110000, hi: 110000, period: "year"},
		{in: "800.00 Weekly", none: true},
		{in: "DOE", none: true},
		{in: "", none: true},
	}
	for _, c := range cases {
		lo, hi, period := parseHCSSalary(c.in)
		if c.none {
			if lo != nil || hi != nil || period != "" {
				t.Errorf("parseHCSSalary(%q) = %v-%v %q, want nothing", c.in, lo, hi, period)
			}
			continue
		}
		if lo == nil || hi == nil || *lo != c.lo || *hi != c.hi || period != c.period {
			t.Errorf("parseHCSSalary(%q) = %v-%v %q, want %d-%d %q", c.in, lo, hi, period, c.lo, c.hi, c.period)
		}
	}
}
