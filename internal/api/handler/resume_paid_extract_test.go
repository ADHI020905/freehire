package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/candidate/resume"
)

// The upload routes stay open to free; what is not free is the model reading the upload.
// paidExtraction is where the background extraction asks, and a refusal must leave a
// failed extract the profile can offer to retry — never a model call, never a forever
// "pending".
func TestPaidExtractionGatesTheBackgroundModelCall(t *testing.T) {
	cases := []struct {
		name       string
		plans      paidEntitlement
		wantRun    bool
		wantDetail string
	}{
		{"free", &fakeEntitlement{tier: plan.TierFree}, false, extractNeedsPaidPlan},
		{"unreadable plan", &fakeEntitlement{err: errors.New("database is down")}, false, "plan unavailable"},
		{"no plan store", nil, false, "plan unavailable"},
		{"pro", &fakeEntitlement{tier: plan.TierPro}, true, ""},
		{"ultra", &fakeEntitlement{tier: plan.TierUltra}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeResumeRepo{key: "resumes/1", set: true}
			h := &resumeHandlers{resume: resume.New(newFakeResumeBlobs(), repo), plans: tc.plans}

			ctx, ok := h.paidExtraction(context.Background(), 1, resumeUploadedAt)
			if ok != tc.wantRun {
				t.Fatalf("paidExtraction ran = %v, want %v", ok, tc.wantRun)
			}
			// Only a run that may go ahead carries the mark the binding checks.
			if plan.PaidAI(ctx) != tc.wantRun {
				t.Errorf("context paid mark = %v, want %v", plan.PaidAI(ctx), tc.wantRun)
			}
			repo.mu.Lock()
			status, detail := repo.extractStatus, repo.extractDetail
			repo.mu.Unlock()
			if tc.wantRun {
				if status == resume.ExtractStatusFailed {
					t.Errorf("a paid extraction was recorded as failed (%q)", detail)
				}
				return
			}
			if status != resume.ExtractStatusFailed || detail != tc.wantDetail {
				t.Errorf("extract status = (%q, %q), want (failed, %q)", status, detail, tc.wantDetail)
			}
		})
	}
}

// A free account's skipped extraction must not reach the client as `failed`: every surface
// renders that as "couldn't read your CV", which is false — the file was read, the model was
// simply not called. Nor is it pending, since no retry will run until the plan changes.
func TestWireParseStatusSeparatesPlanFromFailure(t *testing.T) {
	cases := []struct {
		name        string
		in          resume.ExtractStatus
		wantStatus  string
		wantPending bool
	}{
		{"skipped for plan", resume.ExtractStatus{Status: resume.ExtractStatusFailed, Detail: extractNeedsPaidPlan}, parseStatusRequiresPlan, false},
		{"real failure", resume.ExtractStatus{Status: resume.ExtractStatusFailed, Detail: "extract failed"}, resume.ExtractStatusFailed, true},
		{"plan unreadable is a failure", resume.ExtractStatus{Status: resume.ExtractStatusFailed, Detail: "plan unavailable"}, resume.ExtractStatusFailed, true},
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
