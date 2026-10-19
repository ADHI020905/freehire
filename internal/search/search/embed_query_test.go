package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// teiQuery serves one descDimensions-wide vector and records what it was sent.
func teiQuery(t *testing.T, delay time.Duration, status int, gotInputs *[]string, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Inputs []string `json:"inputs"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if gotInputs != nil {
			*gotInputs = body.Inputs
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode([][]float32{make([]float32, descDimensions)})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNewQueryEmbedderWithoutURLIsNil(t *testing.T) {
	if e := NewQueryEmbedder("", "k", time.Second); e != nil {
		t.Errorf("NewQueryEmbedder(\"\") = %v, want nil so the caller knows semantic search is unavailable", e)
	}
}

func TestEmbedQuerySendsTheE5QueryForm(t *testing.T) {
	var inputs []string
	var calls atomic.Int32
	srv := teiQuery(t, 0, http.StatusOK, &inputs, &calls)

	vec, err := NewQueryEmbedder(srv.URL, "k", time.Second).EmbedQuery(context.Background(), "go backend")
	if err != nil {
		t.Fatalf("EmbedQuery: %v", err)
	}
	if len(vec) != descDimensions {
		t.Errorf("vector width = %d, want %d", len(vec), descDimensions)
	}
	if len(inputs) != 1 || inputs[0] != "query: go backend" {
		t.Errorf("sent inputs = %q, want [\"query: go backend\"]", inputs)
	}
}

// A slow backend must cost the request its timeout and no more — and one attempt only.
func TestEmbedQueryGivesUpAtTheTimeoutWithoutRetrying(t *testing.T) {
	var calls atomic.Int32
	srv := teiQuery(t, 2*time.Second, http.StatusOK, nil, &calls)

	start := time.Now()
	_, err := NewQueryEmbedder(srv.URL, "k", 50*time.Millisecond).EmbedQuery(context.Background(), "go")
	if err == nil {
		t.Fatal("EmbedQuery succeeded against a backend slower than its timeout")
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("EmbedQuery took %v, want it bounded by its 50ms timeout", took)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("backend called %d times, want exactly 1 — the request path never retries", n)
	}
}

func TestEmbedQueryFailsOnAnErrorStatusWithoutRetrying(t *testing.T) {
	var calls atomic.Int32
	srv := teiQuery(t, 0, http.StatusServiceUnavailable, nil, &calls)

	if _, err := NewQueryEmbedder(srv.URL, "k", time.Second).EmbedQuery(context.Background(), "go"); err == nil {
		t.Fatal("EmbedQuery succeeded against a 503")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("backend called %d times, want exactly 1", n)
	}
}

// A backend serving a different model would put the query in another vector space; the
// index would reject the width anyway, so refuse it here and fall back cleanly.
func TestEmbedQueryRejectsAWrongWidth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([][]float32{{1, 2, 3}})
	}))
	t.Cleanup(srv.Close)

	if _, err := NewQueryEmbedder(srv.URL, "", time.Second).EmbedQuery(context.Background(), "go"); err == nil {
		t.Fatal("EmbedQuery accepted a 3-dimensional vector")
	}
}
