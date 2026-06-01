# Tasks: /keep-run — Autonomous SDD Pipeline Runner

**Input**: Design documents from `.codexspec/specs/2026-0531-23020o-keep-run-sdd-pipeline/`
**Prerequisites**: `plan.md` (required), `spec.md` (required for user stories and acceptance criteria)

**Tests**: This project mandates TDD per constitution. All code tasks follow Red-Green-Refactor.

**Organization**: Tasks are grouped by phase (setup → core TDD → integration → interface → validation). User story mappings are indicated per task.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Include exact file paths in descriptions

## Path Conventions

- **Go source**: `internal/keeprun/`
- **Go tests**: `internal/keeprun/` (colocated `*_test.go` files)
- **Prompt commands**: `.claude/commands/codexspec/`
- **Config**: `keep-run.config.json` (project root, runtime only)
- **Backlog**: `BACKLOG.md` (project root, runtime only)

---

## Phase 1: Setup

**Purpose**: Create the `internal/keeprun/` package directory and package documentation.

- [ ] T001 [US1] Create `internal/keeprun/` package with `doc.go`
  - **Type**: Setup
  - **Files**: `internal/keeprun/doc.go`
  - **Description**: Create the package directory and `doc.go` with package-level block comment. The comment should describe the keeprun package as providing data types, parsers, and algorithms for the `/keep-run` autonomous SDD pipeline runner — including backlog parsing, slug generation, config loading, phase definitions, and git worktree management.
  - **Dependencies**: None
  - **Est. Complexity**: Low

---

## Phase 2: Core TDD — Pure Logic Modules

**Purpose**: Implement all pure-logic modules with TDD. These modules have no internal dependencies on each other and can be developed in parallel.

**CRITICAL**: All four module tracks (slug, config, phase, backlog) are independent. Within each track, tests MUST be written first and fail before implementation begins.

### Track A: Slug Module [P]

> Covers FR-005 (Git Worktree Isolation — slug algorithm). Referenced by US1 (worktree branch naming), US2 (task slug from title).

- [ ] T002 [P] [US1] Write tests for `GenerateSlug` in `internal/keeprun/slug_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/slug_test.go`
  - **Description**: Write table-driven tests for `GenerateSlug` covering all cases from spec FR-005: (1) standard title `[feature] Add dark mode support` → `add-dark-mode-support`, (2) special characters `[fix] Fix timeout on slow connections!!!` → `fix-timeout-on-slow-connections`, (3) type prefix stripping, (4) lowercase conversion, (5) hyphen collapse, (6) leading/trailing hyphen strip, (7) truncation at 60 chars with hyphen boundary, (8) unicode input, (9) empty string after stripping, (10) title with only special characters. Verify all tests fail (Red phase).
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [ ] T003 [US1] Implement `GenerateSlug` in `internal/keeprun/slug.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/slug.go`
  - **Description**: Implement `GenerateSlug(title string) string` following the exact 7-step algorithm from spec FR-005. Include block comment documenting the algorithm steps. All tests from T002 must pass (Green phase).
  - **Dependencies**: T002
  - **Est. Complexity**: Low

- [ ] T004 [P] [US1] Write tests for `DeduplicateSlug` in `internal/keeprun/slug_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/slug_test.go`
  - **Description**: Write table-driven tests for `DeduplicateSlug(slug string, existing []string) string` covering: (1) no collision → returns slug unchanged, (2) single collision → appends `-2`, (3) multiple collisions → increments (`-2`, `-3`, etc.), (4) slug already has numeric suffix and collides → continues incrementing, (5) empty existing list. Verify tests fail.
  - **Dependencies**: T002
  - **Est. Complexity**: Low

- [ ] T005 [US1] Implement `DeduplicateSlug` in `internal/keeprun/slug.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/slug.go`
  - **Description**: Implement `DeduplicateSlug` — step 8 of the slug algorithm. On collision with an existing branch name, append `-2`, `-3`, etc. All tests from T004 must pass.
  - **Dependencies**: T004
  - **Est. Complexity**: Low

