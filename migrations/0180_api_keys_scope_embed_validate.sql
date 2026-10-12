-- Validates the NOT VALID constraint added in 0179 against existing rows,
-- in its own transaction: VALIDATE CONSTRAINT takes a ShareUpdateExclusiveLock
-- (doesn't block concurrent reads/writes), but only if it's not sharing a
-- transaction with the ADD CONSTRAINT that created it.
ALTER TABLE public.api_keys VALIDATE CONSTRAINT api_keys_scope_check;
