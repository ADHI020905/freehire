package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// propagateBatchSize bounds how many job rows one statement locks. The single
// statement this replaced locked every diverging job at once and deadlocked
// against the live ingest writers (2026-10-21); a few thousand rows hold their
// locks for well under a second.
const propagateBatchSize = 5000

// maxDeadlockRetries is how many times one batch is re-run after PostgreSQL broke a
// lock cycle by aborting it. A deadlock says nothing is wrong with the batch, only
// that a concurrent writer wanted the same rows in another order, so the batch is
// simply run again; a batch that keeps losing is a real contention problem and
// fails the run rather than spinning.
const maxDeadlockRetries = 5

// deadlockBackoff is the base pause before a retry, multiplied by the attempt, so
// the writer that won has time to commit and move on.
var deadlockBackoff = time.Second

type collectionsPropagator interface {
	PropagateCollectionsToJobsBatch(ctx context.Context, arg db.PropagateCollectionsToJobsBatchParams) (db.PropagateCollectionsToJobsBatchRow, error)
}

// propagate copies every company's collection set onto its jobs, one keyset batch
// at a time, until a batch finds nothing left to change. Each batch is its own
// transaction, so a failure leaves the batches before it committed and a re-run
// picks up the rest through the query's IS DISTINCT FROM guard.
func propagate(ctx context.Context, q collectionsPropagator) (int64, error) {
	var after, total int64
	for {
		got, err := propagateBatch(ctx, q, after)
		if err != nil {
			return total, fmt.Errorf("batch after id %d (%d jobs already updated): %w", after, total, err)
		}
		if got.Updated == 0 {
			return total, nil
		}
		after, total = got.LastID, total+got.Updated
	}
}

// propagateBatch runs one batch, retrying it while it loses deadlocks.
func propagateBatch(ctx context.Context, q collectionsPropagator, after int64) (db.PropagateCollectionsToJobsBatchRow, error) {
	params := db.PropagateCollectionsToJobsBatchParams{AfterID: after, BatchSize: propagateBatchSize}
	for attempt := 1; ; attempt++ {
		got, err := q.PropagateCollectionsToJobsBatch(ctx, params)
		if err == nil || !pgerr.IsDeadlock(err) || attempt > maxDeadlockRetries {
			return got, err
		}
		log.Printf("import-collections: propagate batch after id %d deadlocked (attempt %d), retrying", after, attempt)
		select {
		case <-ctx.Done():
			return got, ctx.Err()
		case <-time.After(time.Duration(attempt) * deadlockBackoff):
		}
	}
}
