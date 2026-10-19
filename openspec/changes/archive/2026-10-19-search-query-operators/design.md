# Search query operators: `OR` and `title:`

Date: 2026-10-18

## Why

Agents that call the search API (the `freehire-search` skill in
MadsLorentzen/ai-job-search, the busiest third-party client) want to ask for
several role names at once, for example `produktchef OR "product owner"`, and
to match a role name in the title only. Today `OR` is an ordinary word, so that
query returns exactly what `"product owner"` alone returns (10,352). The skill
works around it by sending one call per term and merging by URL (their #653).

## What `q` does today (measured on prod, 2026-10-18)

| q | total |
|---|---|
| `golang` | 3,518 |
| `golang rust` | 3,465 |
| `rust golang` | 2,285 |
| `"golang rust"` / `"rust golang"` | 97 / 97 |
| `golang -senior` | 2,524 |

- Unquoted words follow Meilisearch's default `matchingStrategy: last`. They
  are neither AND nor OR, and the order of the words changes the result.
- Double quotes make the quoted words all required, in any order. This is not
  a contiguous phrase.
- A leading `-` excludes a word or a quoted group. Meilisearch does this on its
  own, and nothing documents it.
- `q_fields=title` already restricts `q` to the title. The web app's title
  suggestion uses it.

`web/static/openapi.yaml` says unquoted words "match as an OR". That is wrong
and gets fixed here.

## What changes

`q` gains two operators. A query that uses neither behaves exactly as today,
through the same single Meilisearch request.

- **`OR`** (uppercase, standalone token, outside quotes) splits `q` into
  alternatives. A posting matching any alternative is a hit. Lowercase `or`
  stays a word.
- **`title:`** at the start of an alternative restricts that whole
  alternative to the `title` field. `title:"product owner" -junior` means: in
  the title, both words, and not `junior`. `title:` is the only field prefix.
  Meilisearch scopes fields per query, not per word, so a prefix in the middle
  of an alternative is not an operator and stays text.
- Quotes and `-` keep working inside every alternative, because each
  alternative is passed to Meilisearch as written.

Example: `title:"product owner" OR title:produktchef -junior`.

Limits: at most 5 alternatives. More than that is a 400 (`ErrBadQuery`) whose
message names the limit. The endpoints are public, and each alternative is one
more Meilisearch query.

`q_fields` and `title:` combine like this: an alternative with `title:` searches
the title; an alternative without it uses `q_fields` (or all fields when that is
absent).

## How

**Parsing** — new `internal/search/search/query_syntax.go`:
`ParseQuery(q string) ([]Alternative, error)`, where
`Alternative{Text string; Fields []string}`. It is pure, with no Meilisearch
types, and is unit-tested on its own: quotes protect `OR` and `title:`, empty
alternatives (`OR OR`, a leading or trailing `OR`) are dropped, more than 5 is
an error, and a query with no operators returns one alternative equal to the
input.

**Querying** — `Client.Search`, `Client.FacetCounts` and
`Client.DisjunctiveFacetCounts` call `ParseQuery`.

- One alternative and no `title:`: the current request, unchanged.
- Otherwise: one federated `/multi-search` request. Each alternative is a
  query carrying the same `Filter`, `Sort` and `Vector`/`Hybrid`, with its own
  `AttributesToSearchOn`. `federation` carries `limit`/`offset`, and for the
  facet calls `facetsByIndex` plus `mergeFacets`.

A spike against `getmeili/meilisearch:v1.49.0` (the version in
`docker-compose.yml`) confirmed:

- a document matched by two alternatives appears once;
- with this index's ranking rules (`words, sort, ...`) a shared
  `sort: [d:desc]` orders the merged hits globally;
- `offset`/`limit` page the merged list, and `estimatedTotalHits` counts it;
- merged facet counts are per document, not per alternative.

Every caller gets the operators at once: the web search, `/api/v1/agent/jobs/search`,
OJCP, the MCP app and the assistant's search tool. They all build `SearchParams`
and go through the same client.

**Errors** — a parse error is `ErrBadQuery`, which the handlers already render as
400.

## Docs

- `web/static/openapi.yaml`, the `Q` parameter: replace the wrong "OR" claim
  with the measured behaviour, and document quotes, `-`, `OR`, `title:` and the
  limit of 5.
- `internal/search/search/AGENTS.md`: one paragraph on the federated path and
  why a single alternative skips it.
- After deploy, a PR to `freehire-search` in ai-job-search: its `Query syntax:`
  line (their #653) can say that freehire honours `OR`, `title:`, quotes and `-`.

## Testing

- Unit: `ParseQuery` table tests; a request-builder test showing that one
  alternative builds today's `SearchRequest` byte-for-byte and several build a
  federated request carrying filter, sort and fields per query.
- Integration (`-tags=integration`, a testcontainers Meilisearch on v1.49):
  OR dedup, `title:` scoping, `-` exclusion, sort across alternatives, paging,
  merged facet counts and stats, and the disjunctive facet path.
- Live check after deploy: `produktchef OR "product owner"` returns more than
  `"product owner"` alone, and `title:"product owner"` returns fewer.

## Out of scope

- Changing what plain spaces mean. The order dependence is real, but fixing it
  changes every person's search on the site.
- Parentheses, `AND`, and field prefixes other than `title:`.
- The UI. The search box passes `q` through as typed, so operators work there
  without a change. A syntax hint in the UI is a separate decision.
