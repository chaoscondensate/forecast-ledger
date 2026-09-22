## Context

See `proposal.md` for motivation and the delta specs for required behavior. The
current runtime embeds the exact Forecast Ledger v2.0.1 contract. Its semantic
validator compares lifecycle events only with each other, its canonical
forecast target deliberately excludes lifecycle events, and its ledger model
has only one integrity state for the recorded-belief target. Layered
verification consequently has no activity layer or lifecycle evidence to
check.

The current private plaintext is an exact seven-field Go value. Its schema,
input contracts, reveal schema, reference implementation, and seal vector all
assume that rationale, key factors, and comment are present. A local-only
change would produce ciphertext and revealed ledgers that disagree with the
authoritative contract, so the schema correction must precede the CLI import.

The repository requires one exact contract, retained deterministic evidence,
shared application services for CLI and MCP, no network during local
validation, and atomic source-preserving mutations. Those constraints rule out
a compatibility parser, a locally invented schema profile, and automatic
network stamping during a lifecycle mutation.

## Goals / Non-Goals

**Goals:**

- Make lifecycle chronology invalid when it predates the forecast it modifies.
- Give each retained activity checkpoint an immutable, independently
  timestampable target without changing `forecast-envelope/v2`.
- Let verification distinguish current activity, unbound history, partial
  coverage, complete retained coverage, missing evidence, and a proven
  mismatch.
- Preserve optional private-field presence byte-for-byte through protected
  input, sealing, authentication, reveal, targets, and presentation.
- Produce useful missing-field diagnostics without leaking private text.

**Non-Goals:**

- Proving that an event never existed after the ledger, all activity
  declarations, all retained targets, and every external copy have been
  deleted.
- Putting lifecycle events back into `forecast-envelope/v2` or invalidating
  retained forecast evidence when activity changes.
- Automatically contacting a TSA from withdraw, expire, or reaffirm.
- Reading the superseded contract, decrypting old ciphertext under new bundle
  rules, converting old ledgers, or retaining dual vectors.
- Treating a valid activity transition as evidence of authorship, truth,
  completeness, or an honest timestamp-authority clock.

## Decisions

### 1. Publish upstream first and import one exact correction

The upstream Forecast Ledger contract will change the lifecycle semantic
validator, ledger schema, private bundle definition, reference crypto and
target builders, conformance fixtures, invalid cases, seal vectors, and
documentation in one release. Only after that immutable release exists will
the CLI replace its embedded contract and all provenance pins together.

The implementation records the actual commit, annotated tag object, release
archive digest, `SHA256SUMS` asset digest, schema digest, version, attribution,
and vector bytes from the published release. Placeholder hashes or a floating
tag are not acceptable. A local schema patch is rejected because it would make
the binary claim upstream conformance while using different seal and lifecycle
semantics.

### 2. Store append-only activity checkpoints outside the event stream

The corrected schema will add an optional forecast-level append-only activity
evidence collection. Each checkpoint identifies the lifecycle head event and
contains integrity metadata for one canonical activity target. A checkpoint
covers the complete event prefix ending at that head. Later events append new
checkpoints; they do not replace or reinterpret earlier ones.

The `forecast-lifecycle/v1` target will contain a closed object with:

- the scope identifier;
- question and forecast identity;
- the SHA-256 digest of the corresponding `forecast-envelope/v2` bytes;
- the ordered lifecycle prefix through the selected head; and
- the selected head event ID.

Integrity metadata, activity checkpoints, and forecast integrity are excluded
from the activity target itself. Canonical bytes use the existing bounded RFC
8785 implementation. CLI-authored target paths deterministically include both
forecast and head identity so a new checkpoint cannot overwrite an earlier
prefix. Conforming ledgers may declare another confined portable path; target
identity comes from the canonical digest rather than one local path convention.
Publication manifests include every declared activity target and its timestamp
resources.

A checkpoint list was chosen over one mutable `activity_integrity` value
because replacing the current head would discard the ledger's reference to
earlier activity evidence. Per-event ciphertext or including events in the
forecast envelope was rejected because activity is public mutable state, not
part of the original recorded belief.

### 3. Reuse explicit target and timestamp workflows with a scope selector

Target build/check and timestamp stamp/status/verify gain a closed
`forecast|lifecycle` scope selector in CLI and MCP. The default remains
`forecast`; lifecycle scope requires a question, forecast, and a current event
head. The shared service selects the target builder and integrity destination
before any artifact or network effect. Lifecycle stamping appends or updates a
checkpoint for that exact head through the existing journaled resource plan.

Lifecycle mutations themselves remain local and append only the event. This
keeps withdraw/expire/reaffirm usable offline and avoids hiding a network side
effect inside an ordinary ledger mutation. Users who need tamper evidence run
the explicit lifecycle target/timestamp workflow, and verification reports any
events after the newest checkpoint as uncovered.

Dedicated duplicate command trees were rejected because the evidence
operations and safety machinery are otherwise identical and a closed selector
keeps CLI/MCP parity auditable.

### 4. Validate activity checkpoints as immutable prefixes

Semantic validation first enforces event chronology and transition rules, then
checks that checkpoint heads exist, are strictly ordered without duplicates,
and reference successively longer prefixes. Local verification rebuilds each
declared target from the corresponding prefix and compares canonical bytes,
digest, request binding, RFC 3161 response, and retained trust bytes.

The activity result derives current state from the full validated event stream
and reports evidence coverage separately:

- `unbound` when no checkpoint exists;
- `partial` when checkpoints cover only an earlier prefix;
- `pending` or `not_checked` when declared evidence cannot be completed;
- `verified` when verified evidence covers the current head; and
- `failed` only after complete retained observations establish a mismatch.

