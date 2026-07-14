# Design Document: One-Line Installer

## Context

foxharness already publishes `fox_<os>_<arch>.tar.gz` archives and matching `.sha256` files for the supported macOS and Linux targets. The change adds a Release-hosted POSIX shell bootstrap, deterministic black-box tests, safe user-level installation, and synchronized installation documentation. No Go runtime behavior or package format changes are required.

## Goals / Non-Goals

**Goals:**

- Install a verified latest or pinned release with one command and no sudo.
- Preserve an existing binary across every pre-install validation failure.
- Safely and idempotently integrate a caller-selected directory into PATH.
- Exercise platform, security, destination, and PATH behavior without public-network access.
- Publish the installer once per GitHub Release and document its controls consistently.

**Non-Goals:**

- Windows PowerShell or package-manager installers.
- Independent artifact signing.
- A custom download domain or new archive layout.
- Changes to `scripts/release-patch.sh` or Go application code.

## Existing Repository Constraints

- `.github/workflows/release.yml` builds `darwin` and `linux` archives for `amd64` and `arm64` with `CGO_ENABLED=0` and publishes both versioned and stable names plus checksums.
- The Release job merges six matrix artifacts, so `install.sh` must be staged once after matrix artifact download rather than uploaded by each build job.
- The repository has four synchronized README variants: `README.md`, `README.zh-CN.md`, `README.zh-TW.md`, and `README.ja.md`.
- The project constitution requires test-first implementation, deterministic tests, explicit error paths, and security validation at system boundaries.
- macOS commonly provides `shasum` but not `sha256sum`; POSIX and BSD/GNU utility differences must be respected.

## Plan-Level Decisions

### Decision 1: One POSIX shell production script with explicit phases

**Covers:** REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, NFR-001, NFR-002

**Decision:** Implement `scripts/install.sh` as focused POSIX functions for argument resolution, platform detection, URL construction, download, digest verification, archive validation, destination staging, shell serialization, profile staging, and final activation. Start with `#!/bin/sh` and `set -eu`; do not use Bash arrays, `[[ ]]`, `local`, `pipefail`, GNU-only `sed -i`, `readlink -f`, or `stat` flags.

**Rationale:** Function boundaries keep the security-critical script reviewable while remaining executable by the default macOS and Linux `/bin/sh`.

**Trade-off:** POSIX shell requires more explicit validation than Bash, but avoids introducing a bootstrap dependency.

### Decision 2: Validate and canonicalize caller-controlled configuration before download

**Covers:** REQ-002, REQ-004, REQ-005, REQ-006, NFR-002

**Decision:** Parse CLI values after environment defaults so options take precedence. Validate the effective version and boolean, reject empty/colon/CR/LF installation-directory values, create and resolve the directory with `cd -P` plus `pwd -P`, and preflight any existing PATH marker block before network access. Require HOME only when the default directory or a profile path is needed.

**Rationale:** Early boundary validation avoids network side effects and ensures malformed profile state cannot be discovered after replacing the binary.

### Decision 3: Use stable archive names for latest and pinned Release URLs

**Covers:** REQ-002, REQ-003, NFR-003

**Decision:** For both latest and pinned releases, request `fox_<os>_<arch>.tar.gz` and its `.sha256`. The enclosing URL selects either `releases/latest/download` or `releases/download/<version>`.

**Rationale:** Every existing Release contains these stable aliases, and keeping the archive basename identical makes the checksum record and fake-downloader tests simpler.

### Decision 4: Verify before extraction and inspect tar semantics twice

**Covers:** REQ-003, NFR-002

**Decision:** Download into a private `mktemp -d` directory with an exit/signal cleanup trap. Strictly parse one checksum record, calculate SHA-256 through the first available supported tool, then inspect `tar -tzf` for exactly `fox` or `./fox` and `tar -tvzf` for a regular-file type before extraction. After extraction, reject a missing file or symlink again.

