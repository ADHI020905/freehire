//go:build integration

// Integration test for RenameSlugBoardCompany: cmd/backfill-company-names writes a resolved
// display name back to the board, because the adapter copies boards.company onto every
// posting and a rename applied to jobs alone was undone by the next crawl.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

// TestRenameSlugBoardCompany pins the query's reach: every region and status of the
// matching board, the board id compared case-insensitively — and nothing whose company is
// no longer the slug being replaced, so a curator's name survives a later run.
func TestRenameSlugBoardCompany(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	seed := func(provider, board, region, company, status string) int64 {
		t.Helper()
		b, err := q.InsertBoard(ctx, InsertBoardParams{
			Provider: provider, Board: board, Region: region, Company: company,
			Tenants: []byte("{}"), Status: status, Surface: "curator",
		})
		if err != nil {
			t.Fatalf("seed %s/%s/%s: %v", provider, board, region, err)
		}
		return b.ID
	}
	active := seed("ashby", "a-place-for-mom", "", "a-place-for-mom", "active")
	retiredOtherRegion := seed("ashby", "A-Place-For-Mom", "us", "a-place-for-mom", "retired")
	curated := seed("ashby", "a-place-for-mom", "eu", "APFM Europe", "active")
	otherProvider := seed("lever", "a-place-for-mom", "", "a-place-for-mom", "active")

	n, err := q.RenameSlugBoardCompany(ctx, RenameSlugBoardCompanyParams{
		Name: "A Place for Mom", Provider: "ashby", Board: "a-place-for-mom", OldName: "a-place-for-mom",
	})
	if err != nil {
		t.Fatalf("RenameSlugBoardCompany: %v", err)
	}
	if n != 2 {
		t.Errorf("rows = %d, want 2", n)
	}

	for id, want := range map[int64]string{
		active:             "A Place for Mom",
		retiredOtherRegion: "A Place for Mom",
		curated:            "APFM Europe",
		otherProvider:      "a-place-for-mom",
	} {
		var got string
		if err := pool.QueryRow(ctx, "SELECT company FROM boards WHERE id = $1", id).Scan(&got); err != nil {
			t.Fatalf("read board %d: %v", id, err)
		}
		if got != want {
			t.Errorf("board %d company = %q, want %q", id, got, want)
		}
	}
}
