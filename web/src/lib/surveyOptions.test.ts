import { describe, expect, it } from 'vitest';
import {
  INCOME_CURRENCIES,
  INCOME_PERIODS,
  JOB_CHALLENGE_OPTIONS,
  JOB_CHALLENGE_OTHER,
  JOB_SEARCH_STAGE_OPTIONS,
  incomeScale,
} from './surveyOptions';

// These values are validated server-side against internal/dict/vocab, and anything outside
// it is a 400. A drifted value is therefore not a cosmetic bug — it is a step the candidate
// can never answer. Locked here against the exact strings docs/API.md publishes.
describe('the vocabularies match the server', () => {
  it('offers exactly the job-search stages the API accepts', () => {
    expect(JOB_SEARCH_STAGE_OPTIONS.map((o) => o.value).sort()).toEqual(
      ['employed_looking', 'exploring', 'not_started', 'searching'].sort(),
    );
  });

  it('offers exactly the challenges the API accepts', () => {
    expect(JOB_CHALLENGE_OPTIONS.map((o) => o.value).sort()).toEqual(
      ['english', 'other', 'recruiter_contact', 'technical_interviews', 'working_abroad'].sort(),
    );
  });

  it('offers only periods the salary vocabulary knows', () => {
    for (const period of INCOME_PERIODS) {
      expect(['year', 'month', 'day', 'hour']).toContain(period.value);
    }
  });
});

describe('the option lists are well-formed', () => {
  it('gives every option a label and a unique value', () => {
    for (const list of [JOB_SEARCH_STAGE_OPTIONS, JOB_CHALLENGE_OPTIONS, INCOME_PERIODS]) {
      const values = list.map((o) => o.value);
      expect(new Set(values).size).toBe(values.length);
      for (const o of list) expect(o.label.trim()).not.toBe('');
    }
  });

  // The note field is revealed only for this member, and the server rejects a note sent
  // with any other — so an options list without it makes the free-text answer unreachable.
  it('includes the member the free-text note hangs off', () => {
    expect(JOB_CHALLENGE_OPTIONS.map((o) => o.value)).toContain(JOB_CHALLENGE_OTHER);
  });
});

describe('incomeScale', () => {
  it('scales the ceiling with the period', () => {
    expect(incomeScale('USD', 'year').max).toBeGreaterThan(incomeScale('USD', 'month').max);
  });

  // One scale for every currency topped a ruble salary out at 30,000 a month.
  it('gives the ruble a range sized to the ruble', () => {
    expect(incomeScale('RUB', 'month').max).toBeGreaterThanOrEqual(1_000_000);
  });

  // Not a fallback worth thinking about — but an unknown value must not yield NaN or
  // Infinity into a slider's attributes.
  it('falls back to the major-currency monthly scale for anything else', () => {
    expect(incomeScale('JPY', 'fortnight')).toEqual(incomeScale('USD', 'month'));
  });

  it('keeps every ceiling a whole number of steps, so the top stop is reachable', () => {
    for (const c of INCOME_CURRENCIES) {
      for (const p of INCOME_PERIODS) {
        const { max, step } = incomeScale(c.value, p.value);
        expect(max % step).toBe(0);
      }
    }
  });
});
