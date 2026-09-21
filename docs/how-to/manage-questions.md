# Manage questions, revisions, and resolutions

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-21
owner: interface
generated: false
security-critical: false
prerequisites: ../getting-started/create-ledger.md
next: manage-public-forecasts.md
-->

Forecast Ledger v2 separates a stable question ID from immutable revisions of
its wording and domain. A new revision is appended; an earlier revision is
never edited or removed.

Create a binary question with its first revision:

```sh
forecast-ledger question add \
  --file ledger.yaml \
  --question q-launch \
  --revision-id qr-launch-1 \
  --effective-at 2026-09-01T08:00:00Z \
  --title "Will the launch happen by the deadline?" \
  --resolution-criteria "Resolve from the operator's public launch record." \
  --expected-resolution-at "15 Jan 2027" \
  --outcome-kind binary
```

The six outcome kinds are `binary`, `categorical`, `ordinal`, `numeric`,
`date`, and `datetime`. Categorical and ordinal domains use a versioned option
set. Numeric, date, and datetime domains can define bounds and continuous,
step, or allowed-values policies. Numeric domains can also define scale, units,
and versioned bins. Run `forecast-ledger question add --help` for the direct
flags and their repeatable CSV field order.

Omit `--initial-forecast` to keep an empty forecast list. A public initial
forecast uses `--initial-*` representation flags. A sealed initial forecast
keeps its representations and private reasoning in protected
`--initial-secret-input` and requires a new `--key-file`.

When wording, resolution criteria, expected time, or domain changes, append a
complete replacement revision:

```sh
forecast-ledger question revise \
  --file ledger.yaml \
  --question q-launch \
  --revision-id qr-launch-2 \
  --effective-at 2026-10-01T08:00:00Z \
  --title "Will the revised product launch by the deadline?" \
  --resolution-criteria "Resolve from the revised product's public record." \
  --expected-resolution-at 2027-02-15T23:59:59Z \
  --outcome-kind binary
```

Later forecasts must explicitly choose `--question-revision`. This prevents a
new revision from reinterpreting an old forecast.

Only question-level metadata and a nonterminal status are mutable in place:

```sh
forecast-ledger question update \
  --file ledger.yaml \
  --question q-launch \
  --status closed \
  --tag product \
  --notes "Ready for resolution review."
```

Title, criteria, times, outcome space, and domain are revision fields and are
not accepted by `question update`.

Groups and typed relationships organize questions without changing them:

```sh
forecast-ledger group add \
  --file ledger.yaml \
  --group launches \
  --title "Product launches"
forecast-ledger relationship add \
  --file ledger.yaml \
  --relationship rel-launches-q-launch \
  --kind group_membership \
  --group launches \
  --question q-launch
```

A `conditional` relationship names a parent question, its exact revision and
outcome, and a child question. Conditional relationships must form an acyclic
graph. A referenced group or relationship cannot be removed.

List or inspect questions locally:

```sh
forecast-ledger question list --file ledger.yaml
forecast-ledger question show --file ledger.yaml --question q-launch
```

To resolve a closed or awaiting-resolution question, bind the outcome to the
exact revision and provide at least one source:

```sh
forecast-ledger question resolve \
  --file ledger.yaml \
  --question q-launch \
  --question-revision qr-launch-2 \
  --outcome-boolean=true \
  --outcome-known-at 2027-01-02T00:00:00Z \
  --source "Official result,https://example.com/result,2027-01-02T00:10:00Z" \
  --yes
```

Use `question ambiguous` when the criteria do not select one outcome,
`question void` when the question itself became invalid, and `question dispute`
to challenge a terminal claim. A conditional child whose parent condition did
not occur uses `question not-applicable` with the relationship ID. These are
distinct v2 terminal states; the removed v1 `question annul` command is not an
alias for them.

Resolution sources document what was consulted. They do not prove that a
source is authoritative or substantively correct.

[How-to index](index.md) · [Documentation index](../index.md)
