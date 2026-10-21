-- name: ObserveLearnedDomain :one
-- Record one confident job-mail sighting for a sender domain, returning its running
-- count. The classifier calls this whenever it confidently labels an email as
-- application mail, so a recurring unknown ATS domain accrues hits toward promotion.
INSERT INTO learned_ats_domains (domain, confident_hits)
VALUES ($1, 1)
ON CONFLICT (domain) DO UPDATE
  SET confident_hits = learned_ats_domains.confident_hits + 1,
      last_seen_at = now()
RETURNING confident_hits;

-- name: MissLearnedDomain :exec
-- Record one non-job sighting (classified `other`) against the learned domain that covers
-- this sender host: the domain itself or a parent, the way from:domain matches. Updates only
-- — a host no learned domain covers is not inserted, so newsletters never enter the table.
UPDATE learned_ats_domains
SET other_hits = other_hits + 1,
    last_seen_at = now()
WHERE domain = sqlc.arg(host)::text
   OR right(sqlc.arg(host)::text, length(domain) + 1) = '.' || domain;

-- name: PromotedDomains :many
-- Domains with enough confident hits AND mostly job mail: at most max_other_share of their
-- sightings classified `other`. The sync worker unions these into the Gmail search query.
-- The share is compared as a quotient so an exact boundary (2 of 6 against 1/3) is inclusive;
-- NULLIF because SQL does not promise to evaluate the threshold first.
SELECT domain
FROM learned_ats_domains
WHERE confident_hits >= sqlc.arg(threshold)::int
  AND other_hits::float8 / NULLIF(confident_hits + other_hits, 0) <= sqlc.arg(max_other_share)::float8
ORDER BY domain;
