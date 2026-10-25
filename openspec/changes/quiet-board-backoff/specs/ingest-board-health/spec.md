## ADDED Requirements

### Requirement: A board whose listing is unchanged is crawled progressively less often
After each complete, successful crawl the system SHALL compute the board's listing fingerprint: the
sha256 of the board's de-duplicated, sorted posting external ids, counting every posting the listing
named whatever its later outcome. When the fingerprint equals the one stored from the previous complete
successful crawl, the system SHALL increment the board's quiet streak and SHALL set a quiet pause of
4h·2^(streak−1), capped at 72h. When it differs, or no fingerprint is stored, the system SHALL store
the new fingerprint, reset the quiet streak to zero and clear the quiet pause.

#### Scenario: Unchanged listing grows the pause
- **WHEN** a board's complete successful crawl yields the same set of posting ids as its previous one, for the third time in a row
- **THEN** its quiet streak is 3 and its quiet pause ends 16h after that crawl

#### Scenario: The pause is capped
- **WHEN** a board's quiet streak reaches 7
- **THEN** its quiet pause is 72h, not longer

#### Scenario: A posting appearing resets the board
- **WHEN** a board's complete crawl yields one posting id that its previous crawl did not
- **THEN** its quiet streak is reset to 0 and it has no quiet pause

#### Scenario: A posting disappearing resets the board
- **WHEN** a board's complete crawl no longer yields one posting id its previous crawl yielded
- **THEN** its quiet streak is reset to 0 and it has no quiet pause

#### Scenario: Order and duplicates do not count as change
- **WHEN** a board's crawl yields the same posting ids in a different order, or with one id repeated
- **THEN** the fingerprint is unchanged and the crawl counts as quiet

### Requirement: An incomplete crawl leaves the quiet state untouched
A crawl that did not read the board's whole listing — interrupted by the run's deadline, failed, or cut
short by the context ending — SHALL NOT change the board's fingerprint, quiet streak or quiet pause.

#### Scenario: Interrupted crawl
- **WHEN** a board's crawl is interrupted by the run's crawl deadline
- **THEN** its stored fingerprint, quiet streak and quiet pause are exactly what they were before the run

### Requirement: A board in its quiet pause is skipped
Before invoking a board's adapter the system SHALL skip the board when its quiet pause lies in the future,
exactly as it skips a board in failure cooldown, and SHALL count it in the run's quiet total, separately
from cooled boards. The check SHALL fail open: a health-store error crawls the board.

#### Scenario: Quiet board skipped
- **WHEN** a run reaches a board whose quiet pause ends in the future
- **THEN** the adapter is not invoked and the run's quiet count increases by one

#### Scenario: Health-store error crawls the board
- **WHEN** reading the board's pauses fails
- **THEN** the board is crawled

### Requirement: The quiet pause is independent of the failure cooldown
The quiet pause SHALL be stored separately from the failure cooldown. Clearing cooldowns after a
provider-wide recovery SHALL NOT clear any quiet pause, and a failed crawl SHALL NOT change the quiet state.

#### Scenario: Recovery probe keeps quiet pauses
- **WHEN** a provider's recovery probe succeeds and clears its boards' cooldowns
- **THEN** every quiet pause of that provider's boards is unchanged

### Requirement: Self-closing providers are never paced by quietness
For a provider that closes its own postings from a change stream, the system SHALL NOT compute a
fingerprint or apply a quiet pause, because its emissions are changes, not a listing.

#### Scenario: Self-closing provider crawl
- **WHEN** a self-closing provider's board completes a crawl that emits nothing
- **THEN** its quiet streak stays 0 and it has no quiet pause
