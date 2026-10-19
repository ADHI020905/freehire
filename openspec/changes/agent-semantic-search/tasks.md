## 1. Chunk vectors in the database layer

- [x] 1.1 Add sqlc query `ListSemanticChunksForJobs(job_ids bigint[])` in `internal/platform/db/queries/semantic.sql` returning `(job_id, chunk_index, embedding)` ordered by job, chunk; run `make sqlc`
- [x] 1.2 Add a migration with a statement-level `AFTER INSERT` trigger on `job_semantic_chunks` that enqueues `DISTINCT job_id` into `search_outbox` (`job_posted_at` from `jobs`, open + canonical only, `ON CONFLICT (job_id) DO NOTHING`); `pnpm check:sql` passes
- [x] 1.3 Integration test: `cmd/embed`'s `CompleteOpen` and a raw-SQL replace shaped like the volunteer broker's `promote.Replace` each leave exactly one `search_outbox` row; re-embedding an already-queued job does not fail; the closed-job delete path enqueues nothing

## 2. Index documents carry `desc` vectors

- [x] 2.1 Change `JobDocument.Vectors` so it carries `skills` (one vector) and `desc` (many); a unit test pins the `skills` JSON as byte-identical to today
- [x] 2.2 Declare the `desc` `userProvided` embedder (768 dims) in the jobs index settings next to `skills`; add it to `search-settings-drift`'s expected set
- [x] 2.3 Thread a `map[int64][][]float32` of chunk vectors into the document builder; a job with no chunks gets no `desc` key
- [x] 2.4 Wire the batch chunk read into `cmd/reindex`, `cmd/search-drain` and `internal/ingest/linkimport`
- [x] 2.5 Integration test (real Meilisearch): a document pushed through the drain's partial-update path keeps both `skills` and `desc` vectors; the embedder survives `EnsureIndex` and `Rebuild.Prepare` (extend `embedder_integration_test.go`)

## 3. Query embed and hybrid request

- [x] 3.1 Add `search.QueryEmbedder` (`NewQueryEmbedder` returns nil without a URL): one `query: `-prefixed input, own deadline (`EMBED_QUERY_TIMEOUT_MS`, default 400), no retry; unit tests with an httptest TEI for success, timeout, non-200 and wrong width
- [x] 3.2 Add `SearchParams.SemanticVector` and `ScoreThreshold`; `buildSearchRequest` emits the `desc` hybrid (`semanticRatio` constant, `rankingScoreThreshold`) and keeps the `skills` match sort unchanged; unit tests for both branches and for "no vector → no hybrid"

## 4. OJCP surface

- [x] 4.1 Add `Semantic *bool` to `ojcp.SearchInput` and `Semantic string` (`omitempty`) to `ojcp.SearchJobsResponse`; schema-oracle test still validates a response carrying it
- [x] 4.2 In `ojcpHandlers.SearchJobs`: flag + non-empty query → `EmbedQuery` → hybrid search, report `applied`; embed error → keyword search, report `unavailable`; flag + empty query → keyword search, `semantic` in `ignored_params`; no flag → request unchanged
- [x] 4.3 Handler tests with fake searcher + fake embedder covering all four paths, asserting no embed call when the flag is absent and that filters reach the hybrid request
- [x] 4.4 MCP transport: `search_jobs` input schema lists `semantic`; an in-memory SDK test calls it with `semantic: true` and the response passes the SDK's output validation; REST and MCP answers match
- [x] 4.5 Wire the embedder into `newOJCPHandlers` from config (`EMBED_URL`/`EMBED_API_KEY`); server boots and answers `unavailable` without them

## 5. Docs and checks

- [x] 5.1 Update `internal/search/search/AGENTS.md` (second embedder, rollout order, threshold), `internal/ai/embed/AGENTS.md` (trigger, volunteer broker as bulk embedder), `internal/api/ojcp/AGENTS.md` (the extension fields)
- [x] 5.2 `gofmt -l .` empty, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`; full tagged suite for `internal/search/search`, `internal/platform/db`, `internal/api/handler`

## 6. Rollout (prod, in order)

- [x] 6.1 Deploy; confirm the trigger migration applied and the flag answers `unavailable` (2026-10-19: 0182 applied, trigger enabled, `semantic: true` → `unavailable`)
- [x] 6.2 ~~Stop the server-side bulk embed~~ — dropped by the owner's decision (2026-10-19): `cmd/embed` keeps running on the shared TEI and the keyword fallback absorbs the contention. The `freehire-embed-supervisor` backfill drained and exited on its own at 05:54 UTC; the incremental `freehire-embed.timer` remains
- [ ] 6.3 Full rebuild with `desc` (deferred by the owner, 2026-10-19). NOTE: the scheduled `freehire-reindexw` run now uses the new binary, so ANY scheduled rebuild adds the vectors — and with `EMBED_URL` already set, semantic search goes live the moment it swaps. Set `SEMANTIC_SCORE_THRESHOLD` (6.6) before that, or skip the scheduled run. Previous rebuild took 6h20m against a 12h unit timeout; record time and index size; `search-settings-drift` reports 0
- [x] 6.4 `EMBED_URL`/`EMBED_API_KEY` reach the API server already — they sit in the shared `/opt/freehire/.env`. Verified 2026-10-19 by the server log: the query embedded, the hybrid was refused (`Cannot find embedder with name desc`) and fell back. Query embed 0.08s with no bulk run on the TEI
- [ ] 6.5 Hand-check: same query with/without `semantic`, with filters, with TEI unreachable (expect `unavailable`)
- [ ] 6.6 Pick `rankingScoreThreshold` and `semanticRatio` from sampled agent queries; then announce the field
