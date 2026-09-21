## REMOVED Requirements

### Requirement: Exclusive v1.3.0 admission without compatibility
**Reason**: The application now admits the published incompatible v2.0.0 contract exclusively.

**Migration**: None. The project has no active users of the retired pre-v2 format and provides no conversion or compatibility path.

### Requirement: One v1.3.0 identity across public surfaces
**Reason**: Public runtime, package, CLI, and MCP metadata now identify the exact v2.0.0 contract and v2 cryptographic profiles.

**Migration**: None. Current public surfaces report only the v2 contract.
