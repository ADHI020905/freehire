-- users.api_until_stripe / users.api_data_until_stripe: how far a live Stripe subscription to
-- the API product reaches, per tier (API $29, Data $99). See
-- openspec/changes/commercial-api-paid-keys.
--
-- A separate product from Pro/Ultra, so separate columns: holding API access confers no Pro
-- plan and holding Pro confers no API access. Stripe is the only seller, so unlike
-- pro_until/ultra_until (0135, 0141) there are no store or granted sources and no derived
-- column — the tier is read straight from these two.
--
-- NULL means Stripe confers nothing. A nullable ADD COLUMN with no default is catalogue-only
-- in PostgreSQL 11+, so this does not rewrite the table.
ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS api_until_stripe timestamptz,
    ADD COLUMN IF NOT EXISTS api_data_until_stripe timestamptz;
