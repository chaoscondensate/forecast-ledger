# Documentation baseline

<!-- doc-metadata
coverage: current-main
reviewed: 2026-08-31
owner: project-maintainer
generated: false
security-critical: true
prerequisites: ../../AGENTS.md
next: documentation-traceability.md
-->

Reviewed: 2026-08-31

This page records the implementation and policy inputs used by the maintained
product documentation. It describes the repository at the reviewed commit; it
does not make unfinished interfaces available or stable.

## Documentation ownership

The `establish-open-source-product-documentation` OpenSpec change owns public
documentation and community-health deliverables. The older documentation tasks
in `build-forecast-ledger-cli-mcp` are handoff records, not a second checklist.

| Earlier task | Owning tasks in this change |
| --- | --- |
| 11.1 README and workflow quick starts | 4.1–4.8, 5.3–5.4, and 6.1–6.3 |
| 11.2 CLI reference, JSON, and exit codes | 7.1–7.2 and 7.5–7.6 |
| 11.3 Security and evidence limits | 3.5, 4.4, 8.2–8.3, and 8.6 |
| 11.4 MCP setup and safety | 6.4 and 7.3 |
| 11.5 Schema update and conformance | 6.7, 7.5, 8.1, and 8.6 |
| 11.6 Checked examples and tutorials | 5.3–5.4, 6.1–6.6, and 9.1–9.6 |

Completion is recorded only in the owning checklist. A handoff does not mean
that the corresponding document or test has been implemented.

## Implementation inventory

This inventory was checked against current main on 2026-09-21 and describes the
v2-only surface.

### Working CLI surface

The following commands have connected application services:

| Command | Behavior | Network |
| --- | --- | --- |
| `forecast-ledger init --file <new-path> ... [authoring flags]` | Exclusively creates a schema-valid JSON/YAML ledger. Direct flags can add root metadata, platforms, one question, and an optional first public forecast. Sealed private values use protected `--initial-secret-input`. | None |
| `forecast-ledger ledger update --file <path> <set-or-clear flags>` | Minimally patches allowed root and current forecaster metadata through flags only. | None |
| `forecast-ledger platform add|update|list|show|remove ...` | Creates, patches, lists, inspects, or removes unreferenced platform records. List/show accept `--file -`; removal requires approval. | None |
| `forecast-ledger group ...`, `relationship ...` | Manages groups, memberships, and acyclic conditional relationships with guarded removal. | None |
| `forecast-ledger question add|revise|update|list|show|resolve|ambiguous|void|dispute|not-applicable ...` | Creates immutable typed revisions, applies question-level patches, reads redacted summaries, and records approved v2 terminal states. List/show accept `--file -`. | None |
| `forecast-ledger forecast add|list|show|seal|reveal|withdraw|expire|reaffirm ...`, `forecast key-hint update ...` | Appends revision-bound public or sealed forecasts and activity events, authenticates approved reveal, repairs safe logical hints, or reads redacted history. List/show accept `--file -`. | None |
| `forecast-ledger target build|check ...` | Builds or checks exact `forecast-envelope/v2` RFC 8785 target bytes at deterministic ledger-relative paths. | None |
| `forecast-ledger timestamp stamp|status|verify ...` | Acquires RFC 3161 evidence through `auto` (currently FreeTSA), one named built-in provider, or a custom public HTTPS TSA and CA pair; status and verify inspect retained bytes locally. | Selected TSA only for stamp; none for status or verify |
| `forecast-ledger verify --file <path> ...` | Reports document, content-binding, RFC 3161 existence-timing, reveal, and outcome-evidence layers with stable states, reasons, evidence, and limitations. Pass requires applicable forecast evidence; empty or all-not-applicable selections return `no_evidence`. | Outcome URLs only with `--check-sources`; timestamp checks are local |
| `forecast-ledger publish build|verify ...` | Builds a deterministic allowlisted package containing the exact RFC 3161 artifacts and verifies it locally. Manifest/file integrity is reported separately from the evidence aggregate. It does not use Git or a hosted publisher. | None |
| `forecast-ledger mcp serve --ledger-root <name=path> ...` | Starts a protocol-clean stdio server with closed CLI-parity tools and redacted addressed resources. Default is read-write/online within roots; read-only/offline are whole-server modes and reveal is separately default-off. | Explicit TSA only for timestamp stamp unless server is offline |
| `forecast-ledger validate --file <path>` | Parses and validates an embedded-schema JSON or YAML ledger. Eligible for `--file -`. | None |
| `forecast-ledger status --file <path>` | Validates a ledger and summarizes questions, forecasts, and timestamp states. Eligible for `--file -`. | None |
| `forecast-ledger version [--json]` | Reports labeled binary, source, Go, schema, MCP, and RFC 3161 metadata; human labels may use TTY-safe color while plain/JSON/non-TTY output remains undecorated. | None |
| `forecast-ledger completion <shell>` | Generates Bash, Zsh, Fish, or PowerShell completion text through urfave. | None |

