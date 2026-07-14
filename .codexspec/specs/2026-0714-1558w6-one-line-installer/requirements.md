# Confirmed Requirements: one-line-installer

**Feature ID**: `2026-0714-1558w6`
**Status**: Confirmed
**Last Confirmed**: 2026-07-14 16:03 CST

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- Open entries must not become product requirements without explicit user confirmation.
- Superseded entries remain historical and must identify their replacement.

## Needs

### NEED-001: One-line installer with automatic platform selection

- **Status**: confirmed
- **Statement**: Provide a POSIX shell installer for macOS and Linux that detects the operating system, maps amd64/x86_64 and arm64/aarch64 to existing release assets, and selects native arm64 on an Apple Silicon Mac when invoked through Rosetta 2.
- **Rationale**: Users should not need to choose and manually unpack a platform-specific archive.
- **User Evidence**: The user requested automatic macOS/Linux, amd64/arm64, and Rosetta detection.
- **Confirmed At**: 2026-07-14 16:03 CST

### NEED-002: Latest and pinned release installation

- **Status**: confirmed
- **Statement**: Install the latest GitHub release by default and support a pinned `vMAJOR.MINOR.PATCH` through `--version` or `FOX_VERSION`. Download both the matching archive and its `.sha256` file before installation.
- **Rationale**: The default path should be convenient while automation remains reproducible.
- **User Evidence**: The user requested latest by default and `--version v0.1.30` support, then confirmed the detailed option semantics.
- **Confirmed At**: 2026-07-14 16:03 CST

### NEED-003: Verified and atomic binary installation

- **Status**: confirmed
- **Statement**: Verify SHA-256 before extraction, reject an archive unless it contains exactly one expected regular file named `fox`, and replace the destination binary atomically only after every validation succeeds.
- **Rationale**: A corrupt or malicious archive must not replace a working installation.
- **User Evidence**: The user explicitly required checksum verification, strict archive inspection, and atomic installation.
- **Confirmed At**: 2026-07-14 16:03 CST

### NEED-004: Idempotent PATH integration

- **Status**: confirmed
- **Statement**: When the selected installation directory is absent from `PATH`, add one marked PATH block to the appropriate shell profile without duplicating it on repeated runs. Allow all PATH modification to be disabled and print actionable current-shell instructions when needed.
- **Rationale**: The installed command should be usable without repeated manual setup while respecting managed dotfiles.
- **User Evidence**: The user requested idempotent PATH configuration and `--no-modify-path`.
- **Confirmed At**: 2026-07-14 16:03 CST

### NEED-005: Release and README integration

- **Status**: confirmed
- **Statement**: Publish `install.sh` as a stable GitHub Release asset and replace the repeated manual macOS/Linux README commands with the one-line installer while retaining manual, Windows, and source-install alternatives.
- **Rationale**: The installer needs a stable URL and discoverable user documentation.
- **User Evidence**: The user requested Release workflow publication in the context of replacing the current README installation flow.
- **Confirmed At**: 2026-07-14 16:03 CST

### NEED-006: Deterministic shell test coverage

- **Status**: confirmed
- **Statement**: Add dependency-free shell tests that do not use the public network and cover platform mapping, version validation, checksum failure, custom/default installation directories, atomic replacement behavior, and idempotent or disabled PATH updates.
- **Rationale**: Installer boundary and failure behavior must remain safe across future changes.
- **User Evidence**: The user explicitly requested shell tests for platform mapping, version validation, checksum failure, and installation-directory behavior; PATH behavior is part of the confirmed installer contract.
- **Confirmed At**: 2026-07-14 16:03 CST

## Constraints

### CON-001: Portable, dependency-free bootstrap

- **Status**: confirmed
- **Statement**: `scripts/install.sh` must use POSIX `sh` with `set -eu`, rely only on standard macOS/Linux command-line tools plus `curl` or `wget`, introduce no third-party test framework, and never invoke `sudo`.
- **User Evidence**: The user explicitly required POSIX sh, `set -eu`, and no sudo; the confirmed summary excluded third-party test dependencies.

