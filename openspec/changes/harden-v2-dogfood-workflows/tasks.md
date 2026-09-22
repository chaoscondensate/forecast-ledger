## 1. Publish the upstream contract correction

- [x] 1.1 In the separate `chaoscondensate/schema` repository, create the v2.0.1 contract change and update the schema `$id`, exact `schema_version`, release metadata, and compatibility text without retaining a second runtime contract.
- [x] 1.2 Remove `lifecycle_events` from both public and sealed `forecast-envelope/v2` projections in the upstream reference implementation and document lifecycle activity as outside the immutable recorded-belief target.
- [x] 1.3 Add upstream canonical target vectors and conformance tests proving public and sealed envelopes omit present lifecycle events while retaining every other allowed forecast field byte-for-byte.
- [x] 1.4 Update upstream examples, invalid cases, docs, release notes, checksums, and provenance; run the full schema/reference test suite and independently inspect the target projection change.
- [x] 1.5 Publish exact release v2.0.1 at commit `55b1431d379128e1d75b9c30a3874398cea9ff0f` and record tag object `ac69f718de92f2c087b44b1b02d325ba37945db6`, archive SHA-256 `bfe00b166efdef229848afd6d623eeb965507f314482e59b9c0663bb87ff7a41`, `SHA256SUMS` SHA-256 `fa4c72bc5f4f27dd936e95cf9ad61fdee7c1d12ecb17ca847ac00b919c8664d6`, and schema SHA-256 `5ceeeca7b2b3e46884c116aeb4f20998daf619569c0191728f574c245a4a595b`.

## 2. Adopt the exact corrected contract

- [x] 2.1 Verify the published upstream commit, tag object, archive, checksum asset, schema digest, license, and per-file digests before changing any embedded bytes.
- [x] 2.2 Replace the retained v2.0.0 contract directory with the corrected release files and update `internal/schema`, build information, attribution, generated examples/schemas, fixtures, and all exact version and digest assertions together.
- [x] 2.3 Keep unsupported v2.0.0 input rejection before locks, key creation, artifact directories, entropy, or network effects; add CLI and MCP negative coverage and do not add a converter or compatibility bundle.
- [x] 2.4 Run embedded-schema metaschema, published valid/invalid corpus, reference parity, seal vector, and new target vector checks against the exact corrected release.

## 3. Preserve evidence across lifecycle events

- [x] 3.1 Remove lifecycle events from the Go target projection and classify the model field as deliberately excluded while preserving the remaining public and sealed field allowlists and `forecast-envelope/v2` scope.
- [x] 3.2 Add target-byte and digest tests for withdrawal, expiry, and reaffirmation both before and after target construction, covering public, sealed, and revealed forecasts.
- [x] 3.3 Use deterministic retained RFC 3161 fixtures to prove local verification and publication verification still pass after lifecycle events while reporting the derived active state separately.
- [x] 3.4 Add a negative v2.0.0 fixture, including retained lifecycle-bearing target metadata, and assert unsupported-schema rejection before target artifact access, locks, writes, entropy, or network effects.

## 4. Restore relationship authoring on YAML

- [x] 4.1 Normalize whole-collection creation and item append values for discriminated unions through their ordered JSON contract shape before JSON or YAML rendering.
- [x] 4.2 Add document and shared-service tests for both relationship variants in absent and populated collections, expanded block style, stable field order, unrelated-byte preservation, and atomic validation failure.
- [x] 4.3 Add CLI and in-process MCP acceptance tests showing group-membership and conditional `relationship add` requests produce equivalent YAML and JSON models in commit and dry-run modes.
- [x] 4.4 Extend the maintained YAML mutation matrix so any future union addition that emits wrapper fields or loses JSON/YAML parity fails before release.

## 5. Make revision defaults strictly monotonic

- [x] 5.1 Add a shared timestamp-derivation helper that selects an omitted revision effective time strictly after the prior effective instant and an omitted recorded time no earlier than the operation observation, derived effective time, or prior recorded time.
- [x] 5.2 Apply the helper only to omitted revision times; preserve explicit values and the existing invalid-field failures for equal, backdated, overflowed, or otherwise invalid explicit chronology.
- [x] 5.3 Add service, CLI, and MCP tests with identical and regressing clock observations, prior fractional timestamps, previous recorded time later than effective time, and deterministic RFC 3339 fractional output.

## 6. Use the ledger timezone for default operation times

- [x] 6.1 Route omitted CLI `forecast reveal --revealed-at` through the existing ledger-timezone observation path and preserve explicit RFC 3339 input validation.
- [x] 6.2 Reuse the already loaded ledger timezone for omitted `recorded_at` in resolve, ambiguous, void, dispute, and not-applicable CLI operations.
- [x] 6.3 Add fixed-clock CLI/MCP parity tests for UTC and a DST-aware non-UTC timezone, asserting equivalent instants, expected offsets, and no host-timezone leakage.

## 7. Correct protected-file diagnostics

- [x] 7.1 Generalize safe existing-file resolution and protected reads to accept a bounded purpose label while preserving regular-file, symlink/reparse, ACL, size, race, and path-confinement checks.
- [x] 7.2 Pass accurate labels from ledger, key, protected-input, CA-bundle, and other callers; keep stable application codes and prevent full paths or secret material from entering normal output.
- [x] 7.3 Add storage, service, CLI, and MCP tests proving a missing reveal key reports `key file does not exist` with `not_found`, while missing ledgers retain the ledger-specific diagnostic and all failures are non-mutating.

## 8. Update user and release documentation

- [x] 8.1 Correct `docs/how-to/build-targets.md` so the envelope shape names `question.id` rather than a root ledger ID and lists lifecycle events among the excluded mutable activity fields.
- [x] 8.2 Update lifecycle, verification, security/evidence, compatibility, getting-started, README/status, and reference guidance for the corrected contract and strict unsupported-v2.0.0 boundary, pointing historical interpretation to the immutable upstream tag without adding a runtime compatibility path.
- [x] 8.3 Update the changelog, documentation baseline, release instructions, exact contract pins, release artifact inventory, and generated public metadata without presenting unavailable compatibility or migration behavior.
- [x] 8.4 Re-run generated command/MCP/request-schema inventories and copyable examples; commit generated changes only where contract identity or actual output changed.

## 9. Verify and release the correction

- [x] 9.1 Import the round-13 no-network relationship and same-tick revision reproducers as maintained deterministic acceptance coverage and reproduce that all six confirmed findings are closed.
- [x] 9.2 Run `gofmt -w cmd internal`, `go mod verify`, `go test ./...`, `go vet ./...`, `go tool govulncheck ./...`, and `go test ./internal/doccheck`; run targeted fuzz/property checks for structural patches, timestamps, and canonical targets.
- [x] 9.3 Run the deterministic seal, target, RFC 3161, publication-package, YAML/JSON, and CLI/MCP parity suites with no live TSA dependency, and compare the Go target bytes to the published upstream vector.
- [ ] 9.4 Exercise protected-file handling, locks, safe replacement, path confinement, interruption recovery, and the corrected dogfooding workflows on native macOS, Linux, and Windows CI before publishing the next release.
- [ ] 9.5 Review the final release artifacts and documentation against the built binary, record independent review status honestly, and confirm no v2.0.0 runtime contract, converter, or compatibility code remains.
