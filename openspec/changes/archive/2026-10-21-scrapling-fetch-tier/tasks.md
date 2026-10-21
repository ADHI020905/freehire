## 1. Fetch service (`services/scrapling-fetch`)

- [x] 1.1 Request parsing and validation: required `url`, `mode` in {http, browser}, browser mode GET-only (400), `proxy: true` without a configured proxy (400), request-body cap — with unit tests
- [x] 1.2 Bearer-token auth (401 without `SCRAPLING_FETCH_TOKEN`), `/health`, upstream-response cap, upstream status returned as data and service failures as 5xx — with tests over the starlette app and faked fetchers
- [x] 1.3 `http` mode over `AsyncFetcher` (impersonate chrome, method/headers/body passthrough, final URL, response headers)
- [x] 1.4 Browser session pool keyed by `(host, proxy, slot)`: reuse, idle TTL eviction, global `SCRAPLING_MAX_BROWSERS` cap with queueing, a crashed session discarded — with tests over a fake session factory
- [x] 1.5 `browser` mode over `AsyncStealthySession` (`solve_cloudflare`, timeout) using the pool
- [x] 1.6 Dockerfile `FROM` the pinned digest, entrypoint, README (run, env, curl example); bind `127.0.0.1`
- [x] 1.7 CI: run the service's tests and hadolint the Dockerfile in the existing jobs

## 2. Go transport (`internal/platform/scraplingfetch`)

- [x] 2.1 `RoundTripper`: encode the request (method, URL, headers, body, mode, proxy) to `/fetch` with the bearer token; rebuild `*http.Response` (status, headers, body, final URL) — `httptest` tests
- [x] 2.2 Service 5xx / unreachable / malformed answer surface as transport errors — tests
- [x] 2.3 Register the package in `internal/platform/arch/layering/blocks.go`; AGENTS.md for the package

## 3. Allowlist and wiring (`internal/ingest/sources`)

- [x] 3.1 `NewScraplingClient(mode, proxy)` building an ordinary `*Client` over the transport, with a timeout sized for a browser fetch
- [x] 3.2 `scraplingProviders` + `ApplyScraplingEgress` (no-op without `SCRAPLING_FETCH_URL`; fails fast when the URL is set without a token) — tests mirroring the proxy tier's
- [x] 3.3 Call it from `cmd/ingest/main.go` in place of `ApplyFirecrawlEgress`
- [x] 3.4 Move bayt, gulftalent, wellfound, wantapply (sitemap), hh (detail) onto the allowlist with the modes in the spec; adapt their constructors where they took `*firecrawlClient`
- [x] 3.5 Add infojobs (browser, proxy; it stays in `proxiedProviders` as the fallback when the service is unset), startupjobs (browser, its site pages only), cleverstaff (http) and kontur (http, proxy)

## 4. Remove the hosted Firecrawl tier

- [x] 4.1 Delete `internal/platform/firecrawl` and `firecrawltier.go`; drop it from `blocks.go`; remove `FIRECRAWL_*` from `.env.example` and config
- [x] 4.2 Update docs: root AGENTS.md module table, `internal/ingest/sources/AGENTS.md` hosted-tier section, `hhru.go`/`proxy.go` comments that name Firecrawl

## 5. Deploy and verify

- [x] 5.1 Build the image on host2 and start it under a systemd unit written in `freehire-ops` (`provision/host2/`); `/health` answers
- [x] 5.2 Live-verify each allowlisted provider through the deployed service with its configured mode (one real page each); correct any row whose mode no longer passes
- [x] 5.3 InfoJobs detail pages through browser + proxy: if they pass, restore hydration for unseen offers (with a test); otherwise record the measurement in the adapter header
- [x] 5.4 InfoJobs parallel sessions (listing pages fetched 3 at a time — done in code): measure a full listing crawl's duration; set its `ingest_schedule` timeout to fit
- [x] 5.5 Set `SCRAPLING_FETCH_URL` / `SCRAPLING_FETCH_TOKEN` on host2, deploy, and confirm each provider's next run in `board_health`

## 6. Proxy rotation

- [x] 6.1 `ProxyConfig` in the service: a rotating gateway for http mode and a sticky `{n}` pool for browser sessions, falling back to `SOURCES_PROXY_URL` — with tests
- [x] 6.2 Retry a 403/429 through the rotating gateway (3 attempts); retire a refused browser session so the next takes another sticky IP — with tests
- [x] 6.3 Deploy: rebuild the image, add the rotating and sticky settings to `/opt/freehire/env/scrapling-fetch.env`, restart, and confirm wellfound's boards through it