**Rationale:** Name listing alone cannot distinguish a regular file from a symlink, and extracting before checksum verification would violate the confirmed boundary.

### Decision 5: Stage on the destination filesystem and activate last

**Covers:** REQ-003, REQ-004, NFR-002

**Decision:** Copy the validated candidate into a unique `mktemp` file inside the installation directory, set mode `0755`, and keep it tracked by cleanup. Complete required PATH/profile validation and update before the final `mv` to `<install-dir>/fox`; the final same-directory rename is the last fallible activation step.

**Rationale:** A same-filesystem rename provides atomic visibility, and ordering the destination update last preserves an existing binary for validation, profile, and staging failures.

**Trade-off:** If the final rename itself fails after a profile update, the PATH entry may already exist, but the old binary remains unchanged and the profile entry still points at the intended user-owned directory.

### Decision 6: Serialize PATH values as POSIX single-quoted words

**Covers:** REQ-005, REQ-008, NFR-002

**Decision:** Convert the canonical directory to a single POSIX shell word by surrounding it with single quotes and replacing each literal apostrophe with the standard close-quote/escaped-quote/reopen sequence. Reuse that exact serialized value for both persisted and printed `export PATH=<word>:"$PATH"` commands. Use a marked block and an `awk` rewrite through a temporary profile file; reject duplicate, reversed, or unterminated marker blocks.

**Rationale:** This preserves spaces and shell metacharacters without allowing command, parameter, quote, or backtick evaluation.

### Decision 7: Black-box tests through command stubs and local fixtures

**Covers:** REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-008, NFR-001, NFR-002

**Decision:** Add `scripts/install_test.sh` with `#!/bin/sh` and `set -eu`. Each case creates an isolated HOME, destination, fixture directory, and fake-command directory. Fake `uname`, `sysctl`, `curl`, and `wget` scripts record requests and copy local fixtures; the production installer receives no test-only option or alternate-network hook. Valid and malicious tar archives are generated locally with host tools. PATH commands are executed in an isolated `sh` to verify exact results and absence of code execution. The suite also covers curl preference, wget fallback, and the missing-downloader error.

**Rationale:** PATH-level dependency replacement tests the public installer contract without public-network requests or production test hooks.

### Decision 8: Stage and test the installer once in the Release job

**Covers:** REQ-007, REQ-008, NFR-003

**Decision:** After artifact aggregation, run `sh scripts/install_test.sh`, require `scripts/install.sh` to exist, and copy it to `release-assets/install.sh`. Continue publishing `release-assets/*` through the existing create/upload logic.

**Rationale:** A single staged copy avoids matrix name collisions and prevents a failing installer suite from being published.

### Decision 9: Keep all README installation sections synchronized

**Covers:** REQ-006, REQ-007

**Decision:** Update the English, Simplified Chinese, Traditional Chinese, and Japanese README installation sections with localized one-line, pinned-version, custom-directory, PATH opt-out, and environment-pipeline examples while retaining manual, Windows, and source alternatives.

**Rationale:** The existing files present equivalent installation guidance and should not diverge after a user-facing workflow change.

## Architecture

**Covers:** REQ-001 through REQ-008; NFR-001 through NFR-003

```text
CLI options / FOX_* environment
              |
              v
  validate config + detect platform
              |
              v
     construct GitHub Release URLs
              |
              v
  private temp download -> SHA-256 verify
              |
              v
  tar name/type check -> private extraction
              |
              v
 destination-local staged fox (0755)
              |
              v
 safe PATH block/manual command preparation
              |
              v
  atomic mv -> <install-dir>/fox
```

## Implementation Phases

### Phase 1: Test contract (Red)

**Covers:** REQ-001 through REQ-006, REQ-008; NFR-001, NFR-002

- Create the dependency-free black-box test harness and dynamic fixtures.
- Add cases for every required mapping, option/environment boundary, verification failure, archive type, destination behavior, PATH serialization, idempotency, and opt-out.
- Run the suite and record the expected failure because `scripts/install.sh` does not yet exist.

