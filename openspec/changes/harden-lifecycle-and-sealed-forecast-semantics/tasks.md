## 1. Publish the Authoritative Contract Correction

- [x] 1.1 Add upstream failing semantic cases for lifecycle `effective_at` before `forecasted_at` and lifecycle `recorded_at` before the forecast's `recorded_at`, with exact pointers and stable issue codes.
- [x] 1.2 Define the closed upstream activity-checkpoint collection and `forecast-lifecycle/v1` target shape, including head identity, full event-prefix coverage, forecast-envelope digest binding, integrity exclusion, and deterministic artifact paths.
- [x] 1.3 Update upstream reference validation and target construction for lifecycle chronology, ordered checkpoint heads, canonical prefix bytes, and retained-evidence verification semantics.
- [x] 1.4 Change the upstream ledger schema so revealed forecasts require representations and a revealed commitment while rationale, key factors, and comment remain independently optional.
- [x] 1.5 Change the upstream protected/private bundle contract so representations remain required, optional field presence is preserved, unknown properties are rejected, and absent values are not converted to empty values.
- [x] 1.6 Update upstream reference seal and reveal operations for presence-aware private fields and conditional reveal projection.
- [x] 1.7 Publish cross-language lifecycle target vectors for empty, withdrawn, reaffirmed, and multi-checkpoint histories, including tampered-prefix negative cases.
- [x] 1.8 Publish seal vectors for a minimal representation-only bundle, absent-versus-empty optional fields, mixed optional-field presence, and unknown/missing-property negatives.
- [x] 1.9 Update upstream data-model, verification, security, compatibility, and release documentation to distinguish current activity, lifecycle coverage, forecast evidence, and the completeness limitation.
- [x] 1.10 Run upstream schema, semantic, reference, conformance, vector, and release checks; publish one immutable correction release and record its exact commit, annotated tag object, archive digest, checksum-asset digest, and schema digest.

## 2. Import the Exact Contract Release

- [x] 2.1 Verify the published upstream commit, annotated tag object, release archive, `SHA256SUMS` asset, schema digest, license, attribution, fixtures, reference files, and vectors before changing retained bytes.
- [x] 2.2 Replace the embedded v2.0.1 contract as one unit and update schema version constants, build metadata, source pins, digests, attribution, package metadata, compatibility statements, and release checks without retaining superseded material.
- [x] 2.3 Add early-rejection coverage proving that the superseded schema version cannot reach locks, artifact reads or writes, entropy, protected keys, network calls, target interpretation, or compatibility conversion.
- [x] 2.4 Re-run embedded-schema extraction, checksum, attribution, fixture inventory, and reference/vector parity tests against the exact published release.

## 3. Implement Lifecycle Chronology and Checkpoints

- [x] 3.1 Add typed ledger models and closed JSON/YAML handling for append-only activity checkpoints, lifecycle target metadata, head identity, and checkpoint integrity states.
- [x] 3.2 Enforce event effective time at or after `forecasted_at` and event recording time at or after both effective time and forecast `recorded_at`, retaining existing ordering, uniqueness, and transition validation.
- [x] 3.3 Enforce checkpoint head existence, strict checkpoint order, unique heads, successively longer covered prefixes, target scope, confined portable declared paths, deterministic CLI-authored paths, and forecast identity binding.
- [x] 3.4 Make lifecycle mutation preflight return exact invalid fields for chronology failures while retaining prospective full-ledger validation as the shared CLI/MCP gate.
- [x] 3.5 Add deterministic `forecast-lifecycle/v1` construction for the current or selected checkpoint head using the bounded RFC 8785 profile and byte-for-byte upstream vector comparison.
- [x] 3.6 Keep `forecast-envelope/v2` construction unchanged by events and checkpoints and retain regression vectors proving public, sealed, and revealed target stability.
- [x] 3.7 Extend target build/check services with the closed forecast/lifecycle scope, head selection, deterministic non-overwriting resource paths, dry-run effects, collision checks, and safe retry behavior.
- [x] 3.8 Extend timestamp stamp/status/verify services to operate on lifecycle targets and append or update the exact-head checkpoint through the existing journaled atomic resource plan.
- [x] 3.9 Preserve earlier checkpoints after later events, classify a new uncovered head as partial coverage, and prevent retries for one head from rewriting another checkpoint's artifacts.
- [x] 3.10 Include every declared lifecycle target, request, response, trust file, digest, and checkpoint identity in publication packages and manifests with portable path checks.
- [x] 3.11 Add recovery and interruption tests for lifecycle target/timestamp transactions, including partially created resources, safe retry, cancellation, and ledger-write rollback.

