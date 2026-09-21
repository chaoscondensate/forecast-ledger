## Context

See `proposal.md` for motivation and the delta specs for observable behavior.
The current binary is internally consistent around Forecast Ledger v1.3.0, but
that consistency is the main implementation hazard: v1 field shapes are present
in the typed ledger model, service request structs, JSON Schema-derived request
definitions, CLI flag parsing, MCP dispatch, source patches, summaries,
cryptographic projections, timestamp metadata, publication manifests, fixtures,
and documentation.

The published v2.0.0 release is an intentionally incompatible contract. The
reviewed release identity is:

| Item | Value |
| --- | --- |
| Version | `2.0.0` |
| Commit | `1d3b186a15136bc5aff38647cb59fbef475dbe55` |
| Annotated tag object | `7b4a9e85e0df9350750828a57b03ff729f704ee4` |
| Published | `2026-09-21T10:29:27Z` |
| Release archive SHA-256 | `1d56cbe4f6cbd1fccb046a99add2ff4f88c709904d027669039ba4139664f47e` |
| `SHA256SUMS` SHA-256 | `77d093fbdb393dc9c1e3bdae053724e5ecdccd2e211f6f6c08c7678e78b178dc` |
| Schema SHA-256 | `efd87b7432f7cb017fbedaba217e4cd1d3bff06133457927fb786a249a040b21` |
| v2 seal-vector SHA-256 | `7cd814473f3e84617660e704f1aea8dc48e4b4a32cc86b93fd46c1649a4728d5` |

The release changes the semantic axes rather than adding fields to v1:

| V1.3 assumption | V2 replacement |
| --- | --- |
| Mutable question definition | Ordered immutable revisions |
| Four question/value unions | Six outcome domains plus seven representation kinds |
| Integer basis points | Canonical decimal strings and exact arithmetic |
| One type-specific `value` | One or more compatible `representations` |
| Unversioned options | Versioned option and bin sets |
| Question-ID-only forecast binding | Exact question-revision binding |
| `annulled` terminal state | `ambiguous`, `void`, and conditional `not_applicable` |
| Loose platform references | Object provenance with optional retained snapshot digest |
| No relationship or activity model | Groups, conditional DAG, and lifecycle event stream |
| `forecast-seal/v1` and envelope v1 | Revision-bound seal and envelope v2 |

The upstream schema defines local shape; its reference validator and crypto
tools are also normative inputs for semantic and byte-level parity. The Go
binary must remain self-contained at runtime and use the same application
services for CLI and MCP.

## Goals / Non-Goals

**Goals:**

- Make one coherent v2-only runtime cutover with no period where adapters,
  validation, cryptography, and public metadata disagree.
- Represent every v2 union with closed typed Go data and preserve exact source
  strings needed for canonicalization.
- Match every published v2 schema, semantic, and cryptographic conformance
  case, including retained-v2-artifact checks and negative cases.
- Give every supported non-secret authoring field an ordinary direct CLI and
  MCP route while preserving atomic writes and secret isolation.

**Non-Goals:**

- In-place conversion, automatic relabeling, dual-read operation, or a schema
  migration command.
- Scoring rules, platform API clients, platform-native import automation, Git
  publication, hosted publishing, or authorship signatures.
- A generic expression language, arbitrary-precision JSON numbers, or generic
  JSON/YAML authoring inputs.
- Editing a historical revision, forecast statement, lifecycle event, seal, or
  timestamp entry after it has acquired append-only meaning.

## Decisions

### 1. Vendor one exact v2 release

The active embedded contract, examples, invalid mutations, positive fixtures,
v2 vector, license, provenance record, and release pins will come unchanged
from the exact v2.0.0 release. The source record will include every digest above
and the individual retained-file digests checked by tests. Runtime constants,
build information, MCP metadata, package manifests, docs, and release audits
will read the same active identity.

Alternatives rejected:

