-- The jobs search index carries each posting's description chunk vectors under its `desc`
-- embedder (openspec/changes/agent-semantic-search). Those vectors are usually computed
-- AFTER the posting was indexed — embedding drains its own queue, hours behind ingest — so a
-- freshly written chunk set has to put its job back on search_outbox, or the vectors reach
-- Meilisearch only at the next full rebuild.
--
-- A trigger rather than one more statement in each writer: two processes write this table,
-- cmd/embed here and the volunteer broker's promote step, which is raw SQL in another
-- repository (freehire-volunteer-embed). A writer that forgot the enqueue would look exactly
-- like a posting nobody has embedded yet, and nothing would report it.
--
-- STATEMENT level over a transition table, not FOR EACH ROW: a job contributes ~2.5 rows per
-- write (measured 2026-10-18: 2,124,913 chunks over 861,802 open jobs), and both writers
-- insert a whole batch in one statement. INSERT only — the closed-job path only DELETEs,
-- and a closed posting leaves the index through the delete outbox, not this one.
--
-- Same enqueue shape as every other writer of search_outbox: open postings only, the
-- effective posted date as the claim's sort key, and ON CONFLICT DO NOTHING against
-- search_outbox_job_id_key so a job already waiting is not queued twice.
CREATE OR REPLACE FUNCTION public.job_semantic_chunks_enqueue_search() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT j.id, COALESCE(j.posted_at, j.created_at)
    FROM (SELECT DISTINCT job_id FROM new_chunks) c
    JOIN jobs j ON j.id = c.job_id
    WHERE j.closed_at IS NULL
    ON CONFLICT (job_id) DO NOTHING;
    RETURN NULL;
END;
$$;

CREATE TRIGGER job_semantic_chunks_enqueue_search
    AFTER INSERT ON public.job_semantic_chunks
    REFERENCING NEW TABLE AS new_chunks
    FOR EACH STATEMENT
    EXECUTE FUNCTION public.job_semantic_chunks_enqueue_search();
