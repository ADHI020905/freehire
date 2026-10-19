package config

import (
	"testing"
	"time"
)

func TestLoadSemanticSearchDefaults(t *testing.T) {
	t.Setenv("EMBED_URL", "")
	t.Setenv("EMBED_QUERY_TIMEOUT_MS", "")
	t.Setenv("SEMANTIC_SCORE_THRESHOLD", "")

	s, err := LoadSemanticSearch()
	if err != nil {
		t.Fatalf("LoadSemanticSearch: %v", err)
	}
	if s.URL != "" {
		t.Errorf("URL = %q, want empty: no default backend on the request path", s.URL)
	}
	if s.QueryTimeout != 400*time.Millisecond || s.ScoreThreshold != 0 {
		t.Errorf("got timeout %v threshold %v, want 400ms and 0", s.QueryTimeout, s.ScoreThreshold)
	}
}

func TestLoadSemanticSearchReadsValues(t *testing.T) {
	t.Setenv("EMBED_URL", "http://tei/embed")
	t.Setenv("EMBED_QUERY_TIMEOUT_MS", "250")
	t.Setenv("SEMANTIC_SCORE_THRESHOLD", "0.72")

	s, err := LoadSemanticSearch()
	if err != nil {
		t.Fatalf("LoadSemanticSearch: %v", err)
	}
	if s.URL != "http://tei/embed" || s.QueryTimeout != 250*time.Millisecond || s.ScoreThreshold != 0.72 {
		t.Errorf("got %+v", s)
	}
}

func TestLoadSemanticSearchRefusesUnreadableValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"EMBED_QUERY_TIMEOUT_MS", "fast"},
		{"EMBED_QUERY_TIMEOUT_MS", "0"},
		{"SEMANTIC_SCORE_THRESHOLD", "high"},
		{"SEMANTIC_SCORE_THRESHOLD", "72"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv("EMBED_QUERY_TIMEOUT_MS", "")
			t.Setenv("SEMANTIC_SCORE_THRESHOLD", "")
			t.Setenv(tc.key, tc.value)
			if _, err := LoadSemanticSearch(); err == nil {
				t.Errorf("%s=%q accepted, want an error", tc.key, tc.value)
			}
		})
	}
}
