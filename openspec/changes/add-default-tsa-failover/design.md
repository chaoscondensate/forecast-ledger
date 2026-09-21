> v2 cutover note (2026-09-21): `adopt-forecast-ledger-v2` supersedes the
> historical v1 target examples in this design. The provider and transport
> design is unchanged, but its current message imprint is over the exact v2
> `forecast-envelope/v2` target.

## Context

See `proposal.md` for motivation and
`specs/default-tsa-failover/spec.md` for observable behavior. Version 0.4.0 has
the v1.2.0 RFC 3161 data model and a bounded local verifier, but stamping still
requires an explicit HTTPS endpoint and PEM bundle.

Dogfood against real public responders exposed three implementation gaps:

- `tspclient-go v1.0.0` resolves only the RFC 5816
  `SigningCertificateV2` attribute. The application calls that validation
  before its later certificate checks and flattens its errors to
  `rfc3161.binding_mismatch`, although imprint and nonce may already match.
- The product requires the CMS signer digest itself to be SHA-256, conflating
  the ledger's SHA-256 message imprint with a separate strong signature choice.
  FreeTSA currently signs with SHA-512.
- Package-aware validation exists in the service, but the CLI leaf preflight
  first validates the ledger relative to `package/ledger/`. This rejects the
  emitted sibling `package/proofs/` and `package/trust/` layout before
  publication verification can select the package root.

The removed-command help path also misclassifies
`timestamp upgrade --help` as internal exit `3` instead of usage exit `2`.

Provider qualification changed the original catalog design. FreeTSA's
first-party guide explicitly documents anonymous RFC 3161 requests over hashes
of arbitrary files and publishes an HTTPS endpoint and certificate bytes.
DigiCert publishes an HTTP RFC 3161 endpoint and chain but does not affirmatively
authorize unauthenticated arbitrary-hash use. GlobalSign's reviewed
`r45standard` endpoint emits a code-signing policy and its current subscriber
agreement ties the non-chargeable service to code signed with a GlobalSign code
signing certificate. The current released catalog therefore contains only
FreeTSA.

Primary operator references:

