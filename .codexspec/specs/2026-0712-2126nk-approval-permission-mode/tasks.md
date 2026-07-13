# Tasks: Approval Permission Mode

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Feature ID**: `2026-0712-2126nk`
**Input**: `.codexspec/specs/2026-0712-2126nk-approval-permission-mode/plan.md`
**Status**: Draft

## Task Groups

Tasks preserve the approved plan's seven implementation phases and the project constitution's mandatory Red-Green-Refactor workflow. Each production behavior task depends on a test task whose expected RED result is recorded before implementation. The permission boundary is implemented only for interactive TUI execution and does not claim operating-system isolation.

## Phase 1: Domain State And Settings

### T001 - [x] Add Failing Tests For Permission State And Persisted Settings

- **Outcome**: Tests define mode parsing and invalid-mode fallback, selected/effective mode separation, acknowledged and unacknowledged Full Access startup, independent acknowledgment reset, grant retention across mode changes, grant clearing, settings round-trip, explicit `false` merge, and unknown-field preservation.
- **Paths**: `internal/permission/state_test.go`, `internal/settings/settings_test.go`
- **Dependencies**: None
- **Verification**: Focused permission and settings tests fail for the missing domain types and settings fields for the expected reasons; existing settings tests remain buildable.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-017, REQ-018; Plan: Phase 1, Components 1 and 6, PLD-001, PLD-007, PLD-009

### T002 - [x] Implement Permission State And Transactional Settings Schema

- **Outcome**: A documented `internal/permission` package defines normalized modes, selected/effective state, warning acknowledgment, synchronized in-memory grants, and lifecycle operations; settings persist `tui.permission_mode` and `tui.full_access_warning_acknowledged`, preserve unrelated JSON, and support explicit acknowledgment reset without activating invalid Full Access state.
- **Paths**: `internal/permission/doc.go`, `internal/permission/mode.go`, `internal/permission/types.go`, `internal/permission/state.go`, `internal/settings/settings.go`, tests from T001
- **Dependencies**: T001
- **Verification**: Focused `internal/permission` and `internal/settings` tests pass, including missing, invalid, round-trip, reset, and unknown-field cases.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-017, REQ-018; Plan: Phase 1, Components 1 and 6, PLD-001, PLD-007, PLD-009

## Phase 2: Canonical Policy And Bash Fast Path

### T003 - [x] Add Failing Tests For Canonical Paths, Grant Keys, And Shell Policy

- **Outcome**: Table tests define workspace containment for relative, absolute, missing, `..`, and symlink paths; every confirmed read-only command and rejected option; all-safe chain and pipeline handling; every unsupported shell construct; policy categories; and canonical typed grant equivalence.
- **Paths**: `internal/permission/path_test.go`, `internal/permission/grant_test.go`, `internal/permission/policy_test.go`, `internal/permission/shell_test.go`
- **Dependencies**: T002
- **Verification**: `go test ./internal/permission` fails because structured parsing, canonical resolution, policy classification, and grant-key generation are not implemented; failure cases distinguish review-required results from denial.
- **Covers**: REQ-004, REQ-006, REQ-007, REQ-017, NFR-001; Plan: Phase 2, Component 2, PLD-004, PLD-007

### T004 - [x] Implement Canonical Policy And Structured Bash Classification

- **Outcome**: Policy classification uses `mvdan.cc/sh/v3/syntax`, symlink-aware path resolution, command-specific option validators, complete atomic-command validation for chains and pipelines, fail-closed review routing, and typed grant keys without prefix-, directory-, or tool-wide expansion.
- **Paths**: `go.mod`, `go.sum`, `internal/permission/path.go`, `internal/permission/grant.go`, `internal/permission/policy.go`, `internal/permission/shell.go`, tests from T003
- **Dependencies**: T003
- **Verification**: `go test ./internal/permission` passes all allowlist, unsafe-option, unsupported-AST, path-escape, and canonical-key tables; unknown or uncertain input is review-required and never fast-path allowed.
- **Covers**: REQ-004, REQ-006, REQ-007, REQ-017, NFR-001; Plan: Phase 2, Component 2, PLD-004, PLD-007, Security Considerations

