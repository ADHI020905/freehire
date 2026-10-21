//go:build integration

// Integration tests for the learned-ATS-domain promotion gate. Promotion is decided in
// SQL (PromotedDomains), so the precision bar can only be verified against a real
// Postgres. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"testing"
)

func TestPromotedDomainsRequiresPrecision(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	observe := func(domain string, n int) {
		t.Helper()
		for range n {
			if _, err := q.ObserveLearnedDomain(ctx, domain); err != nil {
				t.Fatalf("observe %s: %v", domain, err)
			}
		}
	}
	miss := func(host string, n int) {
		t.Helper()
		for range n {
			if err := q.MissLearnedDomain(ctx, host); err != nil {
				t.Fatalf("miss %s: %v", host, err)
			}
		}
	}

	observe("niche.example", 3)
	observe("noisy.example", 3)
	// From a subdomain: the Gmail query's from:noisy.example fetches it, so its
	// non-job mail counts against noisy.example.
	miss("e.noisy.example", 2)
	// A sender the cache has never learned stays out of the table entirely.
	miss("newsletter.example", 5)

	got, err := q.PromotedDomains(ctx, PromotedDomainsParams{Threshold: 3, MaxOtherShare: 1.0 / 3})
	if err != nil {
		t.Fatalf("PromotedDomains: %v", err)
	}
	if !slices.Equal(got, []string{"niche.example"}) {
		t.Errorf("promoted = %v, want [niche.example] (noisy.example is 2/5 non-job mail)", got)
	}

	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM learned_ats_domains WHERE domain = 'newsletter.example'`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Errorf("a miss on an unlearned domain created %d row(s), want 0", rows)
	}

	// The bar is inclusive: 4 hits and 2 misses is exactly a third.
	observe("edge.example", 4)
	miss("edge.example", 2)
	got, err = q.PromotedDomains(ctx, PromotedDomainsParams{Threshold: 3, MaxOtherShare: 1.0 / 3})
	if err != nil {
		t.Fatalf("PromotedDomains: %v", err)
	}
	if !slices.Contains(got, "edge.example") {
		t.Errorf("promoted = %v, want edge.example (exactly a third is still within the bar)", got)
	}
}
