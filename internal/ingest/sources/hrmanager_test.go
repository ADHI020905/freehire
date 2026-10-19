package sources

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// hrManagerFake serves the real energinet listing captured 2026-10-19 (32 postings, one
// request). Items are trimmed of the empty custom fields and the recruiters' personal details;
// everything else is as served. Any other customer gets the platform's real answer to an
// unknown alias.
type hrManagerFake struct {
	mutate func(body map[string]any)
	fail   bool
	asked  []string
}

func (f *hrManagerFake) GetJSON(_ context.Context, raw string, v any) error {
	f.asked = append(f.asked, raw)
	if f.fail {
		return errors.New("HTTP 503")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	file := "testdata/hrmanager/energinet.json"
	if !strings.HasPrefix(u.Path, "/jobportal.svc/energinet/") {
		file = "testdata/hrmanager/unknown_customer.json"
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var body map[string]any
	if err := json.Unmarshal(b, &body); err != nil {
		return err
	}
	if f.mutate != nil {
		f.mutate(body)
	}
	out, _ := json.Marshal(body)
	return json.Unmarshal(out, v)
}

func hrManagerFetch(f *hrManagerFake, board string) ([]Job, error) {
	return NewHRManager(f).Fetch(context.Background(), CompanyEntry{Company: "Energinet", Board: board})
}

// hrManagerDrop removes the first n items, as a listing that served fewer rows than it counts.
func hrManagerDrop(n int) func(map[string]any) {
	return func(body map[string]any) {
		items := body["Items"].([]any)[n:]
		body["Items"] = items
		body["PositionCountList"] = len(items)
	}
}

func TestHRManagerReadsTheWholeListingInOneRequest(t *testing.T) {
	f := &hrManagerFake{}
	jobs, err := hrManagerFetch(f, "energinet")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 32 {
		t.Fatalf("got %d jobs, want all 32", len(jobs))
	}
	if len(f.asked) != 1 {
		t.Fatalf("%d requests, want one", len(f.asked))
	}
	u, _ := url.Parse(f.asked[0])
	if got := u.Scheme + "://" + u.Host + u.Path; got != "https://api.hr-manager.net/jobportal.svc/energinet/positionlist/json/" {
		t.Errorf("listing path = %s", got)
	}
	if q := u.Query(); q.Get("incads") != "1" || q.Get("take") != "2000" || q.Has("skip") {
		t.Errorf("listing query %v must ask for the advertisements and more rows than the board holds", q)
	}
}

func TestHRManagerMapsAPosting(t *testing.T) {
	jobs, err := hrManagerFetch(&hrManagerFake{}, "energinet")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}
	got, ok := byID["188775"]
	if !ok {
		t.Fatal("posting 188775 missing")
	}
	if got.Title != "Platform Engineer til Data Science Platform" || got.Company != "Energinet" {
		t.Errorf("title/company = %q / %q", got.Title, got.Company)
	}
	if got.URL != "https://candidate.hr-manager.net/ApplicationInit.aspx?cid=316&ProjectId=188775&DepartmentId=21682&MediaId=5" {
		t.Errorf("url = %s", got.URL)
	}
	if got.Location != "Fredericia" {
		t.Errorf("location = %q", got.Location)
	}
	if !strings.Contains(got.Description, "<p>") || len(got.Description) < 500 {
		t.Errorf("description is not the advertisement HTML: %.200q", got.Description)
	}
	want := time.UnixMilli(1792065786000).UTC()
	if got.PostedAt == nil || !got.PostedAt.Equal(want) {
		t.Errorf("posted_at = %v, want %v", got.PostedAt, want)
	}
	// 188958 states no WorkPlace text; its location picklist says Fredericia.
	if l := byID["188958"].Location; l != "Fredericia" {
		t.Errorf("188958 location = %q, want the picklist's Fredericia", l)
	}
	// 188952's picklist names two cities; its WorkPlace text names both.
	if l := byID["188952"].Location; l != "Fredericia / Ballerup" {
		t.Errorf("188952 location = %q", l)
	}
}

func TestHRManagerSkipsTheOpenApplication(t *testing.T) {
	f := &hrManagerFake{mutate: func(body map[string]any) {
		body["Items"].([]any)[0].(map[string]any)["ProjectType"] = "OpenApplication"
	}}
	jobs, err := hrManagerFetch(f, "energinet")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 31 {
		t.Fatalf("got %d jobs, want 31 without the standing unsolicited-application entry", len(jobs))
	}
}

func TestHRManagerToleratesAHiddenPosting(t *testing.T) {
	// The shape statensrekrutteringsloesning_tr serves: counted 414, served 413.
	jobs, err := hrManagerFetch(&hrManagerFake{mutate: hrManagerDrop(1)}, "energinet")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 31 {
		t.Fatalf("got %d jobs, want 31", len(jobs))
	}
}

func TestHRManagerNeverReportsAPartialListing(t *testing.T) {
	cases := map[string]*hrManagerFake{
		"request fails":        {fail: true},
		"rows short of count":  {mutate: hrManagerDrop(hrManagerMaxHidden + 1)},
		"rows disagree":        {mutate: func(b map[string]any) { b["PositionCountList"] = 31 }},
		"over one request":     {mutate: func(b map[string]any) { b["PositionCountSearch"] = hrManagerTake + 1 }},
		"rows over count":      {mutate: func(b map[string]any) { b["PositionCountSearch"] = 31 }},
		"count missing":        {mutate: func(b map[string]any) { delete(b, "PositionCountSearch") }},
		"platform refuses":     {mutate: func(b map[string]any) { b["TransactionStatus"] = map[string]any{"StatusCode": 1} }},
		"another customer":     {mutate: func(b map[string]any) { b["CustomerAlias"] = "kimdemo" }},
		"posting listed twice": {mutate: func(b map[string]any) { b["Items"].([]any)[1].(map[string]any)["Id"] = 188952 }},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if jobs, err := hrManagerFetch(f, "energinet"); err == nil {
				t.Fatalf("got %d jobs and no error", len(jobs))
			}
		})
	}
}

func TestHRManagerUnknownCustomerFails(t *testing.T) {
	_, err := hrManagerFetch(&hrManagerFake{}, "nosuchcustomerxyz")
	if err == nil || !strings.Contains(err.Error(), "connectionString") {
		t.Fatalf("err = %v, want the platform's own refusal named", err)
	}
}

func TestHRManagerRejectsAMalformedBoard(t *testing.T) {
	f := &hrManagerFake{}
	if _, err := hrManagerFetch(f, "energinet/../x"); err == nil || len(f.asked) != 0 {
		t.Fatalf("err = %v after %d requests, want a refusal before any", err, len(f.asked))
	}
}

func TestHRManagerDate(t *testing.T) {
	if got := hrManagerDate("/Date(1791377991000+0200)/"); got == nil || got.UnixMilli() != 1791377991000 {
		t.Errorf("offset form = %v", got)
	}
	for _, in := range []string{"/Date(-62135596800000)/", "", "2026-10-19"} {
		if got := hrManagerDate(in); got != nil {
			t.Errorf("hrManagerDate(%q) = %v, want nil", in, got)
		}
	}
}

func TestHRManagerIsFullBoardListing(t *testing.T) {
	if _, ok := NewHRManager(nil).(fullBoardListing); !ok {
		t.Fatal("hrmanager must carry fullBoardListing")
	}
}
