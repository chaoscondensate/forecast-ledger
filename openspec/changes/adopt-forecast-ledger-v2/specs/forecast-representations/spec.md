## Purpose

Defines exact v2 forecast representations and their domain-specific invariants
for public authoring, protected sealing, display, and semantic validation.

## ADDED Requirements

### Requirement: Probabilities and numeric scalars use canonical decimal strings
Probability, quantile level, coverage, and numeric outcome fields SHALL use the
v2 canonical decimal-string grammar. Probabilities SHALL be within `[0,1]`
with at most 18 fractional digits; quantile levels and interval coverage SHALL
be strictly inside `(0,1)`. JSON numbers, exponent notation, leading plus or
zero, trailing fractional zero, and negative zero SHALL be rejected where a
canonical decimal string is required. Exact sums SHALL use decimal arithmetic,
not binary floating point or basis-point rounding.

#### Scenario: Exact precision is preserved
- **WHEN** a forecast supplies probability `"0.123456789012345678"`
- **THEN** authoring, validation, YAML/JSON round-trip, display, sealing, and target generation preserve the exact string

#### Scenario: Legacy basis points are not accepted
- **WHEN** a v2 public request supplies `probability_bp` or a JSON number instead of a canonical probability string
- **THEN** the closed request or ledger schema rejects it without conversion

### Requirement: Representation kinds match the bound domain
A public or revealed forecast SHALL contain a non-empty `representations` array
with at most one representation of each kind. Binary domains SHALL accept
probability and point; categorical and ordinal domains SHALL accept PMF and
point; numeric, date, and datetime domains SHALL accept binned PMF, quantiles,
CDF, point, and credible intervals. The system MUST NOT infer or silently
convert a representation that the caller did not provide.

#### Scenario: Several compatible views are retained
- **WHEN** a numeric forecast provides a point, quantiles, and credible intervals
- **THEN** all three remain in caller order, retain distinct semantics, and validate against the same bound revision

#### Scenario: Representation is incompatible with domain
- **WHEN** a categorical forecast supplies a CDF or a binary forecast supplies a PMF
- **THEN** semantic validation rejects the incompatible representation

### Requirement: PMF and binned PMF coverage is complete and exact
A PMF SHALL bind the exact option-set ID and version, cover each option exactly
once, contain no duplicate option ID, and sum exactly to `1`. A binned PMF SHALL
bind one existing bin-set ID and version, cover each bin exactly once, and sum
its entries plus explicit left and right tail probabilities exactly to `1`.
An outside tail SHALL be zero when the bins reach the corresponding finite
domain bound.

#### Scenario: Option-set version does not match
- **WHEN** a PMF references a different version than the forecast's bound revision
- **THEN** validation rejects it even if the option IDs happen to be the same

#### Scenario: Binned total includes tails
- **WHEN** bin entries alone sum to one but a non-zero tail is also present
- **THEN** validation rejects the representation because entries and tails do not sum exactly to one

### Requirement: Distribution and summary invariants are explicit
Quantile levels SHALL increase strictly and values SHALL be non-decreasing in
the domain, with interpolation `none`, `linear`, `step_lower`, or `step_upper`.
CDF values SHALL increase strictly, probabilities SHALL be non-decreasing,
left tail SHALL not exceed the first CDF probability, and the final CDF
probability plus right tail SHALL equal one, with interpolation `step_right` or
`linear`. Points SHALL name `mean`, `median`, `mode`, or `best_estimate` and
belong to the domain; `mean` SHALL be limited to numeric, date, or datetime.
Credible intervals SHALL have unique coverage, ordered in-domain endpoints,
and an explicit interval semantic.

#### Scenario: CDF is not monotonic
- **WHEN** CDF outcomes repeat or decrease, probabilities decrease, or final probability and tail do not total one
- **THEN** validation rejects the exact offending representation

#### Scenario: Interval meaning is retained
- **WHEN** a caller authors an equal-tailed, central, highest-density, or author-selected interval
- **THEN** the selected meaning and exact coverage are retained and no distribution is inferred from it

### Requirement: Direct and protected representation authoring preserve parity
CLI leaves and MCP tools SHALL expose typed direct fields for every non-secret
representation component and SHALL preserve ordered collections without JSON
or YAML encoded inside a public string. Public requests SHALL carry the
representations directly. Sealed requests SHALL keep representations and
private reasoning only in the purpose-named protected input while exposing IDs,
times, revision binding, supersession, public note, key hint, and output path
through ordinary direct fields.

#### Scenario: CLI and MCP author the same CDF
- **WHEN** equivalent ordered CDF points, tails, interpolation, revision ID, and metadata are supplied through CLI flags and direct MCP properties
- **THEN** both adapters produce semantically identical v2 forecasts and stable results

#### Scenario: Sealed representation is not disclosed
- **WHEN** a sealed forecast containing several representations is created or fails validation
- **THEN** no representation, rationale, key factor, comment, key, salt, or plaintext appears in argv, environment, logs, errors, JSON output, MCP resources, or normal stdout
