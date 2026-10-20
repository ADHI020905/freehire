## 1. Entitlement storage

- [x] 1.1 Migration: nullable `users.api_until_stripe`, `users.api_data_until_stripe` (squawk-clean)
- [x] 1.2 Extend `SetStripeEntitlement` to write all four Stripe columns in one statement; widen `ListSubscribersNearProExpiryStripe` to the two new columns; `make sqlc`
- [x] 1.3 Query `ResolveAPIKeyTier(token_hash)` → `(user_id, scope, api_until_stripe, api_data_until_stripe)` without touching `last_used_at`

## 2. Billing

- [x] 2.1 Config: `STRIPE_API_PRICE_IDS`, `STRIPE_API_DATA_PRICE_IDS`; `Sells` admits all four lists
- [x] 2.2 `entitlement` gains `API`/`Data`; `entitlementFrom` resolves them from the same re-read; `store` writes them
- [x] 2.3 Checkout picks the requested price's family for `decideCheckoutTarget` (Pro subscriber buying API opens a new session; API→Data upgrades in place; API subscriber buying Pro opens a new session)
- [x] 2.4 `billing.APITierOf(apiUntil, dataUntil, now)` pure resolver (data > api > none)

## 3. Limiter

- [x] 3.1 Paid-key resolver: Bearer `fhk_` only, hash, 60s in-process TTL cache of results incl. negatives, errors not cached and degrade to anonymous, only `full` scope counts
- [x] 3.2 Verified-bot check: `X-FH-Verified-Bot` vs `VERIFIED_BOT_SECRET`, constant-time, unset disables
- [x] 3.3 `publicLimiter(budget)` gate: trusted peer → bot → paid (per account, tier rate) → anonymous (legacy per-route budgets, or `PUBLIC_ANON_LIMITS=strict` shared 120/min + 2,000/day); 429 message names `/pricing#api`
- [x] 3.4 Mount the gate everywhere `publicReadLimiter`/`agentSearchLimiter` were mounted (both now thin wrappers over the gate, plus `chatAgentLimiter` for MCP); rewrite the guard tests in `public_read_limit*_test.go` for the new caller classes (incl. unpaid full key and paid embed key stay IP-keyed) and drive them through `register`

## 4. Web

- [x] 4.1 `/plans` (or pricing loader) exposes the API and Data prices; `/pricing` gains an "API" section with both plans and checkout links
- [x] 4.2 `/my/api-keys` shows the account's API tier and its rate, or a link to buy
- [x] 4.3 Terms: data licence section (store/display allowed, no raw resale, attribution)

## 5. Docs

- [x] 5.1 `web/static/openapi.yaml`: `bearerAuth` security scheme (optional on public reads), anonymous and paid limits in the description
- [x] 5.2 `.env.example` entries for the new env; AGENTS.md note in `internal/api/handler` on the gate; billing AGENTS.md note on product families

## 6. Ship

- [ ] 6.1 Check upstream source terms (Adzuna and other aggregators) against the licence text; exclude sources if required — the licence carries a general "no rights a source has not given us" clause; a per-source check is still owed
- [x] 6.2 `go vet -tags=integration ./...`, full tagged suite for billing and handler, web checks
- [ ] 6.3 PR → CI → merge → deploy; ops checklist for the user: Stripe product/prices, env, CF Transform Rule + secret, then `PUBLIC_ANON_LIMITS=strict`
