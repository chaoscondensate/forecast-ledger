## Why

Forecast lifecycle events can currently predate both their forecast and its
question revision, and layered verification omits the forecast's current
activity even though the release notes promise a separate activity result.
Sealed authoring also requires three optional textual fields, then reports an
unhelpful whole-file error when protected input omits a required field.

## What Changes

- **BREAKING** Adopt the next exact published Forecast Ledger v2 contract
  correction as a single-contract cutover, including its schema, reference
  semantics, conformance fixtures, seal vectors, provenance pins, and digests;
  do not retain the superseded contract or a compatibility bundle.
- Require lifecycle event effective and recording times to respect the bound
  question revision and forecast chronology, in addition to the existing event
  ordering and transition rules.
- Add a lifecycle-specific canonical binding and retained evidence path that is
  separate from `forecast-envelope/v2`, so activity changes do not invalidate
  recorded-belief evidence while retained lifecycle evidence can reveal event
  mutation or deletion.
- Report every selected forecast's derived active state, lifecycle history
  summary, and binding status as a distinct verification layer in CLI, MCP,
  package, human, plain, and JSON results. State explicitly when lifecycle is
  current ledger state only and therefore cannot prove completeness after all
  lifecycle evidence has been removed.
- Make `rationale`, `key_factors`, and `comment` optional in sealed protected
  input, the authenticated private bundle, and the revealed forecast; continue
  to require at least one `representation`.
- Return protected-input diagnostics that identify the exact missing or invalid
  property and its source location without exposing secret values or unsafe
  paths.
- Update documentation, generated request/result contracts, compatibility
  statements, release evidence, and deterministic regression fixtures together.

## Capabilities

### New Capabilities

- `forecast-lifecycle-integrity`: Defines lifecycle chronology, derived
  activity, a lifecycle-only canonical binding, and the limits of deletion
  detection.
- `sealed-forecast-private-bundle`: Defines required and optional private bundle
  fields, exact seal/reveal behavior, and safe source-aware diagnostics.

### Modified Capabilities

- `verification-outcome-semantics`: Adds a distinct activity layer and its
  contribution to conservative aggregate outcomes.
- `cli-flag-authoring`: Aligns protected sealed-input requirements and
  actionable field diagnostics with public forecast authoring semantics.

## Impact

The change affects the authoritative upstream Forecast Ledger contract, the
embedded schema and attribution, typed ledger and crypto models, semantic
validation, canonical target construction, RFC 3161 evidence handling,
publication manifests, shared verification services, CLI/MCP presentation,
protected JSON/YAML parsing, generated contracts, fixtures, documentation, and
release checks. The immutable forecast target remains lifecycle-free; the new
lifecycle binding is a separate evidence scope. Existing contract-version
files are rejected before side effects under the repository's single-contract
policy.
