# How the v2 data model fits together

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-22
owner: interface
generated: false
security-critical: false
prerequisites: ../getting-started/create-ledger.md
next: verification-claims.md
-->

Forecast Ledger v2 separates four ideas that older files often mixed together.

## Question and revision

The question ID is the long-lived thread. A revision is one exact version of
its wording, resolution rule, expected time, and domain. Revisions are
immutable and ordered. Every forecast names the revision it answered.

This means correcting a question does not rewrite history. Add a new revision,
then bind new forecasts to it.

## Domain and representation

The domain says which outcomes are legal: binary, categorical, ordinal,
numeric, date, or datetime. It can carry versioned options, bounds, allowed or
stepped values, units, and bins.

A representation says how a forecast expresses uncertainty inside that
domain. V2 supports probability, PMF, binned PMF, quantiles, CDF, point, and
credible intervals. One forecast can use several compatible representations.
All decimal strings are checked exactly; the validator does not round them
through binary floating point.

For PMFs, the option-set version prevents an old distribution from being read
against a later option list. Binned PMFs use the same rule for bin sets. Tail
probabilities say how much mass lies outside the named bins. Quantile and CDF
interpolation fields describe what may be inferred between supplied points;
they do not create extra recorded observations.

## Supersession and activity

`supersedes_forecast_id` links a newer forecast to an earlier forecast in the
same question. It does not delete the earlier record.

Withdraw, expire, and reaffirm are activity events on one forecast. They answer
whether that record is active, which is different from saying that a later
forecast supersedes it.

These mutable activity events are excluded from the immutable forecast target.
They can change current activity without changing previously retained target
bytes or timestamp evidence.

## Conditions and not applicable

A conditional relationship names a parent question revision, one typed parent
outcome, and a child question. Relationships must be acyclic. If the parent
condition does not occur, the child can end as `not_applicable` and must name
that relationship. `ambiguous` and `void` mean different things and must not be
used as substitutes.

## Scoring boundary

The ledger stores domain definitions, forecast representations, outcomes, and
evidence. It does not choose a scoring rule or claim that two representations
are interchangeable. A scoring system must state which compatible
representation, option/bin version, interpolation policy, and outcome it uses.

[Explanation index](index.md) · [Verification claims](verification-claims.md)
