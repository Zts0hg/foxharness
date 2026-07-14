# Tasks: One-Line Installer

## 1. Establish the Installer Contract (TDD Red)

- [x] 1.1 Add a dependency-free POSIX black-box harness and dynamic local archive/checksum fixtures in `scripts/install_test.sh`; include fake `uname`, `sysctl`, `curl`, and `wget` commands and assertions that no public network path is used. **Outcome:** the runner itself parses under `sh`, isolates HOME/PATH per case, and can report multiple failures. **Covers:** REQ-008, NFR-001; **Plan:** Decision 7, Phase 1. **Dependencies:** none.
- [x] 1.2 Add failing cases for every OS/architecture mapping, Rosetta, unsupported targets, curl preference, wget fallback, missing downloader, version validation/precedence, option value errors, and PATH boolean validation. **Outcome:** the suite defines the selection and CLI contract before production code exists. **Covers:** REQ-001, REQ-002, REQ-006, REQ-008; **Plan:** Decisions 1-3 and 7, Phase 1. **Dependencies:** 1.1.
- [x] 1.3 Add failing cases for checksum mismatch preservation, malformed checksum, extra/traversal/symlink archives, default/custom destinations, destination precedence, atomic replacement residue, PATH idempotency/opt-out/malformed markers, and safe stored/printed PATH commands with shell metacharacters. **Outcome:** security and side-effect contracts are executable before production code exists. **Covers:** REQ-003, REQ-004, REQ-005, REQ-008, NFR-002; **Plan:** Decisions 4-7, Phase 1. **Dependencies:** 1.2.
- [x] 1.4 Run `sh scripts/install_test.sh` and record the expected failure caused by the missing `scripts/install.sh`. **Outcome:** the Red phase is demonstrated for the intended reason. **Covers:** REQ-008, NFR-001; **Plan:** Phase 1. **Dependencies:** 1.3.

## 2. Implement the POSIX Installer (TDD Green and Refactor)

- [x] 2.1 Create executable `scripts/install.sh` with POSIX option/environment parsing, strict effective-value validation, directory canonicalization, command preflight, platform/Rosetta mapping, stable Release URL construction, curl/wget selection, usage, and actionable errors. Run the shell suite until the configuration/platform/downloader cases pass. **Outcome:** every pre-download boundary behaves as specified without sudo or Bash syntax. **Covers:** REQ-001, REQ-002, REQ-004, REQ-006, NFR-001, NFR-002, NFR-003; **Plan:** Decisions 1-3, Phase 2. **Dependencies:** 1.4.
- [x] 2.2 Implement private temporary lifecycle, strict checksum parsing, portable digest calculation, tar name/type validation, post-extraction checks, destination-local staging, mode `0755`, cleanup, and final atomic rename. Run the shell suite until verification/archive/destination cases pass. **Outcome:** only a complete verified regular `fox` can atomically replace the destination. **Covers:** REQ-003, REQ-004, NFR-002; **Plan:** Decisions 4-5, Phase 2. **Dependencies:** 2.1.
- [x] 2.3 Implement exact PATH-element detection, profile selection, reversible POSIX shell-word serialization, malformed-marker preflight, idempotent profile staging/update, no-modify behavior, and safe manual instructions. Run and refactor the complete shell suite to green. **Outcome:** profile and manual PATH commands preserve exact directory bytes without executing metacharacters. **Covers:** REQ-005, REQ-006, NFR-002; **Plan:** Decisions 2, 5-7, Phase 2. **Dependencies:** 2.2.

## 3. Integrate Release and Documentation

- [x] 3.1 Update `.github/workflows/release.yml` so the aggregate Release job runs `sh scripts/install_test.sh`, verifies `scripts/install.sh`, and stages exactly one `release-assets/install.sh` before existing create/upload commands. **Outcome:** a failing or missing installer cannot be published and matrix artifacts do not collide. **Covers:** REQ-007, REQ-008, NFR-003; **Plan:** Decision 8, Phase 3. **Dependencies:** 2.3.
- [x] 3.2 [P] Update installation sections in `README.md`, `README.zh-CN.md`, `README.zh-TW.md`, and `README.ja.md` with localized default, pinned-version, custom-directory, PATH opt-out, and pipe-safe environment examples while preserving manual, Windows, source, and Gatekeeper guidance. **Outcome:** all user-facing variants describe the same installer contract. **Covers:** REQ-006, REQ-007; **Plan:** Decision 9, Phase 3. **Dependencies:** 2.3.

