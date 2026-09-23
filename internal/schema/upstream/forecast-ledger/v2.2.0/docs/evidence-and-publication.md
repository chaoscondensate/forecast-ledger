# Evidence index and publication packages

This document is normative for `forecast-evidence-index/v1` and
`forecast-ledger-publication/v3`. The executable reference is
[`tools/sidecar_contracts.py`](../tools/sidecar_contracts.py).

## Evidence index

Managed evidence is described by the closed sidecar at exactly
`proofs/evidence-index.json`. The file is exact RFC 8785 JSON with no trailing
LF or other bytes. It binds `ledger_id` and the exact ledger contract identity:
the permanent schema ID, `2.2.0`, and SHA-256 of the released ledger schema.

Entries are unique and strictly sorted by portable relative path. The closed
roles are:

- `forecast_target`, bound to question, forecast, and `forecast-envelope/v3`;
- `lifecycle_target`, additionally bound to checkpoint and head event;
- `rfc3161_request`, referencing its target and message imprint;
- `rfc3161_response`, referencing its target, request, and optional retained
  trust bundle;
- `x509_ca_bundle`, containing no artificial question or forecast owner.

Every entry records role, package-portable path, byte size, and SHA-256. Target,
request, response, and trust references are paths to other exact entries. A
single CA bundle may therefore be referenced by any number of responses.

The index never indexes itself. `proofs/evidence-index.json` is the sole explicit
exception to the rule that every regular file below managed `proofs/` and
`trust/` namespaces needs an index entry. Absolute paths, backslashes, `..`,
symlinks, non-regular files, case/Unicode collisions, duplicate paths, and paths
outside those namespaces are invalid.

A local ledger with no retained evidence has no index and need not have a
`proofs/` directory. A present local index must not be empty.

## Reconciliation

Verification reconciles three sources independently: ledger declarations,
index entries, and retained bytes.

- declaration + index + exact bytes agree: the evidence may be evaluated;
- indexed bytes missing or unreadable: `incomplete` / `not_checked`;
- size, digest, canonical target, role, or binding mismatch: `fail`;
- managed file without an entry: `evidence.unindexed_artifact` and incomplete;
- valid indexed lifecycle target without its event and checkpoint declaration:
  `activity.retained_evidence_unreferenced` and fail;
- no declaration, index entry, or managed evidence: activity may be unbound.

Reconciliation is bounded and offline. It never contacts a TSA, resolves a
remote schema, or falls back to the operating-system trust store. Retained trust
bytes named by the response are the only trust input.

Deleting every event, checkpoint, index entry, managed artifact, package, and
external copy also deletes the evidence that the event once existed. The
contract makes no historical-completeness claim in that state.

## Publication v3

A package manifest uses `forecast-ledger-publication/v3` and exact RFC 8785 JSON
with no trailing data. Every manifest path is relative to the package root. The
manifest contains exactly one ledger, exactly one evidence index at
`proofs/evidence-index.json`, and exactly the files named by that index. Manifest
evidence roles must equal index roles.

The evidence index is mandatory even for a package with no evidence. In that
case the package builder deterministically creates an index with `entries: []`
inside the package without creating a local index beside the source ledger.

Package build is forbidden when evidence is detached, unreferenced, missing,
unsafe, mismatched, or unindexed. Verification rejects extra files, omitted
files, role changes, rebound evidence, changed bytes, noncanonical sidecars, and
`forecast-ledger-publication/v2`. It performs no network request and never
includes key files, raw keys, salts, plaintext, credentials, protected paths,
journals, locks, or temporary files.
