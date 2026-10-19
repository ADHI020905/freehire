## ADDED Requirements

### Requirement: Opt-in hybrid search

The system SHALL run an agent search as a hybrid (keyword + vector) query only when the agent
asks for it with `semantic: true` AND the query text is non-empty. A search without the flag,
or with `semantic: false`, SHALL produce exactly the request it produces today — no embedding
call, no vector, no change in ranking.

The hybrid query SHALL apply every filter, the pagination and the visibility rules the
keyword search applies, because it runs against the same index with the same filter.

#### Scenario: Agent does not ask for semantic search

- **WHEN** an agent calls `search_jobs` without `semantic`
- **THEN** no embedding request is made
- **AND** the result is the keyword search result

#### Scenario: Agent asks for semantic search with a query

- **WHEN** an agent calls `search_jobs` with `semantic: true` and a non-empty query
- **THEN** the query is embedded once in the e5 query form (`query: ` prefix)
- **AND** the search runs as a hybrid query against the `desc` embedder
- **AND** the filters the agent sent narrow the result exactly as they do for keyword search

#### Scenario: Agent asks for semantic search with no query

- **WHEN** an agent calls `search_jobs` with `semantic: true` and an empty query
- **THEN** no embedding request is made
- **AND** the result is the keyword search result
- **AND** `semantic` is reported in `ignored_params`

### Requirement: Weak semantic matches are cut off

A hybrid query SHALL carry the configured ranking-score threshold, so postings whose only
connection to the query is a distant vector are not returned and not counted in
`total_results`. The threshold SHALL be configuration, set from sampled agent queries before
the field is announced.

#### Scenario: Unrelated postings are not padded in

- **WHEN** a semantic search's query relates to a small set of postings
- **THEN** the response does not fill the page with postings below the threshold
- **AND** `total_results` does not report the whole embedded catalogue

### Requirement: Embedding failure degrades to keyword search

The query embed SHALL be bounded by a short timeout and SHALL NOT be retried on the request
path. When the embed times out, fails, or no embedding backend is configured, the system SHALL
answer with the keyword search result rather than an error or a delay.

#### Scenario: Embedding backend is slow

- **WHEN** an agent asks for semantic search and the embedding backend does not answer within
  the timeout
- **THEN** the response arrives with the keyword search result
- **AND** the response reports `semantic: "unavailable"`

#### Scenario: Engine refuses the hybrid query

- **WHEN** the engine rejects the hybrid request as malformed (e.g. the live index does not
  yet declare the `desc` embedder)
- **THEN** the response carries the keyword search result and reports `semantic: "unavailable"`

#### Scenario: Embedding backend is not configured

- **WHEN** the server runs without an embedding backend and an agent asks for semantic search
- **THEN** the response carries the keyword search result and reports `semantic: "unavailable"`

### Requirement: The response states which search ran

When the agent asked for semantic search, the response SHALL carry `semantic` with the value
`applied` when the hybrid query ran and `unavailable` when it fell back to keyword search. When
the agent did not ask, the field SHALL be omitted.

#### Scenario: Hybrid query ran

- **WHEN** the query was embedded and the hybrid search succeeded
- **THEN** the response carries `semantic: "applied"`

#### Scenario: Agent did not ask

- **WHEN** an agent calls `search_jobs` without `semantic`
- **THEN** the response carries no `semantic` field

### Requirement: Postings carry their description vectors in the search index

Every document written to the jobs index SHALL carry, under the `desc` embedder, the vectors of
its posting's current `job_semantic_chunks` rows — one vector per chunk — whichever writer
produced it (full rebuild, incremental drain, link import). A posting with no chunk rows SHALL
be written with no `desc` vectors and remain reachable through the keyword half of a hybrid
query.

#### Scenario: Rebuild exports chunk vectors

- **WHEN** a full rebuild writes a posting that has three chunk rows
- **THEN** its document carries three vectors under `_vectors.desc`

#### Scenario: Posting without vectors stays searchable

- **WHEN** a hybrid query's keywords match a posting that has no chunk rows
- **THEN** that posting can still be returned
