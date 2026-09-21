## Purpose

Defines append-only v2 forecast activity events so active state can be replayed
without rewriting the original forecast or confusing activity with belief changes.

## ADDED Requirements

### Requirement: Forecast lifecycle is an append-only state machine
A forecast SHALL begin active. `withdrawn` and `expired` events SHALL move an
active forecast to inactive; `reaffirmed` SHALL move an inactive forecast to
active. Event IDs SHALL be unique within the forecast. Events SHALL be ordered
by non-decreasing `effective_at` and append-only non-decreasing `recorded_at`,
and each `recorded_at` SHALL not precede its `effective_at`. An operation MUST
reject deactivating an inactive forecast or reaffirming an active forecast.

#### Scenario: Forecast is withdrawn and reaffirmed
- **WHEN** a valid withdrawal is followed by a valid reaffirmation
- **THEN** both events remain in order and the derived current activity is active

#### Scenario: Reaffirm active forecast is invalid
- **WHEN** a caller tries to reaffirm a forecast with no preceding withdrawal or expiry
- **THEN** the request fails without appending an event

### Requirement: Activity events do not replace forecast revisions
Lifecycle events SHALL record activity only. A changed forecast belief SHALL be
a new forecast with `supersedes_forecast_id`; it MUST NOT be represented by an
activity event. Withdrawing or expiring a forecast SHALL not delete its
representations, reasoning, commitment, target, timestamps, or publication
evidence.

#### Scenario: Belief changes after withdrawal
- **WHEN** a forecaster changes the represented probabilities after withdrawing a forecast
- **THEN** the application requires a new forecast record and preserves the withdrawn forecast and its evidence

### Requirement: Lifecycle events support provenance and direct authoring
Each lifecycle event SHALL expose its ID, type, effective time, recorded time,
optional reason, and optional v2 provenance through dedicated CLI operations
and equivalent closed MCP tools. Private forecast content SHALL not be needed
to withdraw, expire, or reaffirm a sealed forecast, and event authoring MUST
not reveal or decrypt it.

#### Scenario: Sealed forecast is withdrawn
- **WHEN** a caller with ledger write access appends a valid withdrawal to a sealed forecast
- **THEN** the event is recorded without reading the key or disclosing private content

#### Scenario: Adapter requests are equivalent
- **WHEN** equivalent lifecycle fields are supplied through CLI and MCP
- **THEN** both route through the same validation and atomic append behavior and produce equivalent results

### Requirement: Lifecycle state is visible without overclaiming
Forecast list, show, status, verification, and publication results SHALL report
the recorded event history or derived active state where relevant. They SHALL
not treat inactive as invalid, erase earlier evidence, or claim that the ledger
contains a platform's complete scoring-duration history.

#### Scenario: Inactive forecast remains verifiable
- **WHEN** a withdrawn forecast has a valid retained timestamp
- **THEN** verification can still establish content binding and existence timing while separately reporting its inactive lifecycle state
