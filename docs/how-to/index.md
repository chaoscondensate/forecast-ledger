# How-to guides

<!-- doc-metadata
coverage: v0.11.0
reviewed: 2026-09-22
owner: documentation
generated: false
security-critical: false
prerequisites: ../getting-started/index.md
next: ../reference/index.md
-->

How-to guides solve one named operational task. Until the corresponding
services are implemented, the maintained task guidance is limited to:

- [validate an existing ledger](../../README.md#quick-start);
- [create an empty or populated ledger, or update its current metadata](../getting-started/create-ledger.md);
- [manage platform records](manage-platforms.md);
- [manage typed questions and resolution lifecycle](manage-questions.md);
- [append, list, and inspect public forecasts](manage-public-forecasts.md);
- [seal, reveal, and repair key hints](seal-and-reveal-forecasts.md);
- [build and check deterministic forecast targets](build-targets.md);
- [create, inspect, and verify RFC 3161 timestamps](timestamp-forecasts.md);
- [run layered evidence verification](verify-evidence.md);
- [build and verify standalone publication packages](publish-evidence.md);
- [run the root-confined MCP stdio server](run-mcp.md);
- [recover after an interrupted ledger write](recover-ledger-writes.md);
- [build the project](../development/build.md); and
- [create and verify a release](../development/releasing.md).

RFC 3161 acquisition defaults to the qualified FreeTSA HTTPS profile and
materialized embedded CA. Custom authorities require a public HTTPS URL and CA
bundle. Check the [provider record](../development/rfc3161-providers.md) and
[implementation baseline](../development/documentation-baseline.md).

[Documentation index](../index.md)
