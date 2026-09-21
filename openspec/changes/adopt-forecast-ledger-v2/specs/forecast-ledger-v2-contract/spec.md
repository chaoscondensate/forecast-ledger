## Purpose

Defines the exact Forecast Ledger v2.0.0 contract that the application embeds,
accepts, validates, reports, and packages without runtime schema retrieval.

## ADDED Requirements

### Requirement: Exact immutable v2.0.0 source identity
The application SHALL embed the unchanged Forecast Ledger `v2.0.0` schema from
commit `1d3b186a15136bc5aff38647cb59fbef475dbe55`, annotated tag object
`7b4a9e85e0df9350750828a57b03ff729f704ee4`, release archive SHA-256
`1d56cbe4f6cbd1fccb046a99add2ff4f88c709904d027669039ba4139664f47e`,
`SHA256SUMS` asset SHA-256
`77d093fbdb393dc9c1e3bdae053724e5ecdccd2e211f6f6c08c7678e78b178dc`,
and schema SHA-256
`efd87b7432f7cb017fbedaba217e4cd1d3bff06133457927fb786a249a040b21`.
The schema, license, examples, conformance cases, v2 vectors, and reference
behavior SHALL be reviewed and pinned as one release identity;
vendored upstream bytes MUST NOT be edited to match existing code.

#### Scenario: Release identity is reproducible
- **WHEN** release conformance checks inspect the vendored v2 material
- **THEN** the tag, commit, release assets, schema, license, fixture, and vector digests match the declared immutable source

### Requirement: Exclusive v2 admission without implicit conversion
The runtime SHALL accept only root `schema_version: 2.0.0`. It SHALL reject
missing and non-current versions with stable code
`unsupported_schema_version` before mutation, protected-file creation,
artifact creation, entropy use, or network access. CLI diagnostics SHALL retain
the established warning and exit-3 contract.

#### Scenario: Supported v2 ledger is admitted
- **WHEN** an otherwise valid ledger declares `schema_version: 2.0.0`
- **THEN** version admission succeeds and local structural and semantic validation continues

#### Scenario: Unsupported version is rejected without side effects
- **WHEN** any operation receives a document declaring a non-current schema version
- **THEN** it reports `unsupported_schema_version`, identifies `2.0.0` as supported, and performs no write, entropy read, protected-file creation, or network request

### Requirement: Local schema and reference-semantic validation
Validation SHALL use the embedded Draft 2020-12 schema and SHALL implement the
published v2 reference semantics for rules that require exact arithmetic,
cross-record lookup, chronology, graph traversal, artifact digests, domain
membership, lifecycle state, or reveal authentication. Local validation MUST
NOT resolve the permanent schema URL, fetch a remote reference, contact a
platform or TSA, or use the system trust store.

#### Scenario: Offline validation exercises both layers
- **WHEN** a v2 ledger is validated without network access
- **THEN** embedded schema checks and every applicable published semantic check run locally and produce the same accept-or-reject result as the pinned reference corpus

#### Scenario: Artifact-backed semantics remain confined
- **WHEN** validation checks a declared target or provenance snapshot digest
- **THEN** it reads only the safely resolved local artifact and never retrieves the declared source URL

### Requirement: Complete published v2 conformance coverage
The application SHALL accept every published v2 valid JSON and YAML fixture,
reject every published invalid mutation for its intended rule, validate the
schema metaschema, reproduce `forecast-seal/v2` and
`forecast-envelope/v2` outputs byte-for-byte. Go conformance coverage SHALL
cover every retained published v2 case.

#### Scenario: Published fixture suite has parity
- **WHEN** the pinned valid examples, relationship/datetime fixture, and invalid-case corpus are run through the application and reference harness
- **THEN** both implementations agree for every case, including exact sums, option and bin versions, distributions, references, cycles, lifecycle, resolution, chronology, reveal, and OpenTimestamps rejection

### Requirement: One v2 identity across public surfaces
Every surface that reports or records the supported contract SHALL use the same
v2 version, commit, tag, schema digest, seal profile, and envelope profile.
This includes version output, MCP initialization and resources, validation
results, generated examples and schemas, package manifests, attribution, and
release checks.

#### Scenario: Adapter and package metadata agree
- **WHEN** a caller reads CLI version JSON, MCP initialization metadata, and a newly built publication manifest from the same binary
- **THEN** all three identify v2.0.0, the pinned commit and digest, `forecast-seal/v2`, and `forecast-envelope/v2`
