// Command rekey-gupy-boards is a one-off: moves every gupy board from its numeric companyId
// to the career-page subdomains its postings live on, and moves the stored postings with it.
//
// WHY. Gupy's companyId-keyed listing (employability-portal.gupy.io/api/v1/jobs) began
// answering an nginx 404 to every query on 2026-09-30. All 1,511 gupy boards failed every
// crawl from then on, and since a failing board is never swept, 73,156 postings stayed open
// with last_seen_at frozen at that day (measured 2026-10-19). The listing that still works is
// the one every career page renders from — job-publication/public/jobs?subdomain=<page> — and
// nothing public lists by companyId any more, so the board key has to change. One company
// commonly owns several career pages (an internal-mobility page, a confidential one): on prod
// 595 boards mapped to one page and 916 to two or more.
//
// WHERE THE MAPPING COMES FROM. Every stored posting's URL names the career page it was
// published on (https://<page>.gupy.io/job/...), so the companyId -> pages mapping is read
// from the catalogue itself.
//
// TWO PASSES, and the split is load-bearing.
//
// Pass 1, per companyId board in ONE transaction: each posting's external_id
// "<companyId>:<job id>" becomes "<page>:<job id>" (the row keeps its id, so saves, tracking,
// enrichment and views follow it, and the first crawl of the page UPDATES the posting instead
// of inserting a duplicate beside it), and the companyId board is retired. Pass 1 creates
// nothing a crawler can see.
//
// Pass 2, only once EVERY companyId board has moved: one 'pending' board per page namespace
// the stored postings now carry, through boardcatalog's Inserter. Adding a page board before
// all its postings were moved would be a race, because one page can be shared by two
// companyIds (datalawyer: 41023 and 68685): a crawl of the new board between the two moves
// writes fresh "<page>:<id>" rows, and the second move then collides on (source, external_id)
// forever. So a failed board in pass 1 skips pass 2 entirely, and the run exits non-zero
// naming every board that failed. Pass 2 reads its pages back from the data rather than from
// pass 1's plan, so a re-run after a fix picks up exactly what is still missing.
//
// WHAT CLOSES THE STALE POSTINGS. gupy is now a fullBoardListing adapter, so the first
// successful crawl of a page board closes, through the ordinary board-scoped sweep, every
// posting the page no longer lists — all last seen 2026-09-30, far past the 48h grace. Two
// kinds of page that sweep can never reach are closed here, in pass 2, on Gupy's own answer:
//   - a page that lists nothing ({"data":[]}): a zero-yield crawl licenses no sweep, and the
//     empty-feed pass in close-chronic-boards both refuses a board with no recorded yield and
//     runs report-only on prod (its unit passes --apply, never --apply-empty-feed). Its open
//     postings close as 'feed_empty'; the board is still added, the page being live.
//   - a page Gupy answers 404 "Career Page not Found" for: its open postings close as
//     'board_unreachable' and no board is added — one would fail every crawl.
//
// Both closes queue search_delete_outbox in the same statement. A page whose probe fails any
// other way is added as a board and closes nothing.
//
// Reports by default; --apply writes. Idempotent: a moved board is retired and a page with a
// live board is skipped. Needs only DATABASE_URL, and pass 2 reaches private-api.gupy.io.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/worker"
)

const provider = "gupy"

// gupyListURL lists one career page's published postings (mirrors sources.gupyJobsURL, which
// is unexported).
const gupyListURL = "https://private-api.gupy.io/job-publication/public/jobs"

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "actually write; without it the run only reports")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	if err := rekey(ctx, pool, gupyLister{http: sources.NewClient()}, *apply); err != nil {
		log.Printf("rekey-gupy-boards: %v", err)
		return 1
	}
	return 0
}

// pageLister asks Gupy how many postings a career page publishes. An error wrapping a
// *sources.StatusError with code 404 is Gupy's "Career Page not Found".
type pageLister interface {
	listed(ctx context.Context, page string) (int, error)
}

type gupyLister struct{ http sources.JSONGetter }

func (g gupyLister) listed(ctx context.Context, page string) (int, error) {
	var resp struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := g.http.GetJSON(ctx, gupyListURL+"?subdomain="+url.QueryEscape(page), &resp); err != nil {
		return 0, err
	}
	return len(resp.Data), nil
}

func rekey(ctx context.Context, pool *pgxpool.Pool, lister pageLister, apply bool) error {
	failed, err := movePostings(ctx, pool, apply)
	if err != nil {
		return err
	}
	if !apply {
		log.Print("dry run, nothing written. Re-run with --apply to move the boards.")
		return nil
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d boards failed to move, page boards NOT added (fix and re-run): %s",
			len(failed), strings.Join(failed, "; "))
	}
	return addPageBoards(ctx, pool, lister)
}

