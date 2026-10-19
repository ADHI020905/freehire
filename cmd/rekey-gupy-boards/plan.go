package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// careerPageHost captures the career-page subdomain of a stored Gupy posting URL. Both shapes
// the old feed handed out carry it: https://caju.gupy.io/jobs/123 and the portal's
// https://caju.gupy.io/job/<base64>?jobBoardSource=gupy_portal. Gupy subdomains may carry an
// underscore (bbc_digital), which no DNS label rule would admit but Gupy does.
var careerPageHost = regexp.MustCompile(`(?i)^https?://([a-z0-9_-]+)\.gupy\.io/`)

// careerPage returns the lower-cased career-page subdomain a posting URL lives on.
func careerPage(url string) (string, bool) {
	m := careerPageHost.FindStringSubmatch(url)
	if m == nil {
		return "", false
	}
	return strings.ToLower(m[1]), true
}

// isCompanyIDBoard reports whether a gupy board is keyed the old way, by numeric companyId.
// A board already moved is retired, so a re-run does not see it again. Gupy does not forbid
// an all-digit subdomain, which is why planBoard also reports whether a board is its own
// career page — such a board is already keyed the new way and must be left alone.
func isCompanyIDBoard(board string) bool {
	_, err := strconv.ParseUint(board, 10, 64)
	return err == nil
}

// boardPlan is what moving one companyId board to its career pages means.
type boardPlan struct {
	// pages are the career-page subdomains the board's postings live on, most postings first.
	pages []string
	// ids/externalIDs are the parallel RekeyJobExternalIDs arguments: each posting's new
	// "<subdomain>:<job id>" key.
	ids         []int64
	externalIDs []string
	// unplaced counts postings whose URL names no career page; they keep their old key.
	unplaced int
	// alreadyMoved is set when one of the pages IS the board: an all-digit subdomain board
	// written by a previous run, not a companyId board.
	alreadyMoved bool
}

// planBoard derives a companyId board's plan from its stored postings. The job id is the part
// of the old key after the board namespace, so the posting keeps its identity under the new one.
func planBoard(board string, rows []db.ListBoardJobURLsRow) boardPlan {
	var p boardPlan
	prefix := externalid.Namespace(board, "")
	counts := map[string]int{}
	for _, r := range rows {
		page, ok := careerPage(r.URL)
		jobID, hasPrefix := strings.CutPrefix(r.ExternalID, prefix)
		if !ok || !hasPrefix || jobID == "" {
			p.unplaced++
			continue
		}
		counts[page]++
		p.ids = append(p.ids, r.ID)
		p.externalIDs = append(p.externalIDs, externalid.Namespace(page, jobID))
	}
	for page := range counts {
		p.pages = append(p.pages, page)
		if page == strings.ToLower(board) {
			p.alreadyMoved = true
		}
	}
	sort.Slice(p.pages, func(i, j int) bool {
		if counts[p.pages[i]] != counts[p.pages[j]] {
			return counts[p.pages[i]] > counts[p.pages[j]]
		}
		return p.pages[i] < p.pages[j]
	})
	return p
}
