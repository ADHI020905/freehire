package billing

import "time"

// APITier is which tier of the API product an account holds right now. It is not a plan
// tier (internal/ai/plan): holding API access confers no metered AI allowance, and Pro
// confers no API access.
type APITier string

const (
	APITierNone APITier = ""
	APITierAPI  APITier = "api"
	APITierData APITier = "api_data"
)

// APITierOf resolves the tier from how far each of the account's API subscriptions reaches
// (users.api_until_stripe, users.api_data_until_stripe): Data while it is live, then API.
// A zero time is "Stripe confers nothing".
func APITierOf(apiUntil, dataUntil, now time.Time) APITier {
	switch {
	case dataUntil.After(now):
		return APITierData
	case apiUntil.After(now):
		return APITierAPI
	default:
		return APITierNone
	}
}
