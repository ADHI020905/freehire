package sources

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The fixtures are trimmed from live itviec.com responses fetched from the crawl host on
// 2026-10-15: the listing cards' opening tags and the two "company spotlight" links verbatim,
// and each posting page's ld+json blocks, heading and header row verbatim.

func itviecFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func itviecParse(t *testing.T, name string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(itviecFixture(t, name)))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// itviecFake serves fixed pages by URL; an unknown URL is a 404.
type itviecFake struct {
	pages map[string]string
	errs  map[string]error
	got   []string
}

func (f *itviecFake) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.got = append(f.got, url)
	if err, ok := f.errs[url]; ok {
		return nil, err
	}
	body, ok := f.pages[url]
	if !ok {
		return nil, &StatusError{Method: "GET", Code: 404, URL: url}
	}
	return html.Parse(strings.NewReader(body))
}

const (
	itviecFirstCard = "https://itviec.com/it-jobs/tmo-devops-engineer-senior-manager-devsecops-aws-ngan-hang-a-chau-acb-0103"
	itviecFirstID   = "747e3af5-f7ef-4bd9-87f2-69c6499f4914"
)

func TestITviecCardsReadKeyAndSlugAndIgnoreSpotlightLinks(t *testing.T) {
	cards := parseITviecCards(itviecParse(t, "itviec_list_page1.html"))
	if len(cards) != 4 {
		t.Fatalf("cards = %d, want 4 (three cards and the short-key one): %+v", len(cards), cards)
	}
	if cards[0].ID != itviecFirstID || cards[0].URL != itviecFirstCard {
		t.Errorf("first card = %+v", cards[0])
	}
	// One live card carries a key that is not a UUID; it is still the posting's key.
	if cards[3].ID != "e9661b73-ffd" {
		t.Errorf("short-key card = %+v", cards[3])
	}
	for _, c := range cards {
		if strings.Contains(c.URL, "netcompany") {
			t.Errorf("a company-spotlight link was read as a card: %s", c.URL)
		}
	}
	// The page past the end still links the spotlight postings, and has no card.
	if got := parseITviecCards(itviecParse(t, "itviec_list_past_end.html")); len(got) != 0 {
		t.Errorf("past-end page yielded cards: %+v", got)
	}
}

