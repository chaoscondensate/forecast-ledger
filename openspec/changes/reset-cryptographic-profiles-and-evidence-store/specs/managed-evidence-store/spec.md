## Purpose

Defines a closed, indexed local evidence store whose ledger declarations, files,
and publication packages can be reconciled without trusting directory proximity.

## ADDED Requirements

### Requirement: Every retained target is declared
Building a forecast or lifecycle target SHALL atomically add a retained target
declaration to the selected forecast or lifecycle checkpoint. The v2.2.0
contract SHALL provide a closed retained integrity state for a target that has
no RFC 3161 attempt yet. Timestamp stamping SHALL advance that same declaration
to pending or verified evidence; it MUST NOT discover an undeclared neighboring
target and silently adopt it.

Lifecycle target creation SHALL require the caller to provide the checkpoint ID
and checkpoint `recorded_at` directly. CLI SHALL expose them as ordinary
leaf-local `--checkpoint` and `--recorded-at` flags, and MCP SHALL expose closed
`checkpoint` and `recorded_at` request properties. The runtime MUST NOT derive
either value from the selected head, operation clock, filename, random data, or
hidden state.

The runtime MUST NOT create a successful standalone target file that is absent
from both the ledger and evidence index.

#### Scenario: Forecast target is built before stamping
- **WHEN** `target build` succeeds for an unanchored forecast
- **THEN** the target bytes, retained target declaration, and evidence-index entry are committed together and the forecast is no longer unanchored

#### Scenario: Lifecycle target is built before stamping
- **WHEN** `target build --scope lifecycle --head <event> --checkpoint <id> --recorded-at <timestamp>` succeeds
- **THEN** a retained checkpoint with those exact authored fields and head and its target/index entries are committed together without contacting a TSA

#### Scenario: Target transaction is interrupted
- **WHEN** interruption occurs before the target, index, and ledger replacement are durably complete
- **THEN** recovery either completes the one validated transaction or preserves all material for explicit recovery without reporting a successful retained target

### Requirement: Managed evidence has one canonical index
When managed evidence exists, the ledger directory SHALL contain the canonical
RFC 8785 JSON file `proofs/evidence-index.json` with profile
`forecast-evidence-index/v1`. The index SHALL bind the ledger ID and v2.2.0
contract identity and SHALL contain one sorted closed entry for every managed
target, RFC 3161 request, RFC 3161 response, and retained CA bundle.

Each entry SHALL contain its closed role, normalized portable relative path,
byte length, and SHA-256 digest. Forecast-target entries SHALL bind question,
forecast, and `forecast-envelope/v3` scope. Lifecycle-target entries SHALL also
bind checkpoint, head, and `forecast-lifecycle/v2` scope. Request entries SHALL
reference their target and bind the SHA-256 message imprint. Response entries
SHALL reference their target, request, and optional retained trust bundle and
bind the TSA URL. CA-bundle entries SHALL declare PEM certificate-bundle format
and MUST NOT invent a question or forecast owner. The index itself SHALL not
list itself. Paths MUST remain confined, symlink-free, collision-free under
supported filesystem comparison rules, and within the named managed `proofs/`
or `trust/` namespaces.

#### Scenario: First evidence is retained
- **WHEN** the first target is successfully built for a ledger
- **THEN** the runtime creates the canonical index and one exact entry for the target in the same recoverable transaction

#### Scenario: Shared trust bytes are reused
- **WHEN** several declared timestamp entries use the same retained CA bundle path and bytes
- **THEN** the index contains one trust entry and every referencing evidence branch resolves to that same digest

#### Scenario: No evidence exists
- **WHEN** a ledger has no retained target or timestamp declaration and no managed evidence bytes
- **THEN** no empty `proofs` directory or evidence index is required or created

### Requirement: Reconciliation is complete and bounded
Layered verification, target and timestamp evidence mutations, and publication
build SHALL reconcile ledger declarations, the evidence index, and managed
files before reporting success or committing new evidence. Reconciliation SHALL
use bounded entry counts, file sizes, path lengths, total bytes, and directory
walk depth; SHALL reject symlinks and non-regular managed entries; and SHALL make
no network request.

