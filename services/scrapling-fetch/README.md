# scrapling-fetch

The fetch service behind the ingest crawl's Scrapling tier (`internal/platform/scraplingfetch`,
`scraplingProviders` in `internal/ingest/sources`). It fetches pages for the sources a plain Go
client cannot reach — an IP the source refuses, a TLS fingerprint it refuses, a Cloudflare
challenge — and hands back exactly the bytes the adapter would have read.

Two modes, both from [Scrapling](https://github.com/D4Vinci/Scrapling):

- `http` — one request through curl_cffi with Chrome's TLS fingerprint. GET, POST, PUT, DELETE.
  ~0.2-2 s.
- `browser` — a navigation in a stealth Chromium session that solves Cloudflare challenges.
  GET only, and the caller's headers are not sent: the browser presents its own, which is the
  point of using it. The first page on a host costs the challenge (measured: bayt ~110 s); later pages on
  the same warm session ~5-9 s.

## API

```
POST /fetch   Authorization: Bearer $SCRAPLING_FETCH_TOKEN
  {"url": "...", "method": "GET", "headers": {...}, "body": "<base64>", "mode": "http"|"browser", "proxy": false}
  -> 200 {"status": 403, "url": "<final url>", "headers": {...}, "body": "<base64>"}   upstream answer, as data
  -> 400/401/413 {"error": "..."}                                                      nothing was fetched
  -> 502 {"error": "..."}                                                              the fetch itself failed
GET /health -> {"status": "ok"}
```

The upstream status is DATA: a 403 page comes back with HTTP 200 and `"status": 403`. Only a
fetch that could not complete is a 502. `proxy: true` egresses through `SOURCES_PROXY_URL`, which
only the service reads — no proxy credential crosses the wire. The caller's `User-Agent` and
transport headers are dropped (they would undo the impersonation), and the returned headers omit
`Content-Encoding`/`Content-Length`/`Transfer-Encoding`, because the body comes back decoded.

## Browser sessions

One warm session per `(host, proxy)`, reused across requests so a challenge is solved once per
crawl; a host crawled in parallel gets several. At most `SCRAPLING_MAX_BROWSERS` live at once
(~1-1.7 GB each); past that an idle session of another host is closed, and if none is idle the
request waits — for at most 120 s, after which it fails with a 502 rather than queue on behalf
of a caller that has already given up. A session unused for `SCRAPLING_IDLE_SECONDS` is closed; a session whose page
raised is discarded.

## Configuration

| variable | default | |
|---|---|---|
| `SCRAPLING_FETCH_TOKEN` | — | required |
| `SCRAPLING_FETCH_ADDR` | `127.0.0.1:8098` | the crawlers run on the same host |
| `SCRAPLING_MAX_BROWSERS` | `3` | |
| `SCRAPLING_IDLE_SECONDS` | `600` | |
| `SOURCES_PROXY_URL` | — | what `proxy: true` egresses through |

## Run

```
docker build -t freehire/scrapling-fetch services/scrapling-fetch
docker run --network host -e SCRAPLING_FETCH_TOKEN=... -e SOURCES_PROXY_URL=... freehire/scrapling-fetch
curl -s 127.0.0.1:8098/fetch -H "Authorization: Bearer $TOKEN" \
  -d '{"url":"https://www.bayt.com/en/uae/jobs/","mode":"browser"}' | jq '.status'
```

On host2 it runs under a systemd unit kept in `freehire-ops` (`provision/host2/`). `release.sh`
does not build or restart it: an image bump is a manual `docker build` and unit restart.

## Test

```
pip install -r requirements-test.txt
python -m pytest -q
```

The tests fake both fetchers and the session factory, so they need neither Scrapling nor a
browser. What they cannot cover — that a real page passes a real challenge — is checked live
against the deployed service.
