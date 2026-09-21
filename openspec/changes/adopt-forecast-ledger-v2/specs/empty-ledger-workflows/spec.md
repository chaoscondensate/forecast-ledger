## MODIFIED Requirements

### Requirement: Initialize a ledger without a question
`forecast-ledger init` SHALL require an explicit ledger file and scalar identity
flags and SHALL NOT accept generic `--input` or a template document. It SHALL
create a valid v2.0.0 document containing explicit empty `platforms` and
`questions` collections. Optional `groups` and `relationships` SHALL be omitted
unless authored. Every supported non-secret root metadata and platform value
SHALL be authorable through flags.

#### Scenario: CLI creates a minimal empty ledger
- **WHEN** a caller runs `forecast-ledger init` with the required `--file`, ledger ID, timezone, forecaster ID, and forecaster name flags
- **THEN** the command creates a locally valid v2.0.0 ledger with explicit empty required collections and no forecast or key artifact

#### Scenario: Dry-run plans only the empty ledger
- **WHEN** the same flag-only command is run with `--dry-run`
- **THEN** it validates the prospective empty ledger, reports only the deferred ledger-file effect, and writes no file

#### Scenario: Optional metadata input has no question
- **WHEN** init receives supported root metadata or repeated platform values through flags and no question flags
- **THEN** the command applies those values and creates a ledger with `questions: []` without reading an input document

### Requirement: Optionally initialize one question and forecast
`forecast-ledger init` MAY create one question with one fully specified initial
revision and an optional initial forecast from the same direct fields exposed
by the corresponding question-add and forecast-add workflows. The question
SHALL contain matching outcome-space and domain kinds, set
`current_revision_id` to its initial revision, and contain `forecasts: []` when
forecast fields are absent. A public initial forecast SHALL bind that revision
and contain compatible v2 representations. A sealed initial forecast SHALL
keep its private representation bundle and reasoning in protected file or
stdin channels while accepting all non-secret metadata through flags.

#### Scenario: Init creates a question backlog item
- **WHEN** valid init flags describe a question revision and omit initial-forecast fields
- **THEN** the new ledger contains that open question, names the initial revision as current, and has an explicit empty forecasts array

#### Scenario: Init retains public first-forecast behavior
- **WHEN** valid init flags describe one revision and compatible public representations
- **THEN** the ledger, revision, forecast binding, and representations are created atomically under v2 validation and chronology rules

#### Scenario: Init retains sealed first-forecast behavior
- **WHEN** init flags describe a question revision and sealed forecast metadata while protected input and a new key-file destination supply secret material
- **THEN** the ledger, revision-bound v2 commitment, and protected key file are created under the atomic secret-handling rules

### Requirement: Add a question without a forecast
`forecast-ledger question add` SHALL require the explicit ledger, question ID,
initial revision ID, outcome/domain kind, and all fields required by that domain
and SHALL NOT accept generic `--input`. Initial-forecast fields SHALL be
optional. Omitting them SHALL append a source-preserving question whose one
revision is current and whose forecasts value is an explicit empty array. MCP
SHALL use flattened direct properties and route through the same application
service.

#### Scenario: Add question to empty JSON ledger
- **WHEN** valid type-specific direct fields add a question and initial revision without a forecast to an empty JSON ledger
- **THEN** exactly one question is appended, its initial revision is current, its forecasts array is empty, and unrelated source content and formatting remain unchanged

#### Scenario: Add question to YAML ledger
- **WHEN** valid domain-specific fields add a revision-bearing question without a forecast to a YAML ledger
- **THEN** it is appended through the same source-preserving mutation path and the resulting YAML remains valid v2.0.0

#### Scenario: Add with an initial forecast remains atomic
- **WHEN** question-add fields include valid public representations or sealed forecast metadata plus protected secret inputs
- **THEN** revision binding, representation validation, conditional key-file handling, and ledger commit remain one atomic operation

