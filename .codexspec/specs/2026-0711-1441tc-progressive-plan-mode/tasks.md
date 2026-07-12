# Tasks: Progressive Plan Mode

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Feature ID**: `2026-0711-1441tc`
**Input**: `.codexspec/specs/2026-0711-1441tc-progressive-plan-mode/plan.md`
**Status**: Draft

## Task Groups

Tasks follow the approved plan's six implementation phases and the project constitution's mandatory Red-Green-Refactor workflow. Every behavior-changing implementation task depends on a test task whose expected RED result is recorded before production code changes.

## Phase 1: Collaboration State And Legacy CLI Boundary

### T001 - [x] Add Failing Tests For Collaboration Selection And CLI Removal

- **Outcome**: Tests define Default startup/resume/new-session behavior, typed Default/Formal selection, active-run deferral, idempotent `/plan` and `/plan off`, primary-input Shift+Tab, unchanged sidebar/Esc behavior, and rejection of the removed `-plan` flag.
- **Paths**: `internal/collaboration/mode_test.go`, `internal/app/runner_test.go`, `internal/tui/model_test.go`, `cmd/fox/main_test.go`
- **Dependencies**: None
- **Verification**: Focused tests fail for missing collaboration APIs/commands and the still-registered `-plan` flag, while existing unrelated tests remain buildable where possible.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-011, REQ-012, NFR-004, NFR-005; Plan: Phase 1, C1, C6, C7

### T002 - [x] Implement Typed Collaboration Selection And Remove CLI Plan Configuration

- **Outcome**: `internal/collaboration` defines Default/Formal Plan; runner and TUI use selected collaboration state instead of legacy Plan boolean accessors; new/resumed/new sessions select Default; `/plan`, `/plan off`, and Shift+Tab follow confirmed semantics; `-plan` and `EnablePlanMode` CLI/config plumbing are removed.
- **Paths**: `internal/collaboration/mode.go`, `internal/app/runner.go`, `internal/app/cli.go`, `internal/app/autodev.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/statusline.go`, `cmd/fox/main.go`, tests from T001
- **Dependencies**: T001
- **Verification**: Focused collaboration, app, TUI, and CLI tests pass; active runner tests prove selection changes do not block or mutate the captured active mode.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-011, REQ-012, NFR-004, NFR-005; Plan: Phase 1, PLD-001, C1, C6, C7

## Phase 2: Exact Plan Persistence And Submission

### T003 - [x] Add Failing Tests For Atomic PLAN Replacement And `submit_plan`

- **Outcome**: Tests define exact-byte PLAN replacement, preservation of prior content on failure, whitespace rejection, review-after-write ordering, no review on write failure, exact reviewer payload, approval notification, revision feedback (including empty continue-planning feedback), cancellation, and reviewer errors.
- **Paths**: `internal/memory/store_test.go`, `internal/tools/submit_plan_test.go`
- **Dependencies**: T002
- **Verification**: `go test ./internal/memory ./internal/tools` fails for missing `ReplacePlan` and `submit_plan` behavior for the expected reasons.
- **Covers**: REQ-005, REQ-006, REQ-007, REQ-010, REQ-014, NFR-002, NFR-004, NFR-005; Plan: Phase 2, PLD-004, C2

### T004 - [x] Implement Atomic PLAN Replacement And `submit_plan`

- **Outcome**: `memory.Store` atomically replaces PLAN with the exact input bytes; `submit_plan(plan_markdown)` persists before review, exposes injected store/reviewer collaborators, returns revision feedback, and notifies approval without exposing incremental plan mutation.
- **Paths**: `internal/memory/store.go`, `internal/tools/submit_plan.go`, tests from T003
- **Dependencies**: T003
- **Verification**: `go test ./internal/memory ./internal/tools` passes, including deterministic failure collaborators and filesystem replacement cases.
- **Covers**: REQ-005, REQ-006, REQ-007, REQ-010, REQ-014, NFR-002, NFR-004, NFR-005; Plan: Phase 2, PLD-004, C2

## Phase 3: Turn Lifecycle, Completion Gate, And Formal Prompt

### T005 - [x] Add Failing Engine Tests For Turn-Aware Registries And Completion Gates

