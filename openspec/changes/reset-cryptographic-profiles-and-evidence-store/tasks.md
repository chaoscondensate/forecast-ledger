## 1. Publish and pin the v2.2.0 contract

- [x] 1.1 Update the upstream Forecast Ledger schema, semantic rules, reference tools, and documentation with retained target integrity variants and the exact `forecast-seal/v3`, `forecast-key/v3`, `forecast-envelope/v3`, and `forecast-lifecycle/v2` profiles.
- [x] 1.2 Generate positive, negative, presence, tamper, and cross-language vectors for every changed seal, key, envelope, lifecycle, retained-target, evidence-index, and publication byte sequence, and pass the upstream conformance suite on the candidate release commit.
- [x] 1.3 Publish one immutable upstream v2.2.0 release and independently verify its commit, annotated tag object, source archive, `SHA256SUMS` asset, standalone schema, fixtures, vectors, attribution, and release notes.
- [x] 1.4 Record the exact upstream commit, tag object, release archive digest, checksum-asset digest, and schema digest only after the release assets exist and agree.

## 2. Replace the embedded contract and typed model

- [x] 2.1 Replace the embedded v2.1.0 source tree with the exact v2.2.0 release bytes and update schema/build metadata, attribution, third-party notices, and releasecheck pins together.
- [x] 2.2 Update typed ledger unions for retained forecast targets and retained lifecycle checkpoints with closed JSON/YAML decoding and cloning.
- [x] 2.3 Update schema, format, semantic, and artifact-context validation for retained states, v3/v2 profile constants, checkpoint ordering, target metadata, and exact v2.2.0 admission.
- [x] 2.4 Add negative fixtures proving v2.1.0 and every superseded profile fail before locks, protected files, evidence access, entropy, writes, and network effects.
- [x] 2.5 Remove v2.1.0 schema bytes, profile constants, parsers, converters, vectors, compatibility branches, and superseded generated artifacts from the runtime.

## 3. Implement the new cryptographic profiles

- [x] 3.1 Implement `forecast-key/v3` encoding and strict bounded decoding with exact question, revision, and forecast binding.
- [x] 3.2 Implement `forecast-seal/v3` plaintext, associated data, commitment, encryption, and reveal using the exact published canonical vectors.
- [x] 3.3 Implement `forecast-envelope/v3` for public, sealed, and revealed forecasts while preserving target bytes across reveal.
- [x] 3.4 Implement `forecast-lifecycle/v2` with the v3 envelope digest, exact lifecycle prefix, and head binding.
- [x] 3.5 Return typed internal crypto failure stages and map only AEAD failure to `reveal.authentication_failed` and authenticated closed-profile mismatch to `reveal.bundle_profile_mismatch`.
- [x] 3.6 Add deterministic positive tests plus wrong-key, altered-AAD, nonce, ciphertext, digest, canonicality, binding, unknown-field, oversized, property, and fuzz coverage without exposing protected values.
- [ ] 3.7 Reproduce every published upstream and independent cross-language vector byte-for-byte on supported architectures.

## 4. Add the canonical evidence index

- [x] 4.1 Define bounded transport-neutral index types for `forecast-evidence-index/v1`, its ledger/contract binding, closed role-specific entries, references, bindings, formats, and strict path ordering.
- [x] 4.2 Implement canonical RFC 8785 index encoding and strict bounded decoding with duplicate-key, duplicate-path, ordering, role, binding, size, and digest validation.
- [x] 4.3 Enforce confined portable `proofs/` and `trust/` paths, regular-file-only access, symlink refusal, case and Unicode collision detection, maximum depth, entry count, per-file size, path length, and total-byte budgets.
- [x] 4.4 Implement deterministic shared trust-file indexing and reference validation without duplicating identical CA bytes or accepting conflicting roles.
- [x] 4.5 Add malformed, excessive, traversal, absolute-path, separator, collision, symlink, special-file, digest, size, duplicate, and non-canonical index tests and fuzz targets.
- [x] 4.6 Prove that a ledger with no retained evidence creates no empty index, `proofs` directory, journal, or other artifact.

