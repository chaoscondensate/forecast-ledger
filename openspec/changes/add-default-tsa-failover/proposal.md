## Why

RFC 3161 stamping currently requires every caller to discover a TSA endpoint and
retain the matching trust bundle before the first forecast can be stamped. A
qualified built-in profile can make the normal CLI and MCP path usable without
hiding the provider trust decision or weakening fully local verification.

## What Changes

- Make `timestamp stamp` and `timestamp_stamp` use a built-in `auto` profile
  when no TSA arguments are supplied.
- Ship FreeTSA as the only current built-in provider. It is the only reviewed
  candidate whose first-party material affirmatively documents anonymous RFC
  3161 timestamping of arbitrary file hashes without an account, certificate,
  or contract.
- Keep automatic selection as an ordered, versioned provider-catalog operation
  so later releases can add independently qualified providers without changing
  the CLI or MCP selection model. The current catalog has one entry, so `auto`
  currently makes at most one provider request.
- Model each built-in provider with an exact HTTPS or HTTP transport profile.
  The current FreeTSA profile uses HTTPS. A future built-in HTTP profile must be
  separately qualified, compiled in with an exact origin and path, and receive
  the same complete local cryptographic verification. Caller-controlled custom
  endpoints remain public HTTPS-only.
- Retain the successful provider identity, exact endpoint, request, response,
  and a materialized copy of the embedded CA bundle so verification remains
  offline and independent of later binary or system trust-store changes.
- Keep an optional built-in-provider selector and the existing explicit
  `tsa_url` plus `ca_bundle` pair for custom TSAs. Custom endpoints never enter
  automatic selection.
- Return a bounded attempt summary when a built-in provider fails, without
  committing failed-attempt artifacts or weakening the timing claim.
- Restore interoperability with valid public RFC 3161 responses: accept both
  ESS `SigningCertificate` and `SigningCertificateV2` signer bindings under
  strict unambiguous certificate checks, keep the forecast message imprint at
  SHA-256, and accept strong SHA-256, SHA-384, or SHA-512 CMS signer digests.
- Split request/imprint/nonce failures from signer-attribute, algorithm,
  signature, and trust failures so a valid token is not reported as
  `rfc3161.binding_mismatch` merely because its ESS attribute version is
  unsupported.
- Fix package verification so the byte-exact ledger can remain under `ledger/`
  while its ledger-relative `proofs/` and `trust/` artifacts remain rooted at
  the package directory. CLI and MCP preflight SHALL use the same package-aware
  artifact root as the verification service.
- Make removed subcommands return the same bounded usage error and exit `2`
  with or without `--help`; an absent command such as `timestamp upgrade` SHALL
  never surface as an internal error.
- Add a provider-qualification and maintenance gate covering official usage
  terms, endpoint documentation, trust roots, certificate rotation, respectful
  request rates, live canaries, and release documentation. Do not describe a
  provider as free, generally authorized, independent, or continuously
  available beyond evidence its operator publishes.
- Keep DigiCert outside the catalog until its operator affirmatively permits
  unauthenticated timestamping of arbitrary forecast imprints. Exclude the
  reviewed GlobalSign `r45standard` and Sectigo endpoints because their current
  published terms tie those services to code-signing customers or certificates.

## Capabilities

### New Capabilities

- `default-tsa-failover`: A qualified built-in RFC 3161 provider catalog,
  transport-profile enforcement, automatic verified-first-success selection,
  retained embedded trust, custom TSA overrides, and honest attempt reporting
  shared by CLI and MCP.

### Modified Capabilities

- None. The Forecast Ledger v1.2.0 file contract is unchanged; this change adds
  application behavior around its existing RFC 3161 fields.

## Impact

- CLI timestamp flags and help, MCP input schemas and tool descriptions, shared
  timestamp services, RFC 3161 endpoint policy, result contracts, and generated
  operation schemas.
- The pinned RFC 3161/CMS parser boundary, project-owned verifier, and conformance fixtures,
  including real-profile ESS v1/v2 and strong signer-digest cases.
- Embedded provider metadata and FreeTSA CA material, third-party attribution
  and provenance checks, publication packages, deterministic fixtures, and a
  scheduled provider-health check.
- README, getting-started and timestamp how-to material, reference pages,
  security guidance, network-boundary documentation, provider maintenance, and
  release checks.
- The default online operation contacts the current FreeTSA origin. Local
  validation, timestamp status/verify, publication verification, offline mode,
  and dry-run remain network-free.
> Provider selection remains applicable only where it agrees with
> `reset-cryptographic-profiles-and-evidence-store`; its old target/profile
> references are superseded.
