package main

import (
	"context"
	"testing"
)

func TestHarriProbe(t *testing.T) {
	p := harriProber{}
	const search = "https://gateway.harri.com/core/api/v1/harri_search/search_jobs"
	// Bodies as the platform answered them for Brindisa Kitchens on 2026-10-20, trimmed to the
	// fields the probe reads.
	live := fakeGetter{
		"https://gateway.harri.com/core/api/v1/profile/slug/brindisakitchenscareers": `{"data": {"id": 4018620, "type": "brand", "career_portal_enabled": true, "is_archived": false}, "status_code": 200, "status": "SUCCESS"}`,
		search: `{"data": {"hits": 7, "results": [{"id": 2843559}]}, "status_code": 200, "status": "SUCCESS"}`,
		"https://gateway.harri.com/core/api/v2/career_portal/brands/4018620/basic_info": `{"data": {"id": 4018620, "name": "Brindisa Kitchens", "slug": "BrindisaKitchensCareers", "type": "GP-1"}}`,
	}
	if name, n, err := p.probe(context.Background(), live, "BrindisaKitchensCareers"); err != nil || name != "Brindisa Kitchens" || n != 7 {
		t.Errorf("live: got (%q,%d,%v), want (\"Brindisa Kitchens\",7,nil)", name, n, err)
	}

	// An unknown slug: the real client reports the 400 as an error, the fake as a missing URL.
	if _, n, err := p.probe(context.Background(), live, "no-such-brand"); err != nil || n != 0 {
		t.Errorf("unknown slug: got n=%d err=%v, want 0,nil", n, err)
	}

	empty := fakeGetter{
		"https://gateway.harri.com/core/api/v1/profile/slug/quiet": `{"data": {"id": 1, "type": "brand"}}`,
		search: `{"data": {"hits": 0, "results": []}}`,
	}
	if _, n, err := p.probe(context.Background(), empty, "quiet"); err != nil || n != 0 {
		t.Errorf("no postings: got n=%d err=%v, want 0,nil", n, err)
	}

	// A live brand whose basic info cannot be read is still live; the name is left to the seed.
	nameless := fakeGetter{
		"https://gateway.harri.com/core/api/v1/profile/slug/acme": `{"data": {"id": 2, "type": "brand"}}`,
		search: `{"data": {"hits": 3, "results": []}}`,
	}
	if name, n, err := p.probe(context.Background(), nameless, "acme"); err != nil || name != "" || n != 3 {
		t.Errorf("nameless: got (%q,%d,%v), want (\"\",3,nil)", name, n, err)
	}
}
