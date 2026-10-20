// Package ratelimit provides a single rate-limiting facility shared by every
// rate-limited HTTP route in the API, backed by Redis.
package ratelimit

import (
	"context"
	"log"
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// allowTimeout bounds how long a single Allow call may take before Middleware
// gives up and fails open. Long enough for a healthy same-DC Redis round-trip,
// short enough that a partitioned backend degrades to "every request logs a
// warning and passes through" instead of stalling requests.
const allowTimeout = 100 * time.Millisecond

// Decision is one rate-limit verdict, carrying everything the caller needs to
// answer the request AND to tell the client where it stands.
//
// It is a struct rather than a longer return list because the backend already
// computes all of it in the same round trip, and reporting a budget the decision
// did not produce is how a client ends up pacing itself against a number that
// was never true.
type Decision struct {
	Allowed bool
	// Limit is the ceiling in force for this check, echoed back so a client
	// never has to infer it from observed rejections.
	Limit int
	// Remaining is how many further requests the window still permits.
	Remaining int
	// ResetAfter is how long until the caller's budget is full again.
	ResetAfter time.Duration
	// RetryAfter is how long until the next request would be permitted. It is
	// meaningful only when Allowed is false.
	RetryAfter time.Duration
}

// Throttler checks whether a request rate limit has been exceeded for a key.
type Throttler interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Decision, error)
}

// Middleware creates a Fiber middleware that enforces rate limiting via a
// Throttler, and reports the caller's remaining budget on every response it
// checks.
//
// It fails open — logging a warning and allowing the request — on any error from
// Allow, including exceeding the bounded per-call timeout it imposes. A nil
// throttler (no backend configured) also fails open, with no warning logged. A
// fail-open response carries no budget headers: no check happened, so there is
// no budget to report, and an invented one is worse than silence.
//
// A request from a trusted peer (see TrustedCIDRs) skips the check entirely and
// carries no headers, because our own server-rendered front end reaches the API
// over loopback and would otherwise spend one shared budget for the whole site.
func Middleware(throttler Throttler, keyFunc func(c *fiber.Ctx) string, limit int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if TrustedPeer(c) {
			return c.Next()
		}
		return checkAndRespond(c, throttler, keyFunc(c), limit, window)
	}
}

// MiddlewareIgnoringTrustedPeers is Middleware without the trusted-peer exemption, for a
// caller whose OWN authentication already establishes who it is more specifically than any
// IP range could — see internal/api/handler's own autoApplyOrchestratorGate, whose entire
// reason for existing is a compensating rate limit for a shared secret with no per-caller
// scoping. Going through the ordinary Middleware would silently void that control: the
// orchestrator's own default HireBaseURL is http://127.0.0.1:<port> (the real host-2
// deploy shape), and 127.0.0.1 is unconditionally trusted regardless of TrustedCIDRs
// (isTrusted's own IsLoopback check).
func MiddlewareIgnoringTrustedPeers(throttler Throttler, keyFunc func(c *fiber.Ctx) string, limit int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return checkAndRespond(c, throttler, keyFunc(c), limit, window)
	}
}

// checkAndRespond is Middleware's and MiddlewareIgnoringTrustedPeers' shared body: consult
// the Throttler for one key and either continue or answer 429, identically either way. Only
// whether the trusted-peer exemption runs FIRST differs between the two callers.
func checkAndRespond(c *fiber.Ctx, throttler Throttler, key string, limit int, window time.Duration) error {
	if err := Enforce(c, throttler, key, limit, window); err != nil {
		return err
	}
	return c.Next()
}

// Enforce is one budget check WITHOUT continuing the chain: nil means the request may
// proceed, a 429 error means it may not. It is for a caller that must run more than one
// check, or choose between budgets, inside a single handler — calling another Middleware
// from inside a handler would run c.Next() and skip whatever that caller meant to check
// next. Fail-open, headers and the Retry-After rule are exactly Middleware's.
//
// It does NOT apply the trusted-peer exemption; a caller that wants it asks TrustedPeer first.
func Enforce(c *fiber.Ctx, throttler Throttler, key string, limit int, window time.Duration) error {
	d, ok := Check(c, throttler, key, limit, window)
	if !ok {
		return nil
	}
	return Apply(c, d, key, window)
}

// Check asks the Throttler about one key and touches nothing on the response. ok is false when
// no decision was made — no backend, or a backend error or timeout, which is logged — and the
// caller must then treat the request as allowed, the fail-open every limiter here shares.
//
// Check and Apply are Enforce in two halves, for a caller that must see several decisions
// before choosing which one the response reports.
func Check(c *fiber.Ctx, throttler Throttler, key string, limit int, window time.Duration) (Decision, bool) {
	if throttler == nil {
		return Decision{}, false
	}
	ctx, cancel := context.WithTimeout(c.Context(), allowTimeout)
	defer cancel()
	d, err := throttler.Allow(ctx, key, limit, window)
	if err != nil {
		log.Printf("ratelimit: Allow error for key %q: %v (failing open)", key, err)
		return Decision{}, false
	}
	return d, true
}

// Apply reports a decision on the response: its budget headers always, and when it refuses, a
// Retry-After and the 429 to return.
func Apply(c *fiber.Ctx, d Decision, key string, window time.Duration) error {
	setBudgetHeaders(c, d)
	if d.Allowed {
		return nil
	}
	// Retry-After is whole seconds (RFC 9110 delta-seconds), rounded UP and
	// floored at one: every way of losing the fraction tells a compliant
	// client to retry early, into a denial it was just given. 1.7s must read
	// as "2", and 200ms as "1", never "0".
	c.Set("Retry-After", wholeSecondsAtLeastOne(d.RetryAfter))
	log.Printf("ratelimit: refused key %q on %s (limit %d/%v, ua=%q)",
		key, c.Path(), d.Limit, window, c.Get(fiber.HeaderUserAgent))
	return fiber.NewError(fiber.StatusTooManyRequests, "too many requests, please try again later")
}

// setBudgetHeaders writes the caller-facing view of a Decision. Reset is rounded
// UP for the same reason as Retry-After: a bucket that refills in 400ms must not
// report "0", which reads as "you are already clear" to a client that then
// retries into the same denial.
func setBudgetHeaders(c *fiber.Ctx, d Decision) {
	c.Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
	c.Set("X-RateLimit-Remaining", strconv.Itoa(max(d.Remaining, 0)))
	c.Set("X-RateLimit-Reset", strconv.Itoa(ceilSeconds(d.ResetAfter)))
}

// ceilSeconds renders a duration as whole seconds, rounding any fraction up.
func ceilSeconds(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(math.Ceil(d.Seconds()))
}

// wholeSecondsAtLeastOne is ceilSeconds with a floor of one, for the headers
// that name a wait: "0" would invite an immediate retry.
func wholeSecondsAtLeastOne(d time.Duration) string {
	return strconv.Itoa(max(ceilSeconds(d), 1))
}
