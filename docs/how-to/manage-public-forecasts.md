# Manage public forecasts

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-22
owner: interface
generated: false
security-critical: false
prerequisites: ../getting-started/create-ledger.md
next: ../reference/index.md
-->

Public forecasts are append-only records. Every forecast names the exact
question revision it answers, so later wording changes cannot silently change
the meaning of an earlier prediction. Question and forecast IDs are unique
across the ledger.

Append a binary probability:

```sh
forecast-ledger forecast add \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002 \
  --question-revision qr-launch-1 \
  --forecasted-at "1 Sep 2026 09:00" \
  --recorded-at "1 Sep 2026 09:01" \
  --probability 0.65 \
  --probability-outcome \
  --rationale "Evidence moved slightly in favor." \
  --key-factor "The latest measurement increased." \
  --supersedes-forecast f-launch-001
```

Probabilities are exact decimal strings from `0` to `1`. There are no basis
points and no generic value object. A forecast can carry more than one
compatible representation:

- `--probability` and `--probability-outcome` for a binary probability;
- `--pmf-set id,version` plus repeated `--pmf option-id,probability`;
- `--binned-pmf-set id,version`, repeated `--bin-probability`, and optional
  tail probabilities;
- `--quantile-interpolation` plus repeated `--quantile level,value`;
- `--cdf-interpolation` plus repeated `--cdf value,probability` and optional
  tails;
- `--point statistic,value`; and
- repeated `--credible-interval coverage,kind,lower,upper`.

PMF and binned-PMF input must name the exact option-set or bin-set version from
the bound question revision. CSV values follow normal CSV quoting, so quote a
field when it contains a comma.

`forecasted_at` cannot precede the bound revision's effective time or optional
forecasting opening, and cannot follow `recorded_at`. If both times are omitted,
the command observes the clock once in the ledger timezone. These are
self-reported times, not cryptographic evidence.

Use `--dry-run` to validate without writing. Public authoring is flag-only; a
generic request document is not accepted.

Append activity events without changing forecast content:

```sh
forecast-ledger forecast withdraw \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002 \
  --event event-withdraw-1 \
  --effective-at 2026-09-02T12:00:00Z \
  --reason "Source was corrected"
```

`withdraw`, `expire`, and `reaffirm` append lifecycle events. Supersession and
activity are separate: `supersedes_forecast_id` links forecast history, while
lifecycle events say whether a particular record is currently active.
Lifecycle events are outside the immutable forecast target. Appending one does
not invalidate retained target bytes or timestamp evidence; verification
reports evidence validity and the derived current activity state separately.

List or inspect records locally:

```sh
forecast-ledger forecast list --file ledger.yaml --question q-launch
forecast-ledger forecast show \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002
```

These reads accept `--file -`, make no network request, and never decrypt sealed
content. Stored target and timestamp metadata is reported as retained data, not
as a fresh verification result.

[How-to index](index.md) · [Documentation index](../index.md)
