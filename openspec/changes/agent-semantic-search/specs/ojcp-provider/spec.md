## ADDED Requirements

### Requirement: search_jobs semantic extension

`search_jobs` SHALL accept an optional boolean `semantic` input field and SHALL report the
outcome in an optional `semantic` response field, as extensions permitted by the OJCP
extensibility rule. Both transports SHALL accept and report it identically, and the tool's
published input schema SHALL declare the input field so an agent can discover it.

The response with the extension present SHALL still validate against the OJCP `search-jobs`
response schema.

#### Scenario: REST and MCP agree

- **WHEN** the same `search_jobs` input with `semantic: true` is sent over REST and over MCP
- **THEN** both answer with the same postings and the same `semantic` value

#### Scenario: Extension keeps the response conforming

- **WHEN** a response carries `semantic`
- **THEN** it validates against the vendored OJCP `search-jobs` response schema

#### Scenario: Agent discovers the field

- **WHEN** an MCP client lists the tools
- **THEN** `search_jobs`'s input schema names `semantic` as an optional boolean
