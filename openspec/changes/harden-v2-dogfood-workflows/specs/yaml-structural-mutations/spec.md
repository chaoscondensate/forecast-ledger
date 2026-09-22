## ADDED Requirements

### Requirement: Discriminated-union additions preserve their contract shape
When a mutation adds a value represented by a discriminated union, the YAML
and JSON documents SHALL contain the same flattened schema fields and SHALL
not expose implementation-only union branches or wrapper names. Populated YAML
values MUST use expanded block style with stable field order, and the shared
prospective validation and atomic-write rules SHALL remain mandatory.

#### Scenario: Add a group-membership relationship to YAML
- **WHEN** a valid group-membership relationship is added to a YAML ledger with no existing relationships
- **THEN** the stored item contains only `id`, `kind`, `group_id`, and `question_id`, passes local validation, and is semantically equivalent to the JSON result

#### Scenario: Append a conditional relationship to YAML
- **WHEN** a valid conditional relationship is appended to an existing YAML relationships sequence
- **THEN** the stored item contains the flattened conditional fields in stable block order, passes reference and cycle validation, and is semantically equivalent to the JSON result

#### Scenario: CLI and MCP route through the same union mutation
- **WHEN** equivalent valid relationship requests are sent through CLI and MCP against equivalent YAML ledgers
- **THEN** both succeed with equivalent changed pointers and stored relationship models without reformatting unrelated source bytes
