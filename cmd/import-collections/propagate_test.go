package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakePropagator replays scripted batch results and records the cursor each call
// was given.
type fakePropagator struct {
	results []db.PropagateCollectionsToJobsBatchRow
	errs    []error
	afters  []int64
}

func (f *fakePropagator) PropagateCollectionsToJobsBatch(_ context.Context, arg db.PropagateCollectionsToJobsBatchParams) (db.PropagateCollectionsToJobsBatchRow, error) {
	i := len(f.afters)
	f.afters = append(f.afters, arg.AfterID)
	if i < len(f.errs) && f.errs[i] != nil {
		return db.PropagateCollectionsToJobsBatchRow{}, f.errs[i]
	}
	if i < len(f.results) {
		return f.results[i], nil
	}
	return db.PropagateCollectionsToJobsBatchRow{}, nil
}

var deadlock = &pgconn.PgError{Code: "40P01"}

func init() { deadlockBackoff = 0 }

func TestPropagate_WalksTheCursorUntilABatchChangesNothing(t *testing.T) {
	f := &fakePropagator{results: []db.PropagateCollectionsToJobsBatchRow{
		{Updated: 5000, LastID: 900},
		{Updated: 12, LastID: 70000},
		{Updated: 0},
	}}
	total, err := propagate(context.Background(), f)
	if err != nil {
		t.Fatalf("propagate: %v", err)
	}
	if total != 5012 {
		t.Errorf("total = %d, want 5012", total)
	}
	if want := []int64{0, 900, 70000}; !slices.Equal(f.afters, want) {
		t.Errorf("cursors = %v, want %v", f.afters, want)
	}
}

func TestPropagate_RetriesADeadlockedBatchFromTheSameCursor(t *testing.T) {
	f := &fakePropagator{
		errs: []error{nil, deadlock, nil, nil},
		results: []db.PropagateCollectionsToJobsBatchRow{
			{Updated: 3, LastID: 40},
			{},
			{Updated: 2, LastID: 90},
			{Updated: 0},
		},
	}
	total, err := propagate(context.Background(), f)
	if err != nil {
		t.Fatalf("propagate: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if want := []int64{0, 40, 40, 90}; !slices.Equal(f.afters, want) {
		t.Errorf("cursors = %v, want %v (the deadlocked batch must be re-run, not skipped)", f.afters, want)
	}
}

func TestPropagate_GivesUpOnAPersistentDeadlock(t *testing.T) {
	errs := make([]error, maxDeadlockRetries+1)
	for i := range errs {
		errs[i] = deadlock
	}
	f := &fakePropagator{errs: errs}
	if _, err := propagate(context.Background(), f); !errors.Is(err, deadlock) {
		t.Fatalf("propagate err = %v, want the deadlock after %d retries", err, maxDeadlockRetries)
	}
	if len(f.afters) != maxDeadlockRetries+1 {
		t.Errorf("attempts = %d, want %d", len(f.afters), maxDeadlockRetries+1)
	}
}

func TestPropagate_DoesNotRetryOtherErrors(t *testing.T) {
	f := &fakePropagator{errs: []error{errors.New("connection refused")}}
	if _, err := propagate(context.Background(), f); err == nil {
		t.Fatal("propagate swallowed a non-deadlock error")
	}
	if len(f.afters) != 1 {
		t.Errorf("attempts = %d, want 1", len(f.afters))
	}
}
