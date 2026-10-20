package main

import (
	"context"
	"testing"
)

func TestAppliTrackProbe(t *testing.T) {
	p := applitrackProber{}
	// The pages are JavaScript carrying escaped markup, which is why the prober reads the raw
	// body. Only the "All Vacancies" listing is routed, so any other listing request fails.
	// One posting is linked twice (its title control and a plain permalink), so counting
	// distinct ids is what reports postings rather than links.
	listing := `document.write('<span class=\'title\'>Network Technician <a href=\'javascript:updateHrefFromCurrentWindowLocation(` +
		`"ApplitrackHardcodedURL%3F1%3D1%26AppliTrackJobId%3D739%26AppliTrackLayoutMode%3Ddetail")\'>view</a></span>` +
		`<a href=\'view.asp?AppliTrackJobId=739\'>Network Technician</a>` +
		`<span class=\'title\'>Help Desk Technician <a href=\'javascript:updateHrefFromCurrentWindowLocation(` +
		`"ApplitrackHardcodedURL%3F1%3D1%26AppliTrackJobId%3D812%26AppliTrackLayoutMode%3Ddetail")\'>view</a></span>')`
	base := "https://www.applitrack.com"
	all := "/onlineapp/jobpostings/Output.asp?AppliTrackLayoutMode=condensed&all=1"
	getter := fakeGetter{
		base + "/acme" + all: listing,
		base + "/acme/onlineapp/jobpostings/view.asp": `<html><head><title>Online Employment Application</title></head>` +
			`<body><h1 style="display: none;">Open Positions for Acme County Public Schools</h1></body></html>`,
		// A district with nothing open.
		base + "/empty" + all: `document.write('<div id=\'AppliTrackListContent\'></div>')`,
	}

	if name, n, err := p.probe(context.Background(), getter, "acme"); err != nil || n != 2 ||
		name != "Acme County Public Schools" {
		t.Errorf("live: got (%q,%d,%v), want (\"Acme County Public Schools\",2,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "empty"); err != nil || n != 0 {
		t.Errorf("empty board: got n=%d err=%v, want 0,nil", n, err)
	}
	// A tenant that does not exist answers 404; that is a skip, never a fatal error — and the
	// name request is never spent on it.
	if _, n, err := p.probe(context.Background(), getter, "gone"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}

func TestAppliTrackProberDedupKey(t *testing.T) {
	// The tenant path segment is case-insensitive, so a candidate spelled in another case than a
	// board already committed is the same board.
	if got := (applitrackProber{}).dedupKey("Saskatoon"); got != "saskatoon" {
		t.Errorf("dedupKey(%q) = %q, want %q", "Saskatoon", got, "saskatoon")
	}
}

func TestAppliTrackProberRegistered(t *testing.T) {
	p, ok := proberFor("applitrack")
	if !ok {
		t.Fatal(`proberFor("applitrack") not found`)
	}
	// The bespoke prober must win: the adapter fallback would run a whole crawl per candidate,
	// hydrating every posting from a page of its own.
	if _, fellBack := p.(adapterProber); fellBack {
		t.Errorf("applitrack resolved to the adapter fallback, got %T", p)
	}
}
