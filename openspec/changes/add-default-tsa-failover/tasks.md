> v2 cutover note (2026-09-21): `adopt-forecast-ledger-v2` owns the current
> target and ledger shapes. All unfinished provider, verification, publication,
> dogfood, and release tasks below run against v2 `forecast-envelope/v2`
> targets and v2 ledgers; older target names are historical context only.

## 1. Reconcile the RFC 3161 Contract and Qualify the Current Provider

- [x] 1.1 Reconcile the completed `replace-ots-with-rfc3161` and `complete-forecast-ledger-command-surface` artifacts and current project invariants so their explicit-TSA-only requirements are visibly superseded by this change without rewriting historical release facts.
- [x] 1.2 Record the dated, source-linked qualification result: FreeTSA admitted for anonymous arbitrary-hash use, DigiCert unresolved pending affirmative permission, and GlobalSign `r45standard` plus Sectigo excluded for their reviewed code-signing restrictions.
- [x] 1.3 Confirm FreeTSA's exact endpoint, HTTPS transport, request media type, redirect behavior, response profile, operator identity, rate guidance, best-effort availability, certificate rotation, and service limitations from current first-party material.
- [x] 1.4 Retrieve FreeTSA CA material from the operator-controlled source, normalize exact PEM bytes, validate CA constraints and the current response chain, and record source URL, retrieval date, certificate identities, expiry, and SHA-256 digest without the system root store.
- [x] 1.5 Capture byte-exact synthetic target, `.tsq`, `.tsr`, and trust fixtures for the current FreeTSA ESS v1/SHA-512 profile plus retained OpenSSL ESS v2 and strong-digest profiles, with no private forecast data or caller identity.
- [x] 1.6 Verify every positive fixture independently with pinned OpenSSL commands and record exact commands, versions, expected metadata, and provenance; keep live services out of ordinary tests and releases.

## 2. Restore RFC 3161 and CMS Interoperability

- [x] 2.1 Retain and review the exact `tspclient-go v1.0.0` parser pin and upstream commit, document why no fork is required, and keep all new trust decisions in the bounded project-owned verifier layer.
- [x] 2.2 Add bounded DER parsing and validation for ESS `SigningCertificate` v1 with exactly one unambiguous signer certificate, signed certificate-hash verification, signer-SID agreement, and retained-chain compatibility.
- [x] 2.3 Preserve and harden `SigningCertificateV2` verification, reject missing, duplicate, empty, malformed, excessive, unsupported, or non-matching bindings, and require v1 and v2 to identify the same certificate when both are present.
- [x] 2.4 Add typed project errors for response status, policy, nonce, message imprint, ESS signer binding, token structure, algorithm, CMS signature, certificate profile, and trust so callers never parse flattened dependency error strings.
- [x] 2.5 Keep request and target message imprints fixed at SHA-256 while accepting strong SHA-256, SHA-384, and SHA-512 CMS signer digests and supported RSA/ECDSA signatures; reject SHA-1/MD5 signer digests, weak keys, and weak certificate signatures.
- [x] 2.6 Map typed verifier failures to honest stable application reason codes so ESS/profile failures cannot become `rfc3161.binding_mismatch` after nonce and imprint binding pass.
- [ ] 2.7 Run the current FreeTSA ESS v1/SHA-512 fixture and retained ESS v2, dual-ESS, SHA-256, SHA-384, and SHA-512 fixtures through request/response parsing, signature/chain verification, metadata extraction, status, layered verification, and publication verification.
- [ ] 2.8 Add negative mutations for ESS hash, issuer/serial, signer SID, v1/v2 disagreement, duplicate attributes, missing signer certificate, malformed DER/BER, trailing data, signer digest, CMS signature, EKU, key strength, chain, nonce, policy, imprint, and target.
- [ ] 2.9 Add property and bounded fuzz coverage for ESS v1/v2, signed attributes, CMS signer selection, strong digest variants, typed-error classification, and all existing byte/count/depth limits; retain every discovered regression corpus.
- [x] 2.10 Confirm `go.mod`, `go.sum`, dependency documentation, license inventory, third-party notices, vulnerability policy, and reproducibility checks retain the exact upstream parser pin without importing a general signing or PKI framework.

## 3. Add the Versioned Provider and Transport Catalog