- Fetching the tag or schema during build/test/runtime: not reproducible and
  violates offline validation.
- Editing upstream fixtures to fit Go behavior: reverses the source precedence.
- Keeping both versions in runtime dispatch: doubles every service and crypto
  invariant and conflicts with the established exclusive-admission policy.

### 2. Replace the v1 domain model rather than layering v2 onto it

`internal/ledger` will define closed discriminated unions for outcome space,
domain, values policy, forecast representation, relationship, resolution,
integrity, and commitment. A question contains revisions; a forecast contains a
revision ID and representation array. Shared scalar values will use a closed
boolean-or-string type, then domain-aware helpers will parse them as option ID,
canonical decimal, date, or timestamp. No public model field will use
`map[string]any` or `json.RawMessage` as an escape hatch.

Timestamp, date, decimal, slug, digest, nonce, ciphertext, and relative-path
types remain source strings so decoding never normalizes bytes later covered by
JCS. Clone and marshal helpers will exhaustively switch on every union branch;
unknown and multiply populated branches are errors.

Alternatives rejected:

- Extending `QuestionType` and `ForecastValue`: v2 deliberately separates
  outcome/domain from representation, so the v1 abstraction cannot express the
  contract without invalid combinations.
- Generic maps matching the schema: easy to decode, but unsafe for authoring,
  cloning, secret redaction, stable output, and cryptographic projection.
- Normalizing timestamps or decimals into floating types: loses authored bytes
  and can change equality, exact sums, and target hashes.

### 3. Use one exact-scalar package for all semantic comparisons

A small internal helper will validate the canonical decimal grammar before
parsing and use exact integer/rational arithmetic for comparison, addition, and
step alignment. It will enforce the probability digit/range rules separately
from unrestricted canonical numeric outcomes. Date and datetime helpers will
use calendar-day or exact-second arithmetic for bounds, allowed values, steps,
and ordering while retaining original strings in the model.

Domain-aware scalar operations will be the only path used by forecast,
relationship, and resolution checks. This prevents the CLI builder, service
validation, and conformance harness from acquiring slightly different numeric
rules.

Alternatives rejected:

- `float64`: cannot prove exact PMF/tail sums or stable 18-digit behavior.
- Repeated ad hoc string comparisons: incorrect for signed and differently
  sized decimals and too easy to diverge across representations.
- Adding a broad decimal framework: unnecessary when a bounded grammar plus
  standard-library exact arithmetic is sufficient.

### 4. Port reference semantics as ordered validation phases

Validation will remain schema-first and local, followed by named semantic
phases that mirror the pinned reference behavior:

1. root timezone, forecaster, platform, global question and forecast IDs;
2. question revision identity, chronology, outcome/domain agreement, bounds,
   values policies, options, and bins;
3. forecast revision binding, chronology, supersession, representation-kind
   uniqueness, domain compatibility, exact distributions, provenance,
   lifecycle, integrity, and revealed-bundle checks;
4. resolution shape, domain membership, sources, timestamps-before-outcome;
5. group and relationship references, membership uniqueness, conditional DAG,
   and not-applicable agreement;
6. confined SHA-256 checks for declared targets and provenance snapshots.

Schema failures stop semantic traversal so malformed union data cannot cause
panics or misleading follow-on issues. Semantic issue codes and locations will
be stable application data even if human messages improve. Artifact checks
receive an explicit ledger-relative root; validation never follows an
unconfined path or source URL.

Every retained upstream v2 case is vendored into normal deterministic Go tests
so release gates do not depend on Python or network access.

Alternatives rejected:

- Translating only the currently published invalid cases: would miss normative
  behavior in the reference validator and allow the next fixture to expose
  drift.
- Putting cross-record constraints into adapters: breaks CLI/MCP parity and
  makes read-only validation incomplete.

### 5. Make immutable revision append the only semantic question edit