Every ledger declaration MUST have one matching index entry and byte-exact
artifact. Every index entry MUST resolve to its declared binding. Missing or
unreadable indexed bytes SHALL be `not_checked` with an incomplete-evidence
reason. Digest, binding, canonical-byte, or role mismatches SHALL fail. A file in
a managed namespace that is absent from the index SHALL be
`evidence.unindexed_artifact`, make the aggregate incomplete, and block package
creation without being called a cryptographic mismatch.

#### Scenario: Indexed artifact is missing
- **WHEN** a declaration and index entry agree but the indexed file is unavailable
- **THEN** verification reports `not_checked` with a stable missing-artifact reason and does not claim tampering or pass

#### Scenario: Indexed bytes differ
- **WHEN** an indexed managed file has a different size, digest, canonical target, or binding than its declaration and index entry
- **THEN** verification fails with a specific evidence mismatch reason and package build creates no destination

#### Scenario: Unindexed neighbor is planted
- **WHEN** a regular file appears inside a managed evidence namespace without an index entry
- **THEN** verification reports `evidence.unindexed_artifact` as incomplete and package build refuses the unreconciled store

### Requirement: Retained lifecycle evidence cannot be silently detached
If an indexed lifecycle target remains after its lifecycle event or checkpoint
declaration is removed, reconciliation SHALL parse and validate the bounded
target against its recorded question ID, forecast ID, v3 forecast-envelope
digest, lifecycle prefix, and head. A valid retained target that no longer has a
matching ledger declaration SHALL fail the activity layer with
`activity.retained_evidence_unreferenced` and prevent aggregate pass. Missing,
malformed, or unverifiable bytes SHALL remain incomplete unless a concrete
cryptographic or digest mismatch is established.

#### Scenario: Covered withdrawal and checkpoint are deleted
- **WHEN** a valid indexed lifecycle target for a withdrawn forecast remains but the event and checkpoint are removed from the ledger
- **THEN** local verification fails with `activity.retained_evidence_unreferenced`, preserves the independent forecast-content result, and does not report the forecast as cleanly unbound

#### Scenario: Event, declaration, index, and every copy are removed
- **WHEN** no ledger declaration, index entry, managed artifact, package, or external copy of an earlier lifecycle event remains
- **THEN** verification may report current activity as unbound while stating that historical completeness cannot be established

### Requirement: Evidence packages use the reconciled v3 profile
Publication build SHALL emit only `forecast-ledger-publication/v3` packages.
Before creating output it SHALL complete local reconciliation and SHALL refuse
unreferenced, unindexed, missing, unsafe, or mismatched managed evidence. A
package SHALL contain exactly one canonical evidence index at
`proofs/evidence-index.json` and every file named by it; the manifest SHALL list
the index and each evidence file by a closed role, path, size, and SHA-256
digest. If the source ledger has no retained evidence and no local index, the
builder SHALL create the canonical `entries: []` index inside the package
without creating an empty local index or `proofs` directory beside the source
ledger.

Package verification SHALL verify manifest closure, reject extra files, verify
the evidence index against the packaged ledger and listed bytes, and then run
the same content, activity, timestamp, reveal, chronology, and outcome checks
without network access.

#### Scenario: Unreferenced lifecycle proof blocks publication
- **WHEN** local reconciliation finds a valid indexed lifecycle target whose ledger declaration was removed
- **THEN** `publish build` fails with verification category and creates no partial package

#### Scenario: Complete evidence package is verified offline
- **WHEN** a v3 package contains one reconciled ledger, evidence index, manifest, and all indexed evidence bytes
- **THEN** package verification checks every layer locally and rejects any omitted, added, replaced, or rebound entry

#### Scenario: Evidence-free package is built
- **WHEN** publication build selects a valid ledger with no retained evidence, managed bytes, or local evidence index
- **THEN** the package contains one canonical empty evidence index and a manifest that lists the ledger and index, while the source ledger directory remains unchanged

#### Scenario: V2 package is presented
- **WHEN** the new runtime receives a `forecast-ledger-publication/v2` manifest
- **THEN** it rejects the unsupported package profile without attempting compatibility verification
