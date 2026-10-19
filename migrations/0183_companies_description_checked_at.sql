-- companies.company_description_checked_at: when an ingest crawl last fetched this
-- company's own "about us" text from its ATS board (sources.CompanyDescriber — greenhouse
-- and workable today) and got an answer, whether that answer was a description or blank.
--
-- Until now the pipeline fetched it on every crawl of every board. Measured on prod
-- 2026-10-19 that was half of Workable's ~63k requests a day (3,963 boards x 8 runs) and
-- half of its 18,520 refusals in two days. A company that already holds a description
-- needs no column to be skipped — the fill keeps what is stored, so re-asking could not
-- change it — but a board whose employer left the field blank is indistinguishable from a
-- never-asked one without a record of the attempt. This is that record, the same shape
-- company_info_wikipedia_checked_at (0154) gives the Wikipedia backfill.
--
-- NULL means "never answered". company_info_at cannot serve: every company-info source
-- stamps it, and only when something was actually written.
--
-- A nullable ADD COLUMN with no default is catalogue-only in PostgreSQL 11+, so this does not
-- rewrite the table and needs no CONCURRENTLY split.
ALTER TABLE public.companies
    ADD COLUMN IF NOT EXISTS company_description_checked_at timestamptz;
