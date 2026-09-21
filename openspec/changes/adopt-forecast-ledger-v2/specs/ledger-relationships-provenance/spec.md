## Purpose

Defines portable v2 question relationships and imported-object provenance,
including reference integrity, graph safety, and local snapshot evidence.

## ADDED Requirements

### Requirement: Groups and memberships use stable references
Optional groups SHALL have unique stable IDs and titles. A group-membership
relationship SHALL have its own unique ID and SHALL reference one existing group
and one existing question. The same group/question membership pair MUST NOT be
duplicated. Adding, listing, showing, updating, and removing groups or
memberships SHALL use direct CLI fields and closed top-level MCP properties and
SHALL not rewrite questions or forecasts.

#### Scenario: Membership references a missing record
- **WHEN** a relationship names a group or question that does not exist
- **THEN** validation and authoring reject it without changing the ledger

### Requirement: Conditional relationships are typed and acyclic
A conditional relationship SHALL identify an existing parent question, one
exact parent revision, an outcome belonging to that revision's domain, and an
existing child question. Conditional edges SHALL form an acyclic graph.
Authoring SHALL reject self-reference, missing or incompatible references, and
any edge that would create a cycle before mutation.

#### Scenario: New condition would create a cycle
- **WHEN** adding a conditional relationship would make any question reachable from itself
- **THEN** the operation fails atomically and reports the relationship conflict

#### Scenario: Child becomes not applicable
- **WHEN** a parent resolves to an outcome different from the condition and the child is marked `not_applicable`
- **THEN** the child resolution records that exact conditional relationship and validates successfully

### Requirement: Not-applicable resolution agrees with its condition
A `not_applicable` resolution SHALL reference a conditional relationship whose
child is the selected question. It MUST NOT be valid when the parent has
resolved to the activating outcome. Missing, non-conditional, wrong-child, and
satisfied-condition references SHALL be rejected.

#### Scenario: Satisfied condition cannot produce not applicable
- **WHEN** the parent has resolved to the relationship's activating outcome
- **THEN** validation rejects a `not_applicable` child resolution using that relationship

### Requirement: Provenance binds a known platform and optional retained bytes
Question revisions, forecasts, and lifecycle events MAY carry provenance that
names an existing root platform, remote object ID, retrieval time, optional
remote version, URL, source times, importer identity, and optional snapshot
artifact with SHA-256 digest. Source timestamps SHALL remain untrusted claims.
Snapshot checking SHALL use a safely confined local path and SHALL prove only
byte equality, not authorship or retrieval time.

#### Scenario: Snapshot digest matches retained bytes
- **WHEN** local validation encounters provenance with a retained snapshot
- **THEN** it confines and hashes the named local artifact, reports mismatch or absence, and performs no source-platform request

#### Scenario: Provenance names an unknown platform
- **WHEN** any provenance object references a platform absent from the ledger registry
- **THEN** semantic validation rejects the reference at its provenance location

### Requirement: Relationship and provenance authoring is adapter-equivalent
Every non-secret field accepted by relationship and provenance services SHALL
have an ordinary CLI flag or dedicated subcommand and an equivalent closed MCP
property. Ordered values and omission semantics SHALL be preserved. No adapter
MUST accept a generic input document, public side-loaded payload, or
platform-native blob as an alternative authoring route.

#### Scenario: Equivalent imported revision is authored twice
- **WHEN** the same revision provenance is supplied through CLI and MCP against equivalent ledgers
- **THEN** both adapters write semantically identical provenance and return equivalent stable outcomes
