# CLI reference

<!-- doc-metadata
coverage: current-main
reviewed: 2026-08-30
owner: interface
generated: false
security-critical: false
prerequisites: ../getting-started/index.md
next: generated/index.md
-->

Every ledger leaf defines its own required `--file/-f`; groups do not own that
flag. Read-only `validate`, `status`, `platform list|show`, `question list|show`,
and `forecast list|show` accept `--file -`. Artifact, timestamp, verification,
publication, mutation, and MCP operations require real paths. Stdin provides
only ledger bytes and cannot resolve sibling target or timestamp paths.

## Command surface

| Command | Required selection or destination | Main effect |
| --- | --- | --- |
| `init` | new `--file`, identity and direct authoring flags; conditional protected `--initial-secret-input` and `--key-file` | Create an empty ledger or include platforms, one question, and an optional first forecast. |
| `ledger update` | `--file` plus one or more set/clear flags | Patch allowed root/current-forecaster fields. |
| `validate`, `status` | `--file` | Validate or summarize. |
| `platform add|update` | `--file --platform` plus authoring flags | Add or patch one platform. Add requires `--name` and `--kind`. |
| `platform list|show` | `--file`; show also `--platform` | Read sorted/redacted platform data. |
| `platform remove` | `--file --platform --yes` | Remove only an unreferenced platform. |
| `group add|update|list|show|remove` | `--file` and stable group selector where applicable | Manage question groups; referenced groups cannot be removed. |
| `relationship add|list|show|remove` | `--file` and stable relationship selector | Manage group-membership and conditional relationships. |
| `question add` | `--file --question --revision-id --outcome-kind` plus domain flags; conditional protected `--initial-secret-input` and `--key-file` | Add one question and its first immutable revision, with an optional first forecast. |
| `question revise` | `--file --question --revision-id` plus a complete domain | Append a complete immutable question revision. |
| `question update` | `--file --question` plus tags, notes, or nonterminal status | Patch only question-level metadata. |
| `question list|show` | `--file`; show also `--question` | Read sorted/redacted question data. |
| `question resolve|ambiguous|void|dispute|not-applicable` | `--file --question`, state-specific fields, and `--yes` | Record one typed v2 terminal state while retaining revisions and forecasts. |
| `forecast add` | `--file --question --forecast --question-revision` and one or more representation families | Append a public forecast bound to one immutable revision. |
| `forecast list|show` | `--file --question`; show also `--forecast` | Read append-only/redacted history. |
| `forecast seal` | `--file --question --forecast --question-revision --secret-input --key-file`; optional public time metadata | Append ciphertext after protected key creation while keeping private representations out of argv. |
| `forecast reveal` | `--file --question --forecast --key-file --yes` | Authenticate and disclose a sealed forecast. |
| `forecast key-hint update` | `--file --question --forecast --key-hint` | Replace only the safe logical hint. |
| `forecast withdraw|expire|reaffirm` | `--file --question --forecast --event --effective-at` | Append a forecast activity event. |
| `target build|check` | `--file` plus `--all` or question+forecast | Create or compare canonical target bytes. |
| `timestamp stamp` | `--file --question --forecast`; optional `--tsa-provider`, custom `--tsa-url`+`--ca-bundle`, or `--offline` | Request and locally verify RFC 3161 evidence; omission selects `auto` (currently FreeTSA). |
| `timestamp status|verify` | `--file --question --forecast` | Inspect or locally verify retained RFC 3161 evidence. |
| `verify` | `--file`; optional question+forecast | Run layered evidence checks. |
| `publish build` | `--file --output` | Create a new standalone package. |
| `publish verify` | package ledger `--file` and `--manifest` | Verify the package and retained RFC 3161 evidence locally. |
| `mcp serve` | one or more `--ledger-root name=path` | Serve the shared operations over protocol-clean stdio. |
| `version` | none | Print binary, source, schema, MCP, and RFC 3161 support metadata. |

Mutation and resource-creation leaves provide `--dry-run`. It performs complete
preflight but does not persist files; timestamp dry-runs also skip entropy and
network. Timestamp status and verification are local and need no network mode.
Approval uses an interactive prompt or `--yes`; `--no-input` never prompts.