## 5. Reconcile ledger declarations, index entries, and files

- [x] 5.1 Build one bounded service reconciler that inventories ledger declarations, canonical index entries, and actual managed files without network access.
- [x] 5.2 Classify missing or unreadable indexed bytes as not checked, byte/digest/binding mismatches as verification failures, and unindexed managed files as `evidence.unindexed_artifact` incomplete state.
- [x] 5.3 Reconstruct every declared forecast and lifecycle target and every RFC 3161 request/response/trust branch against exact indexed bytes.
- [x] 5.4 Parse unmatched indexed lifecycle targets and establish `activity.retained_evidence_unreferenced` only when question, forecast, v3 envelope, prefix, and head binding validate.
- [x] 5.5 Preserve `unbound` only when no declared, indexed, or managed lifecycle evidence remains and retain the explicit completeness limitation.
- [x] 5.6 Integrate reconciliation into CLI/MCP verification, target/timestamp mutation preconditions, publication build, and publication verification through shared services.
- [x] 5.7 Add deterministic tests for deleted events/checkpoints with retained targets and timestamps, missing indexes, missing files, planted files, unrelated forecast entries, custom safe paths, and all aggregate precedence branches.

## 6. Make target and timestamp retention atomic

- [x] 6.1 Change forecast `target build` to plan and atomically commit the retained ledger state, target file, and evidence-index entry.
- [x] 6.2 Change lifecycle `target build` to require direct checkpoint ID and `recorded_at` inputs, append those exact authored fields in the retained checkpoint, and atomically commit its ledger, target, and index effects without contacting a TSA or deriving either field.
- [x] 6.3 Update dry-run results, human/plain/JSON output, MCP results, tool annotations, and effect reporting to show that target build mutates both ledger and evidence resources.
- [x] 6.4 Change timestamp stamp/retry flows to consume retained target state and atomically update request, response, trust, index, and pending/verified ledger state.
- [x] 6.5 Extend the existing recoverable resource journal with exact evidence-index before/after identity and validated multi-resource replacement ordering.
- [ ] 6.6 Add fault injection at every create, flush, journal, replace, directory sync, cleanup, cancellation, and recovery boundary and prove no successful result leaves ledger, index, or artifacts divergent.
- [ ] 6.7 Test concurrent target/timestamp writers, stale locks, retries, artifact collisions, and native replacement behavior without introducing a second transaction framework.

## 7. Update verification outcomes

- [x] 7.1 Add index and reconciliation observations to transport-neutral verification reports with stable safe reason codes and no unrestricted paths.
- [x] 7.2 Update activity verification so a valid detached lifecycle target fails with `activity.retained_evidence_unreferenced` instead of returning `activity.unbound` and aggregate pass.
- [x] 7.3 Update aggregate classification to require successful reconciliation before pass and preserve fail, incomplete, pending, no-evidence, and pass precedence exactly.
- [ ] 7.4 Preserve independently established document, content, activity, timestamp, reveal, outcome, manifest, index, and file observations on every recoverable non-success result.
- [ ] 7.5 Add CLI/MCP human, plain, and JSON parity goldens for reconciled pass, detached evidence failure, missing indexed bytes, unindexed files, pending evidence, and true no-evidence selections.

## 8. Replace publication packages with v3