Question creation builds one complete first revision. A new `question revise`
service accepts the full replacement semantic revision and appends it after
validating chronology and version changes. Existing `question update` is
reduced to mutable question-level metadata and nonterminal lifecycle fields;
it cannot edit revision content. Terminal state services become explicit
resolved, ambiguous, void, disputed, and not-applicable operations with their
distinct required fields. The v1 annul operation and request schema are
removed.

Forecast create/seal and resolved outcome requests require an explicit question
revision selector. The only exception is an initial forecast constructed in
the same atomic request as a new question's sole revision, where the binding is
structurally unambiguous.

Alternatives rejected:

- Reusing `question update` to clone an implicit revision: hides a major
  semantic operation and makes timestamps/version increments easy to miss.
- Defaulting later forecasts to `current_revision_id`: a concurrent or simply
  overlooked revision could silently change what the forecast means.

### 6. Treat the operation registry as the complete adapter contract

The service registry will enumerate every authoring operation, direct request
field, requiredness, enum, selector, control, side effect, protected reference,
and result type. New operation families are:

- question revision append and the v2 terminal resolution leaves;
- group create/update/remove/list/show;
- relationship add/remove/list/show, with guarded removal;
- forecast withdraw, expire, and reaffirm;
- provenance fields on revision, forecast, and lifecycle creation.

Forecast representation requests use typed nested objects in services and MCP.
CLI maps them from ordinary scalar and repeatable CSV field-group flags using
the existing escaping-aware parser. Each representation kind has distinct flag
names, so several kinds can coexist without an ambiguous `--value-kind` switch.
Required references such as revision, option-set version, and bin-set version
are standalone flags. Public flags never decode JSON/YAML or read public input
files. The command-surface inventory and generation tests fail on any registry,
CLI, MCP, help, or documentation mismatch.

For sealed forecasts, only the private representation array and private
reasoning remain in the protected input schema. Revision ID and other public
metadata are direct fields and are checked against the authenticated bundle.

Alternatives rejected:

- One giant generic authoring command: poor help, ambiguous conditional fields,
  and difficult least-capability MCP exposure.
- One CLI leaf per representation combination: combinatorial and cannot express
  multiple compatible representations cleanly.
- JSON string flags: recreate the forbidden document wrapper and expose quoting
  and secret-leak hazards.

### 7. Keep source-preserving patching below all new services

Builders produce complete typed records in schema field order. File services
lock, parse, validate, clone, mutate, validate again, create any required
protected/artifact files in safe order, and commit through the existing journal
and sibling-replace mechanism. JSON and YAML patch values use the same ordered
builder; populated v2 revisions, domains, representations, provenance,
relationships, events, and resolutions render as two-space block YAML.

Append operations patch only the addressed array or optional root collection.
Removal checks all inbound references before patching. Unrelated source bytes,
including comments and line endings, remain untouched.

Alternatives rejected:

- Re-serializing the whole document: destroys source preservation and reviewable
  diffs.
- Hand-building YAML fragments in individual services: repeats the indentation
  and field-order bugs already eliminated by the shared renderer.

### 8. Implement v2 cryptography as a new closed profile

The v2 seal implementation gets its own payload, associated-data, key-file,
and projection types. Its key file is application-owned and will be versioned
as `forecast-key/v2`, binding question, revision, and forecast IDs.

The target builder resolves the forecast's exact revision and constructs the
normative nested `question: {id, revision}` v2 envelope. A single allowlisted
projection function includes all and only the upstream public forecast fields;
sealed and revealed forecasts share the original sealed projection. Projection
tests compare Go bytes and SHA-256 to the upstream tool/vector and fail if a new
model field is not classified as included, excluded, or secret.

RFC 3161 transport and cryptographic verification remain protocol-compatible,
but every stored target scope, request imprint, verification check, package
entry, and result now binds the v2 envelope. Multiple TSA branches and provider
failover stay orthogonal to the schema cutover.

