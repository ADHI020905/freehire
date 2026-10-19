package sources

import (
	"testing"
	"time"
)

// TestWorkablePaceFitsTheRunBudget pins the arithmetic behind workableRequestInterval (see its
// comment for the measurements). Too fast and the refusals come back; too slow and systemd
// kills a shard mid-crawl, which board_health cannot tell apart from the platform being down.
func TestWorkablePaceFitsTheRunBudget(t *testing.T) {
	const (
		// Measured on prod 2026-10-19.
		boards = 3963
		// Workable began refusing after ~1,500 boards × 2 requests in ~30 minutes.
		refusalOnsetPerSecond = 3000.0 / 1800.0
		// The scheduler override this pace is sized for: 2 shards, 4500s per run.
		shards      = 2
		unitTimeout = 4500 * time.Second
		headroom    = 0.8
	)

	perSecond := float64(time.Second) / float64(workableRequestInterval)
	// One listing per board: the description is asked once per company, not per run.
	perShard := float64(boards) / shards
	runtime := time.Duration(perShard / perSecond * float64(time.Second))

	if budget := time.Duration(float64(unitTimeout) * headroom); runtime > budget {
		t.Errorf("a shard takes %v at %.2f req/s, past the %v working budget — raise the shard "+
			"count or the pace", runtime.Round(time.Second), perSecond, budget)
	}
	// The limiter is per process: two shards running at once double the provider's rate.
	if both := perSecond * shards; both >= refusalOnsetPerSecond {
		t.Errorf("two overlapping shards ask %.2f req/s, at or past the ~%.2f req/s Workable "+
			"began refusing at", both, refusalOnsetPerSecond)
	}
	if workableRequestBurst != 1 {
		t.Errorf("burst = %d, want 1 — the point is to stop front-loading the run", workableRequestBurst)
	}
}

// TestWorkableIsPaced guards both wirings: the registry, and the refusal-retry rewiring
// ApplyProxyEgress does when SOURCES_PROXY_URL is set (production). Losing the pacer in either
// is silent — the crawl simply starts being refused again.
func TestWorkableIsPaced(t *testing.T) {
	src, ok := All(NewClient())["workable"].(workable)
	if !ok {
		t.Fatal("workable is not registered, or no longer the workable type")
	}
	if _, paced := src.http.(rateLimitedJSONGetter); !paced {
		t.Errorf("registry workable is not paced: got %T", src.http)
	}

	rewired, ok := refusalRetryProviders["workable"](NewClient()).(workable)
	if !ok {
		t.Fatal("refusal-retry workable is missing, or no longer the workable type")
	}
	if _, paced := rewired.http.(rateLimitedJSONGetter); !paced {
		t.Errorf("refusal-retry workable is not paced: got %T", rewired.http)
	}
}
