package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
)

// The public read API is unauthenticated and, until this existed, unbounded:
// every other limiter in this package guards an auth route, a write, or an LLM
// spend, and nginx applies its own limit_req to the SvelteKit pages and not to
// /api/. Both ceilings below sit above the highest per-IP minute measured on
// production over a 4.6-hour window, so they bound abuse without cutting off any
// caller that exists today.
//
// The split is by cost, not by path. `/agent/jobs/search` rehydrates every hit's
// full description from Postgres — ~833 KB and ~1.3s at limit=100, against
// ~123 KB and ~0.55s for the ordinary search — so one shared budget would have
// to be sized for it and would then throttle facet lookups seven times harder
// than their cost warrants.
const (
	// publicReadsPerMinute covers the cheap reads. 600/min is 10 r/s, deliberately
	// the same ceiling nginx already applies to the HTML pages, so the site has one
	// number to explain rather than two. Measured peak: 258/min.
	publicReadsPerMinute = 600

	// agentSearchPerMinute covers the one expensive read. Measured peak: 184/min,
	// held steadily by a single third-party client, so this leaves that caller room
	// to grow by two thirds before it ever sees a 429.
	agentSearchPerMinute = 300

	// suggestPerMinute covers the search box's completions. The split here is by
	// FREQUENCY rather than by cost: each call is cheap — a query against a
	// dictionary of tens of thousands of documents, not the 8M-document catalogue —
	// but the box issues one per settled keystroke, so a visitor composing a query
	// spends ten of these where the same visitor spends one ordinary read. Sharing
	// the public-read budget would mean typing a query throttles the page that
	// answers it.
	//
	// 1200/min is 20 r/s per caller, which is faster than anybody types and far
	// short of what a scraper would want from an endpoint that returns no postings.
	suggestPerMinute = 1200

	// talentCatalogPerMinute covers the public Talent Network catalogue. The split
	// here is by WHAT IS BEING READ rather than by cost or frequency: this is the one
	// public read that returns people rather than postings. The catalogue is small,
	// complete and machine-readable, which makes copying it wholesale a matter of
	// patience — and the members consented to being FOUND, not to being enumerated
	// into somebody's database.
	//
	// A separate budget so it can be tightened when a scrape is seen without touching
	// the reads the rest of the site depends on, and so exhausting it cannot take the
	// catalogue down with the job pages. 120/min is two per second: far past a person
	// paging and filtering, and slow enough that walking a few hundred members takes
	// long enough to notice.
	talentCatalogPerMinute = 120
)

// A signed-in COOKIE caller is budgeted by IP, deliberately. Fiber runs a route's handlers in
// registration order, and every public read registers its limiter BEFORE the optional-auth
// gate (jobs.go, companies.go, search.go) or without a gate at all — moving the gate in front
// would put a database lookup ahead of throttling on the hottest reads on the site, against a
// host where crawlers are most of the traffic, and OptionalAuth answers 503 when a key lookup
// fails. Fifty colleagues behind one office NAT share one budget.
//
// The one caller told apart is a PAID API key (public_gate.go), and only because the gate
// resolves it itself: only when a key is presented, through a cache, falling back to the
// anonymous budget on any failure. Nothing else about the caller is known when the limiter
// runs.
//
// The tests mount the gate FIRST, so an authenticated cookie caller really is authenticated
// when the limiter runs: they fail if anyone reintroduces a user-keyed budget here. A second
// guard drives these routes exactly as `register` mounts them. See
// internal/api/handler/AGENTS.md for the rule.

// publicReadLimiter throttles the cheap public reads. Which budget a caller draws on — a paid
// key's, a verified bot's exemption, or the anonymous one — is the gate's decision
// (public_gate.go); this names the route class.
func publicReadLimiter(g *publicGate) fiber.Handler {
	return g.limiter(publicReadBudget)
}

// agentSearchLimiter throttles the full-description search. Before PUBLIC_ANON_LIMITS=strict
// its anonymous budget is its own, so exhausting it leaves the ordinary search endpoints
// serving; under strict the two share the anonymous budgets (see public_gate.go).
func agentSearchLimiter(g *publicGate) fiber.Handler {
	return g.limiter(agentSearchBudget)
}

// chatAgentLimiter is agentSearchLimiter for the ChatGPT app's MCP surfaces, which keep the
// agent-search budget for anonymous callers even under strict (see chatAgentBudget).
func chatAgentLimiter(g *publicGate) fiber.Handler {
	return g.limiter(chatAgentBudget)
}

// suggestLimiter throttles the search box's completions on their own budget, so a
// visitor typing quickly cannot exhaust the allowance the rest of the site reads on.
func suggestLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByIP("suggest"), suggestPerMinute, time.Minute)
}

// talentCatalogLimiter throttles the public catalogue of candidates on its own budget.
func talentCatalogLimiter(throttler ratelimit.Throttler) fiber.Handler {
	return ratelimit.Middleware(throttler, ratelimit.KeyByIP("talent"), talentCatalogPerMinute, time.Minute)
}
