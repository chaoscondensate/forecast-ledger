# Forecast recording and verification workflows

This guide gives the required call order for Forecast Ledger v2. Byte formats
and algorithms are normative in
[cryptographic-verification.md](cryptographic-verification.md).

## Separate evidence claims

| Mechanism | Establishes | Does not establish |
| --- | --- | --- |
| Schema plus semantic validator | Internal conformance | Historical existence or truth |
| SHA-256 target digest | Equality to retained target bytes | When they existed |
| RFC 3161 response | A TSA signed the target imprint at `genTime` | Authorship or outcome truth |
| `forecast-seal/v2` | Hidden content opens uniquely and authentically | Existence before timestamping |
| Git history | Repository publication sequence | Independent trusted time |
| Resolution sources | Evidence for an outcome | Forecast timing |

Report content integrity, existence timing, reveal validity, and outcome
verification separately. Do not collapse them into one `verified` label.

## Required retained artifacts

For every timestamped forecast retain:

- ledger file at the containing Git commit;
- canonical `proofs/targets/<forecast-id>.json`;
- `.tsq` and `.tsr` for every TSA;
- CA bundle required to validate each response;
- for sealed forecasts, original commitment, nonce, and ciphertext;
- after reveal, disclosed key and exact public plaintext mirror.

The verifier does not need a live TSA. It does need the original evidence files
and an acceptable retained trust chain.

## A. Create or revise a question

1. Assign a stable question ID.
2. Create a revision with a unique ID and exact effective/recording times.
3. Store the title, resolution criteria, and expected resolution time.
4. Declare matching outcome-space and domain kinds.
5. Version option or bin sets when present.
6. If imported, record platform provenance and preferably retain a digested raw
   snapshot.
7. Set `current_revision_id` to the final revision.
8. Validate before adding forecasts.

When semantics change, append a revision. Do not edit an already referenced
revision. A question may remain with `forecasts: []` indefinitely.

## B. Record and timestamp a public forecast

### 1. Append the forecast

Bind `question_revision_id`, add one or more compatible representations, and
record `forecasted_at` and `recorded_at`. Use a new forecast ID and
`supersedes_forecast_id` for a changed belief.

Start with:

```yaml
integrity:
  status: unanchored
```

### 2. Validate

```bash
python tools/validate.py ledger.yaml
```

Stop if validation fails.

### 3. Build the canonical target

```bash
python tools/build_targets.py ledger.yaml --output proofs/targets
```

The builder resolves `question_revision_id`, embeds that entire revision in
`forecast-envelope/v2`, canonicalizes it, writes the exact bytes, and prints the
SHA-256 digest.

### 4. Record pending integrity

Copy the reported path and digest into the forecast:

```yaml
integrity:
  status: pending
  target:
    scope: forecast-envelope/v2
    canonicalization: RFC8785
    artifact_path: proofs/targets/<forecast-id>.json
    digest:
      algorithm: sha-256
      value: <64-lowercase-hex>
  timestamps:
    - type: rfc3161
      request_path: proofs/timestamps/<forecast-id>.tsa-a.tsq
      response_path: proofs/timestamps/<forecast-id>.tsa-a.tsr
      tsa_url: https://tsa-a.example/
      hash_algorithm: sha256
      state: pending
```

`integrity` is excluded from the target, so adding this block does not create a
recursive hash.

`lifecycle_events` is also excluded. Appending a withdrawal, expiry, or
reaffirmation changes derived activity state but does not require rebuilding or
retimestamping the recorded belief.

### 5. Request one or more RFC 3161 timestamps

For each TSA:

```bash
openssl ts -query \
  -data proofs/targets/<forecast-id>.json \
  -sha256 -cert \
  -out proofs/timestamps/<forecast-id>.tsa-a.tsq

curl --fail --silent --show-error \
  -H "Content-Type: application/timestamp-query" \
  --data-binary @proofs/timestamps/<forecast-id>.tsa-a.tsq \
  "$TSA_A_URL" \
  --output proofs/timestamps/<forecast-id>.tsa-a.tsr
```

Multiple TSA entries are independent. A failure at one service does not
invalidate another service's valid response.

### 6. Verify every response before marking it verified

```bash
openssl ts -verify \
  -queryfile proofs/timestamps/<forecast-id>.tsa-a.tsq \
  -in proofs/timestamps/<forecast-id>.tsa-a.tsr \
  -CAfile proofs/timestamps/tsa-a-ca.pem
openssl ts -verify \
  -data proofs/targets/<forecast-id>.json \
  -in proofs/timestamps/<forecast-id>.tsa-a.tsr \
  -CAfile proofs/timestamps/tsa-a-ca.pem
openssl ts -reply -in proofs/timestamps/<forecast-id>.tsa-a.tsr -text
```

Only after both verification commands succeed, copy `genTime`, policy OID, and
serial number; set the timestamp entry to `verified`; add its CA bundle path;
and set integrity to `verified` with `verified_at`.

### 7. Revalidate and publish atomically

```bash
python tools/validate.py ledger.yaml
git add ledger.yaml proofs/targets/<forecast-id>.json proofs/timestamps/
git commit -m "Record forecast <forecast-id>"
git push
```

The RFC 3161 response, not the Git commit time, supplies the external timing
claim.

## C. Record a sealed forecast

### 1. Prepare the private bundle