- **Outcome**: Engine/tool tests define one `BeginTurn` call before each turn's tool discovery, delegation through filtered registries, one blocking completion reminder, successful continuation after gate satisfaction, and deterministic error after repeated unsatisfied no-tool responses.
- **Paths**: `internal/tools/filter_test.go`, `internal/engine/loop_test.go`
- **Dependencies**: T004
- **Verification**: Focused engine/tools tests fail because the optional turn and completion hooks do not yet exist.
- **Covers**: REQ-003, REQ-008, REQ-009, NFR-004, NFR-005; Plan: Phase 3, PLD-002, PLD-003, C3, C4

### T006 - [x] Implement Engine Turn And Completion Hooks

- **Outcome**: Registries may implement `BeginTurn`; filtered registries preserve it; the engine invokes it once per turn before tool definitions and supports a bounded run-local completion gate without changing provider or session schemas.
- **Paths**: `internal/tools/registry.go`, `internal/tools/filter.go`, `internal/engine/config.go`, `internal/engine/loop.go`, tests from T005
- **Dependencies**: T005
- **Verification**: Focused `internal/tools` and `internal/engine` tests pass, followed by existing engine reporter/TODO-gate regression tests.
- **Covers**: REQ-003, REQ-008, REQ-009, NFR-004, NFR-005; Plan: Phase 3, PLD-002, PLD-003, C3, C4

### T007 - [x] Add Failing Tests For Formal Lifecycle Registries And Prompt Guidance

- **Outcome**: App/context tests define the required canonical Formal tools, forbidden tool absence, compatible ask alias, restricted-command preflight, high-priority file/Git/side-effect prohibitions, revision staying Formal, turn-delayed approval, checklist gating, full approved-plan reinjection, successful TODO transition, same-batch denial, and one-run/one-user-message continuity.
- **Paths**: `internal/context/prompt_test.go`, `internal/app/plan_lifecycle_test.go`, `internal/app/runner_test.go`
- **Dependencies**: T006
- **Verification**: Focused context/app tests fail for missing lifecycle registry, prompt mode option, reminder, and registry transitions.
- **Covers**: REQ-003, REQ-004, REQ-005, REQ-007, REQ-008, REQ-009, REQ-010, NFR-001, NFR-002, NFR-004, NFR-005; Plan: Phase 3, PLD-002, PLD-003, PLD-006, PLD-007, C3-C5, C7

### T008 - [x] Implement The Same-Run Formal Lifecycle And Prompt Contract

- **Outcome**: Formal runs use a private formal/checklist/default registry state machine, approval selects Default but commits active tools next turn, approved plan reminders survive compaction, checklist initialization precedes full Default tools, restricted incompatible Formal runs fail before model invocation, and Default runs retain the existing direct registry path.
- **Paths**: `internal/app/plan_lifecycle.go`, `internal/app/runner.go`, `internal/context/prompt.go`, tests from T007
- **Dependencies**: T007
- **Verification**: Focused app/context acceptance tests pass with fake providers; assertions confirm one run ID, one original user record, full plan context, `update_todo` before implementation tools, and unchanged Default call count.
- **Covers**: REQ-003, REQ-004, REQ-005, REQ-007, REQ-008, REQ-009, REQ-010, NFR-001, NFR-002, NFR-003, NFR-004, NFR-005; Plan: Phase 3, PLD-002, PLD-003, PLD-006, PLD-007, C3-C5, C7

## Phase 4: TUI Plan Review Experience

### T009 - [x] Add Failing Tests For The Plan Reviewer Bridge And Inline Form

- **Outcome**: Tests define blocking request/reply behavior, context cancellation, exact source retention, Markdown display origin, plan scrolling, approve/continue selection, optional revision feedback, cancel-without-approval, listener re-arming, approval footer reset, and no interference with ask/rewind/input controls.
- **Paths**: `internal/tui/plan_reviewer_test.go`, `internal/tui/planform_test.go`, `internal/tui/planform_integration_test.go`, `internal/tui/model_test.go`
- **Dependencies**: T008
- **Verification**: `go test ./internal/tui` fails for missing reviewer/form integration and approval state synchronization.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-006, REQ-007, REQ-008, NFR-002, NFR-004, NFR-005; Plan: Phase 4, PLD-005, C6