- [FreeTSA service, endpoint, certificates, and rotation](https://freetsa.org/index_en.php)
- [FreeTSA CPS](https://www.freetsa.org/freetsa_cps.html)
- [DigiCert RFC 3161 endpoint and chain](https://knowledge.digicert.com/general-information/rfc3161-compliant-time-stamp-authority-server)
- [GlobalSign subscriber agreement](https://www.globalsign.com/en/repository/GlobalSign_Subscriber_Agreement.pdf)
- [GlobalSign timestamping practice statement](https://www.globalsign.com/en/repository/GlobalSign-TPS-v1.1.pdf)

The schema remains exact v1.2.0. Provider and transport choice are application
policy; every successful timestamp still records ordinary `tsa_url`, request,
response, CA-bundle path, and verified metadata.

## Goals / Non-Goals

**Goals:**

- Make the zero-configuration online path complete in one invocation while
  retaining an explicit, reviewable provider trust choice.
- Keep the provider-selection service ready for an ordered catalog without
  claiming unqualified providers today.
- Support exact built-in HTTPS and HTTP transport profiles while preventing
  caller-controlled HTTP.
- Accept the strong standards-defined response variants emitted by the current
  qualified provider without accepting weak message imprints or signatures.
- Preserve one shared service implementation and result contract across CLI and
  MCP.
- Make the build-emitted publication layout verify without copying artifacts.

**Non-Goals:**

- Claiming FreeTSA is qualified for a legal regime, independently audited,
  enterprise-grade, always available, free forever, or suitable for every use.
- Shipping DigiCert, GlobalSign, Sectigo, or another provider before its exact
  endpoint, usage policy, transport, and trust material pass qualification.
- Sending one forecast to multiple providers after the first verified success;
  redundancy remains an explicit later stamp.
- Dynamically downloading provider lists, trust bundles, CRLs, OCSP responses,
  or catalog updates at stamping or verification time.
- Allowing arbitrary HTTP TSAs, credentials, custom headers, proxies, or
  environment-selected providers.
- Changing the v1.2.0 schema, migrating evidence, or adding RFC 4998 renewal and
  long-term revocation archival.
- Making live public services a unit-test, conformance-test, snapshot, or
  ordinary release dependency.

## Decisions

### 1. Treat providers and transports as separate versioned product profiles

Add a small catalog below `internal/timestamp`. Each immutable entry contains a
stable provider ID, exact normalized endpoint, explicit transport policy,
exact PEM bundle, bundle SHA-256, operator source URL, source review date, and
request guidance. The current catalog contains only:

- `freetsa`: `https://freetsa.org/tsr`, HTTPS, embedded FreeTSA CA bytes.

The catalog and bundle invariants are validated in tests and build checks. No
remote profile is parsed at runtime. `auto` iterates the released order;
`--tsa-provider`/`tsa_provider` selects one released entry. The explicit
custom URL and CA path remain a separate mutually exclusive mode.

Transport policy is an enum owned by the compiled profile, not inferred from
caller input. HTTPS is the default. The catalog type can also represent an
exact HTTP endpoint for a later qualified provider. No current HTTP entry ships.
Tests use private synthetic catalog entries to prove both transport paths and
the boundary that prevents any runtime input from creating HTTP permission.

Materialize the selected embedded CA bytes at
`trust/rfc3161/<provider>-<bundle-prefix>.pem`. Verification never needs to
know that the bundle began as a built-in resource.

Alternative considered: hard-code FreeTSA directly in the command. That would
couple provider choice, transport, and adapter behavior and make a later
qualified provider another command redesign.

Alternative considered: ship technically reachable DigiCert and GlobalSign
profiles now. Endpoint reachability is not permission and fails the explicit
qualification gate.

Alternative considered: use the OS trust store or fetch CA files on first use.
Both make later verification depend on mutable external state.

### 2. Keep automatic selection transactional even with one current provider

The service accepts a selection model rather than putting provider logic in
adapters. Preflight validates the ledger, target, every current catalog
destination, embedded bundle, and artifact collision without entropy or
network. An already complete matching built-in entry is locally reverified and
returned unchanged.

For each candidate, generate a fresh request and nonce, submit it outside the
ledger lock, parse and verify it against that profile's embedded bundle, and
retain only a safe attempt result. Stop at the first verified response. Then
re-read under the existing ledger/resource locks and atomically commit only the
successful target/request/response/bundle/ledger set. If no attempt verifies,
write nothing.

The current catalog has one candidate, but the loop, result shape, deadline,
and closed fallback-eligible error set remain ordered and multi-provider-safe.
Tests inject deterministic two- and three-entry HTTPS/HTTP catalogs to prove
fallback without shipping those entries.

Explicit custom mode retains its current pending evidence because the caller
made that single trust choice and may need to repair it. Named built-in mode
uses embedded trust but follows automatic all-failure no-op behavior.

Alternative considered: create pending entries for failed built-in attempts.
That would retain unusable artifacts from an operation intended to hide
provider recovery details.

Alternative considered: issue candidates concurrently. It unnecessarily sends
the imprint to every provider and makes ordering nondeterministic.

### 3. Treat built-in HTTP as cryptographically authenticated but observable

Custom endpoint normalization remains public HTTPS-only, with no user info,
query, fragment, prohibited address, or unsafe redirect. A built-in provider
profile may opt into exact HTTP only when the qualification record names the
same operator-published origin and path.

Both built-in HTTPS and HTTP disable redirects and apply the existing DNS/IP
confinement on every dial. They send only the digest, nonce, algorithm OID, and
`certReq`, and subject responses to all size, media-type, parsing, binding,
CMS, and retained-chain checks.

HTTP permits interception, delay, loss, and observation but not a forged
verified token without a trusted TSA key. Documentation for any future HTTP
profile must state that network observers can see client IP, request time,
target digest, nonce, and fallback patterns. The current FreeTSA profile uses
HTTPS, which reduces this exposure.

Alternative considered: refuse to model HTTP until one ships. That would mix
transport capability with provider qualification and repeat the transport
redesign when an otherwise qualified operator publishes only HTTP.

Alternative considered: accept caller-selected HTTP because RFC 3161 signs the
response. That unnecessarily expands SSRF, privacy, downgrade, and user-error
surface and is rejected.

### 4. Keep the pinned parser and own the verifier policy

The Notary Project dependency intentionally follows a V2-only trust profile;
its public validation and signature-verification methods both call the V2-only
certificate selector. Its parsed token nevertheless exposes the bounded CMS
fields needed by a caller-owned verifier. No Forecast Ledger fork exists, and
creating one would add repository, release, patch-provenance, and module-drift
risks without adding parsing capability.

Keep the existing exact `tspclient-go v1.0.0` module and commit pin. Use it only
for request and bounded response/CMS parsing. Add a small project-owned verifier
layer inside `internal/timestamp/rfc3161` that:

- parse `SigningCertificate` v1 and `SigningCertificateV2` with explicit DER,
  count, length, algorithm, and ambiguity bounds;
- select exactly one CMS signer certificate from signed identifiers plus signer
  SID and validate every present ESS certificate hash;
- require v1 and v2, when both exist, to select the same certificate;
- exposes typed stages for nonce, imprint, policy, ESS, algorithm, CMS signature,
  and trust failures; and
- allow CMS signer digests SHA-256, SHA-384, and SHA-512 without changing the
  request/target imprint from SHA-256.

ESS v1 uses SHA-1 only to identify the DER signer certificate. It is reinforced
by signer SID, CMS signature, issuer/serial, CA signature, and retained chain.
SHA-1 remains rejected for message imprints, CMS digests, token signatures, and
certificate signatures.

The project-owned layer is the security boundary for byte/count/depth limits,
timestamping EKU, key strength, chain validation at `gen_time`, metadata, and
stable reasons. OpenSSL is an independent fixture oracle, not a runtime
dependency.

Alternative considered: skip ESS validation, fork the complete dependency, or
import a large general signing framework. Each weakens or expands the security
and supply-chain boundary more than the narrow project-owned verification
layer over the retained parser.

### 5. Separate imprint, signer digest, and certificate-signature policy

Keep `forecast-envelope/v1` target hashing and `MessageImprint` fixed at
SHA-256. Permit CMS `SignerInfo` digest SHA-256/384/512 and supported strong
RSA/ECDSA signatures. Continue to reject SHA-1/MD5 signer digests, weak keys,
and weak certificate signatures. Unsupported signer digests map to
`rfc3161.algorithm_unsupported`, not a target-binding failure.

### 6. Keep the package layout and move admission to package-aware verification

The package root is the manifest directory. Keep the manifest and evidence at
that root and the byte-exact source ledger under `ledger/`. Publication verify
owns package admission and uses the manifest-root resolver for semantic and
cryptographic artifact lookup.

Give CLI leaves an operation-specific preflight hook, or disable generic
`admitSupportedLedger` only for `publish verify`, so the adapter cannot reject
the package before the service sees its manifest. Apply the same contract in
MCP. Do not duplicate `proofs/` or `trust/` below `ledger/`.

### 7. Normalize unknown-child help before presentation

Handle an unknown child command in one group-level path before asking urfave to
render help for a nonexistent topic. Return a stable `usage` application error
and exit `2` for both ordinary and `--help` forms. Do not restore hidden
tombstone commands.

### 8. Treat provider provenance and health as maintained release inputs

Check in a provider provenance record containing:

- FreeTSA's exact endpoint, usage guide, CPS, certificate sources and digests,
  2026 signer rotation, best-effort availability language, personal contact,
  and lack of an independent audit or SLA;
- DigiCert as unresolved pending affirmative arbitrary-imprint permission; and
- GlobalSign `r45standard` and Sectigo as excluded for the reviewed use.

A maintainer reviews operator endpoint documentation, compatible usage terms,
CA lineage, certificate rotation, transport exposure, and request guidance
before a provider first ships and whenever a canary reports drift.

Add a manually dispatchable, low-frequency canary for released providers only.
It submits one non-secret synthetic digest, verifies locally, reports safe
metadata/digests, respects guidance, and never gates ordinary tests or releases.
Existing evidence always carries its original trust bytes.

## Risks / Trade-offs

- **[FreeTSA is a best-effort key-person-operated service]** → Describe no SLA
  or institutional assurance, fail without mutation, retain custom TSA control,
  and keep already obtained evidence independently verifiable.
- **[A future HTTP profile exposes metadata and permits denial or delay]** →
  Qualify it explicitly, pin the exact endpoint, disable redirects, use fresh
  nonces and deadlines, verify every response cryptographically, and document
  disclosure.
- **[ESS v1 contains a SHA-1 certificate identifier]** → Accept SHA-1 only for
  this signed legacy identifier and keep it forbidden in every signature and
  data-imprint role.
- **[A project-owned CMS verifier can diverge from independent tools]** → Keep
  the parsed subset narrow, add OpenSSL differential fixtures, negative and fuzz
  coverage, retain the exact parser pin, and document ownership.
- **[Automatic selection can contact more organizations in future releases]**
  → Version and publish the catalog, stop after success, expose named selection,
  and report attempts.
- **[Provider roots and intermediates rotate]** → Retain exact evidence trust
  bytes, run low-frequency drift canaries, and release catalog updates without
  rewriting old evidence.
- **[Skipping generic CLI admission could weaken package confinement]** →
  Move admission into the package-aware service and cover both adapters with
  traversal, symlink, manifest, and unexpected-file negatives.

## Migration Plan

1. Reconcile active RFC 3161 artifacts and record the provider qualification
   decision and exact FreeTSA trust/profile fixtures.
2. Review the pinned parser boundary and project-owned verifier; add ESS v1/v2, strong signer-digest,
   typed-error, OpenSSL-differential, negative, property, and fuzz coverage.
3. Add the one-entry provider catalog and separate HTTPS/HTTP transport policy,
   then implement transaction-safe automatic selection and retained trust.
4. Update CLI/MCP inputs, results, generated schemas, dry-run/offline behavior,
   help, and unknown-child normalization.
5. Correct package-aware preflight and add end-to-end emitted-layout tests.
6. Update invariants, user docs, security/privacy limits, provider maintenance,
   dependency documentation, and release metadata.
7. Run the security, conformance, documentation, adapter, package,
   cross-platform, snapshot, vulnerability, and native-network test matrix.
8. Release as a new minor pre-adoption version through the explicit release
   workflow. Rollback is a source/release revert to v0.4.0; built-in evidence
   remains ordinary locally verifiable RFC 3161 evidence.
