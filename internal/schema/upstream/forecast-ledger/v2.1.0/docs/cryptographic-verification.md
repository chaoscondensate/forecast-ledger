# Cryptographic verification

This document is normative for `forecast-envelope/v2`, `forecast-lifecycle/v1`,
and `forecast-seal/v2`.
The executable reference is
[`tools/forecast_crypto.py`](../tools/forecast_crypto.py). The same module keeps
the frozen v1 algorithm solely to verify historical `forecast-seal/v1` vectors.

## Security claims and non-claims

The protocol can establish:

- content binding: the timestamp covers one exact forecast and question revision;
- hiding: sealed representations and reasoning remain confidential until reveal;
- reveal binding: a valid key opens the commitment to one canonical plaintext;
- existence timing: a trusted TSA signed the exact envelope digest at RFC 3161 `genTime`;
- portable verification from retained target, request, response, and trust chain.
- lifecycle-prefix integrity when a retained activity checkpoint exists.

It does not prove authorship, ledger completeness, outcome truth, or that a
self-reported `forecasted_at` is exact. Forecast Ledger defines no Git-signature
or per-forecast signature requirement. A commitment, Git commit hash, hosting
account, and RFC 3161 token are not author signatures.

## Canonicalization

All envelope and seal objects are serialized with RFC 8785 JSON Canonicalization
Scheme. Forecast Ledger restricts canonicalized data to:

- strings, booleans, null, arrays, and objects;
- integers within the I-JSON safe range;
- no floating-point JSON numbers;
- no duplicate keys or lone Unicode surrogates.

Probabilities and numeric values are strings. The reference implementation uses
UTF-16 property ordering as required by RFC 8785 and outputs UTF-8 without a
trailing newline.

## `forecast-envelope/v2`

The envelope is the exact object sent to RFC 8785 canonicalization and then
SHA-256. It includes the full bound revision, not only its ID.

### Public envelope

```json
{
  "schema": "forecast-envelope/v2",
  "question": {
    "id": "q-example",
    "revision": {
      "id": "qr-example-1",
      "effective_at": "2026-09-21T10:00:00Z",
      "recorded_at": "2026-09-21T10:00:00Z",
      "title": "Will the event occur?",
      "resolution_criteria": "Resolve YES if ...",
      "expected_resolution_at": "2027-01-01T00:00:00Z",
      "outcome_space": {"kind": "binary"},
      "domain": {"kind": "binary"}
    }
  },
  "forecast": {
    "id": "f-example-1",
    "question_revision_id": "qr-example-1",
    "forecasted_at": "2026-09-21T10:05:00Z",
    "recorded_at": "2026-09-21T10:05:10Z",
    "visibility": "public",
    "representations": [
      {"kind": "probability", "outcome": true, "probability": "0.62"}
    ]
  }
}
```

The builder includes every present immutable forecast statement field supported
by `public_forecast_envelope()`, including reasoning, supersession, and
provenance. It excludes `integrity` to avoid a recursive target. It also excludes
`lifecycle_events`: those append-only events change derived active state, not
the recorded belief. Adding a withdrawal, expiry, or reaffirmation therefore
does not change canonical target bytes, target SHA-256, or existing RFC 3161
evidence.

Binding the complete revision prevents a valid timestamp from being reinterpreted
under later wording, options, bounds, units, bins, or resolution criteria.

### Sealed envelope

The sealed envelope has the same `schema` and full `question`. Its forecast
contains the visible IDs and times plus:

```json
{
  "visibility": "sealed",
  "commitment": {
    "scheme": "forecast-seal/v2",
    "commitment_hash": {"algorithm": "sha-256", "value": "..."},
    "encryption": {
      "algorithm": "chacha20-poly1305",
      "nonce": "...",
      "ciphertext": "..."
    }
  }
}
```

`key_hint` is excluded because it is an operational pointer that may rotate
without changing the sealed claim. `revealed_at` and `revealed_key` are excluded
so rebuilding a revealed forecast produces the original sealed target.
`lifecycle_events` and `activity_checkpoints` are excluded for the same
activity-metadata reason; the original sealed target remains byte-for-byte
reproducible after either is appended.

## `forecast-lifecycle/v1`

A lifecycle checkpoint covers one complete ordered event prefix. Construct this
closed object, canonicalize it with RFC 8785, and hash the exact bytes with
SHA-256:

```json
{
  "schema": "forecast-lifecycle/v1",
  "question_id": "q-example",
  "forecast_id": "f-example-1",
  "forecast_envelope_sha256": {
    "algorithm": "sha-256",
    "value": "64 lowercase hexadecimal characters"
  },
  "lifecycle_events": ["the complete ordered prefix through the named head"],
  "head_event_id": "event-example-withdrawn"
}
```

The envelope digest is computed from the corresponding public or sealed
`forecast-envelope/v2`. Checkpoint objects, integrity records, timestamp
metadata, and events after the head are never included. Timestamp the lifecycle
target through the same RFC 3161 procedure used for an envelope target.

Verification rebuilds the envelope digest and exact covered prefix, compares
canonical bytes and SHA-256, and then verifies retained RFC 3161 evidence. A
later event cannot invalidate an earlier checkpoint. It makes the earlier
checkpoint partial coverage until a new head is checkpointed.

A retained checkpoint detects alteration, deletion, reordering, or binding to
another forecast within its prefix. No protocol can prove a deleted event once
every event record, checkpoint, target artifact, evidence package, and external
copy has also been deleted.

## `forecast-seal/v2`

### Inputs

Generate independently for every forecast:

- 32 random salt bytes from a CSPRNG;
- 32 random ChaCha20-Poly1305 key bytes from a CSPRNG;
- 12 nonce bytes, unique for that key;
- a non-secret `key_hint`.

Never reuse a `(key, nonce)` pair. A fresh key per forecast is recommended.

The closed private bundle permits exactly four fields. Only non-empty
`representations` is required; the other fields are independently optional:

```json
{
  "representations": ["..."],
  "rationale": "...",
  "key_factors": ["..."],
  "comment": "..."
}
```

Absence is authenticated and differs from an explicit empty string or empty
`key_factors` array. Unknown properties are forbidden.

### Step 1: canonical plaintext

Construct this object and canonicalize it:

```json
{
  "schema": "forecast-seal/v2",
  "question_id": "q-example",
  "question_revision_id": "qr-example-1",
  "forecast_id": "f-example-1",
  "bundle": {"...": "..."},
  "salt": "64 lowercase hexadecimal characters"
}
```

The salt is inside canonical JSON, so there is no delimiter ambiguity.

### Step 2: commitment

```text
commitment_hash = SHA-256(canonical_plaintext)
```

### Step 3: associated data

Construct and canonicalize:

```json
{
  "scheme": "forecast-seal/v2",
  "question_id": "q-example",
  "question_revision_id": "qr-example-1",
  "forecast_id": "f-example-1",
  "commitment_sha256": "64 lowercase hexadecimal characters"
}
```

The question revision ID is repeated in the payload and associated data. A
mismatch is always an error. Visible forecast times are independently bound by
the timestamped `forecast-envelope/v2`.

### Step 4: encryption

Encrypt the canonical plaintext using ChaCha20-Poly1305 with the 32-byte key,
12-byte nonce, and canonical associated data. Publish the scheme, commitment,
algorithm, standard base64 nonce/ciphertext, and `key_hint`. Keep the key and
plaintext secret.

### Step 5: external timestamp

Create the full sealed `forecast-envelope/v2`, canonicalize it, compute its
SHA-256 digest, and request RFC 3161 timestamps for those exact bytes. Do not
timestamp ciphertext alone.

## Reveal verification order

When publishing a reveal, retain the original commitment, nonce, and ciphertext;
add the key and exact plaintext mirror. A verifier performs these operations in
order:

1. check key length and decode nonce/ciphertext;
2. reconstruct associated data from ledger IDs and the commitment hash;
3. authenticate and decrypt ChaCha20-Poly1305;
4. compute SHA-256 over the decrypted bytes and compare it to the commitment;
5. parse JSON and require `schema: forecast-seal/v2`;
6. compare question, revision, and forecast IDs;
7. recanonicalize and require byte-for-byte equality with decrypted bytes;
8. require `representations` and compare it with the decrypted bundle;
9. require each optional private field to be present if and only if it was
   present in the authenticated bundle, then compare its exact value;
10. rebuild the sealed envelope and compare it with the timestamped target;
11. independently verify at least one RFC 3161 response.

Failure at any step invalidates the reveal or timing claim; do not continue and
report a generic success.

## RFC 3161 timestamp procedure

Build targets:

```bash
python tools/build_targets.py ledger.yaml --output proofs/targets
```

