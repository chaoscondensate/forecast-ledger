# Changelog

Notable user-visible changes to Forecast Ledger CLI are recorded here. Release
tags and downloadable files are published on GitHub Releases.

## Unreleased

## 0.11.0 - 2026-09-23

### Changed

- **Breaking:** accept only the exact Forecast Ledger v2.2.0 contract and reset
  cryptographic identities to `forecast-seal/v3`, `forecast-key/v3`,
  `forecast-envelope/v3`, and `forecast-lifecycle/v2`. No v2.1.0 reader,
  converter, or compatibility bundle is included.
- Make target build retain ledger metadata, canonical target bytes, and the
  closed `forecast-evidence-index/v1` sidecar together. Lifecycle target build
  now requires an explicit checkpoint ID and recording time.
- Reconcile ledger declarations, index entries, and managed `proofs/` and
  `trust/` files before mutation, verification, or publication.
- Replace publication v2 with `forecast-ledger-publication/v3`, including the
  exact evidence index and every indexed artifact. Empty-evidence packages
  contain a package-local empty index.
- Expose sealed nonce and ciphertext as public commitment fields while keeping
  raw keys, salts, plaintext, private fields, credentials, and protected paths
  redacted.

## 0.10.0 - 2026-09-22

### Changed

- **Breaking:** replace the embedded Forecast Ledger v2.0.1 contract with the
  exact v2.1.0 correction. The runtime accepts only v2.1.0 and rejects v2.0.1
  before side effects; no converter or compatibility bundle is included.
- Add deterministic `forecast-lifecycle/v1` targets, exact-head RFC 3161
  checkpoints, retained prefix verification, and portable publication-package
  support without changing `forecast-envelope/v2` bytes.
- Report current activity and lifecycle coverage separately from forecast
  integrity across CLI, JSON, MCP, and package verification. Coverage can be
  unbound, partial, pending, not checked, verified, or failed.

### Fixed

- Reject lifecycle events dated before their forecast and events recorded
  before either their effective time or the forecast record time.
- Require only representations in protected sealed-forecast input. Rationale,
  key factors, and comment are independently optional, preserve absent versus
  explicit empty values, and are revealed only when authenticated as present.
- Point protected-input diagnostics at the exact missing or invalid property
  without inventing a line-one scalar or exposing rejected secret content.
- Accept the documented nested question object in MCP `ledger_init`, including
  sealed initial forecasts, instead of misreading it as a string selector.

### Security and evidence limits

- A verified lifecycle checkpoint detects changes within its covered event
  prefix, but cannot prove that a removed event existed after every independent
  observation and retained artifact for it has also been deleted.

## 0.9.1 - 2026-09-22

### Changed

- **Breaking:** replace the embedded Forecast Ledger v2.0.0 contract with the
  exact v2.0.1 correction. The runtime accepts only v2.0.1 and provides no
  converter or compatibility bundle; v2.0.0 is rejected before side effects.
- Exclude mutable lifecycle events from public and sealed
  `forecast-envelope/v2` targets, preserving retained targets and RFC 3161
  evidence across withdrawal, expiry, and reaffirmation while reporting current
  activity separately.

### Fixed

- Preserve flattened, ordered YAML relationship variants with JSON parity for
  whole-collection creation, append, and dry-run paths.
- Derive omitted question-revision times strictly after the prior revision,
  including identical or regressing clock observations and fractional times.
- Use the ledger timezone for omitted reveal and terminal-question operation
  times, and identify missing protected reveal keys as key files without
  exposing full paths.

## 0.9.0 - 2026-09-21

### Changed

- **Breaking:** replace the pre-v2 runtime contract with the exact v2.0.0
  schema. No migration or compatibility bundle is provided because there were
  no active users to migrate; non-v2 files are rejected before side effects.
- Add immutable question revisions, six domain kinds, versioned option and bin
  sets, groups, typed relationships, terminal v2 resolution states, and
  forecast activity events.