### Phase 2: Installer implementation (Green and Refactor)

**Covers:** REQ-001 through REQ-006; NFR-001, NFR-002, NFR-003

- Implement the smallest POSIX functions needed to make the tests pass.
- Run focused tests after each behavior group, then the complete shell suite.
- Refactor repeated validation and error handling while retaining green tests.

### Phase 3: Release and documentation integration

**Covers:** REQ-007, REQ-008, NFR-003

- Update the Release job to test and stage one stable `install.sh` asset.
- Update all README installation sections without removing supported alternatives.
- Validate workflow syntax structurally and review the resulting asset paths.

### Phase 4: Full verification and review

**Covers:** All requirements

- Run `sh -n scripts/install.sh` and `sh -n scripts/install_test.sh`.
- Run `dash -n` and the test suite under dash when available.
- Run `shellcheck` when installed; otherwise record degraded static-analysis coverage.
- Run `sh scripts/install_test.sh`, `go test ./...`, and `git diff --check`.
- Perform the required final code review on changed shell source and auto-fix eligible verified defects without regressing tests.

## Verification Strategy

| Risk / Contract | Verification |
|-----------------|--------------|
| OS/architecture/Rosetta mapping | Fake `uname` and `sysctl`; assert recorded asset URL |
| Downloader selection | Fake curl/wget presence; assert curl preference, wget fallback, and no-downloader failure |
| Version validation and precedence | Invalid-value no-download assertions; pinned URL assertions |
| Checksum mismatch | Corrupt checksum fixture; assert old destination bytes remain |
| Archive traversal/type/extra entries | Local malicious tar fixtures; assert no replacement |
| Default/custom destination and atomic outcome | Isolated HOME/directories; assert executable bytes and no staging residue |
| PATH exact-element and marker idempotency | Repeated black-box installs; marker count and current PATH assertions |
| Shell metacharacter safety | Source captured profile/manual commands in isolated sh; assert exact PATH and no marker side effect |
| Release asset publication | Workflow structure check and staged `release-assets/install.sh` path review |
| Repository regression | `go test ./...` and diff hygiene |

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| BSD and GNU tar verbose output differ | A valid archive could be rejected on one OS | Depend only on the first type character and exact name listing; run tests with host tar |
| Checksum and archive share GitHub trust | Publisher compromise is not independently detected | Preserve SHA-256 corruption protection; independent signing remains explicitly out of scope |
| Profile files are executable shell input | Unsafe path text could execute on shell startup | Reject non-PATH separators and use one tested POSIX serializer for stored and printed commands |
| Release-only CI runs after tag push | Installer regressions may be found late | Keep a fast local test command and make Release publication depend on it; separate PR CI remains outside confirmed scope |
| A process may terminate during final rename | The new binary may not activate | Same-directory atomic rename leaves either the old or complete new file visible |

## Requirements Coverage

| Spec Requirement | Plan Coverage |
|------------------|---------------|
| REQ-001 | Decisions 1, 2, 7; Phases 1-2 |
| REQ-002 | Decisions 1-3, 7; Phases 1-2 |
| REQ-003 | Decisions 1, 4, 5, 7; Phases 1-2 |
| REQ-004 | Decisions 1, 2, 5, 7; Phases 1-2 |
| REQ-005 | Decisions 1, 2, 6, 7; Phases 1-2 |
| REQ-006 | Decisions 1, 2, 7, 9; Phases 1-3 |
| REQ-007 | Decisions 8-9; Phase 3 |
| REQ-008 | Decisions 6-8; Phases 1, 3-4 |
| NFR-001 | Decisions 1, 7; Phases 1-2, 4 |
| NFR-002 | Decisions 1-2, 4-7; Phases 1-2, 4 |
| NFR-003 | Decisions 3, 8; Phases 2-3 |

## Assumptions

- The current Release workflow continues to publish stable archive aliases within every versioned Release.
- Basic POSIX tools listed in the specification are available on supported hosts.

## Open Items

None.
