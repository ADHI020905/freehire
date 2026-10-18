// Package outbox is the shared claim-wave drain loop behind every Postgres outbox
// queue worker in this repo. It owns the loop, not the queue: claim a wave, process
// it, tally the outcome, repeat until a claim comes back empty. Each caller keeps its
// own Store/Indexer/Provider port and its own Complete/Fail/Discard logic; Runner
// never touches persistence itself.
package outbox

import (
	"context"
	"sync"
)

// Outcome is what a Processor did with one claimed item. The caller has already made
// whatever Store call follows from it (Complete, Fail, Discard); Runner only tallies.
type Outcome int

const (
	Succeeded Outcome = iota
	Failed
	DeadLettered
	Discarded
	// Deferred is an item put back untouched: claimed, but not attempted, so no attempt
	// is counted against it; its lease expiring is what makes it claimable again. It
	// still counts toward MaxPerRun, since claiming it was work. It is what a
	// caller reports when the far side has asked it to slow down — a 429 is a statement
	// about the moment, not about the item, and spending the item's attempts on it turns
	// one throttled afternoon into a queue of dead letters.
	Deferred
)

// Stats is what one Run did.
type Stats struct {
	Succeeded    int
	Failed       int
	DeadLettered int
	Discarded    int
	Deferred     int
}

// Claimer leases up to batch live, unleased entries — the shape every package's
// existing Store.Claim method already has.
//
// Deliberately absent: Runner has no built-in context-cancellation early-exit. Two of
// its four RunPool callers (applyform, adzunadesc) return (stats, nil) as soon as a
// wave finishes if ctx is cancelled, so a shutdown doesn't burn the rest of a large
// backlog's retry attempts; the other two (enrich, maillink) let a cancelled ctx
// surface as an ordinary error from the next Claim call. Baking either behavior into
// Runner would silently change the other callers' exit code on SIGTERM. A caller that
// wants the early-exit implements it in its own Claim wrapper: check ctx.Err() first
// and return (nil, nil) instead of querying — Runner's normal empty-batch stop then
// reproduces the exact same outcome.
type Claimer[C any] interface {
	Claim(ctx context.Context, batch, leaseSeconds int) ([]C, error)
}

// RunOptions are the per-run knobs. MaxAttempts and any per-call timeout are
// deliberately absent here — they govern a single Processor call's own retry/timeout
// behavior, which stays inside each caller's closure alongside its Store calls, not in
// the shared loop.
type RunOptions struct {
	BatchSize    int
	LeaseSeconds int
	// Concurrency is RunPool-only; RunBatch ignores it (its concurrency lives inside
	// whatever the caller's BatchProcessor does in one call).
	Concurrency int
	// MaxPerRun bounds how much of the backlog one Run takes, shared by RunPool and
	// RunBatch; 0 is unbounded.
	MaxPerRun int
	// OnWave, if set, is called after each wave is claimed and processed, with the
	// cumulative Stats so far — the hook a caller's own per-wave progress-heartbeat
	// log line (e.g. "enrich: progress enriched=... failed=... dead=...") is built
	// from, so migrating onto Runner doesn't silently drop that log line.
	OnWave func(Stats)
}

// Processor handles one claimed item end to end, including whatever Store.Complete /
// Store.Fail / Store.Discard call follows, and reports what happened.
type Processor[C any] func(ctx context.Context, item C) Outcome

// RunPool drains via a bounded pool when opt.Concurrency > 1. At Concurrency <= 1 it
// processes the wave as a plain sequential loop — no goroutine is spawned at all — so
// a caller that needs in-order processing (internal/application/maillink, where a later message in
// a thread must see the same wave's earlier link) gets a real ordering guarantee: Go's
// channel semantics between blocked senders are not formally FIFO, so a size-1
// semaphore-gated pool would not have been a safe substitute for a literal loop.
func RunPool[C any](ctx context.Context, claimer Claimer[C], opt RunOptions, process Processor[C]) (Stats, error) {
	var stats Stats
	for {
		batchSize, budgetSpent := nextBatchSize(opt, stats)
		if budgetSpent {
			return stats, nil
		}
		batch, err := claimer.Claim(ctx, batchSize, opt.LeaseSeconds)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			return stats, nil
		}
		if opt.Concurrency <= 1 {
			for _, item := range batch {
				tally(&stats, process(ctx, item))
			}
		} else {
			runPoolWave(ctx, batch, opt.Concurrency, process, &stats)
		}
		if opt.OnWave != nil {
			opt.OnWave(stats)
		}
	}
}

