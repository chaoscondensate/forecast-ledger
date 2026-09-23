# Verify ledger evidence

<!-- doc-metadata
coverage: v0.11.0
reviewed: 2026-09-23
owner: security
generated: false
security-critical: true
prerequisites: timestamp-forecasts.md
next: publish-evidence.md
-->

Run the evidence report:

```sh
forecast-ledger verify --file ledger.yaml
```

Timestamp verification is always local. Network is used only when you add
`--check-sources` for outcome-source reachability and stored-digest checks.
Add `--offline` to prevent those optional requests:

```sh
forecast-ledger verify --file ledger.yaml --check-sources --offline
```

When network checks are enabled, only public HTTPS destinations are eligible.
The client does not use environment proxies and rejects mixed DNS answers or
reserved destinations, including CGNAT, on the connection it actually opens
and again after redirects. A rejected or unavailable source is `not_checked`;
it is not evidence that the recorded outcome is false.

Narrow the report with `--question`; `--forecast` also requires its question.
The report first reconciles ledger declarations, the canonical evidence index,
and managed files, then reports content binding, activity, existence timing,
reveal authentication, and outcome evidence. Human, plain, and JSON output
report these observations separately.

Existence timing rebuilds the exact target and locally verifies every retained
RFC 3161 request, response, and CA bundle. One complete valid entry passes. It
fails only when all entries were completely checked and failed. A missing or
pending entry keeps the layer pending or not checked. If the outcome is known,
at least one verified `gen_time` must be earlier than `outcome_known_at`.
`verified_at`, filesystem time, Git time, and package time are not substitutes.

Withdraw, expire, and reaffirm events affect the forecast's derived active
state, not its immutable `forecast-envelope/v3` target. Existing target and
timestamp verification can therefore still pass after lifecycle activity; the
report presents current activity separately and does not call an inactive
forecast active merely because its evidence passes.

Activity reports the derived active state, event count, last event identity and
times, covered head, and one coverage value:

- `unbound`: no lifecycle checkpoint is declared; this observation cannot make
  the aggregate pass;
- `partial`: verified evidence covers an older prefix and a later event exists;
- `pending`: retained checkpoint evidence is not yet verified;
- `not_checked`: declared local evidence is unavailable or incomplete;
- `verified`: the current event head is covered by locally verified evidence;
  and
- `failed`: a covered prefix, target, timestamp, identity, or checkpoint order
  does not verify.

Each checkpoint is rebuilt through its exact head and checked against its
canonical target, digest, RFC 3161 request and response, metadata, and retained
trust bytes. This detects deletion, alteration, and reordering within a covered
prefix. It cannot prove completeness after every independent observation of a
removed event has also been deleted.

The aggregate precedence is fail, incomplete/not checked, pending, no evidence,
then pass. An indexed missing file is incomplete; a digest, role, binding, or
canonical-byte mismatch fails. A managed file absent from the index is
`evidence.unindexed_artifact` and incomplete.
An index entry with no matching ledger declaration is
`evidence.index_entry_unreferenced`; a fully validated detached lifecycle
target uses the stronger `activity.retained_evidence_unreferenced` failure.
`pass` requires at least one applicable evidence layer. An empty ledger or a
selection with only non-applicable layers returns `no_evidence` and exit 9.
Source unavailability is not a cryptographic proof mismatch.

Exit categories include usage 2; invalid data or unsupported schema 3; not found
4; conflict 5; verification 6; I/O 7; network 8; pending or incomplete 9;
unavailable 10; and interrupted 130. JSON errors remain one stable error value.

Even a complete pass does not prove authorship, that no forecast was omitted,
forecast or outcome truth, TSA clock honesty, current revocation status, or the
exact self-reported forecast time.

[Build and verify an evidence package](publish-evidence.md)
