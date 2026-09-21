# Create a ledger

<!-- doc-metadata
coverage: v0.9.0
reviewed: 2026-09-21
owner: interface
generated: false
security-critical: true
prerequisites: install.md
next: ../how-to/index.md
-->

`forecast-ledger init` creates one new JSON or YAML ledger. It never overwrites
an existing file and makes no network request. Forecast Ledger schema v2.0.0
allows an empty question list. Ordinary authoring uses flags only.

Create an empty ledger first:

```sh
forecast-ledger init \
  --file ledger.yaml \
  --ledger-id my-forecasts \
  --timezone Europe/London \
  --forecaster-id me \
  --forecaster-name "My Name"
```

Add a backlog question and, later, its first forecast directly from flags:

```sh
forecast-ledger question add \
  --file ledger.yaml \
  --question q-example \
  --revision-id qr-example-1 \
  --title "Will the named event happen by the deadline?" \
  --resolution-criteria "Resolve from the named public source." \
  --created-at "26 Aug 2026 12:00" \
  --expected-resolution-at "15 Jan 2027" \
  --outcome-kind binary
forecast-ledger forecast add \
  --file ledger.yaml \
  --question q-example \
  --forecast f-example-001 \
  --question-revision qr-example-1 \
  --probability 0.65 \
  --probability-outcome
```

The forecast flags are shown in [Manage public forecasts](../how-to/manage-public-forecasts.md).
The first forecast has no implicit `supersedes_forecast_id`. If that field is
supplied, it must name an existing forecast in the same question.

Use `--dry-run` to validate all supplied flags and destinations without
writing. Generic public JSON/YAML request documents are not accepted. The
ledger destination itself never accepts `-`.

Application-written YAML keeps every populated mapping and sequence in expanded
block style for review. Explicit empty collections remain compact as `[]` or
`{}`. Mutations preserve unrelated comments, ordering, scalar style, and the
source newline convention.

Init observes the local operation clock once. An omitted ledger, supplied
question, or supplied initial-forecast time uses that observation where the
schema allows a default.
An explicit ledger `created_at` is never copied into an omitted question
`created_at`, initial `forecasted_at`, or initial `recorded_at`; those remain independent facts. If you
need reproducible historical import times, provide each timestamp explicitly.
Equality at inclusive forecast-window and recorded-time boundaries is valid.

For a sealed first forecast, keep only the private bundle in an owner-only file:

```yaml
representations:
  - kind: probability
    outcome: true
    probability: "0.65"
rationale: Private reasoning.
key_factors:
  - A private observation.
comment: Private working note.
```

Supply public question and forecast metadata as flags, plus the protected input
and an unused key destination:

```sh
forecast-ledger init \
  --file ledger.yaml \
  --ledger-id my-forecasts \
  --timezone Europe/London \
  --forecaster-id me \
  --forecaster-name "My Name" \
  --question q-example \
  --question-revision-id qr-example-1 \
  --question-title "Will the named event happen?" \
  --question-resolution-criteria "Resolve from the named public source." \
  --question-expected-resolution-at 2027-01-15T12:00:00Z \
  --question-outcome-kind binary \
  --initial-forecast f-example-001 \
  --initial-visibility sealed \
  --initial-forecasted-at 2026-09-01T09:00:00+01:00 \
  --initial-secret-input private-forecast.yaml \
  --key-file f-example-001.key
```

Treat the complete sealed input as private. The command creates the protected
key first with owner-only access, then creates the public ledger. It never puts
the key path or private forecast fields in the ledger or normal output. If the
ledger cannot be created after the key is durable, the key is retained and the
error returns a safe recovery instruction. Keep the key outside publication
packages and source repositories.

Initialization supports `binary`, `categorical`, `ordinal`, `numeric`, `date`,
and `datetime` questions. IDs are global stable slugs. Probabilities and numeric
values use exact canonical decimal strings, never binary floating point. CLI timestamp flags accept RFC 3339,
ISO local date/time, or an English month date such as `10 Aug 2030`. Local
forms use `--timezone`; ambiguous DST wall times require an explicit offset.

## Update current metadata

Set or clear only the fields to change:

```sh
forecast-ledger ledger update \
  --file ledger.yaml \
  --title "Updated ledger title" \
  --clear-description \
  --forecaster-name "Current display name"
```

Omitted fields stay unchanged. `--clear-*` removes only optional title,
description, contact, profiles, or members. A switch to a team must
set `kind: team` and at least two unique members in the same patch; a switch to
an individual must set `kind: individual` and `members: null` together. Ledger,
forecaster, question, and forecast IDs are immutable here. Forecast Ledger v2
stores only current forecaster metadata, has no internal identity history, and
does not prove authorship.

[Getting started index](index.md) · [Documentation index](../index.md)