### CON-002: Supported platform boundary

- **Status**: confirmed
- **Statement**: The shell installer supports macOS and Linux on amd64 and arm64 only and must fail with a clear message for other systems or architectures.
- **User Evidence**: The user requested macOS/Linux amd64/arm64 detection and confirmed that Windows PowerShell support is outside this change.

### CON-003: Existing release contract

- **Status**: confirmed
- **Statement**: Reuse the repository's existing `fox_<os>_<arch>.tar.gz` archives and matching `.sha256` assets rather than introducing a new binary package format.
- **User Evidence**: The requested installer is an optimization of the existing GitHub Release installation method.

## Decisions

### DEC-001: Default user-level destination

- **Status**: confirmed
- **Decision**: Install to `$HOME/.local/bin/fox` by default. A selected destination directory is created when absent, must be writable, and is interpreted as a directory rather than a complete binary path.
- **Alternatives Rejected**: Defaulting to `/usr/local/bin` and requiring sudo.
- **Reason**: A one-line installer should work without privilege escalation.
- **User Evidence**: The user confirmed the proposed default and no-sudo behavior.

### DEC-002: Environment variables, options, validation, and precedence

- **Status**: confirmed
- **Decision**: Support `FOX_VERSION`/`--version`, `FOX_INSTALL_DIR`/`--install-dir`, and `FOX_NO_MODIFY_PATH`/`--no-modify-path`. Command-line options override environment variables, which override defaults. Versions accept `latest` or exactly `vMAJOR.MINOR.PATCH`. `FOX_NO_MODIFY_PATH` accepts empty, `0`, `false`, or `no` as false and `1`, `true`, or `yes` as true, case-insensitively; other values fail. Pipeline documentation must attach environment variables to the right-hand `sh` process.
- **Alternatives Rejected**: Undocumented permissive values, silently accepting malformed versions, and placing environment variables on the `curl` process.
- **Reason**: Human invocations need explicit flags while CI and managed environments need stable environment-variable controls.
- **User Evidence**: The user requested a detailed explanation of these controls and then explicitly confirmed it.

### DEC-003: Shell profile mapping

- **Status**: confirmed
- **Decision**: Select profiles as follows: macOS zsh uses `~/.zprofile`, macOS bash uses `~/.bash_profile`, Linux zsh uses `~/.zshrc`, Linux bash uses `~/.bashrc`, and other shells use `~/.profile`. Use a marked installer-owned block for idempotent updates.
- **Alternatives Rejected**: Writing to every shell profile or always choosing one profile across platforms.
- **Reason**: The mapping follows normal login/interactive shell behavior without broad dotfile mutation.
- **User Evidence**: The user confirmed the proposed profile mapping.

## Out of Scope

### OUT-001: Windows installer

- **Status**: confirmed
- **Statement**: A Windows PowerShell installer is not part of this change; existing Windows archive instructions remain available.
- **Reason**: The confirmed shell scope is macOS and Linux.
- **User Evidence**: The user confirmed this exclusion.

### OUT-002: Independent artifact signing

- **Status**: confirmed
- **Statement**: GPG, Sigstore, cosign, and other independent signing systems are not part of this change; verification uses the existing SHA-256 release assets.
- **Reason**: Independent signing is a separate release-security feature.
- **User Evidence**: The user confirmed this exclusion.

## Open Questions

None.

## Superseded Entries

None.

## Confirmation Log

### Session 2026-07-14 16:03 CST

- **Summary Presented**: Six installer/release/testing needs, one portability constraint, three implementation decisions, two exclusions, and no open questions.
- **Clarification Presented**: Detailed semantics and examples for version selection, installation directory selection, PATH suppression, truthy values, pipeline placement, and precedence.
- **User Confirmation**: `确认`
- **Entries Confirmed**: NEED-001 through NEED-006; CON-001 through CON-003; DEC-001 through DEC-003; OUT-001 through OUT-002.