func TestITviecPostingWithStructuredSalary(t *testing.T) {
	l := itviecListing{ID: "ecfb30b1-c7dd-4a9f-b90e-8b4c40f4f7b8", URL: "https://itviec.com/it-jobs/ai-software-engineer-python-go-c-c-ai-agents-mb-bank-5051"}
	job, ok := parseITviecPosting(itviecParse(t, "itviec_detail_salary.html"), l)
	if !ok {
		t.Fatal("posting not parsed")
	}
	if job.ExternalID != l.ID || job.URL != l.URL {
		t.Errorf("identity = %q %q", job.ExternalID, job.URL)
	}
	if job.Title != "AI Software Engineer (Python/Go/C/C++ & AI Agents)" {
		t.Errorf("title = %q (the heading keeps punctuation the JobPosting strips)", job.Title)
	}
	if job.Company != "MB Bank" {
		t.Errorf("company = %q", job.Company)
	}
	if job.Location != "Hà Nội, Vietnam" || !slices.Equal(job.Countries, []string{"vn"}) {
		t.Errorf("location = %q countries = %v", job.Location, job.Countries)
	}
	if job.WorkMode != "onsite" || job.Remote {
		t.Errorf("work mode = %q remote = %v", job.WorkMode, job.Remote)
	}
	if job.SalaryMin == nil || *job.SalaryMin != 800 || job.SalaryMax == nil || *job.SalaryMax != 2000 ||
		job.SalaryCurrency != "USD" || job.SalaryPeriod != "month" {
		t.Errorf("salary = %v-%v %s/%s", job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
	}
	if job.PostedAt == nil || job.PostedAt.Format("2006-01-02") != "2026-09-24" {
		t.Errorf("posted = %v", job.PostedAt)
	}
	if !job.IsTechHint {
		t.Error("IsTechHint not set on an IT-only board")
	}
	if !slices.Equal(job.Skills, []string{"ai", "cpp", "docker", "fastapi", "go", "python"}) {
		t.Errorf("skills = %v", job.Skills)
	}
	for _, want := range []string{"<h3>Top 3 Reasons To Join Us</h3>", "<p>Mức lương cạnh tranh, hấp dẫn</p>", "<h3>The Job</h3>", "2026TD450888"} {
		if !strings.Contains(job.Description, want) {
			t.Errorf("description lacks %q", want)
		}
	}
}

func TestITviecPostingWithHiddenSalaryYieldsNone(t *testing.T) {
	l := itviecListing{ID: "5ec2a660-7001-447f-98ca-964b420e5c60", URL: "https://itviec.com/it-jobs/bridge-system-engineer-japanese-n2-itviec-recruitment-consulting-4252"}
	job, ok := parseITviecPosting(itviecParse(t, "itviec_detail_hidden_salary.html"), l)
	if !ok {
		t.Fatal("posting not parsed")
	}
	if job.SalaryMin != nil || job.SalaryMax != nil || job.SalaryCurrency != "" || job.SalaryPeriod != "" {
		t.Errorf("a hidden salary (\"You'll love it\") yielded %v-%v %s/%s", job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod)
	}
	if job.Title != "Bridge System Engineer (Japanese N2)" || job.Company != "ITviec Recruitment Consulting" {
		t.Errorf("title/company = %q / %q", job.Title, job.Company)
	}
	if job.Location != "Hồ Chí Minh, Vietnam" {
		t.Errorf("location = %q", job.Location)
	}
	if !strings.Contains(job.Description, "<h3>Your Skills and Experience</h3>") {
		t.Error("description lost a section heading")
	}
}

func TestITviecPageWithoutJobPostingIsNotAPosting(t *testing.T) {
	root, _ := html.Parse(strings.NewReader("<html><body><h1>658 IT jobs in Vietnam</h1></body></html>"))
	if _, ok := parseITviecPosting(root, itviecListing{ID: "k", URL: "u"}); ok {
		t.Error("a page without a JobPosting was mapped")
	}
}

func itviecCatalogue(t *testing.T) *itviecFake {
	return &itviecFake{pages: map[string]string{
		itviecListURL:             itviecFixture(t, "itviec_list_page1.html"),
		itviecListURL + "?page=2": itviecFixture(t, "itviec_list_past_end.html"),
		itviecFirstCard:           itviecFixture(t, "itviec_detail_salary.html"),
	}}
}

func TestITviecFetchNewWalksToTheFirstPageWithoutCards(t *testing.T) {
	f := itviecCatalogue(t)
	seen := func(id string) bool { return id != itviecFirstID }
	jobs, err := NewITviec(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("jobs = %d, want 4", len(jobs))
	}
	var fresh, refresh int
	for _, j := range jobs {
		if j.SeenRefresh {
			refresh++
			continue
		}
		fresh++
		if j.ExternalID != itviecFirstID || j.Company != "MB Bank" {
			t.Errorf("hydrated job = %+v", j)
		}
	}
	if fresh != 1 || refresh != 3 {
		t.Errorf("fresh = %d refresh = %d", fresh, refresh)
	}
	if slices.Contains(f.got, itviecListURL+"?page=3") {
		t.Error("walked past the first page without cards")
	}
	// Only the unseen posting's page is bought.
	details := 0
	for _, u := range f.got {
		if strings.HasPrefix(u, itviecListURL+"/") {
			details++
		}
	}
	if details != 1 {
		t.Errorf("detail requests = %d, want 1", details)
	}
}

func TestITviecFirstPageFailingFailsTheCrawl(t *testing.T) {
	f := &itviecFake{errs: map[string]error{itviecListURL: &StatusError{Method: "GET", Code: 403, URL: itviecListURL}}}
	if _, err := NewITviec(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a refused first page did not fail the crawl")
	}
}

func TestITviecLaterPageFailingKeepsWhatWasListed(t *testing.T) {
	f := itviecCatalogue(t)
	f.errs = map[string]error{itviecListURL + "?page=2": &StatusError{Method: "GET", Code: 500, URL: "p2"}}
	jobs, err := NewITviec(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id != itviecFirstID })
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Errorf("jobs = %d, want the 4 listed before the failure", len(jobs))
	}
}

func TestITviecFirstPageWithoutCardsIsAFailure(t *testing.T) {
	f := &itviecFake{pages: map[string]string{itviecListURL: itviecFixture(t, "itviec_list_past_end.html")}}
	if _, err := NewITviec(f).Fetch(context.Background(), CompanyEntry{}); err == nil {
		t.Fatal("a first page without cards read as an empty market")
	}
}

func TestITviecReadingNoNewPostingIsAFailure(t *testing.T) {
	f := itviecCatalogue(t)
	delete(f.pages, itviecFirstCard) // every new posting's page 404s
	_, err := NewITviec(f).(HydratingSource).FetchNew(context.Background(), CompanyEntry{}, func(id string) bool { return id != itviecFirstID })
	if err == nil {
		t.Fatal("a crawl that read none of its new postings succeeded")
	}
}

func TestITviecIsABoardlessAggregator(t *testing.T) {
	src := NewITviec(nil)
	if _, ok := src.(boardless); !ok {
		t.Error("itviec is not boardless")
	}
	if _, ok := src.(aggregator); !ok {
		t.Error("itviec is not an aggregator")
	}
	if _, ok := src.(fullBoardListing); ok {
		t.Error("itviec claims fullBoardListing without meeting the bar")
	}
	if _, ok := src.(fullCatalog); ok {
		t.Error("itviec claims fullCatalog without meeting the bar")
	}
	if _, ok := Taxonomy()["itviec"]; !ok {
		t.Error("itviec is not registered")
	}
}
