package main

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
)

// The AppOne endpoints mirror internal/ingest/sources/appone.go's, which are unexported.
const (
	apponePortalAPI  = "https://jobs.appone.com/api/portal/v1/companyjobposts/"
	apponePostingAPI = "https://apply.appone.com/api/apply/v2/jobposting/"
)

// apponeProber validates an AppOne portal slug with the same one request the adapter lists it
// by, which also names the employer — so the corroboration gate has a name to stand on.
//
// Its seeds may be POSTING links rather than slugs: an apply.appone.com/job/<id> URL names no
// portal, and the only place the portal appears is the posting's own API record
// (jobPortalUrl), so such a seed is resolved with one request before it can be a candidate at
// all (see resolveSeed).
type apponeProber struct{}

// apponePostingPath is a posting link's path: /job/<24-hex id>.
var apponePostingPath = regexp.MustCompile(`^/job/([0-9a-f]{24})/?$`)

// dedupKey folds a portal slug to lower case: the platform answers "QCRP" and "qcrp" with the
// same portal (verified live 2026-10-20).
func (apponeProber) dedupKey(board string) string { return strings.ToLower(board) }

func (apponeProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	var resp struct {
		CompanyName string `json:"companyName"`
		JobPosts    []struct {
			ID string `json:"jobPostId"`
		} `json:"jobPosts"`
	}
	if err := c.GetJSON(ctx, apponePortalAPI+url.PathEscape(slug), &resp); err != nil {
		// An unknown portal is a 404 and simply not a board. Anything else — the platform's
		// edge answers a stray 403 to a share of requests (see sources/appone.go) — is
		// surfaced, so a refused probe is counted rather than mistaken for a dead board.
		if boardGone(err) {
			return "", 0, nil
		}
		return "", 0, err
	}
	return strings.TrimSpace(resp.CompanyName), len(resp.JobPosts), nil
}

// resolveSeed turns a seed token into a portal slug. A posting link costs one request for the
// portal its record names; a portal link is read offline; a bare token is taken as a slug. It
// returns "" for a token that names no portal — a closed posting (404), a posting published
// without a portal (jobPortalUrl empty, 2 of 40 sampled), or a URL on any other host, which
// includes the legacy AppOne product's MainInfoReq.asp links: they carry a search-screen id no
// public listing is keyed by.
func (apponeProber) resolveSeed(ctx context.Context, c httpClient, token string) (string, error) {
	token = strings.TrimSpace(token)
	u, err := url.Parse(token)
	if err != nil || u.Host == "" {
		return token, nil // a bare slug
	}
	if strings.EqualFold(u.Hostname(), "apply.appone.com") {
		m := apponePostingPath.FindStringSubmatch(u.Path)
		if m == nil {
			return "", nil
		}
		var rec struct {
			JobPortalURL string `json:"jobPortalUrl"`
		}
		if err := c.GetJSON(ctx, apponePostingAPI+m[1], &rec); err != nil {
			if boardGone(err) {
				return "", nil
			}
			return "", err
		}
		token = rec.JobPortalURL
	}
	if src, board, _, ok := atsboard.Recognize(token); ok && src == "appone" {
		return board, nil
	}
	return "", nil
}
