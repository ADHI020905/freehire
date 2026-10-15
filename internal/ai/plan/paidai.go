package plan

import (
	"context"
	"errors"
)

// ErrPaymentRequired is RequirePaid's answer for a caller on no paid plan. It is distinct
// from ErrRefused, which is a spent allowance: this one means the plan never included any
// of it, and that no amount of waiting for midnight changes the answer.
var ErrPaymentRequired = errors.New("plan: a paid plan is required")

// RequirePaid answers whether this caller may have a model call made for them at all.
//
// Every user-initiated model call needs a live Pro or Ultra entitlement — a product
// invariant, not an allowance: there is no free figure to raise and no environment variable
// that reopens one. A metered action still draws on its own daily allowance afterwards;
// this is the question in front of all of them, including the AI actions that have no
// allowance of their own (extraction, the ATS review, search intent, mail recall, autofill).
//
// A failed read returns the raw error and no tier. The caller must refuse on it (503), never
// authorise: an entitlement nobody could read is not an entitlement, and the fail-open rule
// the meters follow does not apply here — what this protects is model spend, not
// bookkeeping.
func (s *Store) RequirePaid(ctx context.Context, userID int64) (Tier, error) {
	tier, err := s.Tier(ctx, userID)
	if err != nil {
		return "", err
	}
	if !tier.Paid() {
		return tier, ErrPaymentRequired
	}
	return tier, nil
}

// paidAIKey is the context key WithPaidAI sets. Unexported, so the only way to mark a
// context is the function below.
type paidAIKey struct{}

// WithPaidAI marks ctx as carrying a verified paid entitlement for the request it serves.
//
// Call it ONLY after RequirePaid answered yes for the user the work belongs to. The mark is
// what the user-call binding in internal/api/handler checks before it hands out a model
// client, so a route that makes a model call without having been through the gate fails
// closed on its first request — and in its first test — instead of quietly spending on
// somebody who never paid.
func WithPaidAI(ctx context.Context) context.Context {
	return context.WithValue(ctx, paidAIKey{}, true)
}

// PaidAI reports whether ctx was marked by WithPaidAI.
func PaidAI(ctx context.Context) bool {
	v, _ := ctx.Value(paidAIKey{}).(bool)
	return v
}
