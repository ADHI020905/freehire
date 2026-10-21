## MODIFIED Requirements

### Requirement: Hosted-tier transport past the Cloudflare challenge

The adapter's pages MUST be fetched through the self-hosted Scrapling tier in its `http` mode
over the proxy, because Wellfound's search and listing pages answer 403 to a plain direct or
proxied request (measured 2026-10-20: the impersonating `http` fetcher through the proxy was
served). The adapter is registered in the crawl registry unconditionally (so classification and
cross-source dedup always know about it, the same reasoning already applied to
`bayt`/`gulftalent`/`jobleads`); without `SCRAPLING_FETCH_URL` configured, it is still present
but every crawl attempt fails loudly on the refusal, exactly as `bayt`/`gulftalent` do without
it — never a silent, provider-absent no-op.

#### Scenario: Requests are served, not challenged

- **WHEN** the adapter fetches a role-slice search page and `SCRAPLING_FETCH_URL` is configured
- **THEN** it does so through the Scrapling tier so the response is the real page rather than
  the refusal

#### Scenario: No credential means every crawl attempt fails loudly

- **WHEN** `SCRAPLING_FETCH_URL` is not configured
- **THEN** `wellfound` remains present in the crawl registry, and a crawl attempt fails with
  the refusal response rather than the provider being silently absent
