## 1. Pin and Vendor the Published Contract

- [x] 1.1 Re-download the immutable v2.0.0 release assets and verify commit `1d3b186a15136bc5aff38647cb59fbef475dbe55`, annotated tag object `7b4a9e85e0df9350750828a57b03ff729f704ee4`, publication time, archive SHA-256 `1d56cbe4f6cbd1fccb046a99add2ff4f88c709904d027669039ba4139664f47e`, and `SHA256SUMS` SHA-256 `77d093fbdb393dc9c1e3bdae053724e5ecdccd2e211f6f6c08c7678e78b178dc`
- [x] 1.2 Copy the v2 schema, license, valid examples, positive conformance fixture, invalid mutation corpus, v2 seal vector, and required v2 reference documents unchanged into versioned vendor or testdata locations
- [x] 1.3 Record the v2 source provenance and every retained-file digest, including schema SHA-256 `efd87b7432f7cb017fbedaba217e4cd1d3bff06133457927fb786a249a040b21` and v2 seal-vector SHA-256 `7cd814473f3e84617660e704f1aea8dc48e4b4a32cc86b93fd46c1649a4728d5`
- [x] 1.4 Remove all pre-v2 schemas, fixtures, vectors, manifests, and compatibility tooling instead of retaining a legacy-test boundary
- [x] 1.5 Remove superseded active v1 contract and fixture paths
- [x] 1.6 Update `internal/schema` embedding, immutable constants, license access, conformance access, source tests, and third-party attribution to the reviewed v2 release
- [x] 1.7 Update contributor guidance and release-pin checks so a future schema update must change commit, tag object, archive, checksum asset, schema digest, attribution, fixtures, reference semantics, and vectors together

## 2. Replace the Typed Ledger Model

- [x] 2.1 Replace the root ledger model with the exact v2 root shape, including optional groups and relationships and the v2 publication/platform definitions
- [x] 2.2 Implement closed typed question, revision, outcome-space, and domain unions for binary, categorical, ordinal, numeric, date, and datetime kinds
- [x] 2.3 Implement typed bounds, continuous/step/allowed-values policies, units, option sets, bin sets, and version references with exact schema field names and requiredness
- [x] 2.4 Implement the closed scalar outcome type that preserves boolean versus string and rejects null, numbers, objects, arrays, or multiply populated values
- [x] 2.5 Replace v1 forecast values with typed probability, PMF, binned-PMF, quantiles, CDF, point, and credible-interval representation unions
- [x] 2.6 Implement v2 provenance, importer, artifact snapshot, group, group-membership, conditional relationship, and lifecycle-event types
- [x] 2.7 Implement all v2 question status, resolved/unresolved/not-applicable resolution, resolution-source, commitment, target, RFC 3161, and integrity union shapes
- [x] 2.8 Add exhaustive JSON marshal/unmarshal tests for every union branch, unknown discriminator, missing discriminator, multiple branch, optional field, and closed-object failure
- [x] 2.9 Replace clone, selector, summary, and equality helpers with exhaustive v2-aware implementations that preserve order and exact scalar strings
- [x] 2.10 Add model round-trip tests for every published JSON/YAML fixture and assert no decode/encode path introduces JSON floats or normalizes target-covered strings

## 3. Build Exact Scalar and Domain Semantics

- [x] 3.1 Implement one canonical-decimal parser covering sign, leading and trailing zero, exponent, negative-zero, and precision rejection without using binary floating point
- [x] 3.2 Implement exact decimal comparison, addition, range, probability, open-probability, and step-alignment helpers using bounded standard-library integer or rational arithmetic
- [x] 3.3 Implement typed binary, option, numeric, date, and datetime scalar parsing and domain-membership checks while retaining the original ledger strings
- [x] 3.4 Implement date `P<n>D` and datetime `PT<n>S` step parsing, default origins, offset-aware instant comparison, bounds, and allowed-values ordering
- [x] 3.5 Add table, property, and fuzz coverage for canonical decimals, exact sums, negative values, 18-digit probabilities, steps, bounds, dates, datetime offsets, and malformed inputs

