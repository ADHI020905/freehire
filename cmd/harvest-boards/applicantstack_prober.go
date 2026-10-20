package main

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

// applicantstackProber validates an ApplicantStack board "<tenant>" with one request: its
// listing page ("<tenant>.applicantstack.com/x/openings"), whose pager states the board's whole
// open count ("Showing 1 - 100 of 282") even when the listing spans several pages. An unknown
// tenant redirects to the vendor's marketing site, which carries no openings table, and a live
// board with no openings renders the table without a pager — both read as zero. The listing
// names no employer, so the company comes from the seed.
type applicantstackProber struct{}

// applicantstackPagerTotal reads the total from the pager's "Showing 1 - 100 of 282".
var applicantstackPagerTotal = regexp.MustCompile(`\bof\s+(\d+)`)

func (applicantstackProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://%s.applicantstack.com/x/openings", strings.ToLower(slug)))
	if err != nil {
		return "", 0, nil
	}
	var pager *html.Node
	hasTable := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if sources.Attr(n, "id") == "data-table" {
				hasTable = true
			}
			if pager == nil && sources.HasClass(n, "tablepager") {
				pager = n
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	if !hasTable || pager == nil {
		return "", 0, nil
	}
	var text strings.Builder
	for t := pager.FirstChild; t != nil; t = t.NextSibling {
		if t.Type == html.TextNode {
			text.WriteString(t.Data)
		}
	}
	m := applicantstackPagerTotal.FindStringSubmatch(text.String())
	if m == nil {
		return "", 0, nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return "", 0, nil
	}
	return "", n, nil
}
