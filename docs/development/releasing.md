# Release process

<!-- doc-metadata
coverage: current-main
reviewed: 2026-09-22
owner: release
generated: false
security-critical: true
prerequisites: build.md, dependencies.md
next: index.md
-->

Forecast Ledger releases use GoReleaser 2.18.0. A pushed annotated SemVer tag
creates a GitHub Release, six cross-platform archives, eight native Linux
packages, one Windows Chocolatey package, SHA-256 checksums, SBOMs, GitHub
artifact attestations, and a Homebrew formula update.

Before attesting a release, the workflow compares every checksum entry's name
and digest with the published GitHub asset. Prerelease Linux package filenames
avoid characters that GitHub normalizes during upload, so the downloaded files
remain directly verifiable with the published checksum manifest.

The release targets are:

- macOS: amd64 and arm64
- Linux: amd64 and arm64
- Windows: amd64 and arm64

Unix archives use `tar.gz`; Windows archives use `zip`. Linux also receives
`deb`, `rpm`, `apk`, and Arch Linux packages for both architectures. Windows
x86-64 receives a Chocolatey `nupkg`; Windows ARM64 uses its native ZIP. All
builds use `CGO_ENABLED=0`, `-trimpath`, read-only modules, and embedded version
and source revision metadata.

The Linux packages and Chocolatey package are GitHub Release downloads, not
published package repositories. The release runs on Windows, where GoReleaser
can use the preinstalled Chocolatey CLI while still cross-building the macOS
and Linux artifacts. GoReleaser retains the generated `nupkg` locally instead
of treating it as a GitHub Release asset, so the workflow explicitly attaches
that package before checking the release and installing and removing the
package. GoReleaser creates the `nupkg` after `checksums.txt`, so the package
receives a separate GitHub artifact attestation rather than a checksum-manifest
entry.

## One-time repository setup

1. Confirm that the top-level Apache-2.0 `LICENSE` and public `README.md` are
   present and accurate. The release workflow refuses to publish without them.
2. Create the public `chaoscondensate/homebrew-tap` repository and make an
   initial README commit on `main`; an empty repository has no branch for the
   release workflow to update. The workflow creates and maintains
   `Formula/forecast-ledger.rb`.
3. Create a fine-grained GitHub personal access token or GitHub App credential
   with `Contents: Read and write` access to only
   `chaoscondensate/homebrew-tap`. It does not need access to this repository.
4. Add that credential to this repository as the Actions secret
   `HOMEBREW_TAP_TOKEN`.
5. Configure the `release` GitHub environment. Require approval for stable
   releases and allow release tags from protected `main`.
6. In repository Actions settings, keep the default `GITHUB_TOKEN` restricted.
   The release workflow grants only `contents`, `id-token`, and `attestations`
   permissions explicitly.
7. Protect `main` with the `CI` workflow and prevent release-tag deletion or
   movement if the organization policy supports it. Do not enable immutable
   releases while the workflow attaches the Chocolatey package after GoReleaser
   publishes the GitHub Release.

The tap requires a separate token because GitHub's workflow token cannot write
to a different repository. Never reuse a broad classic PAT when a tap-only
fine-grained credential is available.

## Pre-release checks

Run these commands from a clean checkout:

```sh
gofmt -w cmd internal
go mod verify
go test ./...
go vet ./...
go tool govulncheck ./...
goreleaser check
goreleaser release --snapshot --clean --skip=publish,chocolatey
```

For a Forecast Ledger contract update, also run the generated-contract parity
tests, documentation examples, and removed-surface denylist. For v2.0.1,
confirm together schema commit `55b1431d379128e1d75b9c30a3874398cea9ff0f`,
annotated tag object `ac69f718de92f2c087b44b1b02d325ba37945db6`,
release archive digest
`bfe00b166efdef229848afd6d623eeb965507f314482e59b9c0663bb87ff7a41`,
checksum-asset digest
`fa4c72bc5f4f27dd936e95cf9ad61fdee7c1d12ecb17ca847ac00b919c8664d6`,
embedded schema digest
`5ceeeca7b2b3e46884c116aeb4f20998daf619569c0191728f574c245a4a595b`,
attribution, fixtures, reference semantics, the seal vector, and both
lifecycle-bearing target vectors as one identity. Do not publish while any of
those identities disagree.

Then inspect `dist/artifacts.json`, run each native binary available on the
current host, and confirm that `forecast-ledger version --json` contains the
expected version, commit, schema pin, Go version, and MCP protocol version.
Confirm that every archive contains `THIRD_PARTY_NOTICES.md` and that every
Linux package installs it under `/usr/share/doc/forecast-ledger/` alongside the
license material. Review the generated SBOMs rather than treating the notice as
a substitute for dependency inventory.
For a release that changes RFC 3161 support, also:

- run the checked-in fixtures through the Go verifier and the pinned OpenSSL
  commands recorded in `internal/timestamp/rfc3161/testdata/README.md`;
- validate the compiled provider catalog, embedded PEM digest, source record,
  trust expiry, exact HTTP/HTTPS transport permission, and generated provider
  metadata;
