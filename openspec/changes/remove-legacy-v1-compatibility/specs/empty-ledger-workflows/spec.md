## MODIFIED Requirements

### Requirement: Generated contracts and documentation teach empty-first use
Generated request schemas, MCP tool schemas, CLI help, README,
getting-started material, command reference, examples, and changelog SHALL
agree that CLI init and question creation reject generic input documents and do
not require initial forecasts. CLI examples SHALL show a complete v2 flag-only
empty-first flow, including an initial question revision, domain, explicit later
forecast revision binding, and canonical decimal representation. Protected
sealed examples SHALL keep private data out of argv. Maintained documentation
SHALL NOT provide v1 compatibility instructions, migration steps, or links to a
retained legacy contract. The changelog SHALL state only that the v2 cutover is
breaking and that no migration is provided because there were no active users.

#### Scenario: Generated schemas match runtime behavior
- **WHEN** maintained schemas and MCP contracts are regenerated and checked
- **THEN** direct v2 fields and required revision selectors match runtime, generic public wrappers remain absent, and protected sealed-input restrictions remain represented

#### Scenario: A new user follows the empty-first documentation
- **WHEN** a user copies the documented init, platform-add, revision-bearing question-add, and later representation-bearing forecast-add commands
- **THEN** each command uses real flags, requires no prepared public JSON/YAML fragment, and produces a valid v2.0.0 ledger at every step

#### Scenario: A reader looks for legacy guidance
- **WHEN** a reader searches maintained documentation, navigation, generated references, and CLI or MCP help for migration or compatibility instructions
- **THEN** no v1 migration procedure, compatibility promise, legacy bundle, or conversion path is presented