## 4. Cut Over Admission, Schema, and Semantic Validation

- [x] 4.1 Change bounded version admission to accept only `2.0.0` and prove missing, malformed, non-current, and future versions fail with `unsupported_schema_version` before write, entropy, secret, artifact, or network effects
- [x] 4.2 Point Draft 2020-12 validation and schema resources exclusively at the embedded v2 bytes and verify the schema against its metaschema without remote resolution
- [x] 4.3 Implement root, forecaster, platform, global question-ID, global forecast-ID, and timezone semantic checks from the pinned reference validator
- [x] 4.4 Implement revision uniqueness, current-last binding, effective/recorded chronology, outcome/domain matching, domain policy, option, bounds, and bin continuity checks
- [x] 4.5 Implement forecast revision lookup, forecast/recorded chronology, forecasting-open checks, append order, global ID uniqueness, and earlier-same-question supersession checks
- [x] 4.6 Implement representation uniqueness, domain compatibility, PMF coverage/sums, binned coverage/tails, quantile ordering, CDF monotonicity/tails, point semantics, and credible-interval checks
- [x] 4.7 Implement platform-provenance lookup and confined local SHA-256 verification for optional snapshot artifacts without following source URLs
- [x] 4.8 Implement lifecycle transition, event identity, effective/recorded ordering, and event-provenance checks
- [x] 4.9 Implement relationship identity/reference checks, duplicate memberships, typed parent outcomes, conditional DAG detection, and not-applicable agreement
- [x] 4.10 Implement resolved outcome/domain checks, terminal status/detail agreement, resolution chronology, required sources, and verified-token-before-known-outcome rules
- [x] 4.11 Implement integrity target digest checks and v2 revealed-commitment verification with explicit ledger-relative artifact roots and confined path handling
- [x] 4.12 Assign stable semantic issue codes and precise paths to every new validator class and verify schema failures stop unsafe semantic traversal
- [x] 4.13 Build a deterministic Go runner for every upstream valid fixture and mutation-based invalid case and require the intended rejection class
- [x] 4.14 Add case-by-case deterministic Go coverage for every retained published v2 fixture while keeping the normal test suite independent of Python, checkout state, and network access

## 5. Rebuild Application Requests and Registry

- [x] 5.1 Replace v1 question/value input schemas with closed typed v2 revision, domain, representation, provenance, relationship, lifecycle, and resolution request schemas
- [x] 5.2 Define explicit application operations and selectors for question revision append; ambiguous, void, disputed, and not-applicable resolution; groups; relationships; and forecast withdraw, expire, and reaffirm
- [x] 5.3 Require question revision IDs on later forecast add/seal and resolved-outcome requests, and require exact option-set or bin-set references on PMF requests
- [x] 5.4 Keep the one atomic initial-question/initial-forecast exception explicit and bind its forecast to the newly created sole revision without accepting a caller-supplied conflicting revision
- [x] 5.5 Extend the service registry to inventory every public field, requiredness, enum, collection order, selector, control, side effect, protected reference, root class, and result type
- [x] 5.6 Generate or mechanically check request JSON Schemas from the registry and embedded v2 definitions, with no generic `input`, public `input_file`, v1 `value`, or basis-point property
- [x] 5.7 Add registry audits proving each non-secret request field has both a CLI route and MCP property and each protected field has only a purpose-named secret route
- [x] 5.8 Add negative registry tests for property collisions, missing revision selectors, unclassified fields, unknown properties, and accidental v1 command or schema reintroduction

## 6. Implement V2 Builders and Transactional File Services

