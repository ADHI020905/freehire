//go:build integration

// Integration tests for SetJobGeography, the write behind the geography backfill
// (cmd/backfill-remote-region-restriction).
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"testing"
)

// jobderive.Derive returns a nil slice for a facet it resolves nothing for, and pgx
// encodes nil as NULL — which the NOT NULL columns refuse. The write must store the
// empty array instead, and a repeat of it must be a no-op like any other.
func TestSetJobGeographyStoresNilAsEmpty(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:geo", "Backend Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	params := SetJobGeographyParams{ID: job.ID, Countries: nil, Regions: []string{"north_america"}}
	if _, err := q.SetJobGeography(ctx, params); err != nil {
		t.Fatalf("write with nil countries: %v", err)
	}
	n, err := q.SetJobGeography(ctx, params)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if n != 0 {
		t.Fatalf("second write affected %d rows, want 0 — nil and '{}' compared as different", n)
	}

	stored, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(stored.Countries) != 0 || !slices.Equal(stored.Regions, []string{"north_america"}) {
		t.Fatalf("countries=%v regions=%v, want [] [north_america]", stored.Countries, stored.Regions)
	}
}
