# Tasks: /keep-run — Autonomous SDD Pipeline Runner

**Input**: Design documents from `.codexspec/specs/2026-0531-23020o-keep-run-sdd-pipeline/`
**Prerequisites**: `plan.md` (required), `spec.md` (required for user stories and acceptance criteria)

**Tests**: This project mandates TDD per constitution. All code tasks follow Red-Green-Refactor.

**Organization**: Tasks are grouped by phase (setup → core TDD → integration → interface → validation). User story mappings are indicated per task.

---

## ⚠️ Design Revision (2026-06-02): Hybrid Architecture

The architecture changed from a **prompt-command driver** to a **Hybrid Go orchestrator** (see plan Decision 1). Impact on tasks:

- **T001–T017 (Go pure-logic + worktree)**: still valid and **reused as runtime control** — no longer "testable specifications." `worktree.Create` needs a small revision (explicit `baseRef`) — see **T021**.
- **T018 (keep-run.md prompt driver)**: **SUPERSEDED**. The `.md` driver is removed (**T028**); control moves into the Go orchestrator (**T021–T029**).
- **T019–T020 (validation)**: **re-opened** — must validate the orchestrator (TC-011/012/013, orchestrator unit tests with a fake runner).
- **New Phase 5** adds the orchestrator, PhaseRunner seam, verifier, retry policy, the real engine adapter, and the built-in registration.

Only the twelve `/codexspec:*` SDD commands remain LLM-driven; everything else is deterministic Go.

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

- [x] T001 [US1] Create `internal/keeprun/` package with `doc.go`
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

- [x] T002 [P] [US1] Write tests for `GenerateSlug` in `internal/keeprun/slug_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/slug_test.go`
  - **Description**: Write table-driven tests for `GenerateSlug` covering all cases from spec FR-005: (1) standard title `[feature] Add dark mode support` → `add-dark-mode-support`, (2) special characters `[fix] Fix timeout on slow connections!!!` → `fix-timeout-on-slow-connections`, (3) type prefix stripping, (4) lowercase conversion, (5) hyphen collapse, (6) leading/trailing hyphen strip, (7) truncation at 60 chars with hyphen boundary, (8) unicode input, (9) empty string after stripping, (10) title with only special characters. Verify all tests fail (Red phase).
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [x] T003 [US1] Implement `GenerateSlug` in `internal/keeprun/slug.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/slug.go`
  - **Description**: Implement `GenerateSlug(title string) string` following the exact 7-step algorithm from spec FR-005. Include block comment documenting the algorithm steps. All tests from T002 must pass (Green phase).
  - **Dependencies**: T002
  - **Est. Complexity**: Low

- [x] T004 [P] [US1] Write tests for `DeduplicateSlug` in `internal/keeprun/slug_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/slug_test.go`
  - **Description**: Write table-driven tests for `DeduplicateSlug(slug string, existing []string) string` covering: (1) no collision → returns slug unchanged, (2) single collision → appends `-2`, (3) multiple collisions → increments (`-2`, `-3`, etc.), (4) slug already has numeric suffix and collides → continues incrementing, (5) empty existing list. Verify tests fail.
  - **Dependencies**: T002
  - **Est. Complexity**: Low

- [x] T005 [US1] Implement `DeduplicateSlug` in `internal/keeprun/slug.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/slug.go`
  - **Description**: Implement `DeduplicateSlug` — step 8 of the slug algorithm. On collision with an existing branch name, append `-2`, `-3`, etc. All tests from T004 must pass.
  - **Dependencies**: T004
  - **Est. Complexity**: Low

### Track B: Config Module [P]

> Covers FR-008 (Configuration File), FR-009 (Config defaults). Referenced by US1, US4, US5.