### Requirement: Empty collections are valid read and lifecycle states
Validation, status, platform operations, group and relationship list
operations, question list and show, forecast list, and allowed question metadata,
revision, resolved, ambiguous, void, disputed, and not-applicable operations
SHALL accept valid ledgers with empty collections whenever their selected record
exists. List and status outputs SHALL report empty collections and zero counts
without indexing assumptions. Existing v2 revision, relationship, domain, and
resolution rules SHALL apply even when a question has no forecasts.

#### Scenario: Empty ledger read commands succeed
- **WHEN** validate, status, question list, platform list, group list, or relationship list reads a valid ledger with no questions or optional collections
- **THEN** the command succeeds and reports an empty collection or zero count in human, plain, and JSON modes

#### Scenario: Forecast list on backlog question succeeds
- **WHEN** forecast list selects an existing question whose forecasts array is empty
- **THEN** it succeeds with an empty list and the human result says `No forecasts`

#### Scenario: Resolve a question without forecasts
- **WHEN** an existing forecast-free question satisfies one v2 terminal-state operation and its revision, domain, source, reason, or relationship rules
- **THEN** the operation records the matching terminal status and resolution without adding or requiring a forecast

### Requirement: The first later forecast starts history
Public add and sealed forecast commands SHALL accept an existing open question
with no forecasts and SHALL require the exact question revision they bind. The
first forecast SHALL NOT gain an implicit `supersedes_forecast_id`; an
explicitly supplied superseded ID SHALL still have to identify an earlier
forecast in that question.

#### Scenario: First public forecast is appended
- **WHEN** forecast add targets a revision of a question with no forecasts and omits `supersedes_forecast_id`
- **THEN** it appends the first forecast with the explicit revision binding and no supersedes link

#### Scenario: Missing superseded forecast is rejected
- **WHEN** the first forecast names a superseded forecast ID that does not exist
- **THEN** the operation fails without changing the ledger

### Requirement: Publication supports empty evidence sets
Publication build SHALL accept a valid empty v2 ledger and create a deterministic
package containing the ledger and manifest with no forecast target or receipt
entries. Publication verify SHALL validate that package, preserve its manifest
and file-integrity observations, report an empty evidence list and zero network
requests, and return overall `no_evidence` with application category
`incomplete` and exit 9. It SHALL NOT claim forecast evidence or forecast-set
completeness.

#### Scenario: Build package from empty ledger
- **WHEN** publication build receives a valid v2 ledger with no forecasts
- **THEN** it creates only the ledger entry and manifest, records the exact v2.0.0 schema pin and v2 profiles, and reports evidence state `complete`

#### Scenario: Verify empty package
- **WHEN** publication verify checks that package
- **THEN** it returns `evidence: []`, overall `no_evidence`, application category `incomplete`, zero requests, and the standard limitation that forecast-set completeness is not proved

### Requirement: Generated contracts and documentation teach empty-first use
Generated request schemas, MCP tool schemas, CLI help, README,
getting-started material, command reference, examples, and
changelog SHALL agree that CLI init and question creation reject generic input
documents and do not require initial forecasts. CLI examples SHALL show a
complete v2 flag-only empty-first flow, including an initial question revision,
domain, explicit later forecast revision binding, and canonical decimal
representation. Protected sealed examples SHALL keep private data out of argv.

#### Scenario: Generated schemas match runtime behavior
- **WHEN** maintained schemas and MCP contracts are regenerated and checked
- **THEN** direct v2 fields and required revision selectors match runtime, generic public wrappers remain absent, and protected sealed-input restrictions remain represented

#### Scenario: A new user follows the empty-first documentation
- **WHEN** a user copies the documented init, platform-add, revision-bearing question-add, and later representation-bearing forecast-add commands
- **THEN** each command uses real flags, requires no prepared public JSON/YAML fragment, and produces a valid v2.0.0 ledger at every step
