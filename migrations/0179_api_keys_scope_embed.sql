-- Widens api_keys.scope to allow 'embed': a key scoped for the
-- volunteer-embed-mvp service (a separate repo) that lets internet
-- volunteers help drain the semantic_outbox embedding backlog. That
-- service validates these keys directly against this table; it grants
-- nothing on freehire's own API surface.
--
-- NOT VALID defers the full-table scan: the constraint applies to new/updated
-- rows immediately, taking only a brief AccessExclusiveLock to swap the
-- constraint definition. Existing rows are checked separately in
-- 0180_api_keys_scope_embed_validate.sql, under a lock that doesn't block
-- reads/writes — squawk (constraint-missing-not-valid) requires that
-- validation run in its own transaction, which here means its own file.
ALTER TABLE public.api_keys
    DROP CONSTRAINT api_keys_scope_check,
    ADD CONSTRAINT api_keys_scope_check CHECK (scope IN ('full', 'cv', 'embed')) NOT VALID;
