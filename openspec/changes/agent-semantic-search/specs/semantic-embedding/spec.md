## ADDED Requirements

### Requirement: Written chunks reach the search index

When a job's `job_semantic_chunks` set is inserted or replaced, the system SHALL queue that job
for the search index (`search_outbox`) in the same transaction, whichever process wrote the
chunks — the server-side embed worker or the volunteer broker. Deleting a job's chunks on the
closed-job path SHALL NOT queue it, since a closed posting leaves the index through the
existing delete path.

#### Scenario: Server-side worker embeds a job

- **WHEN** `cmd/embed` writes the chunk set of an open job
- **THEN** that job has a `search_outbox` entry when the transaction commits

#### Scenario: Volunteer broker promotes a job

- **WHEN** the volunteer broker replaces a job's chunk set with raw SQL from its own repository
- **THEN** that job has a `search_outbox` entry when the transaction commits

#### Scenario: Re-embedding an already queued job

- **WHEN** a job that already has a pending `search_outbox` entry is embedded again
- **THEN** no second entry is created and the write does not fail
