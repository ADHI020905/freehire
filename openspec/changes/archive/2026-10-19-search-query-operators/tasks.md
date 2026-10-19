Step-by-step code for each task: `docs/superpowers/plans/2026-10-18-search-query-operators.md`.

## 1. Query parser

- [x] 1.1 Write the failing `ParseQuery` table tests in `internal/search/search/query_syntax_test.go` (no-operator passthrough, lowercase `or`, quoted `OR`, minus, `title:` placements, empty alternatives, five allowed, six refused).
- [x] 1.2 Implement `Alternative`, `ParseQuery`, `ErrTooManyAlternatives` in `internal/search/search/query_syntax.go`.

## 2. Federated hits

- [x] 2.1 Add `fakeManager` to `testhelpers_test.go` and write the failing `federated_test.go` tests: per-query filter/sort/fields, no per-query limit/offset, no-operator single request, single `title:` stays single-index, OR goes federated, too many refused.
- [x] 2.2 Implement `buildFederatedSearch` (`federated.go`) and route `Client.Search` through `ParseQuery`.
- [x] 2.3 Render `ErrTooManyAlternatives` as a 400 with its own message in `internal/api/handler/errors.go`.

## 3. Facet counts under OR

- [x] 3.1 Write the failing tests: merged facets carry `maxValuesPerFacet`, no per-query facets, the federated response converts to `FacetResult`, no-operator facets stay single-index.
- [x] 3.2 Implement `buildFederatedFacets`, `federatedAsSearchResponse` and `Client.facetSearch`, and route `FacetCounts` and `DisjunctiveFacetCounts` through it.

## 4. Real engine

- [x] 4.1 Pin `startMeili` to `getmeili/meilisearch:v1.49.0` and confirm that the existing integration suite passes.
- [x] 4.2 Add `TestSearchOperatorsOnRealEngine`: `title:` + minus, OR dedup and global sort, paging, merged facets, disjunctive facets.

## 5. Docs

- [x] 5.1 Rewrite the `q` description in `web/static/openapi.yaml` and in `web/src/lib/docs/api-spec.ts` (regenerating `docs/API.md` and the generated OpenAPI JSON).
- [x] 5.2 Add a "Query operators" section to `internal/search/search/AGENTS.md`.

## 6. Ship

- [x] 6.1 Full gate: gofmt, vet (plain and integration), unit tests, the search integration suite.
- [x] 6.2 PR → CI → merge → deploy, then the live check.
- [x] 6.3 Told MadsLorentzen/ai-job-search (comment on their #653, which writes the freehire query-syntax line).
