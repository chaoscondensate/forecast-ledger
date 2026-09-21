# Install Forecast Ledger CLI

<!-- doc-metadata
coverage: v0.9.0
reviewed: 2026-09-21
owner: release
generated: false
security-critical: true
prerequisites: index.md
next: ../development/releasing.md
-->

Forecast Ledger CLI ships portable archives for macOS, Linux, and Windows on
x86-64 and ARM64. It also ships native Linux packages and a Windows x86-64
Chocolatey package.

Release `v0.9.0` provides Homebrew, platform archives, native Linux packages,
and the Windows x86-64 Chocolatey package described here. Always check the asset
list on the selected
[GitHub Release](https://github.com/chaoscondensate/forecast-ledger/releases).

## Choose an asset

| System | Architecture | Preferred asset |
| --- | --- | --- |
| macOS | Apple Silicon or Intel | Homebrew or `darwin_arm64.tar.gz` / `darwin_x86_64.tar.gz` |
| Debian or Ubuntu | ARM64 or x86-64 | `arm64.deb` / `amd64.deb` |
| Fedora, RHEL, or openSUSE | ARM64 or x86-64 | `aarch64.rpm` / `x86_64.rpm` |
| Alpine Linux | ARM64 or x86-64 | `aarch64.apk` / `x86_64.apk` |
| Arch Linux | ARM64 or x86-64 | `aarch64.pkg.tar.zst` / `x86_64.pkg.tar.zst` |
| Windows | x86-64 | Chocolatey `.nupkg` or `windows_x86_64.zip` |
| Windows | ARM64 | `windows_arm64.zip` |

Use the package name shown on the release page. In commands below, replace
`VERSION` with the release version without the leading `v`, such as `0.9.0`.

## Verify a download

Download `checksums.txt` with the asset. On Linux, verify the downloaded files
that are present in the current directory:

```sh
sha256sum --ignore-missing -c checksums.txt
```

On macOS:

```sh
shasum -a 256 -c checksums.txt
```

`shasum` reports missing assets as errors when only one release asset was
downloaded; the downloaded asset must still report `OK`.

Release assets also have GitHub artifact attestations. If the GitHub CLI is
installed, verify an asset with:

```sh
gh attestation verify --owner chaoscondensate ./DOWNLOADED_ASSET
```

GoReleaser creates the Chocolatey package after the main checksum manifest, so
the `nupkg` is verified through its GitHub artifact attestation rather than an
entry in `checksums.txt`.

Checksums and attestations protect download integrity and provenance. They do
not make the current macOS or Windows binaries platform-signed.

## macOS with Homebrew

Install the stable release:

```sh
brew install chaoscondensate/tap/forecast-ledger
```

Upgrade or remove it:

```sh
brew update
brew upgrade forecast-ledger
brew uninstall forecast-ledger
```

Homebrew installs the matching Apple Silicon or Intel archive and generates
shell completions.

## Debian and Ubuntu

Install or upgrade the downloaded package:

```sh
sudo apt install ./forecast-ledger_VERSION_amd64.deb
```

Use the `arm64.deb` asset on ARM64. Remove it with:

```sh
sudo apt remove forecast-ledger
```

## Fedora and RHEL

Install or upgrade the downloaded RPM:

```sh
sudo dnf install ./forecast-ledger-VERSION-1.x86_64.rpm
sudo dnf upgrade ./forecast-ledger-VERSION-1.x86_64.rpm
```

Use the `aarch64.rpm` asset on ARM64. Remove it with:

```sh
sudo dnf remove forecast-ledger
```

On openSUSE, use `sudo zypper install ./PACKAGE.rpm` and
`sudo zypper remove forecast-ledger`.

## Alpine Linux

The release APK is not signed by an Alpine package repository key. Verify its
checksum and attestation first, then install or upgrade the downloaded package:

```sh
sudo apk add --allow-untrusted ./forecast-ledger_VERSION_x86_64.apk
sudo apk add --upgrade --allow-untrusted ./forecast-ledger_VERSION_x86_64.apk
```

Use the `aarch64.apk` asset on ARM64. Remove it with:

```sh
sudo apk del forecast-ledger
```

## Arch Linux

Install or upgrade the downloaded package:

```sh
sudo pacman -U ./forecast-ledger-VERSION-1-x86_64.pkg.tar.zst
```

Use the `aarch64.pkg.tar.zst` asset on ARM64. Remove it with:

```sh
sudo pacman -R forecast-ledger
```

## Windows with Chocolatey

Download the `.nupkg` into an empty directory and verify its GitHub artifact
attestation. In an Administrator PowerShell session, change to that directory
and install the x86-64 package from the local source:

```powershell
choco install forecast-ledger --source . --version VERSION
```

To upgrade, download the newer `.nupkg` into the local source directory and
run:

```powershell
choco upgrade forecast-ledger --source . --version VERSION
```

Remove it with:

```powershell
choco uninstall forecast-ledger
```

The package is not yet published to the public Chocolatey repository. It uses
the x86-64 release archive; use the ZIP method for native Windows ARM64.

## Install from an archive

Archives remain the fallback on every supported platform. On macOS or Linux,
extract the selected archive and put the binary on `PATH`:

```sh
tar -xzf forecast-ledger_VERSION_PLATFORM_ARCH.tar.gz
sudo install -m 0755 forecast-ledger /usr/local/bin/forecast-ledger
forecast-ledger version --json
```

To remove this manual installation, remove only the installed binary:

```sh
sudo rm /usr/local/bin/forecast-ledger
```

On Windows, expand the selected ZIP into a stable directory, such as
`C:\Program Files\Forecast Ledger`, add that directory to the user or system
`PATH`, open a new PowerShell session, and run:

```powershell
forecast-ledger version --json
```

To upgrade an archive installation, verify and replace the binary with the one
from the newer release. To remove it, delete the installed binary and remove
the directory from `PATH` if it is no longer used.

## Package repository status

The project does not currently operate APT, RPM, APK, Arch, public Chocolatey,
Winget, or Scoop repositories. Native packages are attached directly to GitHub
Releases, so package managers cannot discover updates automatically. This
boundary will remain explicit until a repository has update metadata, signing,
and a tested publication process.

[Getting started](index.md) · [Release process](../development/releasing.md) ·
[Documentation index](../index.md)