## Phase 3: Coordinator, FIFO, Grants, And Cancellation

### T005 - [x] Add Failing Tests For Authorization Coordination And Registry Ordering

- **Outcome**: Tests define fast-path and matching-grant bypass, Ask and Approve routing, exact allow-once, session-grant reuse, deny-and-continue results, feedback and reviewer cancellation, mode/grant re-evaluation, one active reviewed request, model-order FIFO, Full Access boundary, non-rollback, registry not-found behavior, and safe composite re-entry without deadlock.
- **Paths**: `internal/permission/coordinator_test.go`, `internal/permission/queue_test.go`, `internal/permission/registry_test.go`
- **Dependencies**: T004
- **Verification**: Focused tests fail for missing coordinator, queue, request decisions, context propagation, and registry decorator behavior; concurrency cases are deterministic and bounded by test contexts.
- **Covers**: REQ-004, REQ-005, REQ-008, REQ-009, REQ-015, REQ-016, REQ-017, REQ-018, NFR-001, NFR-002; Plan: Phase 3, Component 4, PLD-002, PLD-003, PLD-007

### T006 - [x] Implement The Shared Coordinator And Permission Registry

- **Outcome**: A context-aware FIFO coordinator authorizes canonical requests, stores typed session grants, re-evaluates queued calls, returns typed denial/cancellation results, and decorates registries outside tool middleware. TUI registries report calls as non-parallel-safe, reviewed leaf calls retain queue ownership through execution, and authorized composite calls release the parent ticket before nested execution while the engine still blocks sibling dispatch.
- **Paths**: `internal/permission/coordinator.go`, `internal/permission/queue.go`, `internal/permission/registry.go`, `internal/permission/context.go`, tests from T005
- **Dependencies**: T005
- **Verification**: `go test ./internal/permission` and `go test -race ./internal/permission` pass; tests prove FIFO ordering, idempotent cancellation cleanup, no started-call rollback, no nested deadlock, and no approval bypass through Full Access of outer hard constraints.
- **Covers**: REQ-004, REQ-005, REQ-008, REQ-009, REQ-015, REQ-016, REQ-017, REQ-018, NFR-001, NFR-002; Plan: Phase 3, Component 4, PLD-002, PLD-003, PLD-007

## Phase 4: Evidence And LLM Reviewer

### T007 - [x] Add Failing Tests For Trust-Aware Reviewer Evidence

- **Outcome**: Tests define typed trust labels, direct ask-user answer recognition, trusted project-instruction loading, initial/latest message retention, separate trusted and untrusted budgets, exact action/cwd/workspace/source inclusion, explicit truncation markers, external-source scope limits, and escalation when trusted authorization is insufficient.
- **Paths**: `internal/permission/evidence_test.go`, `internal/context/prompt_test.go`
- **Dependencies**: T006
- **Verification**: Focused permission/context tests fail for the absent evidence builder and reusable project-instruction loader, with assertions that assistant, tool, Skill, and file content cannot independently establish authorization.
- **Covers**: REQ-013, REQ-014; Plan: Phase 4, Component 3, PLD-006, Execution Evidence Context

### T008 - [x] Implement Bounded Trust-Aware Evidence Construction

- **Outcome**: The reviewer evidence builder consumes typed session records, applicable project instructions, exact canonical request context, and separate bounded trusted/untrusted sections while preserving initial and recent user intent and marking every truncation; the prompt composer and reviewer reuse one project-instruction loading contract.
- **Paths**: `internal/permission/evidence.go`, `internal/context/prompt.go`, tests from T007
- **Dependencies**: T007
- **Verification**: `go test ./internal/permission ./internal/context` passes all trust, budget, retention, and truncation cases; untrusted evidence never changes the computed authorization boundary.
- **Covers**: REQ-013, REQ-014; Plan: Phase 4, Component 3, PLD-006, Execution Evidence Context

