# Tasks: TUI Interaction Compatibility

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Feature ID**: `2026-0709-2113sj`  
**Input**: `.codexspec/specs/2026-0709-2113sj-tui-interaction-compatibility/plan.md`  
**Status**: Draft

## Task Groups

Tasks follow the approved plan phases and the project constitution's mandatory TDD workflow. Each behavior-changing task has a test-first predecessor.

## Phase 1: Settings Persistence

### T001 - Add Failing Tests For Nested TUI Settings

- **Status**: Complete
- **Outcome**: `internal/settings` has tests that fail because `Settings` does not yet load, save, or preserve the nested `tui` object.
- **Paths**: `internal/settings/settings_test.go`
- **Dependencies**: None
- **Verification**: `go test ./internal/settings -run 'TestLoad|TestSave'` fails for the expected missing TUI settings behavior.
- **Covers**: REQ-009, NFR-002, NFR-003; Plan: Phase 1, C1

### T002 - Implement TUI Settings Load And Raw Merge

- **Status**: Complete
- **Outcome**: `Settings` supports `TUI.Theme` and `TUI.Statusline`, writes them under `tui`, preserves unknown top-level fields, preserves unknown `tui` fields, and keeps existing LLM merge behavior intact.
- **Paths**: `internal/settings/settings.go`, `internal/settings/settings_test.go`
- **Dependencies**: T001
- **Verification**: `go test ./internal/settings`
- **Covers**: REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, NFR-002, NFR-003; Plan: Phase 1, C1

## Phase 2: Status And Slash Command Conflicts

### T003 - Add Failing Tests For `/status`, `/session`, And `/clear`

- **Status**: Complete
- **Outcome**: `internal/tui` has failing tests for grouped `/status`, `/session` alias equivalence, `/clear` using the `/new` command path, and deferred commands remaining unsupported.
- **Paths**: `internal/tui/model_test.go`
- **Dependencies**: T002
- **Verification**: `go test ./internal/tui -run 'Status|Session|Clear|Unsupported'` fails for the expected missing/old command behavior.
- **Covers**: REQ-002, REQ-003, REQ-004, REQ-012, NFR-002, NFR-004; Plan: Phase 2, C5, C6

### T004 - Implement Status Overview And Command Conflict Resolutions