## 4. Correct Sealed Private Bundle Semantics

- [x] 4.1 Make rationale, key factors, and comment presence-aware in standalone, initial, crypto, and revealed bundle models while keeping representations required and non-empty.
- [x] 4.2 Update generated protected-input schemas for standalone and initial sealed forecasts so only representations are required and all three text fields vary independently.
- [x] 4.3 Update canonical seal encoding and strict open decoding to omit absent properties, preserve present empty values, reject unknown properties, and match every published vector byte-for-byte.
- [x] 4.4 Update standalone sealed forecast and sealed initial-forecast builders, planners, dry-runs, and atomic key workflows for every optional-field combination.
- [x] 4.5 Update reveal authentication and source-preserving patches to add only authenticated fields that were present while preserving target continuity and unrelated YAML/JSON bytes.
- [x] 4.6 Add round-trip tests for minimal, complete, empty-present, and mixed-presence bundles across seal, open, reveal, public target construction, and JSON/YAML mutation.
- [x] 4.7 Add negative tests for missing/empty representations, invalid key factors, unknown properties, altered field presence, mismatched keys, and side-effect-free failure before entropy or output creation.

## 5. Make Protected-Input Diagnostics Actionable

- [x] 5.1 Preserve structured schema-keyword and missing-property metadata instead of reducing every `required` failure to a generic message.
- [x] 5.2 Produce an escaped property pointer such as `/representations` for a missing field without fabricating a scalar at line 1; retain actual bounded line and column for syntax failures and present invalid nodes.
- [x] 5.3 Map semantic failures such as an empty indexed key factor to the exact protected-input pointer and source span.
- [x] 5.4 Ensure CLI, JSON, MCP, logs, and errors expose only stable classification, safe property identity, and bounded location metadata without rejected values, surrounding secret text, raw validator causes, or unrestricted paths.
- [x] 5.5 Prove diagnostic parity and zero side effects with multi-line JSON and YAML protected inputs through standalone seal and both sealed initial-forecast routes.

## 6. Add Activity to Verification and Presentation

- [x] 6.1 Add a shared activity result that derives active state, event count, last event identity/type/times, covered head, and coverage state for every selected forecast.
- [x] 6.2 Verify every declared checkpoint against its exact lifecycle prefix, canonical target, digest, RFC 3161 request/response binding, metadata, and retained trust bytes without network access.
- [x] 6.3 Return `unbound`, `partial`, `pending`, `not_checked`, `verified`, and `failed` coverage only under their specified evidence conditions, with stable reason codes and safe next actions.
- [x] 6.4 Detect covered event deletion, prefix alteration, reordered events, missing heads, mismatched forecast identities, target-byte changes, and broken checkpoint order while leaving independent forecast layers visible.
- [x] 6.5 Treat unavailable declared lifecycle resources as incomplete or pending rather than a proven mismatch, and never claim completeness when all lifecycle evidence is absent.
- [x] 6.6 Update aggregate verification so an observational activity result cannot create `pass`, verified lifecycle evidence can count as applicable, and lifecycle failure/incomplete/pending states follow existing precedence.
- [x] 6.7 Render equivalent activity fields and outcomes in CLI human, plain, stable JSON, MCP structured/text results, and publication verification without exposing sealed content.
- [x] 6.8 Update stable result schemas, outcome classification, reason-code inventories, application categories, exit behavior, and presentation snapshots for the new layer.

