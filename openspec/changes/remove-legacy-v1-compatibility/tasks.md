## 1. Reconcile the V2 Cutover Contract

- [x] 1.1 Update the active `adopt-forecast-ledger-v2` proposal, design, delta specs, and tasks to remove frozen-v1, legacy-vector, Python-legacy-parity, compatibility-guide, and migration obligations without weakening v2 conformance
- [x] 1.2 Define the final archive order and verify that the v2 change removes the old runtime-v1 requirements before this change removes the remaining v1 retention requirements
- [x] 1.3 Update `AGENTS.md` and contributor guidance so source precedence, schema-update rules, and release expectations name only retained v2 contract material

## 2. Delete Legacy Contract and Code Paths

- [x] 2.1 Inventory every v1.3, `forecast-seal/v1`, legacy-manifest, migration, and Forecast Ledger compatibility reference; classify the changelog statement and generic unsupported-version tests as the only intentional non-OpenSpec exceptions
- [x] 2.2 Delete `internal/schema/legacy` and every v1-only file from the retained upstream subset, including compatibility guidance, the v1 seal vector, legacy digest manifest, and legacy verifier, without editing retained v2 contract or fixture bytes
- [x] 2.3 Update application-owned v2 provenance and retained-file digests so they list only files that remain and still verify every retained byte against its exact published digest
- [x] 2.4 Remove v1 schema/vector embedding, accessors, constants, compatibility metadata, and test-only APIs from `internal/schema`, build information, and dependent packages
- [x] 2.5 Remove v1-only Go and Python conformance, cryptographic, fixture, and release-pin tests while preserving complete v2 schema, semantic, target, seal, reveal, timestamp, and publication coverage
- [x] 2.6 Keep a generic early schema-version gate and replace fixture-based legacy checks with a minimal inline unsupported-version test proving no mutation, artifact, secret, or network side effect
- [x] 2.7 Remove v1-specific CLI, MCP, result-schema, help, completion, inventory, and diagnostic wording while retaining the stable generic `unsupported_schema_version` behavior

## 3. Remove Migration and Compatibility Documentation

- [x] 3.1 Delete the maintained v1.3-to-v2 migration guide and retained upstream compatibility page rather than replacing either with another legacy page
- [x] 3.2 Remove migration-guide and v1-compatibility links, navigation entries, examples, warnings, preservation advice, and support claims from README and maintained documentation
- [x] 3.3 Replace v1-specific contributor, security, release, documentation-baseline, dependency, and third-party guidance with current v2-only facts; preserve unrelated OS, package, MCP, and representation compatibility guidance
- [x] 3.4 Reduce the v2 entry in `CHANGELOG.md` to one clear historical statement that the release is breaking, no migration is provided, and there were no active users to migrate
- [x] 3.5 Regenerate request schemas, MCP catalogs, operation contracts, indexes, and other generated references and prove no deleted legacy or migration link remains

## 4. Add Absence and Regression Gates

- [x] 4.1 Extend release checks with a path and content denylist that fails if a v1 schema, fixture, vector, legacy manifest, migration page, compatibility bundle, converter, dual reader, or v1-specific public surface returns
- [x] 4.2 Add focused documentation checks proving the migration page and navigation are absent and the changelog contains the sole allowed public explanation
- [x] 4.3 Prove CLI and MCP reject an arbitrary non-v2 schema version before side effects without importing or naming a retained v1 contract
- [x] 4.4 Re-run all published v2 valid/invalid fixtures and v2 target/seal vectors byte-for-byte after removing mixed or legacy upstream tooling

## 5. Verify and Hand Off

- [x] 5.1 Run `gofmt -w cmd internal tools`, `go mod verify`, focused package tests, `go test ./...`, `go vet ./...`, and `go tool govulncheck ./...`
- [x] 5.2 Run `go test ./internal/doccheck`, REUSE licensing checks, documentation examples, generated-reference drift checks, release checks, and the v2 dogfood workflow
- [x] 5.3 Build deterministic GoReleaser snapshots and inspect macOS, Linux, and Windows archives, packages, SBOMs, embedded v2 identity, notices, and absence of legacy material
- [x] 5.4 Run repository-wide removed-surface searches, review the final diff for accidental deletion of unrelated compatibility guidance or user work, and confirm retained upstream v2 bytes are unchanged
- [x] 5.5 Run `openspec validate remove-legacy-v1-compatibility --strict` and strict validation of every reconciled active change, then record that no user-data migration or compatibility handoff remains
