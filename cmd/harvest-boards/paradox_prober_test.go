package main

import (
	"context"
	"testing"
)

const paradoxProbeURL = "https://olivia.paradox.ai/api/job-list-site-ui/search-job?compsite_slug=kfc28&limit=1&page=1"

func TestParadoxProbe(t *testing.T) {
	spy := &headerSpy{fakeGetter: fakeGetter{
		// The live answer for KFC28, trimmed to one row: total_jobs counts the whole site.
		paradoxProbeURL: `{"jobs": [{"job_title": "Team Member"}], "total_jobs": 709, "success": true}`,
	}}
	// The board's site is lower-cased into the request, the way the catalogue stores it.
	if name, n, err := (paradoxProber{}).probe(context.Background(), spy, "olivia.paradox.ai/KFC28"); err != nil || name != "" || n != 709 {
		t.Errorf("got (%q,%d,%v), want (\"\",709,nil)", name, n, err)
	}
	if len(spy.sent) != 1 || spy.sent[0]["X-Requested-With"] != "XMLHttpRequest" {
		t.Errorf("headers sent = %v; the API answers 404 without X-Requested-With", spy.sent)
	}
}

func TestParadoxProbeSkipsWhatIsNotALiveBoard(t *testing.T) {
	p := paradoxProber{}
	for _, board := range []string{
		"olivia.paradox.ai/nolist", // 404 "Site X is not found": the fake has no answer for it
		"kfc28",                    // no host
		"example.com/kfc28",        // not a paradox.ai instance — never fetched
		"olivia.paradox.ai/a/b",
	} {
		if _, n, err := p.probe(context.Background(), fakeGetter{}, board); err != nil || n != 0 {
			t.Errorf("%q: got n=%d err=%v, want 0,nil", board, n, err)
		}
	}
}
