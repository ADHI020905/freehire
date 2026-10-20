# Scrapling fetch tier — design

Date: 2026-10-20. Status: approved for planning.

## Problem

A set of sources cannot be crawled today, for reasons no current transport answers:

- **The hosted Firecrawl tier is out of credits** (`api status 402` on wantapply since
  2026-10-18), and bayt / gulftalent have had no successful crawl since 2026-09-07.
- **InfoJobs** refuses the crawler host's IP outright (405), and through `SOURCES_PROXY_URL`
  serves five listing pages before a GeeTest challenge.
- Several other sources answer 403 or time out on our plain Go client.

## What the spike measured (2026-10-20, on host2)

Scrapling 0.5 (`ghcr.io/d4vinci/scrapling`), two fetchers: `http` (curl_cffi, Chrome TLS
fingerprint impersonation) and `browser` (`StealthySession`, patchright Chromium,
`solve_cloudflare=True`). Each was run direct (host2 IP) and through `SOURCES_PROXY_URL`.

| source | http direct | http proxy | browser direct | browser proxy |
|---|---|---|---|---|
| infojobs (listing, 20 pages) | 0/20 (405) | 2/20 (challenge from page 3) | 0/20 (405) | **20/20**, 7.5 s/page |
| bayt (listing, 20 pages) | 0/20 (403) | 0/20 (403) | **20/20**, 9.4 s | **20/20**, 7.2 s |
| hh (vacancy detail, 20) | **20/20**, 0.5 s | 20/20, 1.8 s | — | — |
| gulftalent | 403 | **200** | **200** | timeout |
| wantapply (sitemap) | 403 | 403 | **200** | **200** |
| wellfound (role page) | 403 | **200** | 200 (159 s) | timeout |
| startupjobs | 403 | 403 | **200** | timeout |
| cleverstaff (API) | **200** | 403 | 200 | 403 |
| kontur | timeout | **200** | timeout | 200 |
| tbank | TLS: unknown CA | TLS: unknown CA | **200** | timeout |
| echojobs | 403 | 403 | 403 | 403 |
| neogov | timeout | not the listing | timeout | timeout |
| dodo | connect timeout | connect timeout | timeout | timeout |

The first Cloudflare-protected page costs ~40-80 s (the challenge is solved once); later pages
on the same warm session cost ~6-9 s. A browser session peaks at ~1.7 GB RSS.

## Decision

Replace the hosted Firecrawl tier with a **self-hosted Scrapling fetch service** on host2, and
move every source the spike showed it rescues onto it. Not on it: the 200+ adapters a plain Go
client already serves, and echojobs / neogov / dodo, which Scrapling did not rescue.

## Components

### 1. `services/scrapling-fetch` (Python, new)

A small HTTP service in the shape of `services/pii-filter`, built on the pinned Scrapling image.

- `POST /fetch` — request: `{url, method, headers, body, mode: "http"|"browser", proxy: bool}`;
  response: `{status, url, headers, body}` (`body` base64, `url` the final URL after
  redirects). Upstream status codes are returned as data, never as the service's own status:
  the service answers 200 for a completed fetch and 5xx only for its own failure.
- `GET /health`.
- **The proxy URL lives in the service's environment** (`SOURCES_PROXY_URL`); a caller only
  says `proxy: true`, so the credential never crosses the wire.
- **Warm browser sessions**, keyed by `(host, proxy)`, reused across requests and closed after
  an idle TTL, so a Cloudflare challenge is solved once per session rather than per page.
  A global cap on live browsers (`SCRAPLING_MAX_BROWSERS`, default 3) queues the rest.
- `browser` mode supports `GET` only (a navigation); `http` mode supports any method.
- Bound to `127.0.0.1` (the crawlers run on the same host), bearer-token auth
  (`SCRAPLING_FETCH_TOKEN`), request-body and response-body caps.

### 2. Go transport (`internal/platform/scraplingfetch`, new)

An `http.RoundTripper` that forwards each request to the service and rebuilds the upstream
response from its answer. `sources` builds an ordinary `*Client` over it, so retries, body caps,
`StatusError` and every getter (`GetJSON`, `GetHTML`, `GetText`, `GetXML`, POST paths) are the
existing code, not a second copy. The transport carries the per-provider `mode` and `proxy`.
Platform block, next to `safehttp` — transport, no domain knowledge; added to
`internal/platform/arch/layering/blocks.go`.

### 3. `scraplingProviders` (in `internal/ingest/sources`, replaces `firecrawlProviders`)

The allowlist, each entry naming its mode and proxy, applied by `ApplyScraplingEgress` at
`cmd/ingest` startup. Without `SCRAPLING_FETCH_URL` it is a no-op and every provider keeps
today's transport — that is also the rollback.

| provider | mode | proxy | part |
|---|---|---|---|
| infojobs | browser | yes | whole crawl |
| bayt | browser | no | whole crawl |
| gulftalent | browser | no | whole crawl — its sitemap answered a Cloudflare challenge to `http` + proxy on 2026-10-20 |
| wantapply | browser | no | sitemap only; pages stay on the free `.cy` client, as today |
| wellfound | http | yes | whole crawl |
| hh | http | no | detail hydration only, as today with Firecrawl |
| startupjobs | browser | no | whole crawl |
| cleverstaff | http | no | whole crawl |
| kontur | http | yes | whole crawl |

Each row's mode is the cheapest one the spike saw pass; the plan re-verifies each one live
before its row lands.

### 4. Removal of the hosted Firecrawl tier

`internal/platform/firecrawl`, `firecrawltier.go`, `FIRECRAWL_*` configuration and the docs
describing them go. Every provider it served moves to the table above.

## InfoJobs specifics

- Listing via browser + proxy. 2.9k pages at ~7.5 s is ~6 h on one session, so its pages are
  spread across up to `SCRAPLING_MAX_BROWSERS` sessions; cadence stays daily with a timeout
  sized to the measured run.
- Detail pages were not part of the spike. The plan tests them through browser + proxy; if they
  pass, hydration (removed in #3334) comes back for new offers, otherwise offers keep the teaser.

## Error handling

- Service unreachable or 5xx → a transport error, which the existing retry and board-health
  cooldown already treat as a failed crawl. A crawl never falls back to the direct client.
- A challenge page that comes back 200 is the adapter's to detect, as today (InfoJobs'
  missing props blob, #3336). The service does not guess.

## Testing

- Service: unit tests for request validation, the body caps, session keying and idle eviction
  (fetchers faked).
- Go transport: `httptest` server standing in for the service — request encoding, response
  rebuild, status passthrough, transport errors.
- Each migrated provider: one live fetch through the deployed service before its row is merged.

## Deployment

Image built from `services/scrapling-fetch/Dockerfile` (pinned Scrapling tag) on host2, run by
a systemd unit that lives in `freehire-ops` (`provision/host2/`), not in this repo.
`release.sh` does not carry it, so the first deploy and any image bump are manual.

## Out of scope

echojobs (Vercel checkpoint), neogov, dodo, energyjobline (its sitemap moved — an adapter fix),
jobleads (its POST search answers 503), wuzzuf / workatastartup / naukri (not measured), and
tbank: its API is a POST, which browser mode cannot send, and the `http` fetcher refuses its
certificate chain ("self signed certificate in certificate chain") — a trust-store question,
not an anti-bot one.
