package pii

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPDetectorParsesSpans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spans":[{"start":0,"end":4,"kind":"NAME"},{"start":10,"end":20,"kind":"ADDRESS"}]}`))
	}))
	defer srv.Close()

	d := NewHTTPDetector(srv.URL, srv.Client())
	spans, err := d.Detect(context.Background(), "Alex lives at 5th Avenue")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(spans) != 2 || spans[0].Kind != KindName || spans[1].Kind != KindAddress {
		t.Fatalf("unexpected spans: %+v", spans)
	}
	if spans[0].Start != 0 || spans[0].End != 4 {
		t.Fatalf("bad first span: %+v", spans[0])
	}
}

func TestHTTPDetectorErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	d := NewHTTPDetector(srv.URL, srv.Client())
	if _, err := d.Detect(context.Background(), "text"); err == nil {
		t.Fatal("expected error on non-200 response, got nil")
	}
}

func TestHTTPDetectorErrorsOnTransport(t *testing.T) {
	d := NewHTTPDetector("http://127.0.0.1:1/detect", http.DefaultClient)
	if _, err := d.Detect(context.Background(), "text"); err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

// The endpoint is Python, and its tokenizer reports offsets in CHARACTERS (code points);
// Span is a BYTE range. Read unconverted, every span after the first multi-byte character
// slides right — measured on prod, "••• Résumé — John Smith" came back as the NAME
// "sumé — ", so the real name reached the model unmasked. Detect must hand back byte
// offsets that cut the same value the endpoint meant.
func TestHTTPDetectorConvertsCharacterOffsetsToBytes(t *testing.T) {
	text := "••• Résumé — John Smith, \xffjohn@example.com"
	runes := []rune(text)
	charRange := func(value string) (int, int) {
		for i := range runes {
			if strings.HasPrefix(string(runes[i:]), value) {
				return i, i + len([]rune(value))
			}
		}
		t.Fatalf("%q not in text", value)
		return 0, 0
	}
	nameStart, nameEnd := charRange("John Smith")
	mailStart, mailEnd := charRange("john@example.com")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"spans":[{"start":%d,"end":%d,"kind":"NAME"},{"start":%d,"end":%d,"kind":"EMAIL"}]}`,
			nameStart, nameEnd, mailStart, mailEnd)
	}))
	defer srv.Close()

	spans, err := NewHTTPDetector(srv.URL, srv.Client()).Detect(context.Background(), text)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	want := []string{"John Smith", "john@example.com"}
	if len(spans) != len(want) {
		t.Fatalf("spans = %+v, want %d", spans, len(want))
	}
	for i, s := range spans {
		if got := text[s.Start:s.End]; got != want[i] {
			t.Errorf("span %d cuts %q, want %q", i, got, want[i])
		}
	}
}