Ordinary authoring uses leaf-local flags. Repeated simple collections use
repeated flags. Coupled public records use one CSV record per repeat, with the
exact field order shown in leaf help; CSV quoting handles commas inside values.
Patch leaves distinguish omission from explicit `--clear-*`. There is no
generic public document-input mode. Sealed private values, keys, salts, and
credentials are the exception: they use purpose-named protected files or stdin,
never argv or environment variables.

Timestamp flags accept exact RFC 3339 or deterministic local forms such as
`2030-08-10 14:05`, `10 Aug 2030 14:05`, and `August 10 2030`. Local forms use
the ledger `default_timezone`; init uses `--timezone`. Date-only question
boundaries normalize by field policy. A date-only `forecasted_at`,
`recorded_at`, or evidence time is rejected. Skipped or repeated DST wall times
also require an explicit numeric offset. If `--forecasted-at` is omitted for a
new public, sealed, or initial forecast, it defaults to the single operation
time in the ledger timezone; omitted `recorded_at` uses that same instant.

Global result modes are normal human output, `--json`, `--plain`, or `--quiet`.
The last three are mutually exclusive. `--no-color` and `TERM=dumb` disable
decoration. `--timeout` bounds network/wait operations.

It does not turn ledger-lock conflicts into a wait queue. A second writer fails
immediately with exit 5. Callers that intentionally contend must serialize
mutations or implement a bounded retry with backoff.

Question and forecast show output includes business fields and type-aware public
values in normal human and plain modes. Forecast show also exposes safe stored
target/timestamp/timing metadata without network access. Layered verification
prints its complete ordered evidence matrix, including safe evidence values,
without requiring `--verbose`. Plain verification rows are ordered as question
ID, forecast ID, layer, state, comma-separated reason codes, and compact JSON
evidence. Quote RFC 3339 values in maintained YAML input, such as
`recorded_at: "2026-09-01T09:01:00Z"`.

Verification commands emit their complete expected outcome report to stdout
before returning a nonzero semantic exit. `timestamp stamp` uses network exit 8
when every selected built-in authority is unavailable. A built-in invalid
response uses verification exit 6 and commits nothing; custom invalid evidence
is retained as pending. Layered and package verification
use `incomplete`/exit 9 with overall `no_evidence` when no applicable
forecast-evidence layer exists. Passing document, manifest, or file checks
remain visible but do not turn that aggregate into `pass`.

Current-main JSON uses one outcome contract for both adapters. Compared with
v0.6.1, successful local timestamp verification is `timestamp.verified`; a
publication build dry-run is `publication.build.planned`; supported no-op
updates use `ledger.unchanged`, `platform.unchanged`, `question.unchanged`,
`forecast.reveal.unchanged`, or `forecast.key_hint.unchanged`; and a target
integrity failure is `target.failed` with the complete target report. Embedded
timestamp fields are serialized at the same level as `verification`.

`mcp serve` also accepts repeatable named `--output-root` and `--secret-root`,
whole-server `--read-only` and `--offline`, default-off `--allow-reveal`, and
bounded `--max-concurrent` and `--max-tool-bytes` limits. It has no general
write or network grant flags.
Read-only startup omits every mutating tool from discovery; calling an omitted
name returns the MCP unknown-tool protocol response.

## Stable exits

| Exit | Codes |
| --- | --- |
| 0 | complete success |
| 1 | `internal` |
| 2 | `usage` |
| 3 | `invalid_data`, `unsupported_schema_version` |
| 4 | `not_found` |
| 5 | `conflict` |
| 6 | `verification` |
| 7 | `io` |
| 8 | `network`, `network_disabled` |
| 9 | `pending`, `incomplete` |
| 10 | `unavailable` |
| 130 | `interrupted` |

The supported schema is exact. A ledger other than v2.0.0 produces an explicit
warning on stderr, returns `unsupported_schema_version`/exit 3, and stops before
any file, key, artifact, or network side effect.

Exact direct request schemas and operation policies are generated under
[generated interface reference](generated/index.md). Candidate-binary
`--help` is authoritative for flags in an installed release.

[Reference index](index.md) · [MCP reference](mcp.md)
