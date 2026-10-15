package sources

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

func friendworkFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestFriendWorkProvider(t *testing.T) {
	src := NewFriendWork(nil)
	if got := src.Provider(); got != "friendwork" {
		t.Errorf("Provider() = %q, want friendwork", got)
	}
	if _, ok := src.(fullBoardListing); !ok {
		t.Error("friendwork must be fullBoardListing: one request returns the whole list")
	}
	if _, ok := src.(boardless); ok {
		t.Error("friendwork is board-keyed (board = career-portal token)")
	}
}

func TestFriendWorkFetchMapsEveryField(t *testing.T) {
	fake := (&routedHTTP{}).route("token=c656f1c111eb44075d21d77f57216c72", friendworkFixture(t, "friendwork_itgeo.json"))

	jobs, err := NewFriendWork(fake).Fetch(context.Background(), CompanyEntry{
		Company: "IT Geo", Provider: "friendwork", Board: "c656f1c111eb44075d21d77f57216c72",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len(jobs) = %d, want 3", len(jobs))
	}
	byID := map[string]Job{}
	for _, j := range jobs {
		byID[j.ExternalID] = j
	}

	j, ok := byID["6080"]
	if !ok {
		t.Fatalf("posting 6080 missing; got ids %v", keysOf(byID))
	}
	if j.Title != "Middle backend developer (.NET)" {
		t.Errorf("Title = %q", j.Title)
	}
	if j.URL != "https://jobs.friend.work/c656f1c111eb44075d21d77f57216c72/6080" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Company != "IT Geo" {
		t.Errorf("Company = %q", j.Company)
	}
	if j.Location != "Москва" {
		t.Errorf("Location = %q", j.Location)
	}
	if j.WorkMode != "onsite" || j.Remote {
		t.Errorf("WorkMode = %q Remote = %v, want onsite (workplaceType Office)", j.WorkMode, j.Remote)
	}
	if j.EmploymentType != "full_time" {
		t.Errorf("EmploymentType = %q", j.EmploymentType)
	}
	if j.SalaryMin == nil || *j.SalaryMin != 150000 || j.SalaryMax == nil || *j.SalaryMax != 170000 {
		t.Errorf("salary = %v..%v, want 150000..170000", deref(j.SalaryMin), deref(j.SalaryMax))
	}
	if j.SalaryCurrency != "RUB" || j.SalaryPeriod != "month" {
		t.Errorf("salary currency/period = %q/%q, want RUB/month", j.SalaryCurrency, j.SalaryPeriod)
	}
	if j.ExperienceYearsMin == nil || *j.ExperienceYearsMin != 2 {
		t.Errorf("ExperienceYearsMin = %v, want 2", deref(j.ExperienceYearsMin))
	}
	want := time.Date(2019, 10, 23, 14, 39, 25, 150000000, time.UTC)
	if j.PostedAt == nil || !j.PostedAt.Equal(want) {
		t.Errorf("PostedAt = %v, want %v", j.PostedAt, want)
	}
	if j.Description == "" {
		t.Error("Description empty; the listing carries the body inline")
	}

	if pt := byID["5514"]; pt.EmploymentType != "part_time" {
		t.Errorf("Halftime posting EmploymentType = %q, want part_time", pt.EmploymentType)
	}
	if to := byID["6077"]; to.SalaryMin != nil || to.SalaryMax == nil || *to.SalaryMax != 200000 {
		t.Errorf("an upper-only salary = %v..%v, want nil..200000", deref(to.SalaryMin), deref(to.SalaryMax))
	}
}

func TestFriendWorkRemoteAndUnsetEnums(t *testing.T) {
	fake := (&routedHTTP{}).route("token=wowvendor", friendworkFixture(t, "friendwork_wowvendor.json"))

	jobs, err := NewFriendWork(fake).Fetch(context.Background(), CompanyEntry{Company: "WowVendor", Board: "wowvendor"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("len(jobs) = %d, want 5", len(jobs))
	}
	var remote, unsetEmployment int
	for _, j := range jobs {
		if j.WorkMode == "remote" && j.Remote {
			remote++
		}
		if j.EmploymentType == "" {
			unsetEmployment++
		}
		if j.SalaryMin == nil && j.SalaryMax == nil && j.SalaryCurrency != "" {
			t.Errorf("%s: a currency without an amount must not be stated", j.ExternalID)
		}
	}
	if remote != 4 {
		t.Errorf("remote postings = %d, want 4", remote)
	}
	if unsetEmployment != 1 {
		t.Errorf("postings with the \"-1\" employment type = %d, want 1 (left unset)", unsetEmployment)
	}
}

func TestFriendWorkEnumMappings(t *testing.T) {
	for in, want := range map[string]string{"Office": "onsite", "Remote": "remote", "Hybrid": "hybrid", "-1": ""} {
		if got := workplaceTypeMode(in); got != want {
			t.Errorf("workplaceTypeMode(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Fulltime": "full_time", "Halftime": "part_time", "Internship": "internship", "Temporary": "", "-1": ""} {
		if got := friendworkEmploymentType[in]; got != want {
			t.Errorf("employment type %q = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[int]string{0: "RUB", 1: "USD", 2: "EUR", 3: "UAH", 4: "KZT", 5: "BYN", 6: "AED", 7: "UZS", 99: ""} {
		if got := friendworkCurrency[in]; got != want {
			t.Errorf("currency %d = %q, want %q", in, got, want)
		}
	}
}

func TestFriendWorkFallsBackToTheTokenURL(t *testing.T) {
	// A publication without externalPublicationUrl still gets an address on the career portal.
	body := `{"company":{"name":"X"},"publications":[{"publishedJobId":7,"title":"Dev","description":"<p>d</p>","salaryFrom":0,"salaryTo":0,"currency":0,"city":"","address":"Казань, ул. Баумана 1","workplaceType":"Hybrid","employmentType":"Fulltime","externalPublicationUrl":null,"publishDateTime":null,"dateCreated":"2024-05-20T16:54:10.1"}]}`
	fake := (&routedHTTP{}).route("token=acme", body)

	jobs, err := NewFriendWork(fake).Fetch(context.Background(), CompanyEntry{Company: "X", Board: "acme"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch = %v, %v", jobs, err)
	}
	j := jobs[0]
	if j.URL != "https://jobs.friend.work/acme/7" {
		t.Errorf("URL = %q", j.URL)
	}
	if j.Location != "Казань, ул. Баумана 1" {
		t.Errorf("Location = %q, want the address when no city is stated", j.Location)
	}
	if j.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q", j.WorkMode)
	}
	if j.PostedAt == nil || j.PostedAt.Year() != 2024 {
		t.Errorf("PostedAt = %v, want dateCreated when publishDateTime is null", j.PostedAt)
	}
	if j.SalaryMin != nil || j.SalaryMax != nil || j.SalaryCurrency != "" || j.SalaryPeriod != "" {
		t.Errorf("zero amounts mean no salary, got %+v", j)
	}
}

func TestFriendWorkUnknownTokenFailsTheBoard(t *testing.T) {
	fake := (&routedHTTP{}).routeErr("token=nope", &StatusError{Method: "GET", Code: http.StatusNotFound, URL: "x"})

	if _, err := NewFriendWork(fake).Fetch(context.Background(), CompanyEntry{Board: "nope"}); err == nil {
		t.Fatal("Fetch succeeded on an unknown token; want an error")
	}
}