- [x] 8.1 Define the closed `forecast-ledger-publication/v3` manifest profile and evidence-index role with canonical encoding, ordering, size, digest, and portable path rules.
- [x] 8.2 Change publication build to require a fully reconciled store and copy the byte-exact ledger, evidence index, and every indexed declared artifact without scanning or omitting an allowed subset; when no local evidence exists, synthesize the canonical empty index inside the package without mutating the source directory.
- [x] 8.3 Refuse package creation for detached, unindexed, missing, unsafe, mismatched, or conflicting evidence and remove all partial output on failure or interruption.
- [x] 8.4 Change publication verification to reject v2 manifests, extra files, missing index entries, unlisted files, changed bytes, rebound artifacts, and profile mismatches before layered offline verification.
- [ ] 8.5 Add deterministic cross-platform package fixtures for empty, retained-only, pending, verified, lifecycle-partial, shared-trust, detached-evidence, tampered, and unexpected-file cases.
- [ ] 8.6 Prove package build and verification never read protected key/input roots, contact a network service, or include key, salt, plaintext, credential, journal, lock, or temporary bytes.

## 9. Correct public forecast inspection

- [x] 9.1 Remove service-level blanking of sealed nonce and ciphertext and expose the complete stored public commitment through one shared forecast view.
- [x] 9.2 Preserve raw-key, salt, plaintext, private-field, credential, protected-path, and unrestricted-error redaction for sealed and revealed records.
- [x] 9.3 Update CLI human/plain/JSON `forecast show`, MCP `forecast_show`, and forecast resources to return equivalent commitment fields and reveal reason codes.
- [ ] 9.4 Add unrevealed/revealed inspection goldens, secret canaries, wrong-key tests, authenticated profile-mismatch tests, and zero-mutation assertions across CLI and MCP.

## 10. Regenerate interfaces and documentation

- [x] 10.1 Regenerate CLI/MCP request and result schemas, operation contracts, command-surface inventories, examples, help, completions, and resource descriptions for retained targets, direct lifecycle checkpoint ID/recording-time inputs, role-specific index results, profile IDs, and publication v3.
- [x] 10.2 Update README, getting started, target, timestamp, verification, sealed forecast, publication, MCP, troubleshooting, and error/exit documentation with copy-tested v2.2.0 commands and outputs.
- [x] 10.3 Update security and verification-claim guidance for the closed managed store, unindexed-file handling, detached lifecycle evidence, public nonce/ciphertext, profile-stage diagnostics, and the remaining deletion/completeness limits.
- [x] 10.4 Update documentation baseline, dependencies, platform support, release instructions, compatibility statements, changelog, and historical v0.10.0 guidance without advertising conversion or dual support.
- [x] 10.5 Reconcile or supersede active OpenSpec text that still requires v2.1.0 profile names, standalone undeclared targets, declaration-only publication, or the seven-field private bundle.
- [x] 10.6 Run documentation metadata, navigation, link, example, release-asset-name, cross-platform command, and `internal/doccheck` validation.

## 11. Complete security, conformance, and release gates

- [x] 11.1 Run `gofmt -w cmd internal`, `go mod verify`, `go test ./...`, `go vet ./...`, and `go tool govulncheck ./...` with the selected toolchain.
- [x] 11.2 Run race tests plus targeted fuzz/property suites for canonicalization, seal/reveal, index parsing, reconciliation, RFC 3161, manifests, storage journals, and path confinement.
- [ ] 11.3 Verify native macOS, Linux, and Windows locks, ACL/key protection, case behavior, symlink refusal, safe replacement, cancellation, and journal recovery; do not substitute cross-builds for native filesystem tests.
- [x] 11.4 Verify CLI/MCP parity, stdout/stderr and JSON purity, non-TTY/no-color behavior, read-only/offline/reveal modes, root confinement, cancellation, concurrency, and real stdio protocol sessions.
- [x] 11.5 Run a denylist audit proving superseded schema/profile/package identifiers occur only in explicit history and no converter, compatibility flag, legacy vector, or dual parser ships in the binary.
- [ ] 11.6 Build release candidates from a clean checkout, verify archives, checksums, provenance, embedded contract identity, generated artifacts, and documentation, and publish only after every v2.2.0 and v3/v2 identity agrees.