// movePostings is pass 1. It returns the boards whose move failed; a failure is logged and the
// run continues with the next board, so one bad board does not hide the state of the rest.
func movePostings(ctx context.Context, pool *pgxpool.Pool, apply bool) (failed []string, err error) {
	q := db.New(pool)
	boards, err := boardcatalog.NewQueriesRepository(q).ListActiveForProvider(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("list boards: %w", err)
	}

	var seen, moved, postings, unplaced int
	var noPages []string
	for _, b := range boards {
		if !isCompanyIDBoard(b.Board) {
			continue
		}
		rows, err := q.ListBoardJobURLs(ctx, db.ListBoardJobURLsParams{
			Source: provider, BoardPattern: externalid.BoardPattern(b.Board),
		})
		if err != nil {
			return nil, fmt.Errorf("board %s: list postings: %w", b.Board, err)
		}
		plan := planBoard(b.Board, rows)
		if plan.alreadyMoved {
			continue
		}
		seen++
		postings += len(plan.ids)
		unplaced += plan.unplaced
		if len(plan.pages) == 0 {
			noPages = append(noPages, fmt.Sprintf("%s (%s)", b.Board, b.Company))
		}
		log.Printf("board %s (%s): %d postings -> pages [%s], %d unplaced",
			b.Board, b.Company, len(plan.ids), strings.Join(plan.pages, " "), plan.unplaced)
		if !apply {
			continue
		}
		if err := moveBoard(ctx, pool, b, plan); err != nil {
			log.Printf("board %s: FAILED: %v", b.Board, err)
			failed = append(failed, fmt.Sprintf("%s: %v", b.Board, err))
			continue
		}
		moved++
	}

	log.Printf("pass 1: %d companyId boards, %d moved, %d failed; %d postings re-keyed, %d naming no page",
		seen, moved, len(failed), postings, unplaced)
	if len(noPages) > 0 {
		log.Printf("boards with no career page in the catalogue (retired, re-add by page if wanted): %s",
			strings.Join(noPages, ", "))
	}
	return failed, nil
}

// moveBoard applies one board's plan in a single transaction: re-key its postings and retire
// the companyId board.
func moveBoard(ctx context.Context, pool *pgxpool.Pool, b boardcatalog.Board, plan boardPlan) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if len(plan.ids) > 0 {
			if _, err := q.RekeyJobExternalIDs(ctx, db.RekeyJobExternalIDsParams{
				Source: provider, Ids: plan.ids, ExternalIds: plan.externalIDs,
			}); err != nil {
				return fmt.Errorf("re-key postings: %w", err)
			}
		}
		if _, err := boardcatalog.NewQueriesRepository(q).Retire(ctx, provider, b.Board, b.Region); err != nil {
			return fmt.Errorf("retire: %w", err)
		}
		return nil
	})
}

// addPageBoards is pass 2: a board for every page namespace the stored postings carry that has
// no live board yet, with the stale postings of an empty or vanished page closed on Gupy's own
// answer (see the package comment).
func addPageBoards(ctx context.Context, pool *pgxpool.Pool, lister pageLister) error {
	q := db.New(pool)
	repo := boardcatalog.NewQueriesRepository(q)
	live, err := repo.ListActiveForProvider(ctx, provider)
	if err != nil {
		return fmt.Errorf("list boards: %w", err)
	}
	hasBoard := map[string]bool{}
	for _, b := range live {
		hasBoard[strings.ToLower(b.Board)] = true
	}
	namespaces, err := q.ListJobNamespacesForSource(ctx, provider)
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}

	inserter := boardcatalog.NewInserter(repo, sources.Taxonomy())
	var added, empty, gone, unprobed int
	var closedEmpty, closedGone int64
	var failed []string
	for _, ns := range namespaces {
		page := ns.Namespace
		// An all-digit namespace is the old companyId key: postings no URL placed.
		if page == "" || isCompanyIDBoard(page) || hasBoard[strings.ToLower(page)] {
			continue
		}
		n, probeErr := lister.listed(ctx, page)
		var se *sources.StatusError
		switch {
		case errors.As(probeErr, &se) && se.Code == http.StatusNotFound:
			closed, err := closePage(ctx, q, page, "board_unreachable")
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", page, err))
				continue
			}
			gone++
			closedGone += closed
			log.Printf("page %s: career page not found, %d open postings closed, no board added", page, closed)
			continue
		case probeErr != nil:
			unprobed++
			log.Printf("page %s: probe failed (%v), adding the board and closing nothing", page, probeErr)
		case n == 0:
			closed, err := closePage(ctx, q, page, "feed_empty")
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", page, err))
				continue
			}
			empty++
			closedEmpty += closed
			log.Printf("page %s: publishes nothing, %d open postings closed", page, closed)
		}
		company := ns.Company
		if company == "" {
			company = page
		}
		nb, err := inserter.Insert(ctx, boardcatalog.InsertInput{
			Provider: provider, Board: page, Company: company,
		}, boardcatalog.StatusPending)
		switch {
		case errors.Is(err, boardcatalog.ErrDuplicateBoard):
		case err != nil:
			failed = append(failed, fmt.Sprintf("%s: add board: %v", page, err))
		case nb.Status == boardcatalog.StatusRejected:
			failed = append(failed, fmt.Sprintf("%s: add board: rejected: %s", page, nb.RejectedReason))
		default:
			added++
		}
	}

	log.Printf("pass 2: %d page boards added (%d of them empty, %d postings closed feed_empty; %d unprobed); "+
		"%d pages gone, %d postings closed board_unreachable", added, empty, closedEmpty, unprobed, gone, closedGone)
	if len(failed) > 0 {
		return fmt.Errorf("%d pages failed (re-run to retry): %s", len(failed), strings.Join(failed, "; "))
	}
	return nil
}

func closePage(ctx context.Context, q *db.Queries, page, reason string) (int64, error) {
	closed, err := q.CloseBoardJobsWithReason(ctx, db.CloseBoardJobsWithReasonParams{
		ClosedReason: reason, Source: provider, BoardPattern: externalid.BoardPattern(page),
	})
	if err != nil {
		return 0, fmt.Errorf("close %s: %w", reason, err)
	}
	return closed, nil
}
