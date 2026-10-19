-- migrate: no-transaction
--
-- Indexes the last eight characters of jobs.public_slug: the shortcode
-- normalize.JobSlug derives from (source, external_id) alone, so it is the one part
-- of a job's URL that no change to its title or company can move.
--
-- A job page addressed by a slug that no longer exists resolves through it to the
-- posting's current slug and redirects there (freehire#3284). Until that fix,
-- cmd/backfill-derive re-derived public_slug from the CURRENT title, so every posting
-- a source retitled under the same external_id lost its published URL — one run on
-- 2026-10-19 moved 42,226 of them before it was stopped, each old link a 404 for
-- search engines, shared links and cached clients alike. The pass no longer writes
-- the column; this index is what lets the links it already broke find their posting.
--
-- CONCURRENTLY, so the build over ~12.7M rows blocks neither reads nor ingest
-- writes — hence the no-transaction marker above. Not unique: 40 bits over this
-- many rows collide a few dozen times, and the lookup refuses an ambiguous code
-- rather than redirecting to the wrong posting.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_public_slug_code_idx
    ON public.jobs (right(public_slug, 8));
