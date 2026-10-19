//go:build integration

// Integration tests for how job_semantic_chunks reaches the search index
// (openspec/changes/agent-semantic-search): the migration-0182 trigger that re-queues a job
// on search_outbox whenever its chunk set is written, and ListSemanticChunksForJobs, which
// the index writers read the vectors back through. Reuses startPostgres/truncate and the
// chunk helpers from semantic_chunks_integration_test.go.
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// searchOutboxCount counts a job's search_outbox rows.
func searchOutboxCount(t *testing.T, pool *pgxpool.Pool, jobID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM search_outbox WHERE job_id = $1", jobID).Scan(&n); err != nil {
		t.Fatalf("count search_outbox for job %d: %v", jobID, err)
	}
	return n
}

func TestJobSemanticChunksEnqueueSearch(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		truncate(t, pool)
		if _, err := pool.Exec(ctx, "TRUNCATE search_outbox"); err != nil {
			t.Fatalf("truncate search_outbox: %v", err)
		}
	}

	t.Run("cmd/embed's batch insert queues each job once", func(t *testing.T) {
		reset(t)
		a := insertChunkTestJob(t, pool, "a", "acme")
		b := insertChunkTestJob(t, pool, "b", "globex")

		if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
			JobIds:       []int64{a, a, a, b},
			ChunkIndices: []int16{0, 1, 2, 0},
			Embeddings:   []string{unitVector768(0), unitVector768(1), unitVector768(2), unitVector768(3)},
		}); err != nil {
			t.Fatalf("insert chunks: %v", err)
		}

		if got := searchOutboxCount(t, pool, a); got != 1 {
			t.Errorf("job a search_outbox rows = %d, want 1", got)
		}
		if got := searchOutboxCount(t, pool, b); got != 1 {
			t.Errorf("job b search_outbox rows = %d, want 1", got)
		}
	})

	// The volunteer broker (freehire-volunteer-embed, internal/promote) writes this table
	// with its own raw SQL from another repository — this is the writer the trigger exists
	// for, so its statement shape is reproduced here rather than routed through sqlc.
	t.Run("a raw-SQL replace shaped like the volunteer broker's queues the job", func(t *testing.T) {
		reset(t)
		j := insertChunkTestJob(t, pool, "volunteer", "acme")

		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `DELETE FROM job_semantic_chunks WHERE job_id = $1`, j); err != nil {
			t.Fatalf("delete: %v", err)
		}
		for i, dim := range []int{4, 5} {
			if _, err := tx.Exec(ctx,
				`INSERT INTO job_semantic_chunks (job_id, chunk_index, embedding) VALUES ($1, $2, $3::vector(768))`,
				j, i, unitVector768(dim)); err != nil {
				t.Fatalf("insert chunk %d: %v", i, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}

		if got := searchOutboxCount(t, pool, j); got != 1 {
			t.Errorf("search_outbox rows = %d, want 1", got)
		}
	})

	t.Run("re-embedding a job already waiting neither fails nor queues it twice", func(t *testing.T) {
		reset(t)
		j := insertChunkTestJob(t, pool, "requeue", "acme")
		insert := func(dim int) error {
			return q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
				JobIds: []int64{j}, ChunkIndices: []int16{0}, Embeddings: []string{unitVector768(dim)},
			})
		}

		if err := insert(0); err != nil {
			t.Fatalf("first insert: %v", err)
		}
		if err := q.DeleteJobSemanticChunks(ctx, []int64{j}); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if err := insert(1); err != nil {
			t.Fatalf("re-embed insert: %v", err)
		}

		if got := searchOutboxCount(t, pool, j); got != 1 {
			t.Errorf("search_outbox rows = %d, want 1", got)
		}
	})

	t.Run("a closed job is not queued", func(t *testing.T) {
		reset(t)
		j := insertChunkTestJob(t, pool, "closed", "acme")
		if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", j); err != nil {
			t.Fatalf("close job: %v", err)
		}

		if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
			JobIds: []int64{j}, ChunkIndices: []int16{0}, Embeddings: []string{unitVector768(0)},
		}); err != nil {
			t.Fatalf("insert chunks: %v", err)
		}

		if got := searchOutboxCount(t, pool, j); got != 0 {
			t.Errorf("search_outbox rows = %d, want 0", got)
		}
	})

	t.Run("the closed-job delete path queues nothing", func(t *testing.T) {
		reset(t)
		j := insertChunkTestJob(t, pool, "delete-only", "acme")
		if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
			JobIds: []int64{j}, ChunkIndices: []int16{0}, Embeddings: []string{unitVector768(0)},
		}); err != nil {
			t.Fatalf("insert chunks: %v", err)
		}
		if _, err := pool.Exec(ctx, "TRUNCATE search_outbox"); err != nil {
			t.Fatalf("truncate search_outbox: %v", err)
		}

		if err := q.DeleteJobSemanticChunks(ctx, []int64{j}); err != nil {
			t.Fatalf("delete: %v", err)
		}

		if got := searchOutboxCount(t, pool, j); got != 0 {
			t.Errorf("search_outbox rows = %d, want 0", got)
		}
	})
}

func TestListSemanticChunksForJobs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	a := insertChunkTestJob(t, pool, "a", "acme")
	b := insertChunkTestJob(t, pool, "b", "globex")
	other := insertChunkTestJob(t, pool, "other", "initech")
	// Inserted out of order on purpose: the query, not the insert, owns the ordering.
	if err := q.InsertJobSemanticChunks(ctx, InsertJobSemanticChunksParams{
		JobIds:       []int64{b, a, other, a},
		ChunkIndices: []int16{0, 1, 0, 0},
		Embeddings:   []string{unitVector768(3), unitVector768(1), unitVector768(9), unitVector768(0)},
	}); err != nil {
		t.Fatalf("insert chunks: %v", err)
	}

	rows, err := q.ListSemanticChunksForJobs(ctx, []int64{b, a})
	if err != nil {
		t.Fatalf("list chunks: %v", err)
	}

	type key struct {
		job   int64
		chunk int16
		dim   int
	}
	want := []key{{a, 0, 0}, {a, 1, 1}, {b, 0, 3}}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	for i, r := range rows {
		vec := r.Embedding.Slice()
		if r.JobID != want[i].job || r.ChunkIndex != want[i].chunk || vec[want[i].dim] != 1 {
			t.Errorf("row %d = (job %d, chunk %d), want (job %d, chunk %d, hot dim %d)",
				i, r.JobID, r.ChunkIndex, want[i].job, want[i].chunk, want[i].dim)
		}
	}
}
