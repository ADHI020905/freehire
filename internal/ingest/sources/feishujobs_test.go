package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

// feishuJobsFake serves trimmed real responses (2026-10-15) of one tenant: the filter
// configuration, and a search that applies the request's category and hire-type filters to the
// tenant's real R&D rows and pages them. ByteDance has six (four experienced, one campus
// graduate, one campus intern); MiniMax four, its R&D one level deeper in the tree.
type feishuJobsFake struct {
	t            *testing.T
	host, tenant string
	rows         []map[string]any
	ignoreFilter bool // answer the unfiltered rows, as a search that dropped the filter would
	count        func(n int) int
	failOffset   int // a POST at this offset fails; -1 never
	asked        []feishuJobsQuery
	gotURLs      []string
}

func newFeishuJobsFake(t *testing.T) *feishuJobsFake {
	return newFeishuJobsTenantFake(t, "jobs.bytedance.com", "bytedance")
}

func newFeishuJobsTenantFake(t *testing.T, host, tenant string) *feishuJobsFake {
	t.Helper()
	raw, err := os.ReadFile("testdata/feishujobs/" + tenant + "_rd.json")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Data struct {
			Posts []map[string]any `json:"job_post_list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return &feishuJobsFake{t: t, host: host, tenant: tenant, rows: body.Data.Posts, failOffset: -1}
}

func (f *feishuJobsFake) GetJSON(_ context.Context, u string, v any) error {
	f.gotURLs = append(f.gotURLs, u)
	if u != "https://"+f.host+"/api/v1/config/job/filters/2" {
		return errors.New("unexpected GET " + u)
	}
	raw, err := os.ReadFile("testdata/feishujobs/" + f.tenant + "_filters.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// feishuJobsChain is the ids of a decoded category or recruit-type node and its ancestors.
func feishuJobsChain(node any) []string {
	var ids []string
	for m, ok := node.(map[string]any); ok; m, ok = m["parent"].(map[string]any) {
		id, _ := m["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func (f *feishuJobsFake) PostJSON(_ context.Context, u string, body, v any) error {
	f.gotURLs = append(f.gotURLs, u)
	if u != "https://"+f.host+"/api/v1/search/job/posts" {
		return errors.New("unexpected POST " + u)
	}
	q := body.(feishuJobsQuery)
	f.asked = append(f.asked, q)
	if q.Offset == f.failOffset {
		return errors.New("HTTP 502")
	}
	var matched []map[string]any
	for _, r := range f.rows {
		if f.ignoreFilter || (slices.Contains(feishuJobsChain(r["recruit_type"]), q.Recruitment[0]) &&
			slices.ContainsFunc(feishuJobsChain(r["job_category"]), func(id string) bool {
				return slices.Contains(q.Categories, id)
			})) {
			matched = append(matched, r)
		}
	}
	count := len(matched)
	if f.count != nil {
		count = f.count(count)
	}
	page := []map[string]any{}
	if q.Offset < len(matched) {
		page = matched[q.Offset:min(q.Offset+q.Limit, len(matched))]
	}
	out, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"job_post_list": page, "count": count}})
	return json.Unmarshal(out, v)
}

func feishuJobsFetch(t *testing.T, f *feishuJobsFake, board string) ([]Job, error) {
	t.Helper()
	s := NewFeishuJobs(f).(feishuJobs)
	s.pageSize = 2 // page the six fixture rows
	return s.Fetch(context.Background(), CompanyEntry{Board: board, Company: "ByteDance"})
}

func TestFeishuJobsFetchMapsTheBoard(t *testing.T) {
	f := newFeishuJobsFake(t)
	jobs, err := feishuJobsFetch(t, f, "jobs.bytedance.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 {
		t.Fatalf("got %d jobs, want 6", len(jobs))
	}
	for _, q := range f.asked {
		if len(q.Categories) != 1 || q.Categories[0] != "6704215862603155720" {
			t.Errorf("not scoped to R&D: %+v", q)
		}
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
		if !j.IsTechHint || j.Company != "ByteDance" || j.Description == "" || j.PostedAt == nil {
			t.Errorf("%s: tech=%v company=%q body=%d posted=%v", j.ExternalID, j.IsTechHint, j.Company, len(j.Description), j.PostedAt)
		}
	}

	backend := byID["7680112755274828037"]
	if backend.Title != "平台Agent研发工程师 - 风控" || backend.Category != "backend" ||
		backend.URL != "https://jobs.bytedance.com/experienced/position/7680112755274828037/detail" {
		t.Errorf("backend: %q %q %s", backend.Title, backend.Category, backend.URL)
	}
	if backend.Location != "Shenzhen" || len(backend.Countries) != 1 || backend.Countries[0] != "cn" {
		t.Errorf("backend place: %q %v", backend.Location, backend.Countries)
	}
	if backend.Seniority != "" || backend.EmploymentType != "" {
		t.Errorf("experienced hire got %q/%q", backend.Seniority, backend.EmploymentType)
	}

	hk := byID["7652289717715896629"]
	if hk.Location != "Hong Kong" || len(hk.Countries) != 1 || hk.Countries[0] != "hk" || hk.Category != "" {
		t.Errorf("hong kong: %q %v %q", hk.Location, hk.Countries, hk.Category)
	}
	if multi := byID["7686792988366719285"]; multi.Location != "Beijing; Hangzhou" {
		t.Errorf("multi-city: %q", multi.Location)
	}

	grad := byID["7667104237897287989"]
	if grad.Seniority != "junior" || grad.EmploymentType != "" ||
		grad.URL != "https://jobs.bytedance.com/campus/position/7667104237897287989/detail" {
		t.Errorf("campus graduate: %q %q %s", grad.Seniority, grad.EmploymentType, grad.URL)
	}
	intern := byID["7679892629651310901"]
	if intern.Seniority != "intern" || intern.EmploymentType != "internship" ||
		len(intern.Countries) != 1 || intern.Countries[0] != "us" {
		t.Errorf("campus intern: %q %q %v", intern.Seniority, intern.EmploymentType, intern.Countries)
	}
	if !strings.Contains(intern.Description, "<p>") {
		t.Errorf("body not rebuilt into paragraphs: %.80s", intern.Description)
	}
}

func TestFeishuJobsFetchACustomerTenant(t *testing.T) {
	f := newFeishuJobsTenantFake(t, "vrfi1sk8a0.jobs.feishu.cn", "minimax")
	jobs, err := feishuJobsFetch(t, f, "vrfi1sk8a0.jobs.feishu.cn")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4", len(jobs))
	}
	// R&D is a second-level node under an industry root on this tenant.
	if q := f.asked[0]; len(q.Categories) != 1 || q.Categories[0] != "6791702736615426317" {
		t.Errorf("not scoped to R&D: %+v", q)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	be := byID["7641124619067410697"]
	if be.Category != "backend" || be.URL != "https://vrfi1sk8a0.jobs.feishu.cn/index/position/7641124619067410697/detail" ||
		be.Location != "Beijing; Shanghai" || len(be.Countries) != 1 || be.Countries[0] != "cn" {
		t.Errorf("backend: %q %s %q %v", be.Category, be.URL, be.Location, be.Countries)
	}
	if rd := byID["7681517111994403126"]; rd.Category != "" || !rd.IsTechHint {
		t.Errorf("R&D itself names no discipline: %q", rd.Category)
	}
	if sf := byID["7687134817943472447"]; len(sf.Countries) != 1 || sf.Countries[0] != "us" {
		t.Errorf("san francisco: %v", sf.Countries)
	}
}

func TestFeishuJobsRejectsForeignHosts(t *testing.T) {
	for _, board := range []string{"evil.com", "jobs.feishu.cn.evil.com", "a.b.jobs.feishu.cn", ""} {
		if _, err := feishuJobsFetch(t, newFeishuJobsFake(t), board); err == nil {
			t.Errorf("board %q accepted", board)
		}
	}
}

func TestFeishuJobsFailsRatherThanTruncate(t *testing.T) {
	cases := map[string]func(f *feishuJobsFake){
		"a later page failing":    func(f *feishuJobsFake) { f.failOffset = 2 },
		"the filter ignored":      func(f *feishuJobsFake) { f.ignoreFilter = true },
		"a count at the window":   func(f *feishuJobsFake) { f.count = func(int) int { return feishuJobsWindow } },
		"more postings than read": func(f *feishuJobsFake) { f.count = func(n int) int { return n + 1 } },
		"nothing at all":          func(f *feishuJobsFake) { f.rows = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFeishuJobsFake(t)
			mutate(f)
			if jobs, err := feishuJobsFetch(t, f, "jobs.bytedance.com"); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestFeishuJobsIsFullBoardListing(t *testing.T) {
	if _, ok := Source(feishuJobs{}).(fullBoardListing); !ok {
		t.Fatal("feishujobs must be fullBoardListing")
	}
	if _, ok := Source(feishuJobs{}).(aggregator); ok {
		t.Fatal("a tenant is one employer, not an aggregator")
	}
}