- [x] 3.1 Implement an immutable catalog containing only `freetsa` with its exact HTTPS endpoint, order, embedded PEM bundle, bundle digest, source metadata, expiry visibility, and request guidance.
- [x] 3.2 Model built-in transport policy separately as exact HTTPS or exact HTTP, default to HTTPS, and make it impossible for CLI, MCP, environment, config, or URL input to construct HTTP permission.
- [x] 3.3 Add catalog self-checks for stable unique IDs, exact order, endpoint normalization, transport policy, CA parsing, source attribution, embedded-byte digests, portable trust paths, and absence of credentials, headers, queries, fragments, or dynamic configuration.
- [x] 3.4 Materialize the selected embedded bundle to deterministic `trust/rfc3161/<provider>-<digest>.pem` through the existing confined recoverable resource transaction and reject different-byte collisions before network access.
- [x] 3.5 Enforce public DNS/IP validation on every connection, disable every built-in redirect, and preserve request deadline, media-type, response-size, body, proxy, credential, and safe-diagnostic limits for both transport policies.
- [ ] 3.6 Add transport tests for shipped FreeTSA HTTPS, synthetic exact built-in HTTP success, lookalike host/path rejection, case and port variants, DNS rebinding/private resolution, all redirects, proxy/environment isolation, truncated/oversized bodies, wrong media types, and custom HTTP rejection before effects.

## 4. Implement Transactional Automatic Provider Selection

- [x] 4.1 Add a transport-neutral selection model for `auto`, one named catalog provider, or the explicit custom URL/bundle pair, including mutual-exclusion and required-pair validation before entropy, files, mutation, or sockets.
- [x] 4.2 Extend stamp preflight to validate the ledger, forecast, exact target, every current catalog path and embedded bundle, existing entries, and collision/idempotency state before the first automatic request.
- [x] 4.3 Reverify and reuse an already complete matching built-in entry without network, entropy, or writes; safely stop on existing non-matching artifacts rather than overwriting durable evidence.
- [x] 4.4 Implement ordered catalog attempts with a fresh nonce/request per provider, the caller's one overall deadline, a closed fallback-eligible error set, and immediate stop after the first fully verified response; current production auto makes at most one FreeTSA request.
- [x] 4.5 Re-read under ledger/resource locks after network I/O, prove the forecast, target, destinations, and selected bundle unchanged, and atomically commit only the successful target/request/response/trust/ledger set with recoverable journal semantics.
- [x] 4.6 Make all-failure automatic mode a no-op and aggregate all-unavailable as `network`/exit `8` versus any invalid response as `verification`/exit `6`.
- [x] 4.7 Add ordered bounded attempt results with provider ID, ordinal, attempted state, stable reason, request count, and selected provider while excluding request/response bytes, certificates, IPs, unrestricted transport errors, and private ledger data.
- [x] 4.8 Preserve single-provider pending retention, safe partial result, retry, and idempotency for explicit custom TSA mode; named built-in mode uses embedded trust and built-in all-failure no-op semantics.
- [ ] 4.9 Add deterministic service tests for FreeTSA success/unavailable/invalid, synthetic second/third HTTPS and HTTP fallback success, invalid-plus-unavailable, global timeout, interruption, non-fallback local errors, retry, existing evidence, trust collision, and no-effects guarantees.
- [ ] 4.10 Add concurrency tests that mutate the selected forecast, ledger metadata, target, trust path, or timestamp destinations during provider I/O and prove conflict without partial resources.
- [x] 4.11 Prove dry-run reports the current ordered provider/resource plan without entropy, sockets, or writes, and offline mode returns `network_disabled` before provider effects in CLI and MCP.

## 5. Update CLI, MCP, Results, and Command Errors

- [x] 5.1 Make CLI `--tsa-url` and `--ca-bundle` optional as a required-together custom pair, add optional `--tsa-provider` with current values `auto|freetsa`, and default omission to `auto` without changing explicit ledger or forecast selectors.
- [x] 5.2 Add optional MCP `tsa_provider`, make `tsa_url`/`ca_bundle` a required-together custom pair, enforce the same mutual exclusions and server-wide offline/read-write boundaries, and keep provider selection below adapters.
- [x] 5.3 Extend stable JSON, human, plain, and MCP results with selection mode, selected provider, safe ordered attempts, request count, retained trust path/digest, and honest aggregate failure without exposing remote bodies or certificates.
- [x] 5.4 Regenerate CLI/MCP operation schemas, fixtures, examples, help goldens, tool descriptions, and documentation contracts; assert CLI and MCP selection parity.
- [ ] 5.5 Add loopback adapter tests for zero-argument automatic success/failure, named FreeTSA, custom pair, mixed inputs, unknown provider, dry-run, offline, timeout, redaction, session survival, stdout/stderr separation, non-TTY behavior, and exits `0`, `6`, and `8`.
- [x] 5.6 Normalize unknown child-command and help lookup handling at root and every command group so absent commands return stable usage exit `2` with and without `--help`, never an internal error.
- [ ] 5.7 Add table-driven human/plain/JSON and TTY/non-TTY regression tests for `timestamp upgrade`, other removed names, arbitrary unknown children, suggestions, and help flags without hidden tombstone commands.