```python
from secrets import token_bytes

from tools.forecast_crypto import SEAL_SCHEME_V2, seal_forecast

key = token_bytes(32)
commitment, canonical_plaintext = seal_forecast(
    scheme=SEAL_SCHEME_V2,
    question_id="q-example",
    question_revision_id="qr-example-1",
    forecast_id="f-example-1",
    bundle={
        "question_revision_id": "qr-example-1",
        "forecasted_at": "2026-09-21T10:05:00Z",
        "recorded_at": "2026-09-21T10:05:10Z",
        "representations": [
            {"kind": "probability", "outcome": True, "probability": "0.62"}
        ],
        "rationale": "...",
        "key_factors": ["..."],
        "comment": "Reveal after resolution.",
    },
    salt=token_bytes(32),
    key=key,
    nonce=token_bytes(12),
    key_hint="secret-manager://forecast-ledger/f-example-1",
)
```

Store `key` in a secret manager before publishing. Never commit
`canonical_plaintext`, salt, or key. Never reuse a `(key, nonce)` pair.

### 2. Publish only the seal

Create a ledger forecast with `visibility: sealed`, the visible IDs/times,
returned commitment, and no `representations`, `rationale`, `key_factors`, or
`comment`.

### 3. Follow the public timestamp workflow

Run B.2 through B.7 unchanged. The target contains the full question revision
and the complete public seal inputs.

## D. Reveal a sealed forecast

### 1. Test the key before changing the ledger

```python
from tools.forecast_crypto import reveal_forecast

payload = reveal_forecast(
    question_id="q-example",
    question_revision_id="qr-example-1",
    forecast_id="f-example-1",
    commitment=commitment,
    key=bytes.fromhex(revealed_key_hex),
)
```

Stop if authentication, commitment, canonicalization, or ID binding fails.

### 2. Publish the exact mirror

Change visibility to `revealed`, retain the original cryptographic fields, and
add `revealed_at`, `revealed_key`, and the exact bundle fields. Do not edit the
decrypted values for presentation.

### 3. Validate and compare the target

```bash
python tools/validate.py ledger.yaml
python tools/build_targets.py ledger.yaml --output proofs/rebuilt-targets
cmp proofs/targets/<forecast-id>.json proofs/rebuilt-targets/<forecast-id>.json
```

The rebuilt revealed target must equal the original sealed target byte for
byte. A reveal does not need a new timestamp.

### 4. Commit the reveal

Commit the updated ledger. Keep the earlier sealed commit and all timestamp
evidence.

## E. Import a platform forecast safely

1. Fetch the question and forecast through an authenticated or documented API.
2. Retain the unmodified response as a repository-relative artifact when lawful.
3. Hash the artifact and record `snapshot.artifact_path`, media type, and digest.
4. Record platform slug, remote object ID and version, URL, source timestamps,
   retrieval time, and importer name/version.
5. Map the remote question into one immutable revision.
6. Map probabilities into exact canonical strings without rounding beyond the
   source precision.
7. Bind the exact option/bin-set version.
8. Validate and report any lossy mapping instead of silently coercing it.
9. Timestamp the resulting Forecast Ledger envelope if an independent existence
   claim is required.

Platform timestamps remain provenance claims even when transported over TLS.

## F. Append lifecycle events

Append `withdrawn` or `expired` only while the forecast is active. Append
`reaffirmed` only while inactive. Keep effective and recording timestamps
non-decreasing. A lifecycle event changes whether a forecast is active; it does
not change the recorded belief and is outside the immutable timestamp target.
The original target and RFC 3161 evidence remain valid after the event.

## G. Resolve a question

1. Apply the resolution criteria from the exact revision being resolved.
2. Record a typed outcome belonging to that revision's domain.
3. Record when the outcome became knowable and when it was written.
4. Retain at least one independently inspectable source.
5. Use `ambiguous`, `void`, or `disputed` with a reason when no unique valid
   outcome is available.
6. Use `not_applicable` only for a conditional child whose parent outcome did
   not satisfy the named relationship.
7. Validate. For verified forecasts, the validator requires at least one TSA
   `gen_time` before `outcome_known_at`.

## H. Independent verification

Start from an exact tag or Git commit, never a moving branch.

1. Check out the containing commit and validate the ledger.
2. Rebuild targets and compare exact bytes and SHA-256 digests.
3. For each accepted TSA, verify `.tsr` against both `.tsq` and target bytes
   using the retained trust chain.
4. Check signed `genTime`, policy OID, serial number, and message imprint against
   ledger metadata.
5. For resolved questions, require a valid `genTime` before the known outcome.
6. For reveals, perform the ten reveal checks in the cryptographic specification.
7. Evaluate resolution sources independently against the bound criteria.
8. Report each evidence claim separately.

## Failure interpretation

| Failure | Conclusion |
| --- | --- |
| Schema or semantic validation fails | Ledger record is non-conformant. |
| Rebuilt target differs | Content binding is invalid. |
| Timestamp is pending | External existence time is not established. |
| RFC 3161 signature/request/chain fails | That TSA evidence is invalid. |
| One TSA is unavailable but another retained response verifies | Remaining TSA evidence is still usable. |
| All valid timestamps are after the outcome | Evidence does not exclude hindsight. |
| Reveal authentication/hash/mirror fails | Reveal is invalid. |
| Resolution source does not support the criteria | Outcome remains unverified. |
