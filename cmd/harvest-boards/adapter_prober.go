package main

import (
	"context"
	"slices"
	"sync"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// proberFor resolves the prober for a provider: the bespoke one when probers names it, else
// the provider's own source adapter run as a probe.
//
// The fallback exists because the two lists drift. probers is hand-maintained and names ~35
// platforms; internal/ingest/sources carries close to a hundred adapters, and a platform missing from
// the first is not a platform we cannot crawl — it is one whose boards nobody can propose. That
// gap is what stranded 485 detected boards across 13 platforms in the August 2026 harvest. A
// bespoke prober is still worth writing (one cheap request instead of a full crawl, and on ~10
// platforms the employer name that powers the corroboration gate), so it always wins here.
//
// Two things the fallback does not inherit: -pace and the refusal counter. The adapter carries
// its own sources.Client, so its requests bypass the run's paced, counting client — the same
// blind spot the hand-written adapterProber entries have always had.
func proberFor(provider string) (prober, bool) {
	if p, ok := probers[provider]; ok {
		return p, true
	}
	reg := adapterRegistry()
	src, ok := reg[provider]
	if !ok || !slices.Contains(sources.BoardKeyedProviders(reg), provider) {
		return nil, false
	}
	return adapterProber{provider: provider, newSource: func() sources.Source { return src }}, true
}

// adapterRegistry builds the source registry once and shares it across probes, which is how
// cmd/ingest uses it too — the adapters hold a concurrency-safe client and no per-crawl state.
var adapterRegistry = sync.OnceValue(func() map[string]sources.Source {
	return sources.All(sources.NewClient())
})

// adapterProber validates a board by running the real source adapter and counting the
// postings it returns. It suits platforms (Taleo, Cornerstone) whose crawl is too stateful
// (a session-bound careersection flow, or a home-page token exchange) to cheaply reimplement
// as a one-shot probe — reusing the proven adapter is both correct and DRY. These adapters
// expose no cheap employer name (the board id is an opaque tenant/host), so the empty name
// falls back to the seed-supplied company.
//
// A fetch error is returned, so probeAll counts it as a probe failure: an adapter error is as
// likely a dead platform or a broken adapter as a dead board, and reporting it as "0 postings"
// is what hides the former behind a clean found=0.
type adapterProber struct {
	provider  string
	newSource func() sources.Source
}

func (a adapterProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	return a.probeSeeded(ctx, c, board, "")
}

// probeSeeded runs the adapter with the company the seed names for the board. The entry must
// carry SOME company: several adapters (hrmanager, tencent) take the posting's employer from
// the catalogue row rather than the platform and drop every posting when it is empty, so a
// company-less probe reads every live board as empty — measured on prod 2026-10-19, hrmanager
// found 0 of 216 seeded boards that way. With no seed company the board id stands in: it is
// only a label on postings this probe counts and discards, and it is the same fallback
// chooseCompany puts on the harvested entry.
func (a adapterProber) probeSeeded(ctx context.Context, _ httpClient, board, company string) (string, int, error) {
	if company == "" {
		company = board
	}
	e := sources.CompanyEntry{Provider: a.provider, Board: board, Company: company}
	src := a.newSource()
	// A hydrating adapter is asked for its listing alone: every posting reads as already held,
	// so the probe costs the listing and no detail request — the body is not what it counts,
	// and on a platform whose detail is slow (oorwin's takes seconds) Fetch would hydrate
	// every board it probes only to throw the bodies away.
	if hs, ok := src.(sources.HydratingSource); ok {
		jobs, err := hs.FetchNew(ctx, e, func(string) bool { return true })
		if err != nil {
			return "", 0, err
		}
		return "", len(jobs), nil
	}
	jobs, err := src.Fetch(ctx, e)
	if err != nil {
		return "", 0, err
	}
	return "", len(jobs), nil
}