### T009 - [x] Add Failing Tests For Isolated Review Decisions And Reliability

- **Outcome**: Tests define live provider/model lookup, isolated messages, an empty tool surface, strict structured parsing, the confirmed risk/authorization matrix, exact-invocation approval, immediate valid escalation, three logical attempts, one shared 90-second deadline, provider-retry independence, cancellability, retry status, and fallback after technical exhaustion.
- **Paths**: `internal/permission/reviewer_test.go`
- **Dependencies**: T008
- **Verification**: `go test ./internal/permission` fails for the missing reviewer adapter, strict result parser, decision matrix, retry loop, and fallback signaling; fake providers distinguish logical calls from provider transport retries.
- **Covers**: REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, REQ-015; Plan: Phase 4, Component 3, PLD-005, PLD-006

### T010 - [x] Implement The Tool-Free LLM Reviewer And Retry Budget

- **Outcome**: A separate reviewer invocation resolves the provider and model at call time, receives only bounded evidence and no tools, accepts only strict `approve` or `escalate` results, applies the confirmed risk matrix, retries technical failures within three logical attempts and one 90-second context, and signals user fallback after exhaustion without denying the task.
- **Paths**: `internal/permission/reviewer.go`, tests from T009
- **Dependencies**: T009
- **Verification**: `go test ./internal/permission` passes exact-call, model-switch, risk matrix, malformed-output, timeout, cancellation, retry, immediate-escalation, and exhaustion-fallback tests.
- **Covers**: REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, REQ-015; Plan: Phase 4, Component 3, PLD-005, PLD-006

## Phase 5: Main And Nested Runtime Wiring

### T011 - [x] Add Failing Tests For TUI-Scoped Main And Plan-Lifecycle Wiring

- **Outcome**: Runner tests define optional TUI-only coordinator attachment, authorization before checkpoint/tool effects, decorated default/Formal/checklist base registries, Plan lifecycle and restricted-tool filters outside permission bypass, active-model lookup after `/model`, new-session grant clearing, rewind/compact grant retention, and unchanged non-interactive registries.
- **Paths**: `internal/app/runner_test.go`, `internal/app/plan_lifecycle_test.go`, `internal/app/tui_test.go`
- **Dependencies**: T010
- **Verification**: Focused app tests fail because the runner cannot yet receive or consistently apply a shared coordinator to every TUI base registry; assertions prove outer Plan and allow-list constraints remain effective in Full Access.
- **Covers**: REQ-005, REQ-010, REQ-016, REQ-018, REQ-021, NFR-001, NFR-002; Plan: Phase 5, Component 5, PLD-001, PLD-002, PLD-003, PLD-005

### T012 - [x] Wire The Coordinator Into Main And Plan-Lifecycle Registries

- **Outcome**: `AgentRunner` accepts an optional permission coordinator supplied only by `RunTUI`; default, Formal, and checklist base registries are individually decorated outside checkpoint/tool middleware, while Plan lifecycle and allowed-tool filters remain outer constraints. Session lifecycle hooks clear or retain grants according to the approved boundary, and provider lookup remains live.
- **Paths**: `internal/app/runner.go`, `internal/app/plan_lifecycle.go`, `internal/app/tui.go`, tests from T011
- **Dependencies**: T011
- **Verification**: `go test ./internal/app` passes registry-ordering, lifecycle, provider-switch, Full Access constraint, new/clear, rewind/compact, and non-interactive regression tests.
- **Covers**: REQ-005, REQ-010, REQ-016, REQ-018, REQ-021, NFR-001, NFR-002; Plan: Phase 5, Component 5, PLD-001, PLD-002, PLD-003, PLD-005

### T013 - [x] Add Failing Tests For Delegated, Skill, Forked, And Nested Enforcement

