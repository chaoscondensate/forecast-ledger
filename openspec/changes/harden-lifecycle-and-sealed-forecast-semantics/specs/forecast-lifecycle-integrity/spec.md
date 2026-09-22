## Purpose

Defines valid lifecycle chronology and evidence that reports forecast activity
without changing the immutable recorded-belief target or overstating history
completeness.

## ADDED Requirements

### Requirement: Lifecycle events cannot predate their forecast
Every lifecycle event SHALL have an `effective_at` at or after the selected
forecast's `forecasted_at` and a `recorded_at` at or after both the event's
`effective_at` and the forecast's `recorded_at`. The event SHALL also retain the
existing non-decreasing event-time order, append-only recording order, unique
ID, and active/inactive transition rules. These rules SHALL be normative
Forecast Ledger semantic constraints and SHALL apply identically to direct
authoring, full-document validation, CLI, MCP, dry-run, JSON, and YAML.

#### Scenario: Withdrawal predates the forecast
- **WHEN** a withdrawal has an `effective_at` earlier than the selected forecast's `forecasted_at`
- **THEN** validation rejects the event at its `effective_at` with a stable lifecycle-chronology issue and no side effect

#### Scenario: Event recording predates the forecast record
- **WHEN** a lifecycle event has a valid effective time but its `recorded_at` precedes the forecast's `recorded_at`
- **THEN** validation rejects the event at its `recorded_at` even when `recorded_at` does not precede the event's own effective time

#### Scenario: Valid same-instant withdrawal
- **WHEN** a forecast is withdrawn with effective and recording times equal to the corresponding forecast lower bounds and all transition rules are satisfied
- **THEN** the lifecycle event is accepted and appended

### Requirement: Activity is derived from the ordered lifecycle stream
A forecast SHALL begin active. `withdrawn` and `expired` SHALL make it inactive,
and `reaffirmed` SHALL make it active again. Every forecast view and
verification result SHALL derive the same current state from the validated
ordered event stream and SHALL expose the event count plus the last event's
public identity and times when an event exists.

#### Scenario: Reaffirmed forecast is active
- **WHEN** a forecast has an ordered withdrawal followed by a reaffirmation
- **THEN** all shared service, CLI, MCP, and package views report it active and identify the reaffirmation as the last activity event

#### Scenario: Forecast has no lifecycle events
- **WHEN** a valid forecast has no lifecycle event stream
- **THEN** it is reported active with event count zero and without a fabricated last event

### Requirement: Lifecycle evidence uses a separate canonical binding
The contract SHALL define a closed, deterministic lifecycle evidence scope that
binds the selected question and forecast, the immutable
`forecast-envelope/v2` digest, the complete ordered lifecycle prefix, and the
current lifecycle head. It SHALL exclude integrity records from its own input
and SHALL use the bounded RFC 8785/JCS profile. Creating or appending lifecycle
events SHALL change this lifecycle binding but SHALL NOT change the existing
`forecast-envelope/v2` bytes or digest. Retained lifecycle evidence SHALL use
RFC 3161 with SHA-256 and the same local-only verification and trust-retention
boundaries as forecast evidence.

#### Scenario: Withdrawal follows verified forecast evidence
- **WHEN** a forecast with verified `forecast-envelope/v2` evidence receives a valid withdrawal
- **THEN** the original forecast target and evidence still verify, while the lifecycle binding changes to a new inactive head

#### Scenario: Same lifecycle prefix is deterministic
- **WHEN** independent conforming implementations bind the same forecast and ordered lifecycle prefix
- **THEN** they produce byte-identical canonical lifecycle target bytes and the same SHA-256 digest

### Requirement: Retained lifecycle evidence detects covered mutations
Verification SHALL compare every declared retained lifecycle target, digest,
chain or head reference, and RFC 3161 response with the current validated event
stream. A changed or deleted covered event, broken prefix, mismatched forecast
identity, altered ordering, or altered target byte SHALL fail the lifecycle
binding with a stable reason. Missing bytes needed to evaluate declared
evidence SHALL be incomplete or not checked rather than a cryptographic
mismatch. Removing all lifecycle events, declarations, and retained evidence
cannot prove that no event ever existed; an unbound stream SHALL therefore be
reported as current ledger state only and MUST NOT be described as complete or
tamper-proof.

#### Scenario: Covered withdrawal is deleted by hand
- **WHEN** a retained lifecycle binding covers a withdrawal and the withdrawal is removed from the ledger while its binding evidence remains available
- **THEN** lifecycle verification fails with a stable history or head mismatch while forecast content evidence remains independently valid

#### Scenario: All activity evidence is absent
- **WHEN** a forecast has no declared or discoverable retained lifecycle binding
- **THEN** verification reports the derived current activity as unbound and explicitly states that prior-event completeness was not established

#### Scenario: Declared lifecycle target is missing
- **WHEN** the ledger or package declares lifecycle evidence but a required local target, request, response, or retained trust file is unavailable
- **THEN** the lifecycle layer is incomplete or not checked and does not claim either a verified history or a proven mismatch

### Requirement: Corrected semantics come from one exact upstream contract
The lifecycle chronology, lifecycle binding, optional sealed-text semantics,
private bundle shape, reveal rules, reference builders, conformance cases, and
cryptographic vectors SHALL be published together as one exact Forecast Ledger
v2 contract correction. The CLI SHALL replace all contract provenance, schema,
archive, checksum-asset, fixture, vector, attribution, build, and documentation
pins together and SHALL reject the superseded schema version before side
effects. It MUST NOT hand-edit vendored bytes, fetch a floating tag, retain a
converter, or offer a compatibility bundle.

#### Scenario: Contract import is partial
- **WHEN** any retained schema, fixture, reference semantic, vector, attribution, version, commit, tag object, archive digest, checksum-asset digest, or schema digest disagrees with the selected published release
- **THEN** conformance and release checks fail before the binary or contract artifacts are published
