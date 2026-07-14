# Feature Specification: One-Line Installer

## Context

foxharness currently publishes platform-specific archives and SHA-256 files for macOS, Linux, and Windows. macOS and Linux users must manually select an archive, extract `fox`, change its mode, and move it to `/usr/local/bin` with `sudo`. This feature introduces a stable Release-hosted bootstrap that performs those steps safely in a user-owned directory.

## Goals

- Provide a single installation command for supported macOS and Linux systems.
- Preserve a reproducible pinned-version path alongside the default latest-release path.
- Prevent corrupt or structurally unsafe archives from replacing an installed binary.
- Make the installed command discoverable without requiring root privileges.
- Keep the installer and its tests portable and dependency-free.

## Non-Goals

- A Windows PowerShell installer.
- Homebrew, apt, rpm, or other package-manager integration.
- GPG, Sigstore, cosign, or another independent signing system.
- A custom branded download domain.

## User Stories

### Story: Install the latest release

**As a** macOS or Linux user
**I want** one command to select and install the correct binary
**So that** I do not need to understand Release asset naming or use sudo.

### Story: Pin an installation

**As an** automation author
**I want** to select an exact `vMAJOR.MINOR.PATCH` and destination directory
**So that** installations are reproducible and isolated.

### Story: Preserve managed shell configuration

**As a** user who manages dotfiles
**I want** to disable PATH changes
**So that** the installer does not mutate files owned by another configuration workflow.

## Functional Requirements

### REQ-001: Platform and architecture detection

The installer must detect `Darwin` as `darwin` and `Linux` as `linux`. It must map `x86_64` and `amd64` to `amd64`, and `arm64` and `aarch64` to `arm64`. On macOS, an x86_64 process with `sysctl.proc_translated=1` must select `arm64`. Other operating systems or architectures must fail before downloading an archive with a message that includes the unsupported value.

**Sources:** NEED-001, CON-002, CON-003

#### Scenario: Rosetta selects the native asset

- **WHEN** `uname -s` is `Darwin`, `uname -m` is `x86_64`, and `sysctl -n sysctl.proc_translated` returns `1`
- **THEN** the installer requests `fox_darwin_arm64.tar.gz` and its checksum.

#### Scenario: Unsupported platform is rejected

- **WHEN** the detected operating system or architecture is outside the confirmed support matrix
- **THEN** the installer exits non-zero without downloading or replacing `fox`.

### REQ-002: Version selection and URL construction

The effective version must be selected with this precedence: `--version`, then `FOX_VERSION`, then `latest`. Only `latest` or a fully anchored `vMAJOR.MINOR.PATCH` containing decimal numeric segments is valid. The latest path must use `https://github.com/Zts0hg/foxharness/releases/latest/download/<asset>`. A pinned version must use `https://github.com/Zts0hg/foxharness/releases/download/<version>/<asset>`. The checksum URL must append `.sha256` to the archive URL.

**Sources:** NEED-002, CON-003, DEC-002

#### Scenario: Pinned version overrides the environment

- **WHEN** `FOX_VERSION=v0.1.29` and `--version v0.1.30` are both provided
- **THEN** the installer downloads from the `v0.1.30` Release.

#### Scenario: Invalid version is rejected

- **WHEN** the effective version is not `latest` or exactly `vMAJOR.MINOR.PATCH`
- **THEN** the installer exits non-zero before any download.

### REQ-003: Download, checksum, and archive validation

The installer must create a private temporary directory with `mktemp -d`, register cleanup for normal exit and termination, and download the archive and checksum file there using `curl -fsSL` or a `wget` fallback. It must obtain the expected 64-character hexadecimal SHA-256 from the checksum file, calculate the archive digest with `sha256sum`, `shasum -a 256`, or `openssl dgst -sha256`, and compare them case-insensitively before extraction. A malformed checksum file or mismatch must exit non-zero without extracting or replacing the destination.

Before extraction, the installer must verify that the gzip tar archive contains exactly one entry, that the normalized entry name is `fox`, and that the entry is a regular file rather than a directory, symlink, hard link, device, or other type. It must reject absolute paths, parent traversal, duplicate entries, and any additional entry.