- **Outcome**: Tests define one coordinator and grant set across delegated Subagents, fork runners, top-level Skill calls, embedded shell/hooks, policy-unknown advertised tools, and nested calls; composite authorization re-entry does not deadlock, and unregistered/unadvertised tools preserve the existing not-found result.
- **Paths**: `internal/subagent/manager_test.go`, `internal/subagent/filter_test.go`, `internal/slash/skilltool/tool_test.go`, `internal/app/fork_runner_test.go`, `internal/app/runner_test.go`
- **Dependencies**: T012
- **Verification**: Focused app/subagent/skilltool tests fail for missing coordinator propagation and pre-pipeline review; tests show that each nested source reaches the same policy and that unknown registered tools default to review.
- **Covers**: REQ-004, REQ-005, REQ-018, REQ-021, NFR-001, NFR-002; Plan: Phase 5, Component 5, PLD-001, PLD-002, PLD-003

### T014 - [x] Enforce Permissions Across Delegated And Composite Execution

- **Outcome**: Subagent managers, fork runners, and Skill execution inherit the shared coordinator and invocation context; every nested base registry is decorated, top-level Skill calls are reviewed before shell/hooks/forks run, and composite ticket handling permits nested authorization without allowing sibling calls to overtake model order.
- **Paths**: `internal/subagent/manager.go`, `internal/slash/skilltool/tool.go`, `internal/app/runner.go`, tests from T013
- **Dependencies**: T013
- **Verification**: `go test ./internal/app ./internal/subagent ./internal/slash/skilltool` passes inherited-policy, pre-pipeline, filter-ordering, unknown-tool, nested-re-entry, and non-interactive regression cases.
- **Covers**: REQ-004, REQ-005, REQ-018, REQ-021, NFR-001, NFR-002; Plan: Phase 5, Component 5, PLD-001, PLD-002, PLD-003

## Phase 6: TUI Commands, Forms, Audit, And Status

### T015 - [x] Add Failing Tests For `/permissions` And Full Access Transactions

- **Outcome**: Model/form tests define the canonical command, exactly three modes, current selection, enabled/counting grant-clear action, current-run and remembered Full Access choices, startup warning, dismissal behavior, persistence-before-live-state ordering, save rollback, listener re-arming, narrow-width rendering, and no interference with existing overlays or input controls.
- **Paths**: `internal/tui/model_test.go`, `internal/tui/slash_registry_test.go`, `internal/tui/permission_form_test.go`, `internal/tui/full_access_form_test.go`
- **Dependencies**: T014
- **Verification**: `go test ./internal/tui` fails for the missing command, forms, settings transaction, startup warning, and grant-clear interaction while existing selector and overlay tests remain buildable.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-018, NFR-004; Plan: Phase 6, Components 6 and 7, PLD-009

### T016 - [x] Implement `/permissions`, Grant Clearing, And Full Access Warning

- **Outcome**: The TUI exposes the three-mode picker and separate clear action, persists mode and remembered acknowledgment before committing live state, restores prior state on save failure, supports one-run Full Access without persisting acknowledgment, gates unacknowledged startup Full Access, and re-arms form listeners safely.
- **Paths**: `internal/tui/permission_form.go`, `internal/tui/full_access_form.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/slash_registry.go`, tests from T015
- **Dependencies**: T015
- **Verification**: `go test ./internal/tui` passes command discovery, all mode transitions, warning paths, clear-count, persistence failure, cancellation, overlay compatibility, and narrow-terminal tests.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-018, NFR-004; Plan: Phase 6, Components 6 and 7, PLD-009

### T017 - [x] Add Failing Tests For Approval Forms, Audit Correlation, And Status

- **Outcome**: Tests define all four user decisions, optional feedback entry, exact action/cwd/grant scope, review/retry status, exhaustion disclosure, cancellation, tool-call-ID event ordering, existing-entry auto-approval annotation, collapsed rationale, no fast-path annotation, `/status` mode/count, optional non-default `permissions` statusline item, and unconditional bottom Full Access warning.
- **Paths**: `internal/tui/permission_bridge_test.go`, `internal/tui/approval_form_test.go`, `internal/tui/reporter_test.go`, `internal/tui/model_test.go`, `internal/engine/reporter_test.go`
- **Dependencies**: T016
- **Verification**: Focused TUI/engine tests fail for missing request/reply bridges, detailed reporter events, pending audit correlation, status surfaces, and persistent bottom warning; duplicate tool rows are explicitly rejected.
- **Covers**: REQ-008, REQ-015, REQ-016, REQ-019, REQ-020, NFR-004; Plan: Phase 6, Components 7 and 8, PLD-008, User Approval And Review, Optional Detailed Reporter

