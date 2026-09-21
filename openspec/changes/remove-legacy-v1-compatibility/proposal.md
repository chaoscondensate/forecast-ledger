## Why

Forecast Ledger CLI has no active users on the pre-v2 format, so retaining v1.3 schemas, fixtures, cryptographic vectors, compatibility tests, and migration guidance adds maintenance and security-review cost without protecting anyone. The project should ship one clear v2 contract and avoid implying that a supported migration path exists.

## What Changes

- **BREAKING** Remove the retained v1.3 schema, fixtures, legacy seal vector, digest manifest, reference tooling, and all tests whose only purpose is preserving or checking v1 bytes.
- Remove v1-to-v2 migration and compatibility guides, navigation, examples, release instructions, and contributor rules.
- Remove v1-specific runtime metadata, compatibility constants, generated references, denylist exceptions, and release gates. Keep only the generic rule that any schema version other than the current v2 contract is rejected before side effects.
- Reconcile the active v2 OpenSpec artifacts so they no longer require frozen v1 evidence or migration documentation.
- Keep one short `CHANGELOG.md` statement that v2 is intentionally breaking, no migration is provided, and there were no active users to migrate.
- Do not add a converter, dual reader, compatibility mode, deprecation period, or archived compatibility bundle.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `forecast-ledger-v1-3-contract`: Remove the remaining requirements to retain and regression-test the obsolete v1.3 contract; the capability is retired completely.
- `empty-ledger-workflows`: Remove compatibility-note and migration-documentation obligations from generated and public empty-first guidance while retaining v2-only examples.

## Impact

This removes legacy files under `internal/schema`, v1-only validation and cryptographic test paths, migration/compatibility documentation, stale generated references, and release checks for v1 material. Runtime authoring, validation, sealing, reveal, timestamp, and publication remain v2-only; unsupported versions still fail safely and atomically. The active `adopt-forecast-ledger-v2` change must be updated before either change is archived so their requirements and tasks do not conflict.
