package main

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestCareerPage(t *testing.T) {
	cases := map[string]string{
		// The portal-tracked shape most stored postings carry (real prod URLs).
		"https://clicksign.gupy.io/job/eyJqb2JJZCI6MTE0MTU0NzksInNvdXJjZSI6Imd1cHlfcG9ydGFsIn0=?jobBoardSource=gupy_portal": "clicksign",
		"https://grupoboticario.gupy.io/jobs/11679903":                                             "grupoboticario",
		"https://bbc_digital.gupy.io/job/eyJqb2JJZCI6MTE2NDE1MDAsInNvdXJjZSI6Imd1cHlfcG9ydGFsIn0=": "bbc_digital",
		"https://RDSaude-Farmacia.gupy.io/jobs/1":                                                  "rdsaude-farmacia",
	}
	for url, want := range cases {
		if got, ok := careerPage(url); !ok || got != want {
			t.Errorf("careerPage(%q) = %q, %v; want %q", url, got, ok, want)
		}
	}
	for _, url := range []string{"", "https://example.com/jobs/1", "https://gupy.io/jobs/1"} {
		if got, ok := careerPage(url); ok {
			t.Errorf("careerPage(%q) = %q, want no page", url, got)
		}
	}
}

func TestIsCompanyIDBoard(t *testing.T) {
	for board, want := range map[string]bool{"2285": true, "caju": false, "": false, "12a": false} {
		if got := isCompanyIDBoard(board); got != want {
			t.Errorf("isCompanyIDBoard(%q) = %v, want %v", board, got, want)
		}
	}
}

func TestPlanBoardSplitsACompanyAcrossItsCareerPages(t *testing.T) {
	// Board 48481 on prod: most postings on origoenergia, some on its internal page.
	rows := []db.ListBoardJobURLsRow{
		{ID: 1, ExternalID: "48481:100", URL: "https://origoenergia.gupy.io/job/abc?jobBoardSource=gupy_portal"},
		{ID: 2, ExternalID: "48481:101", URL: "https://origoenergiainterno.gupy.io/jobs/101"},
		{ID: 3, ExternalID: "48481:102", URL: "https://origoenergia.gupy.io/jobs/102"},
		{ID: 4, ExternalID: "48481:103", URL: "https://example.com/elsewhere"},
	}
	p := planBoard("48481", rows)

	if want := []string{"origoenergia", "origoenergiainterno"}; !reflect.DeepEqual(p.pages, want) {
		t.Errorf("pages = %v, want %v (most postings first)", p.pages, want)
	}
	if want := []int64{1, 2, 3}; !reflect.DeepEqual(p.ids, want) {
		t.Errorf("ids = %v, want %v", p.ids, want)
	}
	if want := []string{"origoenergia:100", "origoenergiainterno:101", "origoenergia:102"}; !reflect.DeepEqual(p.externalIDs, want) {
		t.Errorf("externalIDs = %v, want %v", p.externalIDs, want)
	}
	if p.unplaced != 1 {
		t.Errorf("unplaced = %d, want 1", p.unplaced)
	}
	if p.alreadyMoved {
		t.Error("alreadyMoved = true for a companyId board")
	}
}

func TestPlanBoardRecognisesAnAllDigitPageBoard(t *testing.T) {
	// A board whose postings live on a page named exactly like it is a page board a previous
	// run wrote, and must not be retired as a companyId board.
	p := planBoard("99", []db.ListBoardJobURLsRow{{ID: 1, ExternalID: "99:5", URL: "https://99.gupy.io/jobs/5"}})
	if !p.alreadyMoved {
		t.Error("alreadyMoved = false, want true")
	}
}
