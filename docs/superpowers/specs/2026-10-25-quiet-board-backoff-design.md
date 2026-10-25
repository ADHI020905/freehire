# Quiet-board backoff: crawl a board less often while its listing does not change

Date: 2026-10-25. Status: approved design, not yet implemented.

## Problem

Every crawl of a provider re-reads every one of its boards, whether or not the board ever
changes. Thousands of boards change rarely or never: on 2026-10-25 `close-chronic-boards`
reported 7,229 boards whose crawls all succeed and yield nothing. Those are re-read every cycle on a
crawl fleet whose capacity is the host's bottleneck (see `internal/ingest/ingestsched/AGENTS.md`).

The only per-board pacing today is the FAILURE backoff (`pipeline.CooldownFor`,
`board_health.cooldown_until`): from the third consecutive failure, 6h doubling to a 24h
cap. Nothing paces a board that answers correctly but has nothing new.

## Goal

A board whose listing has not changed since its previous successful crawl is crawled
progressively less often, up to a cap of 3 days. The first crawl that sees a change resets
it to the provider's normal cadence. This is the change-rate scheduling search engines
apply to pages.

"Changed" means the SET of postings the board lists is different: a posting appeared or a
posting disappeared. An edit to a posting's content does not count.

## Design

### Signal: a listing fingerprint

After a crawl, the external ids the board's listing yielded are de-duplicated, sorted and
hashed (sha256, hex). That is the board's listing fingerprint. It is compared with the
fingerprint stored from the previous successful crawl.

It is computed in the two places a board's outcome is already recorded:

- the buffered path (`ingestFetched`), from the `raw []sources.Job` it already holds;
- the streaming path (`ingestStream`), from the ids collected inside the emit sink.

Every posting the listing named counts, whatever happened to it afterwards: ingested,
rejected by the catalogue filter, covered by a first-party ATS (aggregator gate), a liveness
`SeenRefresh`, or `Unreadable`. A posting the adapter dropped (a failed detail fetch on a
hydrating adapter) is missing from the set, so the fingerprint changes and the backoff
resets. That is the safe direction.

### Storage (one migration)

Three nullable or defaulted columns on `board_health`. Defaulted columns are a catalogue-only
change on Postgres 11+ (no table rewrite):

- `listing_fingerprint text` — the fingerprint from the last COMPLETE successful crawl.
- `quiet_streak int NOT NULL DEFAULT 0` — consecutive complete crawls with an unchanged fingerprint.
- `quiet_until timestamptz` — the board is skipped until this instant; NULL = not quiet.

`quiet_until` is a separate column from `cooldown_until` on purpose. The recovery probe
(`ClearCooldowns`, after a provider-wide outage) clears every cooled board of a provider at
once. Sharing the column would erase every quiet backoff on most runs, and it would mix
"failing" with "quiet" in the health reports.

### Policy (Go, beside `CooldownFor`)

`QuietFor(streak int) time.Duration` returns `quietBase · 2^(streak-1)`, capped at
`quietMax`. `quietBase` = 4h and `quietMax` = 72h, so the sequence is
4h, 8h, 16h, 32h, 64h, then 72h. Streak 0 means no quiet pause.

### Recording

`RecordSuccess` gains a `fingerprint string` argument. The empty string means "this crawl
cannot vouch for the whole listing". The SQL keeps everything it does today and adds:

- fingerprint empty: leave `listing_fingerprint`, `quiet_streak` and `quiet_until` untouched;
- fingerprint equal to the stored one: `quiet_streak = quiet_streak + 1`;
- otherwise (changed, or no previous fingerprint): store the new fingerprint,
  `quiet_streak = 0`, `quiet_until = NULL`.

It returns the new `quiet_streak`. As with `RecordFailure`, the Go side computes `QuietFor`
from it and sets `quiet_until` (this keeps the policy in Go, where it is tested). The
fingerprint is passed as empty, and so stays neutral, when the crawl is not complete:
`Stats.Interrupted > 0`, `Stats.Failed > 0`, or the context ended mid-crawl. Only a
complete crawl knows the whole listing.

### Skipping

`cooledDown` (fail-open) also reads `quiet_until`: a board is skipped while either pause is in
the future. A skipped quiet board is counted in a new `Stats.Quiet`, separate from
`Stats.Cooled`, and every run logs it ("skipped N quiet boards").

### Exclusions

Providers marked `selfClosing` (`sources.SelfClosingProviders`, e.g. `jobtech`) re-emit only
CHANGED postings, so a fingerprint of their emissions means nothing. They always pass an
empty fingerprint. Recency-budgeted adapters (`himalayas` and the like) see a different slice
every run, so their fingerprint changes and they are never backed off. That needs no special
case.

## Interactions verified against the code

- **Unseen sweep.** The board-scoped close acts only on boards the run finished, so a board
  skipped as quiet is not swept. When it is crawled again, every posting it still lists
  is re-stamped `last_seen_at`, and only the ones genuinely gone fall past the 48h cutoff.
- **Safety nets.** `close-chronic-boards` keys on `last_success_at` (60 days) and
  `last_yield_at` (30 days). A quiet board still succeeds at least every 72h, so neither
  net is starved. An empty board keeps yielding nothing and still ages into the empty-feed net.
- **Failure backoff.** It is untouched: a failed crawl goes through `RecordFailure` and
  `cooldown_until` exactly as today.
- **Scheduler.** It is untouched. `ingestsched` paces providers and shards; this paces boards
  inside a run.

## Testing

- `QuietFor`: the sequence above, the cap, and that streak 0 gives no pause.
- Fingerprint: order-independent, duplicate-independent, and changed by one id added or removed.
- Runner, against the fake BoardHealth: a quiet board is skipped and counted in `Stats.Quiet`.
  An unchanged complete crawl grows the streak, a changed one resets it, and an interrupted
  or failed crawl passes an empty fingerprint. A selfClosing provider never passes one.
- Integration (tagged): the `RecordSuccess` SQL for all three branches, and that
  `ClearCooldowns` leaves `quiet_until` alone.

## Out of scope

- Detecting content edits (a changed body under the same id).
- Per-provider tuning of `quietBase`/`quietMax`. Add it when a measurement asks for it.
- Turning on `close-chronic-boards --apply-empty-feed`, which is a separate operator decision.