// BatchProcessor handles a whole wave in one call (e.g. one Meilisearch bulk push).
// A nil error means the closure fully handled every item itself — including any
// internal sub-grouping and its own per-item fallback, as internal/ai/embed's open/closed
// split does — and the returned Stats is trusted verbatim; RunBatch adds it to the
// running total and does NOT also invoke Processor. A non-nil error means the call
// itself failed outright (the closure could not attempt anything) and RunBatch falls
// back to processing the wave item by item through Processor; the returned Stats is
// ignored in that case.
//
// Returning (Stats{}, nil) — no error, an all-zero Stats — is deliberately meaningful:
// it says "I chose to do nothing with this wave" (e.g. a call context merely timed out,
// so the closure leaves the wave claimed for a later run's lease-expiry retry rather
// than falling back per-item, which would turn one slow-but-fine batch into many
// equally slow calls). RunBatch tallies nothing for that wave and does not fall back.
type BatchProcessor[C any] func(ctx context.Context, items []C) (Stats, error)

// RunBatch drains via one BatchProcessor call per wave, falling back to per-item
// Processor calls only when the batch call itself fails outright — so one
// poison/corrupted item can't sink an otherwise-healthy wave, and a closure that
// handles its own sub-grouping/fallback (internal/ai/embed) never gets double-processed.
func RunBatch[C any](ctx context.Context, claimer Claimer[C], opt RunOptions, processBatch BatchProcessor[C], processOne Processor[C]) (Stats, error) {
	var stats Stats
	for {
		batchSize, budgetSpent := nextBatchSize(opt, stats)
		if budgetSpent {
			return stats, nil
		}
		batch, err := claimer.Claim(ctx, batchSize, opt.LeaseSeconds)
		if err != nil {
			return stats, err
		}
		if len(batch) == 0 {
			return stats, nil
		}
		waveStats, err := processBatch(ctx, batch)
		if err != nil {
			for _, item := range batch {
				tally(&stats, processOne(ctx, item))
			}
		} else {
			stats.Succeeded += waveStats.Succeeded
			stats.Failed += waveStats.Failed
			stats.DeadLettered += waveStats.DeadLettered
			stats.Discarded += waveStats.Discarded
			stats.Deferred += waveStats.Deferred
		}
		if opt.OnWave != nil {
			opt.OnWave(stats)
		}
	}
}

func runPoolWave[C any](ctx context.Context, batch []C, concurrency int, process Processor[C], stats *Stats) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, concurrency)

	for _, item := range batch {
		wg.Add(1)
		go func(it C) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()

			outcome := process(ctx, it)
			mu.Lock()
			tally(stats, outcome)
			mu.Unlock()
		}(item)
	}
	wg.Wait()
}

// nextBatchSize applies opt.MaxPerRun (shared by RunPool and RunBatch) to
// opt.BatchSize, reporting whether the budget is already spent — in which case the
// caller must stop without claiming again.
func nextBatchSize(opt RunOptions, stats Stats) (size int, budgetSpent bool) {
	if opt.MaxPerRun <= 0 {
		return opt.BatchSize, false
	}
	processed := stats.Succeeded + stats.Failed + stats.DeadLettered + stats.Discarded + stats.Deferred
	remaining := opt.MaxPerRun - processed
	if remaining <= 0 {
		return 0, true
	}
	return min(opt.BatchSize, remaining), false
}

func tally(stats *Stats, o Outcome) {
	switch o {
	case Succeeded:
		stats.Succeeded++
	case Failed:
		stats.Failed++
	case DeadLettered:
		stats.DeadLettered++
	case Discarded:
		stats.Discarded++
	case Deferred:
		stats.Deferred++
	}
}
