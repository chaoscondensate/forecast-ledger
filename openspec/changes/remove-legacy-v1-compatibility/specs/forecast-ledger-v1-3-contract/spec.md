## REMOVED Requirements

### Requirement: Exact immutable v1.3.0 contract
**Reason**: The pre-release v1.3 contract has no active users and is not a supported runtime input. Retaining its schema, source pins, and release artifacts creates a false compatibility obligation and unnecessary maintenance surface.

**Migration**: None. The project does not provide a v1 reader, converter, archive bundle, or migration procedure.

### Requirement: Published v1.3.0 conformance corpus
**Reason**: V1 fixtures and cryptographic vectors test an obsolete implementation path that the product no longer supports or promises to preserve.

**Migration**: None. Remove the v1-only corpus and test harness; current conformance starts with the v2 contract.
