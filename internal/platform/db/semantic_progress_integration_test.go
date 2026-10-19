//go:build integration

// Integration tests for the two counts behind the public embedding-progress page
// (openspec/changes/embedding-progress-page): coverage, measured by cmd/rollup-stats,
// and the claimable queue depth, sampled by cmd/server. Reuses the helpers in
// enrichment_integration_test.go and semantic_integration_test.go.
package db

import (
	"context"
	"testing"
)

func TestSemanticCoverageCounts(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	covered := insertJob(t, pool, "covered")
	setContentHash(t, pool, covered, "h1")
	setSemanticStamp(t, pool, covered, targetModel, "h1")

	never := insertJob(t, pool, "never-embedded")
	setContentHash(t, pool, never, "h1")

	staleHash := insertJob(t, pool, "stale-hash")
	setContentHash(t, pool, staleHash, "new")
	setSemanticStamp(t, pool, staleHash, targetModel, "old")

	staleModel := insertJob(t, pool, "stale-model")
	setContentHash(t, pool, staleModel, "h1")
	setSemanticStamp(t, pool, staleModel, "older-model", "h1")

	// A NULL content_hash matches a NULL stamped hash: the stamp copies whatever the
	// job held, and EnqueuePendingSemanticJobs uses IS DISTINCT FROM on purpose.
	nullHash := insertJob(t, pool, "null-hash")
	setSemanticStamp(t, pool, nullHash, targetModel, "")
	if _, err := pool.Exec(ctx, "UPDATE jobs SET content_hash = NULL, semantic_embedded_hash = NULL WHERE id = $1", nullHash); err != nil {
		t.Fatalf("null hashes: %v", err)
	}

	// Not eligible at all, embedded or not.
	closed := insertJob(t, pool, "closed")
	setContentHash(t, pool, closed, "h1")
	setSemanticStamp(t, pool, closed, targetModel, "h1")
	closeJob(t, pool, closed)

	// duplicate_of is derived from the three owned marker columns (migration 0115).
	dup := insertJob(t, pool, "duplicate")
	if _, err := pool.Exec(ctx, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", covered, dup); err != nil {
		t.Fatalf("mark duplicate: %v", err)
	}

	nonTech := insertJob(t, pool, "non-tech")
	f := false
	setIsTech(t, pool, nonTech, &f)

	unknownTech := insertJob(t, pool, "unknown-tech")
	setIsTech(t, pool, unknownTech, nil)

	// One user's pasted JD: embedded for them, never counted in a public figure.
	private := insertJob(t, pool, "private")
	setContentHash(t, pool, private, "h1")
	setSemanticStamp(t, pool, private, targetModel, "h1")
	if _, err := pool.Exec(ctx, "UPDATE jobs SET is_private = true WHERE id = $1", private); err != nil {
		t.Fatalf("mark private: %v", err)
	}

	got, err := q.SemanticCoverageCounts(ctx, targetModel)
	if err != nil {
		t.Fatalf("SemanticCoverageCounts: %v", err)
	}
	// Eligible: covered, never, staleHash, staleModel, nullHash. Covered: covered, nullHash.
	if got.Eligible != 5 || got.Covered != 2 {
		t.Errorf("counts = (eligible %d, covered %d), want (5, 2)", got.Eligible, got.Covered)
	}
}

func TestSemanticOutboxClaimableDepth(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	for _, ext := range []string{"a", "b", "c"} {
		insertJob(t, pool, ext)
	}
	if _, err := q.EnqueuePendingSemanticJobs(ctx, targetModel); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// One dead letter: it will never be claimed, so it is not work left to do.
	if _, err := pool.Exec(ctx, "UPDATE semantic_outbox SET failed_at = now() WHERE id = (SELECT min(id) FROM semantic_outbox)"); err != nil {
		t.Fatalf("dead-letter: %v", err)
	}

	depth, err := q.SemanticOutboxClaimableDepth(ctx)
	if err != nil {
		t.Fatalf("SemanticOutboxClaimableDepth: %v", err)
	}
	if depth != 2 {
		t.Errorf("depth = %d, want 2 (three queued, one dead-lettered)", depth)
	}
}