Alternatives rejected:

- Mutating the old structs and constants in place: makes it too easy for removed
  shapes to survive in v2 behavior.
- Including only the revision ID in the target: permits reinterpretation if the
  referenced revision bytes are later tampered with.

### 9. Cut over generated output and documentation in the same commit series

Stable result envelopes and error categories remain where their meaning has not
changed, but v1 field summaries, enums, examples, flags, tool schemas, resource
descriptions, version pins, and package metadata are regenerated from the v2
registry/model. Human summaries name revision and representation kinds without
rounding exact values.

README, getting started, how-to, reference, explanation, security, release,
third-party, and documentation-baseline material will be audited
against an actually built candidate binary. Examples use the real command tree
and protect private bundles. The changelog explicitly marks the file-format and
crypto-profile break and states that no migration is provided because there
were no active users.

## Risks / Trade-offs

- **[Reference parity drift]** The schema alone does not express many v2 rules.
  → Port every reference-validation phase, vendor every fixture, compare case by
  case with the exact upstream harness, and add property/fuzz tests around
  decimals, bins, graphs, dates, and unions.
- **[Cryptographic incompatibility]** A missing optional field or normalized
  string changes envelope/seal bytes. → Use closed projection types, exhaustive
  field-classification tests, the published vector, and cross-platform release
  checks. Keep the absence of an independent review prominent in public
  security and maturity documentation; external review is desirable follow-up
  work rather than a `0.9.0` release gate.
- **[Large authoring surface]** Six domains and seven representations can make
  CLI help unwieldy. → Use dedicated lifecycle/entity leaves, consistent flag
  families, copyable per-kind examples, generated inventory checks, and no
  generic fallback.
- **[Accidental historical mutation]** Reusing v1 question-update code could
  rewrite a bound definition. → Separate question metadata patching from the
  append-only revision service and reject edits/removals of referenced records.
- **[Exact arithmetic or timezone edge cases]** Negative decimals, steps,
  offsets, DST, and date/datetime origins are easy to mishandle. → Keep authored
  strings, centralize exact parsing, test boundary values and independent
  reference cases, and never use binary floats.
- **[Artifact-root ambiguity]** Provenance snapshots introduce digest checks to
  ordinary validation. → Pass a resolved ledger root explicitly, apply existing
  confinement and symlink policy, and return a clear incomplete/invalid result
  when stdin has no usable sibling context.
- **[Unsupported schema input]** A document may declare a non-v2 version. → Fail
  before side effects with the supported version and do not provide a converter
  or compatibility path.
- **[Active-change conflict]** Timestamp failover, command-surface, and docs
  changes still contain v1 shapes. → Reconcile their registries, tasks, and docs
  during implementation and run strict validation for every active change.

## Migration Plan

1. Verify the downloaded v2.0.0 release assets and check in the unchanged
   retained v2 contract, fixtures, vector, license, provenance, and digest
   records.
2. Replace the ledger model, exact scalar helpers, and full local validator;
   make all published Go/reference conformance tests pass before enabling any
   mutation service.
3. Replace builders and application services for revisions, domains,
   representations, resolutions, groups, relationships, provenance, and
   lifecycle events; preserve transactional and source-patch behavior.
4. Implement and verify v2 sealing, reveal, target generation, timestamp
   binding, and publication packaging with published vectors, negative,
   property, fuzz, parity, and cross-platform tests before exposing them
   through adapters. Record that no independent audit has occurred and defer
   external review without claiming that it is complete.
5. Cut CLI, MCP, generated contracts, presentation, examples, and docs to v2,
   then run denylist checks for every removed v1 field, command, protocol, and
   public claim.
6. Release as a breaking CLI version. No data migration or compatibility
   bundle is provided because there were no active users.

Rollback means reverting the source change before release. Implementation must
never rewrite an unsupported ledger, so rollback does not require reversing
user data.
