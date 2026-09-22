# Forecast Ledger v2 data model

Forecast Ledger stores quantitative forecasts made by one forecasting identity.
That identity may be an individual or a team, but one ledger never combines
independent track records.

The JSON Schema defines local structure. `tools/validate.py` is part of the
contract's reference conformance behavior for rules requiring exact arithmetic
or cross-record lookup.

## Design principles

1. Record the exact question before its outcome is known.
2. Bind every forecast to an immutable question revision.
3. Separate the outcome space, its domain, and the forecast representation.
4. Preserve exact probabilities and values as canonical decimal strings.
5. Append forecast updates and lifecycle events; never edit prior statements.
6. Store observed outcomes and evidence, while deriving scores outside the ledger.
7. Keep optional platform/import and relationship features out of the minimal core.

## Root document

| Field | Required | Purpose |
| --- | --- | --- |
| `schema_version` | yes | Exact contract version; this release requires `2.0.1`. |
| `ledger_id` | yes | Stable ID for this forecasting track record. |
| `created_at` | yes | Ledger creation claim. |
| `default_timezone` | yes | IANA timezone for authoring tools. |
| `forecaster` | yes | One individual or team identity. |
| `publication` | no | Git repository and ledger path. |
| `platforms` | yes | Registry referenced by imported provenance. May be empty. |
| `groups` | no | Named question collections. |
| `relationships` | no | Group memberships and conditional dependencies. |
| `questions` | yes | Question records. May be empty. |

`publication` is optional. A private local ledger remains conformant without a
repository URL.

## Questions and immutable revisions

A question contains stable lifecycle state and an ordered, append-only list of
revisions. Each revision contains:

- `id`, `effective_at`, and `recorded_at`;
- title and resolution criteria;
- required `expected_resolution_at`;
- optional `forecasting_opens_at`;
- `outcome_space`;
- `domain`;
- optional platform `provenance`.

`current_revision_id` must reference the last revision. Revision IDs are unique
within the question. `effective_at` must increase strictly and `recorded_at`
cannot move backwards.

A forecast's `question_revision_id` makes its meaning stable. If wording,
criteria, bounds, options, or another semantic field changes, append a new
revision and point new forecasts at it. Do not mutate a revision already used by
a forecast.

`forecasts: []` is valid. This represents a backlog question, a handoff to a
colleague, or a question retained for later work.

## Outcome space and domain

The outcome space states what kind of fact will resolve. The domain defines its
allowed values.

| Outcome kind | Domain |
| --- | --- |
| `binary` | Boolean `true` or `false`. |
| `categorical` | Unordered, versioned option set. |
| `ordinal` | Ordered, versioned option set; array order is semantic. |
| `numeric` | Optional bounds, discreteness, scale, unit, and bin sets. |
| `date` | ISO dates with optional bounds, allowed values or day step, and bins. |
| `datetime` | RFC 3339 instants with optional bounds, allowed values or second step, and bins. |

`outcome_space.kind` and `domain.kind` must match.

### Bounds and openness

Each lower or upper bound contains a value and `inclusive: true|false`. Numeric
bounds use canonical decimal strings, date bounds use `YYYY-MM-DD`, and datetime
bounds use RFC 3339 with an explicit offset or `Z`.

### Discreteness

Numeric, date, and datetime domains declare one values policy:

- `continuous`: every value allowed by the bounds;
- `step`: values aligned to a positive step from an optional origin;
- `allowed_values`: a sorted explicit set.

Numeric steps are canonical decimal strings. Date steps use `P<n>D`; datetime
steps use `PT<n>S`, with a positive integer `n`.

### Scale and unit

`elicitation_scale` is `linear` or `log`. It describes how a numeric forecast
was elicited or displayed; it does not transform stored values. Units may carry
a human name, symbol, and UCUM code.

### Versioned option sets

Categorical and ordinal domains contain:

```json
{
  "option_set": {
    "id": "coalitions",
    "version": 2,
    "options": [
      {"id": "a", "label": "Coalition A"},
      {"id": "b", "label": "Coalition B"}
    ]
  }
}
```

A PMF repeats the exact ID and version. When options change, create a new
question revision and increment the option-set version. Reusing a version for a
different option list is non-conformant operational behavior.

### Bin sets

Bins are a representation partition, not an outcome kind. Numeric, date, and
datetime domains may define versioned bin sets. Bins must be ordered, have no
gaps or overlaps, and exactly one adjacent bin must include a shared boundary.
They describe only the explicit interior bins. A binned forecast separately
records mass below the first bin and above the last bin.

## Exact decimal encoding

JSON floating-point numbers are never used for probabilities or measured
numeric values.

