package pii

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Detector returns PII spans for a text. The production implementation is the local
// openai/privacy-filter span-detection endpoint (HTTPDetector); tests use a fake.
type Detector interface {
	Detect(ctx context.Context, text string) ([]Span, error)
}

// HTTPDetector calls the co-located privacy-filter detection endpoint. The endpoint owns
// the model and returns spans already mapped to our Kind vocabulary.
type HTTPDetector struct {
	url    string
	client *http.Client
}

// NewHTTPDetector builds a detector posting to url; client defaults to http.DefaultClient.
func NewHTTPDetector(url string, client *http.Client) *HTTPDetector {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPDetector{url: url, client: client}
}

type detectRequest struct {
	Text string `json:"text"`
}

type detectResponse struct {
	Spans []Span `json:"spans"`
}

// Detect posts the text and returns the endpoint's spans, as byte ranges in text. A
// transport error or a non-2xx response is returned as an error so the caller can fail
// closed.
func (d *HTTPDetector) Detect(ctx context.Context, text string) ([]Span, error) {
	body, err := json.Marshal(detectRequest{Text: text})
	if err != nil {
		return nil, fmt.Errorf("pii: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("pii: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pii: detect request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pii: detector returned status %d", resp.StatusCode)
	}
	var out detectResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("pii: decode response: %w", err)
	}
	return charSpansToBytes(text, out.Spans), nil
}

// charSpansToBytes converts the endpoint's offsets to the byte offsets Span carries. The
// endpoint is Python, and its tokenizer counts CHARACTERS (code points); read as bytes, every
// span after the first multi-byte character — an accent, a bullet, a dash — slides right and
// cuts the wrong text, so the value it meant is never masked.
//
// Ranging over the string yields one step per character the endpoint counted, an invalid
// byte included: encoding/json sends it as a single U+FFFD, and the range yields it as a
// single one-byte RuneError. An offset outside the text stays outside it, so sanitizeSpans
// still drops the span.
func charSpansToBytes(text string, spans []Span) []Span {
	byteAt := make([]int, 0, len(text)+1)
	for i := range text {
		byteAt = append(byteAt, i)
	}
	byteAt = append(byteAt, len(text))
	toByte := func(c int) int {
		switch {
		case c < 0:
			return c
		case c >= len(byteAt):
			return len(text) + 1
		default:
			return byteAt[c]
		}
	}
	out := make([]Span, len(spans))
	for i, s := range spans {
		out[i] = Span{Start: toByte(s.Start), End: toByte(s.End), Kind: s.Kind}
	}
	return out
}
