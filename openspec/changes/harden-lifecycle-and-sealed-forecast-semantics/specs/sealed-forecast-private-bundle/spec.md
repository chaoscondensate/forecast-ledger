## Purpose

Defines the minimal authenticated private forecast bundle and actionable,
secret-safe diagnostics for protected sealed-forecast input.

## ADDED Requirements

### Requirement: Only forecast representations are mandatory private content
A protected sealed-forecast input SHALL require a non-empty
`representations` collection. `rationale`, `key_factors`, and `comment` SHALL be
independently optional and omission SHALL remain distinct from an explicitly
supplied empty string or empty collection. The same required and optional shape
SHALL apply to standalone seal, sealed initial forecast, CLI, MCP, dry-run, and
the authenticated private bundle.

#### Scenario: Minimal sealed forecast
- **WHEN** protected input contains one valid representation and omits rationale, key factors, and comment
- **THEN** sealing succeeds without inventing empty textual fields and without exposing the representation through a public request surface

#### Scenario: Representation is absent
- **WHEN** protected input omits `representations`
- **THEN** sealing fails before entropy, key creation, ledger mutation, or other side effects and identifies `representations` as the missing required property

#### Scenario: Optional fields vary independently
- **WHEN** protected input supplies any subset of rationale, key factors, and comment with valid values
- **THEN** the authenticated bundle preserves exactly that presence and value combination

### Requirement: Seal and reveal preserve optional-field presence exactly
The closed `forecast-seal/v2` plaintext object SHALL require the question
revision ID, forecast and recording times, and representations, SHALL permit
the three optional textual fields, and SHALL reject unknown properties. Seal
commitment bytes and published vectors SHALL distinguish absent fields from
present empty values. Reveal SHALL authenticate the exact bundle and add only
the private fields that were present; the revealed forecast SHALL require
representations and its revealed commitment but SHALL keep rationale, key
factors, and comment optional under the public ledger schema.

#### Scenario: Minimal bundle is revealed
- **WHEN** a minimally populated sealed forecast is revealed with its matching protected key
- **THEN** the revealed forecast contains its representation and revealed commitment and omits the three absent optional fields

#### Scenario: Optional-field presence is altered
- **WHEN** decrypted plaintext has an optional field added, removed, or changed relative to the committed canonical bundle
- **THEN** reveal authentication fails and no ledger mutation occurs

#### Scenario: Unknown private property is supplied
- **WHEN** the protected object contains a property outside the closed private bundle schema
- **THEN** sealing rejects that exact property before cryptographic or filesystem side effects

### Requirement: Protected-input errors identify the exact property safely
Schema and semantic failures in protected JSON or YAML SHALL return the stable
usage or invalid-data category appropriate to the operation and SHALL identify
the exact public property name or JSON Pointer without echoing a secret value.
For a property that exists, the diagnostic SHALL use its bounded source span.
For a missing property, the diagnostic SHALL name the absent property and MUST
NOT fabricate line 1 as though source text existed there. Syntax failures SHALL
retain their actual bounded line and column. Diagnostics MUST NOT disclose an
unrestricted path or protected contents in CLI, JSON, logs, or MCP results.

#### Scenario: Missing representations is actionable
- **WHEN** a multi-line protected input omits `representations`
- **THEN** the error names `/representations`, does not generically say only that a required field is missing, and does not attribute the absent value to line 1

#### Scenario: Invalid key factor has a source location
- **WHEN** one present key factor is empty or otherwise invalid
- **THEN** the error identifies its indexed pointer and actual source location without printing the factor or surrounding secret text

#### Scenario: Secret-safe adapter parity
- **WHEN** equivalent invalid protected input is used through CLI and MCP
- **THEN** both expose equivalent stable field identity and classification without secret values, unrestricted paths, or duplicate diagnostics
