## Context

See `proposal.md` for motivation. The current runtime embeds one exact v2.1.0
contract. Its v2 seal name covers two incompatible historical plaintext shapes,
target build can retain files without a ledger reference, activity verification
returns early when checkpoints are absent, and publication deliberately follows
ledger references without reconciling adjacent files.

The repository requires upstream-first contract changes, exact immutable pins,
shared services for CLI and MCP, no network during local verification, bounded
parsing, confined portable paths, recoverable multi-resource writes, and no
legacy bundles in the current runtime. The next contract can break because the
operator explicitly rejects compatibility and migration work.

## Goals / Non-Goals

**Goals:**

- make every canonical cryptographic profile name identify one byte contract;
- make every CLI-retained target a declared ledger state and indexed local
  artifact;
- detect deletion of ledger lifecycle declarations while managed evidence
  remains;
- keep verification and publication bounded, offline, transport-neutral, and
  shared by CLI and MCP;
- expose public commitment bytes without weakening the private-data boundary.

**Non-Goals:**

- reading, converting, resealing, or repackaging v2.1.0 material;
- proving an event existed after the ledger, index, managed store, packages, and
  every external observation have all been deleted;
- treating an unsigned lifecycle target as proof of authorship, truth, or an
  honest time;
- scanning outside the closed managed `proofs/` and `trust/` namespaces;
- fixing unrelated build-version, TSA-classification, or generic message issues.

## Decisions

### 1. Publish v2.2.0 upstream before changing the CLI

The prepared upstream contract assigns new identities to every profile whose
bytes or interpretation change: `forecast-seal/v3`, `forecast-key/v3`,
`forecast-envelope/v3`, and `forecast-lifecycle/v2`. It adds the retained target
integrity variants and exact conformance/vector corpus. The contract was
published as the immutable v2.2.0 release and independently reproduced and
verified before CLI import:

| Release identity | Exact value |
| --- | --- |
| Commit | `ae02de9ebca3eb2ae87c480596bf620bdcdced11` |
| Annotated tag object | `8feb323ce895a6a197183c4eb857a55463648873` |
| Release archive SHA-256 | `e8f92450e7e73eb559e762878dd4188156968cdad6328c03ea94d6b0c01ff419` |
| `SHA256SUMS` asset SHA-256 | `7e1d79e6d8bd4df20a5877ef51c17f149cec7619d49ddfa2fff82899034989a0` |
| Ledger schema SHA-256 | `6a048928f2573519fd25988ba23f0006b6f8d84eb2f7179d7ff8edd7affa4f9e` |

The CLI imports the resulting schema, code-independent semantics, fixtures,
vectors, attribution, and these exact identities as one unit.

Keeping `forecast-seal/v2` with better release notes was rejected because the
identifier would remain ambiguous to independent implementations. A local-only
v3 fork was rejected because the runtime would no longer implement the
authoritative contract. Dual v2/v3 dispatch was rejected because it is
compatibility code and would retain superseded parsers and vectors.

### 2. Target build creates declared retained integrity

The v2.2.0 ledger model gains a closed retained target state for forecast
integrity and lifecycle checkpoints. It contains exact target metadata but no
timestamp entry. `target build` moves an unanchored forecast, or a lifecycle
head without a checkpoint, into this state while writing target bytes and the
index. Creating a lifecycle retained target requires explicit checkpoint ID and
recording-time inputs; neither value is derived from the head, wall clock,
filename, random data, or hidden state. `timestamp stamp` consumes the same
declaration and advances it to pending or verified without changing target
identity.

This replaces the current distinction between an adjacent standalone target and
a ledger-retained target. Retaining standalone files was rejected because it is
the source of local-versus-package disagreement and makes safe ownership
discovery impossible without heuristics.

### 3. Treat the evidence namespaces as one closed indexed store

`proofs/evidence-index.json` uses canonical RFC 8785 JSON and profile
`forecast-evidence-index/v1`. Its root binds the ledger ID and exact contract
identity. Entries are strictly sorted by portable relative path and always
contain a closed role, path, size, and SHA-256 digest. Role-specific closed
fields then bind forecast targets to question and forecast, lifecycle targets
also to checkpoint and head, requests to their target and message imprint,
responses to their target, request, optional trust bundle, and TSA URL, and CA
bundles to their PEM format without an artificial question or forecast owner.
Target, request, response, and trust bytes are indexed; the index does not index
itself.

The managed namespaces are closed. A directory walk is allowed only below the
reviewed `proofs/` and `trust/` roots, with fixed depth, count, size, total-byte,
path, regular-file, and symlink limits. Any managed file not listed in the
index is an incomplete store, not automatically evidence and not a
cryptographic mismatch. This gives planted or stale files a conservative result
without parsing arbitrary neighbors as trusted data.

A sidecar SQLite database, opaque journal-only catalog, and filename-only scan
were rejected. They would respectively add an unnecessary dependency, disappear
after normal journal cleanup, or fail to cover custom safe paths and file
replacement.

### 4. Reconcile declarations, index entries, and bytes in one service

