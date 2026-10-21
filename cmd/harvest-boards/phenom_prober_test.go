package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// statusPoster answers every POST with an HTTP status error, standing in for a platform that
// refuses or no longer hosts the site.
type statusPoster struct {
	fakeGetter
	code int
}

func (s statusPoster) PostJSON(context.Context, string, any, any) error {
	return &sources.StatusError{Code: s.code}
}

func TestPhenomProbe(t *testing.T) {
	p := phenomProber{}
	getter := fakeGetter{
		"https://careers.example.com/widgets": `{"refineSearch":{"totalHits":2027,"data":{"jobs":[{"jobSeqNo":"ABB1"}]}}}`,
		"https://empty.example.com/widgets":   `{"refineSearch":{"totalHits":0,"data":{"jobs":[]}}}`,
		// A tenant that omits totalHits still says on the page whether a posting exists.
		"https://nototal.example.com/widgets": `{"refineSearch":{"data":{"jobs":[{"jobSeqNo":"X1"}]}}}`,
	}
	cases := []struct {
		board string
		want  int
	}{
		{"Careers.Example.com", 2027},
		{"empty.example.com", 0},
		{"nototal.example.com", 1},
	}
	for _, c := range cases {
		name, n, err := p.probe(context.Background(), getter, c.board)
		if err != nil || name != "" || n != c.want {
			t.Errorf("%s: got (%q,%d,%v), want (\"\",%d,nil)", c.board, name, n, err, c.want)
		}
	}
	// A site that is not a Phenom tenant answers 404: not a board, not a failure.
	if _, n, err := p.probe(context.Background(), statusPoster{code: 404}, "gone.example.com"); err != nil || n != 0 {
		t.Errorf("404: got n=%d err=%v, want 0,nil", n, err)
	}
	// Anything else is the probe failing, never an empty board.
	if _, _, err := p.probe(context.Background(), statusPoster{code: 403}, "blocked.example.com"); err == nil {
		t.Error("403: got nil error, want the probe to fail")
	}
}
