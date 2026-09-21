# Documentation requirement traceability

<!-- doc-metadata
coverage: development
reviewed: 2026-08-25
owner: documentation
generated: false
security-critical: false
prerequisites: documentation-baseline.md
next: index.md
-->

Reviewed: 2026-08-25

This matrix assigns every requirement in the `product-documentation` and
`open-source-community` delta specifications. A task reference in the check or
gate column means that the check is planned by that task; it does not claim the
check already exists.

The **project maintainer** role is currently held by Andrey Korchak. The
**interface owner** is the maintainer responsible for the affected CLI, MCP, or
schema declarations. The **documentation owner** is the reviewer assigned by
the project maintainer and defaults to the project maintainer until delegated.

## Open-source community requirements

| Requirement | Document or generated artifact | Generator or test | Owner | Release gate |
| --- | --- | --- | --- | --- |
| Open-source rights are explicit | `LICENSE`, `README.md`, `LICENSES/`, `REUSE.toml`, `THIRD_PARTY_NOTICES.md` | Pinned REUSE/SPDX check (3.2) and SBOM review | Project maintainer | Licensing and notices check (11.5) |
| Contribution path is reproducible | `CONTRIBUTING.md`, `docs/development/build.md`, `docs/development/documentation-checks.md` | Clean contributor walkthrough (12.2) | Documentation owner | Community-health and contributor gate (11.5–11.6) |
| Community conduct is enforceable | `CODE_OF_CONDUCT.md`, `CONTRIBUTING.md`, `README.md` | Contact and policy-link check (11.5); private-route walkthrough (12.3) | Project maintainer | Non-placeholder conduct route (11.5) |
| Security reporting is private and actionable | `SECURITY.md`, issue-form configuration, `README.md` | Contact, supported-version, and policy-link check (11.5); researcher walkthrough (12.3) | Security triage owner | Current security policy and private route (11.5–11.6) |
| Support boundaries are discoverable | `SUPPORT.md`, `README.md`, `docs/index.md` | Internal-link and community-health checks (11.2, 11.5) | Documentation owner | Support routes resolve without placeholders (11.5) |
| Governance and maintenance are transparent | `GOVERNANCE.md`, `CODEOWNERS`, baseline contacts | Ownership and policy completeness check (11.5) | Project maintainer | Named release authority and governance owner (11.5) |
| Issue and pull-request workflows collect useful evidence | `.github/ISSUE_TEMPLATE/`, `.github/pull_request_template.md` | YAML/schema, secret-warning, and required-field checks (11.5) | Project maintainer | Valid forms and PR template (11.5) |
| Changes and releases are communicated for humans | `CHANGELOG.md`, `.github/release.yml`, release-note template | Changelog-format and release-note derivation checks (10.2, 11.6) | Release authority | Updated changelog and release notes (11.6) |
| Project citation is stable | `CITATION.cff` | CFF validation and release metadata synchronization (3.12, 11.5) | Project maintainer | Version, date, license, and URL match release (11.5–11.6) |
| Community health is a release gate | Root policy files and `.github/` community files | Aggregated community-health validator (11.5) | Project maintainer | Required files, contacts, forms, and links pass (11.5–11.6) |

## Product documentation requirements

| Requirement | Document or generated artifact | Generator or test | Owner | Release gate |
| --- | --- | --- | --- | --- |
| README is the product landing page | `README.md`, `docs/index.md` | README coverage and no-image review (4.8, 11.2) | Documentation owner | Landing-page and required-link coverage (11.6) |
| Quick starts are complete and safe | `README.md`, `docs/getting-started/quick-start.md`, sealed tutorial | Documentation scenario harness and secret/output assertions (9.1–9.4) | Documentation and security owners | Public and sealed scenarios pass (11.4, 11.6) |
| Documentation has a task-oriented information architecture | `docs/index.md` and section indexes | Navigation, metadata, orphan, and relative-link validator (2.6) | Documentation owner | No maintained orphan or invalid internal link (11.2) |
| Core user journeys are documented | Getting-started, tutorials, how-to, and troubleshooting pages | Journey coverage manifest and native scenario jobs (9.1–9.4, 11.4) | Documentation owner | Core-journey coverage complete (11.6) |
| Reference matches released interfaces | `docs/reference/cli/`, `docs/reference/mcp/`, output and exit-code references | Candidate-binary and declaration generators with clean-tree diff (7.1–7.6) | Interface owner | Generated reference has no drift (11.2, 11.6) |
| Security and evidence claims are precise | Claims guide, verification explanation, `docs/security/`, `SECURITY.md` | Terminology/prohibited-claim checks and implementation review (8.6, 11.2) | Security and interface owners | Current limitations and security review recorded (11.6) |
| Project identity includes the broader context | `README.md`, `docs/explanation/project-background.md` | Required URL, authority, and prohibited-commercial-status checks (11.2) | Documentation owner | Repository authority and website context remain explicit (11.6) |
| Presentation is accessible and evidence-based | Style and visual guides, generated assets, all maintained pages | Markdown, alt-text, text-equivalent, contrast, and badge checks (2.6, 9.5, 11.2) | Documentation owner | Essential content works without external assets (12.4–12.5) |
| Documentation is continuously verified | CI workflows and `docs/development/documentation-checks.md` | Fast docs checks, bounded external links, scenarios, and native jobs (11.1–11.5) | Documentation and interface owners | Complete documentation release gate (11.6) |
| Documentation is versioned and maintained | Page metadata, changelog, correction policy | Metadata, review-date, and version checks (2.5–2.6, 10.4–10.5) | Documentation owner | Version/support labels and release baseline are current (11.6, 12.6) |

[Development documentation](index.md) · [Documentation index](../index.md)
