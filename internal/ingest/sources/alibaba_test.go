package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
)

// alibabaFake serves two trimmed real search pages (2026-10-15) of the technology category,
// five postings in all, by the page the request asks for.
type alibabaFake struct {
	mutate  func(page int, body map[string]any)
	fail    int // a request for this page fails; 0 never
	asked   []alibabaQuery
	headers map[string]string
}

func (f *alibabaFake) PostJSONWithHeaders(_ context.Context, u string, headers map[string]string, body, v any) error {
	if u != alibabaSearchURL {
		return errors.New("unexpected POST " + u)
	}
	q := body.(alibabaQuery)
	f.asked = append(f.asked, q)
	f.headers = headers
	if q.PageIndex == f.fail {
		return errors.New("HTTP 502")
	}
	var resp map[string]any
	raw, err := os.ReadFile(fmt.Sprintf("testdata/alibaba/search_page%d.json", q.PageIndex))
	if err != nil {
		resp = map[string]any{"success": true, "content": map[string]any{"datas": []any{}, "totalCount": 5}}
	} else if err := json.Unmarshal(raw, &resp); err != nil {
		return err
	}
	if f.mutate != nil {
		f.mutate(q.PageIndex, resp)
	}
	out, _ := json.Marshal(resp)
	return json.Unmarshal(out, v)
}

func TestAlibabaFetchMapsTheCatalogue(t *testing.T) {
	f := &alibabaFake{}
	jobs, err := NewAlibaba(f).Fetch(context.Background(), CompanyEntry{Company: "Alibaba Group"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 || len(f.asked) != 2 {
		t.Fatalf("got %d jobs over %d requests, want 5 over 2", len(jobs), len(f.asked))
	}
	if q := f.asked[0]; q.SubCategories != alibabaTechSubcategories || q.PageSize != 500 || q.Channel != "group_official_site" {
		t.Errorf("query: %+v", q)
	}
	if tok := f.headers["X-XSRF-TOKEN"]; tok == "" || f.headers["Cookie"] != "XSRF-TOKEN="+tok {
		t.Errorf("csrf pair: %v", f.headers)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
		if !j.IsTechHint || j.Company != "Alibaba Group" || j.PostedAt == nil || j.Description == "" {
			t.Errorf("%s: %+v", j.ExternalID, j)
		}
	}
	algo := byID["100048720006"]
	if algo.Title != "阿里国际站-Agentic AI 算法工程师-深圳" ||
		algo.URL != "https://talent-holding.alibaba.com/off-campus/position-detail?lang=zh&positionId=100048720006" ||
		algo.Location != "Shenzhen" || len(algo.Countries) != 1 || algo.Countries[0] != "cn" ||
		algo.ExperienceYearsMin == nil || *algo.ExperienceYearsMin != 1 {
		t.Errorf("algorithm posting: %+v", algo)
	}
	if seoul := byID["100015743007"]; seoul.Location != "Seoul" || len(seoul.Countries) != 1 || seoul.Countries[0] != "kr" {
		t.Errorf("seoul: %q %v", seoul.Location, seoul.Countries)
	}
	// 河源 is not in the city map: it keeps its spelling and states no country of its own.
	if multi := byID["100012663003"]; multi.Location != "Ulanqab; Chengdu; Beijing; 河源; Zhangjiakou; Nantong; Shenzhen; Hangzhou; Shanghai" ||
		len(multi.Countries) != 1 || multi.Countries[0] != "cn" {
		t.Errorf("multi-city: %q %v", multi.Location, multi.Countries)
	}
}

func TestAlibabaFailsRatherThanTruncate(t *testing.T) {
	cases := map[string]*alibabaFake{
		"a later page failing": {fail: 2},
		"more postings than the pages hold": {mutate: func(_ int, b map[string]any) {
			b["content"].(map[string]any)["totalCount"] = 6
		}},
		"a non-technical row": {mutate: func(page int, b map[string]any) {
			if page == 2 {
				rows := b["content"].(map[string]any)["datas"].([]any)
				rows[0].(map[string]any)["categories"] = []string{"运营-产品运营"}
			}
		}},
		"an unsuccessful answer": {mutate: func(_ int, b map[string]any) { b["success"] = false }},
		"nothing at all": {mutate: func(_ int, b map[string]any) {
			b["content"] = map[string]any{"datas": []any{}, "totalCount": 0}
		}},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := NewAlibaba(f).Fetch(context.Background(), CompanyEntry{Company: "Alibaba Group"}); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestCNPlaces(t *testing.T) {
	loc, countries := cnPlaces([]string{"兰州市", "中国香港", "东京", "未知县", "北京"})
	if loc != "Lanzhou; Hong Kong; Tokyo; 未知县; Beijing" || fmt.Sprint(countries) != "[cn hk jp]" {
		t.Errorf("got %q %v", loc, countries)
	}
}

func TestAlibabaIsBoardlessFullBoardListing(t *testing.T) {
	s := NewAlibaba(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("alibaba must be boardless")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("alibaba must be fullBoardListing")
	}
}