### Track B: Config Module [P]

> Covers FR-008 (Configuration File), FR-009 (Config defaults). Referenced by US1, US4, US5.

- [ ] T006 [P] [US1] Write tests for config package in `internal/keeprun/config_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/config_test.go`
  - **Description**: Write table-driven tests for `LoadConfig` and `DefaultConfig` covering: (1) valid complete config file, (2) missing config file → returns defaults, (3) partial config (some fields missing) → defaults for missing fields, (4) empty JSON object `{}` → all defaults, (5) invalid JSON → error, (6) `DefaultConfig()` returns expected defaults matching spec FR-008 table: `remote_enabled: true`, `review_mode: "subagent"`, default prompts, exponential backoff. Verify tests fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [ ] T007 [US1] Implement config package in `internal/keeprun/config.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/config.go`
  - **Description**: Define `Config` and `RetryPolicy` structs matching plan's interface. Implement `LoadConfig(dir string) (Config, error)` — read `keep-run.config.json` from dir, apply defaults for missing fields. Implement `DefaultConfig() Config` returning spec FR-008 default values. All tests from T006 must pass.
  - **Dependencies**: T006
  - **Est. Complexity**: Low

### Track C: Phase Module [P]

> Covers FR-003 (SDD Pipeline Execution). Referenced by US1 (pipeline phases), US3 (phase-level resume).

- [ ] T008 [P] [US1] Write tests for phase package in `internal/keeprun/phase_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/phase_test.go`
  - **Description**: Write tests for `PipelinePhases` covering: (1) returns exactly 12 phases, (2) phases are in correct order matching spec FR-003 (specify → clarify → generate-spec → review-spec → spec-to-plan → review-plan → plan-to-tasks → review-tasks → implement-tasks → review-code → commit-staged → pr), (3) review phases (4, 6, 8, 10) have `Review: true`, (4) only phase 12 has `Remote: true`, (5) each phase has correct `Command` string prefixed with `codexspec:`, (6) each phase has non-empty `Name`. Verify tests fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [ ] T009 [US1] Implement phase package in `internal/keeprun/phase.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/phase.go`
  - **Description**: Define `Phase` struct (Name, Command, Review, Remote fields). Implement `PipelinePhases() []Phase` returning the 12 SDD phases in exact spec FR-003 order with correct properties. All tests from T008 must pass.
  - **Dependencies**: T008
  - **Est. Complexity**: Low

### Track D: Backlog Module [P]

> Covers FR-001 (Backlog File Format), FR-002 (Task State Machine, State File). Referenced by US1, US2, US3.

- [ ] T010 [P] [US2] Write tests for `ParseBacklog` in `internal/keeprun/backlog_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/backlog_test.go`
  - **Description**: Write table-driven tests for `ParseBacklog(content string) ([]Task, error)` covering: (1) valid backlog with multiple tasks, (2) tasks with different types (feature, fix, refactor, docs, chore, test), (3) tasks with different statuses (pending, done), (4) tasks with different priorities, (5) multi-line descriptions, (6) empty file → empty slice, (7) single task, (8) `HeadingLine` is correctly tracked, (9) whitespace variations around fields. Verify tests fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Medium

- [ ] T011 [US2] Implement `ParseBacklog` in `internal/keeprun/backlog.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/backlog.go`
  - **Description**: Define `Task` struct matching plan's interface (Type, Title, Priority, Status, Description, HeadingLine). Implement `ParseBacklog` to parse BACKLOG.md markdown content per spec FR-001 format — tasks delimited by `## [type]` headings, with `**Priority**`, `**Status**`, `**Description**` fields. All tests from T010 must pass.
  - **Dependencies**: T010
  - **Est. Complexity**: Medium

- [ ] T012 [US1] Write tests for `UpdateStatus` in `internal/keeprun/backlog_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/backlog_test.go`
  - **Description**: Write tests for `UpdateStatus(content string, headingLine int, newStatus string) string` covering: (1) change pending → done, (2) task already done → unchanged, (3) invalid headingLine → unchanged, (4) update middle task in multi-task backlog, (5) return value preserves all other content. Verify tests fail.
  - **Dependencies**: T010
  - **Est. Complexity**: Low