## 4. Verify and Review

- [x] 4.1 Run `sh -n` for both scripts, `dash -n` and the shell suite under dash when available, `shellcheck` when available, and `sh scripts/install_test.sh`; address only specification-backed defects. **Outcome:** production and test scripts are portable and the complete installer suite is green. **Covers:** REQ-001 through REQ-006, REQ-008, NFR-001, NFR-002; **Plan:** Phase 4. **Dependencies:** 3.1.
- [x] 4.2 Run `go test ./...`, validate the Release workflow changes and README command consistency, run `git diff --check`, and inspect the final worktree. **Outcome:** repository regressions and integration mistakes are absent. **Covers:** REQ-007, NFR-003; **Plan:** Phase 4. **Dependencies:** 3.1, 3.2, 4.1.
- [x] 4.3 Run the required final `codexspec:review-code` pass on changed shell source, auto-fix eligible CRITICAL/HIGH and grounded MEDIUM findings with test-safe changes, rerun all verification, and record final review status. **Outcome:** no unresolved CRITICAL/HIGH defect remains and tests stay green. **Covers:** All requirements; **Plan:** Phase 4. **Dependencies:** 4.2.

## Dependencies

```text
1.1 -> 1.2 -> 1.3 -> 1.4 -> 2.1 -> 2.2 -> 2.3
                                              |   \
                                              v    v
                                             3.1  3.2
                                              |    |
                                              v    |
                                             4.1   |
                                               \   /
                                                v v
                                                4.2 -> 4.3
```

Tasks 3.1 and 3.2 operate on separate files after the installer contract is green and may proceed in parallel.

## Coverage

| Requirement / Plan Item | Task References | Result |
|-------------------------|-----------------|--------|
| REQ-001 | 1.2, 2.1, 4.1 | Covered |
| REQ-002 | 1.2, 2.1, 4.1 | Covered |
| REQ-003 | 1.3, 2.2, 4.1 | Covered |
| REQ-004 | 1.3, 2.1, 2.2, 4.1 | Covered |
| REQ-005 | 1.3, 2.3, 4.1 | Covered |
| REQ-006 | 1.2-1.3, 2.1, 2.3, 3.2, 4.1 | Covered |
| REQ-007 | 3.1-3.2, 4.2 | Covered |
| REQ-008 | 1.1-1.4, 3.1, 4.1 | Covered |
| NFR-001 | 1.1, 1.4, 2.1, 4.1 | Covered |
| NFR-002 | 1.3, 2.1-2.3, 4.1 | Covered |
| NFR-003 | 2.1, 3.1, 4.2 | Covered |
| Decision 1 | 1.2, 2.1 | Covered |
| Decisions 2-3 | 1.2-1.3, 2.1, 2.3 | Covered |
| Decisions 4-6 | 1.3, 2.2-2.3 | Covered |
| Decision 7 | 1.1-1.4 | Covered |
| Decision 8 | 3.1 | Covered |
| Decision 9 | 3.2 | Covered |

## Notes

- TDD evidence is the intentional Task 1.4 failure before `scripts/install.sh` exists, followed by focused and full green runs in Tasks 2.1-2.3.
- Red evidence (2026-07-14): `sh -n` and `dash -n` accepted the test runner; `sh scripts/install_test.sh` completed all 33 cases with 0 passing and 33 failing because `/bin/sh` could not open the intentionally absent `scripts/install.sh` (exit 127).
- Green evidence (2026-07-14): the expanded suite completed 51 cases with 51 passing under both `sh` and `dash`, including every architecture alias, Rosetta, strict scalar and canonical-path boundaries, checksum/archive failures, atomic destination behavior, profile replacement/symlink/concurrency preservation, and hostile PATH serialization.
- Final review evidence (2026-07-14): `review-code.md` records PASS at 99/100 with no unresolved CRITICAL, HIGH, or MEDIUM finding. ShellCheck was unavailable; syntax checks, two-shell black-box suites, and independent manual review provided the fallback static coverage.
- Documentation and workflow tasks are implemented directly after the code contract is green.
- `shellcheck` absence is a reported static-analysis limitation rather than a blocker when syntax checks and deterministic tests pass.
