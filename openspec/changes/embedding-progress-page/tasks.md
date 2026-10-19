## 1. Queries

- [x] 1.1 Add `SemanticCoverageCounts` (eligible, covered for a target model) and `SemanticOutboxClaimableDepth` to `internal/platform/db/queries/`, run `make sqlc`
- [x] 1.2 Integration test: seed open/closed/duplicate/non-tech/stale-model/stale-hash/covered postings and assert both counts; seed claimable and dead-lettered outbox rows and assert the depth

## 2. `internal/search/embedprogress`

- [x] 2.1 Add the package to `internal/platform/arch/layering/blocks.go` (search block) and an AGENTS.md stub only if the block's convention asks for one
- [x] 2.2 `Snapshot` type, cache key, `Store`/`Load` with a bounded read timeout (mirror `catalogstats`); test a missing key reads as "no snapshot", not zeros
- [x] 2.3 Sample ring: keep 10 minutes, drain rate = sum of decreases / span, no rate under 5 minutes of span; tests for the even drain, the enqueue jump, and the fresh server (spec scenarios)
- [x] 2.4 `Progress` document: combine snapshot + latest sample (`covered` clamped, `eta_seconds` absent without a positive rate, every unmeasured field `null`); table test
- [x] 2.5 Sampler (ticker, 15s) + broadcaster (subscribe returns the current document first; slow subscribers dropped, not blocking); test with a fake depth reader and a fake clock

## 3. `cmd/rollup-stats`

- [x] 3.1 Best-effort coverage pass after the existing snapshots, own `statement_timeout`, keeps the previous snapshot on failure; unit test of the publish step with a fake counter

## 4. API

- [x] 4.1 Wire the sampler/broadcaster in `cmd/server` beside the recent-jobs feed
- [x] 4.2 `GET /api/v1/stats/embeddings` handler (`{"data": ...}`); handler test incl. the no-snapshot case
- [x] 4.3 `GET /api/v1/stats/embeddings/stream` SSE handler with per-IP connect limit, reusing the SSE helpers; test the first event and 429
- [x] 4.4 Document both endpoints in `web/src/lib/docs/api-spec.ts` (the source for `docs/API.md` and the generated reference; `web/static/openapi.yaml` is a curated subset that lists no `/stats` route) and regenerate

## 5. Web

- [x] 5.1 API client method + type for the progress document
- [x] 5.2 `web/src/routes/embeddings/+page.server.ts` (best-effort load, short cache) and `+page.svelte` (progress bar, figures, "measuring…" for nulls, EventSource updates, volunteer block linking the repo and `HOW_TO_RUN_THE_CLIENT.md`) from design-system primitives
- [x] 5.3 Component test: null figures render "measuring…", never 0; an event replaces the figures
- [x] 5.4 Add `/embeddings` to the pages sitemap if public pages are listed there
- [x] 5.5 Link `/embeddings` from the site footer

## 6. Ship

- [ ] 6.1 `gofmt`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, the tagged tests for the touched packages, web lint/check/tests
- [ ] 6.2 PR, CI green, merge, release; run `cmd/rollup-stats` once by hand so the snapshot exists; open `/embeddings` on production and watch one live update
- [ ] 6.3 Merge the `freehire-ops` PR for `freehire-embed.service`/`.timer` (already installed on fsn 2026-10-19)