- [ ] T013 [US1] Implement `UpdateStatus` in `internal/keeprun/backlog.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/backlog.go`
  - **Description**: Implement `UpdateStatus` to modify the `**Status**: pending` line for the task at the given heading line number. All tests from T012 must pass.
  - **Dependencies**: T012
  - **Est. Complexity**: Low

- [ ] T014 [P] [US3] Write tests for state file operations in `internal/keeprun/backlog_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/backlog_test.go`
  - **Description**: Write tests for `ReadState`, `WriteState`, and `State.NextPhase` covering: (1) write then read round-trip, (2) `ReadState` on nonexistent file → zero-value State, no error, (3) `ReadState` on invalid JSON → error, (4) `NextPhase` with empty `CompletedPhases` → returns 1, (5) `NextPhase` with phases [1,2,3] → returns 4, (6) `NextPhase` with non-contiguous phases [1,3,7] → returns 8, (7) `WriteState` creates parent directory if needed, (8) state file matches spec FR-002 JSON schema (field names, types). Use `t.TempDir()` for filesystem tests. Verify tests fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Medium

- [ ] T015 [US3] Implement state file operations in `internal/keeprun/backlog.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/backlog.go`
  - **Description**: Define `State` struct matching spec FR-002 schema (TaskSlug, WorktreePath, CompletedPhases, RemoteEnabled, LastPhaseAt). Implement `ReadState(worktreeDir string) (State, error)`, `WriteState(worktreeDir string, state State) error`, and `NextPhase() int` method. All tests from T014 must pass.
  - **Dependencies**: T014
  - **Est. Complexity**: Medium

**Checkpoint**: After Phase 2 — all pure logic modules are implemented and tested. Run `go test ./internal/keeprun/... -v` and verify all tests pass.

---

## Phase 3: Integration — Worktree Management

**Purpose**: Implement git worktree lifecycle operations. This module depends on the slug package for branch naming.

> Covers FR-005 (Git Worktree Isolation). Referenced by US1 (worktree creation), US3 (worktree reuse), US4 (worktree cleanup).

- [ ] T016 [US1] Write tests for worktree package in `internal/keeprun/worktree_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/worktree_test.go`
  - **Description**: Write tests for `Manager.Create`, `Manager.Remove`, and `Manager.ListBranches` covering: (1) `Create` produces worktree at `.claude/worktrees/<slug>` with branch `keep-run-<slug>`, (2) `Create` with valid slug succeeds, (3) `Create` with collision → error or deduplication, (4) `Remove` cleans up worktree directory, (5) `Remove` on already-removed worktree → error, (6) `ListBranches` returns branches matching `keep-run-*`, (7) `ListBranches` with no keep-run branches → empty slice, (8) `NewManager` with options. Use `t.TempDir()` with `git init` for isolated test repos. Verify tests fail.
  - **Dependencies**: T005 (slug package complete)
  - **Est. Complexity**: High

- [ ] T017 [US1] Implement worktree package in `internal/keeprun/worktree.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/worktree.go`
  - **Description**: Define `Manager` struct with `repoDir` and `timeout` fields. Implement `NewManager(repoDir string, opts ...ManagerOption) *Manager` with functional options. Implement `Create(ctx context.Context, slug string) (string, error)` using `git worktree add`. Implement `Remove(ctx context.Context, worktreeDir string) error` using `git worktree remove`. Implement `ListBranches(ctx context.Context) ([]string, error)` using `git branch --list`. Use `exec.Command` with argument arrays (not shell strings) for security. All tests from T016 must pass.
  - **Dependencies**: T016
  - **Est. Complexity**: High

**Checkpoint**: After Phase 3 — worktree management is implemented and tested. Run `go test ./internal/keeprun/... -v` and verify all tests pass.

---

## Phase 4: Interface — Prompt Command

**Purpose**: Create the `/keep-run` slash command prompt that drives the entire SDD pipeline.

