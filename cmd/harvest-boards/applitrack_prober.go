package main

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// applitrackProber validates an AppliTrack tenant ("<board>") by counting the postings on the
// board's "All Vacancies" listing — the same listing the adapter crawls.
//
// It mirrors the adapter's URL shape, which is unexported (this tool lives outside that
// package). It costs two requests per live candidate — the adapter fallback would instead run a
// whole crawl, hydrating a posting page each.
//
// Everything is read off the RAW body rather than a DOM: the platform document.writes its markup
// as escaped JavaScript, and the thing wanted here — a posting id in a link — is ASCII and
// matchable where it sits, which needs neither the unescaping nor the parse the adapter does.
type applitrackProber struct{}

const applitrackProbeBase = "https://www.applitrack.com"

// applitrackProbeID matches a posting link's id in the raw listing body. The listing links a
// posting through a javascript: call whose argument is percent-encoded, so the parameter
// separator arrives as "%3D" there and as a plain "=" elsewhere on the page; both are accepted,
// mirroring the adapter's own pattern.
var applitrackProbeID = regexp.MustCompile(`(?i)AppliTrackJobId(?:=|%3D)(\d+)`)

// applitrackProbeName captures the tenant name the career site renders in its heading. This is
// AppliTrack's own tenant field rather than free text a district typed into a page — every site
// sampled renders the same sentence — so it is worth reporting: it gates a candidate against what
// the seed claimed and, where the seed claimed nothing, it is what names the board in the file.
var applitrackProbeName = regexp.MustCompile(`(?is)<h1[^>]*>\s*Open Positions for\s+(.*?)\s*</h1>`)

func (applitrackProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	// A tenant that does not exist answers 404 and the client surfaces that as an error; for
	// harvest that is simply "not a live board" — skip silently, do not propagate.
	listing, err := c.GetText(ctx, applitrackProbeURL(board))
	if err != nil {
		return "", 0, nil
	}
	ids := map[string]bool{}
	for _, id := range applitrackProbeID.FindAllStringSubmatch(listing, -1) {
		ids[id[1]] = true
	}
	if len(ids) == 0 {
		return "", 0, nil
	}
	// The name lives on the shell page, not on the payload endpoint above, so it is one more
	// request — spent only once the board is known to carry postings.
	return applitrackName(ctx, c, board), len(ids), nil
}

// applitrackProbeURL is the board's condensed "All Vacancies" listing.
func applitrackProbeURL(board string) string {
	return fmt.Sprintf("%s/%s/onlineapp/jobpostings/Output.asp?AppliTrackLayoutMode=condensed&all=1",
		applitrackProbeBase, url.PathEscape(board))
}

// dedupKey folds a tenant to lower case. The path segment is case-insensitive (Saskatoon,
// SASKATOON and saskatoon all serve the same board, confirmed live), so a candidate spelled in
// another case than one the board file already holds is the same board — without this it probes
// live, passes the gate against its own district, and is appended a second time.
func (applitrackProber) dedupKey(board string) string { return strings.ToLower(board) }

// applitrackName reads the district name off the career site's heading, or "" when the page
// cannot be read or does not carry one — an absent name simply leaves the seed's own to label
// the board.
func applitrackName(ctx context.Context, c httpClient, board string) string {
	page, err := c.GetText(ctx, fmt.Sprintf("%s/%s/onlineapp/jobpostings/view.asp",
		applitrackProbeBase, url.PathEscape(board)))
	if err != nil {
		return ""
	}
	m := applitrackProbeName.FindStringSubmatch(page)
	if m == nil {
		return ""
	}
	return strings.Join(strings.Fields(m[1]), " ")
}
