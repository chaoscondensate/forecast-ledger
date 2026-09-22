## Context

See `proposal.md` for motivation and the delta specs for required behavior.
The tested binary is the published `v0.9.0` commit `4ff13ef`. Its embedded
Forecast Ledger contract is the exact upstream `v2.0.0` release at commit
`1d3b186a15136bc5aff38647cb59fbef475dbe55`.

The L1 behavior was not only a CLI projection mistake. The pinned v2.0.0
`tools/forecast_crypto.py` included `lifecycle_events` in public and sealed
`forecast-envelope/v2` projections. Upstream has now published the corrective
`v2.0.1` contract at commit `55b1431d379128e1d75b9c30a3874398cea9ff0f`.
Its reference projections exclude lifecycle events, its public and sealed
target vectors fix exact canonical bytes and digests, and its validation and
release workflows passed. The other findings remain local application or
documentation defects.

The application already has the right safety boundaries: transport-neutral
services, source-preserving patches, prospective validation, atomic writes,
protected secret files, and deterministic retained evidence. The design keeps
those boundaries and makes narrow corrections inside them.

## Goals / Non-Goals

**Goals:**

- Make lifecycle activity orthogonal to the immutable recorded-belief target
  in both the upstream reference implementation and the CLI.
- Adopt the corrected contract with complete provenance and conformance pins,
  never a hand-edited vendored file or floating tag.
- Restore YAML/JSON and CLI/MCP parity for the confirmed authoring paths.
- Centralize strict default-time and artifact-label behavior sufficiently that
  the same defect cannot survive in another adapter call path.
- Leave actionable release and compatibility evidence for independent review.

**Non-Goals:**

- A v2.0.0 converter, compatibility reader, dual target-profile verifier, or
  retained superseded contract bundle.
- Changing lifecycle state-machine rules, relationship types, command flags,
  MCP request properties, stable exit codes, or the RFC 3161 protocol.
- Addressing unclassified observations from round 13 or older findings carried
  forward by the dogfooding report.
- Reformatting whole YAML documents or changing unrelated source bytes.

## Decisions

### 1. Import the published target-profile correction exactly

Use the published `v2.0.1` release archive, not a floating branch or the local
schema checkout. Before import, verify the commit
`55b1431d379128e1d75b9c30a3874398cea9ff0f`, annotated tag object
`ac69f718de92f2c087b44b1b02d325ba37945db6`, release archive SHA-256
`bfe00b166efdef229848afd6d623eeb965507f314482e59b9c0663bb87ff7a41`,
`SHA256SUMS` asset SHA-256
`fa4c72bc5f4f27dd936e95cf9ad61fdee7c1d12ecb17ca847ac00b919c8664d6`,
and schema SHA-256
`5ceeeca7b2b3e46884c116aeb4f20998daf619569c0191728f574c245a4a595b`.

Replace the CLI's retained v2.0.0 material with the new exact release, update
every source/build/schema pin, and make
`targetForecast` classify lifecycle events as excluded. A target vector with an
event present will compare the upstream and Go canonical bytes and digest.

Changing only `internal/service/target.go` is rejected because it would make the
CLI disagree with the authoritative reference implementation while continuing
to claim the same contract identity. Blocking lifecycle events after evidence
exists is also rejected: it contradicts the accepted lifecycle behavior and
prevents honest withdrawal of an already timestamped forecast.

The profile name remains `forecast-envelope/v2`; the contract release records
the corrected normative projection. Strict schema-version handling provides
the contract boundary, and the CLI does not retain v2.0.0 runtime support.

### 2. Reject the superseded contract before interpreting its evidence

The corrected runtime accepts only schema version v2.0.1. Any v2.0.0 ledger,
including one carrying a target produced after a lifecycle event, is rejected
as unsupported before locks, artifact access, entropy, or network effects. The
runtime does not compare those bytes against the v2.0.1 projection and does not
claim a content mismatch under semantics the ledger did not declare.

Evidence explicitly created under v2.0.0 remains interpretable from the
immutable upstream v2.0.0 tag, outside this single-contract CLI. Documentation
will state that boundary without offering an automatic conversion path.

A compatibility verifier that selects projection behavior from retained target
bytes is rejected because it would preserve superseded semantics, complicate
the evidence claim, and conflict with the repository's single-contract policy.

### 3. Normalize union patch values through their JSON contract shape

`ledger.Relationship` flattens its selected variant through `MarshalJSON` but
has no equivalent YAML marshaler. When the first relationship creates the
whole collection, the current builder passes the typed Go union directly to
the YAML encoder, which emits implementation fields instead of the schema
shape. Appending to an existing collection already uses the JSON-normalized
patch path.

Collection creation and append paths for discriminated unions will both convert
the selected value or collection to the document layer's ordered JSON value
before either renderer sees it. This retains custom union flattening, stable
field order, and one semantic value for JSON and YAML without teaching the
generic document package about ledger domain types. Tests cover both
relationship variants, absent and populated collections, CLI and MCP routing,
dry-run, source preservation, and atomic failure.

