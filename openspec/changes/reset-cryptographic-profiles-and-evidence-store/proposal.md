## Why

Forecast Ledger v2.1.0 intentionally changed sealed plaintext bytes while
retaining `forecast-seal/v2`, so the profile name no longer identifies one
canonical byte contract. Lifecycle verification also trusts only declarations
inside the ledger, which lets manually removed events and checkpoints disappear
from reports and packages even while CLI-created proof artifacts remain beside
the ledger.

The project is still pre-adoption and has explicitly chosen exact-contract
cutovers over compatibility. This is the right point to give every changed byte
profile a new identity and make retained local evidence a closed, reconciled
store instead of preserving ambiguous names or adding a compatibility reader.

## What Changes

- **BREAKING** Publish and adopt Forecast Ledger v2.2.0 as one exact immutable
  upstream contract, with new `forecast-seal/v3`, `forecast-key/v3`,
  `forecast-envelope/v3`, and `forecast-lifecycle/v2` identifiers and new exact
  vectors for every canonical byte sequence affected by the cutover.
- **BREAKING** Accept only v2.2.0. Reject v2.1.0 before locks, protected input,
  entropy, artifact access, writes, or network activity. Do not ship a
  converter, dual parser, compatibility bundle, historical vector set, or
  fallback reveal path.
- **BREAKING** Replace standalone undeclared target files with declared retained
  target state. `target build` atomically records the target in the ledger and a
  canonical local evidence index; timestamping advances that declared state
  instead of discovering an adjacent file. Lifecycle target creation requires
  an explicitly authored checkpoint ID and recording time.
- Introduce a closed `forecast-evidence-index/v1` at
  `proofs/evidence-index.json`. It inventories every managed target, RFC 3161
  request and response, and retained trust file by role, portable path, size,
  SHA-256 digest, and the role-specific binding, reference, or format fields
  defined by the upstream contract.
- Reconcile the ledger, index, and managed evidence bytes during verification,
  evidence mutations, and package build. Report missing indexed bytes as not
  checked, mismatches as failures, and valid indexed lifecycle evidence whose
  declaration was removed as `activity.retained_evidence_unreferenced`.
- **BREAKING** Upgrade publication packages to
  `forecast-ledger-publication/v3`, always include exactly one evidence index,
  synthesize the canonical empty index inside an evidence-free package without
  creating a local empty index, reject unindexed or unreferenced managed
  evidence, and never create a package that silently omits a retained lifecycle
  proof.
- Return the complete public sealed commitment from CLI and MCP forecast
  inspection, including nonce and ciphertext, while continuing to redact keys,
  salts, plaintext, protected paths, credentials, and private forecast fields.
- Distinguish AEAD authentication failure from an authenticated plaintext that
  does not match the selected closed seal profile, using a stable secret-safe
  reason without exposing decrypted bytes.
- Update generated contracts, release metadata, security guidance, examples,
  and compatibility statements in the same cutover. Remove superseded profile
  constants, fixtures, parsing branches, and documentation from the runtime.

## Capabilities

### New Capabilities

- `forecast-ledger-v2-contract`: Defines the exclusive v2.2.0 contract import,
  unambiguous v3/v2 cryptographic profile identities, exact vectors, and the
  no-compatibility cutover.
- `managed-evidence-store`: Defines declared target state, the closed local
  evidence index, atomic artifact/index/ledger updates, reconciliation, and
  publication-package v3 behavior.
- `public-forecast-inspection`: Defines the complete public commitment view and
  secret-safe reveal failure reasons across CLI and MCP.

### Modified Capabilities

- `verification-outcome-semantics`: Adds index reconciliation outcomes and
  prevents aggregate pass when retained evidence contradicts or is no longer
  referenced by the ledger.

## Impact

The change affects the authoritative upstream schema repository and its release
assets; embedded contract bytes and provenance pins; ledger integrity variants;
seal, key, envelope, and lifecycle target builders; protected key files;
artifact paths and journaling; target and timestamp commands; layered and
package verification; publication manifest roles; CLI/MCP result contracts;
generated schemas; deterministic, negative, property, fuzz, and cross-language
vectors; public documentation; and release checks.

Existing v2.1.0 ledgers, keys, targets, timestamps, and publication packages are
not accepted by the new runtime. They remain interpretable only with the exact
v0.10.0 release and its pinned v2.1.0 source. Build-version fallback, TSA error
wording, generic empty-result messages, and historical changelog repair remain
outside this change because they do not determine cryptographic or retained
evidence meaning.
