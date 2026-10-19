//go:build integration

// Run with: go test -tags=integration ./cmd/rekey-gupy-boards/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package main

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// fakeLister answers a page's published-posting count; a page absent from it is Gupy's 404
// "Career Page not Found".
type fakeLister map[string]int

func (f fakeLister) listed(_ context.Context, page string) (int, error) {
	n, ok := f[page]
	if !ok {
		return 0, &sources.StatusError{Method: http.MethodGet, Code: http.StatusNotFound}
	}
	return n, nil
}

func seedGupyJob(t *testing.T, pool *pgxpool.Pool, externalID, url, company string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('gupy', $1, $2, 'A job', $3, 'job-' || replace($1, ':', '-')) RETURNING id`,
		externalID, url, company).Scan(&id); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return id
}

type jobState struct {
	externalID, closedReason string
	queuedForDelete          bool
}

func stateOf(t *testing.T, pool *pgxpool.Pool, id int64) jobState {
	t.Helper()
	var s jobState
	if err := pool.QueryRow(context.Background(),
		`SELECT external_id, COALESCE(closed_reason, ''),
		        EXISTS (SELECT 1 FROM search_delete_outbox o WHERE o.job_id = jobs.id)
		 FROM jobs WHERE id = $1`, id).Scan(&s.externalID, &s.closedReason, &s.queuedForDelete); err != nil {
		t.Fatalf("read job %d: %v", id, err)
	}
	return s
}

func liveBoards(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	rows, err := boardcatalog.NewQueriesRepository(db.New(pool)).ListActiveForProvider(context.Background(), provider)
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	out := map[string]string{}
	for _, b := range rows {
		out[b.Board] = b.Company
	}
	return out
}

func seedBoards(t *testing.T, pool *pgxpool.Pool, boards ...boardcatalog.InsertInput) {
	t.Helper()
	ins := boardcatalog.NewInserter(boardcatalog.NewQueriesRepository(db.New(pool)), sources.Taxonomy())
	for _, in := range boards {
		in.Provider = provider
		if _, err := ins.Insert(context.Background(), in, boardcatalog.StatusActive); err != nil {
			t.Fatalf("seed board %s: %v", in.Board, err)
		}
	}
}

func TestRekeyMovesBoardsAndPostingsToCareerPages(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	seedBoards(t, pool,
		boardcatalog.InsertInput{Board: "48481", Company: "Órigo Energia"},
		boardcatalog.InsertInput{Board: "137", Company: "Grupo Casas Bahia"},
		boardcatalog.InsertInput{Board: "999", Company: "No Postings"},
		boardcatalog.InsertInput{Board: "caju", Company: "Caju"}, // already a page board
	)
	origo := seedGupyJob(t, pool, "48481:100", "https://origoenergia.gupy.io/job/abc?jobBoardSource=gupy_portal", "Órigo Energia")
	interno := seedGupyJob(t, pool, "48481:101", "https://origoenergiainterno.gupy.io/jobs/101", "Órigo Energia")
	// A page two companyId boards both published on.
	shared1 := seedGupyJob(t, pool, "48481:102", "https://datalawyer.gupy.io/jobs/102", "Órigo Energia")
	shared2 := seedGupyJob(t, pool, "137:200", "https://datalawyer.gupy.io/jobs/200", "Grupo Casas Bahia")
	gone := seedGupyJob(t, pool, "137:201", "https://lojasgrupocasasbahia.gupy.io/jobs/201", "Grupo Casas Bahia")
	// A board whose id is a prefix of another's must not claim its postings.
	other := seedGupyJob(t, pool, "1370:300", "https://other.gupy.io/jobs/300", "Other")

	lister := fakeLister{"origoenergia": 3, "origoenergiainterno": 0, "datalawyer": 1}

	// A dry run writes nothing.
	if err := rekey(ctx, pool, lister, false); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if got := stateOf(t, pool, origo).externalID; got != "48481:100" {
		t.Fatalf("dry run re-keyed a posting: %q", got)
	}

	if err := rekey(ctx, pool, lister, true); err != nil {
		t.Fatalf("apply: %v", err)
	}

	for id, want := range map[int64]jobState{
		origo:   {externalID: "origoenergia:100"},
		shared1: {externalID: "datalawyer:102"},
		shared2: {externalID: "datalawyer:200"},
		// A live page that publishes nothing: no crawl could ever sweep it.
		interno: {externalID: "origoenergiainterno:101", closedReason: "feed_empty", queuedForDelete: true},
		// A page Gupy no longer has.
		gone:  {externalID: "lojasgrupocasasbahia:201", closedReason: "board_unreachable", queuedForDelete: true},
		other: {externalID: "1370:300"},
	} {
		if got := stateOf(t, pool, id); got != want {
			t.Errorf("job %d = %+v, want %+v", id, got, want)
		}
	}

	boards := liveBoards(t, pool)
	want := map[string]string{
		"origoenergia": "Órigo Energia", "origoenergiainterno": "Órigo Energia",
		"datalawyer": "Grupo Casas Bahia", // the company most of its postings name (a tie: the lesser)
		"caju":       "Caju",
	}
	keys := make([]string, 0, len(boards))
	for b := range boards {
		keys = append(keys, b)
	}
	sort.Strings(keys)
	if len(boards) != len(want) {
		t.Fatalf("live boards = %v, want %v", keys, want)
	}
	for b, company := range want {
		if boards[b] != company {
			t.Errorf("board %s company = %q, want %q", b, boards[b], company)
		}
	}

	// A re-run finds nothing left to move and changes nothing.
	if err := rekey(ctx, pool, lister, true); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if got := len(liveBoards(t, pool)); got != len(want) {
		t.Errorf("re-run changed the live boards: %d, want %d", got, len(want))
	}
}

// A board whose move fails must not stop the others, must not let any page board appear (a
// crawl of a shared page before every companyId moved is what causes the collision), and must
// make the run fail naming it. Re-running after the cause is gone finishes the job.
func TestRekeyContinuesPastAFailedBoardAndAddsNoPageBoards(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	seedBoards(t, pool,
		boardcatalog.InsertInput{Board: "41023", Company: "Avita"},
		boardcatalog.InsertInput{Board: "68685", Company: "Ativa"},
	)
	avita := seedGupyJob(t, pool, "41023:1", "https://datalawyer.gupy.io/jobs/1", "Avita")
	ativa := seedGupyJob(t, pool, "68685:2", "https://datalawyer.gupy.io/jobs/2", "Ativa")
	// The row a premature crawl of the shared page would have written.
	squatter := seedGupyJob(t, pool, "datalawyer:2", "https://datalawyer.gupy.io/jobs/2", "Ativa")
	lister := fakeLister{"datalawyer": 2}

	err := rekey(ctx, pool, lister, true)
	if err == nil || !strings.Contains(err.Error(), "68685") {
		t.Fatalf("rekey err = %v, want a failure naming board 68685", err)
	}
	if got := stateOf(t, pool, avita).externalID; got != "datalawyer:1" {
		t.Errorf("the healthy board did not move: %q", got)
	}
	if got := stateOf(t, pool, ativa).externalID; got != "68685:2" {
		t.Errorf("the failed board's move was not rolled back: %q", got)
	}
	boards := liveBoards(t, pool)
	if _, ok := boards["datalawyer"]; ok {
		t.Error("a page board was added although a board failed to move")
	}
	if _, ok := boards["68685"]; !ok {
		t.Error("the failed companyId board was retired")
	}

	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, squatter); err != nil {
		t.Fatalf("remove squatter: %v", err)
	}
	if err := rekey(ctx, pool, lister, true); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if got := stateOf(t, pool, ativa).externalID; got != "datalawyer:2" {
		t.Errorf("re-run did not move the board: %q", got)
	}
	if _, ok := liveBoards(t, pool)["datalawyer"]; !ok {
		t.Error("re-run did not add the page board")
	}
}
