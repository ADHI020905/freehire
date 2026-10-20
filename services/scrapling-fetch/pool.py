"""A pool of warm browser sessions, keyed by (host, proxy URL).

A Cloudflare clearance belongs to one host and one exit IP, so a session is reused only for the
key it was opened for: the challenge is solved once and every later page of the crawl rides on
it. Several sessions may share a key when one host is crawled in parallel. A global cap bounds
memory (a browser session peaks at ~1.7 GB); past it, an idle session of another key is closed
to make room, and if none is idle the caller waits — but only for `acquire_timeout`, because a
caller that has given up and retried would otherwise leave its abandoned request queued, and
then spend a browser on a page nobody reads.

The factory is injected, so the pool's tests need no browser.
"""

import asyncio
import time
from dataclasses import dataclass
from typing import Any, Awaitable, Callable, Hashable, List, Optional


class PoolBusy(Exception):
    """No session came free within the acquire timeout."""


class PoolClosed(Exception):
    """The pool is shutting down."""


@dataclass
class _Entry:
    key: Hashable
    session: Any
    busy: bool
    last_used: float


class SessionPool:
    def __init__(
        self,
        factory: Callable[[Hashable], Awaitable[Any]],
        *,
        max_sessions: int,
        idle_ttl: float,
        acquire_timeout: Optional[float] = None,
        clock: Callable[[], float] = time.monotonic,
    ):
        if max_sessions < 1:
            raise ValueError("max_sessions must be at least 1")
        self._factory = factory
        self._max = max_sessions
        self._ttl = idle_ttl
        self._acquire_timeout = acquire_timeout
        self._clock = clock
        self._entries: List[_Entry] = []
        self._opening = 0  # slots reserved by a session still being opened
        self._closed = False
        self._cond = asyncio.Condition()

    def size(self) -> int:
        return len(self._entries)

    async def run(self, key: Hashable, fn: Callable[[Any], Awaitable[Any]]) -> Any:
        """Run fn on a session for key. A session whose use raised is closed, never reused."""
        entry = await self._acquire(key)
        try:
            result = await fn(entry.session)
        except BaseException:
            await self._discard(entry)
            raise
        await self._release(entry)
        return result

    async def _acquire(self, key: Hashable) -> _Entry:
        deadline = None if self._acquire_timeout is None else self._clock() + self._acquire_timeout
        evicted = None
        async with self._cond:
            while True:
                if self._closed:
                    raise PoolClosed("the pool is closed")
                for e in self._entries:
                    if e.key == key and not e.busy:
                        e.busy = True
                        return e
                if len(self._entries) + self._opening < self._max:
                    break
                evicted = self._idle_lru()
                if evicted is not None:
                    self._entries.remove(evicted)
                    break
                await self._wait(deadline)
            self._opening += 1  # from here on the slot is ours until the session is registered
        try:
            if evicted is not None:
                await _close_quietly(evicted.session)  # outside the lock: closing a browser is slow
            session = await self._factory(key)
        except BaseException:
            async with self._cond:
                self._opening -= 1
                self._cond.notify_all()
            raise
        async with self._cond:
            self._opening -= 1
            if self._closed:
                self._cond.notify_all()
                closed = True
            else:
                entry = _Entry(key=key, session=session, busy=True, last_used=self._clock())
                self._entries.append(entry)
                closed = False
        if closed:
            await _close_quietly(session)
            raise PoolClosed("the pool closed while the session was opening")
        return entry

    async def _wait(self, deadline: Optional[float]) -> None:
        if deadline is None:
            await self._cond.wait()
            return
        remaining = deadline - self._clock()
        if remaining <= 0:
            raise PoolBusy(f"no browser session came free within {self._acquire_timeout}s")
        try:
            await asyncio.wait_for(self._cond.wait(), remaining)
        except asyncio.TimeoutError:
            raise PoolBusy(f"no browser session came free within {self._acquire_timeout}s") from None

    def _idle_lru(self):
        idle = [e for e in self._entries if not e.busy]
        return min(idle, key=lambda e: e.last_used) if idle else None

    async def _release(self, entry: _Entry) -> None:
        async with self._cond:
            entry.busy = False
            entry.last_used = self._clock()
            self._cond.notify_all()

    async def _discard(self, entry: _Entry) -> None:
        async with self._cond:
            if entry in self._entries:
                self._entries.remove(entry)
            self._cond.notify_all()
        await _close_quietly(entry.session)

    async def reap(self) -> None:
        """Close every idle session unused for longer than the idle TTL."""
        now = self._clock()
        async with self._cond:
            stale = [e for e in self._entries if not e.busy and now - e.last_used > self._ttl]
            for e in stale:
                self._entries.remove(e)
            if stale:
                self._cond.notify_all()
        for e in stale:
            await _close_quietly(e.session)

    async def close_all(self) -> None:
        """Close every session and refuse new work; a session still opening closes on arrival."""
        async with self._cond:
            self._closed = True
            entries, self._entries = self._entries, []
            self._cond.notify_all()
        for e in entries:
            await _close_quietly(e.session)


async def _close_quietly(session: Any) -> None:
    """Close a session, finishing the close even if the caller is cancelled meanwhile: a browser
    left half-closed is a leaked process."""
    task = asyncio.ensure_future(session.close())
    try:
        await asyncio.shield(task)
    except asyncio.CancelledError:
        try:
            await task
        except Exception:
            pass
        raise
    except Exception:
        pass  # a session that cannot close cleanly is gone either way
