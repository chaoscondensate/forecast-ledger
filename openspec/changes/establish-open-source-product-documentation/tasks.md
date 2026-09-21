> v2 cutover note (2026-09-21): `adopt-forecast-ledger-v2` owns current public
> behavior and examples. Every unfinished documentation, scenario, reference,
> compatibility, and release task below must describe and test the v2 command
> surface; v1.x material may appear only as clearly labelled migration or
> historical information.

## 1. Decisions and Documentation Baseline

- [x] 1.1 Reconcile this change with documentation tasks 11.1–11.6 in `build-forecast-ledger-cli-mcp`, assigning each deliverable to one checklist and eliminating duplicate ownership
- [x] 1.2 Inventory the implemented CLI commands, MCP tools/resources, supported platforms, release artifacts, schema pin, maturity labels, audit status, network behavior, and known limitations that documentation must reflect
- [x] 1.3 Record maintainer-approved values for the OSI software license, copyright holder and years, documentation/assets licensing, third-party notice policy, and contributor sign-off or agreement policy
- [x] 1.4 Record non-placeholder security and conduct contacts, public support and issue channels, canonical repository URL, governance owners, release authority, and supported-version policy
- [x] 1.5 Define the approved one-sentence product value, intended audiences, product name, executable name, maturity vocabulary, and evidence-claim vocabulary in plain English
- [x] 1.6 Create a traceability matrix from every requirement in both delta specs to its document, generator, test, owner, and release gate

## 2. Documentation System and Editorial Standards

- [x] 2.1 Create the repository-first `docs/` structure and indexes for getting started, tutorials, how-to guides, reference, explanation, and security, with relative navigation and no orphan pages
- [x] 2.2 Write the documentation style guide covering plain international English, headings, links, code blocks, terminology, capitalization, callouts, version labels, deprecations, and prohibited overclaims
- [x] 2.3 Write the claims and terminology guide for valid, sealed, timestamped, anchored, verified, revealed, published, evidence, proof, authorship, completeness, truth, and outcome evidence
- [x] 2.4 Define the accessible visual system for colors, contrast, status labels, diagrams, terminal captures, alt text, text equivalents, and safety callouts
- [x] 2.5 Define page metadata and ownership conventions for supported versions, last substantive review, generated content, security-critical pages, prerequisites, and next steps
- [x] 2.6 Add navigation and coverage validation that detects orphan pages, missing indexes, invalid relative links, missing language tags, and missing required page metadata

## 3. Open-Source Legal and Community Health

- [x] 3.1 Add the approved license text and SPDX identifier, copyright notices, documentation/assets policy, vendored-material rules, and third-party notices without inferring terms from another repository
- [x] 3.2 Configure and pass a pinned REUSE/SPDX-compatible licensing compliance check for source, documentation, examples, generated assets, fixtures, and vendored files
- [x] 3.3 Write `CONTRIBUTING.md` with cross-platform setup, architecture and source precedence, work selection, change flow, tests, docs checks, conformance gates, review rules, release boundaries, and contribution support
- [x] 3.4 Adopt and configure `CODE_OF_CONDUCT.md` with scope, enforcement responsibilities, confidential contact, and expected response process; link it from README and contribution guidance
- [x] 3.5 Write `SECURITY.md` with supported versions, private reporting, requested evidence, safe-harbor language, acknowledgment/update targets, coordinated disclosure, and advisory location
- [x] 3.6 Write `SUPPORT.md` that routes usage, bugs, security, conduct, schema interoperability, and broader Chaos Condensate questions without unapproved service-level promises
- [x] 3.7 Write `GOVERNANCE.md` with current roles, decision process, protocol-affecting review, release authority, access expectations, conflicts, inactive-maintainer handling, and governance evolution
- [x] 3.8 Add bug, documentation, and feature-request issue forms that request actionable redacted evidence and warn against keys, credentials, or private ledger content
- [x] 3.9 Add public issue configuration that directs security and conduct matters to confidential channels and disables unsupported blank-report paths
- [x] 3.10 Add a pull-request template covering scope, tests, platform impact, schema/crypto compatibility, documentation, security, changelog, and release-note impact
- [x] 3.11 Add ownership/review rules for security-critical docs, generated references, release material, licensing, and community policy files
- [x] 3.12 Add valid `CITATION.cff` metadata and a release check that synchronizes approved author/maintainer, license, version, date, and canonical URL fields

## 4. Product README