**Sources:** NEED-002, NEED-003, CON-001, CON-003, OUT-002

#### Scenario: Checksum mismatch preserves the installed binary

- **WHEN** the downloaded archive digest differs from the checksum asset
- **THEN** the installer exits non-zero and an existing destination `fox` remains byte-for-byte unchanged.

#### Scenario: Unsafe archive is rejected

- **WHEN** a checksum-valid archive includes an entry other than one regular `fox`
- **THEN** the installer exits non-zero before extraction into the installation directory.

### REQ-004: Destination selection and atomic installation

The effective installation directory must be selected with this precedence: `--install-dir`, then `FOX_INSTALL_DIR`, then `$HOME/.local/bin`. The value represents a directory and must be non-empty. Before any download, the installer must reject a directory containing a colon, carriage return, or newline because it cannot be represented as one stable PATH element. The installer must create the directory when absent, resolve it to an absolute physical path, verify it is a directory and writable, extract the validated archive only inside the private temporary directory, set the candidate binary mode to `0755`, and verify that it is executable.

The final update must use a uniquely named temporary file created inside the installation directory and rename that file to `<install-dir>/fox`, so the replacement is atomic on the destination filesystem. The installer must not invoke `sudo`. A failure before the rename must preserve any existing destination.

**Sources:** NEED-003, CON-001, DEC-001, DEC-002

#### Scenario: Custom destination overrides the environment

- **WHEN** `FOX_INSTALL_DIR` and `--install-dir` specify different writable directories
- **THEN** only the command-line directory receives the installed `fox`.

### REQ-005: PATH behavior

The installer must determine whether the effective installation directory is already an exact PATH element. If it is present, the installer must not modify a profile. Otherwise, unless PATH modification is disabled, it must select the profile defined by DEC-003 and maintain exactly one installer-owned block:

```sh
# >>> foxharness installer >>>
export PATH='<encoded-install-dir>':"$PATH"
# <<< foxharness installer <<<
```

The installation directory in this executable profile block must be serialized as one reversible POSIX shell word. The serialized form must reproduce the exact directory bytes when sourced and must prevent parameter expansion, command substitution, backtick evaluation, quote termination, backslash interpretation, or word splitting. A suitable representation uses correctly escaped single-quoted shell segments for the directory followed by `:"$PATH"`; raw interpolation into a double-quoted string is forbidden.

Repeated installations must not duplicate the block. An existing complete installer-owned block must be updated if the directory changes. A malformed block with no end marker must cause the PATH update to fail without rewriting the profile.

`--no-modify-path` must set the behavior to disabled. Otherwise `FOX_NO_MODIFY_PATH` accepts empty, `0`, `false`, or `no` as false and `1`, `true`, or `yes` as true, case-insensitively; any other value must fail before download. When PATH remains unavailable in the current process, the installer must print an `export PATH=...` instruction and state which profile was changed, if any. Every executable PATH command, whether persisted in a profile or printed for manual use, must reuse the same reversible POSIX shell-word serialization and must never interpolate the raw installation directory.

**Sources:** NEED-004, DEC-002, DEC-003

#### Scenario: PATH update is idempotent

- **WHEN** the installer runs twice for a directory absent from the original PATH
- **THEN** the selected profile contains one marked installer block.

#### Scenario: PATH modification is disabled

- **WHEN** `--no-modify-path` or a true `FOX_NO_MODIFY_PATH` is effective
- **THEN** no shell profile is created or modified and a manual export instruction is printed.

### REQ-006: Command-line contract

The installer must support `--version VALUE`, `--install-dir DIRECTORY`, `--no-modify-path`, and `--help`/`-h`. Missing option values and unknown options must produce usage text and exit non-zero. Help must document defaults, corresponding environment variables, precedence, supported boolean values, and pipe-safe examples that attach variables to the right-hand `sh` process.

**Sources:** DEC-002, NEED-005

### REQ-007: Release and user documentation integration

Every tag-triggered GitHub Release must include a stable asset named `install.sh` containing the repository's `scripts/install.sh`. The workflow must fail rather than publish a Release when that source file is absent. README installation guidance must lead with:

```sh
curl -fsSL https://github.com/Zts0hg/foxharness/releases/latest/download/install.sh | sh
```

