import { describe, expect, it } from 'vitest';

import { MEASURING, coveredPercent, formatClock, formatCount, formatEta, formatRate } from './embedProgress';

describe('embedProgress formatting', () => {
  it('renders an unmeasured figure as "measuring", never as 0', () => {
    expect(formatCount(null)).toBe(MEASURING);
    expect(formatRate(null)).toBe(MEASURING);
    expect(formatEta(null)).toBe(MEASURING);
    expect(coveredPercent(null, 100)).toBeNull();
    expect(coveredPercent(5, null)).toBeNull();
  });

  it('formats counts with separators', () => {
    expect(formatCount(1903114)).toBe('1,903,114');
    expect(formatCount(0)).toBe('0');
  });

  it('computes the covered share, one decimal, never 100 before it is', () => {
    expect(coveredPercent(1897020, 1903114)).toBe(99.6);
    expect(coveredPercent(999, 1000)).toBe(99.9);
    expect(coveredPercent(99999, 100000)).toBe(99.9); // rounding must not announce done
    expect(coveredPercent(1000, 1000)).toBe(100);
    expect(coveredPercent(0, 0)).toBeNull(); // nothing eligible is not "0% covered"
  });

  it('formats the rate per minute', () => {
    expect(formatRate(570.4)).toBe('570 / min');
    expect(formatRate(0.4)).toBe('<1 / min');
    expect(formatRate(0)).toBe('0 / min');
  });

  it('formats the ETA in minutes or hours', () => {
    expect(formatEta(641)).toBe('~11 min');
    expect(formatEta(20)).toBe('<1 min');
    expect(formatEta(7500)).toBe('~2 h 5 min');
    expect(formatEta(7200)).toBe('~2 h');
  });

  it('says nothing is left when the queue is empty, not "<1 min"', () => {
    expect(formatEta(0, 0)).toBe('nothing left');
    expect(formatEta(null, 0)).toBe('nothing left');
    expect(formatEta(null, null)).toBe(MEASURING);
  });
});

describe('formatClock', () => {
  it('prints a UTC clock so server and browser agree', () => {
    expect(formatClock('2026-10-19T09:05:00Z')).toBe('09:05 UTC');
    expect(formatClock(null)).toBe(MEASURING);
    expect(formatClock('not a date')).toBe(MEASURING);
  });
});
