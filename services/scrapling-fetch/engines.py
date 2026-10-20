"""The two Scrapling-backed fetchers the service serves. Scrapling is imported lazily, so the
rest of the service (and its tests) loads without it."""

from typing import Optional
from urllib.parse import urlsplit

from app import FetchError, FetchRequest, FetchResult
from pool import SessionPool

# Headers a caller may not pass through. User-Agent would undo the fetcher's browser
# impersonation (the Go client identifies itself as freehire); the rest are the transport's.
_DROPPED = frozenset({"user-agent", "host", "content-length", "accept-encoding", "connection"})

# Response headers that frame the upstream's bytes. The fetcher returns the body decoded and
# whole, so these would describe bytes the caller never receives.
_FRAMING = frozenset({"content-encoding", "content-length", "transfer-encoding"})

HTTP_TIMEOUT_SECONDS = 40
BROWSER_TIMEOUT_MS = 90_000


def forwardable_headers(headers: dict) -> dict:
    return {k: v for k, v in headers.items() if k.lower() not in _DROPPED}


def _result(page) -> FetchResult:
    headers = getattr(page, "headers", None) or {}
    return FetchResult(
        status=int(page.status),
        url=str(page.url),
        headers={str(k): str(v) for k, v in dict(headers).items() if str(k).lower() not in _FRAMING},
        body=bytes(page.body),
    )


async def http_fetch(req: FetchRequest, proxy_url: Optional[str]) -> FetchResult:
    """One request through curl_cffi with Chrome's TLS fingerprint. No retries: the Go client
    owns the retry policy."""
    from scrapling.fetchers import AsyncFetcher

    send = {"GET": AsyncFetcher.get, "POST": AsyncFetcher.post, "PUT": AsyncFetcher.put, "DELETE": AsyncFetcher.delete}[req.method]
    kwargs = dict(
        impersonate="chrome",
        headers=forwardable_headers(req.headers),
        proxy=proxy_url,
        timeout=HTTP_TIMEOUT_SECONDS,
        follow_redirects=True,
        retries=1,
    )
    if req.body and req.method in ("POST", "PUT"):
        kwargs["data"] = req.body
    try:
        return _result(await send(req.url, **kwargs))
    except Exception as e:
        raise FetchError(f"http fetch {req.url}: {type(e).__name__}: {e}") from e


def browser_fetcher(pool: SessionPool):
    """A fetcher that navigates on a warm stealth session for (host, proxy) from the pool."""

    async def fetch(req: FetchRequest, proxy_url: Optional[str]) -> FetchResult:
        key = (urlsplit(req.url).hostname, proxy_url)

        async def navigate(session):
            return _result(await session.fetch(req.url))

        try:
            return await pool.run(key, navigate)
        except Exception as e:
            raise FetchError(f"browser fetch {req.url}: {type(e).__name__}: {e}") from e

    return fetch


async def open_stealth_session(key) -> object:
    """The pool's factory: a started stealth browser that solves Cloudflare challenges."""
    from scrapling.fetchers import AsyncStealthySession

    _, proxy_url = key
    kwargs = dict(headless=True, solve_cloudflare=True, timeout=BROWSER_TIMEOUT_MS, retries=1)
    if proxy_url:
        kwargs["proxy"] = proxy_url
    session = AsyncStealthySession(**kwargs)
    await session.start()
    return session