- [x] T006 [P] [US1] Write tests for config package in `internal/keeprun/config_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/config_test.go`
  - **Description**: Write table-driven tests for `LoadConfig` and `DefaultConfig` covering: (1) valid complete config file, (2) missing config file → returns defaults, (3) partial config (some fields missing) → defaults for missing fields, (4) empty JSON object `{}` → all defaults, (5) invalid JSON → error, (6) `DefaultConfig()` returns expected defaults matching spec FR-008 table: `remote_enabled: true`, `review_mode: "subagent"`, default prompts, exponential backoff. Verify tests fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [x] T007 [US1] Implement config package in `internal/keeprun/config.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/config.go`
  - **Description**: Define `Config` and `RetryPolicy` structs matching plan's interface. Implement `LoadConfig(dir string) (Config, error)` — read `keep-run.config.json` from dir, apply defaults for missing fields. Implement `DefaultConfig() Config` returning spec FR-008 default values. All tests from T006 must pass.
  - **Dependencies**: T006
  - **Est. Complexity**: Low

### Track C: Phase Module [P]

> Covers FR-003 (SDD Pipeline Execution). Referenced by US1 (pipeline phases), US3 (phase-level resume).

- [x] T008 [P] [US1] Write tests for phase package in `internal/keeprun/phase_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/phase_test.go`
  - **Description**: Write tests for `PipelinePhases` covering: (1) returns exactly 12 phases, (2) phases are in correct order matching spec FR-003 (specify → clarify → generate-spec → review-spec → spec-to-plan → review-plan → plan-to-tasks → review-tasks → implement-tasks → review-code → commit-staged → pr), (3) review phases (4, 6, 8, 10) have `Review: true`, (4) only phase 12 has `Remote: true`, (5) each phase has correct `Command` string prefixed with `codexspec:`, (6) each phase has non-empty `Name`. Verify tests fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [x] T009 [US1] Implement phase package in `internal/keeprun/phase.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/phase.go`
  - **Description**: Define `Phase` struct (Name, Command, Review, Remote fields). Implement `PipelinePhases() []Phase` returning the 12 SDD phases in exact spec FR-003 order with correct properties. All tests from T008 must pass.
  - **Dependencies**: T008
  - **Est. Complexity**: Low

### Track D: Backlog Module [P]

> Covers FR-001 (Backlog File Format), FR-002 (Task State Machine, State File). Referenced by US1, US2, US3.

- [x] T010 [P] [US2] Write tests for `ParseBacklog` in `internal/keeprun/backlog_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/backlog_test.go`
  - **Description**: Write table-driven tests for `ParseBacklog(content string) ([]Task, error)` covering: (1) valid backlog with multiple tasks, (2) tasks with different types (feature, fix, refactor, docs, chore, test), (3) tasks with different statuses (pending, done), (4) tasks with different priorities, (5) multi-line descriptions, (6) empty file → empty slice, (7) single task, (8) `HeadingLine` is correctly tracked, (9) whitespace variations around fields. Verify tests fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Medium

- [x] T011 [US2] Implement `ParseBacklog` in `internal/keeprun/backlog.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/backlog.go`
  - **Description**: Define `Task` struct matching plan's interface (Type, Title, Priority, Status, Description, HeadingLine). Implement `ParseBacklog` to parse BACKLOG.md markdown content per spec FR-001 format — tasks delimited by `## [type]` headings, with `**Priority**`, `**Status**`, `**Description**` fields. All tests from T010 must pass.
  - **Dependencies**: T010
  - **Est. Complexity**: Medium

- [x] T012 [US1] Write tests for `UpdateStatus` in `internal/keeprun/backlog_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/backlog_test.go`
  - **Description**: Write tests for `UpdateStatus(content string, headingLine int, newStatus string) string` covering: (1) change pending → done, (2) task already done → unchanged, (3) invalid headingLine → unchanged, (4) update middle task in multi-task backlog, (5) return value preserves all other content. Verify tests fail.
  - **Dependencies**: T010
  - **Est. Complexity**: Low

- [x] T013 [US1] Implement `UpdateStatus` in `internal/keeprun/backlog.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/backlog.go`
  - **Description**: Implement `UpdateStatus` to modify the `**Status**: pending` line for the task at the given heading line number. All tests from T012 must pass.
  - **Dependencies**: T012
  - **Est. Complexity**: Low

