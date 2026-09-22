# Run the MCP server

<!-- doc-metadata
coverage: v0.10.0
reviewed: 2026-09-22
owner: interface
generated: false
security-critical: true
prerequisites: publish-evidence.md
next: ../reference/mcp.md
-->

Forecast Ledger provides a local stdio MCP server backed by the same Go
services as the CLI. Protocol stdout contains only MCP messages.

```sh
forecast-ledger mcp serve \
  --ledger-root main=/data/forecast-ledgers \
  --output-root packages=/data/forecast-packages \
  --secret-root keys=/data/forecast-secrets
```

Root flags are repeatable. Use a short lowercase name and an existing absolute
or relative directory. Tool file references then use
`root-name:relative/path`, for example `main:research/ledger.yaml`. The server
does not infer a default ledger, scan directories, expand shell syntax, or
accept a path outside its configured class. Configured roots may not overlap.
Startup and confinement errors name the root class, originating flag, and safe
route such as `ledger:main`; overlapping-root errors name both routes. Absolute
machine paths are never included in MCP error data.

The default server is read-write and online within its roots. These optional
whole-server modes reduce the surface:

```sh
forecast-ledger mcp serve --ledger-root main=/data/ledgers --read-only
forecast-ledger mcp serve --ledger-root main=/data/ledgers --offline
```

Read-only omits every mutating tool from `tools/list`; a direct call to an
omitted name receives the protocol's unknown-tool response before secret, lock,
file, or network effects. Offline opens no network socket; timestamp stamp is
disabled, while status and verification remain local.

The server accepts at most 16 concurrent tool calls and 8 MiB of decoded
arguments per call by default. Set smaller or larger bounded values at startup
when the client workload needs them:

```sh
forecast-ledger mcp serve \
  --ledger-root main=/data/ledgers \
  --max-concurrent 8 \
  --max-tool-bytes 1048576
```

`--max-concurrent` accepts 1 through 256. `--max-tool-bytes` accepts 1 KiB
through 64 MiB. Calls above either limit fail as recoverable tool errors.

Ledger writers use the same fail-fast lock as the CLI. A second tool call for
the same ledger returns immediate `conflict`; the server timeout does not queue
it behind the first writer. Clients that intentionally contend must serialize
mutations or use their own bounded retry with backoff.

Reveal is the only separate capability boundary. `forecast_reveal` is absent
from discovery unless all three conditions hold: a secret root exists, the
server is not read-only, and startup includes `--allow-reveal`. A reveal call
still needs a protected key reference and `confirm: true`.

`timestamp_stamp` defaults to `auto`, currently the built-in FreeTSA HTTPS
profile and embedded trust. `tsa_provider` may name `auto` or `freetsa`.
Custom `tsa_url` and ledger-relative `ca_bundle` must be supplied together;
custom URLs remain public HTTPS-only and redirects may not change origin. The
server does not accept a proxy or a private, loopback, link-local, or otherwise
non-public timestamp endpoint.
Private seal input and keys are protected-file references under a secret root;
they are never raw tool arguments or resource content.

Expected verification outcomes remain structured results even when their
stable application code makes the tool response recoverable `isError: true`.
For example, `timestamp_stamp` returns a safe authority issue when acquisition
is unavailable; the MCP session stays alive. `timestamp_verify` verifies the
retained request, response, target, and CA bundle without a network call.
`verification_run` and `publication_verify` return `no_evidence` with
`incomplete` when no applicable forecast-evidence layer exists. Neither outcome
is relabeled as a successful proof.

For `publication_verify`, address both the package ledger and manifest through
one named output root, such as `packages:evidence/ledger/ledger.yaml` and
`packages:evidence/manifest.json`. Do not add the package's nested `ledger/` as
a second ledger root; configured root classes must remain non-overlapping.

Addressed redacted resources use the versioned form
`forecast-ledger://v1/<kind>/<root>/<path>`, with optional question and forecast
query fields. Resource reads are local and non-mutating. Supported kinds are
`ledger`, `question`, `forecast`, `target`, `timestamp`, `report`, and `manifest`.

[MCP reference](../reference/mcp.md) · [Security](../security/index.md)
