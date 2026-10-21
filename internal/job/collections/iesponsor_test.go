package collections

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseIEPermitEmployers_SkipsEachYearsBannerHeaderAndTotalRows(t *testing.T) {
	// The workbooks change layout every year; these are the 2023, 2025 and 2026
	// shapes. Only column A is relied on.
	layouts := map[string][][]any{
		"2023": {{"Grand Total", "2023"}, {nil, "Issued"}, {"Employer Name", nil, "Jan"}, {"Grand Total", 30981}, {" Stripe Payments Europe Limited", 4, 1}},
		"2025": {{nil, "January", "Grand Total"}, {"Grand Total", 1, 31044}, {"Stripe Payments Europe Limited", 1, 4}},
		"2026": {{"Employer Name", "Permits Issued Jan", "Permits Issued Grand Total"}, {"Stripe Payments Europe Limited", 1, 4}, {"Total", 29989, 29989}},
	}
	want := []Record{{Name: "Stripe Payments Europe Limited"}}
	for year, rows := range layouts {
		got, err := ParseIEPermitEmployers(workbook(t, rows))
		if err != nil {
			t.Fatalf("%s: ParseIEPermitEmployers: %v", year, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: ParseIEPermitEmployers = %+v, want %+v", year, got, want)
		}
	}
}

func TestParseIEPermitEmployers_RejectsAnEmptyParse(t *testing.T) {
	if _, err := ParseIEPermitEmployers(workbook(t, [][]any{{"Employer Name", "Total"}, {"Total", 0}})); err == nil {
		t.Fatal("ParseIEPermitEmployers accepted a workbook with no employers")
	}
}

func TestPickRecentIEYearPages_HandlesBothPathCasingsNewestFirst(t *testing.T) {
	index := "" +
		`<a href="/en/Publications/Employment-Permit-Statisitcs-2013.html">2013</a>` + //nolint:misspell // the live page misspells 2013's link; the pattern must still read it
		`<a href="/en/Publications/Employment-Permit-Statistics-2022.html">2022</a>` +
		`<a href="/en/publications/employment-permit-statistics-2024.html">2024</a>` +
		`<a href="/en/publications/employment-permit-statistics-2023.html">2023</a>` +
		`<a href="/en/publications/employment-permit-statistics-2024.html">2024 again</a>`
	got, err := pickRecentIEYearPages([]byte(index), 3)
	if err != nil {
		t.Fatalf("pickRecentIEYearPages: %v", err)
	}
	var years []int
	for _, p := range got {
		years = append(years, p.Year)
	}
	if want := []int{2024, 2023, 2022}; !reflect.DeepEqual(years, want) {
		t.Errorf("years = %v, want %v", years, want)
	}
}

func TestPickRecentIEYearPages_ErrorsWhenFewerThanNYearsAreListed(t *testing.T) {
	if _, err := pickRecentIEYearPages([]byte(`<a href="/en/publications/employment-permit-statistics-2026.html">`), 3); err == nil {
		t.Fatal("pickRecentIEYearPages accepted a shrunken index")
	}
}

// ieServer serves the index → year page → workbook chain for years; a year in
// missingWorkbook links a workbook that 404s.
func ieServer(t *testing.T, years []string, missingWorkbook string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats/", func(w http.ResponseWriter, r *http.Request) {
		for _, y := range years {
			_, _ = w.Write([]byte(`<a href="/en/publications/employment-permit-statistics-` + y + `.html">` + y + `</a>`))
		}
	})
	for _, y := range years {
		file := "/en/publications/publication-files/permits-issued-to-companies-" + y + ".xlsx"
		mux.HandleFunc("/en/publications/employment-permit-statistics-"+y+".html", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`<a href="` + file + `">Permits issued to companies</a>`))
		})
		if y == missingWorkbook {
			continue
		}
		body := workbook(t, [][]any{{"Employer Name", "Grand Total"}, {"Acme Robotics " + y + " Limited", 1}})
		mux.HandleFunc(file, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) })
	}
	return httptest.NewServer(mux)
}

func TestFetchIEPermitEmployers_CombinesTheYearWindow(t *testing.T) {
	srv := ieServer(t, []string{"2026", "2025", "2024", "2023"}, "")
	defer srv.Close()

	got, err := fetchIEPermitEmployers(context.Background(), srv.Client(), srv.URL+"/stats/")
	if err != nil {
		t.Fatalf("fetchIEPermitEmployers: %v", err)
	}
	want := []Record{{Name: "Acme Robotics 2026 Limited"}, {Name: "Acme Robotics 2025 Limited"}, {Name: "Acme Robotics 2024 Limited"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fetchIEPermitEmployers = %+v, want %+v", got, want)
	}
}

func TestFetchIEPermitEmployers_FailsWholeCallWhenOneYearIsUnreachable(t *testing.T) {
	srv := ieServer(t, []string{"2026", "2025", "2024"}, "2025")
	defer srv.Close()

	if _, err := fetchIEPermitEmployers(context.Background(), srv.Client(), srv.URL+"/stats/"); err == nil {
		t.Fatal("fetchIEPermitEmployers returned a partial window instead of failing")
	}
}
