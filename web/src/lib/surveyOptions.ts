// The labels for the onboarding survey's two closed vocabularies.
//
// The VALUES here must match internal/dict/vocab exactly — the server validates against
// that list and rejects anything else, so a typo in a value is a step that can never be
// answered. `surveyOptions.test.ts` locks each list against the strings the API documents.
//
// The labels are the wizard's alone: the server never sends prose, and it should not — a
// vocabulary member is an identifier, and how it is worded to a candidate is a product
// decision that changes far more often than the value does.

export interface SurveyOption {
  value: string;
  label: string;
}

/** How far along the candidate says their search is. Ordered by commitment: the two who are
 *  actively looking come first, so the most common answer is not at the bottom. */
export const JOB_SEARCH_STAGE_OPTIONS: SurveyOption[] = [
  { value: 'searching', label: "I'm looking for a job now and could use help" },
  { value: 'not_started', label: "I haven't started yet, but I want to begin" },
  { value: 'employed_looking', label: "I'm already working and looking for something better" },
  { value: 'exploring', label: "I'm just looking around — it's not a priority right now" },
];

/** The single biggest thing in the candidate's way. Most of the audience writes in English,
 *  so the language comes after the obstacles every candidate shares — first in the list, it
 *  read as an assumption about who was answering. */
export const JOB_CHALLENGE_OPTIONS: SurveyOption[] = [
  { value: 'recruiter_contact', label: 'Getting recruiters to reply' },
  { value: 'technical_interviews', label: 'Passing technical interviews' },
  { value: 'working_abroad', label: 'Finding work abroad' },
  { value: 'english', label: 'English' },
  { value: 'other', label: 'Something else' },
];

/** The vocabulary member that admits a written note. Mirrors vocab.JobChallengeOther; the
 *  note field is revealed only for this value, and the server rejects a note sent with any
 *  other. */
export const JOB_CHALLENGE_OTHER = 'other';

/** The currencies a candidate may state a figure in — the same four the job filter's
 *  currency facet offers, so a stated expectation and a posting's salary are comparable
 *  without a conversion nobody has the rates for. */
export const INCOME_CURRENCIES: SurveyOption[] = [
  { value: 'USD', label: 'USD' },
  { value: 'EUR', label: 'EUR' },
  { value: 'GBP', label: 'GBP' },
  { value: 'RUB', label: 'RUB' },
];

/** Mirrors vocab.SalaryPeriodValues, minus the two nobody states a salary in when asked
 *  about their own pay. `day` and `hour` exist because a POSTING can quote them; a
 *  candidate answering "what do you earn" does not. */
export const INCOME_PERIODS: SurveyOption[] = [
  { value: 'month', label: 'per month' },
  { value: 'year', label: 'per year' },
];

/** One slider's range: its ceiling and its step, in the stated currency and period.
 *
 *  The step is coarse on purpose: the question is what bracket someone is in, and a control
 *  that invites a to-the-unit answer implies a precision the answer does not have.
 *
 *  The last stop is the maximum itself, which the UI labels as an open-ended "or more"
 *  rather than as a figure — a candidate at the top of the range has not told us their
 *  salary is exactly this. */
interface IncomeScale {
  max: number;
  step: number;
}

// USD, EUR and GBP are close enough in value to share one scale. RUB is not — one scale for
// all four topped a ruble salary out at 30,000 a month, under what most of the market earns —
// so it carries its own, sized to the ruble rather than converted from the dollar: a rate
// moves daily, and a ceiling that drifts with it would move under a stored answer.
const MAJOR_CURRENCY_SCALE = {
  month: { max: 30_000, step: 500 },
  year: { max: 360_000, step: 500 },
} as const;

const INCOME_SCALES: Record<string, { month: IncomeScale; year: IncomeScale }> = {
  USD: MAJOR_CURRENCY_SCALE,
  EUR: MAJOR_CURRENCY_SCALE,
  GBP: MAJOR_CURRENCY_SCALE,
  RUB: {
    month: { max: 1_500_000, step: 10_000 },
    year: { max: 18_000_000, step: 50_000 },
  },
};

/** The range for the selected currency and period. An unknown currency takes the major
 *  currencies' scale and an unknown period the monthly one, so neither can put NaN into a
 *  slider's attributes. */
export function incomeScale(currency: string, period: string): IncomeScale {
  const scales = INCOME_SCALES[currency] ?? MAJOR_CURRENCY_SCALE;
  return period === 'year' ? scales.year : scales.month;
}
