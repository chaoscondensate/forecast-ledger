> Supersession note (2026-08-30): explicit-TSA-only statements below describe
> the completed v0.4.0 command surface. `add-default-tsa-failover` controls
> timestamp acquisition for the next release.

## Why

> Authoring transport update: `make-authoring-direct-readable` supersedes every
> generic public `--input`, MCP `input`, and public `input_file` clause in this
> active change. Purpose-named protected secret channels are retained.

> v2 cutover: `adopt-forecast-ledger-v2` supersedes the v1.2/v1.3 data,
> command, target, seal, resolution, fixture, and example shapes described
> here. This proposal records historical scope; unfinished gates apply to the
> current v2 implementation.

The repository currently exposes only validation, status, version, and completion while the command surface that creates, maintains, seals, timestamps, verifies, packages, and serves forecast ledgers remains unavailable. The complete product needs one reviewable contract that defines every command's inputs, business rules, side effects, outputs, failure modes, and CLI/MCP parity before implementation resumes.

## What Changes

- Complete 30 command actions: ledger initialization and root-metadata update; platform, question, forecast, and key-hint management; target generation; seal and reveal; RFC 3161 lifecycle operations; layered verification; evidence-package build and verification; and MCP stdio service.
- Define exact leaf-local flags, typed input-file schemas, selector rules, JSON result shapes, exit categories, dry-run behavior, confirmation rules, network boundaries, and cross-platform path behavior.
- Enforce the published Forecast Ledger v1.2.0 model: globally unique stable IDs, append-only forecast history, flexible empty ledger/question states, typed question/value/resolution agreement, lifecycle chronology, exact schema validation, and format-preserving transactions. The `replace-ots-with-rfc3161` change supersedes this change's earlier v1.0.0 minimum-item assumptions.
- Define the complete byte contract for `forecast-envelope/v1`, the pinned `forecast-seal/v1` canonical plaintext and AAD, and a new `forecast-key/v1` protected key file; implement deterministic targets, atomic sealing, authenticated reveal, repairable non-authoritative key hints, explicit non-claims, and byte-for-byte vector conformance.
- Implement the supported pure-Go RFC 3161 subset with an explicit public HTTPS TSA, explicit retained CA bundle, bounded request/response handling, recoverable `.tsq`/`.tsr` artifacts, and honest pending/verified/failure reporting without silently persisting `failed` integrity.
- Implement layered verification and deterministic local evidence packages without requiring source control, hosted publication, a TSA, or any blockchain API during verification.
- Start an MCP stdio server that exposes the same application operations through closed schemas and explicit ledger/output/secret roots, with optional read-only/offline modes, no general write/network grants, and one explicit default-off `--allow-reveal` boundary for irreversible disclosure.
- **BREAKING (preview surface)** Correct the currently registered hidden urfave scaffold before commands become available: real-file requirements replace invalid stdin support for artifact-dependent checks, timestamp verify becomes mutating/dry-runnable, the timestamp upgrade action is removed, `question add` keeps its required scalar `--type`, MCP roots become repeatable, and the existing reveal gate remains as the sole capability flag.
- Replace preview placeholders only when each CLI action and corresponding MCP tool passes its command-specific acceptance, rollback, redaction, conformance, documentation, and native-platform tests; incomplete MCP tools are absent from discovery.
- Close the remaining v0.2.2 dogfooding boundary gaps with aggregate target inspection, one recorded-time default, accurate temporal and source-location diagnostics, consistent inserted-field order, actionable MCP root errors, an explicit immediate-lock-contention contract, and locally visible retained RFC 3161 anchoring details.

## Capabilities

### New Capabilities

- `cli-command-execution`: Shared execution contract for every command, including flags, selectors, typed inputs, output envelopes, dry-run, confirmation, errors, and availability.
- `ledger-authoring-commands`: Business behavior for `init`, `ledger update`, all platform and question operations, and public forecast add/list/show operations.
- `forecast-cryptography-commands`: Business behavior for target build/check, atomic forecast seal/reveal, and safe key-hint repair operations.
- `timestamp-commands`: Business behavior for RFC 3161 stamp, status, and verify operations.
- `verification-commands`: Layered local verification behavior for `forecast-ledger verify`.
- `publication-commands`: Deterministic local evidence-package build and verification behavior.
- `mcp-command-runtime`: MCP stdio startup, roots, read-only/offline modes, explicit TSA input for stamping, tool/resource surface, errors, redaction, and parity with CLI services.

### Modified Capabilities

None. The repository has no archived main capability specs yet. This change supersedes the entire active `build-forecast-ledger-cli-mcp` planning change, incorporates the usable foundations already implemented there, and replaces its conflicting/underspecified command contracts. The older change MUST NOT be synced or archived into main specs separately.

## Impact

- Connects the registered urfave command tree to shared application services and removes the `unavailable` preview behavior command by command.
- Changes hidden preview flags/help and their goldens before those actions are advertised; no available implemented command loses supported behavior.
- Extends application/domain, storage transaction, canonicalization, cryptography, RFC 3161, verification, publication, presentation, and MCP adapter packages.
- Adds purpose-named protected secret-file handling, deterministic artifact naming/manifests, direct CLI/MCP request fields, explicit TSA and retained trust inputs, bounded request/response budgets, conditional MCP reveal discovery, and native platform behavior.
- Adds upstream Python/Go validator parity, byte-level seal/target/key fixtures, official RFC 3161 differential tests, CLI/MCP parity tests, crash/rollback tests, fuzzing, and release gates.
- Adds executable regressions for the v0.2.2 dogfooding findings and requires the affected help, README, command reference, MCP descriptions, and packaged-platform evidence to remain current.
- Updates README, command reference, tutorials, security guidance, MCP setup, evidence limitations, and implementation status as each command becomes available.