README must retain manual Release archive guidance, Windows archive names, source installation, pinned-version usage, custom-directory usage, PATH opt-out usage, and the warning that pipeline environment variables belong on the `sh` process.

**Sources:** NEED-005, CON-003, DEC-002, OUT-001

### REQ-008: Shell test suite

The repository must include dependency-free POSIX shell tests that replace external commands or inputs with local deterministic fixtures. Tests must make no public-network request and must cover all platform mappings including Rosetta, unsupported platform rejection, version validation and precedence, invalid PATH boolean values, checksum mismatch preservation, unsafe archive rejection, default and custom destination behavior, atomic replacement outcome, idempotent PATH updates, and disabled PATH updates. PATH serialization tests must include spaces, dollar signs and command-substitution text, single and double quotes, backticks, and backslashes, and must prove that colon and line-break directory values are rejected before download. Tests must execute both a captured profile block and the manual export command from the no-modify branch in an isolated POSIX shell, verify the exact resulting PATH element, and prove that no embedded text is evaluated as code.

**Sources:** NEED-006, CON-001, CON-002, DEC-001, DEC-002, DEC-003

## Non-Functional Requirements

### NFR-001: POSIX portability

`scripts/install.sh` and its test runner must parse and run under POSIX `sh` on supported systems, begin with `set -eu`, quote variable expansions, and avoid Bash-only syntax and external framework dependencies.

**Sources:** CON-001

### NFR-002: Failure safety and cleanup

All validation must precede destination replacement. Temporary download, extraction, and destination staging artifacts must be removed on success, handled failure, interrupt, or termination. Error messages must identify the failed boundary without printing secrets or executing unvalidated input.

**Sources:** NEED-003, CON-001, OUT-002

### NFR-003: Release compatibility

The installer must consume the existing unversioned archive and checksum names produced by the Release workflow and must not require a new package format or a change to `scripts/release-patch.sh`.

**Sources:** CON-003, NEED-005

## Constraints and Confirmed Decisions

- Supported shell platforms are macOS and Linux on amd64 or arm64.
- The installer is POSIX `sh`, uses `set -eu`, has no third-party runtime or test dependency, and never invokes sudo.
- The default destination is `$HOME/.local/bin`.
- Command-line values override environment values, which override defaults.
- PATH profiles follow the confirmed OS/shell mapping and use one marked block.
- Windows installation and independent artifact signing remain out of scope.

## Expected Error Behavior

The installer must exit non-zero and leave an existing binary untouched for invalid arguments or environment values, installation directories that cannot form one safe PATH element, missing required commands, unsupported platforms, download failures, malformed checksums, checksum mismatches, malformed or unsafe archives, non-directory destinations, unwritable destinations, or malformed existing PATH blocks. It must print errors to stderr and never silently fall back to another version, platform, destination, or verification method after a semantic validation failure.

## Assumptions

- Supported hosts provide the basic POSIX utilities already used by the repository's release workflow, including `tar`, `awk`, `sed`, `grep`, `chmod`, `mkdir`, `mv`, and `mktemp`.
- Existing release archives contain a single top-level regular file named `fox`, as produced by the current workflow.

## Requirements Traceability

| Confirmed Entry | Spec Coverage |
|-----------------|---------------|
| NEED-001 | REQ-001 |
| NEED-002 | REQ-002, REQ-003 |
| NEED-003 | REQ-003, REQ-004, NFR-002 |
| NEED-004 | REQ-005 |
| NEED-005 | REQ-006, REQ-007, NFR-003 |
| NEED-006 | REQ-008 |
| CON-001 | REQ-003, REQ-004, REQ-008, NFR-001, NFR-002 |
| CON-002 | REQ-001, REQ-008 |
| CON-003 | REQ-001, REQ-002, REQ-003, REQ-007, NFR-003 |
| DEC-001 | REQ-004, REQ-008 |
| DEC-002 | REQ-002, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008 |
| DEC-003 | REQ-005, REQ-008 |
| OUT-001 | Non-Goals, REQ-007 |
| OUT-002 | Non-Goals, REQ-003, NFR-002 |

## Open Questions

None.