- Replace v1 basis-point values with exact-decimal probability, PMF,
  binned-PMF, quantile, CDF, point, and credible-interval representations.
- Upgrade sealed forecasts, canonical targets, RFC 3161 fixtures, MCP tools,
  generated contracts, and publication metadata to their v2 bindings.

### Security

- Exercise the v2 implementation and release packages on native GitHub-hosted
  macOS, Linux, and Windows runners. No independent security or cryptographic
  audit has occurred; external review remains desirable follow-up work and the
  release makes no audit or formal-verification claim.

## 0.8.0 - 2026-08-31

### Changed

- Rename the canonical repository and Go module from
  `github.com/chaoscondensate/cli` to
  `github.com/chaoscondensate/forecast-ledger`. The executable, CLI and MCP
  interfaces, ledger contract, release asset names, and package tokens are
  unchanged.
- Use the current GitHub repository identity in release and recovery jobs
  instead of a hard-coded repository slug, and publish package metadata and
  Homebrew download URLs under the canonical repository name.

### Fixed

- Keep the embedded contract outside Go's reserved `vendor` directory so
  versioned `go install` builds include the schema and license bytes.

## 0.7.0 - 2026-08-31

### Fixed

- Preserve standard Go JSON shape while redacting secret-named fields. Embedded
  timestamp fields remain flat, JSON tags and large integers are kept, and MCP
  text and structured content use the same sanitized value.
- Use one service-owned outcome contract for CLI and MCP. Timestamp success is
  `timestamp.verified`, package dry-run is `publication.build.planned`, no-op
  mutations use stable `*.unchanged` codes, and failed target checks retain
  their typed report with `target.failed`.
- Recover an unambiguous ledger write journal automatically under the next
  writer lock, while preserving ambiguous state for investigation.
- Bind optional outcome-source checks to the public numeric address actually
  dialed, disable environment proxies, and reject mixed or reserved DNS answers
  including CGNAT.
- Pin `govulncheck` as a project tool and require its completed analysis before
  CI snapshots and tag releases.
- Keep prerelease Linux package and SBOM filenames aligned with the published
  checksum manifest, and verify published asset names and digests before
  attestation.

## 0.6.1 - 2026-08-30

### Fixed

- Fix the release-blocking YAML structural-replacement regression found while
  dogfooding 0.6.0. Question updates and lifecycle changes, platform updates,
  forecast reveal, and RFC 3161 timestamp recording now preserve the existing
  YAML collection context instead of failing with `internal`; equivalent JSON
  and YAML operations again produce the same validated ledger state.
- Keep normalized scalar replacements on the source-preserving scalar path and
  render populated mapping or sequence replacements in expanded block style
  without changing unrelated comments, quoting, ordering, or line endings.

## 0.6.0 - 2026-08-30

### Changed

- **Breaking:** adopt the exact Forecast Ledger schema v1.3.0 contract and
  reject v1.2.0 before side effects. Questions now have an optional opening-only
  forecast window; the removed closing field is rejected.
- **Breaking:** remove the generic public CLI request-document flag and MCP
  request wrappers. Public authoring uses leaf-local CLI flags or flattened MCP
  properties; sealed values retain only purpose-named protected channels.
- Accept deterministic ISO and English-month CLI date forms in the ledger or
  init timezone. Ambiguous DST wall times require an explicit numeric offset.
  Omitted forecast times default to one current operation observation.
- Write every populated application-authored YAML mapping and sequence in
  expanded block style while keeping explicit empty collections compact.

## 0.5.2 - 2026-08-30

### Changed

- Make direct CLI flags the complete ordinary authoring path for ledger,
  platform, question, lifecycle, and public forecast data. Closed `--input`
  documents remain optional, mutually exclusive batch inputs.
- Split direct sealed-forecast authoring into public metadata flags and a
  protected `--secret-input`; sealed initial forecasts use
  `--initial-secret-input`. Private values and keys remain outside argv.
