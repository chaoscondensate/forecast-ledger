# Reference

<!-- doc-metadata
coverage: v0.11.0
reviewed: 2026-09-22
owner: interface
generated: false
security-critical: false
prerequisites: ../index.md
next: ../explanation/index.md
-->

Reference material records exact interfaces and contract facts. The
current reviewed sources are:

- [implementation, release, and contract baseline](../development/documentation-baseline.md);
- [dependency versions and licenses](../development/dependencies.md); and
- [complete CLI command and exit reference](cli.md);
- candidate-binary help from `forecast-ledger --help` and
  `forecast-ledger <command> --help`;
- [MCP runtime and tool behavior](mcp.md); and
- [generated operation, tool, input, and result contracts](generated/index.md).

The working initialization interface is described in
[Create a ledger](../getting-started/create-ledger.md) and pinned by
candidate-binary help and JSON goldens.

Public forecast input and behavior are described in
[Manage public forecasts](../how-to/manage-public-forecasts.md).
Question input, update constraints, and current-resolution behavior are
described in [Manage questions and resolutions](../how-to/manage-questions.md).
The exact target workflow and projection boundaries are described in
[Build and check forecast targets](../how-to/build-targets.md).
The cryptographic lifecycle and its security boundaries are described in
[Seal and reveal forecasts](../how-to/seal-and-reveal-forecasts.md).
Timestamp, layered verification, publication, and MCP behavior are described in
their linked how-to guides. RFC 3161 acquisition uses an explicitly selected
authority; later verification is local against retained evidence and trust.

The [generated interface reference](generated/index.md) records shared service
declarations and closed direct request schemas. Runtime discovery remains authoritative
when a root or startup mode conditionally removes a tool.

[Documentation index](../index.md)
