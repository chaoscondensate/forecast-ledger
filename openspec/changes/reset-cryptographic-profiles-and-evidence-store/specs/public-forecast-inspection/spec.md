## Purpose

Defines complete public forecast inspection and secret-safe reveal diagnostics
that preserve useful cryptographic identity without disclosing private material.

## ADDED Requirements

### Requirement: Inspection returns the complete public commitment
CLI `forecast show`, MCP `forecast_show`, and addressed forecast resources SHALL
return the same public commitment for sealed and revealed forecasts. The public
commitment SHALL include scheme, commitment hash, encryption algorithm, nonce,
ciphertext, and safe logical key hint, plus reveal time when present. These
surfaces MUST NOT blank a public nonce or ciphertext merely because the forecast
has not been revealed.

Revealed keys, salts, sealed plaintext, private representations and reasoning,
credentials, protected paths, and unrestricted error text MUST remain absent or
redacted according to the existing secret boundary.

#### Scenario: Unrevealed sealed forecast is shown
- **WHEN** a caller inspects an unrevealed sealed forecast through human, plain, JSON, MCP tool, or MCP resource output
- **THEN** every mode exposes the same non-empty public nonce and ciphertext while exposing no key, salt, plaintext, or private forecast field

#### Scenario: Revealed forecast is shown
- **WHEN** a caller inspects a revealed forecast
- **THEN** the original public commitment and ciphertext remain visible, disclosed forecast fields follow the ledger, and the raw revealed key remains redacted

### Requirement: Reveal failures identify the verification stage safely
Reveal SHALL retain the application category `verification` for cryptographic
or closed-profile failures while exposing a stable secret-safe reason. An AEAD
tag failure caused by a wrong key, altered nonce, altered ciphertext, or altered
associated data SHALL remain `reveal.authentication_failed`. If AEAD and the
commitment digest succeed but the authenticated canonical plaintext does not
match the selected closed seal profile, the reason SHALL be
`reveal.bundle_profile_mismatch`.

Neither reason MAY include decrypted bytes, field values, keys, salts,
ciphertext excerpts, unrestricted parser text, or protected paths. Both failures
MUST occur before ledger mutation.

#### Scenario: Wrong key is supplied
- **WHEN** the selected protected key cannot authenticate the stored ciphertext and associated data
- **THEN** reveal returns `reveal.authentication_failed`, changes no ledger or evidence byte, and discloses no further cryptographic detail

#### Scenario: Authenticated plaintext uses another profile shape
- **WHEN** AEAD and commitment verification succeed but the canonical plaintext is not the exact closed profile selected by the v2.2.0 contract
- **THEN** reveal returns `reveal.bundle_profile_mismatch`, changes no ledger byte, and exposes no protected content

### Requirement: Inspection and reveal diagnostics have adapter parity
CLI and MCP SHALL use the same transport-neutral public forecast view and stable
reveal reason codes. Human formatting MAY differ, but public fields,
redactions, application category, and mutation outcome MUST agree.

#### Scenario: CLI and MCP inspect the same sealed forecast
- **WHEN** equivalent CLI and MCP calls inspect one sealed forecast
- **THEN** their structured public commitment fields and secret omissions are equivalent

#### Scenario: CLI and MCP encounter a profile mismatch
- **WHEN** equivalent reveal calls reach an authenticated closed-profile mismatch
- **THEN** both return `reveal.bundle_profile_mismatch` as a recoverable verification failure and keep the original ledger unchanged