### T018 - [x] Implement Approval UX, Review Audit, And Permission Status

- **Outcome**: Context-aware bridges connect the coordinator to inline approval and review states; feedback is queued as the next user instruction before the current run is cancelled; optional detailed reporter callbacks correlate metadata by tool-call ID, including events that arrive before entries; and status, statusline, rationale details, retry state, and Full Access warning render without duplicate normal tool output.
- **Paths**: `internal/tui/permission_bridge.go`, `internal/tui/approval_form.go`, `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/statusline.go`, `internal/tui/reporter.go`, `internal/engine/reporter.go`, `internal/engine/loop.go`, tests from T017
- **Dependencies**: T017
- **Verification**: `go test ./internal/tui ./internal/engine` passes all approval decisions, cancellation, event reordering, duplicate-name, collapsed-detail, statusline opt-in, Full Access warning, and narrow-width rendering tests.
- **Covers**: REQ-008, REQ-015, REQ-016, REQ-019, REQ-020, NFR-004; Plan: Phase 6, Components 7 and 8, PLD-008, User Approval And Review, Optional Detailed Reporter

## Phase 7: End-To-End Security And Regression Verification

### T019 - [x] Add And Pass Cross-Package Permission Acceptance Tests

- **Outcome**: Deterministic integration tests cover complete Ask and Approve flows, automatic approval, valid escalation, technical exhaustion fallback, nested delegation and Skill execution, model switching, new-session grant clearing, rewind/compact retention, Full Access restart behavior, and persisted-mode isolation from non-interactive entry points using fake providers and temporary settings.
- **Paths**: `internal/app/permission_integration_test.go`, `internal/tui/permission_integration_test.go`, `internal/subagent/manager_test.go`
- **Dependencies**: T018
- **Verification**: New cross-package tests pass with `-count=1`; any discovered behavior gap is first preserved as a failing regression case before the implicated implementation is minimally corrected and refactored green.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, REQ-015, REQ-016, REQ-017, REQ-018, REQ-019, REQ-020, REQ-021, NFR-001, NFR-002, NFR-004; Plan: Phase 7, Unit Tests, Integration Tests

### T020 - [x] Format And Run Race, Full, And Static Verification

- **Outcome**: All changed Go files are formatted; focused package suites, permission/TUI/app/subagent race suites, the complete repository test suite, static analysis, and source-independent artifact checks pass without weakening fail-closed or non-interactive behavior.
- **Paths**: All changed Go files and repository root
- **Dependencies**: T019
- **Verification**: Run `gofmt -w` on changed Go files, focused package tests, `go test -race ./internal/permission ./internal/tui ./internal/app ./internal/subagent`, `go test ./...`, and `go vet ./...`; every command passes. A static artifact scan confirms requirements, specification, plan, and tasks contain no external source paths, symbols, or source-derived implementation claims.
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, REQ-015, REQ-016, REQ-017, REQ-018, REQ-019, REQ-020, REQ-021, NFR-001, NFR-002, NFR-003, NFR-004; Plan: Phase 7, Full Verification

### T021 - [x] Perform Final Security And Code Review

- **Outcome**: Changed code is reviewed for fail-closed shell parsing, symlink/path equivalence, evidence trust boundaries, exact-call binding, wrapper ordering, nested bypasses, queue/cancellation leaks, settings transactionality, non-interactive isolation, Go documentation, and requirement fidelity; every verified defect is fixed test-first and refactored with all tests green.
- **Paths**: Changed analyzable Go files outside `.codexspec/specs`
- **Dependencies**: T020
- **Verification**: Final review reports no unresolved Critical or High findings; focused tests and `go test ./...` pass after each accepted correction.
- **Covers**: REQ-004, REQ-005, REQ-006, REQ-007, REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, REQ-015, REQ-016, REQ-017, REQ-018, REQ-021, NFR-001, NFR-002, NFR-004; Plan: Phase 7, Security Considerations, Risks / Trade-offs

