# Commercial API: paid keys, filtered feeds, change webhooks

Date: 2026-10-19. Status: design approved in conversation, awaiting spec review.

## Why

The public read API is free and IP-limited (600/min reads, 300/min agent search). Measured on
prod for 2026-10-19 (`/api/v1/` only, nginx access log):

- 24,859 distinct IPs used the API. Only 34 made more than 2,000 requests that day.
- Those 34 are almost all catalogue mirrors that name themselves in the User-Agent:
  `aarivi-catalog-sync` (206k requests, 122k of them already 429), `VivekJobBoard` (48.6k),
  `KerrasTechJobRadar` (41.8k), `freehire-dump` (22.4k), `alligator-croningest` (20.7k),
  `careerx` (19k), `OpenTalentJobsDump` (12.9k).
- They mirror by workarounds: paging `/jobs/sitemap` in 50k slices, polling
  `posted_within_days=3` once per source, or fetching every posting by slug. Nothing tells them
  what CHANGED or what CLOSED.
- The ai-job-search skill (our main API client) is 4,814 IPs averaging 14 requests/day, but it
  bursts to 200–300/min. A tight per-minute cap would break it; a daily cap would not.
- ~588k open postings were created in the last 24h across the whole catalogue.

Competitors (17 vendors, see the local research note) meter per job record: mirroring a
2M-posting catalogue costs $1k–3k/month. Change feeds start at $175–765/month; full dumps at
$1,750+ or "contact sales". Almost none state a data licence.

So the paid product is not "more requests". It is: filtered feeds, a change stream with
closures, a file snapshot, and a plain licence — at a flat price.

## Tiers

| | Free (no key) | API — $29/mo | Data — $99/mo |
|---|---|---|---|
| Read API | 120/min **and** 2,000/day per IP | 1,200/min per key | 6,000/min per key |
| Feeds (saved filter + webhook) | — | yes | yes |
| Change pull (`/changes?cursor=`) | — | yes | yes |
| Snapshot file of a feed | — | — | yes |
| Company data | yes (as today) | yes | yes |
| Commercial licence | — | yes | yes |

- Every search filter the site supports is available on a feed, on both paid tiers.
- A key that is not attached to a live API subscription is treated as no key.
- Verified search-engine bots are exempt from the free limits (see Limits).
- Feed counts per tier are a config value, not a design decision; start at 1 ($29) / 10 ($99).

## Components

### 1. Entitlement (Stripe)

A separate Stripe product "API" with two prices. It must NOT join the Pro/Ultra upgrade logic:
`decideCheckoutTarget` today treats any existing subscription as one to change in place, which
would turn an API purchase into a Pro upgrade.

- `STRIPE_API_PRICE_IDS` (starter) and `STRIPE_API_DATA_PRICE_IDS` (data).
- Columns `users.api_tier` (`'' | 'api' | 'data'`) and `users.api_until`, written by the existing
  webhook apply path and `cmd/billing-sync` reconcile, keyed on which price list the
  subscription's price is in.
- Checkout for the API product opens its own subscription and never modifies a Pro/Ultra one.
- Resolution helper `apiplan.TierOf(user, now)` in the same spirit as `plan.TierOf`.

### 2. Limits

- **Keyed callers.** On every public read route, `OptionalAuth` runs BEFORE the limiter (today it
  runs after on `/jobs/search`, so a key holder is counted by IP). A key whose owner has a live
  API tier is limited by key at the tier's rate; any other caller falls through to the IP rules.
- **Anonymous callers.** Per IP: 120/min and 2,000/day, both in Redis (existing GCRA throttler),
  for the read and agent-search budgets. `suggest` and the Talent catalogue keep their own budgets.
- **Verified bots.** A Cloudflare Transform Rule sets `X-FH-Verified-Bot: <secret>` when
  `cf.client.bot` is true. The origin listens on 0.0.0.0:443, so the header is only trusted when
  its value equals the configured secret (`VERIFIED_BOT_SECRET`); nginx forwards it, Go compares
  in constant time and skips the anonymous limits. Without the secret it is ignored.
  The zone is on Cloudflare Free, whose 5 custom WAF rules are all used; a Transform Rule lives in
  a separate entrypoint (`http_request_late_transform`) with its own quota, so it costs none of
  them — confirm the count before adding it.
- 429 responses keep `Retry-After` and `X-RateLimit-*`; the body names the paid tier.
- Rollout: lower the anonymous limits only after the paid tier is purchasable, and announce it
  in the OpenAPI description and the 429 body.

### 3. Feeds

