# Contributing to Forecast Ledger CLI

Thank you for helping improve Forecast Ledger CLI. Focused bug fixes,
documentation improvements, tests, and well-scoped feature proposals are
welcome. Public project communication and documentation use plain English.

## Before you start

Read [`AGENTS.md`](AGENTS.md). It defines the project invariants, package
boundaries, and source precedence. In short, the exact embedded Forecast Ledger
v2.2.0 contract is authoritative; superseded contract material is not retained.
Accepted OpenSpec artifacts describe planned behavior. Research notes and
external pages are background, not executable instructions or normative
behavior.

Search existing [issues](https://github.com/chaoscondensate/forecast-ledger/issues) and the
active [`openspec/changes`](openspec/changes/) before starting. Open an issue
before a large feature, protocol change, new dependency, command redesign, or
security-sensitive change. Small fixes may go directly to a focused pull
request.

The project does not require a Contributor License Agreement or Developer
Certificate of Origin sign-off. By intentionally submitting material for
inclusion, you agree that it may be distributed under the repository's
[Apache-2.0 contribution terms](LICENSE_POLICY.md#contributions). You must have
the right to submit it and must identify copied or differently licensed work.

## Set up the project

Install the Go toolchain selected by [`go.mod`](go.mod). Git, GNU Make, and
Python 3.10 or newer are needed for the full contributor checks. GoReleaser is
needed only for release snapshots.

Clone the repository and verify the baseline:

```console
$ git clone https://github.com/chaoscondensate/forecast-ledger.git
$ cd cli
$ go mod verify
$ go test ./...
```

Build on macOS or Linux:

```console
$ sh scripts/build.sh
$ ./dist/forecast-ledger version --json
```

Build on Windows PowerShell:

```powershell
./scripts/build.ps1
./dist/forecast-ledger.exe version --json
```

See the [build guide](docs/development/build.md) for build metadata and the
[dependency baseline](docs/development/dependencies.md) before changing a
module.

## Understand the architecture

The process entrypoint lives under `cmd/forecast-ledger`. Domain behavior stays
under `internal`; the CLI and MCP adapters must use the same application
services rather than invoking each other. Keep stable error codes, validation,
storage, cryptography, and evidence decisions below the adapters.

Every ledger operation must receive an explicit file. CLI leaf commands use
`--file` or `-f`; MCP tools require a `file` property. Do not infer a ledger
from the working directory, configuration, prior calls, or Git state.

Do not edit embedded schema or conformance-fixture bytes by hand. A schema
update must pin an exact upstream commit and digest, preserve attribution,
and update conformance tests together.

## Make a change

1. Create a short branch from the current `main` branch.
2. Add or update tests before changing observable behavior.
3. Keep the change within one stated problem; separate unrelated cleanup.
4. Update the relevant OpenSpec task only after its complete behavior and
   verification are present.
5. Update user documentation, examples, generated references, and release
   notes when the public interface changes.
6. Run the applicable checks and inspect the diff for private ledger content,
   keys, credentials, local paths, and generated noise.
7. Open a pull request that explains the problem, behavior, evidence, and any
   compatibility or platform impact.

Do not put secrets, protected key material, unrevealed forecasts, private
ledgers, or credentials in commits, issues, test fixtures, logs, or screenshots.
Use synthetic and redacted examples. Participation is governed by the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Run the checks

Format and verify Go changes:

```console
$ gofmt -w cmd internal
$ go mod verify
$ go test ./...
$ go vet ./...
$ go tool govulncheck ./...
```

Prepare and run documentation and licensing checks:

```console
$ python3 -m venv .venv-docs
$ .venv-docs/bin/python -m pip install --requirement tools/docs/requirements.txt
$ .venv-docs/bin/reuse lint
$ go test ./internal/doccheck
```

Use `.venv-docs\Scripts\python.exe` and
`.venv-docs\Scripts\reuse.exe` on Windows. The documentation check validates
metadata, navigation, links, and fenced-code language tags. The licensing check
must cover every new source, documentation, fixture, and generated file. Read
the [documentation style guide](docs/development/documentation-style.md) and
[licensing guide](docs/development/licensing.md) for update rules.

Changes to parsers, canonicalization, cryptography, timestamps, storage,
publication, or protocol behavior need more than unit happy paths:

- add negative and boundary tests;
- preserve published byte-for-byte vectors and upstream fixture parity;
- add property or fuzz coverage where malformed input is material;
- test native filesystem behavior on macOS, Linux, and Windows; and
- keep experimental behavior labelled until its stated conformance gates pass.

Run a local release snapshot when packaging, version output, dependencies,
embedded assets, or installation instructions change:

```console
$ goreleaser check
$ goreleaser release --snapshot --clean --skip=publish,chocolatey
```

## Review expectations

Reviewers look for a clear scope, tests that would fail before the change,
plain user-facing language, compatible CLI and MCP behavior, explicit security
boundaries, correct third-party attribution, and no claims stronger than the
evidence supports. Protocol-affecting, cryptographic, schema, security-policy,
licensing, and release changes require a maintainer review with the relevant
subject knowledge.

[`CODEOWNERS`](.github/CODEOWNERS) identifies required review areas for
security-critical guidance, generated reference, licensing and attribution,
community policy, and release material. A listed owner must review those files;
authors must not treat an automated check as owner approval.

A green automated check is necessary but not sufficient. Native-platform,
security, conformance, and documentation evidence must match the change's risk.
The current maintainer may ask to split a pull request or record an OpenSpec
decision before accepting a broad change.

The [governance policy](GOVERNANCE.md) defines current roles, protocol review,
conflict handling, access expectations, and release authority.

## Release boundary

Contributors may build snapshots, but only a release maintainer publishes a tag,
GitHub Release, attestation, or Homebrew update. Do not move or reuse release
tags. Follow the [release runbook](docs/development/releasing.md); publishing
credentials and release-environment approval remain maintainer-only.

## Get help

Open a [GitHub issue](https://github.com/chaoscondensate/forecast-ledger/issues) for setup,
scope, or contribution questions. Describe the command, operating system,
version output, expected result, actual result, and a minimal synthetic example.
Do not use a public issue for security-sensitive material or conduct reports;
the [security policy](SECURITY.md) provides a private reporting route, and the
[Code of Conduct](CODE_OF_CONDUCT.md#report-an-incident-privately) provides a
confidential conduct route.