The root exposes stable presentation flags `--json`, `--plain`, `--quiet`,
`--verbose`, `--no-color`, `--no-input`, `--yes`, and `--timeout`. JSON, plain, and quiet modes are
mutually exclusive. Stable application error codes are `usage`,
`invalid_data`, `unsupported_schema_version`, `not_found`, `conflict`,
`verification`, `io`, `network`, `network_disabled`, `pending`, `incomplete`,
`unavailable`, `internal`, and `interrupted`; their CLI exits are implemented in
`internal/app/errors.go`.

Expected verification reports remain on stdout even when their semantic result
uses a nonzero exit. TSA acquisition outage uses `network`/exit 8; retained
evidence failure uses `verification`/exit 6; and `no_evidence` uses
`incomplete`/exit 9.

### Availability and maturity

Every visible leaf in the current source has a real action and an availability
test; the old preview unavailable handler has been removed. This does not make
every subsystem stable. RFC 3161 support is bounded and OpenSSL-interoperable,
but no independent security or cryptographic review is recorded. Installed releases may expose less than
unreleased `main`; candidate-binary help and `version --json` are authoritative
for an installed artifact.

Release `v0.6.0` has a release-blocking YAML regression: operations that replace
an existing structure can fail with `internal`, although the transaction leaves
the original ledger valid and unchanged. Release `v0.6.1` restores YAML/JSON
parity for question lifecycle, platform update, forecast reveal, and timestamp
recording. Users of the affected binary can use JSON as a temporary workaround
or upgrade to `v0.6.1`.

### Contract and protocol pins

| Item | Reviewed value |
| --- | --- |
| Forecast Ledger schema | `2.0.0` |
| Schema commit | `1d3b186a15136bc5aff38647cb59fbef475dbe55` |
| Annotated tag object | `7b4a9e85e0df9350750828a57b03ff729f704ee4` |
| Release archive SHA-256 | `1d56cbe4f6cbd1fccb046a99add2ff4f88c709904d027669039ba4139664f47e` |
| `SHA256SUMS` SHA-256 | `77d093fbdb393dc9c1e3bdae053724e5ecdccd2e211f6f6c08c7678e78b178dc` |
| Embedded schema SHA-256 | `efd87b7432f7cb017fbedaba217e4cd1d3bff06133457927fb786a249a040b21` |
| MCP protocol target | `2026-07-28` |
| Timestamp protocol | RFC 3161 with SHA-256 message imprints, strong SHA-256/384/512 CMS signer digests, ESS v1/v2, built-in FreeTSA HTTPS plus custom HTTPS, retained CA bundle, and no system-root fallback |
| Go toolchain | `1.27.0` |

Validation uses embedded contract bytes and does not resolve remote schema
references. Documentation and builds use the exact commit and digest above,
never a floating tag. Documents with any other schema version receive an
`unsupported_schema_version` warning and are rejected before side effects.

### Platforms and release artifacts

GoReleaser builds six `CGO_ENABLED=0` archives:

- macOS arm64 and x86-64 as `.tar.gz`;
- Linux arm64 and x86-64 as `.tar.gz`; and
- Windows arm64 and x86-64 as `.zip`.

Release `v0.8.0` contains those six archives and eight native Linux packages:
`deb`, `rpm`, `apk`, and Arch Linux packages for arm64 and x86-64. Each package
installs the binary in `/usr/bin` and the Apache-2.0 license in
`/usr/share/licenses/forecast-ledger`. The main checksum manifest covers the
archives, Linux packages, and their fourteen SBOMs. GitHub artifact attestations
bind those published digests. CI runs unit tests and vet on current
GitHub-hosted macOS, Ubuntu, and Windows runners, checks the full artifact matrix
on Ubuntu and Windows, and installs and removes the Debian package on Ubuntu. A
cross-build does not establish native filesystem correctness on every
architecture.

Windows x86-64 also receives a Chocolatey `nupkg`, built, published,
smoke-tested, and separately attested on the Windows release runner; Windows
ARM64 uses the native ZIP. GoReleaser creates the `nupkg` after the main checksum
manifest, so that package is not a `checksums.txt` entry. Release `v0.1.0`
predates the native Linux and Chocolatey packages and contains archives only.

These are GitHub Release assets, not APT, RPM, APK, Arch, public Chocolatey,
Winget, or Scoop repositories. Package-manager update discovery is therefore
not available yet.

Homebrew installs the stable release as
`chaoscondensate/tap/forecast-ledger`. The macOS archives are not Developer ID
signed or notarized; the formula removes quarantine metadata and applies a
local ad-hoc signature in the Cellar. Windows archives and the Chocolatey
package are not code signed. Linux packages are not package-signed.

## Licensing and contribution policy

The maintainer approved the following policy on 2026-08-25:

- Original software, documentation, examples, and generated assets use the
  Apache License 2.0, SPDX identifier `Apache-2.0`.
- The copyright notice is `Copyright 2026 Chaos Condensate contributors`.
- Vendored or otherwise incorporated third-party material keeps its upstream
  license and attribution. Its license is stored with the material where
  practical and represented in third-party notices and release SBOMs.
