package collections

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// workbook builds an in-memory XLSX whose first sheet holds rows, so a parser is
// exercised against the real container format rather than a stand-in.
func workbook(t *testing.T, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// lmiaWorkbook is the live file's shape: a title row, the header, data rows, and
// numbered footnotes in column A.
func lmiaWorkbook(t *testing.T, employers ...[2]string) []byte {
	rows := [][]any{
		{"Employers Who Were Issued a Positive Labour Market Impact Assessment (LMIA) ..."},
		{"Province/Territory", "Program Stream", "Employer", "Address", "Occupation", "Incorporate Status", "Approved LMIAs", "Approved Positions"},
	}
	for _, e := range employers {
		rows = append(rows, []any{"Ontario", e[0], e[1], "Toronto, ON M5V 2T6", "21231-Software engineers and designers", "Corporation", 1, 1})
	}
	rows = append(rows, []any{"Notes:"}, []any{"3. The employer name is manually entered in the system."})
	return workbook(t, rows)
}

func TestParseCALMIASponsors_ReadsEmployersAndStreamsBetweenTitleAndFootnotes(t *testing.T) {
	data := lmiaWorkbook(t,
		[2]string{"High Wage          ", "Shopify Inc."},
		[2]string{"Primary Agriculture", "Fennor Farm Ltd"},
	)
	got, err := ParseCALMIASponsors(data)
	if err != nil {
		t.Fatalf("ParseCALMIASponsors: %v", err)
	}
	want := []Record{
		{Name: "Shopify Inc.", Meta: map[string]string{"route": "High Wage"}},
		{Name: "Fennor Farm Ltd", Meta: map[string]string{"route": "Primary Agriculture"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseCALMIASponsors =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseCALMIASponsors_RejectsAWorkbookWithoutTheHeader(t *testing.T) {
	// A renamed column must fail loudly: read positionally it would yield the wrong
	// cells, and read as "nothing found" it would reconcile the tag off everyone.
	data := workbook(t, [][]any{{"Province", "Stream", "Business", "Address"}, {"Ontario", "High Wage", "Shopify Inc.", "Ottawa"}})
	if _, err := ParseCALMIASponsors(data); err == nil {
		t.Fatal("ParseCALMIASponsors accepted a workbook with no Employer / Program Stream header")
	}
}

func TestParseCALMIASponsors_RejectsAnEmptyParse(t *testing.T) {
	if _, err := ParseCALMIASponsors(lmiaWorkbook(t)); err == nil {
		t.Fatal("ParseCALMIASponsors accepted a workbook with no employer rows")
	}
}

func TestCAWorkStreams_AdmitSkilledStreamsOnly(t *testing.T) {
	gate := RequireRoute(caWorkStreams...)
	for stream, want := range map[string]bool{
		"High Wage":               true,
		"Global Talent Stream":    true,
		"Permanent Resident Only": true,
		"Primary Agriculture":     false,
		"Low Wage":                false,
	} {
		if got := gate(Company{}, Record{Meta: map[string]string{"route": stream}}); got != want {
			t.Errorf("stream %q admitted = %v, want %v", stream, got, want)
		}
	}
}

func lmiaPackage(base string, quarters ...string) string {
	var res []string
	for _, q := range quarters {
		res = append(res, fmt.Sprintf(`{"url":"%s/download/tfwp_%s_pos_en.xlsx"}`, base, q),
			fmt.Sprintf(`{"url":"%s/download/tfwp_%s_pos_fr.xlsx"}`, base, q))
	}
	res = append(res, fmt.Sprintf(`{"url":"%s/download/2015_positive_employers_en.csv"}`, base))
	return `{"result":{"resources":[` + strings.Join(res, ",") + `]}}`
}

func TestPickRecentLMIAFiles_ReturnsTheNewestEnglishQuartersDescending(t *testing.T) {
	payload := lmiaPackage("https://x", "2025q2", "2026q1", "2025q3", "2024q4", "2025q4")
	got, err := pickRecentLMIAFiles([]byte(payload), 4)
	if err != nil {
		t.Fatalf("pickRecentLMIAFiles: %v", err)
	}
	var quarters []int
	for _, f := range got {
		quarters = append(quarters, f.Quarter)
		if !strings.HasSuffix(f.URL, "_pos_en.xlsx") {
			t.Errorf("picked a non-English file: %s", f.URL)
		}
	}
	if want := []int{20261, 20254, 20253, 20252}; !reflect.DeepEqual(quarters, want) {
		t.Errorf("quarters = %v, want %v", quarters, want)
	}
}

func TestPickRecentLMIAFiles_ErrorsWhenFewerThanNQuartersAreListed(t *testing.T) {
	if _, err := pickRecentLMIAFiles([]byte(lmiaPackage("https://x", "2026q1", "2025q4")), 4); err == nil {
		t.Fatal("pickRecentLMIAFiles accepted a shrunken dataset")
	}
}

func TestFetchCALMIASponsors_CombinesTheQuarterWindow(t *testing.T) {
	quarters := []string{"2026q1", "2025q4", "2025q3", "2025q2"}
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/package", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(lmiaPackage(srv.URL, quarters...)))
	})
	for _, q := range quarters {
		body := lmiaWorkbook(t, [2]string{"High Wage", "Acme Robotics " + q})
		mux.HandleFunc("/download/tfwp_"+q+"_pos_en.xlsx", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(body)
		})
	}
	srv = httptest.NewServer(mux)
	defer srv.Close()

	got, err := fetchCALMIASponsors(context.Background(), srv.Client(), srv.URL+"/package")
	if err != nil {
		t.Fatalf("fetchCALMIASponsors: %v", err)
	}
	if len(got) != len(quarters) {
		t.Fatalf("fetchCALMIASponsors returned %d records, want %d", len(got), len(quarters))
	}
}

func TestFetchCALMIASponsors_FailsWholeCallWhenOneQuarterIsUnreachable(t *testing.T) {
	quarters := []string{"2026q1", "2025q4", "2025q3", "2025q2"}
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/package", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(lmiaPackage(srv.URL, quarters...)))
	})
	for _, q := range quarters[1:] { // the newest quarter 404s
		body := lmiaWorkbook(t, [2]string{"High Wage", "Acme Robotics"})
		mux.HandleFunc("/download/tfwp_"+q+"_pos_en.xlsx", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(body)
		})
	}
	srv = httptest.NewServer(mux)
	defer srv.Close()

	if _, err := fetchCALMIASponsors(context.Background(), srv.Client(), srv.URL+"/package"); err == nil {
		t.Fatal("fetchCALMIASponsors returned a partial window instead of failing")
	}
}
