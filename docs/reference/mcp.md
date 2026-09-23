# MCP reference

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-23
owner: interface
generated: false
security-critical: true
prerequisites: ../how-to/run-mcp.md
next: generated/index.md
-->

`forecast-ledger mcp serve` uses the official pinned Go SDK and negotiates the
protocol revisions supported by that SDK. Initialization identifies the binary,
source/schema pins, RFC 3161 with SHA-256, access and network modes, and the
timestamp support status.

The server exposes CLI-parity tools named `ledger_init`, `ledger_update`,
`ledger_validate`, `ledger_status`, every `platform_*`, `question_*`, and
`forecast_*` action, `target_build`, `target_check`, `timestamp_stamp`,
`timestamp_status`, `timestamp_verify`,
`verification_run`, `publication_build`, and `publication_verify` when their
required root class and startup capability are available.

Every schema is a closed JSON object. Unknown fields fail before application
work. Every ledger tool requires `file`; selectors match the CLI. Mutations use
`dry_run`, and actions that need approval use `confirm: true`. Expected domain
errors are successful MCP protocol responses with `isError: true` and a stable
application envelope, so one failed call does not terminate the session.
Report-bearing failures retain typed `data`: target mismatch uses
`target.failed`, while an unavailable timestamp authority uses
`timestamp.not_checked`; both set `isError: true`. Fatal failures before a safe
report exists use the ordinary `error` envelope. Status and verification use only retained target,
request, response, and CA-bundle bytes; they open no network connection.
Layered and package verification use overall `no_evidence` with `incomplete`
when no forecast-evidence layer applies.

The outcome code and message come from the same service classifier as CLI
JSON. Publication dry-run returns `publication.build.planned`, supported no-op
mutations return a stable `*.unchanged` code, and successful timestamp
verification returns `timestamp.verified`. Timestamp verification data is flat;
there is no extra wrapper around its embedded artifact fields.

Authoring tools expose their non-secret request fields directly at the tool
root. They do not accept a generic nested request object or a public side-loaded
file. `ledger_init` and `question_add` may omit initial-forecast properties.
A sealed initial forecast uses purpose-named `initial_secret_input_file` and
requires `key_file`; `forecast_seal` similarly uses `secret_input_file`.
Inline secret forecast material is rejected. Each protected document requires
`representations`; `rationale`, `key_factors`, and `comment` are independently
optional and preserve absent versus explicitly empty values.

MCP timestamps remain exact RFC 3339 strings. The service defaults an omitted
`forecasted_at` for public, sealed, and initial forecasts to one captured
operation time formatted in the ledger timezone; omitted `recorded_at` uses the
same instant. This is the same default used by the CLI after deterministic
human-date normalization.

All startup roots use explicit `name=path` syntax. There is no inferred default
root. The default resource limits are 16 concurrent tool calls and 8 MiB of
decoded arguments per call; `--max-concurrent` and `--max-tool-bytes` change
them within the documented bounded ranges.

The generated [tool catalog](generated/mcp-tool-schemas.json),
[operation contracts](generated/operation-contracts.md), and
[request schemas](generated/request-schemas/index.md) are the machine-readable
reference. Startup mode can remove `forecast_reveal` or package tools from
discovery.

`publication_verify` resolves both `file` and `manifest` inside the same named
output-root class, for example `packages:evidence/ledger/ledger.yaml` and
`packages:evidence/manifest.json`. This keeps the package's `ledger/`,
`proofs/`, and `trust/` siblings under one explicit non-overlapping root.

Target and timestamp tools accept closed `scope` values `forecast` or
`lifecycle`. Forecast is the default and rejects `head`. Lifecycle requires one
`head` event ID and does not support target `all`. A lifecycle `target_build`
also requires explicit `checkpoint` and `recorded_at` fields so it can retain
the new checkpoint together with the target and evidence index. The same typed
requests back the CLI and MCP adapters.

`timestamp_stamp` otherwise requires `file`, `question`, and `forecast`.
Omission of TSA fields selects `auto`, currently the one-entry FreeTSA catalog.
Optional `tsa_provider` accepts `auto` or `freetsa`. A custom `tsa_url` and
`ca_bundle` under the managed `trust/` subtree must be supplied together and
remain public HTTPS-only. Before stamping, `target_build` must have retained the
exact target and evidence-index entry. The tool creates an RFC 3161 SHA-256
request and retains all bytes needed for later local verification. Repeating
the call with a different authority appends independent evidence; it does not
replace earlier entries. A lifecycle stamp consumes the retained exact-head
checkpoint; it does not create an implicit checkpoint. MCP resources expose
these artifacts with resource kind `timestamp`.

[Run the MCP server](../how-to/run-mcp.md) · [Reference index](index.md)