- `0`, `1`, `-12`, `0.62`, and `842.5` are canonical.
- `+1`, `01`, `1.0`, `0.620`, `1e3`, and `-0` are not canonical.
- Probabilities are strings in `[0,1]`, with at most 18 fractional digits.
- Quantile levels and credible-interval coverage are strictly inside `(0,1)`.

This encoding makes equality, summation, hashing, and cross-language exchange
deterministic.

## Forecast representations

A public or revealed forecast contains a non-empty `representations` array. A
forecast may include several compatible views of the same belief, such as a
median, quantiles, and a binned PMF. A representation kind may appear only once
per forecast.

### Scalar probability

Valid only for binary questions. `probability` is the exact probability of
`outcome: true`.

### PMF

Valid for categorical and ordinal questions. Entries must cover every option
exactly once and sum exactly to 1. `option_set_ref` must equal the option set in
the bound question revision.

### Binned PMF

Valid for numeric, date, and datetime questions. Entries cover every bin in the
referenced bin-set version exactly once. The entries plus
`left_tail_probability` plus `right_tail_probability` must sum exactly to 1.

If bins reach a finite domain bound, the corresponding outside tail must be 0.

### Quantiles

Points are ordered by strictly increasing `level`; values must be
non-decreasing. `interpolation` is one of:

- `none`: only supplied quantiles are asserted;
- `linear`: interpolate values linearly between supplied quantiles;
- `step_lower`: use the lower neighboring value;
- `step_upper`: use the upper neighboring value.

Interpolation is meaningful only between supplied points. The contract does not
infer tails outside the first and last quantile.

### CDF

CDF values are strictly increasing and probabilities are non-decreasing.
`interpolation` is `step_right` or `linear`.

`left_tail_probability` is the mass strictly below the first supplied value and
cannot exceed the first CDF probability. `right_tail_probability` is the mass
strictly above the last supplied value, so the last CDF probability plus the
right tail must equal 1.

### Point estimate

`statistic` makes a point value interpretable:

- `mean`
- `median`
- `mode`
- `best_estimate`

The value must belong to the domain. `mean` is only defined for numeric, date,
or datetime outcomes.

### Credible intervals

Each interval records exact `coverage`, lower and upper values, and one of:

- `equal_tailed`
- `central`
- `highest_density`
- `author_selected`

The contract validates bounds and ordering but does not infer a distribution
from intervals alone.

## Forecast lifecycle

`lifecycle_events` is an optional append-only event stream. A forecast begins
active.

- `withdrawn` and `expired` change active to inactive.
- `reaffirmed` changes inactive to active.

Events are ordered by both effective and recording time. They do not overwrite
the original forecast or replace a new forecast update. A changed belief is a
new forecast with `supersedes_forecast_id`.

Lifecycle events are activity metadata outside the immutable
`forecast-envelope/v2` timestamp target. Appending `withdrawn`, `expired`, or
`reaffirmed` changes the derived active state but not the recorded belief,
canonical target bytes, target SHA-256, or previously obtained RFC 3161
evidence.

## Platform provenance

Question revisions, forecasts, and lifecycle events may carry provenance. A
record references a root platform and supplies the platform object ID,
retrieval time, optional remote version/URL/source times, importer name/version,
and optional raw snapshot artifact plus digest.

Source timestamps are claims copied from the platform. They are not trusted
timestamps. A snapshot digest proves equality to retained bytes but not when the
platform served them.

## Relationships

Two relationship types are supported:

- `group_membership`: connects a question to a declared group;
- `conditional`: activates a child question for one typed parent outcome.

Conditional references bind the parent question and its exact revision.
References must exist and conditional edges must be acyclic. A child may resolve
`not_applicable` by naming its conditional relationship when the parent resolves
to a different outcome.

## Resolution

Question status is one of:

- `open`
- `closed`
- `awaiting_resolution`
- `resolved`
- `ambiguous`
- `void`
- `disputed`
- `not_applicable`

The first three statuses have no resolution object. Terminal statuses require a
matching resolution object.

A resolved question binds an exact question revision, stores a typed outcome,
separates `outcome_known_at` from `recorded_at`, and includes at least one
evidence source. The outcome must belong to that revision's domain.

`ambiguous`, `void`, and `disputed` require a reason. `not_applicable` requires
the conditional relationship that made the child inapplicable.

## Integrity and scoring boundary

`integrity` reports whether the canonical `forecast-envelope/v2` target is
unanchored, pending, verified, or failed. A verified record has at least one
verified RFC 3161 timestamp. Multiple TSA receipts are allowed.

The ledger stores source facts needed for scoring but does not standardize one
score. Consumers choose the scoring rule appropriate to the representation and
question domain. A scoring implementation must use the question revision bound
by the forecast, not the question's current revision.

See [cryptographic verification](cryptographic-verification.md) and the
[recording and verification workflows](forecast-verification-workflows.md).
