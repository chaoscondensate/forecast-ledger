# Seal and reveal forecasts

<!-- doc-metadata
coverage: v0.8.0
reviewed: 2026-08-30
owner: interface
generated: false
security-critical: true
prerequisites: manage-public-forecasts.md
next: build-targets.md
-->

A sealed forecast publishes an encrypted commitment while keeping its value and
explanation private until an approved reveal. Seal always appends a new forecast
ID; it never hides or reseals an existing public record.

Prepare only the private bundle in an owner-only file, or supply it on stdin:

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

```sh
chmod 600 private-forecast.yaml
forecast-ledger forecast seal \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002 \
  --question-revision qr-launch-1 \
  --forecasted-at 2026-09-01T09:00:00+01:00 \
  --recorded-at 2026-09-01T09:01:00+01:00 \
  --supersedes-forecast f-launch-001 \
  --secret-input private-forecast.yaml \
  --key-file f-launch-002.key
```

Public forecast and record times, `--public-note`, and
`--supersedes-forecast` remain ordinary flags. Value, rationale, key factors,
and comment never enter argv. A generic full request document is not accepted.
Only the purpose-named private bundle channel is file/stdin based.

The key destination must be new. On POSIX it is created with mode `0600`; on
Windows it receives an owner-only ACL. The key is made durable before the
ledger is updated. If the later ledger update fails, the only key copy is
retained and recovery output identifies its safe display name. `--dry-run`
validates input and destinations without generating a salt, key, or nonce.

The `forecast-seal/v2` plaintext authenticates the question ID, exact question
revision ID, forecast ID, random salt, representations, rationale, key factors,
and comment. Associated data also authenticates the scheme and commitment
digest. A separate `forecast-envelope/v2` target binds the full question
revision and public forecast record. After reveal publishes the key, anyone
with the retained ciphertext can recover the private bundle.

Reveal only with explicit approval:

```sh
forecast-ledger forecast reveal \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002 \
  --key-file f-launch-002.key \
  --yes
```

Reveal verifies the protected key file, AEAD, commitment digest, protocol,
IDs, exact canonical plaintext, typed mirror, and original sealed target before
changing the ledger. It publishes the representations, rationale, factors,
comment, and key required by schema v2 while retaining ciphertext, commitment, public fields,
integrity, target, and timestamp evidence. Normal and JSON results never print
the key or private fields. Repeating reveal with the correct key is unchanged.
Use `--revealed-at` for an explicit reproducible RFC 3339 time.

The key hint is public, non-authoritative metadata; it never locates or reads a
key. Repair it without changing seal or target bytes:

```sh
forecast-ledger forecast key-hint update \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002 \
  --key-hint forecast-key:f-launch-002
```

Hints use a path-free `scheme:opaque` form. File paths, URLs with authority,
credentials, slashes, backslashes, queries, fragments, and `file:` are rejected.
For `forecast-key`, the opaque value must be the selected forecast ID.

[How-to index](index.md) · [Documentation index](../index.md)
