package main

import (
	"context"
	"testing"
)

func TestHCSHiringProbe(t *testing.T) {
	p := hcshiringProber{}
	getter := fakeGetter{
		// regencypark's first page as captured 2026-10-20, rows trimmed to one.
		"https://regencypark.hcshiring.com/api/jobs?page=1": `{"jobs":[{"id":"hs4dEISPR0ym2tqFezL11A","title":"Certified Occupational Therapist Assistant - COTA","isInternalOnly":false,"city":"College Place","state":"WA","zip":"99324"}],"meta":{"page":1,"perPage":10,"totalJobs":16,"totalPages":2}}`,
		"https://emptytenant.hcshiring.com/api/jobs?page=1": `{"jobs":[],"meta":{"page":1,"perPage":10,"totalJobs":0,"totalPages":0}}`,
	}
	// A live board is counted by meta.totalJobs (16 over two pages), not by the ten rows the
	// first page carries, and the employer name is left to the seed.
	if name, n, err := p.probe(context.Background(), getter, "regencypark"); err != nil || name != "" || n != 16 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",16,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "emptytenant"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// An unknown tenant answers 404; the prober reads that as "not a live board".
	if _, n, err := p.probe(context.Background(), getter, "nosuchtenant"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
	// A board that is not one DNS label would address some other host, and is refused without
	// a request.
	for _, bad := range []string{"", "regencypark.hcshiring.com", "evil.com/x", "-x"} {
		if _, n, err := p.probe(context.Background(), getter, bad); err != nil || n != 0 {
			t.Errorf("malformed %q: got n=%d err=%v, want 0,nil", bad, n, err)
		}
	}
}

func TestHCSHiringProberRegistered(t *testing.T) {
	p, ok := proberFor("hcshiring")
	if !ok {
		t.Fatal(`proberFor("hcshiring") not found`)
	}
	// The bespoke prober must win: the adapter fallback would walk every page and hydrate every
	// posting's body to count a board.
	if _, fellBack := p.(adapterProber); fellBack {
		t.Errorf("hcshiring resolved to the adapter fallback, got %T", p)
	}
}
