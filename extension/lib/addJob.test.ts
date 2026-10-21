import { describe, it, expect } from 'vitest';
import { offersAddJob } from './addJob';

describe('offersAddJob', () => {
  it('offers the button on a recognised ATS board', () => {
    expect(offersAddJob({ atsBoard: true, jobPostingMarkup: false })).toBe(true);
  });

  it('offers the button on a page marked up as a JobPosting', () => {
    expect(offersAddJob({ atsBoard: false, jobPostingMarkup: true })).toBe(true);
  });

  it('offers nothing on an arbitrary page', () => {
    expect(offersAddJob({ atsBoard: false, jobPostingMarkup: false })).toBe(false);
  });
});
