package main

import (
	"context"
	"testing"
)

func TestHigherMeProbe(t *testing.T) {
	p := highermeProber{}
	getter := fakeGetter{
		highermeProbeURL("6078977d8dfec"): `{"data":[{"id":"607aff1be9251"}],"meta":{"last_page":7,"total":7}}`,
		// An unknown brand is an empty listing, not an error.
		highermeProbeURL("0000000000000"): `{"data":[],"meta":{"last_page":1,"total":0}}`,
	}
	if name, n, err := p.probe(context.Background(), getter, "6078977d8dfec"); err != nil || name != "" || n != 7 {
		t.Errorf("live brand: got (%q,%d,%v), want (\"\",7,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "0000000000000"); err != nil || n != 0 {
		t.Errorf("unknown brand: got n=%d err=%v, want 0,nil", n, err)
	}
	// An unreachable listing is not live, and never a fatal error.
	if _, n, err := p.probe(context.Background(), getter, "fffffffffffff"); err != nil || n != 0 {
		t.Errorf("unreachable: got n=%d err=%v, want 0,nil", n, err)
	}
	if got := highermeProbeURL("6078977d8dfec"); got != "https://api.higherme.com/classic/jobs?filters%5Bbrand.id%5D=6078977d8dfec&limit=1&page=1" {
		t.Errorf("probe url = %q", got)
	}
}