- [x] 6.1 Rebuild empty-ledger initialization for v2 and preserve ledger-only, backlog-question, public-initial-forecast, sealed-initial-forecast, and dry-run result shapes
- [x] 6.2 Build complete first revisions for every domain kind and reject type-specific omissions or incompatible fields before mutation
- [x] 6.3 Implement append-only `question revise` with full revision replacement data, strict chronology, version-reference checks, and current-revision update
- [x] 6.4 Restrict question metadata updates to mutable question-level fields and nonterminal status; prove no path edits or removes historical revisions
- [x] 6.5 Replace v1 resolve/annul/dispute builders with resolved, ambiguous, void, disputed, and not-applicable builders and their distinct required fields
- [x] 6.6 Implement group create/update/remove/list/show services and reject removal while referenced
- [x] 6.7 Implement relationship add/remove/list/show services with typed reference validation, conditional cycle prevention, and guarded removal from terminal resolutions
- [x] 6.8 Implement public forecast creation with explicit revision binding, one-or-more representation kinds, provenance, supersession, and unanchored integrity
- [x] 6.9 Implement lifecycle withdraw, expire, and reaffirm services that append events without reading or changing private forecast content or evidence
- [x] 6.10 Implement provenance builders for revisions, forecasts, and lifecycle events, including optional importer and confined snapshot path/digest fields
- [x] 6.11 Extend ordered JSON/YAML patch construction to every v2 record in schema field order and expanded two-space block style for populated collections
- [x] 6.12 Add JSON/YAML parity and byte-preservation tests for root optional collections, revision append, domain/bin structures, representation arrays, relationships, provenance, lifecycle, and every resolution branch
- [x] 6.13 Route every new mutation through lock, parse, validate, clone, mutate, validate, temporary sibling, flush, safe replace, and recoverable journal behavior
- [x] 6.14 Add dry-run, no-op, conflict, interruption, and recovery tests proving no partial ledger, key, snapshot, target, or journal effect survives failure

## 7. Implement Direct CLI Authoring

- [x] 7.1 Replace v1 question flags with explicit initial-revision and domain flag families for all six outcome kinds, including option versions, bounds, values policies, bins, scale, and unit
- [x] 7.2 Add `question revise` and the v2 terminal resolution leaves; remove `question annul` and obsolete mutable semantic fields from update help and completion
- [x] 7.3 Add direct group and relationship command groups with leaf-local `--file/-f`, stable selectors, guarded removal, list/show output, and copyable examples
- [x] 7.4 Replace `--value-kind`, basis-point, v1 interval, and v1 quantile flags with distinct direct v2 representation flag families that permit several compatible kinds in one forecast
- [x] 7.5 Define and test an escaping-aware repeatable CSV grammar for options, bins, PMF entries, CDF/quantile points, intervals, sources, provenance, and other public nested groups without JSON/YAML strings
- [x] 7.6 Require explicit `--question-revision` and exact set-version flags wherever the service request requires them and reject incomplete, duplicate, ambiguous, or set/clear-conflicting groups as usage errors
- [x] 7.7 Add forecast withdraw, expire, and reaffirm leaves with event ID, effective/recorded times, reason, and optional provenance fields
- [x] 7.8 Update sealed initial and standalone forecast flows so only v2 representations and private reasoning enter protected input and every public binding remains an ordinary flag
- [x] 7.9 Update authoring inventory, help, completion, examples, diagnostics, human/plain/JSON rendering, and golden files for the complete v2 command tree
- [x] 7.10 Add a CLI denylist and acceptance matrix proving no `--input`, public side-loaded file, JSON/YAML string field, v1 type/value flag, `probability_bp`, `multiple_choice`, or annul surface remains

## 8. Implement MCP and Generated Contract Parity

- [x] 8.1 Add closed top-level MCP schemas and dispatch for revision, v2 resolution, group, relationship, lifecycle, provenance, and multi-representation operations
- [x] 8.2 Require the same explicit revision and set-version selectors as CLI and preserve ordered nested arrays as typed MCP data rather than strings
- [x] 8.3 Update MCP root capabilities and protected references so sealed representations remain under configured secret roots and reveal remains default-off
- [x] 8.4 Ensure read-only and offline server modes omit or reject every new mutating or network operation consistently without changing stdio protocol behavior
- [x] 8.5 Regenerate the MCP tool catalog, request schemas, result schemas, resource descriptions, initialization instructions, and maintained references from the shared registry
- [x] 8.6 Add CLI/MCP parity tests for every domain, representation, resolution, relationship, provenance, lifecycle event, dry-run, expected failure, and secret-handling path
- [x] 8.7 Add stdio recovery tests proving expected v2 domain failures remain recoverable tool errors and protocol stdout never contains logs or private data

