# Build and verify an evidence package

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-22
owner: security
generated: false
security-critical: true
prerequisites: verify-evidence.md
next: run-mcp.md
-->

Publication is local and transport-neutral. It does not require Git, a remote
repository, or a hosted publisher.

```sh
forecast-ledger publish build --file ledger.yaml --output evidence-package
```

The destination must not exist. Build validates the ledger, rebuilds referenced
targets, validates timestamp artifact structure, and copies only:

- the ledger at `ledger/<original-name>`;
- each referenced `forecast_target`, including every declared lifecycle target;
- each referenced `timestamp_request` (`.tsq`);
- each referenced `timestamp_response` (`.tsr`); and
- each referenced `timestamp_ca_bundle` (PEM).

A shared CA path is copied once only when its bytes and role agree. Missing,
escaping, symlinked, conflicting, or tampered artifacts fail the build. Secret
keys, private input, locks, journals, temporary files, and unrelated neighbors
are excluded.

A timestamped package keeps proofs and trust beside `ledger/`, not inside it:

```text
evidence-package/
  ledger/ledger.yaml
  proofs/targets/<forecast>.json
  proofs/targets/<forecast>.lifecycle.<head>.json
  proofs/timestamps/<forecast>/.../request.tsq
  proofs/timestamps/<forecast>.lifecycle.<head>/.../response.tsr
  trust/rfc3161/...pem
  manifest.json
```

The canonical `forecast-ledger-publication/v2` manifest pins schema v2.1.0 and
records each allowlisted path, role, size, and SHA-256 digest.

Verify on another machine with no network option:

```sh
forecast-ledger publish verify \
  --file evidence-package/ledger/ledger.yaml \
  --manifest evidence-package/manifest.json
```

The commands are the same in macOS and Linux shells and in PowerShell on
Windows. Quote local paths when they contain spaces. Paths recorded in the
manifest use portable forward slashes on every platform.

For MCP, configure a named output root such as `packages`, then verify both
files from that root:

```json
{
  "file": "packages:evidence-package/ledger/ledger.yaml",
  "manifest": "packages:evidence-package/manifest.json"
}
```

The MCP adapter resolves the packaged ledger and its sibling `proofs/` and
`trust/` trees against the same package root. It does not reinterpret artifact
paths relative to the `ledger/` directory.

Verification first checks the manifest and every listed file, rejects extra
files, then runs the same forecast target, activity checkpoint, RFC 3161,
reveal, chronology, and outcome metadata checks against packaged bytes. It
does not contact a TSA, blockchain, Git host, system trust store, or outcome
URL. Manifest observations remain available even when the evidence aggregate
is pending, failed, or `no_evidence`.

The package is portable evidence, not proof of authorship, completeness, truth,
TSA clock honesty, current revocation status, or long-term validation.

[Run the MCP server](run-mcp.md)