Deleting a covered event while leaving its checkpoint makes the head or prefix
invalid. Deleting or editing a covered prefix while complete retained bytes
remain makes target comparison fail. If the attacker removes every event,
checkpoint, artifact, package, and external copy, the verifier has no
observation from which to prove deletion; it reports unbound current state and
the existing completeness limitation instead of manufacturing a failure.

### 5. Compare lifecycle time against forecast time in one semantic path

The authoritative semantic validator will compare each event's effective time
with `forecasted_at` and its recording time with both `effective_at` and the
forecast's `recorded_at`. Existing within-stream order and transition checks
remain. The CLI service may preflight the same rule to return a direct field
error, but prospective full-document validation remains the final common gate
for CLI and MCP.

The forecast lower bounds are sufficient because an already valid forecast
cannot predate its bound revision. Repeating independent question-time
comparisons in adapter code was rejected because it would create redundant
rules that can drift.

### 6. Model private optional fields by presence, not zero values

The protected input and authenticated bundle use presence-aware optional
values for rationale, key factors, and comment. Canonical encoding omits an
absent property, preserves a present empty string or empty collection, and
rejects unknown properties. Representations remain a required non-empty
collection. The reference implementation and Go implementation will publish
vectors for the minimal bundle and representative presence combinations.

Reveal constructs patches only for properties present in the authenticated
bundle. The corrected revealed-forecast schema requires representations and a
revealed commitment while leaving the three text properties optional. Filling
omissions with empty values was rejected because it destroys author intent and
changes committed plaintext semantics.

### 7. Derive safe field diagnostics from validator metadata and source maps

The protected parser retains the existing bounded JSON/YAML document and node
source map. For a `required` failure, the schema adapter extracts the missing
property from structured validator metadata, appends an escaped segment to the
instance pointer, and returns no fabricated scalar line because the node does
not exist. For an invalid present value, the adapter resolves the exact node
span. Syntax errors keep their actual bounded line and column.

The public error contains the stable category, keyword, pointer, and safe
location metadata only. It never includes the rejected value, surrounding
source, raw validator cause, or unrestricted path. Both adapters consume the
same application error. Parsing a second simplified secret structure was
rejected because it would lose source locations and create a second schema.

### 8. Add activity without letting an observation create evidence pass

Layered verification adds one activity result per forecast after document
validation. Its public evidence contains active state, event count, the last
event's ID/type/times, coverage state, covered head, and safe target/timestamp
metadata. Forecast content, timing, reveal, and outcome layers remain
unchanged.

An unbound activity observation is visible but does not count as applicable
evidence for the aggregate. Complete lifecycle evidence participates in the
existing precedence: a proven mismatch fails, unavailable declared evidence
is incomplete, pending evidence is pending, and verified coverage can pass.
This prevents an ordinary valid ledger from receiving `pass` merely because an
active boolean was computed.

### 9. Test the findings at contract, service, adapter, and release boundaries

Upstream invalid cases cover lifecycle times before the forecast and its
recording time. Cross-language vectors cover lifecycle prefixes and minimal
sealed plaintext. CLI service tests exercise deletion, prefix edits, missing
artifacts, partial coverage, optional-field combinations, and diagnostics for
JSON and YAML. Adapter parity tests cover CLI, MCP, dry-run, human/plain/JSON,
and side-effect-free failures. RFC 3161 tests use retained deterministic
OpenSSL fixtures; no live provider gates conformance or release.

Documentation updates cover the data model, security boundary, verification
claims, protected input, command/MCP reference, examples, compatibility,
changelog, and release checklist. Examples must distinguish current activity
from verified activity history.

## Risks / Trade-offs

- **[Activity checkpoints enlarge ledgers and packages]** → Store one target
  per explicitly stamped prefix, keep target paths deterministic, and avoid
  automatic checkpoints for every local event.
- **[Users mistake unbound activity for verified history]** → Always render
  coverage next to active state and retain the explicit completeness
  limitation in every verification mode.
- **[A new event makes the newest verified checkpoint partial]** → Preserve
  the earlier verified result, identify the uncovered head, and provide the
  explicit lifecycle stamp next action.
- **[Optional-field encoding diverges across languages]** → Publish exact
  absent-versus-empty vectors and require byte-for-byte Go/reference parity.
- **[Missing-property locations are less visually precise than present-node
  errors]** → Prefer an exact pointer with no invented line over misleading
  line 1; keep actual spans for every existing invalid node.
- **[The breaking contract cutover rejects v2.0.1 files and ciphertext]** →
  Fail before side effects, state the boundary prominently, and do not promise
  an in-process migration that the project cannot verify safely.
- **[Deletion remains unknowable after every independent observation is
  destroyed]** → State this as a fundamental evidence limitation and never
  claim ledger completeness from local absence.

## Migration Plan

1. Publish and independently verify the upstream contract correction with all
   schema, semantic, reference, vector, fixture, documentation, tag, archive,
   and checksum assets.
2. Import that exact release in one CLI change and update every version and
   provenance pin; reject the prior schema before any side effect.
3. Add presence-aware private bundles, lifecycle checkpoints, target scope
   routing, semantic validation, verification, publication, and adapter
   presentation behind the new contract version.
4. Regenerate request/result contracts and complete deterministic unit,
   conformance, crypto-vector, parser, property, fuzz, CLI/MCP parity,
   publication, documentation, and native-platform checks.
5. Release as the next breaking pre-1.0 version only after exact artifact and
   checksum verification. Rollback means reverting the whole contract import
   and implementation before release; never ship a mixed-schema intermediate.
