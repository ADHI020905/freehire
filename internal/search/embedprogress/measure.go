package embedprogress

import (
	"context"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// CoverageCounter runs the coverage scan. *db.Queries satisfies it.
type CoverageCounter interface {
	SemanticCoverageCounts(ctx context.Context, targetModel string) (db.SemanticCoverageCountsRow, error)
}

// Measure counts coverage against the model the catalogue is embedded with now, the
// same target cmd/embed enqueues against, so "covered" means "the queue will not take
// it again".
func Measure(ctx context.Context, counter CoverageCounter, now func() time.Time) (Snapshot, error) {
	row, err := counter.SemanticCoverageCounts(ctx, search.CurrentEmbedderModel())
	if err != nil {
		return Snapshot{}, fmt.Errorf("embedprogress: counting coverage: %w", err)
	}
	return Snapshot{Eligible: row.Eligible, Covered: row.Covered, MeasuredAt: now().UTC()}, nil
}
