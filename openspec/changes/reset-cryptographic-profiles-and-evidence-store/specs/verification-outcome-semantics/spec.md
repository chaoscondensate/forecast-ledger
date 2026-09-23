## MODIFIED Requirements

### Requirement: Reserve pass for applicable evidence
An aggregate verification result SHALL be `pass` only when local evidence-store
reconciliation has completed for the selected scope, at least one
forecast-evidence layer is applicable, every applicable layer completed, and
none is pending, not checked, failed, unindexed, or unreferenced. Document
validity, manifest integrity, index syntax, and package-file digest checks SHALL
remain independently visible but SHALL NOT by themselves make an evidence
aggregate pass.

When the selected scope contains no forecasts, or every forecast-evidence layer
is `not_applicable` and reconciliation found no declared, indexed, or managed
evidence, the aggregate SHALL be `no_evidence`. This state SHALL use application
category `incomplete` and CLI exit 9, while preserving all independently
established document, manifest, index, and file observations. The state does not
claim failure, corruption, completeness, or absence of historical evidence.

A valid indexed lifecycle target whose declaration or covered event was removed
SHALL make the activity layer and aggregate `fail` with application category
`verification` and CLI exit 6. Missing indexed bytes, a missing required index,
or an unindexed managed file SHALL make the aggregate incomplete rather than
pass. Aggregate precedence SHALL be `fail`, then `incomplete` caused by
unavailable or unreconciled applicable evidence, then `pending`, then
`no_evidence`, then `pass`. A layer that passes SHALL count as applicable;
`not_applicable` SHALL not.

#### Scenario: Empty ledger verification
- **WHEN** layered verification selects a valid ledger containing no forecasts and no managed evidence
- **THEN** it performs no network requests, reports the document result and `overall no_evidence`, and exits 9

#### Scenario: Empty question selection
- **WHEN** layered verification selects a valid question containing no forecasts and reconciliation finds no evidence for that selection
- **THEN** it reports `overall no_evidence` rather than `pass` and exits 9

#### Scenario: Forecast with no applicable evidence
- **WHEN** every reported forecast-evidence layer is `not_applicable` and the reconciled evidence store contains no entry for the selection
- **THEN** layered or package verification reports `overall no_evidence` and exits 9

#### Scenario: Package bytes pass but evidence does not apply
- **WHEN** a package manifest, index, listed files, and packaged ledger pass their integrity checks but the selected ledger has no applicable forecast-evidence layer
- **THEN** package verification preserves those passing package observations, reports `overall no_evidence`, and does not describe the forecast evidence as verified

#### Scenario: Retained lifecycle evidence lost its declaration
- **WHEN** reconciliation verifies an indexed lifecycle target but finds no matching ledger event and checkpoint declaration
- **THEN** activity and aggregate verification fail with `activity.retained_evidence_unreferenced` and exit 6

#### Scenario: Managed evidence cannot be fully reconciled
- **WHEN** an indexed artifact is unavailable or a managed artifact is absent from the required index
- **THEN** the aggregate is incomplete, does not pass, and preserves a stable reason identifying the unavailable reconciliation step

#### Scenario: At least one applicable layer passes
- **WHEN** reconciliation passes, at least one forecast-evidence layer is applicable and passes, all other applicable layers pass, and remaining layers are `not_applicable`
- **THEN** the aggregate is `pass` and exits 0
