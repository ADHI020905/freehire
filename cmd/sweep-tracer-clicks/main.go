// Command sweep-tracer-clicks deletes CV tracer-link clicks (cv_link_clicks) older than the
// retention window. The privacy policy promises these records are gone after 180 days, so this
// is a retention obligation rather than housekeeping: it deletes by default, with no dry run,
// and runs daily on a timer. Missing a run only delays the deletion by a day.
//
// Only the clicks go. The tracer tokens stay, because a PDF sent long ago must keep redirecting
// after the clicks behind it have aged out.
//
// Needs DATABASE_URL. TRACER_CLICK_RETENTION_DAYS (default 180) sets the window; it is read
// through the strict worker reader, so a set but unreadable value fails the run instead of
// quietly falling back.
package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	days, err := worker.EnvInt32("TRACER_CLICK_RETENTION_DAYS", 180)
	if err != nil {
		log.Printf("sweep-tracer-clicks: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	n, err := sweep(ctx, db.New(pool), days)
	if err != nil {
		log.Printf("sweep-tracer-clicks: %v", err)
		return 1
	}
	log.Printf("sweep-tracer-clicks: deleted %d click(s) older than %dd", n, days)
	return 0
}

// clickDeleter is the one query the sweep needs.
type clickDeleter interface {
	DeleteExpiredTracerClicks(ctx context.Context, maxAge pgtype.Interval) (int64, error)
}

// sweep deletes every click older than days and returns how many went.
func sweep(ctx context.Context, q clickDeleter, days int32) (int64, error) {
	return q.DeleteExpiredTracerClicks(ctx, pgtype.Interval{Days: days, Valid: true})
}
