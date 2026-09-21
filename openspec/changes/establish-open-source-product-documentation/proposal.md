> v2 cutover note (2026-09-21): `adopt-forecast-ledger-v2` supplies the current
> schema and command facts for every deliverable in this proposal. Examples and
> generated references must use v2 unless they are explicitly migration or
> historical material.

## Why

The CLI handles cryptographic evidence and forecasting records, so documentation is part of its safety and trust boundary rather than optional project polish. A new user, operator, contributor, security researcher, or MCP integrator should be able to understand the product, reach a successful workflow, verify its limitations, and find the right participation channel without reading the source or guessing project policy.

## What Changes

- Create an English, product-quality `README.md` that explains the problem, audience, maturity, trust model, supported platforms, installation, a short verified quick start, core CLI and MCP workflows, security warnings, and clear next links without duplicating the full manual.
- Organize maintained documentation using the Diátaxis model: tutorials for learning, task-focused how-to guides, generated or checked reference material, and explanations of the ledger, cryptography, RFC 3161, evidence claims, and architectural choices.
- Treat documentation examples as testable product interfaces: pin expected output where useful, execute safe command snippets in CI, check internal/external links, and fail when command help or documented schemas drift.
- Add complete open-source community health and trust material: license and copyright policy, contribution guide, code of conduct, support policy, security policy with private reporting, governance and maintainer expectations, changelog/release-note policy, citation metadata, issue forms, and pull-request guidance.
- Establish a restrained and accessible presentation system for repository documentation: consistent voice, navigation, status labels, diagrams, terminal captures, alt text, and a small set of evidence-backed badges rather than decorative badge walls or unverifiable claims.
- Link `https://chaoscondensate.com/` from the README and project-background documentation as the broader Chaos Condensate context. State that repository documentation and the pinned Forecast Ledger contract are authoritative for the CLI, and do not claim that the separate website or practice is non-commercial.
- Document support boundaries and honest product claims, including schema pinning, experimental or unaudited components, key custody, timestamp trust sources, evidence limitations, offline behavior, data privacy, and what the tool does not prove.
- Define documentation ownership, review cadence, release gates, and versioning so documentation remains current after the initial launch.

## Capabilities

### New Capabilities

- `product-documentation`: Defines the README, documentation information architecture, user journeys, reference coverage, examples, presentation quality, website attribution, versioning, and automated documentation verification.
- `open-source-community`: Defines licensing, contribution, conduct, support, security disclosure, governance, issue/PR workflows, changelog, citation, and community-health expectations for a trustworthy open-source project.

### Modified Capabilities

None. This change expands the coarse documentation tasks in `build-forecast-ledger-cli-mcp` without changing the CLI, ledger, cryptographic, timestamp, verification, or MCP behavior specified there.

## Impact

- Adds and maintains repository-root project documents, a structured `docs/` tree, checked examples and visual assets, and `.github/` community templates and automation configuration.
- Adds documentation quality checks to CI and release gates, with generated reference input coming from the implemented CLI/MCP schemas rather than hand-maintained parallel contracts where possible.
- Requires maintainers to review documentation alongside behavior changes and to keep release notes, compatibility statements, and security limitations current.
- Uses the CLI/MCP change and pinned Forecast Ledger schema as technical sources of truth; the website link supplies broader project context but is not a runtime dependency or normative CLI specification.
- Does not implement a hosted documentation service, telemetry, user accounts, community forums, translation workflow, or runtime product behavior in this change.