- [x] 4.1 Write the README opening with product name, one-sentence value, intended audience, explicit maturity/audit status, supported platforms, and a short statement of what the tool does and does not prove
- [ ] 4.2 Add verified installation and checksum links plus a complete public-forecast quick start using explicit `--file`, representative output, created-file explanations, and final validation
- [ ] 4.3 Add concise routes to sealed forecasts, timestamps, layered verification, portable evidence packages, MCP setup, command reference, and troubleshooting without duplicating their manuals
- [ ] 4.4 Add local-data, telemetry, network, key-custody, experimental-component, and security-reporting summaries with links to complete policies
- [ ] 4.5 Add a documentation map plus support, contributing, governance, changelog, citation, and license links in conventional discoverable locations
- [ ] 4.6 Add a bounded “Chaos Condensate” project paragraph linking `https://chaoscondensate.com/`, identifying it as broader context and the repository/schema as authoritative for CLI behavior
- [ ] 4.7 Add only evidence-backed release, CI, license, and security/best-practice badges that link to their evidence; omit badges whose state is unavailable or not yet earned
- [ ] 4.8 Verify the README remains complete and usable with images and external badge services unavailable

## 5. Installation and Getting Started

- [ ] 5.1 Write platform-correct install, checksum verification, shell completion, upgrade, and uninstall instructions for every declared macOS, Linux, and Windows release artifact
- [ ] 5.2 Write getting-started concepts for ledgers, stable IDs, explicit files, targets, receipts, keys, evidence packages, and independent verification layers
- [ ] 5.3 Write the expanded public-forecast quick start and test it from a clean temporary directory on each supported native operating-system family
- [ ] 5.4 Write the sealed-forecast quick start with protected input, key placement, permission, backup, loss, reveal, and no-plaintext checks placed before irreversible actions
- [ ] 5.5 Add a support matrix and compatibility page covering product versions, platforms, architectures, schema version/commit/digest, MCP protocol, experimental features, and known unsupported behavior

## 6. Tutorials and Task-Focused How-To Guides

- [ ] 6.1 Write and check the end-to-end public forecast tutorial, including question setup, forecast creation, append-only update, target, timestamp, and verification
- [ ] 6.2 Write and check the end-to-end sealed forecast tutorial, including secret-safe input, key custody, target, timestamp, reveal, and retained evidence
- [ ] 6.3 Write and check the timestamp-and-verification tutorial with mocked normal-CI services and clearly separated real-network integration steps
- [ ] 6.4 Write and check the MCP client tutorial covering stdio configuration, ledger and secret roots, read-only default, write/network/reveal grants, structured errors, and shutdown
- [ ] 6.5 Write how-to guides for managing multiple explicit ledger files, platforms, typed questions, resolutions, and append-only forecast updates
- [ ] 6.6 Write how-to guides for revealing safely, verifying layered evidence, building/verifying portable packages, and operating fully local workflows where supported
- [ ] 6.7 Write recovery, troubleshooting, schema-update, binary-upgrade, and migration guides with error-code routes and platform-specific caveats
- [ ] 6.8 Check that every how-to starts from explicit prerequisites, solves one named task, avoids tutorial digressions, and links to relevant reference and explanation

## 7. Exact CLI and MCP Reference

- [ ] 7.1 Implement deterministic command-reference generation from the candidate binary, including command hierarchy, required leaf-local `--file/-f`, selectors, flags, examples, side effects, and network behavior
- [ ] 7.2 Generate and annotate the exit-code, stable error object, JSON output, environment, color/TTY, timeout, cancellation, and shell-completion references
- [ ] 7.3 Generate the MCP tool/resource reference from the same declarations used by the server, including closed schemas, required `file`, grants, side effects, roots, and error mappings
- [ ] 7.4 Write file and artifact reference for ledgers, targets, RFC 3161 `.tsq` requests, `.tsr` responses, retained PEM CA bundles, key files, manifests, evidence packages, locks, journals, and safe path behavior without restating the embedded schema
- [ ] 7.5 Add schema and version provenance to every generated reference page, including binary version and pinned Forecast Ledger version, commit, and digest
- [ ] 7.6 Add clean-tree generation checks that fail when committed reference differs from candidate CLI help, exit codes, output schemas, or MCP declarations

## 8. Explanations, Security, and Project Background

