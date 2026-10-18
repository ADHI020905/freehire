# internal/dict/classify — Seniority & Category Tagging

Deterministic seniority/category tagging from job title, feeding enrichment facets.

## Design

- Parses the **job title** at ingest into canonical `jobs.seniority`/`jobs.category` columns.
- Values from `vocab.SeniorityValues`/`vocab.CategoryValues` — EN+RU aliases, whole-word matched. Russian forms listed as full surface forms (not stems) since matcher requires word boundaries. **Never guesses**.
- Same alias→canonical dictionary design as `internal/dict/location` and `internal/dict/skilltag`.

## Grade-blind phrases

Some role names **contain** a grade word without stating a grade: "Member of Technical Staff" is a generic IC title, "Lead Generation" is a marketing function, "Middle East" is a region, "Mid-training" is a model-training stage. `gradeBlindPhrases` is cut from the title (each occurrence replaced by a space) **before** the seniority match, so only the remaining words can state a grade.

Ordering alone could not fix this: `staff` outranks `senior`, so "Senior Member of Technical Staff" resolved to `staff`. The mask leaves honest grades untouched — "Senior Staff Engineer" is still `staff`.

The list holds only phrases that shadow a `seniorityTable` alias; the category match reads the untouched title.

## The two design crafts

`design` means product/visual/experience design. Engineering draughting —
mechanical, electrical, civil, and the architectural/BIM family — is the separate
`engineering_design` category, a `vocab.NonTechCategories` member (surfaced as a
facet, off the LLM and embedding budgets). Silicon design is neither: it resolves to
`hardware`, where the rest of that team already sits.

Three groups sit **before** the bare `designer`/`design` entries, because the word
`design` alone would otherwise claim every "… Design Engineer": the titles that name
another craft (`network design engineer`, `cloud design engineer`, the silicon
block), the product-side markers (`product design engineer`, `ux/ui/web design
engineer`, `design engineer, product`, `service/experience/sound/game design
engineer`), and then the draughting aliases themselves. The unqualified `design
engineer` closes the block and resolves to `engineering_design`: on this catalogue
that population is overwhelmingly mechanical.

Titles where a category alias appears but names no category at all — "Software Design
Engineer" is software engineering — carry the `categoryNone` sentinel as their table
entry, and `matchCategory` serves it as `""`. Deliberately NOT a pre-match mask like
`gradeBlindPhrases`: cutting the span exposes the aliases further down the table (which
are mostly the business categories, so "Software Design Engineer - Sales Tools" read as
`sales`) and is boundary-blind. Every exit translates the sentinel — `Parse`,
`Categories`, `CategoryAliases` — because the last two feed a CV profile and the
generated web contracts.

## What the ingest filter refuses

The catalogue is broader than IT. The ingest catalogue filter refuses a crawled posting
only through `ConfirmedNonTech`: its title names a role from the non-tech title list
(`nontech.go`) AND nothing places it — no technical evidence, and **no category of any
kind**. A resolved category, technical or not, is a deliberate placement: the posting is
kept, filed under that facet, and only `is_tech = false` follows. A non-technical title
the dictionary cannot FILE is refused, because a posting no facet reaches is one search
excludes anyway (`search.CategoryUnresolved`).

So growing the catalogue is a dictionary change, not a filter change: give the role a
category and the next crawl admits it. Every term in the non-tech list that names a job
outright is filed in `categoryTable` — in a block declared after every technical block,
so a title some alias above already resolves keeps exactly the category it had. The
terms still filed nowhere (`social worker`, `lab technician`, `community liaison`) are
waiting for a category that fits them, not for an exception.

The hazard this moves: an alias that was harmless while the filter refused every
non-technical title can now admit one into a TECHNICAL category. "Front End Cashier"
(the supermarket checkout lane) resolved to `frontend` all along and nobody saw it,
because the cashier was turned away. Before adding a broad technical alias, run it
against the titles the filter used to refuse — the ingest log's `rejected N/M postings
as non-technical, e.g. ...` lines carry two per board.

## The one place category and `is_tech` disagree on purpose

IT support (`service desk`, `help desk`, `helpdesk`, `it support`, `it supporter`,
`desktop support`, `deskside support`, `end user support`, `technical support
analyst`) resolves the `support` category — a `vocab.NonTechCategories` member — and is *also* a
`techTitleTerms` entry, so `jobderive.deriveIsTech` reads it as `is_tech = true`.
That is deliberate, not drift: the category is right about the FUNCTION (reactive,
ticket-driven, alongside customer service) and wrong about the CRAFT, and only the title
list can say the second thing. `tech.go` carries the argument and the measurements
beside the terms; do not restate them here, or the two copies will drift.

**A term earns its place by naming the ESTATE, not the mood of the work.** That is why
bare `technical support` has no entry — live titles give AGV, automotive, controls,
logistics and instructional support under that phrase — while `desktop support` does.
If you are tempted to widen this family, sample the phrase against live titles first;
every term here was admitted that way, and the whole exception depends on none of them
being loose.

## Serving: dict-only

`jobview.FromRow` overwrites the nested `enrichment.seniority`/`enrichment.category` with the `jobs` column — the dictionary always wins, the LLM's value is never a fallback. They remain **nested under `enrichment`** so existing search facets, SPA, and generated contracts are unchanged.

## Convention

- Adding a value: add it to `vocab.SeniorityValues`/`vocab.CategoryValues` and the title-matching dictionary.
- Dictionary change needs `cmd/backfill-derive` + `cmd/reindex` to reach existing jobs (same caveat as geography/skills).

## Owed right now

Two changes landed on 2026-09-15 that only reach postings written after them, and neither
has had its backfill run. **Delete this section once it has.**

- **#2847** replaced the bare `security` category alias with qualified forms. Measured
  over the 150 commonest live titles carrying the word (10,276 open postings): 10,156 read
  as technical before, 5,917 after.
- **#2849** made an explicit non-tech TITLE outvote a technical category resolved from a
  substring (`jobderive.deriveIsTech`). Measured over the 3,000 commonest titles flagged
  technical (197,916 open postings): 22 titles and 2,067 postings move, every one a
  correction.

Together roughly **6,300 stored postings read `is_tech = true` against what the current
dictionaries say** — mall guards, nurses, HVAC project managers, occupational safety
inspectors.

**An open posting corrects itself on its next crawl, and only that.** `job.New` runs
`jobderive.Derive` on every ingest, and `is_tech` is part of `RefreshUnchangedJob`'s match
key, so a row whose derived value moved fails the cheap refresh and goes through
`UpsertJob`, which writes the new one. What the backfill is for is everything that will
NOT be crawled again: **closed postings above all** — they appear in no listing, so
nothing re-derives them — and any posting whose board or provider has stopped being
crawled. Until then those stay in technical search, and `cmd/search-ping` spends a little
of Google's 200-a-day allowance on the open ones it reaches first (measured 2026-09-15:
14 of 398 announcements, 3.5%).

Clearing it is `cmd/backfill-derive` (~15h; hold `BACKFILL_CONCURRENCY` at 2-3, it has
degraded prod at 6) followed by a full `make reindex` — `is_tech` is not part of
`content_hash`, so an incremental push alone never reaches these rows.
