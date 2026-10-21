/**
 * Whether the panel offers "Add this job" on a page freehire does not carry. Only where
 * the page is plausibly a vacancy: its URL sits on an ATS board the server knows how to
 * crawl (`atsBoard`, answered by `/jobs/find` from the server's own ATS table so no second
 * copy lives here), or the page marks itself up as a schema.org JobPosting. Offered on
 * every page, the button filled the triage queue with LinkedIn feeds, profiles, chat apps,
 * localhost and freehire's own pages. Pure.
 */
export function offersAddJob(page: { atsBoard: boolean; jobPostingMarkup: boolean }): boolean {
  return page.atsBoard || page.jobPostingMarkup;
}
