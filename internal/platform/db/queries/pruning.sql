-- The residual-title report cmd/mine-titles prints: what the classification dictionaries
-- still leave unplaced.

-- name: ResidualTitleGroups :many
-- The residual unclassified mass grouped into clusters an operator can act on: the
-- word groups occurring most often in the titles of live jobs that carry no is_tech
-- signal. Grouping is by word group, not by whole title, because boards append
-- location, schedule and requisition detail — half the unclassified mass has a title
-- that occurs exactly once, so whole-title grouping splits one role into singletons.
-- A word group is also the unit the non-tech dictionary accepts, so a reported
-- cluster is directly usable as an anchored term.
--
-- A title is first split at punctuation into runs, and groups are formed only WITHIN
-- a run. Without that, adjacency spans separators and invents phrases that never
-- occur: "Personal Care Aide - Honolulu" would yield "aide honolulu", and a term
-- copied from that cluster into the dictionary would match none of the jobs that
-- produced it, because the dictionary matches contiguous text. It also inflated the
-- counts of legitimate clusters by every occurrence that straddled punctuation.
--
-- Two-word groups carry the ordinary case. Three-word groups exist only to bridge a
-- connector: in Portuguese and Spanish the preposition is part of the role name
-- ("operador de caixa"), while the two-word fragments around it ("analista de") are
-- noise, so a connector is allowed mid-group and never at an edge. Every other edge
-- token must be at least three characters and non-numeric, which keeps requisition
-- numbers and shredded schedule notation ("m w", "req 12345") out of the ranking.
-- The vocabularies are COALESCEd: a NULL parameter would make every <> ALL(...)
-- comparison NULL and silently return zero rows — indistinguishable from the
-- campaign having exhausted the residual mass, which is the one wrong answer this
-- report must never give.
--
-- Tokenizing on [^[:alnum:]]+ is Unicode-aware, so accented Latin and Cyrillic
-- survive whole — an ASCII class would split "Técnico" and lose the cluster.
-- Closed and duplicate rows are excluded: only a live, canonical posting is worth a
-- dictionary term.
WITH runs AS (
    SELECT j.id,
           j.source,
           regexp_split_to_table(lower(btrim(j.title)), '[^[:alnum:] ]+') AS run
    FROM jobs j
    WHERE j.closed_at IS NULL
      AND j.duplicate_of IS NULL
      AND j.is_tech IS NULL
),
tok AS (
    SELECT id, source, regexp_split_to_array(btrim(run), ' +') AS a
    FROM runs
    WHERE btrim(run) <> ''
),
pos AS (
    SELECT tok.id, tok.source, tok.a, i
    FROM tok, generate_subscripts(tok.a, 1) AS i
),
grp AS (
    -- Two-word groups: both tokens must be role-bearing edges.
    SELECT DISTINCT id, source, (a[i] || ' ' || a[i + 1])::text AS grp
    FROM pos
    WHERE i + 1 <= cardinality(a)
      AND length(a[i])     >= 3 AND a[i]     !~ '^[0-9]+$'
      AND length(a[i + 1]) >= 3 AND a[i + 1] !~ '^[0-9]+$'
      AND a[i]     <> ALL(COALESCE(sqlc.arg(stop_words)::text[], '{}'))
      AND a[i + 1] <> ALL(COALESCE(sqlc.arg(stop_words)::text[], '{}'))
      AND a[i]     <> ALL(COALESCE(sqlc.arg(connectors)::text[], '{}'))
      AND a[i + 1] <> ALL(COALESCE(sqlc.arg(connectors)::text[], '{}'))
    UNION ALL
    -- Three-word groups: role-bearing edges bridging one connector.
    SELECT DISTINCT id, source, (a[i] || ' ' || a[i + 1] || ' ' || a[i + 2])::text
    FROM pos
    WHERE i + 2 <= cardinality(a)
      AND length(a[i])     >= 3 AND a[i]     !~ '^[0-9]+$'
      AND length(a[i + 2]) >= 3 AND a[i + 2] !~ '^[0-9]+$'
      AND a[i]     <> ALL(COALESCE(sqlc.arg(stop_words)::text[], '{}'))
      AND a[i + 2] <> ALL(COALESCE(sqlc.arg(stop_words)::text[], '{}'))
      AND a[i]     <> ALL(COALESCE(sqlc.arg(connectors)::text[], '{}'))
      AND a[i + 2] <> ALL(COALESCE(sqlc.arg(connectors)::text[], '{}'))
      AND a[i + 1]  = ANY(COALESCE(sqlc.arg(connectors)::text[], '{}'))
)
-- count(*) is the job count, not an approximation of it: the two branches cannot
-- emit the same group (a group's space count fixes its branch, and tokens hold no
-- spaces), and each branch already emits DISTINCT (id, source, group) — which is
-- what stops a group repeated inside one verbose title from outranking a real
-- cluster. A second count(DISTINCT id) would only add a per-group sort.
SELECT grp,
       count(*)                           AS jobs,
       array_agg(DISTINCT source)::text[] AS sources
FROM grp
GROUP BY grp
ORDER BY 2 DESC, 1
LIMIT sqlc.arg(row_limit);
