package sources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// meituanFake serves five trimmed real technology postings (2026-10-15) — three experienced,
// one campus graduate, one intern — filtered by the hire type and family the request asks for.
type meituanFake struct {
	t            *testing.T
	ignoreFilter bool
	countDelta   int
	failPage     int
	asked        []meituanQuery
}

func (f *meituanFake) PostJSON(_ context.Context, u string, body, v any) error {
	if u != meituanSearchURL {
		return errors.New("unexpected POST " + u)
	}
	q := body.(meituanQuery)
	f.asked = append(f.asked, q)
	if q.Page.PageNo == f.failPage {
		return errors.New("HTTP 502")
	}
	raw, err := os.ReadFile("testdata/meituan/technology.json")
	if err != nil {
		return err
	}
	var file struct {
		Data struct {
			List []map[string]any `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		f.t.Fatal(err)
	}
	var matched []map[string]any
	for _, r := range file.Data.List {
		if f.ignoreFilter || (r["jobType"] == q.JobType[0].Code && q.JfJgList[0].Code == meituanTechFamily) {
			matched = append(matched, r)
		}
	}
	page := []map[string]any{}
	if start := (q.Page.PageNo - 1) * q.Page.PageSize; start < len(matched) {
		page = matched[start:min(start+q.Page.PageSize, len(matched))]
	}
	out, _ := json.Marshal(map[string]any{"status": 1, "data": map[string]any{
		"list": page, "page": map[string]any{"totalCount": len(matched) + f.countDelta}}})
	return json.Unmarshal(out, v)
}

func TestMeituanFetchMapsTheCatalogue(t *testing.T) {
	f := &meituanFake{t: t}
	jobs, err := NewMeituan(f).Fetch(context.Background(), CompanyEntry{Company: "Meituan"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 5 {
		t.Fatalf("got %d jobs, want 5", len(jobs))
	}
	if q := f.asked[0]; q.Page.PageSize != 500 || q.JfJgList[0].Code != "11001" || q.JobType[0].Code != "3" {
		t.Errorf("query: %+v", q)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
		if !j.IsTechHint || j.Company != "Meituan" || j.Description == "" || j.PostedAt == nil {
			t.Errorf("%s: %+v", j.ExternalID, j)
		}
	}
	java := byID["4839084480"]
	if java.Title != "数据通道-高级Java开发工程师" || java.URL != "https://zhaopin.meituan.com/web/position/detail?jobUnionId=4839084480" ||
		java.Location != "Beijing; Shanghai" || len(java.Countries) != 1 || java.Countries[0] != "cn" || java.Seniority != "" {
		t.Errorf("experienced: %+v", java)
	}
	if grad := byID["4697320043"]; grad.Seniority != "junior" || grad.EmploymentType != "" {
		t.Errorf("campus graduate: %q %q", grad.Seniority, grad.EmploymentType)
	}
	if intern := byID["3647684690"]; intern.Seniority != "intern" || intern.EmploymentType != "internship" {
		t.Errorf("intern: %q %q", intern.Seniority, intern.EmploymentType)
	}
	if us := byID["4786959491"]; us.Location != "San Francisco; Seattle" || len(us.Countries) != 1 || us.Countries[0] != "us" {
		t.Errorf("abroad: %q %v", us.Location, us.Countries)
	}
}

func TestMeituanFailsRatherThanTruncate(t *testing.T) {
	cases := map[string]*meituanFake{
		"the filter ignored":      {ignoreFilter: true},
		"more postings than read": {countDelta: 1},
		"a page failing":          {failPage: 1},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			f.t = t
			if jobs, err := NewMeituan(f).Fetch(context.Background(), CompanyEntry{Company: "Meituan"}); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestMeituanIsBoardlessFullBoardListing(t *testing.T) {
	s := NewMeituan(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("meituan must be boardless")
	}
	if _, ok := s.(fullBoardListing); !ok {
		t.Error("meituan must be fullBoardListing")
	}
}