Adding domain-specific YAML marshalers is rejected because JSON is the model's
closed union contract and a second hand-maintained shape could drift.

### 4. Derive omitted revision times relative to both the clock and history

Only an omitted `effective_at` receives repair behavior. The service will parse
the operation observation and the previous revision times, then choose an
effective instant strictly greater than the previous effective instant: the
operation observation when already later, otherwise the smallest supported
increment after the previous value. An omitted `recorded_at` will be at least
the operation observation, the derived effective time, and the previous
recorded time. Formatting uses RFC 3339 with only the fractional precision
needed to represent the selected instant.

This logic belongs in the shared question-revision service so CLI and MCP
cannot diverge. Explicit caller values are never silently advanced; existing
chronology validation rejects them when invalid. Merely switching the adapters
to `RFC3339Nano` is rejected because equal or non-monotonic clock observations
would still be possible and the domain invariant belongs below the adapters.

### 5. Use one ledger-timezone operation observation in every adapter path

Existing-ledger commands will load the ledger timezone and format the operation
clock through the existing timezone-aware helper before passing a default to a
service. `forecast reveal` will join the other mutation paths instead of using
the host clock offset directly. Question terminal operations already load the
timezone for explicit input normalization; they will reuse it for the default
recording time. MCP already follows this rule and gains parity regressions.

The service continues to accept explicit RFC 3339 values so deterministic
tests and non-CLI callers remain possible. Reformatting all stored timestamps
is rejected because offsets are valid source facts and unrelated bytes must be
preserved.

### 6. Give existing-file resolution a purpose label

Generalize the safe existing-regular-file resolution/read path so callers
supply a non-secret purpose label such as `ledger file`, `key file`, or
`protected input`. Missing-file, unreadable-file, and inspection diagnostics
will use that label while retaining the current application error category and
path-safety checks. Protected reads still verify native owner-only protection
before returning bounded bytes and never include contents or unrestricted paths
in output.

String replacement at the reveal call site is rejected because the misleading
message originates in a shared storage helper and can affect other protected
inputs.

### 7. Make regression coverage reproduce the dogfooding boundaries

Import the no-network relationship and same-tick revision reproductions into
deterministic service/CLI/MCP tests. Use retained RFC 3161 fixtures for
lifecycle continuity; no live TSA gates tests. Add fixed clocks in UTC and a
DST-aware non-UTC zone for reveal and terminal resolution defaults. Exercise
missing keys through normal CLI and MCP-safe error presentation. Compare target
bytes against the corrected upstream vectors and retain an unsupported-v2.0.0
negative case that fails before target artifacts are consulted.

Documentation updates will be checked with `internal/doccheck`, copyable command
tests, generated contract inventories, and the release checklist. D1 is fixed
as prose because the canonical envelope and accepted design contain
`question.id`, not a root `ledger_id`.

## Risks / Trade-offs

- **[An imported file differs from the published v2.0.1 release]** → Verify the
  archive, checksum asset, schema, tag object, and every retained file before
  writing embedded material; reject partial or hand-edited imports.
- **[Changing exact contract version rejects otherwise structurally identical
  v2.0.0 ledgers]** → Mark the cutover as breaking, keep rejection before side
  effects, document manual re-authoring expectations, and do not claim a
  migration path the repository does not implement.
- **[Previously stamped v2.0.0 targets contain lifecycle events]** → Reject the
  declaring ledger as unsupported before artifact access, point to the frozen
  upstream v2.0.0 contract for historical interpretation, and never
  auto-rebuild, restamp, or reinterpret the bytes.
- **[A nanosecond increment produces surprising visible precision]** → Emit
  fractional digits only when required for strict ordering and cover the exact
  JSON/YAML output in adapter tests.
- **[A generalized file-label API changes unrelated diagnostics]** → Enumerate
  every caller, assert stable categories and redaction, and limit message
  changes to replacing the incorrect artifact noun.
- **[Relationship normalization changes field order or formatting]** → Compare
  semantic models and expected expanded block YAML while asserting unrelated
  source bytes and JSON results remain unchanged.

## Migration Plan

1. Treat the published upstream v2.0.1 release and its successful validation
   and release workflows as the completed external prerequisite.
2. Verify the upstream commit, annotated tag object, release archive,
   `SHA256SUMS`, schema digest, and every retained file before importing the
   new contract. Replace v2.0.0 material rather than retaining both versions.
3. Update the Go target projection and conformance tests, then implement the
   independent YAML union, timestamp-default, timezone, and diagnostic fixes.
4. Update all affected public documentation and generated inventories, with an
   explicit unsupported-v2.0.0 boundary and no compatibility claim.
5. Run schema/reference conformance, deterministic crypto and RFC 3161 tests,
   YAML/JSON and CLI/MCP matrices, documentation checks, normal Go checks,
   fuzz/property coverage, and native filesystem workflows before release.

Rollback is a normal source rollback before publishing the corrected CLI. Once
published, do not silently return to v2.0.0 semantics under the same binary
version; issue a new release and preserve evidence diagnostics.
