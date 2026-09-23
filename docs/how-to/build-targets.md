# Build and check forecast targets

<!-- doc-metadata
coverage: v0.11.0
reviewed: 2026-09-22
owner: interface
generated: false
security-critical: true
prerequisites: manage-public-forecasts.md
next: ../reference/index.md
-->

A target is the deterministic byte sequence that later timestamp evidence
binds. Forecast scope binds immutable forecast content. Lifecycle scope binds
one exact prefix of its activity events. Building a target does not contact a
timestamp authority, but it does atomically retain the target declaration,
target bytes, and `proofs/evidence-index.json` entry.

Build one selected target:

```sh
forecast-ledger target build \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002
```

The command writes canonical RFC 8785 JSON to
`proofs/targets/<forecast-id>.json`, relative to the ledger. It exclusively
creates an absent file, accepts an existing byte-identical file as unchanged,
and refuses different bytes. Use `--all` instead of both selectors to build
every forecast target. The all mode checks every destination before creating
the first artifact.

After an activity event is recorded, build the prefix ending at that event:

```sh
forecast-ledger target build \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002 \
  --scope lifecycle \
  --head withdraw-001 \
  --checkpoint checkpoint-withdraw-001 \
  --recorded-at 2026-09-01T10:15:00+01:00
```

Lifecycle targets use
`proofs/targets/<forecast-id>.lifecycle.<head-event-id>.json`. The
`forecast-lifecycle/v2` bytes contain the forecast and head IDs, the digest of
the unchanged `forecast-envelope/v3`, and every lifecycle event through the
selected head. Later heads get different files; building or retrying one head
does not overwrite another. Lifecycle scope requires the directly authored
question, forecast, head, checkpoint ID, and RFC 3339 recording time; neither
checkpoint field is derived. It cannot be combined with `--all`.

`--dry-run` reconstructs the bytes, checks paths and collisions, and reports
deferred writes without creating directories or files. Target commands require
a real ledger file; `--file -` is not supported because artifacts are resolved
relative to the ledger directory.

On an empty ledger, or one whose questions have no forecasts, `target build
--all` succeeds with an empty target/effect list. It does not create a `proofs`
directory or rewrite the ledger. A specific question/forecast selector still
returns `not_found`.

Check retained target bytes without mutation:

```sh
forecast-ledger target check \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002
```

Add `--scope lifecycle --head withdraw-001` to reconstruct and check a retained
lifecycle target.

Check reconstructs the exact bytes from the ledger and compares the file and
SHA-256 digest. If integrity metadata already names a target, scope,
canonicalization, relative path, algorithm, and digest must also match.

If neither integrity metadata nor the deterministic file exists, check returns
one `not_applicable` row with reason `content.no_retained_target`, the expected
safe path, and guidance to run `target build`. That is a successful observation
that no evidence was retained, not a claim that target bytes passed. A retained
but missing, unreadable, unsafe, or mismatched target remains an error.
`target check --all` reports every forecast in ledger order and continues past
never-built rows. Plain rows use question ID, forecast ID, state, reason codes,
path, expected SHA-256, actual SHA-256 when available, and optional guidance.

The `forecast-envelope/v3` target contains `schema`, a `question` object with
`question.id` and the full bound revision, and the selected public forecast
statement or original sealed commitment. It has no root ledger ID. It excludes
lifecycle events, mutable integrity state, key hint, revealed key and reveal
time, question resolution, and unrelated records. A revealed forecast
continues to use its original sealed target.

The forecast target therefore proves nothing about current activity. A
lifecycle checkpoint can detect deletion, alteration, or reordering inside its
covered prefix. A verified older checkpoint is `partial` after new events are
appended. If every independent observation of an event and its artifacts is
removed, the remaining ledger cannot prove that the deleted event once
existed.

A target proves no authorship or time by itself. It is only deterministic input
for later evidence operations.

[How-to index](index.md) · [Documentation index](../index.md)