- **Status**: Complete
- **Outcome**: `/status` renders the grouped overview; `/session` calls the same formatter; `/clear` aliases `/new`; provider/profile metadata is accepted through `tui.Config`; deferred commands remain ordinary unknown commands.
- **Paths**: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/app/tui.go`
- **Dependencies**: T003
- **Verification**: `go test ./internal/tui -run 'Status|Session|Clear|Unsupported'` and `go test ./internal/app`
- **Covers**: REQ-002, REQ-003, REQ-004, REQ-012, NFR-001, NFR-002, NFR-004; Plan: Phase 2, C2, C5, C6

## Phase 3: Theme And Statusline Configuration

### T005 - Add Failing Tests For Theme And Statusline Behavior

- **Status**: Complete
- **Outcome**: `internal/tui` has failing tests for default statusline items, available non-default `run-state`, `/statusline set`, `/statusline default`, shell-hook-like statusline rejection, `/theme` built-in selection, invalid theme rejection, persistence failures, and restoration from settings.
- **Paths**: `internal/tui/model_test.go`
- **Dependencies**: T004
- **Verification**: `go test ./internal/tui -run 'Theme|Statusline'` fails for the expected missing theme/statusline behavior.
- **Covers**: REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, NFR-002, NFR-003, NFR-004; Plan: Phase 3, C3, C4, C5

### T006 - Implement Theme Registry, Statusline Registry, And Persistence Commands

- **Status**: Complete
- **Outcome**: The TUI loads saved TUI preferences, applies a built-in theme registry, renders configured statusline items, persists `/theme` and `/statusline` changes, reports write errors clearly, and keeps invalid selections from changing persisted configuration.
- **Paths**: `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/markdown.go`, `internal/tui/model_test.go`, `internal/tui/theme.go`, `internal/tui/statusline.go`
- **Dependencies**: T005
- **Verification**: `go test ./internal/tui -run 'Theme|Statusline'`
- **Covers**: REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, NFR-001, NFR-002, NFR-003, NFR-004; Plan: Phase 3, C3, C4, C5

## Phase 4: Entry Rendering Alignment

### T007 - Add Failing Tests For Entry-Based Rendering Refinements

- **Status**: Complete
- **Outcome**: Focused tests cover malformed known tool arguments falling back safely, failed shell command rendering using a distinct error style, and preservation of existing folding and queue preview behavior.
- **Paths**: `internal/tui/model_test.go`
- **Dependencies**: T006
- **Verification**: `go test ./internal/tui -run 'Tool|Shell|Queue'` fails only for missing rendering refinements, not unrelated behavior.
- **Covers**: REQ-010, REQ-011, NFR-002, NFR-004; Plan: Phase 4, C7

### T008 - Implement Localized Entry Rendering Refinements

- **Status**: Complete
- **Outcome**: Tool summaries keep safe fallbacks, shell command failures use the error state visually, long tool/shell outputs remain foldable, and queued prompt overflow remains compact.
- **Paths**: `internal/tui/view.go`, `internal/tui/reporter.go`, `internal/tui/model_test.go`
- **Dependencies**: T007
- **Verification**: `go test ./internal/tui -run 'Tool|Shell|Queue'`
- **Covers**: REQ-010, REQ-011, NFR-002, NFR-004; Plan: Phase 4, C7

## Phase 5: Format, Full Verification, And Review Loop

### T009 - Format Changed Go Files

- **Status**: Complete
- **Outcome**: All changed Go files are formatted with `gofmt`.
- **Paths**: `internal/settings/settings.go`, `internal/settings/settings_test.go`, `internal/tui/*.go`, `internal/app/tui.go`
- **Dependencies**: T008
- **Verification**: `gofmt -w` on changed Go files completes without error.
- **Covers**: NFR-002; Plan: Phase 5

### T010 - Run Focused And Full Test Suites

- **Status**: Complete
- **Outcome**: Focused settings/TUI tests and the repository test suite pass.
- **Paths**: repository root
- **Dependencies**: T009
- **Verification**: `go test ./internal/settings`, `go test ./internal/tui`, and `go test ./...`
- **Covers**: REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, NFR-002, NFR-003, NFR-004; Plan: Phase 5, Verification Strategy

### T011 - Perform Code Review And Fix Verified Findings

- **Status**: Complete
- **Outcome**: A code review checks requirement fidelity, persistence safety, TDD coverage, UI regressions, and core-scope boundaries; any verified findings are fixed with tests rerun until review has no verified defects.
- **Paths**: changed files from T002, T004, T006, T008
- **Dependencies**: T010
- **Verification**: Review report has no verified defects and `go test ./...` still passes after any fixes.
- **Covers**: NFR-001, NFR-002, NFR-003, NFR-004; Plan: Phase 5, Verification Strategy

## Dependency Summary

T001 -> T002 -> T003 -> T004 -> T005 -> T006 -> T007 -> T008 -> T009 -> T010 -> T011

The sequence is intentionally linear because each implementation phase builds on the previous phase's model/settings surface, and the constitution requires tests before behavior changes.

## Coverage Table

| Requirement / Plan Item | Task References |
|-------------------------|-----------------|
| REQ-001 | T006 |
| REQ-002 | T003, T004, T010 |
| REQ-003 | T003, T004, T010 |
| REQ-004 | T003, T004, T010 |
| REQ-005 | T002, T005, T006, T010 |
| REQ-006 | T002, T005, T006, T010 |
| REQ-007 | T002, T005, T006, T010 |
| REQ-008 | T002, T005, T006, T010 |
| REQ-009 | T001, T002, T005, T006, T010 |
| REQ-010 | T005, T006, T007, T008, T010 |
| REQ-011 | T007, T008, T010 |
| REQ-012 | T003, T004, T010 |
| NFR-001 | T004, T006, T011 |
| NFR-002 | T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011 |
| NFR-003 | T001, T002, T005, T006, T010, T011 |
| NFR-004 | T003, T004, T005, T006, T007, T008, T010, T011 |
| C1 / Phase 1 | T001, T002 |
| C2 | T004 |
| C3 / Phase 3 | T005, T006 |
| C4 / Phase 3 | T005, T006 |
| C5 / Phase 2-3 | T003, T004, T005, T006 |
| C6 / Phase 2 | T003, T004 |
| C7 / Phase 4 | T007, T008 |
| Phase 5 / Verification Strategy | T009, T010, T011 |

## Unmapped Tasks

None.
