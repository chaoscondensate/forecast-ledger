# Forecast Ledger CLI documentation

<!-- doc-metadata
coverage: v0.11.0
reviewed: 2026-09-23
owner: documentation
generated: false
security-critical: false
prerequisites: none
next: getting-started/index.md
-->

This documentation covers Forecast Ledger CLI release `v0.11.0`.
The product is Preview and implements authoring,
cryptographic targets and sealed forecasts, RFC 3161 timestamps,
layered verification, standalone publication packages, and the MCP stdio
adapter. See the
[reviewed implementation baseline](development/documentation-baseline.md)
before relying on a workflow.

## Start here

- [Getting started](getting-started/index.md) — install the CLI and understand
  the currently supported first commands.
- [Tutorials](tutorials/index.md) — learn complete workflows after their
  implementation and executable checks are available.
- [How-to guides](how-to/index.md) — solve one operational task.
- [Reference](reference/index.md) — inspect exact interfaces, formats, versions,
  and contract details.
- [Explanation](explanation/index.md) — understand the contract, architecture,
  evidence model, and project context.
- [Security](security/index.md) — review threats, key custody, trust sources,
  privacy, and reporting boundaries.
- [Development](development/index.md) — build, test, release, and maintain the
  project and its documentation.

Project help and reporting routes are listed in the root
[support guide](../SUPPORT.md).

## Current safe first workflow

From the current source, start with an empty ledger and add questions or
forecasts when ready in [Create a ledger](getting-started/create-ledger.md).
To inspect an existing Forecast Ledger file without a network request:

```sh
forecast-ledger validate --file ledger.yaml
```

Every ledger operation requires an explicit file. RFC 3161 verification trusts
only the CA bundle retained with the ledger; protocol support is not a
security-audit claim.

[Repository README](../README.md) · [Project repository](https://github.com/chaoscondensate/forecast-ledger)