- [x] T014 [P] [US3] Write tests for state file operations in `internal/keeprun/backlog_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/backlog_test.go`
  - **Description**: Write tests for `ReadState`, `WriteState`, and `State.NextPhase` covering: (1) write then read round-trip, (2) `ReadState` on nonexistent file → zero-value State, no error, (3) `ReadState` on invalid JSON → error, (4) `NextPhase` with empty `CompletedPhases` → returns 1, (5) `NextPhase` with phases [1,2,3] → returns 4, (6) `NextPhase` with non-contiguous phases [1,3,7] → returns 8, (7) `WriteState` creates parent directory if needed, (8) state file matches spec FR-002 JSON schema (field names, types). Use `t.TempDir()` for filesystem tests. Verify tests fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Medium

- [x] T015 [US3] Implement state file operations in `internal/keeprun/backlog.go`
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

- [x] T016 [US1] Write tests for worktree package in `internal/keeprun/worktree_test.go`
  - **Type**: Testing
  - **Files**: `internal/keeprun/worktree_test.go`
  - **Description**: Write tests for `Manager.Create`, `Manager.Remove`, and `Manager.ListBranches` covering: (1) `Create` produces worktree at `.claude/worktrees/<slug>` with branch `keep-run-<slug>`, (2) `Create` with valid slug succeeds, (3) `Create` with collision → error or deduplication, (4) `Remove` cleans up worktree directory, (5) `Remove` on already-removed worktree → error, (6) `ListBranches` returns branches matching `keep-run-*`, (7) `ListBranches` with no keep-run branches → empty slice, (8) `NewManager` with options. Use `t.TempDir()` with `git init` for isolated test repos. Verify tests fail.
  - **Dependencies**: T005 (slug package complete)
  - **Est. Complexity**: High

- [x] T017 [US1] Implement worktree package in `internal/keeprun/worktree.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/worktree.go`
  - **Description**: Define `Manager` struct with `repoDir` and `timeout` fields. Implement `NewManager(repoDir string, opts ...ManagerOption) *Manager` with functional options. Implement `Create(ctx context.Context, slug string) (string, error)` using `git worktree add`. Implement `Remove(ctx context.Context, worktreeDir string) error` using `git worktree remove`. Implement `ListBranches(ctx context.Context) ([]string, error)` using `git branch --list`. Use `exec.Command` with argument arrays (not shell strings) for security. All tests from T016 must pass.
  - **Dependencies**: T016
  - **Est. Complexity**: High

**Checkpoint**: After Phase 3 — worktree management is implemented and tested. Run `go test ./internal/keeprun/... -v` and verify all tests pass.

---

## Phase 4: Interface — Prompt Command  *(SUPERSEDED 2026-06-02 — replaced by Phase 5: Hybrid Orchestrator)*

**Purpose**: Create the `/keep-run` slash command prompt that drives the entire SDD pipeline.

> Covers FR-003 (SDD Pipeline Execution), FR-004 (Non-Interactive Mode), FR-006 (Remote Operations), FR-007 (Error Self-Healing), FR-010 (Merge Prohibition), FR-011 (Slash Command Registration), FR-012 (Progress Reporting). Referenced by all user stories.

- [~] T018 [US1] ~~Create `/keep-run` prompt command in `.claude/commands/codexspec/keep-run.md`~~ — **SUPERSEDED by Hybrid architecture**
  - **Status**: Superseded 2026-06-02. The prompt driver is replaced by the Go orchestrator (T022–T027) and removed (T028). Retained for traceability.
  - **Type**: Implementation (obsolete)
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

## Phase 5: Hybrid Orchestrator (Design Revision 2026-06-02)

**Purpose**: Replace the superseded prompt driver with the deterministic Go control plane (plan Decisions 1–8). Only `/codexspec:*` SDD commands remain LLM-driven.