- Dependencies are not relicensed. Their resolved versions and licenses are
  reviewed and recorded separately from the project's own license.
- The project does not require a Contributor License Agreement or Developer
  Certificate of Origin sign-off. Contributions intentionally submitted for
  inclusion are handled under the contribution terms of Apache-2.0. A future
  policy change applies prospectively and must be documented before use.

Apache-2.0 permits commercial and non-commercial use. Choosing an OSI-approved
license does not state or require that the maintainers plan to commercialize
the project.

## Project contacts and authority

The maintained project routes are:

| Concern | Route |
| --- | --- |
| Canonical repository | <https://github.com/chaoscondensate/forecast-ledger> |
| Public usage, bug, feature, and documentation support | <https://github.com/chaoscondensate/forecast-ledger/issues> |
| Private vulnerability report | <https://github.com/chaoscondensate/forecast-ledger/security/advisories/new> |
| Private conduct report | `andrey@chaoscondensate.com` |
| Forecast Ledger interoperability | <https://github.com/chaoscondensate/schema> |
| Broader project context | <https://chaoscondensate.com/> |

Andrey Korchak (`@57uff3r`) is the current project maintainer, governance owner,
security triage owner, conduct enforcement contact, and release authority.
These roles may be delegated publicly as the maintainer group grows. No other
person may publish an official release or change a protected policy solely by
virtue of having contributed code.

The project supports the latest stable release on a best-effort basis. The
unreleased `main` branch and preview or experimental components receive no
separate compatibility or response-time promise. A security fix may be issued
for an older release when the maintainer judges that users cannot reasonably
upgrade, but that exception does not create a standing long-term-support line.
Support policy does not promise an acknowledgment or resolution service level.

## Product identity and language

**Product name:** Forecast Ledger CLI  
**Executable name:** `forecast-ledger`  
**One-sentence value:** Create and independently check portable forecast
evidence without requiring Git or a hosted service.

The intended audiences are:

- individuals and teams that keep explicit forecast histories;
- reviewers and researchers who independently inspect forecast evidence; and
- local-tool and MCP integrators who need a structured, permission-bounded
  interface.

The maintained maturity labels have these meanings:

| Label | Meaning |
| --- | --- |
| Development | Unreleased work may be incomplete and may change without upgrade support. |
| Preview | A release is available for evaluation, but feature, compatibility, native-platform, or review gates are still open. |
| Stable | The documented interface and required production gates are complete for the stated support window. |
| Experimental | A named component has additional unresolved conformance, security, or interoperability risk. |
| Deprecated | The behavior still works for a stated period but has a documented replacement and removal version. |
| Unsupported | The behavior is outside the maintained contract and must not be presented as working. |

Current main is the breaking v2 cutover for a future preview release. A stable SemVer tag identifies a
reproducible release; it does not promote unfinished commands or unaudited
components to the Stable product maturity label.

Evidence language is bounded as follows:

- **valid** means that named structural, format, and semantic checks passed;
- **sealed** means that forecast plaintext was processed by the named sealing
  profile and is not present in the sealed ledger field;
- **timestamped** names an RFC 3161 entry state and never by itself means
  independently verified timing;
- **trusted timestamp** means the named response passed against its retained CA
  bundle under the documented verification policy;
- **verified** must name the layer that passed, failed, is pending, was not
  checked, or does not apply;
- **revealed** means a disclosed value was checked against the named sealed
  commitment; it does not erase the original commitment evidence;
- **published** means material was made available through some transport and
  adds no cryptographic claim by itself;
- **evidence** is inspectable material relevant to a bounded claim; and
- **proof** is used only when a specific protocol and conclusion are named.

The product does not use validity, timestamps, publication, or layer results to
claim authorship, ledger completeness, forecast truth, exact self-reported
time, or substantive outcome-source correctness.

### Maturity, audit, and limitations

- The approved public status currently visible in README is **Preview**. The
  `v0.8.0` is published through the explicit release workflow; version
  stability does not imply feature completeness or audit.
- No independent security or cryptographic audit is recorded. A pinned
  `go tool govulncheck ./...` completed on 2026-08-31 with no reachable
  vulnerability and one required-module advisory in code this project does not
  call. That bounded result is not an audit or a security guarantee.
- Local validation and status are offline. No telemetry or runtime update check
  is implemented. Only RFC 3161 stamp uses the network; omission contacts the
  built-in FreeTSA HTTPS profile, while custom URLs remain public HTTPS-only.
  Timestamp verification is local.
- Sealing, reveal, target generation, RFC 3161 timestamping, layered
  verification, evidence packages, and MCP tools/resources are connected.
- Pending timestamp entries are not verified timing. Validation and release provenance
  do not prove authorship, completeness, forecast truth, exact self-reported
  time, or outcome-source correctness.
- The embedded schema and conformance fixtures retain upstream attribution in
  `third_party/forecast-ledger`; they are not maintained as original project
  material.

[Development documentation](index.md) · [Documentation index](../index.md)
