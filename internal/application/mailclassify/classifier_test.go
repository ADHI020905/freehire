package mailclassify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/llm"
)

type fakeGen struct {
	raw       string
	gotSystem string
	gotUser   string
	err       error
	called    bool
}

func (f *fakeGen) GenerateJSON(_ context.Context, system, user string, _ ...llm.GenOption) (string, error) {
	f.called = true
	f.gotSystem, f.gotUser = system, user
	return f.raw, f.err
}

func TestClassifyKeywordFastPathSkipsLLM(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"other","confidence":0.1}`} // must NOT be used
	c := &Classifier{gen: f}
	in := Input{
		Subject: "Regarding your application",
		Body:    "Unfortunately we have decided not to proceed with your application.",
		// no candidates → status-only (auto-linked) path
	}
	got, err := c.Classify(context.Background(), in)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if f.called {
		t.Error("LLM was called despite a confident keyword status")
	}
	if got.Signal != SignalRejection || got.Confidence != KeywordConfidence {
		t.Errorf("got (%q, %v), want (rejection, %v)", got.Signal, got.Confidence, KeywordConfidence)
	}
}

func TestClassifyKeywordFastPathDefersWhenAmbiguous(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"acknowledgement","confidence":0.7}`}
	c := &Classifier{gen: f}
	in := Input{Subject: "Thank you for your interest", Body: "Thank you for your interest in Xata!"}
	if _, err := c.Classify(context.Background(), in); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !f.called {
		t.Error("LLM should be called when no confident keyword status")
	}
}

func TestClassifyKeywordFastPathIgnoredWithCandidates(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"rejection","confidence":0.9,"matched_job_id":1}`}
	c := &Classifier{gen: f}
	in := Input{
		Subject:    "Regarding your application",
		Body:       "Unfortunately we have decided not to proceed.",
		Candidates: []Candidate{{JobID: 1, Company: "Acme"}}, // disambiguation needed → LLM
	}
	if _, err := c.Classify(context.Background(), in); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !f.called {
		t.Error("LLM must be called when candidates need disambiguation, even on a keyword hit")
	}
}

func TestClassifySanitizesAndValidatesMatch(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"garbage","confidence":2,"matched_job_id":999}`}
	c := &Classifier{gen: f}
	in := Input{Subject: "Hi", Candidates: []Candidate{{JobID: 1, Company: "Acme"}}}

	got, err := c.Classify(context.Background(), in)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Signal != SignalOther {
		t.Errorf("signal = %q, want coerced to other", got.Signal)
	}
	if got.Confidence != 1 {
		t.Errorf("confidence = %v, want clamped to 1", got.Confidence)
	}
	if got.MatchedJobID != 0 {
		t.Errorf("matched id = %d, want 0 (999 is not an offered candidate)", got.MatchedJobID)
	}
}