> Covers FR-003, FR-004, FR-006, FR-007, FR-010, FR-011, FR-012, FR-013, NFR-006. Referenced by all user stories.

- [x] T021 [P] [US1] Revise `worktree.Create` to take an explicit `baseRef`; add `DefaultBranch`
  - **Type**: Implementation (TDD)
  - **Files**: `internal/keeprun/worktree.go`, `internal/keeprun/worktree_test.go`
  - **Description**: Change `Create(ctx, slug)` → `Create(ctx, slug, baseRef string)` so worktrees branch from an explicit base (the repo default branch) rather than the caller's HEAD. Add `DefaultBranch(ctx) (string, error)` (resolve via `git symbolic-ref refs/remotes/origin/HEAD`, fall back to `main`/current HEAD). Update existing tests; add a test that Create from a non-default checkout still branches from `baseRef`.
  - **Dependencies**: T017
  - **Est. Complexity**: Medium

- [x] T022 [P] [US1] Define the `PhaseRunner` seam in `internal/keeprun/runner.go`
  - **Type**: Implementation
  - **Files**: `internal/keeprun/runner.go`
  - **Description**: Define the `PhaseRunner` interface (`RunPhase(ctx, PhaseRequest) (PhaseOutcome, error)`) plus `PhaseRequest` (Phase, WorktreeDir, SpecDir, Config, Instruction, AllowedTools) and `PhaseOutcome` (Output). No `internal/engine` / `internal/tui` import — this is the only seam to the LLM (NFR-006).
  - **Dependencies**: T009, T007
  - **Est. Complexity**: Low

- [x] T023a [P] [US1] Write verifier tests in `internal/keeprun/verify_test.go`
  - **Type**: Testing (Red)
  - **Files**: `internal/keeprun/verify_test.go`
  - **Description**: Table-driven tests (first, failing) for `VerifyPhase(ctx, phase, TaskContext)` per the plan gate table (artifact exists/non-empty; tests pass; HEAD advanced; working tree clean); for the **review-code** objective gates (`go build` / `vet` / `test` / `gofmt -l`); for the **pr** gate (branch pushed, Issue created, PR references it `Closes #N`); and for `ReviewClean(PhaseOutcome)` (parse the injected verdict block `<!-- keep-run-verdict: {"status":...} -->`; missing/malformed ⇒ not clean, fail-safe). Use `t.TempDir()` + a throwaway git repo. Verify tests fail. (TC-011)
  - **Dependencies**: T022
  - **Est. Complexity**: High

- [x] T023b [US1] Implement the verifier in `internal/keeprun/verify.go`
  - **Type**: Implementation (Green)
  - **Files**: `internal/keeprun/verify.go`
  - **Description**: Implement `VerifyPhase` and `ReviewClean` to pass all T023a tests. Define `TaskContext` here (or share with the orchestrator) — fields: slug, worktreeDir, specDir, baseRef, headCommitBefore, config.
  - **Dependencies**: T023a
  - **Est. Complexity**: Medium