## 6. Repair Portable Publication Verification

- [x] 6.1 Make package verification own package-aware ledger admission and bypass generic CLI ledger-directory preflight for `publish verify` without bypassing schema-version, schema, semantic, manifest, or evidence validation.
- [x] 6.2 Ensure CLI and MCP resolve packaged ledger artifact references against the manifest directory while the selected byte-exact ledger remains under `ledger/`; keep service and adapter roots identical and explicit.
- [x] 6.3 Build and verify unmodified timestamped packages containing `ledger/`, `proofs/targets/`, `proofs/timestamps/`, `trust/`, and `manifest.json` through service, black-box CLI, and live MCP tests.
- [ ] 6.4 Add package negatives for missing, empty, moved-under-ledger, duplicated, unexpected, tampered, digest-mismatched, path-colliding, traversing, symlinked, non-regular, oversized, wrong-role, and wrong-ledger artifacts.
- [x] 6.5 Confirm publication build/verify retain exact request/response/CA bytes, manifest v2 roles, stable ledger paths, local RFC verification, one-of-many timestamp semantics, pending/fail honesty, and no network fallback.
- [x] 6.6 Add the exact dogfood package-layout regression and assert `publish verify` no longer returns `semantic.artifact_missing` solely because proofs and trust are siblings of `ledger/`.

## 7. Document Trust, Privacy, Providers, and Maintenance

- [x] 7.1 Update `AGENTS.md` and active OpenSpec material to replace explicit-TSA-only invariants with the current FreeTSA catalog, extensible exact HTTPS/HTTP built-in transport policy, retained trust, custom HTTPS override, local verification, and no-live-test boundary.
- [x] 7.2 Update README, getting started, timestamp how-to, CLI/MCP references, generated examples, and first-use flows so the minimal command has no TSA arguments and named/custom forms remain accurate.
- [x] 7.3 Update security and verification-claim guidance for FreeTSA's best-effort/key-person trust, absence of SLA/audit, ESS v1's bounded SHA-1 identifier, strong signer digests, future HTTP observation/DoS limits, retained CA trust, rotation, and no legal/independence claims.
- [x] 7.4 Document the package-root layout and demonstrate build-then-verify commands using actual `ledger/`, `proofs/`, `trust/`, and manifest paths on macOS, Linux, Windows, and MCP.
- [x] 7.5 Add a maintained provider provenance and rotation page with doc metadata, navigation, source links, bundle digests, review dates, admission/exclusion state, transport policy, canary interpretation, and replacement procedure.
- [x] 7.6 Add a manual and low-frequency scheduled FreeTSA synthetic canary that respects guidance, verifies locally, emits only safe metadata/digests, uses no private forecast, and cannot gate ordinary CI or releases.
- [x] 7.7 Update dependency, third-party notice, documentation baseline, release instructions, changelog, maturity, network-boundary, and support material for the project-owned verifier, FreeTSA outbound default, transport model, and package fix.
- [x] 7.8 Run doccheck, release/citation checks, link checks, generated examples, help assertions, plain-English checks, and scoped stale-claim scans.

## 8. Verify and Prepare the Minor Release

- [x] 8.1 Run formatting, module verification, generated-contract drift, catalog/bundle digest, parser-pin provenance, fixture digest, and focused RFC/provider/service/storage/publication/adapter/validation/documentation tests.
- [x] 8.2 Run `go test ./...`, affected race tests, `go vet ./...`, and pinned `govulncheck`; resolve every reachable vulnerability, regression, race, unsafe error, or dependency discrepancy.
- [ ] 8.3 Run bounded fuzz targets for RFC request/response, ESS v1/v2, CMS attributes, certificate selection, provider/transport inputs, manifest paths, and result/error classification; retain regressions.
- [ ] 8.4 Re-run independent OpenSSL verification for every profile fixture and negative differential case with correct failure layers.
- [ ] 8.5 Cross-build supported macOS, Linux, and Windows targets and run required native command, filesystem, permission, recovery, package, DNS/IP, redirect, timeout, and loopback tests.
- [ ] 8.6 Build the pinned GoReleaser and Chocolatey snapshot matrix, verify archives/packages/SBOMs/checksums/attestations, smoke-test installs, and confirm default help/provider metadata.
- [x] 8.7 Run the FreeTSA live canary separately after qualification, record safe profile drift, and do not treat reachability as proof of trust quality or future availability.
- [ ] 8.8 Run strict OpenSpec validation for this and reconciled active changes, verify every task/documentation impact, update version/citation/release notes for one coherent minor release, and hand off a clean release-ready tree; publish only through the explicit release workflow.