- build and locally verify a timestamped package with `proofs/` and `trust/`
  beside `ledger/`; and
- run the FreeTSA canary separately when qualification or profile drift needs
  checking. Reachability is never an ordinary CI or release gate.

Do not add a built-in provider, change its endpoint or transport, or replace
its trust bytes as a release-only edit. Those changes require reviewed catalog
provenance, fixtures, security documentation, and local verification in the
same source change. Custom provider URLs remain HTTPS-only.

On Windows, also validate the Chocolatey package with:

```powershell
goreleaser release --snapshot --clean --skip=publish
```

The CI workflow repeats tests on Ubuntu, macOS, and Windows, builds the archives
and Linux packages on Ubuntu, and validates the complete artifact matrix again
on Windows, including Chocolatey package generation. Treat either snapshot
failure as a release blocker.

Before releasing the v2.0.1 correction, exercise relationship mutations,
same-tick question revision defaults, ledger-timezone operation defaults,
protected-file diagnostics, locks, safe replacement, path confinement, and
interruption recovery on native macOS, Linux, and Windows jobs. A local pass or
cross-build does not replace those native checks.

## Publishing

Start with a release candidate:

```sh
git switch main
git pull --ff-only
git status --short
git tag -a v0.1.2-rc.1 -m "forecast-ledger v0.1.2-rc.1"
git push origin v0.1.2-rc.1
```

GoReleaser marks prerelease tags as prereleases and does not update the stable
Homebrew formula. After checking the candidate archives on macOS, Linux, and
Windows, publish the stable tag:

```sh
git tag -a v0.1.2 -m "forecast-ledger v0.1.2"
git push origin v0.1.2
```

A stable `0.x` tag identifies the supported package channel. It does not claim
an independent security or cryptographic audit. If no independent reviewer is
available, retain the public no-audit warning and track external review as
non-blocking follow-up work instead of marking an audit complete.

Do not reuse or move a release tag. Fix a failed or incorrect release in a new
patch or prerelease version.

### Recovering a partial stable release

If GoReleaser publishes the GitHub assets but cannot attach Chocolatey or update
the Homebrew tap, do not rerun the full release against the same tag. Correct
the workflow or `HOMEBREW_TAP_TOKEN`, then run the `Recover stable release`
workflow with the current latest stable tag. The workflow:

- accepts only an annotated stable SemVer tag that matches GitHub's latest
  stable release;
- verifies that the tap token has push access before rebuilding anything;
- rebuilds the Chocolatey package from the exact tag, binds it to the checksum
  of the already-published Windows archive, uploads it when missing or replaces
  it only when that embedded checksum is invalid, then downloads, attests, and
  tests the published package bytes;
- regenerates the formula without republishing release assets;
- binds every formula archive digest to the already-published `checksums.txt` and
  validates the generated Ruby syntax;
- updates `Formula/forecast-ledger.rb`, removing the obsolete cask if present;
  and
- creates checksum-based artifact attestations that the interrupted release
  could not reach.

The normal release workflow performs the same tap-access preflight before
GoReleaser, so an invalid token fails before any GitHub Release is published.

## Post-release verification

1. Confirm the GitHub Release has six archives, eight Linux packages, one
   Chocolatey package, checksum files, and SBOM files.
2. Download the selected archives, Linux packages, or Chocolatey package and
   verify archives and Linux packages with `sha256sum -c checksums.txt` or
   `shasum -a 256 -c checksums.txt`. Verify the Chocolatey package with its
   separate GitHub artifact attestation.
3. Verify provenance with:

   ```sh
   gh attestation verify --owner chaoscondensate <archive>
   ```

4. Check that the tap contains `Formula/forecast-ledger.rb` for the new stable
   version.
5. On both Apple Silicon and Intel macOS, run:

   ```sh
   brew update
   brew install chaoscondensate/tap/forecast-ledger
   forecast-ledger version --json
   brew uninstall forecast-ledger
   ```

6. Install and remove at least one `deb`, `rpm`, `apk`, and Arch package on its
   native distribution. Confirm `/usr/bin/forecast-ledger` and the packaged
   license are removed cleanly.
7. Install, upgrade, and remove the Chocolatey package on Windows x86-64. Also
   smoke-test the native Windows ARM64 ZIP.
8. Smoke-test help, version, validation, and MCP startup without protocol output
   corruption on native Linux and Windows hosts.
9. Update installation documentation and announce the release through
   <https://chaoscondensate.com/> when appropriate.

## Signing status

The initial pipeline creates checksums, SBOMs, and GitHub artifact attestations,
but it does not Apple-sign or notarize the macOS binaries. The Homebrew formula
removes quarantine metadata and applies a local ad-hoc signature after placing
the binary in the Cellar, before it generates shell completions. This status
must be stated in release notes.

Before calling the release channel stable, prefer Apple Developer ID signing
and notarization, remove the formula's `xattr` and `codesign` install fallback,
and add a native verification step for the signed archives. Windows code
signing is also not yet configured and must be documented as such. The Linux
packages and Chocolatey package are not package-signed; users must verify their
published checksums or GitHub attestations as documented for each format.

[Development documentation](index.md) · [Documentation index](../index.md)