## 7. Update CLI and MCP Surfaces

- [x] 7.1 Add a closed forecast/lifecycle scope flag to target and timestamp leaf commands, with lifecycle head requirements, defaults, conflicts, help, completion, and copyable examples.
- [x] 7.2 Add the equivalent closed scope and head properties to MCP target and timestamp tools and route both adapters through the same typed application request.
- [x] 7.3 Update forecast list/show/status output to use the same activity derivation and distinguish forecast integrity from lifecycle coverage.
- [x] 7.4 Regenerate request schemas, MCP tool schemas, completion, help snapshots, command-surface inventory, and result contracts; reject generic wrappers or secret-valued flags.
- [x] 7.5 Add CLI/MCP parity tests for lifecycle target build/check, timestamp status/verify, unbound and partial activity, verified coverage, and all safe failure categories.

## 8. Strengthen Security and Conformance Coverage

- [x] 8.1 Add deterministic service and adapter reproductions for LC1 and S1 against the released binary behavior, then make the same cases pass under the corrected contract.
- [x] 8.2 Add property tests for lifecycle-prefix canonicalization, checkpoint monotonicity, optional private-field presence, and JSON/YAML semantic parity.
- [x] 8.3 Add bounded malformed JSON/YAML and schema-diagnostic fuzz seeds that exercise missing fields, unknown fields, nested representation errors, and secret redaction.
- [x] 8.4 Add bounded malformed RFC 3161 and activity-target tests for request/target mismatch, signature/chain failure, retained trust mismatch, incomplete local evidence, and multiple checkpoint responses.
- [x] 8.5 Verify published seal and lifecycle vectors independently, including exact canonical bytes and SHA-256 values, and keep live TSA canaries separate from normal checks.
- [x] 8.6 Exercise native filesystem permissions, locks, atomic replacement, artifact collisions, interruption recovery, and package portability for lifecycle evidence on macOS, Linux, and Windows.

## 9. Update Public Documentation

- [x] 9.1 Update README status, installation/compatibility text, quick starts, limitations, and release links for the corrected single-contract runtime.
- [x] 9.2 Update getting-started and how-to material with a representation-only protected input, optional private fields, lifecycle target/stamp commands, and copyable cross-platform examples.
- [x] 9.3 Update data-model and security explanations to distinguish recorded belief, current activity, checkpoint coverage, covered deletion detection, and the impossibility of proving history after every independent observation is removed.
- [x] 9.4 Update verification, target, timestamp, publication, CLI, MCP, error/exit, and generated-contract references for scope selection, activity fields, reason codes, and aggregate behavior.
- [x] 9.5 Update the documentation baseline, command-surface inventory, compatibility pins, platform/package support, changelog, release instructions, and third-party attribution affected by the upstream import.
- [x] 9.6 Run documentation metadata, navigation, link, example, generated-schema, help, and `internal/doccheck` checks and verify that no page presents planned or unavailable behavior as released.

## 10. Complete Quality and Release Gates

- [x] 10.1 Run `gofmt -w cmd internal`, `go mod verify`, focused package tests, `go test ./...`, `go vet ./...`, and `go tool govulncheck ./...` with the pinned toolchain.
- [x] 10.2 Run strict OpenSpec validation, conformance fixtures, exact contract-pin checks, generated-file cleanliness, secret scanning, license/notice checks, package verification, and deterministic release tests.
- [x] 10.3 Run native macOS, Linux, and Windows workflows for CLI/MCP behavior, protected permissions, filesystem recovery, RFC 3161 fixtures, lifecycle evidence, packaging, SBOMs, and artifact names.
- [x] 10.4 Prepare the next breaking pre-1.0 release notes and compatibility warning, verify candidate archives and checksums from a clean checkout, and publish only after every retained contract identity and artifact digest agrees.