- [x] T024a [US1] Write orchestrator tests in `internal/keeprun/orchestrator_test.go`
  - **Type**: Testing (Red)
  - **Files**: `internal/keeprun/orchestrator_test.go`
  - **Description**: Tests (first, failing) that drive the full loop against a **fake** `PhaseRunner` (no real LLM — proves NFR-006): parse backlog → first pending → slug+dedup → worktree create/resume → `next_phase` from state → ordered phase loop with `VerifyPhase` gate → `WriteState` after each phase → `UpdateStatus` done → `Remove` worktree → re-read backlog → exit when none pending. When `remote_enabled`, the remote phase (12) performs push → create Issue (LLM body, capture #N) → create PR via `/codexspec:pr` (`Closes #N`); when disabled it is skipped. Assert exact phase order, resume from phase 7 given state 1–6 (TC-012), and that a phase whose artifact is missing is not recorded and is retried (TC-011). Verify tests fail.
  - **Dependencies**: T021, T022, T023b, T011, T013, T015, T005
  - **Est. Complexity**: High

- [x] T024b [US1] Implement the orchestrator core in `internal/keeprun/orchestrator.go`
  - **Type**: Implementation (Green)
  - **Files**: `internal/keeprun/orchestrator.go`
  - **Description**: Implement `Orchestrator`, `NewOrchestrator`, `Run`, the `ProgressSink`/`ProgressEvent` types, and the `BackoffPolicy` to pass all T024a tests.
  - **Dependencies**: T024a
  - **Est. Complexity**: High

- [x] T025 [US1] TDD review-iteration + retry/backoff policy
  - **Type**: Implementation (TDD)
  - **Files**: `internal/keeprun/orchestrator.go`, `internal/keeprun/orchestrator_test.go`
  - **Description**: Review phases inject the verdict-block instruction (a single Go constant, see T029) via `PhaseRequest.Instruction` — no `/codexspec:*` files are edited — then loop `run → if !ReviewClean: run fix (review_fix_prompt) → re-run` until clean. Retry/backoff is rate-limited with NO cap (FR-007); a fake runner that fails N times then succeeds must be retried until the gate passes, with backoff observed (inject a fake clock). Assert no artificial cap exists.
  - **Dependencies**: T024b
  - **Est. Complexity**: Medium

- [x] T026 [P] [US1] Implement the real `PhaseRunner` adapter in `internal/tui/keeprun_runner.go`
  - **Type**: Implementation
  - **Files**: `internal/tui/keeprun_runner.go`, `internal/tui/keeprun_runner_test.go`
  - **Description**: Implement `RunPhase` by resolving `/codexspec:<command>` via the slash `Executor` (argument/Instruction substitution) and calling `runner.RunRestricted(ctx, body, allowedTools, reporter)` with merge-capable tools excluded (FR-010, TC-013); map `*engine.RunResult` → `PhaseOutcome`. Run with `WorktreeDir` as cwd. For **review** phases, dispatch on `Config.ReviewMode` (plan Decision 9): `direct` = inline `RunRestricted`; `subagent` = isolated fork-mode execution of the review command (via the `Executor` fork path) whose report is returned for the orchestrator's fix step. Test the allowed-tools exclusion, body resolution, and both review-mode dispatch paths with a fake runner/executor.
  - **Dependencies**: T022
  - **Est. Complexity**: High

- [x] T027 [US1] Register `/keep-run` as a built-in in `internal/tui/keeprun_builtin.go`
  - **Status (2026-06-03)**: Wired end-to-end and building (full `tui`+`app`+`keeprun` suites pass; `vet`+`gofmt` clean). `/keep-run` dispatches in `model.go` `handleSlashCommand` (before the registry `default:`) → `startKeepRunCmd`; the orchestrator runs on a goroutine streaming `keepRunProgressMsg`/`keepRunDoneMsg` through the events pump (mirrors `runEventMsg` re-arm); Ctrl+C cancels via `runCtx`. The merge guard is installed per-run via `AgentRunner.AddMiddleware` + `buildRegistry` (additive; FR-010). **Needs one live smoke test** (real LLM + TUI) to validate streaming/cancel UX — not unit-verifiable here. Minor follow-up: add `/keep-run` to the autocomplete builtin list (works without it).
  - **Type**: Implementation
  - **Files**: `internal/tui/keeprun_builtin.go`, `internal/tui/keeprun_builtin_test.go`
  - **Description**: Register `/keep-run` as `CommandBuiltin` (name exactly `keep-run`, fixing the earlier `/codexspec:keep-run` naming gap — FR-011). On invoke: construct the real `PhaseRunner`, build the `Orchestrator`, launch it on a goroutine, bridge `ProgressSink` events to the TUI events channel (FR-012), and wire Ctrl+C to cancel the run context. Test registration + that invocation starts the orchestrator (with a fake).
  - **Dependencies**: T024b, T026
  - **Est. Complexity**: High

- [x] T028 [US1] Remove the superseded prompt driver
  - **Status (2026-06-03)**: Done — `.claude/commands/codexspec/keep-run.md` deleted; keep-run is Go-driven only.
  - **Type**: Cleanup
  - **Files**: `.claude/commands/codexspec/keep-run.md` (delete)
  - **Description**: Delete `.claude/commands/codexspec/keep-run.md`; the Go orchestrator is the runtime driver. Confirm no references remain.
  - **Dependencies**: T027
  - **Est. Complexity**: Low

- [x] T029 [P] [US1] Define the injected verdict-block instruction (no review-command edits)
  - **Type**: Implementation
  - **Files**: `internal/keeprun/runner.go` (or a small `review.go` constant)
  - **Description**: Per plan Decision 8 (Option 2): define, as a single Go constant, the instruction the orchestrator injects into every review phase via `PhaseRequest.Instruction`, requiring the run to end with a machine-readable verdict block `<!-- keep-run-verdict: {"status":"pass|needs_work|fail","critical":N,"high":N} -->`. This is the only place the verdict contract is defined. **The four `/codexspec:review-*` command files are NOT modified.** (`ReviewClean` parsing and the review-code objective gates live in T023.)
  - **Dependencies**: T022
  - **Est. Complexity**: Low

**Checkpoint**: After Phase 5 — `/keep-run` is a built-in Go orchestrator; `go test ./internal/keeprun/... ./internal/tui/... -run 'KeepRun|Orchestrator'` passes; merge tools are provably withheld.

---

## Phase 6: Validation — Testing and Acceptance

**Purpose**: Verify all tests pass, code is formatted, and acceptance criteria from spec are validated.

- [ ] T019 [US1] Run full test suite and verify formatting *(re-opened 2026-06-02 — now covers the orchestrator + TUI adapter)*
  - **Type**: Testing
  - **Files**: `internal/keeprun/`, `internal/tui/keeprun_*` (all test files)
  - **Description**: Run `go test ./internal/keeprun/... -v` and `go test ./internal/tui/... -run 'KeepRun|Orchestrator'` and verify all pass. Confirm the orchestrator suite runs with a **fake** `PhaseRunner` (no real LLM) — proves NFR-006. Run `go test ./internal/keeprun/... -cover` and record coverage. Run `gofmt -l ./internal/keeprun/` and `go vet ./internal/keeprun/...` and verify no issues.
  - **Dependencies**: T027, T028, T029 (Phase 5 complete)
  - **Est. Complexity**: Low

- [ ] T020 [US1] Validate acceptance criteria against spec test cases *(re-opened 2026-06-02 — add TC-011/012/013)*
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
    - **TC-011**: Phase-completion verification gate — artifact missing → phase not recorded, retried
    - **TC-012**: Deterministic ordering + resume via a mocked runner — phases 7-12 only, in exact order, no real LLM
    - **TC-013**: Merge tools withheld — phase runs never receive merge-capable operations
    - **Edge cases**: Empty BACKLOG.md, missing BACKLOG.md, all done, invalid type prefix, missing config fields, worktree creation failure, remote push rejected, empty phase output, API quota exhaustion
  - **Dependencies**: T019
  - **Est. Complexity**: High

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Core TDD)**: Depends on T001. All four tracks (A-D) can run in parallel
- **Phase 3 (Integration)**: Depends on Track A (slug) completion
- **Phase 4 (Interface — Prompt Command)**: ~~Depends on all Phase 2 + Phase 3 completion~~ — SUPERSEDED by Phase 5
- **Phase 5 (Hybrid Orchestrator, 2026-06-02)**: Replaces Phase 4. Depends on T017 (worktree) + T005/T007/T009/T011/T013/T015 (pure modules). Within it: T021 ∥ T022; after T022, T023a ∥ T026 ∥ T029; T024a needs T023b; chain T024a→T024b→T025; T027 needs T024b+T026
- **Phase 6 (Validation)**: Depends on **Phase 5** completion (T027/T028/T029)

