-- learned_ats_domains.other_hits: how much of a learned domain's synced mail the classifier
-- labelled `other` (not about an application). Promotion counted only hits, and a promoted
-- domain's whole mailstream joins every user's Gmail query (from:domain), so a domain that
-- sends a few hiring mails and a lot of everything else was learned and kept. On prod
-- (2026-10-21) google.com was learned for all 33 Gmail users with 98% of its 5,799 synced
-- messages `other`; across learned domains 79% of 36,603 were. PromotedDomains now also
-- bounds the `other` share (gmailsync.MaxOtherShare). Misses only increment an existing row —
-- an unlearned sender never enters the table on non-job mail.
ALTER TABLE learned_ats_domains
    ADD COLUMN IF NOT EXISTS other_hits bigint NOT NULL DEFAULT 0;

-- Backfill from the mail already synced, so a domain that is mostly noise drops out on the
-- first sync after deploy instead of after accruing fresh misses. Matched the way the query
-- fetches: the domain itself or any subdomain (from:google.com pulls accounts.google.com).
UPDATE learned_ats_domains d
SET other_hits = s.n
FROM (
    SELECT d2.domain, count(*) AS n
    FROM emails e
    JOIN learned_ats_domains d2
      ON lower(split_part(e.from_addr, '@', 2)) = d2.domain
      OR right(lower(split_part(e.from_addr, '@', 2)), length(d2.domain) + 1) = '.' || d2.domain
    WHERE e.source = 'gmail' AND e.status_signal = 'other'
    GROUP BY d2.domain
) s
WHERE s.domain = d.domain;