## 9. Implement Forecast-Seal and Envelope V2

- [x] 9.1 Define closed v2 private bundle, plaintext, associated-data, commitment, projection, and `forecast-key/v2` types that bind question, revision, and forecast IDs
- [x] 9.2 Implement v2 sealing with independent CSPRNG salt, key, and nonce; exact JCS plaintext; SHA-256 commitment; canonical associated data; and ChaCha20-Poly1305 encryption
- [x] 9.3 Implement protected v2 key-file encode/decode, strict canonical shape, exact ID binding, size limits, permissions, and zeroization without exposing key material
- [x] 9.4 Implement reveal verification in the normative order: key/encoding, AEAD, commitment, JSON parse, closed shape, IDs, canonical bytes, public mirrors, sealed target, and retained timestamp checks
- [x] 9.5 Build the exact `forecast-envelope/v2` public projection containing the full bound revision and every allowed forecast statement field while excluding integrity and mutable reveal/key-hint fields
- [x] 9.6 Add exhaustive projection classification tests so every forecast/revision model field is explicitly included, excluded, or secret and new fields cannot silently alter target meaning
- [x] 9.7 Reproduce the published v2 seal vector and reference-generated public, sealed, and revealed target bytes and SHA-256 values byte-for-byte
- [x] 9.8 Remove v1 seal/vector bytes and all v1 seal or envelope generation paths
- [x] 9.9 Add negative, property, and fuzz tests for revision transplant, altered mirrors, malformed canonical JSON, duplicate keys, non-I-JSON values, nonce/key/ciphertext bounds, commitment mismatch, and secret redaction
- [ ] 9.10 Obtain independent review of the v2 seal, target projection, vector reproduction, and reveal ordering before marking cryptographic tasks complete

## 10. Rebind Timestamp, Verification, and Publication Workflows

- [x] 10.1 Change target metadata, deterministic target build/check, collision handling, and stored integrity scope to `forecast-envelope/v2`
- [x] 10.2 Update RFC 3161 stamp, status, and local verify services to bind exact v2 target bytes while preserving SHA-256, retained trust, multiple TSA branches, provider catalog, failover, and safe partial-report semantics
- [x] 10.3 Update layered verification to report revision ID, representation and lifecycle context where safe while preserving evidence limitations and aggregate outcome precedence
- [x] 10.4 Enforce strict `gen_time < outcome_known_at` for every forecast claiming verified timing on a resolved question and retain later tokens only as later integrity observations
- [x] 10.5 Update publication discovery and manifests for v2 targets, multiple timestamps, retained CA bundles, provenance snapshot artifacts where in scope, and the exact v2 contract identity
- [x] 10.6 Verify publication build and offline verify for empty, public, sealed, revealed, inactive, multi-representation, multi-TSA, and failed/pending evidence cases
- [x] 10.7 Add deterministic RFC 3161 and package parity tests for JSON and YAML; keep all normal tests independent of live TSAs, platform URLs, system roots, and current network state

## 11. Update Presentation, Build Metadata, and Contract Output

- [x] 11.1 Update build information, CLI version output, MCP initialization metadata, schema resources, validation results, and package manifests to one v2 version/commit/digest/seal/envelope identity
- [x] 11.2 Update question and forecast summaries, list/show/status views, resource payloads, and stable result schemas for revisions, domains, representations, activity, relationships, provenance, and v2 resolutions without rounding exact values
- [x] 11.3 Preserve existing stable outcome codes where meaning is unchanged and add documented codes only for genuinely new v2 conflicts or validation classes
- [x] 11.4 Update release checks to reject stale pre-v2 runtime metadata, mismatched pins, missing fixtures, generated-contract drift, unsupported package claims, or removed authoring surfaces
- [x] 11.5 Reconcile `complete-forecast-ledger-command-surface`, `add-default-tsa-failover`, and `establish-open-source-product-documentation` artifacts and implementation so their remaining tasks and examples use v2 shapes