### Parallel Opportunities

- **Within Phase 2**: All four tracks (A, B, C, D) are fully independent
  - Track A tasks (T002-T005) are sequential within track
  - Track B tasks (T006-T007) are sequential within track
  - Track C tasks (T008-T009) are sequential within track
  - Track D tasks (T010-T015) have sequential dependencies within track
- **T014** (state file tests) can start in parallel with T010-T013 since it only depends on T001
- **Within Phase 5**: `[P]` tasks are T021 ∥ T022, then (after T022) T023a ∥ T026 ∥ T029. The orchestrator chain (T024a→T024b→T025) and T027→T028 are sequential.

### Task Dependency Graph

```
Phase 1:  T001
            │
Phase 2:  Track A: T002►T003, T004►T005     Track B: T006►T007
          Track C: T008►T009                Track D: T010►T011, T012►T013, T014►T015
            │
Phase 3:  T016 ──► T017                      (T016 depends on slug T005)
            │
Phase 4:  T018  ✗ SUPERSEDED — removed by T028
            │
Phase 5 (Hybrid Orchestrator):
          T021 [P] (needs T017) ─────────────────────────┐
          T022 [P] (needs T007,T009) ──┬──────────────────┤
                                       ├─ T023a [P] ──► T023b
                                       ├─ T026 [P]
                                       └─ T029 [P]
          T024a (needs T021,T022,T023b,T005,T011,T013,T015) ──► T024b ──► T025
                                                          └─► T027 (needs T024b,T026) ──► T028
            │
Phase 6:  T019 (needs T027,T028,T029) ──► T020
```

