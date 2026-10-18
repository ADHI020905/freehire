package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type fakeDeleter struct{ got pgtype.Interval }

func (f *fakeDeleter) DeleteExpiredTracerClicks(_ context.Context, maxAge pgtype.Interval) (int64, error) {
	f.got = maxAge
	return 3, nil
}

// The window reaches the query as whole days: a months or microseconds component would
// shift the cutoff the privacy policy states in days.
func TestSweepPassesTheWindowInDays(t *testing.T) {
	f := &fakeDeleter{}
	n, err := sweep(context.Background(), f, 180)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n != 3 {
		t.Errorf("deleted = %d, want 3", n)
	}
	want := pgtype.Interval{Days: 180, Valid: true}
	if f.got != want {
		t.Errorf("interval = %+v, want %+v", f.got, want)
	}
}
