package handler

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
)

// paidEntitlement is what the paid-AI gate asks of the plan: whether the caller is on a paid
// tier at all, and — only to word a refusal — where they stand on the feature they asked
// for. *plan.Store in production.
type paidEntitlement interface {
	RequirePaid(ctx context.Context, userID int64) (plan.Tier, error)
	Standing(ctx context.Context, userID int64, f plan.Feature) (plan.Standing, error)
}

// entitlementOf adapts a plan store to the gate, keeping a nil store nil. A nil *plan.Store
// held in the interface would read as configured and dereference on the first request; the
// gate treats a genuinely absent one as "could not tell", which is a 503.
func entitlementOf(s *plan.Store) paidEntitlement {
	if s == nil {
		return nil
	}
	return s
}

// unmeteredAI is the feature an AI route names when it draws on no daily allowance of its
// own — the ATS review, mail recall, autofill, a re-run of extraction. The
// paid gate still stands in front of it; only the refusal has no allowance to quote.
const unmeteredAI plan.Feature = ""

// paidAIGate builds the route middleware every user-initiated model call sits behind. It is
// a function of the feature rather than one handler, so a refusal on a metered route names
// what the plan does not include in the same body every other plan refusal carries; a route
// with no allowance of its own (the ATS review, mail recall, autofill,
// extraction) passes the zero Feature.
//
// Mounted AFTER the authentication gate, which is what put the caller's id in locals.
//
// It is one of two halves. The other is the user-call binding (user_llm.go), which refuses
// to hand out a model client for a context this gate did not mark — so a route that makes a
// model call and was mounted without this middleware fails closed on its first request
// rather than spending on somebody who never paid.
func paidAIGate(plans paidEntitlement) func(plan.Feature) fiber.Handler {
	return func(feature plan.Feature) fiber.Handler {
		return func(c *fiber.Ctx) error {
			userID, err := requireUserID(c)
			if err != nil {
				return err
			}
			if ok, err := authorizePaidAI(c, plans, userID, feature); !ok {
				return err
			}
			return c.Next()
		}
	}
}

// authorizePaidAI is the gate's check for one user, for the handler that cannot use the
// middleware because the user it acts for is not the authenticated caller — the auto-apply
// orchestrator acts for whichever candidate owns a queue entry.
//
// ok is false when the request has been answered: a 402 written to c (with a nil error, since
// `c.Status(402).JSON(...)` returns nil — which is why there is a bool beside it), or a 503
// returned as the error when the plan could not be read. A read failure never authorises:
// an entitlement nobody could read is not one, and what this protects is model spend.
//
// On success the request's user context is marked (plan.WithPaidAI), which is what the
// user-call binding reads before it hands out a model client. Handlers pass c.UserContext()
// — or a context derived from it — to the binding for exactly that reason.
func authorizePaidAI(c *fiber.Ctx, plans paidEntitlement, userID int64, feature plan.Feature) (bool, error) {
	if plans == nil {
		// A deployment wired without a plan store has no way to know who paid, and the
		// answer to "we could not tell" is the same as for a failed read.
		return false, fiber.NewError(fiber.StatusServiceUnavailable, "plan information is unavailable")
	}
	tier, err := plans.RequirePaid(c.Context(), userID)
	if errors.Is(err, plan.ErrPaymentRequired) {
		return false, refusePaidAI(c, plans, userID, tier, feature)
	}
	if err != nil {
		log.Printf("plan: paid entitlement for user %d: %v", userID, err)
		return false, fiber.NewError(fiber.StatusServiceUnavailable, "plan information is unavailable")
	}
	c.SetUserContext(plan.WithPaidAI(c.UserContext()))
	return true, nil
}

// refusePaidAI writes the 402 an unpaid caller gets, in the shape every plan refusal has —
// the sentence, the upgrade link, and for a metered feature the allowance it does not
// include — so the web and the extension render it through the path they already have.
//
// The allowance is read rather than assembled: a refusal built from fields filled in by
// hand is a lie that reads as fact at the next call site (see refuseStanding). If that read
// fails, the refusal still stands and goes out without it; there is nothing left to decide.
func refusePaidAI(c *fiber.Ctx, plans paidEntitlement, userID int64, tier plan.Tier, feature plan.Feature) error {
	if feature != "" {
		st, err := plans.Standing(c.Context(), userID, feature)
		if err == nil {
			return refuseStanding(c, st)
		}
		log.Printf("plan: standing for a paid-AI refusal, user %d on %s: %v", userID, feature, err)
	}
	body := fiber.Map{"error": refusalMessage(feature, false, false)}
	if hasSomethingToBuy(tier) {
		body["upgrade_url"] = upgradePath
	}
	return c.Status(fiber.StatusPaymentRequired).JSON(body)
}
