> Supersession note (2026-08-30): the explicit-TSA-only acquisition behavior
> below records the v0.4.0 cutover. For the next release it is superseded by
> `add-default-tsa-failover`; do not reintroduce it as the current requirement.

## Why

Forecast Ledger schema `1.2.0` replaces OpenTimestamps with RFC 3161 and makes the old timestamp object invalid. The CLI still embeds schema `1.1.0` and carries a large experimental OTS and Bitcoin surface, so it must make one breaking pre-adoption cutover before any users or compatibility obligations exist.

## What Changes

- **BREAKING** Replace the embedded Forecast Ledger `1.1.0` contract with exact upstream `1.2.0` bytes pinned to commit `6c2fe3df99223945b8d1613a03f95796b3c7d1e2` and schema SHA-256 `d609982f0fcea1ce076fdb32b44ef0eebe3265754eea7065de9d78a857dab5b8`; accept only `schema_version: 1.2.0`.
- **BREAKING** Remove OpenTimestamps and Bitcoin implementation code, tests, fixtures, result fields, package roles, build metadata, CI jobs, documentation, flags, MCP inputs, configuration, and dependencies. Do not retain an OTS reader, converter, migration command, deprecation alias, or compatibility mode.
- Replace the OTS stamp/upgrade/status/verify lifecycle with an RFC 3161 lifecycle over the exact `forecast-envelope/v1` target: create and retain a SHA-256 `.tsq`, submit it to an explicitly selected TSA, retain the `.tsr`, verify request binding, target imprint, CMS signature, certificate chain, and declared metadata, then record the v1.2.0 timestamp object.
- Remove OTS-only operations and options, including `timestamp upgrade`, calendar/profile controls, Bitcoin Core credentials, explorer observation, and Bitcoin-specific online verification. Keep transport-neutral layered verification and evidence-package workflows, now backed by retained `.tsq`, `.tsr`, and CA-bundle artifacts.
- Keep local ledger validation network-free. Bound all TSA network requests, keep TSA endpoints explicit, reject unsafe endpoints and redirects, preserve dry-run/offline guarantees, and never put credentials or forecast content into argv, logs, results, or MCP protocol output.
- Update CLI, MCP, generated schemas, stable outputs, help, documentation, security guidance, build metadata, release material, active OpenSpec work, and conformance coverage to describe only the v1.2.0 RFC 3161 product.

## Capabilities

### New Capabilities

- `forecast-ledger-v1-2-contract`: Exact embedded v1.2.0 contract, exclusive version admission, upstream conformance corpus, pins, and public metadata.
- `rfc3161-timestamp-evidence`: RFC 3161 request, submission, verification, ledger mutation, layered reporting, and portable evidence-package behavior shared by CLI and MCP.

### Modified Capabilities

- `forecast-ledger-v1-1-contract`: Remove the superseded v1.1.0-only contract requirements rather than retaining compatibility behavior.
- `verification-outcome-semantics`: Replace OTS/Bitcoin observation and mismatch outcomes with RFC 3161 request, response, signature, trust-chain, metadata, and network-failure outcomes while preserving conservative aggregate semantics.

## Impact

- Removes `internal/timestamp/ots`, its public-calendar and Bitcoin-source profile, Bitcoin Core and explorer adapters, OTS liveness CI, and every direct OTS/Bitcoin type and result field.
- Adds a constrained pure-Go RFC 3161 backend and TSA HTTP client below `internal/timestamp/rfc3161`; any selected third-party module must be pinned, license-reviewed, fuzzed, and proven against OpenSSL and the upstream fixtures before release.
- Changes the timestamp CLI/MCP surface, application contracts, ledger model, semantic validation, publication manifest roles, generated input/result schemas, build information, and release compatibility statement.
- Replaces the vendored upstream contract and fixtures together and updates `AGENTS.md`, README, maintained documentation, CHANGELOG, CI, and the still-active command-surface and documentation OpenSpec artifacts so they do not continue planning or publishing OTS behavior.
- Existing v1.1.0 ledgers and `.ots` receipts become unsupported input. No migration or compatibility tooling is provided.
> Its RFC 3161 choice remains historical context, but publication and evidence
> storage requirements are superseded by
> `reset-cryptographic-profiles-and-evidence-store`.
