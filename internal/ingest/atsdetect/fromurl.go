package atsdetect

import (
	"net/url"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
)

// FromURL classifies a single outbound job-application URL into the (provider, board)
// pair one of our source adapters can crawl, returning ok=false when the URL is not a
// supported ATS or its shape does not expose the exact board id the adapter expects
// (e.g. an ADP vanity path, a Join job link, or a Workable per-job shortlink). Unlike
// Detect, which scans arbitrary HTML, this parses one known URL — the shape an
// aggregator's apply link gives us directly.
//
// The shared table answers first. internal/ingest/atsboard is the ONE definition of "which board
// does this URL address", read by the contribution flow, link resolution and boardresolve;
// this package used to carry its own copy of eleven of those hosts, and the two had already
// drifted in both directions — atsdetect read a SmartRecruiters portal segment as the board,
// while atsboard read a lowercase Workday locale and a per-job "/job" path as career sites.
// Delegating also means an ATS added to atsboard is immediately visible to the harvest tools,
// which was the standing cost of the split.
//
// What stays here are the two shapes atsboard has not taken in: Oracle and Taleo Enterprise.
// They are harvest-only, and the harvest path validates every candidate against the platform's
// API before committing it. The split used to guard a money-affecting accept-set —
// internal/ingest/contribution paid for onboarded boards — and once that reward was retired the
// shapes it kept out began moving into the shared table: iCIMS first, then NEOGOV, Paycom and
// Comeet, each deleted here when it did — see TestLocalShapesStayOutsideTheSharedTable.
func FromURL(rawurl string) (provider, board string, ok bool) {
	if src, brd, _, found := atsboard.Recognize(rawurl); found {
		return src, brd, true
	}

	u, err := url.Parse(rawurl)
	if err != nil {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", "", false
	}
	segs := pathSegments(u.Path)

	switch {
	case strings.HasSuffix(host, ".oraclecloud.com"):
		if site := segAfter(segs, "sites"); site != "" {
			return "oracle", host + "/" + site, true
		}
	case strings.HasSuffix(host, ".taleo.net"):
		// board is the tenant host + the public careersection number, e.g.
		// "valero.taleo.net/2" from /careersection/2/jobsearch.ftl.
		if section := segAfter(segs, "careersection"); section != "" {
			return "taleo", host + "/" + section, true
		}
	}
	return "", "", false
}

// pathSegments splits a URL path into its non-empty segments.
func pathSegments(p string) []string {
	var out []string
	for _, s := range strings.Split(p, "/") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// segAfter returns the path segment following the first occurrence of key, or "" when
// key is absent or terminal.
func segAfter(segs []string, key string) string {
	for i, s := range segs {
		if s == key && i+1 < len(segs) {
			return segs[i+1]
		}
	}
	return ""
}
