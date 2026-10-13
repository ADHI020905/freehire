-- A company_accounts row is inserted the moment a claim is SUBMITTED, before the claimant
-- has proven they can read the work email they typed — the mailed code is consumed later,
-- by ConfirmClaim. Nothing recorded whether that had happened, so the moderator queue
-- (status = 'pending') showed a claim on someone else's address exactly like a confirmed
-- one, and Approve would activate it. Measured on prod 2026-10-13: 3 of the 4 pending
-- claims had never entered their code (0 attempts, the code long expired).
--
-- email_confirmed_at is written by ConfirmClaim once the code is consumed. The queue and
-- the activation statement both require it, so an unconfirmed claim can be neither seen
-- nor approved.
ALTER TABLE company_accounts ADD COLUMN IF NOT EXISTS email_confirmed_at timestamptz;

-- Backfill. An active account was confirmed on its way to active (verified_at). A pending
-- one was confirmed exactly when no verify_work_email code is still outstanding for its
-- user: accounts.Service deletes the code row when, and only when, it is consumed. The
-- confirmation time itself was never stored, so updated_at stands in for it. A revoked
-- account keeps NULL — it can never be activated again, so the column is moot for it.
UPDATE company_accounts SET email_confirmed_at = verified_at
WHERE status = 'active';

UPDATE company_accounts ca SET email_confirmed_at = ca.updated_at
WHERE ca.status = 'pending'
  AND NOT EXISTS (
      SELECT 1 FROM user_email_codes c
      WHERE c.user_id = ca.user_id AND c.purpose = 'verify_work_email'
  );

COMMENT ON COLUMN company_accounts.email_confirmed_at IS
    'When the claimant consumed the code mailed to work_email. NULL = never proven: such a '
    'claim is hidden from the moderator queue and cannot be activated.';