- Present human `version` metadata as compact labeled lines with terminal-safe
  color policy while preserving stable JSON.
- Make `timestamp stamp` and `timestamp_stamp` default to the qualified
  one-entry FreeTSA catalog. `--tsa-provider`/`tsa_provider` can name the
  built-in profile; custom `tsa_url` and `ca_bundle` remain a required-together
  public HTTPS pair.
- Accept RFC 3161 ESS `SigningCertificate` v1 and
  `SigningCertificateV2`, while keeping message imprints at SHA-256 and
  accepting strong SHA-256, SHA-384, and SHA-512 CMS signer digests.
- Retain the successful built-in CA bytes under deterministic
  `trust/rfc3161/` paths. Built-in provider failures commit no evidence.

### Fixed

- Verify emitted publication packages with `proofs/` and `trust/` beside
  `ledger/` instead of resolving artifact paths below `ledger/`.
- Return usage exit 2 for removed or unknown child commands with `--help`
  instead of an internal error.
- Classify a received malformed or unsupported TSA response as verification
  failure rather than provider unavailability, while keeping transport and
  HTTP-status outages in the network category.

## 0.5.0 - 2026-08-30

The stable tag did not produce a stable GitHub Release because it shared its
source commit with the preceding release-candidate tag. Use `v0.5.1`, which
contains the same product changes on a distinct release commit.

## 0.4.0 - 2026-08-30

### Changed

- **Breaking:** adopt the exact Forecast Ledger schema v1.2.0 contract and
  reject v1.1.0 before any file, secret, entropy, or network effect. There is no
  compatibility reader or migration command.
- **Breaking:** replace OpenTimestamps and Bitcoin observation with RFC 3161.
  `timestamp stamp` now requires an explicit public HTTPS TSA URL and retained
  CA bundle; `timestamp status` and `timestamp verify` operate locally.
- Retain the exact target, `.tsq`, `.tsr`, and PEM trust material needed for
  portable verification. Repeated stamping with another TSA preserves an
  independent timestamp entry.
- Package every referenced RFC 3161 artifact and verify publication packages
  without a blockchain, hosted API, system trust store, or network option.

### Removed

- Remove timestamp upgrade, calendar and endpoint profiles, Bitcoin Core and
  explorer options, user-supplied verification time, OTS receipts, Bitcoin
  result fields, and the OTS liveness workflow.

### Security and evidence limits

- RFC 3161 verification trusts only the CA bundle retained with the evidence
  and verifies certificate validity at the signed generation time. It performs
  no revocation lookup and provides no RFC 4998 renewal or long-term-validation
  guarantee.
- A valid TSA response supports a bounded existence-time claim for the target
  digest. It does not prove authorship, completeness, forecast truth, TSA clock
  honesty, exact self-reported time, or outcome-source correctness.

## 0.3.1 - 2026-08-29

### Fixed

- Distinguish unavailable, inconclusive, and budget-limited Bitcoin observation
  from a cryptographic proof mismatch. Timestamp verification now preserves a
  safe structured report and leaves the ledger unchanged when observation does
  not complete.
- Reserve `timing.bitcoin_mismatch` for comparison against complete Bitcoin
  observations, and let a later valid attestation win when a receipt contains
  multiple Bitcoin branches.
- Report `no_evidence` with `incomplete`/exit 9 for empty or entirely
  `not_applicable` evidence selections instead of returning a vacuous pass.
  Document, manifest, and package-file integrity observations remain visible.
- Keep CLI and MCP outcome fields, stable application categories, request
  summaries, safe source IDs, generated contracts, and public guidance aligned.

## 0.3.0 - 2026-08-29

### Changed

- **Breaking:** adopt the exact Forecast Ledger schema v1.1.0 contract. Empty
  ledgers and questions without forecasts are now valid; `init --input` and a
  question's `initial_forecast` are optional.
- Reject v1.0.0 ledgers with an explicit `unsupported_schema_version` warning
  before any write, key/artifact creation, or network request. This preview
  cutover has no migration command or compatibility reader.
