## MODIFIED Requirements

### Requirement: Every non-secret authoring field has a CLI representation
Every `forecast-ledger` command that creates or changes ledger data SHALL
provide flags or a dedicated subcommand for every non-secret field accepted by
that operation's typed application request. The direct CLI route SHALL NOT
require `--input`, an input document, shell-generated JSON/YAML, or manual
editing of the ledger. This requirement SHALL cover `init`, root and platform
changes, question creation and revision, v2 terminal resolution operations,
group and relationship management, public forecast creation, forecast
lifecycle events, non-secret provenance, non-secret seal/reveal metadata, and
key-hint changes, plus any future authoring command. Obsolete v1 question-type,
basis-point, mutable-question, and annul fields or commands SHALL not remain in
help or completion.

#### Scenario: Add a minimal platform from flags
- **WHEN** a user runs `forecast-ledger platform add --file l.yaml --platform metaculus --name Metaculus --kind scoring_platform` against a valid v2 ledger
- **THEN** the command appends a valid platform with ID `metaculus` and does not report `Required flag "input" not set`

#### Scenario: Complete a rich mutation from flags
- **WHEN** a user supplies every applicable non-secret revision, domain, representation, relationship, provenance, lifecycle, or resolution value through documented flags or a dedicated leaf
- **THEN** the command constructs the transport-neutral typed application request and produces the validated v2 ledger state

#### Scenario: Audit catches incomplete flag coverage
- **WHEN** a command input schema gains a non-secret authorable field without a corresponding flag or dedicated subcommand representation
- **THEN** the maintained command-surface test fails before release

## ADDED Requirements

### Requirement: V2 authoring uses explicit semantic selectors
Forecast and resolved-outcome commands SHALL require the question revision they
bind, except when an initial forecast is created atomically with the only new
revision. PMF and binned-PMF requests SHALL require the exact option-set or
bin-set ID and version. Relationship requests SHALL require every referenced
record or revision ID. The CLI MUST NOT infer these semantic selectors from
array positions, timestamps, labels, or the latest record when a user can be
explicit.

#### Scenario: Later forecast omits revision selector
- **WHEN** `forecast add` or `forecast seal` targets an existing question but omits its question-revision selector
- **THEN** usage validation names the missing flag and performs no ledger, key, target, or network effect

#### Scenario: PMF binds its option version
- **WHEN** a user authors a categorical or ordinal PMF
- **THEN** the request includes an explicit option-set ID and version and fails if they do not match the bound revision

### Requirement: Complex v2 collections remain ordinary CLI values
Ordered options, bins, PMF entries, distribution points, intervals, sources,
and similar public collections SHALL use repeatable flags with documented field
groups or dedicated child commands. Their grammar SHALL be unambiguous,
shell-portable, preserve caller order, reject incomplete groups and duplicate
single-value members, and avoid JSON, YAML, delimiter-ambiguous blobs, or
public side-loaded files.

#### Scenario: CDF points preserve order
- **WHEN** a user repeats the documented CDF point flags in valid order
- **THEN** the typed request retains that order and validates each canonical decimal probability and domain value independently

#### Scenario: Nested document is disguised as a flag
- **WHEN** a caller attempts to pass JSON or YAML text as one public option, bin, representation, relationship, provenance, or resolution flag
- **THEN** the adapter rejects it rather than decoding a hidden document mode