> Covers FR-003 (SDD Pipeline Execution), FR-004 (Non-Interactive Mode), FR-006 (Remote Operations), FR-007 (Error Self-Healing), FR-010 (Merge Prohibition), FR-011 (Slash Command Registration), FR-012 (Progress Reporting). Referenced by all user stories.

- [ ] T018 [US1] Create `/keep-run` prompt command in `.claude/commands/codexspec/keep-run.md`
  - **Type**: Implementation
  - **Files**: `.claude/commands/codexspec/keep-run.md`
  - **Description**: Create the slash command prompt file with frontmatter (`description`, `argument-hint`, `user-invocable: true`, `disable-model-invocation: true`) and a detailed prompt body covering:
    - **Startup**: Read `keep-run.config.json` (use defaults if missing). Read `BACKLOG.md` from project root.
    - **Task loop**: Scan tasks in file order. Skip `done` tasks. Process first `pending` task. Re-read `BACKLOG.md` after each task completes to pick up new/changed tasks.
    - **Phase-level resume** (US3): Check for existing worktree with branch `keep-run-{task-slug}`. If exists, read `.keep-run-state.json`, compute `next_phase = max(completed_phases) + 1`, reuse worktree, skip completed phases. If no worktree, create one and start from phase 1.
    - **Worktree creation**: Use slug algorithm (FR-005). Create `.codexspec/specs/{slug}/` inside worktree for SDD artifacts.
    - **Sequential SDD phases**: Execute all 12 phases via `Skill("codexspec:<command>")` invocations in exact FR-003 order. After each phase completes, append phase number to `completed_phases` in state file.
    - **Non-interactive mode** (FR-004): Use `clarify_prompt` and `review_fix_prompt` from config to guide LLM decisions.
    - **Review phase iteration**: For phases 4, 6, 8, 10 — loop until all issues, warnings, and suggestions are resolved.
    - **Commit** (mandatory): Phase 11 via `/codexspec:commit-staged`.
    - **Conditional remote** (FR-006): If `remote_enabled`, push branch, create Issue, create PR via `/codexspec:pr`. If not, skip remote ops.
    - **Merge prohibition** (FR-010): Explicit instruction never to merge any branch into main.
    - **Error recovery** (FR-007): Cover all 7 failure scenarios with retry strategies: network failure → exponential backoff, context exceeded → compress + retry, disconnection → reduce request size, rate limit → wait for reset, quota exhausted → wait for refresh, test failure → fix code, review findings → fix all.
    - **Progress reporting** (FR-012): Log current task, current phase, phase transitions, task completion summary.
    - **Exit condition**: All tasks `done` → report "All tasks completed" and exit.
    - **BACKLOG.md update**: Change task status from `pending` to `done` after successful completion.
  - **Dependencies**: T003, T005, T007, T009, T011, T013, T015, T017 (all Go packages complete)
  - **Est. Complexity**: High

**Checkpoint**: After Phase 4 — `/keep-run` command is registered and invocable from TUI. Manual test with sample `BACKLOG.md`.

---

## Phase 5: Validation — Testing and Acceptance

**Purpose**: Verify all tests pass, code is formatted, and acceptance criteria from spec are validated.

- [ ] T019 [US1] Run full test suite and verify formatting
  - **Type**: Testing
  - **Files**: `internal/keeprun/` (all test files)
  - **Description**: Run `go test ./internal/keeprun/... -v` and verify all tests pass. Run `go test ./internal/keeprun/... -cover` and record coverage. Run `gofmt -l ./internal/keeprun/` and verify no formatting issues. Run `go vet ./internal/keeprun/...` and verify no issues.
  - **Dependencies**: T018
  - **Est. Complexity**: Low