- Clarify in timestamp help and human version output that the Bitcoin observer
  limits apply to each invocation, not to a unit of elapsed time.

## 0.2.4 - 2026-08-27

### Fixed

- Give each MCP parity-test call an independent deadline so slower release
  runners cannot exhaust one shared test budget before later operations.

## 0.2.3 - 2026-08-27

### Fixed

- Report never-built targets as ordered `not_applicable` results and continue
  `target check --all`, while retaining hard failures for missing referenced or
  mismatched evidence.
- Default initial recorded times from one operation-clock observation instead
  of copying an explicit historical ledger creation time; inclusive time
  boundaries now use inclusive wording.
- Omit unknown diagnostic positions instead of printing line/column zero, and
  keep semantic field order when JSON or YAML records are inserted or replaced.
- Make MCP root errors identify safe root classes, flags, and route IDs without
  exposing configured absolute paths.
- Show retained target, receipt, Bitcoin height, anchored-before, and
  verification-time evidence in forecast and layered verification output while
  keeping offline values clearly labeled as stored.
- Keep writer locking fail-fast even when a long `--timeout` is supplied; callers
  that intentionally contend remain responsible for serialization or bounded
  retry with backoff.

### Testing

- Add regressions derived from the v0.2.2 dogfooding findings across CLI modes,
  MCP, JSON/YAML structural edits, fixed clocks, concurrent writers, and stored
  OpenTimestamps evidence.

## 0.2.2 - 2026-08-26

### Fixed

- Preserve retained target and OpenTimestamps evidence while authoring later
  forecasts and question lifecycle changes.
- Keep YAML timestamp inputs, structured validation diagnostics, and private
  input permission errors safe and consistent across CLI transports.
- Preserve readable expanded JSON/YAML ledger formatting as records are added
  or revealed.
- Return exit 9 after presenting pending or incomplete layered and package
  verification reports instead of remapping them to an internal failure.
- Show type-aware question, forecast, and verification details in human and
  plain output without exposing sealed data or internal Go union fields.
- Omit mutating MCP tools from read-only discovery and report static tool
  effects separately from the server's current mode.

### Testing

- Add a reproducible multi-question dogfood lifecycle and run it in native
  macOS, Linux, and Windows CI jobs.

## 0.2.1 - 2026-08-26

### Fixed

- Validate Windows owner-only key ACLs structurally instead of comparing a
  non-canonical SDDL rendering.
- Snapshot artifact-root identity from an open handle so replacement is
  detected reliably on Windows.

## 0.2.0 - 2026-08-26

### Added

- Complete ledger, platform, question, public forecast, sealed forecast,
  target, timestamp, verification, publication, and MCP command surfaces.
- Explicit `--file` selection for every ledger operation; eligible local
  read-only commands also accept stdin with `--file -`.
- Portable publication packages that do not require Git or a hosted service.
- A local stdio MCP server with explicit named roots, optional whole-server
  read-only/offline modes, bounded requests, and a separate default-off reveal
  capability.

### Changed

- Removed hidden preview placeholders: advertised leaf commands now invoke real
  shared application services.
- Kept Forecast Ledger schema compatibility exact at v1.0.0. The CLI does not
  guess or download migrations for another schema version.

### Security and evidence limits

- OpenTimestamps support is experimental until differential, public-calendar
  liveness, native-platform, and independent-review gates pass.
- Sealed operations write protected key files before ledger mutation and report
  recoverable retained-key states after an interrupted commit.
- Verification does not prove authorship, completeness, forecast truth,
  calibration, exact self-reported time, or outcome-source authority.
- A cryptographically valid anchor that is not before the known outcome is
  retained with a warning and fails the layered timing-sufficiency check.
- Public timestamp calls reveal request timing and, during Bitcoin checks, the
  block heights of interest to the fixed public services.

## 0.1.1 - 2026-08-25

- Published local ledger validation, status, and build/schema version metadata.
