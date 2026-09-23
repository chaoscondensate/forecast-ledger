# Timestamp forecast and lifecycle targets

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-23
owner: security
generated: false
security-critical: true
prerequisites: build-targets.md
next: verify-evidence.md
-->

RFC 3161 support is experimental. It binds the exact canonical target bytes to
a signed generation time (`gen_time`) from a timestamp authority (TSA). It does
not prove authorship, completeness, forecast truth, outcome truth, or that the
TSA clock was honest.

## Choose the provider mode

The default `auto` mode uses the released provider catalog. The current catalog
contains only FreeTSA at `https://freetsa.org/tsr`, so it makes at most one
request. FreeTSA is best effort: no numeric rate limit, service-level agreement,
independent TSA audit, or long-term availability promise is published in the
reviewed first-party material. See the maintained [provider qualification and
rotation record](../development/rfc3161-providers.md).

Use `--tsa-provider freetsa` to name the same built-in profile explicitly. For
a custom TSA, obtain and retain its PEM trust bundle beside the ledger. Pass
`--tsa-url` and `--ca-bundle` together. Custom URLs remain public HTTPS-only.
The CLI never downloads trust at runtime and never uses operating-system roots.

## Stamp

Stamp consumes an already retained target. Run `target build` first; stamp does
not create or adopt a standalone target.

```sh
forecast-ledger timestamp stamp \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002
```

Named and custom forms are:

```sh
forecast-ledger timestamp stamp \
  --file ledger.yaml --question q-launch --forecast f-launch-002 \
  --tsa-provider freetsa
forecast-ledger timestamp stamp \
  --file ledger.yaml --question q-launch --forecast f-launch-002 \
  --tsa-url https://tsa.example.com/ \
  --ca-bundle trust/tsa-ca.pem
```

Stamp creates a bounded SHA-256 request with a fresh positive nonce and asks
the TSA to include its signing certificate. It stops after the first locally
verified built-in response and retains:

- `proofs/timestamps/f-launch-002/<tsa-id>/request.tsq` — the DER request;
- `proofs/timestamps/f-launch-002/<tsa-id>/response.tsr` — the DER response; and
- `proofs/evidence-index.json` entries for the request, response, and trust; and
- the exact ledger-relative PEM CA bundle under `trust/`. Built-in trust is materialized at
  `trust/rfc3161/<provider>-<sha256>.pem`.

To timestamp an exact activity prefix, first build its checkpoint target with
the explicit checkpoint ID and recording time, then select its head:

```sh
forecast-ledger timestamp stamp \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002 \
  --scope lifecycle \
  --head withdraw-001
```

Lifecycle evidence uses
`proofs/targets/f-launch-002.lifecycle.withdraw-001.json` and
`proofs/timestamps/f-launch-002.lifecycle.withdraw-001/<tsa-id>/...`. A
successful or retained-pending attempt updates the existing exact-head activity
checkpoint through the same atomic journal as its indexed resources. Evidence
for other heads is preserved.

`<tsa-id>` is a stable short digest of the exact TSA URL. A custom URL must be
public HTTPS without credentials, query, fragment, or a non-default port.
Private, loopback, link-local, reserved, and redirect-to-other-origin
destinations are rejected. Built-in profiles have an exact compiled HTTPS or
HTTP transport policy and reject every redirect; the current catalog contains
no HTTP provider. Caller input can never authorize HTTP.

`--dry-run` validates paths and inputs without entropy, network, or writes.
`--offline` fails before a socket because stamp requires the TSA. A transport
failure is a network error, not a failed signature. Automatic and named
built-in failure commits nothing. Custom mode retains a received but
unverifiable response as `pending` for inspection and retry.

Repeat stamp with another TSA URL to keep independent timestamp entries. An
earlier verified entry is preserved. Repeating the same TSA is idempotent only
when retained bytes match exactly; conflicting files are never overwritten.

## Inspect and verify locally

```sh
forecast-ledger timestamp status \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002

forecast-ledger timestamp verify \
  --file ledger.yaml \
  --question q-launch \
  --forecast f-launch-002
```

For lifecycle evidence, add `--scope lifecycle --head <event-id>` to status or
verify. These commands inspect only that checkpoint and its retained local
bytes.

Both commands are local. They read the target, request, response, declared
metadata, and retained CA bundle. Verify checks request/response nonce and
imprint agreement, SHA-256 target binding, CMS signed attributes and signature,
ESS `SigningCertificate` v1 or `SigningCertificateV2`, critical timestamping
EKU, certificate chain at `gen_time`, supported algorithms, and the ledger
metadata. Message imprints remain SHA-256; CMS signer digests may be SHA-256,
SHA-384, or SHA-512. It never contacts
the TSA, a blockchain, a Git host, a system trust service, or a revocation
service.

At least one independently verified response passes timing. The layer fails
only after every applicable entry was completely checked and failed. Pending or
uncheckable entries prevent an all-branches failure claim. For a resolved
forecast, a verified `gen_time` must predate `outcome_known_at` to exclude
hindsight.

The retained bundle makes future verification portable, but RFC 3161 alone is
not long-term validation. The CLI performs no CRL/OCSP check, archive timestamp
renewal, RFC 4998 evidence-record maintenance, or system-root fallback. Preserve
the ledger, target, `.tsq`, `.tsr`, and PEM bytes together.

[Verify ledger evidence](verify-evidence.md)
