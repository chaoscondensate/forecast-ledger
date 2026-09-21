## Purpose

Defines deterministic revision-bound v2 targets, sealed forecasts, reveals,
and retained RFC 3161 evidence without weakening existing secret boundaries.

## ADDED Requirements

### Requirement: Forecast targets use the exact v2 envelope
Target construction SHALL produce canonical RFC 8785 bytes for
`forecast-envelope/v2`. The envelope SHALL contain the question ID, the complete
bound question revision, and every present public forecast statement field
defined by the v2 profile. It SHALL exclude integrity metadata. For sealed or
revealed forecasts it SHALL retain the original sealed view, excluding mutable
operational `key_hint`, `revealed_at`, and `revealed_key`, so reveal does not
change the target. Canonicalized values SHALL remain within the project's
bounded I-JSON profile and target paths SHALL remain deterministic and
collision-checked.

#### Scenario: Later question revision does not reinterpret a target
- **WHEN** a new question revision is appended after a target was built
- **THEN** rebuilding the earlier forecast target uses its complete bound revision and produces the same bytes

#### Scenario: Reveal preserves sealed target bytes
- **WHEN** a valid sealed forecast is revealed
- **THEN** rebuilding its target reproduces the original sealed envelope byte-for-byte

### Requirement: Sealing implements forecast-seal v2 exactly
Sealing SHALL implement `forecast-seal/v2` with independent random 32-byte salt,
32-byte ChaCha20-Poly1305 key, and 12-byte nonce. The canonical plaintext SHALL
bind schema, question ID, question revision ID, forecast ID, salt, and the exact
private bundle containing revision ID, forecast and recording times,
representations, rationale, key factors, and comment. The commitment SHALL be
SHA-256 of those bytes, and canonical associated data SHALL bind the scheme,
question ID, revision ID, forecast ID, and commitment hash. Key files SHALL be
protected, versioned, and bound to the same IDs without putting keys or private
content in public output.

#### Scenario: Published v2 vector is reproduced
- **WHEN** sealing runs with every deterministic input from the pinned `forecast-seal-v2` vector
- **THEN** canonical plaintext, commitment, associated data, nonce, ciphertext, key material, and public commitment match the published bytes exactly

#### Scenario: Revision transplant is rejected
- **WHEN** a valid ciphertext or key is presented under a different question revision ID
- **THEN** authentication or binding verification fails without disclosing plaintext

### Requirement: Reveal authenticates before publishing plaintext
Reveal SHALL verify protected key shape and binding, nonce and ciphertext
bounds, AEAD authentication, commitment digest, closed v2 plaintext shape,
canonical byte equality, all three IDs, and every public mirror field before
mutating the ledger. It SHALL retain the original commitment and ciphertext,
add only the v2 reveal fields, and preserve the original target. Any mismatch
SHALL fail atomically and MUST NOT expose private values, keys, salts, decrypted
bytes, or unrestricted cryptographic errors.

#### Scenario: Public mirror was tampered with
- **WHEN** a revealed probability, representation, time, rationale, factor, or comment differs from the authenticated bundle
- **THEN** validation rejects the reveal and no success claim is emitted

### Requirement: V2 RFC 3161 evidence remains local and layered
Timestamp acquisition SHALL hash the exact v2 target with SHA-256 and retain a
request, response, provider URL, target metadata, and trust bundle for each TSA
attempt. Local verification SHALL bind request, response, target, signature,
chain, algorithms, and recorded metadata without a live provider, catalog
lookup, or system-root fallback. Multiple timestamp entries SHALL remain
independent and the aggregate SHALL follow the established pass, pending,
not-checked, and failure semantics.

#### Scenario: One of several timestamps verifies
- **WHEN** at least one complete retained RFC 3161 branch verifies the v2 target and another branch fails or is incomplete
- **THEN** existence timing passes using the verified branch while preserving per-branch results

### Requirement: Resolution chronology uses verified v2 evidence
For a resolved question, every forecast whose integrity state claims verified
timing SHALL contain at least one verified RFC 3161 `gen_time` strictly before
the resolution's `outcome_known_at`. Later tokens MAY remain as integrity
evidence but MUST NOT support a no-hindsight claim.

#### Scenario: All verified tokens postdate the known outcome
- **WHEN** a forecast is marked verified but none of its verified tokens predates `outcome_known_at`
- **THEN** semantic validation rejects the chronology claim
