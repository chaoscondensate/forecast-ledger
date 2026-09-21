## Purpose

Defines immutable v2 question meaning, typed outcome domains, revision binding,
and terminal resolution behavior for authoring and local verification.

## ADDED Requirements

### Requirement: Questions use ordered immutable revisions
Each question SHALL contain a stable ID, lifecycle status, creation time,
non-empty ordered `revisions`, `current_revision_id` naming the final revision,
and an explicit forecasts array. Revision IDs SHALL be unique within the
question; `effective_at` SHALL increase strictly; `recorded_at` SHALL not
precede `effective_at` or move backwards. Once any forecast or resolution binds
a revision, authoring operations MUST NOT edit or remove that revision; a
semantic change SHALL append a new revision and update `current_revision_id`.

#### Scenario: Semantic question edit appends a revision
- **WHEN** a user changes wording, criteria, expected resolution time, outcome space, domain, options, bounds, bins, scale, unit, or provenance
- **THEN** the application appends a fully specified revision and leaves every earlier revision and forecast binding unchanged

#### Scenario: Invalid revision chronology is rejected
- **WHEN** a revision is duplicated, reordered, backdated, or made current without being the last revision
- **THEN** local validation rejects the ledger without changing it

### Requirement: Outcome space and domain form a closed compatible pair
Every revision SHALL declare matching `outcome_space.kind` and `domain.kind`
from `binary`, `categorical`, `ordinal`, `numeric`, `date`, or `datetime`.
Binary domains SHALL contain no invented options. Categorical and ordinal
domains SHALL contain a versioned option set with unique stable option IDs;
ordinal array order SHALL be semantic. Numeric, date, and datetime domains
SHALL use canonical scalar values and declare exactly one continuous, positive
step, or sorted allowed-values policy, with optional valid bounds and bin sets.

#### Scenario: Outcome and domain kinds disagree
- **WHEN** a revision pairs one outcome kind with a different domain kind
- **THEN** validation rejects the revision before any forecast operation uses it

#### Scenario: Discrete value violates its domain
- **WHEN** an authored or resolved numeric, date, or datetime value falls outside a bound, misses the declared step, or is absent from allowed values
- **THEN** validation rejects the value with its domain location identified

### Requirement: Bounds, option versions, and bin versions are exact
Finite lower and upper bounds SHALL describe a non-empty domain and SHALL retain
explicit inclusivity. Option-set and bin-set `(id, version)` pairs SHALL be
unique within their revision. Bin IDs SHALL be unique and ordered; adjacent
bins SHALL have neither gaps nor overlaps and exactly one bin SHALL include a
shared boundary. A changed option or bin definition SHALL require a new
question revision and a new version reference.

#### Scenario: Adjacent bins are ambiguous
- **WHEN** two adjacent bins overlap, leave a gap, or both include or exclude their shared boundary
- **THEN** semantic validation rejects the bin set

#### Scenario: Options change for later forecasts
- **WHEN** a question's available options change
- **THEN** authoring appends a new revision with a new option-set version while earlier forecasts remain bound to the earlier option set

### Requirement: Forecasts and resolved outcomes bind an exact revision
Every forecast SHALL name a revision of its containing question, SHALL not
predate that revision's `effective_at` or optional `forecasting_opens_at`, and
SHALL be append-only ordered by `recorded_at`. A resolved outcome SHALL name an
existing revision and belong to that revision's domain. Direct forecast and
resolution requests SHALL require an explicit revision ID except for an initial
forecast created atomically with its one new revision.

#### Scenario: Forecast names the wrong question revision
- **WHEN** a forecast names a missing revision or a revision owned by another question
- **THEN** the request fails without appending the forecast or creating evidence artifacts

#### Scenario: Resolution uses historical meaning
- **WHEN** a question has several revisions and the observed outcome applies to an earlier one
- **THEN** the resolution records that exact revision and validates the outcome against its domain rather than the current revision

### Requirement: V2 lifecycle and resolution states are enforced
Question status SHALL be one of `open`, `closed`, `awaiting_resolution`,
`resolved`, `ambiguous`, `void`, `disputed`, or `not_applicable`. The first
three states SHALL have no resolution object. Every terminal state SHALL have a
matching closed resolution object: `resolved` requires a typed outcome,
revision, known time, recording time, and at least one source; `ambiguous`,
`void`, and `disputed` require a reason; `not_applicable` requires the
conditional relationship that made the question inactive. `annulled` SHALL not
be accepted or authored.

#### Scenario: Terminal status lacks matching detail
- **WHEN** a question has a terminal status without its required matching resolution object
- **THEN** schema or semantic validation rejects the ledger

#### Scenario: Obsolete annul state is requested
- **WHEN** a CLI or MCP caller attempts to author `annulled` or invoke the removed annul operation
- **THEN** the adapter rejects the unsupported state or operation and directs the caller to an applicable v2 terminal operation