func TestClassifyKeepsValidMatch(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"interview_invitation","confidence":0.9,"matched_job_id":1}`}
	c := &Classifier{gen: f}
	in := Input{Subject: "Interview", Candidates: []Candidate{{JobID: 1, Company: "Acme"}}}

	got, err := c.Classify(context.Background(), in)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Signal != SignalInterviewInvitation || got.MatchedJobID != 1 {
		t.Errorf("got (%q, %d), want (interview_invitation, 1)", got.Signal, got.MatchedJobID)
	}
	// The LLM answers one confidence for both; it stands for the pick too.
	if got.MatchConfidence != 0.9 {
		t.Errorf("match confidence = %v, want 0.9", got.MatchConfidence)
	}
}

type fakeJev struct {
	out    Classification
	err    error
	called bool
}

func (f *fakeJev) Classify(context.Context, Input) (Classification, error) {
	f.called = true
	return f.out, f.err
}

func TestClassifyJevSkipsLLM(t *testing.T) {
	gen := &fakeGen{raw: `{"signal":"other","confidence":0.1}`}
	jev := &fakeJev{out: Classification{Signal: SignalOffer, Confidence: 0.91}}
	c := &Classifier{gen: gen, jev: jev}

	got, err := c.Classify(context.Background(), Input{Subject: "Update", Body: "Thank you for your interest in Xata!"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if gen.called {
		t.Error("LLM was called after Jev answered")
	}
	if !jev.called {
		t.Error("Jev was not called")
	}
	if got.Signal != SignalOffer || got.Confidence != 0.91 {
		t.Errorf("got (%q, %v), want (offer, 0.91)", got.Signal, got.Confidence)
	}
	if got.Model != jevModel {
		t.Errorf("model = %q, want %q", got.Model, jevModel)
	}
}

func TestClassifyJevFallsBackToLLM(t *testing.T) {
	gen := &fakeGen{raw: `{"signal":"screening","confidence":0.86}`}
	jev := &fakeJev{err: errors.New("jev: connection refused")}
	c := &Classifier{gen: gen, jev: jev}

	got, err := c.Classify(context.Background(), Input{Subject: "Hello", Body: "Can you tell us more about your notice period?"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !gen.called {
		t.Error("LLM was not called after Jev failed")
	}
	if got.Signal != SignalScreening || got.Confidence != 0.86 {
		t.Errorf("got (%q, %v), want (screening, 0.86)", got.Signal, got.Confidence)
	}
	if got.Model != "" {
		t.Errorf("model = %q, want empty (the runner's LLM model)", got.Model)
	}
}

func TestClassifyKeywordSkipsJev(t *testing.T) {
	jev := &fakeJev{out: Classification{Signal: SignalOther, Confidence: 0.1}}
	c := &Classifier{gen: &fakeGen{}, jev: jev}

	got, err := c.Classify(context.Background(), Input{
		Subject: "Regarding your application",
		Body:    "Unfortunately we have decided not to proceed with your application.",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if jev.called {
		t.Error("Jev was called despite a confident keyword status")
	}
	if got.Signal != SignalRejection {
		t.Errorf("signal = %q, want rejection", got.Signal)
	}
}

func TestClassifyJevUnusableAnswerFallsBack(t *testing.T) {
	gen := &fakeGen{raw: `{"signal":"rejection","confidence":0.9,"matched_job_id":1}`}
	jev := &fakeJev{out: Classification{Signal: SignalRejection, Confidence: 0.99, MatchedJobID: 999}}
	c := &Classifier{gen: gen, jev: jev}
	in := Input{Subject: "Hi", Body: "No thanks", Candidates: []Candidate{{JobID: 1, Company: "Acme"}}}

	got, err := c.Classify(context.Background(), in)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !gen.called {
		t.Error("LLM was not called after Jev picked an application that was not offered")
	}
	if got.MatchedJobID != 1 {
		t.Errorf("matched id = %d, want the LLM's 1", got.MatchedJobID)
	}
}

func TestClassifyCanceledContextDoesNotFallBack(t *testing.T) {
	gen := &fakeGen{raw: `{"signal":"other","confidence":0.2}`}
	jev := &fakeJev{err: context.Canceled}
	c := &Classifier{gen: gen, jev: jev}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Classify(ctx, Input{Subject: "Hi", Body: "Hello"}); err == nil {
		t.Fatal("Classify returned no error after the caller was canceled")
	}
	if gen.called {
		t.Error("LLM was called after the caller was canceled")
	}
}

func TestNewClassifierWiresJevOnlyWhenKeySet(t *testing.T) {
	if got := NewClassifier(nil, "  "); got.jev != nil {
		t.Fatal("Jev client wired without an api key")
	}
	if got := NewClassifier(nil, "k"); got.jev == nil {
		t.Fatal("Jev client missing")
	}
}

func TestClassifyUserPromptCarriesCandidates(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"other","confidence":0.1}`}
	c := &Classifier{gen: f}
	in := Input{Subject: "S", Candidates: []Candidate{{JobID: 7, Company: "Globex"}}}

	if _, err := c.Classify(context.Background(), in); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !strings.Contains(f.gotUser, "Globex") || !strings.Contains(f.gotUser, "7") {
		t.Errorf("user prompt does not carry candidate id/company:\n%s", f.gotUser)
	}
}
