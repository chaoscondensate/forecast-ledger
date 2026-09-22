## Purpose

Defines the corrected v2 contract adoption and user-visible reliability rules
needed to close the confirmed Forecast Ledger CLI v0.9.0 dogfooding findings.

## ADDED Requirements

### Requirement: Lifecycle-stable targets come from one corrected upstream contract
The normative Forecast Ledger `v2.0.1` contract SHALL exclude
`lifecycle_events` from both public and sealed `forecast-envelope/v2`
projections because lifecycle activity is not part of the recorded belief. The
CLI MUST adopt those exact upstream semantics and retained artifacts without a
local profile fork. The contract version, exact commit, annotated tag object,
release archive digest, checksum-asset digest, schema digest, attribution,
fixtures, reference semantics, vectors, application metadata, and
documentation SHALL identify commit
`55b1431d379128e1d75b9c30a3874398cea9ff0f`, tag object
`ac69f718de92f2c087b44b1b02d325ba37945db6`, release archive SHA-256
`bfe00b166efdef229848afd6d623eeb965507f314482e59b9c0663bb87ff7a41`,
checksum-asset SHA-256
`fa4c72bc5f4f27dd936e95cf9ad61fdee7c1d12ecb17ca847ac00b919c8664d6`,
and schema SHA-256
`5ceeeca7b2b3e46884c116aeb4f20998daf619569c0191728f574c245a4a595b`
consistently.

#### Scenario: Upstream and CLI omit lifecycle activity identically
- **WHEN** the corrected upstream reference builder and the CLI build a target for the same forecast containing lifecycle events
- **THEN** both produce byte-identical `forecast-envelope/v2` output in which the lifecycle events are absent

#### Scenario: A floating or partial contract update is attempted
- **WHEN** retained contract bytes or target semantics do not match the configured exact release provenance and digests
- **THEN** conformance and release checks fail before the binary or contract artifacts are published

### Requirement: Lifecycle activity preserves retained forecast evidence
Appending a valid withdrawal, expiry, or reaffirmation SHALL NOT change the
canonical target bytes or target digest of the affected forecast. Local target
checks, layered verification, and publication verification SHALL continue to
evaluate retained evidence independently of the forecast's derived active
state. The lifecycle history and current active state SHALL remain visible as
separate ledger facts.

#### Scenario: A timestamped forecast is withdrawn
- **WHEN** a valid withdrawal is appended after a forecast has a retained target and verified RFC 3161 timestamp
- **THEN** content binding and existence timing still pass against the original target while the forecast is reported inactive

#### Scenario: A forecast is withdrawn before target construction
- **WHEN** a valid withdrawal exists before the first target is built
- **THEN** the target excludes the event and is identical to the target for the same recorded belief before that event was appended

#### Scenario: A v2.0.0 ledger is presented to the corrected runtime
- **WHEN** any CLI or MCP operation receives a ledger declaring schema version `2.0.0`, including one with retained lifecycle-bearing target bytes
- **THEN** the operation rejects the unsupported schema before locks, artifact reads or writes, entropy, or network effects and does not attempt a compatibility target check

### Requirement: Default revision times satisfy strict ordering
When a question revision omits `effective_at`, the shared application service
SHALL derive a timestamp strictly later than the preceding revision even when
the operation clock is equal to or earlier than that revision's timestamp. If
`recorded_at` is also omitted, its derived value MUST NOT precede the derived
effective time. Explicit timestamps SHALL retain their normal input semantics
and invalid explicit chronology MUST still be rejected. CLI and MCP SHALL
produce equivalent results from equivalent clock observations and ledgers.

#### Scenario: Add and revise share one clock second
- **WHEN** a question is added and immediately revised without either revision supplying `effective_at` or `recorded_at`, and both operations observe the same clock second
- **THEN** the revision succeeds with strictly increasing effective time and valid non-decreasing recorded time

#### Scenario: Caller explicitly repeats the prior effective time
- **WHEN** a revision explicitly supplies an `effective_at` equal to the preceding revision's effective time
- **THEN** the operation fails atomically with the stable invalid-field classification

### Requirement: Default operation times use the ledger timezone
Every timestamp derived from the operation clock for an existing ledger SHALL
be formatted in that ledger's `default_timezone` before it reaches the shared
service. This includes forecast reveal time and every resolved, unresolved,
disputed, and not-applicable resolution recording time. Equivalent CLI and MCP
operations SHALL write the same offset for the same clock instant and ledger
timezone. Explicit RFC 3339 timestamps SHALL remain caller-controlled subject
to existing validation and normalization rules.

#### Scenario: Reveal defaults in a UTC ledger
- **WHEN** a caller reveals a sealed forecast without `revealed_at` in a ledger whose `default_timezone` is `UTC`
- **THEN** the stored reveal time is expressed with `Z` in both CLI and MCP paths

#### Scenario: Resolution defaults in a non-UTC ledger
- **WHEN** a caller records a terminal resolution without `recorded_at` in a ledger whose timezone has a numeric offset at the operation instant
- **THEN** the stored recording time uses that ledger-local offset rather than the host process timezone

### Requirement: Protected-input failures identify the protected artifact
When an existing protected input such as a forecast key cannot be found, the
operation SHALL return the stable `not_found` category and a safe diagnostic
that names the missing artifact type rather than claiming the ledger is
missing. The diagnostic and structured result MUST NOT disclose key contents,
private forecast values, salts, credentials, or an unrestricted filesystem
path. CLI and MCP SHALL preserve equivalent classifications and messages.

#### Scenario: Reveal key is absent while the ledger exists
- **WHEN** `forecast reveal` selects an existing ledger and forecast but the requested protected key file does not exist
- **THEN** the operation returns `not_found` and reports that the key file does not exist without changing the ledger

### Requirement: Public target documentation matches canonical bytes
Maintained target documentation SHALL describe the root fields and exclusions
of the normative envelope exactly. It SHALL identify `question.id`, the full
bound question revision, and the selected forecast projection, and SHALL NOT
claim that a root ledger ID is present when canonical bytes contain none.
Lifecycle events SHALL be documented as mutable activity outside the target.
The compatibility text SHALL state that the current runtime accepts only
v2.0.1 and that v2.0.0 evidence requires the immutable old contract rather than
an in-process compatibility path.

#### Scenario: A reader follows the target how-to
- **WHEN** a reader compares the documented `forecast-envelope/v2` shape with a built target
- **THEN** every documented root field is present, no undocumented ledger ID is expected, and lifecycle activity is correctly described as excluded
