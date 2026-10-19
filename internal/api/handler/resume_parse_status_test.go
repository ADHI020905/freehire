package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resume"
)

// A failed parse is still owed a structure — the retry may fill it — while a landed or
// absent one is not.
func TestWireParseStatus(t *testing.T) {
	cases := []struct {
		name        string
		in          resume.ExtractStatus
		wantStatus  string
		wantPending bool
	}{
		{"failure", resume.ExtractStatus{Status: resume.ExtractStatusFailed, Detail: "extract failed"}, resume.ExtractStatusFailed, true},
		{"pending", resume.ExtractStatus{Status: resume.ExtractStatusPending}, resume.ExtractStatusPending, true},
		{"ok", resume.ExtractStatus{Status: resume.ExtractStatusOK}, resume.ExtractStatusOK, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, pending := wireParseStatus(tc.in)
			if status != tc.wantStatus || pending != tc.wantPending {
				t.Fatalf("wireParseStatus(%+v) = (%q, %v), want (%q, %v)", tc.in, status, pending, tc.wantStatus, tc.wantPending)
			}
		})
	}
}