- [ ] 8.1 Write explanations of the architecture, Forecast Ledger contract, format-preserving mutations, append-only history, sealing profile, RFC 3161 TSA/retained-trust model, and transport-neutral evidence packages
- [ ] 8.2 Write the verification-claims explanation that separates every evidence layer and explicitly excludes unsupported authorship, completeness, truth, self-reported-time, and outcome-source claims
- [ ] 8.3 Write the threat model, key-custody guide, timestamp/network trust-source guide, privacy/local-data statement, telemetry statement, and experimental/no-audit disclosures
- [ ] 8.4 Write project background that links `https://chaoscondensate.com/`, distinguishes website context from normative repository contracts, and avoids claims about the separate site's commercial status
- [ ] 8.5 Write architecture and protocol diagrams with accessible source formats, alt text, text equivalents, and reproducible generation instructions
- [ ] 8.6 Review all security-critical explanations against the implementation, pinned schema, conformance fixtures, and release status with their designated owners

## 9. Executable Examples and Generated Assets

- [ ] 9.1 Implement a cross-platform documentation scenario harness that runs the candidate binary in isolated temporary directories and captures normalized stdout, stderr, exit status, and created files
- [ ] 9.2 Add deterministic fixtures and semantic assertions for public, sealed, reveal, timestamp, verification, package, error, and MCP documentation scenarios
- [ ] 9.3 Add checks that documentation scenarios never expose keys, credentials, secret input, private sealed values, unsafe absolute paths, or undeclared output files
- [ ] 9.4 Separate mocked offline-safe examples from explicit real-network integration checks and label trust assumptions in both documentation and test output
- [ ] 9.5 Generate terminal captures and other product visuals from checked scenarios, store editable/generator sources, and verify their text alternatives and reproduction commands
- [ ] 9.6 Add a documented review process for snapshot normalization so only declared nondeterministic IDs, timestamps, paths, and cryptographic randomness are hidden

## 10. Changelog, Releases, and Versioned Documentation

- [ ] 10.1 Create `CHANGELOG.md` with an Unreleased section, curated change categories, ISO dates, security/breaking visibility, version links, and migration guidance conventions
- [ ] 10.2 Define how pull requests add changelog entries and how release notes are derived without publishing raw commit history as user-facing change documentation
- [ ] 10.3 Create a release-note template with install and checksum verification, headline changes, compatibility/schema pins, breaking migrations, security notes, known limitations, and contributor credit
- [ ] 10.4 Add documentation version/support labels and preserve compatibility and migration guidance for every supported release in source and release archives
- [ ] 10.5 Add a correction policy requiring visible corrections and changelog entries for released safety or security guidance that users may have relied on

## 11. CI and Release Quality Gates

- [ ] 11.1 Pin documentation validation dependencies and record their licenses, update policy, network behavior, and local contributor commands
- [ ] 11.2 Add fast pull-request checks for Markdown, internal links, spelling/style exceptions, terminology, prohibited claims, placeholders, navigation, page metadata, and generated-reference drift
- [ ] 11.3 Add bounded external-link checking with retries, owner/expiry exceptions, and separate reporting from deterministic internal-link failures
- [ ] 11.4 Add native-platform CI for install instructions and checked examples plus mocked network documentation tests on normal pull requests
- [ ] 11.5 Add licensing, citation, issue-form, policy-link, confidential-contact, and community-health completeness validation
- [ ] 11.6 Add release gates for successful quick starts, complete core-journey coverage, updated changelog/reference/compatibility, valid checksums/install commands, and current security limitations
- [ ] 11.7 Document how maintainers run every documentation check locally and diagnose or intentionally update generated reference, snapshots, links, and exceptions

## 12. Final Product Review

- [ ] 12.1 Run a fresh-user review that starts at README and completes installation, checksum verification, one public forecast, and validation without private guidance
- [ ] 12.2 Run a fresh-contributor review from clean checkout through build, tests, documentation validation, and a sample pull request using only public contribution guidance
- [ ] 12.3 Run security-researcher and conduct-reporting walkthroughs to confirm private routes work, contacts are current, and public templates redirect sensitive reports
- [ ] 12.4 Review every maintained page for plain English, navigation, accessibility, accurate status, essential text alternatives, and evidence-bound claims
- [ ] 12.5 Verify the full documentation and required legal/community files remain usable from a source release without the website, badge services, or repository hosting UI
- [ ] 12.6 Run strict OpenSpec validation and the complete documentation release gate, resolve all failures and placeholders, and record the reviewed documentation baseline for the first release