## 12. Update Public Documentation and Examples

- [x] 12.1 Update README status, quick start, examples, evidence boundaries, schema links, and development commands for v2 revision/domain/representation authoring
- [x] 12.2 Rewrite getting-started and how-to flows for empty init, first revision, later revision, all representation families, groups, conditions, provenance, lifecycle events, terminal states, seal/reveal, timestamps, and publication
- [x] 12.3 Update CLI, MCP, file/artifact, output, error/exit, contract, and generated reference pages from a built candidate binary and regenerated contracts
- [x] 12.4 Update explanation pages for immutable revisions, exact decimals, domains versus representations, bins/tails/interpolation, activity versus supersession, conditional not-applicable behavior, and scoring boundaries
- [x] 12.5 Update security guidance for v2 revision binding, protected representation bundles, `forecast-key/v2`, snapshot digests, full-revision targets, multiple TSAs, retained trust, and unchanged non-claims
- [x] 12.6 Record the breaking v2 cutover in the changelog without publishing migration or compatibility guidance because there were no active users
- [x] 12.7 Update changelog, release notes/instructions, documentation baseline, platform support, third-party notices, source provenance, contributor guidance, and maturity statements for the breaking contract cutover
- [x] 12.8 Add or update doc metadata and navigation for every maintained page, then run copyable examples on a candidate binary and verify links, artifact names, flags, and cross-platform syntax

## 13. Complete Conformance, Security, and Platform Gates

- [x] 13.1 Run every published v2 valid and invalid fixture through schema, format, semantic, builder, CLI, MCP, target, seal, reveal, timestamp, and publication paths as applicable
- [x] 13.2 Add end-to-end direct-authoring fixtures covering all six domains, all seven representations, version changes, open tails, allowed/step values, groups, conditions, not-applicable, provenance, and lifecycle transitions
- [x] 13.3 Add removed-surface searches and tests across code, tests, generated files, docs, help, completions, and examples for v1 schema pins, `forecast-envelope/v1`, production `forecast-seal/v1`, basis points, `multiple_choice`, mutable revision fields, and annul
- [x] 13.4 Run parser, canonicalization, exact-scalar, relationship-graph, seal/reveal, and target fuzzers with bounded malformed inputs and preserve useful regression seeds
- [ ] 13.5 Test native lock, ACL, protected-key, safe replacement, path confinement, symlink, interruption, and recovery behavior for the new v2 paths on macOS, Linux, and Windows; do not substitute cross-builds for native filesystem evidence
- [x] 13.6 Run deterministic release snapshots and package inspection for macOS, Linux, and Windows artifact names, embedded schema identity, third-party material, and generated references
- [ ] 13.7 Complete the independent cryptographic/security review and record any accepted limitations without overstating audit status

## 14. Final Verification and Handoff

- [x] 14.1 Run `gofmt -w cmd internal` and confirm generated files are clean and reproducible
- [x] 14.2 Run `go mod verify`, focused package tests, `go test ./...`, `go vet ./...`, and `go tool govulncheck ./...`
- [x] 14.3 Run `go test ./internal/doccheck`, generated-reference drift checks, release checks, command/MCP inventories, denylist checks, and all copyable documentation examples
- [x] 14.4 Run every retained published v2 case through deterministic Go conformance coverage
- [x] 14.5 Run `openspec validate adopt-forecast-ledger-v2 --strict` and strict validation for every reconciled active change
- [x] 14.6 Review the final diff for unchanged upstream bytes, exact pins, secret absence, unsupported-version no-side-effect ordering, source-preserving YAML, public documentation completeness, and accidental unrelated worktree changes
