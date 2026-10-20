package main

import (
	"context"
	"os"
	"testing"
)

func TestIdealTraitsProbe(t *testing.T) {
	live, err := os.ReadFile("../../internal/ingest/sources/testdata/idealtraits/listing_1162292.json")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := os.ReadFile("../../internal/ingest/sources/testdata/idealtraits/listing_8739_empty.json")
	if err != nil {
		t.Fatal(err)
	}
	getter := fakeGetter{
		"https://app.idealtraits.com/company/MTE2MjI5Mg==/careers?page=1": string(live),
		"https://app.idealtraits.com/company/ODczOQ==/careers?page=1":     string(empty),
	}
	p := idealtraitsProber{}
	if name, n, err := p.probe(context.Background(), getter, "1162292"); err != nil || name != "" || n != 5 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",5,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "8739"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// An unknown id (the platform answers 403) and a malformed board are skips, never fatal.
	for _, board := range []string{"99999999", "Comma-Insurance"} {
		if _, n, err := p.probe(context.Background(), getter, board); err != nil || n != 0 {
			t.Errorf("%s: got n=%d err=%v, want 0,nil", board, n, err)
		}
	}
}
