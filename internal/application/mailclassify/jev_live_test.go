package mailclassify

import (
	"context"
	"os"
	"testing"
)

// TestJevLiveSignalAndApplication is the one check the mocks cannot make: that the live
// API answers a two-question request (signal + application) in the shape
// classificationFrom decodes, probabilities included. A drifted shape would not fail
// loudly in production — Classify would fall back to the LLM and only the log would say
// so. Skipped without a key:
//
//	TYPESAFE_API_KEY=... go test -run TestJevLive -v ./internal/application/mailclassify/
func TestJevLiveSignalAndApplication(t *testing.T) {
	key := os.Getenv("TYPESAFE_API_KEY")
	if key == "" {
		t.Skip("TYPESAFE_API_KEY not set")
	}

	got, err := newJevClient(key).Classify(context.Background(), Input{
		FromName: "Greenhouse",
		Subject:  "Interview invitation — Senior Backend Engineer at Derq",
		Body: "Hi Ilya, thanks for applying to Derq. We'd like to invite you to a 45-minute " +
			"technical interview with our engineering team. Please pick a slot using the link below.",
		Candidates: []Candidate{
			{JobID: 11, Company: "Xata"},
			{JobID: 22, Company: "Derq"},
			{JobID: 33, Company: "Fingerprint"},
		},
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	t.Logf("signal=%s confidence=%.3f job=%d match_confidence=%.3f",
		got.Signal, got.Confidence, got.MatchedJobID, got.MatchConfidence)
	if got.Signal != SignalInterviewInvitation {
		t.Errorf("signal = %q, want interview_invitation", got.Signal)
	}
	if got.MatchedJobID != 22 {
		t.Errorf("matched job = %d, want 22 (Derq)", got.MatchedJobID)
	}
	if got.MatchConfidence <= 0 || got.MatchConfidence > 1 {
		t.Errorf("match confidence = %v, want (0,1] — application probabilities missing?", got.MatchConfidence)
	}
	if got.Confidence <= 0 || got.Confidence > 1 {
		t.Errorf("confidence = %v, want (0,1] — probabilities missing from the answer?", got.Confidence)
	}
}
