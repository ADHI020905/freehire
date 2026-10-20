package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestHireClickProbe(t *testing.T) {
	// The list endpoint answers a JSON STRING holding the document, as the live one does.
	inner := `{"Settings":{"GenericJobTitle":"General Application"},"JobList":[` +
		`{"JobTitle":"Welder","JobURL":"https://acme.hireclick.com/jb/welder/view/100"},` +
		`{"JobTitle":"Welder","JobURL":"https://acme.hireclick.com/jb/welder/view/100"},` +
		`{"JobTitle":"Driver","JobURL":"https://acme.hireclick.com/jb/driver/view/200"}]}`
	encoded, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	getter := fakeGetter{
		"https://acme.hireclick.com/jobboard/jobboard.aspx":                                      `<html><body><input type="hidden" id="hdnOrgGuid" value="ABC-123" /></body></html>`,
		"https://acme.hireclick.com/api/Controllers/JobBoard/GetSettingsAndJobs?orgGUID=ABC-123": string(encoded),
		// An unknown tenant lands on the maintenance page, which carries no organisation id.
		"https://gone.hireclick.com/jobboard/jobboard.aspx": `<html><body><h1>Maintenance</h1></body></html>`,
	}
	// The prober reads over its own client, so the harvest's (nil here) is never touched.
	p := hireclickProber{client: getter}
	// live: two DISTINCT postings (the duplicate counts once), no company name.
	if name, n, err := p.probe(context.Background(), nil, "acme"); err != nil || name != "" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",2,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), nil, "gone"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
	if _, n, err := p.probe(context.Background(), nil, "unreachable"); err != nil || n != 0 {
		t.Errorf("unreachable: got n=%d err=%v, want 0,nil", n, err)
	}
}
