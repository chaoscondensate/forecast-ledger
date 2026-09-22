## ADDED Requirements

### Requirement: Protected sealed input matches public forecast optionality
The purpose-named protected input for sealed forecasts SHALL require
`representations` and SHALL describe `rationale`, `key_factors`, and `comment`
as optional. CLI help, copyable examples, generated input schemas, MCP tool
descriptions, and the maintained command-surface inventory SHALL agree on that
shape for standalone and initial sealed forecasts. Private values SHALL remain
unavailable through argv, environment variables, generic public input
documents, logs, and normal output.

#### Scenario: Help describes a minimal protected file
- **WHEN** a user reads `forecast seal` or sealed-initial-forecast help
- **THEN** the user can create a protected file containing only representations and can identify every optional private property

#### Scenario: Optional private text is not converted to a flag
- **WHEN** command-surface and completion checks inspect sealed authoring
- **THEN** rationale, key factors, comment, representations, keys, and salts remain absent from raw-value command flags and environment inputs

### Requirement: Protected-input validation is actionable before mutation
Protected-input parsing and validation SHALL complete before entropy, protected
key creation, ledger locking for mutation, or output writes. A failure SHALL
name the exact missing or invalid property and preserve its safe source pointer
or actual source span without printing secret content. CLI and MCP SHALL use the
same generated protected-input schema and shared validation result.

#### Scenario: Required private field is missing
- **WHEN** the protected object omits representations
- **THEN** the operation fails before side effects with a diagnostic naming `/representations` rather than only a generic required-field message

#### Scenario: Optional private fields are all omitted
- **WHEN** the protected object contains valid representations and none of the optional textual fields
- **THEN** request validation succeeds and sealing proceeds through the normal atomic protected-key workflow