One transport-neutral reconciler builds three bounded maps: ledger declarations,
index entries, and actual managed files. It first validates index syntax and
path safety, then checks file identity/size/digest, then reconstructs target and
RFC 3161 bindings from declarations. It also evaluates unmatched index entries.

For an unmatched lifecycle target, the reconciler parses the bounded canonical
target and validates its question, forecast, envelope digest, prefix, and head.
If those checks establish that retained evidence describes history missing from
the ledger, activity fails with
`activity.retained_evidence_unreferenced`. If bytes are absent or cannot be
fully evaluated, the result is incomplete. The reconciler never contacts a TSA
or uses the system trust store.

CLI verify, MCP verification, evidence mutation preconditions, publication
build, and publication verify call the same reconciler. Schema and semantic
validation of document bytes remains independently reportable; evidence-store
reconciliation is a local evidence check, not a remote schema lookup.

### 5. Extend the existing recoverable resource transaction

Target and timestamp mutations plan ledger, index, target, request, response,
and trust effects under the ledger writer lock. They validate the current
ledger and store, build and validate prospective bytes, create and flush
temporary sibling resources, record exact before/after identities in the
journal, and replace resources in a recoverable order. Success is emitted only
after ledger, index, and artifacts agree.

The implementation reuses the existing resource-plan and journal machinery;
it does not introduce a second transaction framework. Recovery validates both
ledger and index identity and preserves all files when state is ambiguous.

### 6. Publication v3 carries the evidence index

Publication build first requires a fully reconciled local store. It then copies
the byte-exact ledger, canonical evidence index, and every indexed artifact into
a closed `forecast-ledger-publication/v3` package and lists each in the
manifest. Every package contains exactly one index. When the source ledger has
no retained evidence and therefore no local index, package build creates the
canonical empty index inside the package without creating one beside the source
ledger. Because target build now creates declarations, there is no supported
class of adjacent standalone target to omit.

Package verification checks manifest closure and digests, then index closure
and bindings, then the ordinary evidence layers. It rejects extra package files
and performs no network operation. Reusing publication v2 was rejected because
adding an evidence-index role and stronger closure changes the manifest
contract.

### 7. Separate public commitment data from private seal material

The shared forecast view includes the stored public nonce and ciphertext for
sealed and revealed forecasts. Redaction continues to remove raw keys, salts,
plaintext, private fields, credentials, and protected paths. CLI and MCP render
the same view; adapters do not independently blank fields.

The crypto layer returns typed internal failure stages. The service maps an
AEAD failure to `reveal.authentication_failed` and a post-AEAD, post-digest
closed-profile mismatch to `reveal.bundle_profile_mismatch`. Neither mapping
includes the underlying plaintext or unrestricted parser error.

### 8. Remove superseded runtime material in the cutover

The implementation replaces, rather than accumulates, schema bytes, profile
constants, vectors, fixtures, generated contracts, examples, and documentation.
Release checks deny v2.1.0 profile identifiers outside explicit historical
release notes and verify that no compatibility command or parser is linked into
the binary.

## Risks / Trade-offs

- **[The upstream v2.2.0 release identities could drift during import]** → Pin
  the independently verified exact commit, annotated tag object, archive,
  checksum asset, schema digest, fixtures, and vectors together; never use a
  placeholder or floating identity.
- **[Target build now mutates the ledger]** → Make the effect explicit in help,
  MCP annotations, dry-run results, locking tests, and documentation; commit the
  target, index, and ledger through one recoverable transaction.
- **[An unexpected file can make the managed store incomplete]** → Limit this
  rule to closed managed namespaces, report a safe relative path and stable
  reason, and never call the condition a cryptographic mismatch.
- **[Index loss can make otherwise present bytes unusable]** → Treat a non-empty
  managed store without its index as incomplete, preserve all bytes, and give
  repair guidance without guessing ownership.
- **[The index adds another mutable resource]** → Canonicalize it, digest every
  entry, bind it to the ledger and contract, include it in journals and
  packages, and exercise crash recovery at every replacement boundary.
- **[V3 output breaks every existing consumer]** → Make the cutover explicit,
  provide exact v0.10.0 historical-use guidance, and provide no misleading
  best-effort compatibility path.
- **[A profile-stage error could become a plaintext oracle]** → Expose only the
  stable stage reason after AEAD and digest success; keep all field identity,
  parser detail, and plaintext out of public results and logs.

## Migration Plan

1. Publish and independently verify the complete Forecast Ledger v2.2.0
   upstream contract and all exact release assets.
2. Import the release in one CLI change and update every provenance and profile
   pin together.
3. Implement retained target states, the evidence index, shared reconciliation,
   recoverable multi-resource writes, publication v3, and public inspection.
4. Regenerate contracts and documentation, run deterministic conformance,
   crypto, parser, recovery, adapter, package, and native-filesystem gates, and
   cut the next breaking pre-1.0 CLI release only when every identity agrees.
5. Keep v0.10.0 and its source available for historical v2.1.0 inspection. Do
   not convert or rewrite existing user files.

Rollback means using v0.10.0 with untouched v2.1.0 material. A v2.2.0 ledger or
evidence store MUST NOT be downgraded or partially rewritten for v0.10.0.
