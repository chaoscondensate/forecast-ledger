## Why

Forecast Ledger `v2.0.0` is now the published contract, and it replaces the
flat v1.3 question/value model with versioned questions, exact distribution
representations, richer resolution states, and revision-bound cryptography.
The CLI cannot adopt the new schema by changing embedded bytes alone: its typed
model, semantic checks, authoring surfaces, targets, seals, fixtures, output,
and documentation all encode v1 assumptions that would otherwise accept,
author, or verify the wrong meaning.

## What Changes

- **BREAKING** Replace exclusive Forecast Ledger `v1.3.0` admission with the
  exact published `v2.0.0` contract at commit
  `1d3b186a15136bc5aff38647cb59fbef475dbe55`, schema SHA-256
  `efd87b7432f7cb017fbedaba217e4cd1d3bff06133457927fb786a249a040b21`,
  and its matching tag, archive, checksum asset, attribution, examples,
  conformance cases, reference semantics, and cryptographic vectors.
- **BREAKING** Replace mutable question fields with ordered immutable revisions
  and bind each forecast and resolved outcome to one exact revision. Add the
  binary, categorical, ordinal, numeric, date, and datetime outcome/domain
  forms, versioned option and bin sets, bounds, discreteness, scale, and units.
- **BREAKING** Replace basis-point, type-specific forecast values with exact
  canonical decimal strings and non-empty representation arrays supporting
  probability, PMF, binned PMF, quantiles, CDF, point, and credible intervals.
- Add groups, conditional relationships, versioned platform provenance,
  retained snapshot digests, and append-only forecast lifecycle events with
  complete local semantic validation.
- **BREAKING** Replace `annulled` with the v2 terminal states `ambiguous`,
  `void`, and `not_applicable`; bind resolved outcomes to a revision and its
  domain while preserving `disputed`.
- **BREAKING** Implement `forecast-seal/v2` and `forecast-envelope/v2`, binding
  the full question revision and exact representation bundle.
- Replace obsolete CLI flags and MCP properties with complete direct v2
  authoring surfaces, including revision, domain, representation,
  relationship, provenance, lifecycle, and resolution operations. Secrets
  remain restricted to purpose-named protected files or stdin.
- Regenerate examples and machine-readable contracts and update all public
  documentation, security, packaging, attribution, version, and release
  material to describe the v2-only runtime.
- Record that v2 has no independent security or cryptographic audit. An
  external review remains desirable follow-up work, but it is not a release
  gate for the `0.9.0` line while the public no-audit warning and bounded
  evidence claims remain in place.

## Capabilities

### New Capabilities

- `forecast-ledger-v2-contract`: Exact immutable v2 source identity, exclusive
  admission, schema and reference-semantic validation, conformance corpus, and
  public metadata consistency.
- `versioned-question-domains`: Immutable question revisions and the complete
  v2 outcome-space, domain, option-set, bin-set, and resolution rules.
- `forecast-representations`: Exact probability encoding, representation/domain
  compatibility, distribution invariants, and direct public or protected
  authoring.
- `ledger-relationships-provenance`: Groups, conditional relationships,
  imported object provenance, retained snapshots, and acyclic/reference rules.
- `forecast-lifecycle-events`: Append-only withdraw, expire, and reaffirm event
  semantics and their direct adapter surfaces.
- `forecast-ledger-v2-cryptography`: Revision-bound v2 seal, reveal, target,
  RFC 3161, and deterministic vector behavior.

### Modified Capabilities

- `forecast-ledger-v1-3-contract`: Retire v1.3 runtime admission and public
  identity without retaining a compatibility or migration surface.
- `cli-flag-authoring`: Replace v1 type/value and annul authoring with complete,
  unambiguous direct v2 fields and dedicated subcommands without JSON/YAML
  argument wrappers.
- `empty-ledger-workflows`: Create and operate on valid v2 empty ledgers,
  revision-bearing backlog questions, representation-bearing forecasts, and
  the new terminal resolution states.

## Impact

The change touches the embedded schema and third-party notices; ledger models
and cloning; schema, format, semantic, and reference-parity validation;
application request types and services; JSON/YAML source-preserving patches;
CLI flags, command tree, help, completions, and examples; MCP tool schemas,
dispatch, resources, and generated references; canonical target construction;
seal/reveal and protected input; timestamp and publication workflows; stable
presentation and result schemas; build/version metadata; release checks; and
all conformance, negative, property, fuzz, golden, parity, and documentation
tests. The active command-surface, timestamp-failover, and documentation
changes must be reconciled so they no longer reintroduce v1 field shapes or
claims.
> Superseded for current implementation by
> `reset-cryptographic-profiles-and-evidence-store`. This artifact remains only
> as historical rationale for the pre-v2.2 surface.
