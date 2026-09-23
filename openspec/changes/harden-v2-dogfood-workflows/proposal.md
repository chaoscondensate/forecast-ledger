## Why

Dogfooding the published `v0.9.0` binary found two high-priority failures in
new v2 workflows and four smaller correctness or documentation defects. Most
critically, a normal lifecycle event invalidates retained timestamp evidence,
and relationship authoring is unusable on the documented default YAML format.

## What Changes

- **BREAKING** Adopt the published Forecast Ledger `v2.0.1` lifecycle erratum
  atomically so `forecast-envelope/v2` excludes append-only
  `lifecycle_events`, without locally diverging from the pinned upstream
  semantics or retaining the superseded v2.0.0 runtime contract.
- Preserve v2.0.1 target bytes and RFC 3161 verification when a forecast is
  withdrawn, expired, or reaffirmed after it was timestamped. Document the
  release boundary: current CLI binaries reject v2.0.0 ledgers before evidence
  checks, while old evidence remains interpretable only against the immutable
  upstream v2.0.0 release outside the single-contract runtime.
- Serialize both relationship union variants through the same ordered JSON
  projection used by other source-preserving mutations so YAML and JSON
  `relationship add` operations have equivalent validated results through CLI
  and MCP.
- Make an omitted revision `effective_at` produce a valid strictly increasing
  timestamp even when add and revise observe the same clock tick, while keeping
  explicit invalid chronology an error.
- Make missing protected key input report the key file, not the ledger, while
  preserving the stable `not_found` classification and secret-safe output.
- Format default reveal and terminal-resolution recording times in the
  ledger's `default_timezone` consistently across CLI and MCP.
- Correct target documentation to describe the actual envelope (`question.id`,
  full bound revision, and forecast projection) without claiming a root
  `ledger_id` field.
- Add release, conformance, YAML/JSON parity, target-continuity, timezone, and
  diagnostic regressions. The unrelated observations and older open findings
  in the dogfooding report remain outside this change.

## Capabilities

### New Capabilities

- `v2-dogfood-reliability`: Coordinated corrected-contract adoption and the
  lifecycle-evidence, monotonic-default, timezone, protected-input diagnostic,
  and target-documentation behavior found by v0.9.0 dogfooding.

### Modified Capabilities

- `yaml-structural-mutations`: Require discriminated-union additions such as
  v2 relationships to retain their flattened schema shape and YAML/JSON
  semantic parity.

## Impact

- Upstream dependency is satisfied by `chaoscondensate/schema` `v2.0.1` at
  commit `55b1431d379128e1d75b9c30a3874398cea9ff0f`, annotated tag object
  `ac69f718de92f2c087b44b1b02d325ba37945db6`, schema SHA-256
  `5ceeeca7b2b3e46884c116aeb4f20998daf619569c0191728f574c245a4a595b`,
  release archive SHA-256
  `bfe00b166efdef229848afd6d623eeb965507f314482e59b9c0663bb87ff7a41`,
  and checksum-asset SHA-256
  `fa4c72bc5f4f27dd936e95cf9ad61fdee7c1d12ecb17ca847ac00b919c8664d6`.
- Affects embedded contract material and metadata in `internal/schema` and
  `internal/buildinfo`, target construction and verification in
  `internal/service`, union patch encoding in `internal/document` and service
  builders, protected-file path diagnostics in `internal/storage`, and
  operation-time handling in both adapters.
- The CLI/MCP request surfaces and stable error codes do not gain new fields.
  Existing `v2.0.0` ledgers are rejected before locks or evidence side effects;
  no converter, compatibility reader, dual verifier, or compatibility bundle
  is added.
- Documentation impact includes README/status text as applicable, target and
  lifecycle how-tos, reference pages, security/evidence limits, documentation
  baseline, changelog, release instructions, and the maintained command and
  contract inventories.
> Superseded for current implementation by
> `reset-cryptographic-profiles-and-evidence-store`. The v2.2 runtime does not
> retain the profile identities described here.