### T010 - [x] Implement The Plan Reviewer Bridge, Form, And App Wiring

- **Outcome**: The TUI keeps the transcript visible while reviewing the exact submitted plan, supports bounded scrolling and approval/revision controls, returns decisions to the blocked tool, re-arms listeners, resets selected mode on approval, and wires both `UserAsker` and `PlanReviewer` through `RunTUI`.
- **Paths**: `internal/tui/plan_reviewer.go`, `internal/tui/planform.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/reporter.go`, `internal/app/tui.go`, tests from T009
- **Dependencies**: T009
- **Verification**: `go test ./internal/tui ./internal/app` passes, including existing ask form, Esc, sidebar, markdown, selection, and statusline tests.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-006, REQ-007, REQ-008, NFR-002, NFR-004, NFR-005; Plan: Phase 4, PLD-005, C6, C7

## Phase 5: Automatic Planner Migration Across Entry Points

### T011 - [x] Add Failing Migration Tests For Default Entry Points

- **Outcome**: Tests prove one-shot CLI, shared AgentRunner, AgentOps, and benchmark perform no Planner pre-pass; AgentOps and benchmark expose TODO tools; Feishu retains its registry/no-preplan behavior; and the Autodev SDD adapter still creates ordinary Default core runners.
- **Paths**: `internal/app/runner_test.go`, `internal/app/autodev_test.go`, `internal/agentops/runner_test.go`, `internal/feishu/runner_test.go`, `cmd/bench/main_test.go`, `cmd/fox/main_test.go`
- **Dependencies**: T010
- **Verification**: Focused package tests expose remaining Planner invocations and the benchmark's missing TODO tools.
- **Covers**: REQ-011, REQ-012, REQ-013, NFR-003, NFR-004, NFR-005; Plan: Phase 5, PLD-008, C7

### T012 - [x] Finish Legacy Planner Removal And Align Entry-Point Registries

- **Outcome**: Verify the shared runner pre-pass was replaced by T008, remove the remaining AgentOps and benchmark pre-passes, add benchmark `read_todo`/`update_todo`, delete the now-unreferenced Planner implementation/tests and stale Plan comments, and preserve Feishu/Autodev behavior.
- **Paths**: `internal/app/runner.go`, `internal/app/cli.go`, `internal/app/autodev.go`, `internal/agentops/runner.go`, `cmd/bench/main.go`, `internal/memory/plan.go` (delete), `internal/memory/plan_test.go` (delete), tests from T011
- **Dependencies**: T011
- **Verification**: Focused app/AgentOps/Feishu/benchmark/CLI tests pass and static searches find no production Planner or `EnablePlanMode` references.
- **Covers**: REQ-011, REQ-012, REQ-013, NFR-003, NFR-004, NFR-005; Plan: Phase 5, PLD-008, C7

## Phase 6: Rewind Regression, Full Verification, And Review

### T013 - [x] Add End-To-End Lifecycle And Rewind Regression Tests

- **Outcome**: Deterministic acceptance tests cover submit-revise-resubmit-approve, exact latest PLAN, approved-plan TODO derivation before a fake implementation call, and restoration of pre-message PLAN/TODO through existing state history and runner rewind APIs.
- **Paths**: `internal/app/plan_lifecycle_test.go`, `internal/memory/state_history_test.go`, `internal/tui/model_test.go`
- **Dependencies**: T012
- **Verification**: New focused tests pass when earlier phases are complete. Any uncovered integration/rewind failure is recorded before T014 changes the implicated production path.
- **Covers**: REQ-006, REQ-007, REQ-008, REQ-009, REQ-014, NFR-002, NFR-004, NFR-005; Plan: Phase 6, C8, Verification Strategy

### T014 - [x] Close Lifecycle/Rewind Integration Gaps And Refactor Green

