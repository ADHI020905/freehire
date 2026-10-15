package sources

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// konturFake serves fixture bodies by exact URL; an unmapped URL fails like a refused request.
// The detail fan-out runs on a worker pool, so the recorder is locked.
type konturFake struct {
	pages map[string]string
	mu    sync.Mutex
	got   []string
}

func (f *konturFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
	body, ok := f.pages[url]
	if !ok {
		return nil, errors.New("konturFake: unmapped " + url)
	}
	return html.Parse(strings.NewReader(body))
}

func konturFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// konturPages maps the real listing (39 vacancies, captured through the proxy 2026-10-15) and the
// real pages of three vacancies, one per work-format label. The fixtures are trimmed to the
// listing container and, for a vacancy, its JobPosting script, text column and sidebar.
func konturPages(t *testing.T) map[string]string {
	return map[string]string{
		konturListURL:                             konturFixture(t, "kontur_list.html"),
		"https://kontur.ru/career/vacancies/655":  konturFixture(t, "kontur_vacancy_655.html"),
		"https://kontur.ru/career/vacancies/4424": konturFixture(t, "kontur_vacancy_4424.html"),
		"https://kontur.ru/career/vacancies/5565": konturFixture(t, "kontur_vacancy_5565.html"),
	}
}

func TestKonturFetchFromFixtures(t *testing.T) {
	f := &konturFake{pages: konturPages(t)}

	jobs, err := NewKontur(f).Fetch(context.Background(), CompanyEntry{Company: "Kontur"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(f.got) != 1+39 {
		t.Fatalf("want 1 listing + 39 detail requests, got %d", len(f.got))
	}
	if len(jobs) != 3 {
		t.Fatalf("want the 3 hydrated postings, got %d", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	// 655: remote-capable (TELECOMMUTE), nine offices, a one-sentence ld+json lead.
	j := byID["655"]
	if j.URL != "https://kontur.ru/career/vacancies/655" || j.Title != "Системный аналитик, middle+/senior" || j.Company != "Kontur" {
		t.Fatalf("655 identity: %+v", j)
	}
	if !j.Remote || j.WorkMode != "remote" {
		t.Fatalf("655 work mode: remote=%v %q", j.Remote, j.WorkMode)
	}
	if !strings.HasPrefix(j.Location, "Екатеринбург; Ижевск") || strings.Count(j.Location, ";") != 8 {
		t.Fatalf("655 location: %q", j.Location)
	}
	if len(j.Countries) != 1 || j.Countries[0] != "ru" {
		t.Fatalf("655 countries: %v", j.Countries)
	}
	if j.EmploymentType != "full_time" {
		t.Fatalf("655 employment type: %q", j.EmploymentType)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 5 {
		t.Fatalf("655 experience: %v", j.ExperienceYearsMin)
	}
	if want := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC); j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Fatalf("655 posted_at: %v", j.PostedAt)
	}
	// The body comes from the text column, far longer than the one-sentence ld+json lead.
	for _, want := range []string{"О команде", "Задачи", "Мы ожидаем", "Мы предлагаем"} {
		if !strings.Contains(j.Description, want) {
			t.Fatalf("655 description missing %q: %q", want, j.Description)
		}
	}

	// 4424: "В офисе или гибридно", no experience label.
	if j := byID["4424"]; j.Remote || j.WorkMode != "hybrid" || j.ExperienceYearsMin != nil {
		t.Fatalf("4424: remote=%v mode=%q exp=%v", j.Remote, j.WorkMode, j.ExperienceYearsMin)
	}
	// 5565: "В офисе", "Можно без опыта".
	if j := byID["5565"]; j.WorkMode != "onsite" || j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 0 {
		t.Fatalf("5565: mode=%q exp=%v", j.WorkMode, j.ExperienceYearsMin)
	}
}

// A stored posting costs no request and keeps the listing's identity.
func TestKonturFetchNewSkipsSeenDetail(t *testing.T) {
	f := &konturFake{pages: map[string]string{konturListURL: konturFixture(t, "kontur_list.html")}}

	jobs, err := NewKontur(f).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Kontur"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(f.got) != 1 || len(jobs) != 39 {
		t.Fatalf("want 1 request and 39 refreshes, got %d and %d", len(f.got), len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || j.Title == "" || !strings.HasPrefix(j.URL, "https://kontur.ru/career/vacancies/") {
			t.Fatalf("bad refresh: %+v", j)
		}
	}
}

// A listing that renders no vacancy list is a stub or a truncated render, not an empty catalogue.
// An empty list that DID render is a quiet source and succeeds.
func TestKonturListingWithoutTheListFails(t *testing.T) {
	stub := &konturFake{pages: map[string]string{konturListURL: `<html><body><header>Контур</header></body></html>`}}
	if _, err := NewKontur(stub).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error for a listing page with no vacancy list")
	}
	empty := &konturFake{pages: map[string]string{konturListURL: `<html><body><div data-vacancy-list-container="/ajax/career/vacancies/list"></div></body></html>`}}
	jobs, err := NewKontur(empty).Fetch(context.Background(), CompanyEntry{})
	if err != nil || len(jobs) != 0 {
		t.Fatalf("an empty rendered list is a quiet source: %v %v", jobs, err)
	}
}

// Listing new postings and reading none of their pages is a broken crawl.
func TestKonturFailsWhenNoNewPostingIsRead(t *testing.T) {
	f := &konturFake{pages: map[string]string{konturListURL: konturFixture(t, "kontur_list.html")}}
	if _, err := NewKontur(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when no vacancy page could be read")
	}
}

// roundTripFunc lets a test stand in for the network beneath the real Client.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Recon saw kontur.ru answer a short stub with no vacancies to a request without a browser Accept
// header. The adapter reads through the shared client's GetHTML, which always sends
// "Accept: text/html"; this pins that the listing request, through the real Client, carries it.
func TestKonturListingRequestSendsHTMLAccept(t *testing.T) {
	listing := konturFixture(t, "kontur_list.html")
	var accept string
	c := &Client{httpClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == konturListURL {
			accept = r.Header.Get("Accept")
		}
		body := listing
		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			body = `<html><body></body></html>` // what the site serves a non-browser Accept
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/html"}},
			Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}}

	jobs, err := NewKontur(c).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if !strings.Contains(accept, "text/html") {
		t.Fatalf("listing Accept = %q, want it to ask for text/html", accept)
	}
	if len(jobs) != 39 {
		t.Fatalf("want 39 listed postings, got %d", len(jobs))
	}
}

func TestKonturIsProxied(t *testing.T) {
	if _, ok := proxiedProviders["kontur"]; !ok {
		t.Error("kontur must be in proxiedProviders (its edge stalls every HTML page from datacenter IPs; served in full via the residential proxy)")
	}
}
