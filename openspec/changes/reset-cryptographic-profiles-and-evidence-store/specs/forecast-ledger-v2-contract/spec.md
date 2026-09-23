## Purpose

Defines the exclusive Forecast Ledger v2.2.0 contract identity and the exact
versioned cryptographic profiles that determine every canonical byte sequence.

## ADDED Requirements

### Requirement: Adopt one exact v2.2.0 contract
The runtime SHALL embed and accept exactly Forecast Ledger v2.2.0 after that
contract has been published as one immutable release. The schema, semantic
rules, reference builders, conformance fixtures, cryptographic vectors,
attribution, exact commit, annotated tag object, release archive digest,
checksum-asset digest, and schema digest MUST identify the same release.

The runtime MUST reject v2.1.0 and every other schema version before acquiring a
ledger lock, reading protected input or evidence artifacts, generating entropy,
creating a file, mutating a ledger, or opening a network connection.

#### Scenario: Exact v2.2.0 ledger is admitted
- **WHEN** a ledger identifies v2.2.0 and passes the exact embedded schema and semantic contract
- **THEN** the runtime admits it and reports the matching contract version, source identity, and schema digest

#### Scenario: V2.1.0 ledger is rejected before effects
- **WHEN** any CLI or MCP operation receives a v2.1.0 ledger
- **THEN** it returns `unsupported_schema_version` before locks, protected files, evidence files, entropy, writes, or network activity

### Requirement: Every changed canonical profile has a new identity
The v2.2.0 contract SHALL use exactly `forecast-seal/v3`, `forecast-key/v3`,
`forecast-envelope/v3`, and `forecast-lifecycle/v2`. Each identifier SHALL name
one closed structure, canonicalization rule, binding set, and published vector
set. No identifier retained from v2.1.0 MAY produce or accept changed canonical
bytes under v2.2.0.

The seal plaintext and associated data SHALL identify `forecast-seal/v3`; the
protected key file SHALL identify `forecast-key/v3`; forecast targets SHALL
identify `forecast-envelope/v3`; and lifecycle targets SHALL identify
`forecast-lifecycle/v2` and bind the SHA-256 digest of the corresponding v3
forecast envelope.

#### Scenario: Published vectors reproduce exact bytes
- **WHEN** the implementation receives every deterministic input from the published v2.2.0 seal, key, envelope, and lifecycle vectors
- **THEN** it reproduces each canonical byte sequence, digest, nonce, ciphertext, and identifier byte-for-byte

#### Scenario: Superseded profile appears in a v2.2.0 file
- **WHEN** a v2.2.0 ledger, key file, target, or package contains a superseded v2.1.0 profile identifier
- **THEN** validation fails without dispatching to a legacy parser or attempting legacy decryption

### Requirement: The cutover has no compatibility runtime
The v2.2.0 runtime MUST NOT contain a v2.1.0 schema, converter, dual-profile
parser, legacy key decoder, historical reveal path, compatibility flag, or
automatic rewrite. Public guidance SHALL direct operators to the exact v0.10.0
binary and pinned v2.1.0 source when they need to inspect historical v2.1.0
material.

#### Scenario: Operator requests an in-process conversion
- **WHEN** an operator presents v2.1.0 material to the v2.2.0 runtime or looks for a migration command
- **THEN** the runtime refuses the old material and exposes no conversion or compatibility operation

#### Scenario: Historical bytes remain attributable
- **WHEN** documentation describes how to inspect immutable v2.1.0 evidence
- **THEN** it names the exact historical release and source identity without implying that the current runtime accepts those bytes