- **Outcome**: Any T013 integration gaps are fixed without changing product semantics; the same pre-user-message snapshot restores both plan proposal and execution checklist; lifecycle interfaces and synchronization are refactored for readability while focused tests remain green.
- **Paths**: Only production/test files implicated by T013, primarily `internal/app`, `internal/memory`, and `internal/tui`
- **Dependencies**: T013
- **Verification**: Focused lifecycle, state-history, and TUI rewind tests pass with `-count=1`.
- **Covers**: REQ-006, REQ-007, REQ-008, REQ-009, REQ-014, NFR-002, NFR-004, NFR-005; Plan: Phase 6, C8, Verification Strategy

### T015 - [x] Format And Run Static Migration Checks

- **Outcome**: All changed Go files are gofmt-formatted and no production `EnablePlanMode`, legacy Plan accessor, registered `-plan`, `NewPlanner`, or `BuildPlan` remains.
- **Paths**: All changed Go files and repository root
- **Dependencies**: T014
- **Verification**: `gofmt -w` on changed Go files; planned `rg` migration checks return no prohibited production matches.
- **Covers**: REQ-011, REQ-012, NFR-003, NFR-004; Plan: Phase 6, Verification Strategy

### T016 - [x] Run Focused And Full Test Suites

- **Outcome**: Every focused package suite and `go test ./...` pass from a clean test cache where relevant.
- **Paths**: Repository root
- **Dependencies**: T015
- **Verification**: Commands in the plan's Verification Strategy pass, followed by `go test ./...`.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, NFR-001, NFR-002, NFR-003, NFR-004, NFR-005; Plan: Phase 6, Verification Strategy

### T017 - [x] Perform Final Code Review And Fix Verified Findings

- **Outcome**: Changed analyzable code is reviewed for correctness, requirement fidelity, concurrency, error handling, TDD evidence, and Go documentation; eligible verified findings are fixed test-first and the final full suite remains green.
- **Paths**: Changed analyzable Go files outside `.codexspec/specs`
- **Dependencies**: T016
- **Verification**: Final review has no unresolved Critical/High findings and `go test ./...` passes after every accepted fix.
- **Covers**: NFR-001, NFR-002, NFR-004, NFR-005; Plan: Phase 6, Verification Strategy

## Dependency Summary

`T001 -> T002 -> T003 -> T004 -> T005 -> T006 -> T007 -> T008 -> T009 -> T010 -> T011 -> T012 -> T013 -> T014 -> T015 -> T016 -> T017`

The sequence is intentionally linear because each phase changes interfaces consumed by the next phase and the constitution requires a demonstrated RED test before each production behavior change.

## Coverage Table

| Requirement / Plan Item | Task References |
|-------------------------|-----------------|
| REQ-001 | T001, T002, T009, T010, T016 |
| REQ-002 | T001, T002, T009, T010, T016 |
| REQ-003 | T001, T002, T005-T010, T016 |
| REQ-004 | T007, T008, T016 |
| REQ-005 | T003, T004, T007, T008, T016 |
| REQ-006 | T003, T004, T009, T010, T013, T014, T016 |
| REQ-007 | T003, T004, T007-T010, T013, T014, T016 |
| REQ-008 | T005-T010, T013, T014, T016 |
| REQ-009 | T005-T008, T013, T014, T016 |
| REQ-010 | T003, T004, T007, T008, T016 |
| REQ-011 | T001, T002, T011, T012, T015, T016 |
| REQ-012 | T001, T002, T011, T012, T015, T016 |
| REQ-013 | T011, T012, T016 |
| REQ-014 | T003, T004, T013, T014, T016 |
| NFR-001 | T007, T008, T016, T017 |
| NFR-002 | T003, T004, T007-T010, T013, T014, T016, T017 |
| NFR-003 | T001, T002, T008, T011, T012, T015, T016 |
| NFR-004 | T001-T017 |
| NFR-005 | T001-T014, T016, T017 |
| C1 / Phase 1 | T001, T002 |
| C2 / Phase 2 | T003, T004 |
| C3-C5 / Phase 3 | T005-T008 |
| C6 / Phase 4 | T009, T010 |
| C7 / Phase 5 | T011, T012 |
| C8 / Phase 6 | T013, T014 |
| Verification Strategy | T013-T017 |

## Unmapped Tasks

None.
