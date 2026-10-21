import { describe, it, expect } from 'vitest';
import { buildPayload, nextSession, SESSION_TIMEOUT_MS } from './analytics';

describe('nextSession', () => {
  const now = 1_800_000_000_000;

  it('starts a session when there is none', () => {
    expect(nextSession(undefined, now)).toEqual({ id: String(now / 1000), lastAt: now });
  });

  it('keeps the session while activity continues', () => {
    const prev = { id: 'abc', lastAt: now - SESSION_TIMEOUT_MS + 1 };
    expect(nextSession(prev, now)).toEqual({ id: 'abc', lastAt: now });
  });

  // GA's own web tag ends a session after 30 idle minutes; matching it keeps the
  // extension's sessions comparable with the site's.
  it('starts a new session after the idle timeout', () => {
    const prev = { id: 'abc', lastAt: now - SESSION_TIMEOUT_MS };
    expect(nextSession(prev, now).id).toBe(String(now / 1000));
  });
});

describe('buildPayload', () => {
  // The site and the panel share one stream; `surface` is the only thing telling them
  // apart, and session/engagement are what GA needs to count the event as a visit.
  it('marks the event as the extension and carries the session GA needs', () => {
    const body = buildPayload('client-1', 'sess-1', 'plan_refused', { feature: 'assistant' });
    expect(body).toEqual({
      client_id: 'client-1',
      events: [
        {
          name: 'plan_refused',
          params: { feature: 'assistant', surface: 'extension', session_id: 'sess-1', engagement_time_msec: 100 },
        },
      ],
    });
  });
});