## Checkpoints

- **Checkpoint A - Domain And Policy**: After T004, state persistence, canonical paths, typed grants, and deterministic policy are green before concurrency is introduced.
- **Checkpoint B - Authorization Core**: After T010, the queue, registry boundary, evidence builder, and isolated reviewer are race-tested and ready for runtime integration.
- **Checkpoint C - Runtime Enforcement**: After T014, every main, Plan lifecycle, delegated, Skill, forked, and nested TUI execution path shares the coordinator while non-interactive behavior remains unchanged.
- **Checkpoint D - User Experience**: After T018, permission selection, warnings, approvals, audit metadata, and status surfaces are complete and low-noise.
- **Checkpoint E - Release Readiness**: After T021, integration, race, full-suite, vet, and security review gates are green.

## Dependency Summary

`T001 -> T002 -> T003 -> T004 -> T005 -> T006 -> T007 -> T008 -> T009 -> T010 -> T011 -> T012 -> T013 -> T014 -> T015 -> T016 -> T017 -> T018 -> T019 -> T020 -> T021`

The main chain is intentionally linear. Each phase introduces interfaces or enforcement order consumed by the next phase, and every implementation task requires recorded RED evidence from its preceding test task. No task is marked `[P]` because concurrent execution would overlap shared permission contracts or begin against unfinished dependencies.

## Coverage Table

| Requirement / Plan Item | Task References |
|-------------------------|-----------------|
| REQ-001 | T001, T002, T015, T016, T019, T020 |
| REQ-002 | T001, T002, T015, T016, T019, T020 |
| REQ-003 | T001, T002, T015, T016, T019, T020 |
| REQ-004 | T003-T006, T013, T014, T019-T021 |
| REQ-005 | T005, T006, T011-T014, T019-T021 |
| REQ-006 | T003, T004, T019-T021 |
| REQ-007 | T003, T004, T019-T021 |
| REQ-008 | T005, T006, T017-T020 |
| REQ-009 | T005, T006, T009, T010, T019-T021 |
| REQ-010 | T009-T012, T019-T021 |
| REQ-011 | T009, T010, T019-T021 |
| REQ-012 | T009, T010, T019-T021 |
| REQ-013 | T007-T010, T019-T021 |
| REQ-014 | T007-T010, T019-T021 |
| REQ-015 | T005, T006, T009, T010, T017-T021 |
| REQ-016 | T005, T006, T011, T012, T017-T021 |
| REQ-017 | T001-T006, T019-T021 |
| REQ-018 | T001, T002, T005, T006, T011-T016, T019-T021 |
| REQ-019 | T017-T020 |
| REQ-020 | T017-T020 |
| REQ-021 | T011-T014, T019-T021 |
| NFR-001 | T003-T006, T011-T014, T019-T021 |
| NFR-002 | T005, T006, T011-T014, T019-T021 |
| NFR-003 | T020 and this source-independent task artifact |
| NFR-004 | T015-T021 |
| Phase 1 / Components 1 and 6 | T001, T002 |
| Phase 2 / Component 2 | T003, T004 |
| Phase 3 / Component 4 | T005, T006 |
| Phase 4 / Component 3 | T007-T010 |
| Phase 5 / Component 5 | T011-T014 |
| Phase 6 / Components 6-8 | T015-T018 |
| Phase 7 / Verification Strategy | T019-T021 |
| PLD-001 through PLD-009 | T001-T021 as referenced by each task's Plan mapping |

## Unmapped Tasks

None.

## Unresolved Items

None. Implementation must stop for re-planning if verified repository behavior invalidates an approved plan assumption or requires a new product or architecture decision.
