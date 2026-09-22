## ADDED Requirements

### Requirement: Report current activity separately from recorded-belief evidence
Every selected forecast verification result SHALL include an `activity` layer
that reports the derived active state, event count, last public lifecycle event
summary when present, and lifecycle binding status. Content binding, existence
timing, reveal, outcome evidence, and activity SHALL remain separate layers.
An inactive forecast MUST NOT invalidate otherwise valid recorded-belief
evidence, and valid recorded-belief evidence MUST NOT imply that the forecast
is currently active. Human, plain, JSON, MCP, and package results SHALL expose
equivalent public fields and stable reason codes.

#### Scenario: Verified forecast is withdrawn
- **WHEN** a forecast's immutable target and RFC 3161 timestamp verify but its latest lifecycle event is a withdrawal
- **THEN** content and timing pass while activity reports inactive and independently reports whether the lifecycle history is bound

#### Scenario: Activity exists without retained binding
- **WHEN** a valid event stream derives an inactive forecast but has no retained lifecycle binding
- **THEN** activity reports inactive, marks the history unbound, and does not claim that earlier events could not have been removed

#### Scenario: Activity binding fails
- **WHEN** current lifecycle bytes disagree with complete retained lifecycle evidence
- **THEN** the activity layer fails with a specific binding reason while all independently established forecast evidence remains visible

## MODIFIED Requirements

### Requirement: Reserve pass for applicable evidence
An aggregate verification result SHALL be `pass` only when at least one
forecast-evidence layer or lifecycle-binding layer in the selected scope is
applicable, every applicable evidence layer completed, and none is pending, not
checked, or failed. The derived activity observation by itself, document
validity, manifest integrity, and package-file digest checks SHALL remain
independently visible but SHALL NOT by themselves make an evidence aggregate
pass.

When the selected scope contains no forecasts or every forecast-evidence and
lifecycle-binding layer is `not_applicable`, the aggregate SHALL be
`no_evidence`. This state SHALL use application category `incomplete` and CLI
exit 9, while preserving all independently established document, manifest,
file, and current-activity observations. The state does not claim failure,
corruption, completeness, or missing promised evidence.

Aggregate precedence SHALL be `fail`, then `incomplete` caused by unavailable
or unperformed applicable checks, then `pending`, then `no_evidence`, then
`pass`. A layer that passes SHALL count as applicable; `not_applicable` SHALL
not. An activity layer whose current state is observable but whose lifecycle
binding is absent SHALL not count as applicable evidence.

#### Scenario: Empty ledger verification
- **WHEN** layered verification selects a valid ledger containing no forecasts
- **THEN** it performs no network requests, reports the document result and `overall no_evidence`, and exits 9

#### Scenario: Empty question selection
- **WHEN** layered verification selects a valid question containing no forecasts
- **THEN** it reports `overall no_evidence` rather than `pass` and exits 9

#### Scenario: Forecast with no applicable evidence
- **WHEN** every reported forecast-evidence and lifecycle-binding layer is `not_applicable`
- **THEN** layered or package verification reports `overall no_evidence` and exits 9 while retaining the current activity observation

#### Scenario: Package bytes pass but evidence does not apply
- **WHEN** a package manifest, listed files, and packaged ledger pass their integrity checks but the selected ledger has no applicable forecast-evidence or lifecycle-binding layer
- **THEN** package verification preserves those passing package observations, reports `overall no_evidence`, and does not describe the forecast or lifecycle evidence as verified

#### Scenario: At least one applicable layer passes
- **WHEN** at least one forecast-evidence or lifecycle-binding layer is applicable and passes, all other applicable layers pass, and remaining layers are `not_applicable`
- **THEN** the aggregate is `pass` and exits 0

#### Scenario: Complete lifecycle evidence mismatches
- **WHEN** a lifecycle-binding layer completely evaluates retained evidence and establishes that a covered event or head changed
- **THEN** aggregate verification is `fail`, the application category is `verification`, and independently passing layers remain visible
