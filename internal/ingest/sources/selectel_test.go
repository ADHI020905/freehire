package sources

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"
)

// selectelFake serves fixture bodies by exact URL; an unmapped URL fails like a refused request.
// The adapter fans its detail fetches out across a worker pool, so the recorder is locked.
type selectelFake struct {
	pages map[string]string
	mu    sync.Mutex
	got   []string
}

func (f *selectelFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.mu.Lock()
	f.got = append(f.got, url)
	f.mu.Unlock()
	body, ok := f.pages[url]
	if !ok {
		return nil, errors.New("selectelFake: unmapped " + url)
	}
	return html.Parse(strings.NewReader(body))
}

func selectelFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

// selectelNuxtPage wraps a devalue payload in the script tag the real pages carry it in.
func selectelNuxtPage(payload string) string {
	return `<html><body><script type="application/json" id="__NUXT_DATA__">` + payload + `</script></body></html>`
}

// TestSelectelFetchFromFixtures reads the real listing (27 vacancies, captured 2026-10-15) and the
// real page of vacancy 1772. Every other vacancy page is unmapped, so its detail fails and the
// posting is dropped rather than stored body-less.
func TestSelectelFetchFromFixtures(t *testing.T) {
	f := &selectelFake{pages: map[string]string{
		selectelListURL: selectelFixture(t, "selectel_list.html"),
		"https://selectel.ru/careers/all/vacancy/1772/": selectelFixture(t, "selectel_vacancy.html"),
	}}

	jobs, err := NewSelectel(f).Fetch(context.Background(), CompanyEntry{Company: "Selectel"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(f.got) != 28 {
		t.Fatalf("want 1 listing + 27 detail requests, got %d", len(f.got))
	}
	if len(jobs) != 1 {
		t.Fatalf("want only the hydrated posting, got %d: %+v", len(jobs), jobs)
	}
	j := jobs[0]
	if j.ExternalID != "1772" || j.URL != "https://selectel.ru/careers/all/vacancy/1772/" {
		t.Fatalf("identity: %q %q", j.ExternalID, j.URL)
	}
	if j.Title != "Solution sales менеджер (AI и ML решения)" || j.Company != "Selectel" {
		t.Fatalf("title/company: %q %q", j.Title, j.Company)
	}
	if j.Location != "Санкт-Петербург" {
		t.Fatalf("location: %q", j.Location)
	}
	if j.Remote || j.WorkMode != "" {
		t.Fatalf("1772 is not remote-available: remote=%v mode=%q", j.Remote, j.WorkMode)
	}
	want := time.Date(2026, 6, 24, 15, 19, 35, 157188000, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Fatalf("posted_at: %v, want %v", j.PostedAt, want)
	}
	// short_desc leads, detailed_desc follows.
	intro := strings.Index(j.Description, "В Selectel мы строим экосистему")
	tasks := strings.Index(j.Description, "Основные задачи")
	if intro < 0 || tasks < 0 || intro > tasks {
		t.Fatalf("description should hold short_desc then detailed_desc: %q", j.Description)
	}
}

// A stored posting costs no request: it comes back as a SeenRefresh carrying its identity.
func TestSelectelFetchNewSkipsSeenDetail(t *testing.T) {
	f := &selectelFake{pages: map[string]string{selectelListURL: selectelFixture(t, "selectel_list.html")}}

	jobs, err := NewSelectel(f).(HydratingSource).FetchNew(context.Background(),
		CompanyEntry{Company: "Selectel"}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(f.got) != 1 {
		t.Fatalf("want only the listing request, got %v", f.got)
	}
	if len(jobs) != 27 {
		t.Fatalf("want 27 refreshes, got %d", len(jobs))
	}
	for _, j := range jobs {
		if !j.SeenRefresh || j.ExternalID == "" || j.Title == "" ||
			!strings.HasPrefix(j.URL, "https://selectel.ru/careers/all/vacancy/") {
			t.Fatalf("bad refresh: %+v", j)
		}
	}
}

// is_remote_available is the site's structured remote flag; conditions is the third body part.
func TestSelectelDetailRemoteAndConditions(t *testing.T) {
	list := selectelNuxtPage(`[{"store":1},{"item_count":2,"items":3,"page_count":2,"page":2},1,[4],` +
		`{"id":5,"title":6,"city":7,"published_at":9,"is_remote_available":10},1900,"Fullstack-разработчик",` +
		`{"name":8},"Санкт-Петербург","2026-09-23T15:40:35+03:00",true]`)
	detail := selectelNuxtPage(`[{"vacancy":1},{"id":2,"title":3,"city":4,"short_desc":6,"detailed_desc":7,` +
		`"conditions":8,"published_at":9,"is_remote_available":10},1900,"Fullstack-разработчик",{"name":5},` +
		`"Санкт-Петербург","<p>Intro</p>","<p>Tasks</p>","<p>Perks</p>","2026-09-23T15:40:35+03:00",true]`)
	f := &selectelFake{pages: map[string]string{
		selectelListURL: list,
		"https://selectel.ru/careers/all/vacancy/1900/": detail,
	}}

	jobs, err := NewSelectel(f).Fetch(context.Background(), CompanyEntry{Company: "Selectel"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %+v", jobs)
	}
	j := jobs[0]
	if !j.Remote || j.WorkMode != "remote" {
		t.Fatalf("remote: %v %q", j.Remote, j.WorkMode)
	}
	if !strings.Contains(j.Description, "Perks") {
		t.Fatalf("conditions missing: %q", j.Description)
	}
}

// The listing is one page by design; a payload saying otherwise is a truncated catalogue and must
// fail the crawl instead of letting the sweep close what it did not list.
func TestSelectelListingRefusesATruncatedCatalogue(t *testing.T) {
	cases := map[string]string{
		"second page": `[{"store":1},{"item_count":2,"items":3,"page_count":4},1,[],2]`,
		"short page":  `[{"store":1},{"item_count":2,"items":3,"page_count":4},5,[],1]`,
		"no list":     `[{"store":1},{"foo":2},"bar"]`,
		"no payload":  ``,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			f := &selectelFake{pages: map[string]string{selectelListURL: selectelNuxtPage(payload)}}
			if _, err := NewSelectel(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// Listing new postings and reading none of their pages is a broken crawl, not a quiet one.
func TestSelectelFailsWhenNoNewPostingIsRead(t *testing.T) {
	f := &selectelFake{pages: map[string]string{selectelListURL: selectelFixture(t, "selectel_list.html")}}
	if _, err := NewSelectel(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("want an error when no vacancy page could be read")
	}
}