- [ ] T020 [US1] Validate acceptance criteria against spec test cases
  - **Type**: Validation
  - **Files**: None (validation task)
  - **Description**: Validate the implementation against all spec acceptance test cases:
    - **TC-001**: Basic pipeline execution — single pending task goes through all 12 phases
    - **TC-002**: Multi-task sequential processing — three pending tasks processed in order
    - **TC-003**: Resume after interruption — skip done task, process pending task
    - **TC-003b**: Phase-level resume — existing state file with phases 1-6, resume from phase 7
    - **TC-004**: Worktree isolation — no cross-task artifact leakage
    - **TC-005**: No remote operations when `remote_enabled: false`
    - **TC-006**: Merge prohibition — never merges to main
    - **TC-007**: Error self-healing — network failure recovery
    - **TC-008**: Error self-healing — context window recovery
    - **TC-009**: Config defaults — no config file uses sensible defaults
    - **TC-010**: Exit condition — all done → exit immediately
    - **Edge cases**: Empty BACKLOG.md, missing BACKLOG.md, all done, invalid type prefix, missing config fields, worktree creation failure, remote push rejected, empty phase output, API quota exhaustion
  - **Dependencies**: T019
  - **Est. Complexity**: High

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Core TDD)**: Depends on T001. All four tracks (A-D) can run in parallel
- **Phase 3 (Integration)**: Depends on Track A (slug) completion
- **Phase 4 (Interface)**: Depends on all Phase 2 + Phase 3 completion
- **Phase 5 (Validation)**: Depends on Phase 4 completion

### Parallel Opportunities

- **Within Phase 2**: All four tracks (A, B, C, D) are fully independent
  - Track A tasks (T002-T005) are sequential within track
  - Track B tasks (T006-T007) are sequential within track
  - Track C tasks (T008-T009) are sequential within track
  - Track D tasks (T010-T015) have sequential dependencies within track
- **T014** (state file tests) can start in parallel with T010-T013 since it only depends on T001

### Task Dependency Graph

```
Phase 1:  T001
            │
            ├──────────────────────┬──────────────────────┬──────────────────────┐
            │                      │                      │                      │
Phase 2:  Track A (slug)      Track B (config)      Track C (phase)      Track D (backlog)
            │                      │                      │                      │
          T002 ──► T003         T006 ──► T007         T008 ──► T009         T010 ──► T011
            │                                                                 │
          T004 ──► T005                                                T012 ──► T013
            │                                                              │
            │                                                           T014 ──► T015
            │                                                              │
Phase 3:  T016 ──► T017 ◄──────────────────────────────────────────────────┘
            │            (depends on slug T005)
            │
Phase 4:  T018 ◄── (depends on all Phase 2 + Phase 3)
            │
Phase 5:  T019 ──► T020
```

---

## Execution Strategy

### Sequential (Single Agent)

1. Complete Phase 1: T001
2. Complete Phase 2 in order: Track A → Track B → Track C → Track D
3. Complete Phase 3: T016 → T017
4. Complete Phase 4: T018
5. Complete Phase 5: T019 → T020
6. **STOP and VALIDATE**: All acceptance criteria pass

### Parallel (Multi-Agent)

1. Complete T001
2. **Fan out** all Phase 2 tracks concurrently (Tracks A-D)
3. After Track A completes, start Phase 3 (T016-T017)
4. After all Phase 2 + Phase 3 complete, start Phase 4 (T018)
5. After Phase 4, run Phase 5 validation

---

## Checkpoints

- [ ] **Checkpoint 1**: After T001 — package structure created
- [ ] **Checkpoint 2**: After Phase 2 — all pure logic modules tested (`go test ./internal/keeprun/...`)
- [ ] **Checkpoint 3**: After Phase 3 — worktree management tested end-to-end
- [ ] **Checkpoint 4**: After Phase 4 — `/keep-run` command invocable from TUI
- [ ] **Checkpoint 5**: After Phase 5 — all acceptance criteria validated against spec

---

## Notes

- [P] tasks = different files, no dependencies between them
- TDD is mandatory per constitution: Red → Green → Refactor for every module
- Tests use `t.TempDir()` for filesystem operations (no external dependencies)
- Git operations use `exec.Command` with argument arrays for shell injection prevention
- No new external dependencies required — only Go stdlib and existing project deps
- The prompt command (T018) is the runtime bridge — Go packages serve as tested specifications
- Commit after each task completion following conventional commits format