---

## Execution Strategy

### Sequential (Single Agent)

1. Complete Phase 1: T001
2. Complete Phase 2 in order: Track A → Track B → Track C → Track D
3. Complete Phase 3: T016 → T017
4. ~~Complete Phase 4: T018~~ (superseded)
5. Complete Phase 5 (revision): T021 → T022 → T023a → T023b → T024a → T024b → T025 → T026 → T027 → T028; T029 any time after T022
6. Complete Phase 6: T019 → T020
7. **STOP and VALIDATE**: All acceptance criteria pass (incl. TC-011/012/013)

### Parallel (Multi-Agent)

1. Complete T001
2. **Fan out** all Phase 2 tracks concurrently (Tracks A-D)
3. After Track A completes, start Phase 3 (T016-T017)
4. After Phase 2 + Phase 3, start Phase 5: run T021 ∥ T022; then fan out T023a ∥ T026 ∥ T029; converge on T024a→T024b→T025 and T027→T028
5. After Phase 5, run Phase 6 validation (T019 → T020)

---

## Checkpoints

- [x] **Checkpoint 1**: After T001 — package structure created
- [x] **Checkpoint 2**: After Phase 2 — all pure logic modules tested (`go test ./internal/keeprun/...`)
- [x] **Checkpoint 3**: After Phase 3 — worktree management tested end-to-end
- [~] **Checkpoint 4**: ~~After Phase 4 — `/keep-run` command invocable from TUI~~ — SUPERSEDED by Checkpoint 5
- [ ] **Checkpoint 5**: After Phase 5 — `/keep-run` is a built-in Go orchestrator; orchestrator + TUI tests pass with a fake runner; merge tools withheld
- [ ] **Checkpoint 6**: After Phase 6 — all acceptance criteria validated against spec (incl. TC-011/012/013)

---

## Notes

- [P] tasks = different files, no dependencies between them
- TDD is mandatory per constitution: Red → Green → Refactor for every module
- Tests use `t.TempDir()` for filesystem operations (no external dependencies)
- Git operations use `exec.Command` with argument arrays for shell injection prevention
- No new external dependencies required — only Go stdlib and existing project deps
- The Go orchestrator (Phase 5) is the runtime driver; the `internal/keeprun` packages are load-bearing runtime code. Only `/codexspec:*` SDD commands remain LLM-driven (2026-06-02 revision)