Create a request over one target:

```bash
mkdir -p proofs/timestamps
openssl ts -query \
  -data proofs/targets/f-example-1.json \
  -sha256 -cert \
  -out proofs/timestamps/f-example-1.tsa-a.tsq
```

Send the binary request and retain the binary response:

```bash
curl --fail --silent --show-error \
  -H "Content-Type: application/timestamp-query" \
  --data-binary @proofs/timestamps/f-example-1.tsa-a.tsq \
  "$TSA_URL" \
  --output proofs/timestamps/f-example-1.tsa-a.tsr
```

Verify both bindings with a retained CA bundle:

```bash
openssl ts -verify \
  -queryfile proofs/timestamps/f-example-1.tsa-a.tsq \
  -in proofs/timestamps/f-example-1.tsa-a.tsr \
  -CAfile proofs/timestamps/tsa-a-ca.pem
openssl ts -verify \
  -data proofs/targets/f-example-1.json \
  -in proofs/timestamps/f-example-1.tsa-a.tsr \
  -CAfile proofs/timestamps/tsa-a-ca.pem
openssl ts -reply -in proofs/timestamps/f-example-1.tsa-a.tsr -text
```

The request check covers the saved nonce and requested imprint. The data check
covers the exact envelope bytes. Record the TSA URL, request, response, CA
bundle, `gen_time`, policy OID, and serial number only after verification.

### Redundant TSAs

Append one `timestamps` item per TSA. Each item has its own request, response,
URL, trust bundle, and signed metadata. The ledger remains verifiable when one
service disappears if the verifier still has:

- the exact target;
- a valid response from another trusted TSA;
- that response's matching request and retained trust chain.

No live TSA server is needed for verification. Replicate artifacts and trust
bundles outside the repository because service availability and certificate
distribution can change.

### Chronology rule

For a resolved question, at least one verified RFC 3161 `gen_time` must be
strictly earlier than `resolution.outcome_known_at`. A later token may still
prove integrity from that later time, but it does not exclude hindsight.

The semantic validator checks declared digests and chronology. It does not parse
or cryptographically verify `.tsr` files; independent verification must run the
OpenSSL checks above.

## Protocol exclusions

- OpenTimestamps is not accepted by the v2 schema.
- TLS/HTTPS transport does not timestamp a forecast.
- Git commit times are not trusted external timestamps.
- Signed Git commits are not required and do not replace RFC 3161.
- A platform's `source_created_at` or `source_updated_at` is provenance, not a
  cryptographic time claim.

## Test vectors and frozen release bytes

Verify seal and target vectors:

```bash
python tools/forecast_crypto.py verify-vector tests/vectors/forecast-seal-v1.json
python tools/forecast_crypto.py verify-vector tests/vectors/forecast-seal-v2.json
python tools/forecast_crypto.py verify-target-vector tests/vectors/forecast-envelope-v2-public-lifecycle.json
python tools/forecast_crypto.py verify-target-vector tests/vectors/forecast-envelope-v2-sealed-lifecycle.json
python tools/forecast_crypto.py verify-lifecycle-vector tests/vectors/forecast-lifecycle-v1.json
python tools/forecast_crypto.py verify-presence-vector tests/vectors/forecast-seal-v2-presence.json
python tools/run_target_tests.py
python tools/run_seal_tests.py
python tools/run_diagnostic_tests.py
python tools/verify_legacy.py
```

The v2 seal and presence vectors fix every seal input, including salt, key, and
nonce, for all eight optional-field presence combinations and explicit empty
values. The envelope vectors fix exact public and sealed bytes while carrying
lifecycle events that must not appear in those bytes. The lifecycle corpus fixes
exact withdrawal and reaffirmation prefixes and deterministic candidates for
prefix alteration, event deletion, reordered events, and wrong forecast
binding. Reference builders reproduce every published byte string and digest.

Frozen v1.3.0, v2.0.0, and v2.0.1 artifacts must remain byte-for-byte equal to
their tagged files. `verify_legacy.py` enforces their SHA-256 manifests.

## External references

- [RFC 8785: JSON Canonicalization Scheme](https://www.rfc-editor.org/rfc/rfc8785)
- [RFC 3161: Time-Stamp Protocol](https://www.rfc-editor.org/rfc/rfc3161)
- [OpenSSL `ts`](https://docs.openssl.org/master/man1/openssl-ts/)
