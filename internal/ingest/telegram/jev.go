package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// JevJudge is the VacancyJudge backed by TypeSafe's Jev: one yes/no question per post,
// answered with a score in ~0.2s. It is a discrete decision over a fixed question, which
// is what Jev is for and what a generative model is a slow, costly way to ask.
//
// Measured 2026-10-21 on prod posts, with a model reading each one as the reference: of 90
// real vacancies among 150 the markers had declined, it admitted all 90 and 3 non-vacancies
// beside them; of 144 vacancies among 150 the markers admitted, it would have declined one.
//
// It carries its own small client, as internal/ai/autofillagent and internal/api/atsapply
// do: each of those is unexported and shaped around its own question (a choice over form
// options), so there is no shared one to reuse. The seam, once a fourth caller appears, is
// one Jev transport in internal/platform beside llm.
type JevJudge struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

const (
	jevDefaultBaseURL = "https://api.typesafe.ai"
	jevModel          = "jev-latest"
	jevTimeout        = 15 * time.Second

	// jevQuestionKey names the one question in the request and its answer.
	jevQuestionKey = "vacancy"

	// jevThreshold is the score at which a post counts as a vacancy. The measurement above
	// separated cleanly: the same admissions at 0.3 and 0.5, two vacancies lost at 0.7.
	jevThreshold = 0.5

	// jevMaxRunes bounds what one question sends. A Telegram post holds up to 4096
	// characters and the opening is where a vacancy says it is one; the measurement sent
	// the first 3000.
	jevMaxRunes = 3000

	jevInstructions = "Does this Telegram post advertise at least one job vacancy — an employer or " +
		"recruiter offering a position someone can apply to? Answer no for a job SEEKER's post, " +
		"news, articles, courses, events, memes, or chat."
)

// JevJudgeOption configures a JevJudge.
type JevJudgeOption func(*JevJudge)

// WithJevBaseURL points the judge at another endpoint (a test server).
func WithJevBaseURL(url string) JevJudgeOption {
	return func(j *JevJudge) { j.baseURL = url }
}

// NewJevJudge builds a judge authenticated with apiKey.
func NewJevJudge(apiKey string, opts ...JevJudgeOption) *JevJudge {
	j := &JevJudge{
		httpClient: &http.Client{Timeout: jevTimeout},
		baseURL:    jevDefaultBaseURL,
		apiKey:     apiKey,
	}
	for _, opt := range opts {
		opt(j)
	}
	return j
}

type jevRequest struct {
	Model     string                 `json:"model"`
	State     map[string]string      `json:"state"`
	Questions map[string]jevQuestion `json:"questions"`
}

type jevQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

type jevResponse struct {
	Answers map[string]struct {
		Noul *float64 `json:"noul"`
	} `json:"answers"`
}

// IsVacancy asks whether text advertises a vacancy. Any answer that is not a score is an
// error, never a "no" — the caller admits on an error, and a "no" is final.
func (j *JevJudge) IsVacancy(ctx context.Context, text string) (bool, error) {
	if r := []rune(text); len(r) > jevMaxRunes {
		text = string(r[:jevMaxRunes])
	}
	body, err := json.Marshal(jevRequest{
		Model: jevModel,
		State: map[string]string{"post": text},
		Questions: map[string]jevQuestion{
			jevQuestionKey: {Type: "noul", Instructions: jevInstructions},
		},
	})
	if err != nil {
		return false, fmt.Errorf("jev: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("jev: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.apiKey)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("jev: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return false, fmt.Errorf("jev: status %d: %s", resp.StatusCode, snippet)
	}

	var out jevResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, fmt.Errorf("jev: decode response: %w", err)
	}
	answer, ok := out.Answers[jevQuestionKey]
	if !ok || answer.Noul == nil {
		return false, fmt.Errorf("jev: response carries no score for %q", jevQuestionKey)
	}
	return *answer.Noul >= jevThreshold, nil
}

var _ VacancyJudge = (*JevJudge)(nil)

// JudgeFromKey returns the Jev judge for apiKey, or nil when there is no key — and nil as
// an INTERFACE, which is what the runners test for. Returning a nil *JevJudge here would
// hand them a non-nil judge that panics on its first question, so both binaries build the
// judge through this rather than each spelling the condition.
func JudgeFromKey(apiKey string) VacancyJudge {
	if apiKey == "" {
		return nil
	}
	return NewJevJudge(apiKey)
}