Table `api_feeds`: `id`, `user_id`, `name`, `query` (the same canonical query string a saved
search stores, parsed by `search.FilterFromValues`), `webhook_url`, `webhook_secret`,
`cursor_ts`, `cursor_id`, `created_at`, `paused_at`, `failures`.

- Created from the UI (a "Make API feed" action beside the `/jobs` filters) or `POST /api/v1/feeds`.
- A feed must narrow the catalogue OR the owner must be on the data tier (`search.Narrows`).
- The webhook URL goes through `safehttp` (no private addresses), and a test delivery is sent on
  save.

### 4. Change events

Three event kinds, all carrying the job's `id`, `public_slug` and a `seq` cursor:

- `added` / `updated` — the full public job object (`jobview`), plus `company_slug`.
- `removed` — `{id, public_slug, reason: closed|duplicate, at}`.

**Where added/updated come from.** The jobs index gets a new filterable+sortable attribute
`updated_ts` taken from `jobs.updated_at` — NOT from index time, because the full rebuild runs
every few hours and would make every posting look updated. A feed asks the jobs index for
`<feed filter> AND updated_ts >= cursor_ts`, sorted by `(updated_ts, id)`. Because
`search-drain` pushes with a lag, each query re-reads a 30-minute overlap; events are idempotent
on `(id, updated_ts)` and the consumer is told so.

`added` vs `updated` is decided by `created_at >= cursor` at read time; clients must treat both
as upsert.

**Where removed comes from.** A closed or duplicate-suppressed posting is no longer in the jobs
index, so the feed filter cannot be asked of it there. A second index, `jobs_removed`, holds a
slim document (id, public_slug, every FILTERABLE attribute, `removed_ts`, `reason`) for 7 days:

- The `search_delete_outbox` drain writes the slim doc before deleting from `jobs`.
- The full rebuild, which silently drops duplicate-suppressed and newly-ineligible postings,
  writes a slim doc for every id that was in the old index and is absent from the new one.
- A daily pass deletes `removed_ts < now - 7d`.
- `jobs_removed` uses the SAME filterable-attribute list and the same `FilterFromValues`, so a
  feed's filter is one rule evaluated in two indexes, never re-implemented in Go.

A consumer that has not synced for more than 7 days is told to re-snapshot (the change endpoint
answers 410 with that instruction).

### 5. Delivery

`cmd/feed-deliver`, every 2 minutes, `Type=oneshot`. For each active feed:

1. read the next batch (up to 500 events) from both indexes after the feed's cursor;
2. POST it as JSON, signed `X-FH-Signature: sha256=HMAC(webhook_secret, body)`, with the batch's
   last `seq`;
3. on 2xx advance the cursor; otherwise keep it, back off, and pause the feed after N
   consecutive failures (owner notified by email).

`GET /api/v1/feeds/{id}/changes?cursor=` returns the same batches for pull consumers and for
catch-up after an outage. Pull and push share the cursor reader; push only additionally stores
the cursor.

### 6. Snapshot (data tier)

`POST /api/v1/feeds/{id}/snapshot` starts a build; `GET` returns its status and a download URL.
The worker walks the jobs index with the feed filter keyset-style (`id > last`, sort by id) to get
past Meilisearch's pagination window, writes gzipped JSONL to object storage, and stamps the file
with the `seq` to start the change stream from. At most one build per feed per day.

### 7. Licence and docs

- Terms gain a "Data licence" section: paid keys may store and display postings in their own
  product; reselling the raw data as a dataset is not allowed; each displayed posting links to
  its freehire page or carries "via freehire.me".
- `openapi.yaml` gains the `bearerAuth` scheme, the feed endpoints and the change event schema.
- A `/api` page with tiers, limits and a three-step quickstart (create feed → snapshot or page →
  webhook).

## Delivery order

1. Entitlement + key-aware limits + anonymous limits + verified bots + licence. Sellable alone.
2. Feeds, `updated_ts`, `jobs_removed`, change pull, webhook delivery.
3. Snapshot files and the $99 tier.

Each step is its own plan and PR.

## Risks and open items

- Adding `updated_ts` as filterable/sortable is a settings-before-binary change and needs a full
  rebuild (see internal/search/search/AGENTS.md).
- Does `backfill-derive` bump `jobs.updated_at`? If it does, a backfill emits `updated` for every
  touched posting — acceptable, but webhook volume spikes; if it does not, facet changes from a
  dictionary fix never reach feeds. Verify before step 2.
- Whole-catalogue feeds see ~600k events/day; delivery must batch and a slow consumer must not
  delay others (per-feed cursor, bounded batches per run).
- Licence vs upstream source terms (e.g. Adzuna attribution) must be checked before step 1 ships.
- Non-IT postings are in the catalogue; feeds include them unless the filter says `is_tech`.
