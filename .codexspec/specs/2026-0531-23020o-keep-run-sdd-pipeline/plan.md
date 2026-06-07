# Implementation Plan: /keep-run — Autonomous SDD Pipeline Runner

**Related Spec**: `.codexspec/specs/2026-0531-23020o-keep-run-sdd-pipeline/spec.md`
**Created**: 2026-06-01
**Revised**: 2026-06-02 — Hybrid architecture: Go orchestrator owns control; only `/codexspec:*` SDD commands remain LLM-driven
**Status**: Draft

## Context

foxharness-go is an AI agent harness with a mature slash command infrastructure. Users invoke `/codexspec:*` commands sequentially to drive the Spec-Driven Development (SDD) pipeline. Currently this is a manual process — the user must invoke each command, review output, and trigger the next phase.

`/keep-run` automates this by reading a `BACKLOG.md` file and processing each `pending` task through all 12 SDD phases in isolated git worktrees. It runs autonomously, self-heals errors, and produces one commit (and optionally one Issue + PR) per task without ever merging to main.

**Architecture: Hybrid (Go orchestration + reused LLM-driven SDD commands).** All pipeline *control* is deterministic Go: a built-in `/keep-run` command starts a Go orchestrator that parses the backlog, manages worktrees, sequences phases, persists resume state, runs the retry/backoff loop, blocks merge commands via a bash guard, updates `BACKLOG.md`, and decides when to exit. The *only* work delegated to the LLM is the execution of the twelve reused `/codexspec:*` SDD commands — the orchestrator drives the engine to run each phase, then deterministically verifies the phase's artifact before advancing. This is the central design decision (see Decision 1) and the reason the `internal/keeprun/` packages are load-bearing runtime code rather than advisory specifications.

The implementation builds on four existing systems:
- **Engine loop** (`internal/engine/`) — `AgentEngine.Run(ctx, sess, prompt)` drives a full multi-turn LLM run; the orchestrator calls it once per phase
- **Slash command infrastructure** (`internal/slash/`) — command discovery and the `Executor` that resolves a `/codexspec:*` command to its prompt body; built-in command registration (`CommandBuiltin` + `HandlerFunc`)
- **TUI run lifecycle** (`internal/tui/`) — async run plumbing (events channel, cancelation, progress reporting) that the orchestrator goroutine reuses to stream progress
- **Error recovery & compaction** (`internal/recovery/`, `internal/compaction/`) — within-phase failure recovery and context management

## Goals / Non-Goals

**Goals:**

- Provide a single `/keep-run` slash command that drives the full SDD pipeline autonomously
- Parse `BACKLOG.md` and process tasks sequentially in isolated git worktrees
- Self-heal all errors via existing recovery infrastructure and retry strategies
- Produce one commit per task (and one Issue + PR when remote is enabled)
- Support both remote and local-only workflows

**Non-Goals:**

- Parallel task processing (spec explicitly requires sequential)
- Automatic backlog management (adding/removing tasks via agent)
- Integration with project management tools beyond GitHub/GitLab Issues
- Interactive mode during keep-run (all decisions are autonomous)

## Tech Stack

| Category | Technology | Version | Notes |
|----------|------------|---------|-------|
| Language | Go | ≥ 1.22 | Matches existing project |
| TUI Framework | bubbletea | v1.3.x | Existing dependency |
| YAML Parsing | gopkg.in/yaml.v3 | v3.0.1 | Existing dependency |
| JSON Parsing | encoding/json | stdlib | For keep-run.config.json |
| Git Operations | exec.Command | stdlib | Shell out to git CLI |
| LLM Engine | `internal/engine` | existing | `AgentEngine.Run` / `RunRestricted` drives each phase (via `PhaseRunner`) |
| Slash exec | `internal/slash` | existing | `Executor` resolves `/codexspec:*` bodies; built-in registration |
| No new external dependencies required | | | |

## Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| 1. TDD | ✅ | All Go packages (incl. orchestrator) developed test-first; the orchestrator is covered with a fake PhaseRunner (no real LLM). |
| 2. Code Quality | ✅ | Single-responsibility packages. Injectable dependencies. Clear interfaces. |
| 3. Go Documentation Standards | ✅ | Block comments on all exported identifiers. `doc.go` for package docs. No teaching comments. |
| 4. Testing Standards | ✅ | Table-driven unit tests. Edge case coverage. Error path testing. |
| 5. Architecture | ✅ | New `internal/keeprun/` package with focused responsibilities. Interfaces before implementations. |
| 6. Performance | ✅ | Pipeline is I/O bound (LLM calls, git operations). No hot paths. |
| 7. Security | ✅ | Input validation at boundaries (BACKLOG.md, config.json). No hardcoded secrets. Shell arguments sanitized. |

## Architecture Overview

```
User types /keep-run
        |
        v
TUI dispatch: /keep-run is a CommandBuiltin handler
        |  starts a background goroutine, streams progress via events channel
        v
+===================================================================+
|  Go Orchestrator  (internal/keeprun.Orchestrator) — DETERMINISTIC |
|                                                                   |
|  cfg := config.LoadConfig(root)                                   |
|  loop:                                                            |
|    tasks := backlog.ParseBacklog(read BACKLOG.md)   # re-read     |
|    task  := first pending task; if none -> EXIT                   |
|    slug  := slug.GenerateSlug(task.Title) (+ Dedup vs branches)   |
|    wt    := worktree.Manager.Create(slug)  OR reuse existing      |
|    st    := backlog.ReadState(wt);  p0 := st.NextPhase()          |
|    for p := p0; p <= 12; p++:                                     |
|        if phase p is Remote and !cfg.RemoteEnabled: break         |
|        +-----------------------------------------------------+    |
|        | retry/backoff loop (no cap, rate-limited):          |    |
|        |   out := PhaseRunner.RunPhase(phase[p], taskCtx) ---------> drives ONE
|        |   if phase is Review: loop run+fix until clean      |    |  engine.Run(
|        |   VerifyPhase(p, taskCtx)   # artifact gate         |    |   codexspec[p])
|        +-----------------------------------------------------+    |
|        backlog.WriteState(wt, append(completed, p))              |
|    backlog.UpdateStatus(BACKLOG.md, task -> done)                |
|    worktree.Manager.Remove(wt)   # branch preserved              |
+===================================================================+
        |                                   ^
        | RunPhase (the ONLY LLM seam)      | PhaseRunner interface (mocked in tests)
        v                                   |
+-------------------------------------------+----------------------+
|  Engine (internal/engine) — LLM-DRIVEN, one run per phase        |
|  AgentEngine.Run(ctx, sess, codexspec command body)             |
|  - bash guard denies git merge / gh pr merge commands (FR-010)   |
|  - phases are consecutive runs in ONE session; compaction (existing) |
|    bounds context; error-recovery handles transient failures     |
|  - artifacts (spec.md/plan.md/tasks.md) persist on disk =        |
|    the durable handoff; resume reads them regardless of context  |
+------------------------------------------------------------------+
```

## Component Structure

```
internal/keeprun/                # Load-bearing runtime control logic (TDD)
├── doc.go                       # Package documentation
├── backlog.go                   # BACKLOG.md parser, status updates, state file
├── backlog_test.go
├── config.go                    # keep-run.config.json loader with defaults
├── config_test.go
├── slug.go                      # Task slug generation + dedup
├── slug_test.go
├── worktree.go                  # Git worktree lifecycle (base branch explicit)
├── worktree_test.go
├── phase.go                     # SDD phase definitions and ordering
├── phase_test.go
├── orchestrator.go              # NEW: the deterministic control loop
├── orchestrator_test.go         # NEW: TDD with a mocked PhaseRunner (no real LLM)
├── runner.go                    # NEW: PhaseRunner interface + PhaseRequest/Outcome
├── verify.go                    # NEW: deterministic per-phase artifact gates
└── verify_test.go               # NEW

internal/tui/
├── keeprun_builtin.go           # NEW: registers /keep-run (CommandBuiltin), starts
│                                #      the orchestrator goroutine, streams progress
└── keeprun_runner.go            # NEW: real PhaseRunner — adapts engine.Run /
                                 #      RunRestricted + Executor to RunPhase

# Removed: .claude/commands/codexspec/keep-run.md
#   The prompt-driver is superseded by the Go orchestrator. The reused
#   /codexspec:* SDD command files are unchanged and remain the LLM-driven phases.

# Files managed at runtime (not in source control):
BACKLOG.md                      # User-created task backlog
keep-run.config.json            # Optional configuration overrides
```

## Module Dependency Graph

```
internal/tui/keeprun_builtin.go        internal/tui/keeprun_runner.go
   (registers /keep-run builtin,           (real PhaseRunner: wraps
    starts orchestrator goroutine)          engine.Run / RunRestricted + Executor)
            |                                          |
            |  constructs + injects                    | implements
            v                                          v
   +-----------------------------------------------------------------+
   |                       internal/keeprun/                         |
   |                                                                 |
   |   orchestrator  --depends on-->  PhaseRunner (interface)        |
   |       |   \                                                     |
   |       |    \--> verify (artifact gates)                        |
   |       |                                                         |
   |       +--> backlog   config   slug   phase   worktree          |
   |                                        |        |               |
   |                                     (slug)   os/exec (git)      |
   +-----------------------------------------------------------------+
            |
            v
   internal/engine (AgentEngine.Run)  — the LLM, only reached via PhaseRunner
```

Key dependency rules (revised):
- `orchestrator` depends on the `PhaseRunner` interface (not on `internal/engine` directly), so it is unit-testable with a fake runner and no real LLM (NFR-006).
- The real `PhaseRunner` lives in `internal/tui` (it needs the engine + Executor); `internal/keeprun` itself imports neither `internal/engine` nor `internal/tui`, keeping the control core dependency-light and testable.
- `backlog`, `config`, `slug`, `phase` remain pure logic; `worktree` depends on `slug` + `os/exec`; `verify` depends on `os/exec` (git/test checks) and the filesystem.

## Module Specifications

### Module: `internal/keeprun/backlog`
- **Responsibility**: Parse BACKLOG.md into structured task entries; update task status in markdown content; manage state file for phase-level resume
- **Dependencies**: None (pure string/JSON parsing)
- **Interface**:
  ```go
  // Task represents a single entry in BACKLOG.md.
  type Task struct {
      Type        string // feature, fix, refactor, docs, chore, test
      Title       string
      Priority    string // high, medium, low
      Status      string // pending, done
      Description string
      HeadingLine int    // 1-indexed line of the ## [type] heading
  }

  // ParseBacklog parses BACKLOG.md content into ordered task entries.
  func ParseBacklog(content string) ([]Task, error)

  // UpdateStatus returns the content with the task at headingLine
  // having its Status field changed to newStatus.
  func UpdateStatus(content string, headingLine int, newStatus string) string

  // State represents the pipeline progress for a single task.
  // Schema matches spec FR-002.
  type State struct {
      TaskSlug        string `json:"task_slug"`
      WorktreePath    string `json:"worktree_path"`
      CompletedPhases []int  `json:"completed_phases"`
      RemoteEnabled   bool   `json:"remote_enabled"`
      LastPhaseAt     string `json:"last_phase_at"`
  }

  // ReadState reads and parses the state file from the worktree.
  // Returns zero-value State if file does not exist.
  func ReadState(worktreeDir string) (State, error)

  // WriteState writes the state file to the worktree.
  func WriteState(worktreeDir string, state State) error

  // NextPhase returns the 1-indexed phase number to resume from.
  // Returns 1 if no phases completed, max(completed)+1 otherwise.
  func (s State) NextPhase() int
  ```
- **Files**: `backlog.go`, `backlog_test.go`

### Module: `internal/keeprun/config`
- **Responsibility**: Load and validate `keep-run.config.json` with sensible defaults
- **Dependencies**: None (pure JSON parsing)
- **Interface**:
  ```go
  // Config holds the keep-run pipeline configuration.
  type Config struct {
      RemoteEnabled   bool       `json:"remote_enabled"`
      ReviewMode      string     `json:"review_mode"`
      ClarifyPrompt   string     `json:"clarify_prompt"`
      ReviewFixPrompt string     `json:"review_fix_prompt"`
      RetryPolicy     RetryPolicy `json:"retry_policy"`
  }

  // RetryPolicy controls backoff behavior for error recovery.
  type RetryPolicy struct {
      Backoff string `json:"backoff"`
  }

  // LoadConfig reads keep-run.config.json from dir, applying defaults
  // for missing fields. Returns default config if file does not exist.
  func LoadConfig(dir string) (Config, error)

  // DefaultConfig returns the configuration used when no config file exists.
  func DefaultConfig() Config
  ```
- **Files**: `config.go`, `config_test.go`

### Module: `internal/keeprun/slug`
- **Responsibility**: Generate deterministic, filesystem-safe slugs from task titles
- **Dependencies**: None (pure string algorithm)
- **Interface**:
  ```go
  // GenerateSlug converts a task heading like "[feature] Add dark mode"
  // into a filesystem-safe slug: "add-dark-mode".
  //
  // Algorithm (matches spec FR-005 step by step):
  //  1. Take the task title text
  //  2. Strip the [type] prefix if present
  //  3. Convert to lowercase
  //  4. Replace any character not in [a-z0-9] with a hyphen
  //  5. Collapse consecutive hyphens into a single hyphen
  //  6. Strip leading and trailing hyphens
  //  7. Truncate to a maximum of 60 characters, breaking at the
  //     last hyphen boundary if possible
  //  Note: step 8 (collision deduplication) is in DeduplicateSlug.
  func GenerateSlug(title string) string

  // DeduplicateSlug implements step 8 of the slug algorithm:
  // on collision with an existing branch, append -2, -3, etc.
  func DeduplicateSlug(slug string, existing []string) string
  ```
- **Files**: `slug.go`, `slug_test.go`

### Module: `internal/keeprun/worktree`
- **Responsibility**: Create and remove git worktrees for task isolation, rooted at an explicit base branch (revision: the current `worktree.go` roots at the caller's HEAD; it must take an explicit `baseRef`)
- **Dependencies**: `internal/keeprun/slug` (branch naming), `os/exec` (git commands)
- **Interface**:
  ```go
  // Manager handles git worktree lifecycle operations.
  type Manager struct {
      repoDir string
      timeout time.Duration
  }

  // NewManager creates a worktree manager for the given repository.
  func NewManager(repoDir string, opts ...ManagerOption) *Manager

  // Create creates a new worktree at .claude/worktrees/<slug> with a fresh
  // branch keep-run-<slug> rooted at baseRef (the repo's default branch,
  // resolved once at startup — NOT the caller's current HEAD, so worktrees are
  // isolated even when /keep-run is launched from another branch).
  func (m *Manager) Create(ctx context.Context, slug, baseRef string) (worktreeDir string, err error)

  // DefaultBranch resolves the repository's default branch (e.g. main) so the
  // orchestrator can pass a stable baseRef regardless of the current checkout.
  func (m *Manager) DefaultBranch(ctx context.Context) (string, error)

  // Remove removes the worktree and cleans up.
  func (m *Manager) Remove(ctx context.Context, worktreeDir string) error

  // ListBranches returns all branch names matching "keep-run-*".
  func (m *Manager) ListBranches(ctx context.Context) ([]string, error)
  ```
- **Files**: `worktree.go`, `worktree_test.go`

### Module: `internal/keeprun/phase`
- **Responsibility**: Define the 12 SDD phases and their properties
- **Dependencies**: None (constant definitions)
- **Interface**:
  ```go
  // Phase represents a single step in the SDD pipeline.
  type Phase struct {
      Name     string // Human-readable name
      Command  string // codexspec command to invoke (e.g., "codexspec:specify")
      Review   bool   // Whether this is a review phase (iterates until clean)
      Remote   bool   // Whether this phase requires remote operations
  }

  // PipelinePhases returns the ordered list of SDD phases.
  func PipelinePhases() []Phase
  ```
- **Phase definitions** (exact order from spec FR-003):

  | # | Command | Review | Remote | Notes |
  |---|---------|--------|--------|-------|
  | 1 | `codexspec:specify` | no | no | Requirement clarification |
  | 2 | `codexspec:clarify` | no | no | Further clarification |
  | 3 | `codexspec:generate-spec` | no | no | Generate spec.md |
  | 4 | `codexspec:review-spec` | yes | no | Review spec, iterate until clean |
  | 5 | `codexspec:spec-to-plan` | no | no | Generate plan.md |
  | 6 | `codexspec:review-plan` | yes | no | Review plan, iterate until clean |
  | 7 | `codexspec:plan-to-tasks` | no | no | Generate task breakdown |
  | 8 | `codexspec:review-tasks` | yes | no | Review tasks, iterate until clean |
  | 9 | `codexspec:implement-tasks` | no | no | TDD implementation |
  | 10 | `codexspec:review-code` | yes | no | Code review, iterate until clean |
  | 11 | `codexspec:commit-staged` | no | no | Commit (mandatory) |
  | 12 | `codexspec:pr` | no | yes | Push → create Issue → create PR via codexspec:pr (`Closes #N`); only when remote_enabled |

- **Phase 12 (remote) sub-steps**: when `remote_enabled`, the orchestrator (a) pushes `keep-run-<slug>`, (b) creates an Issue with an LLM-composed body from the SDD artifacts and captures its number `N`, then (c) runs `/codexspec:pr` to open the PR with `Closes #N`. All three are gated by `VerifyPhase` (the `pr` row). The Issue is created via `gh issue create` / `glab issue create` (there is no dedicated codexspec command).
- **Files**: `phase.go`, `phase_test.go`

### Module: `internal/keeprun` — PhaseRunner seam (`runner.go`)
- **Responsibility**: Define the single interface through which the orchestrator reaches the LLM. Tests provide a fake; the real implementation lives in `internal/tui`.
- **Dependencies**: `internal/keeprun/phase`, `internal/keeprun/config` (types only)
- **Interface**:
  ```go
  // PhaseRunner executes one SDD phase by driving the engine to run the
  // corresponding /codexspec:* command to completion. This is the ONLY seam
  // through which the orchestrator reaches the LLM.
  type PhaseRunner interface {
      RunPhase(ctx context.Context, req PhaseRequest) (PhaseOutcome, error)
  }

  type PhaseRequest struct {
      Phase        Phase    // which codexspec command (phase.go)
      WorktreeDir  string   // working directory for the run
      SpecDir      string   // .codexspec/specs/<slug>/ for artifacts
      Config       Config   // clarify_prompt, review_fix_prompt, review_mode
      Instruction  string   // extra guidance (e.g. the fix prompt on a review retry)
      AllowedTools []string // optional per-run tool allow-list; merge is blocked by a bash guard, not here (FR-010)
  }

  type PhaseOutcome struct {
      Output string // raw engine output (the injected verdict block is parsed from here)
  }
  ```
- **Files**: `runner.go` (no `_test.go`; exercised via the orchestrator's fake)

### Module: `internal/keeprun` — Verifier (`verify.go`)
- **Responsibility**: Deterministically confirm a phase produced its expected artifact before it is recorded complete (FR-013). Generative phases are judged by the filesystem/git, never by LLM prose.
- **Dependencies**: `os/exec` (git + test commands), filesystem, `internal/keeprun/phase`
- **Interface**:
  ```go
  // VerifyPhase checks the deterministic completion gate for a phase.
  func VerifyPhase(ctx context.Context, phase Phase, tc TaskContext) error

  // ReviewClean parses the injected verdict block
  //   <!-- keep-run-verdict: {"status":"pass","critical":0,"high":0} -->
  // from a review phase's output and reports whether status == "pass".
  // A missing or malformed block counts as not-clean (fail-safe). See Decision 8.
  func ReviewClean(out PhaseOutcome) bool
  ```
- **Verification matrix**:

  | Phase | Gate |
  |-------|------|
  | specify / clarify | non-empty engine output (no file artifact) |
  | generate-spec | `SpecDir/spec.md` exists and is non-empty |
  | spec-to-plan | `SpecDir/plan.md` exists and is non-empty |
  | plan-to-tasks | `SpecDir/tasks.md` exists and is non-empty |
  | implement-tasks | project tests pass; working tree shows changes |
  | review-spec/plan/tasks | `ReviewClean(outcome)` is true (injected verdict block, status=pass) |
  | review-code | `ReviewClean` true **and** objective gates pass: `go build` / `vet` / `test` / `gofmt -l` |
  | commit-staged | HEAD advanced since phase start; working tree clean |
  | pr | branch pushed; **Issue created** (number captured); PR created referencing the issue (`Closes #N`); PR URL present (remote only) |

- **Files**: `verify.go`, `verify_test.go`

### Module: `internal/keeprun` — Orchestrator (`orchestrator.go`)
- **Responsibility**: Run the full pipeline deterministically — task selection, slug/worktree lifecycle, phase sequencing, resume, review iteration, retry/backoff, status updates, cleanup, exit. Reaches the LLM only via `PhaseRunner` (NFR-006).
- **Dependencies**: all pure `internal/keeprun` modules + `PhaseRunner` (injected) + `worktree.Manager`
- **Interface**:
  ```go
  // Orchestrator drives the keep-run pipeline.
  type Orchestrator struct { /* repoDir, runner, wt, backoff, sink */ }

  func NewOrchestrator(repoDir string, runner PhaseRunner, opts ...Option) *Orchestrator

  // Run processes pending tasks until none remain or ctx is canceled.
  // Cancelation (Ctrl+C) stops cleanly; the state file allows later resume.
  func (o *Orchestrator) Run(ctx context.Context) error

  // ProgressSink receives structured progress events for the TUI (FR-012).
  type ProgressSink interface{ Event(ev ProgressEvent) }

  // BackoffPolicy controls retry waits: rate-limited, NO cap (FR-007).
  type BackoffPolicy struct{ Base, Max time.Duration }
  ```
- **Files**: `orchestrator.go`, `orchestrator_test.go` (TDD with a fake `PhaseRunner` — no real LLM)

### Module: `internal/tui` — Built-in registration + real PhaseRunner
- **Responsibility**: Register `/keep-run` as a `CommandBuiltin`; on invoke, build the real `PhaseRunner`, construct an `Orchestrator`, launch it on a goroutine, bridge `ProgressSink` events to the TUI events channel, and wire Ctrl+C cancelation. Implement `RunPhase` by resolving the `/codexspec:<command>` body via the slash `Executor` and calling `runner.RunRestricted(ctx, body, allowedTools, reporter)`; install the `MergeGuard` bash middleware so merge commands are denied (FR-010). For **review** phases, dispatch on `Config.ReviewMode` (Decision 9): `direct` → inline `RunRestricted` (a normal run in the shared session); `subagent` → isolated subagent run of the review command (clean context) whose report feeds the orchestrator's fix step.
- **Dependencies**: `internal/keeprun`, `internal/slash` (Executor, Registry), `internal/engine` (runner)
- **Files**: `internal/tui/keeprun_builtin.go`, `internal/tui/keeprun_runner.go`

## Data Models

### Task (Backlog Entry)

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| Type | string | Task category | One of: feature, fix, refactor, docs, chore, test |
| Title | string | Task heading text | Non-empty, stripped of [type] prefix |
| Priority | string | Processing priority | One of: high, medium, low |
| Status | string | Current state | One of: pending, done |
| Description | string | Detailed task description | May be multi-line |
| HeadingLine | int | 1-indexed line of `## [type]` heading | Used for status updates |

### Config (keep-run.config.json)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| remote_enabled | bool | true | Whether to push/create issues/PRs |
| review_mode | string | "subagent" | Review execution: subagent (isolated reviewer → report → engine fix run) or direct (inline engine run) — Decision 9 |
| clarify_prompt | string | "Make decisions that prioritize correctness, simplicity, and alignment with project conventions." | System prompt for clarification phases |
| review_fix_prompt | string | "Fix all issues, warnings, and suggestions. Prioritize correctness and code quality. Follow project constitution and TDD principles." | System prompt for review fix cycles |
| retry_policy.backoff | string | "exponential" | Backoff strategy for retries |

### Phase (SDD Pipeline Step)

| Field | Type | Description |
|-------|------|-------------|
| Name | string | Human-readable phase name |
| Command | string | Slash command to invoke |
| Review | bool | Whether phase iterates until clean |
| Remote | bool | Whether phase requires remote access |

### State File (.keep-run-state.json)

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| task_slug | string | Slug derived from task title | Must match FR-005 algorithm output |
| worktree_path | string | Absolute path to worktree directory | Must be valid filesystem path |
| completed_phases | []int | 1-indexed phase numbers that fully completed | Ascending order, values in [1, 12] |
| remote_enabled | bool | Cached from config at task start | — |
| last_phase_at | string | ISO 8601 timestamp of most recent phase completion | — |

## Decisions

### Decision 1: Hybrid — Go Orchestrator + Reused LLM-Driven SDD Commands

**Context**: The `/keep-run` command needs to orchestrate 12 SDD phases, each of which is an LLM-driven slash command. The question is whether to build a Go-orchestrated pipeline or let the LLM drive via a prompt.

**Options Considered**:

1. **Built-in Go command** — Go code creates the pipeline, invokes engine.Run() per phase, validates results
2. **Prompt command (.md file)** — Detailed prompt instructs the LLM to drive the pipeline using Skill tool invocations
3. **Hybrid** — Go owns all orchestration/control; the LLM is invoked only to run each reused `/codexspec:*` phase

**Decision**: Option 3 (Hybrid). *(Supersedes the original choice of Option 2.)* A built-in `/keep-run` command starts a Go orchestrator (`internal/keeprun.Orchestrator`) that owns the entire control plane. The orchestrator reaches the LLM only through the `PhaseRunner` seam, which drives `engine.Run` on one reused `/codexspec:*` command per phase, then a deterministic `VerifyPhase` gate confirms the artifact before advancing.

**Why the change from Option 2**: A pure prompt driver cannot *guarantee* phase ordering, resume correctness, the exit condition, or merge prohibition — those depend on the LLM following prose. The product requires precise, enforced control (FR-013, NFR-006). Only Go code can provide it. The irreducibly LLM-driven part — the work *inside* each of the 12 phases — stays delegated to the existing `/codexspec:*` commands; everything around it becomes deterministic Go.

**Rationale**:
- Ordering / no-skip / resume / exit become a Go `for` loop over `PipelinePhases()` + `State.NextPhase()` — impossible to violate.
- Merge prohibition is enforced by a bash-command guard that denies merge commands, not by instruction (FR-010).
- Each phase is gated by a filesystem/git artifact check (`VerifyPhase`), so a phase is recorded complete only when it provably produced output.
- The orchestrator is unit-testable with a fake `PhaseRunner` (no real LLM), satisfying the constitution's TDD mandate for control logic.
- The 12 `/codexspec:*` command files are reused unchanged — the only LLM-driven surface.

**Trade-offs**: More new Go infrastructure than a prompt file, and a new integration point (driving multi-turn engine runs from a built-in handler with progress streaming). Accepted because deterministic control is a hard requirement. The `keep-run.md` prompt driver is removed.

### Decision 2: Phases Are Consecutive Runs in One Session (Compaction Bounds Context)

**Context**: In the engine, a `run` is one user-prompt submission within a `session` (`engine.Run(sess, prompt)` → its own `run.ID`); a session accumulates conversation history across runs (session ⊃ runs ⊃ turns). Should keep-run's phases share one session or each get a fresh one?

**Decision**: All phases of a keep-run invocation are **consecutive runs in one shared session** — exactly like a user sending one message per phase in the TUI. Each phase is still its own `run` (own `run.ID`, trace, metrics), so the Go orchestrator keeps precise per-phase control, gating, and retry; but the runs share one session, so context accumulates naturally. The engine's existing automatic context compaction bounds growth as it nears the token threshold. *(This restores the original single-conversation model. An interim Hybrid revision wrongly equated "one run per phase" with "one session per phase" and switched to fresh-per-phase — over-engineering, now reverted.)*

**Rationale**: This reuses the engine's run/session/compaction machinery with zero special handling and mirrors how the manual SDD workflow already runs (a user invoking `/codexspec:*` in sequence). Accumulated context is helpful (later phases see earlier reasoning) and free (compaction handles overflow). The on-disk SDD artifacts remain the durable source of truth, so resume after a crash reads artifacts regardless of the in-memory conversation.

**Trade-offs**: Context grows within a session and relies on compaction; acceptable because compaction already exists and is automatic. For an unbiased review, a review phase may instead run in an isolated subagent (clean context) via `review_mode: subagent` (FR-008, Decision 9).

### Decision 3: Phase-Level Resume via State File

**Context**: The pipeline may run for hundreds of turns across multiple tasks. Interruptions (Ctrl+C, OOM, crash) are inevitable. Context compaction may discard earlier phase details.

**Decision**: Track pipeline progress in `.keep-run-state.json` (schema defined in spec FR-002). After each phase completes, append its number to `completed_phases`. On resume, compute `next_phase = max(completed_phases) + 1` and skip already-completed phases. Re-read `BACKLOG.md` after each task completes (not during a task's execution).

**Rationale**: Phase-level resume avoids re-running completed phases, saving API calls and time. The state file serves two purposes: (1) intra-task context recovery after compaction, and (2) inter-run resume after interruption. Reusing the existing worktree preserves artifacts from completed phases. Re-reading `BACKLOG.md` between tasks ensures newly added or changed tasks are picked up.

**Trade-offs**: Additional file I/O per phase. The overhead is negligible compared to LLM API calls. Requires strict discipline: a phase must only be marked complete after artifacts are verified (review phases iterate until clean).

### Decision 4: Go Packages Are Load-Bearing Runtime Control

**Context**: Under Hybrid, the `internal/keeprun/` packages are not advisory specifications — they ARE the runtime control plane.

**Decision**: `backlog`, `config`, `slug`, `phase`, `worktree`, plus the new `orchestrator`, `runner` (PhaseRunner seam), and `verify` are compiled into the binary and executed at runtime. All retain full TDD coverage. *(Supersedes the original framing of these packages as "testable specifications" only.)*

**Rationale**:
- The control logic that must be deterministic (parsing, sequencing, resume, gates) now lives in tested Go, not prose.
- The `PhaseRunner` interface keeps the orchestrator free of any `internal/engine`/`internal/tui` import, so the entire control plane is unit-testable with a fake runner (NFR-006).
- The foundational packages built in the prior phase (T001–T017) are reused as-is, except `worktree.Create` gains an explicit `baseRef`.

**Trade-offs**: More Go surface to maintain than a single prompt file. Accepted: this surface is exactly what buys the precise control the product requires.

### Decision 5: Worktree Storage Location

**Context**: Git worktrees need a physical directory. The question is where to place them relative to the repository.

**Decision**: Worktrees are created at `.claude/worktrees/<slug>/` within the repository root.

**Rationale**: This location:
- Is already used by the Claude Code worktree system (consistent with existing patterns)
- Is typically gitignored (worktrees are temporary)
- Keeps all worktrees grouped under one directory
- Allows the LLM to easily reference worktree paths

**Trade-offs**: Worktree directories inside `.claude/` might surprise users. Mitigated by cleanup after task completion.

### Decision 6: Two-Layer Error Recovery (Go retry loop + within-phase LLM recovery)

**Context**: The spec requires all failures to be self-healed with no safety limits (FR-007).

**Decision**: The orchestrator owns a deterministic retry/backoff loop *around* each phase (exponential, rate-limited, no cap). *Within* a single phase run, the existing engine infrastructure (error-recovery prompt injection, context compaction, request chunking) handles transient LLM/tool failures as it does for any run. *(Supersedes the original "no Go-level retry loop" decision.)*

**Rationale**: Deterministic outer retries make recovery observable and bounded in rate (not in count), and pair naturally with the artifact gate: a phase is retried until `VerifyPhase` passes. The inner LLM-level recovery is unchanged and free.

**Trade-offs**: A permanently failing phase retries forever (by design — FR-007 forbids an escape hatch). The rate-limited backoff prevents a hot spin; see Risks. If a true dead-end is possible, that is a product decision to revisit, not a silent cap to add.

### Decision 7: Driving the Engine From a Built-in; Merge Blocked by a Bash Guard

**Context**: `/keep-run` is a long-running, multi-phase, possibly multi-hour process, but built-in handlers (`HandlerFunc`) today own short, synchronous TUI side effects. Separately, the merge prohibition (FR-010) cannot be enforced by a tool allow-list: a merge is a `bash` command (`git merge`, `gh pr merge`), not a distinct tool, so withholding it would mean withholding `bash` (which phases need).

**Decision**: The `/keep-run` built-in launches the orchestrator on a background goroutine that reuses the TUI's existing async run plumbing (events channel, cancelation, reporter) — the same machinery `executePromptCommand` uses — rather than blocking the handler. Each phase is run via `runner.RunRestricted(ctx, body, allowedTools, reporter)`. Merge prohibition is enforced by a **bash-command guard middleware** (`MergeGuard`, using `keeprun.MergeProhibited`) installed on the keep-run tool registry: its `BeforeExecute` denies any bash call whose command is a git/gh/glab merge (a leading-token prefix check, robust to chaining like `cd x && git merge`). This holds FR-010 by construction regardless of LLM behavior, with precedent in the existing `DangerMiddle`.

**Rationale**: Reusing the run lifecycle gives progress streaming, Ctrl+C cancelation, and metrics/tracing for free. The bash guard turns merge prohibition from prose into an enforced invariant that TC-013 can assert — and is the only mechanism that actually works, since merge is a bash command.

**Trade-offs**: New wiring in `internal/tui` to bridge orchestrator progress events into the TUI. The control core stays in `internal/keeprun` and remains testable; only the thin adapter is TUI-coupled.

### Decision 8: Review-Clean Is Decided by an Injected Verdict Block, Not by Editing Review Commands

**Context**: A review phase must signal "no findings remain" in a way Go can read deterministically. Editing all four `/codexspec:review-*` commands is too scattered; parsing their free-form prose is too fragile.

**Decision**: The orchestrator injects **one** runtime instruction (via `PhaseRequest.Instruction`, defined once as a Go constant) into every review phase, requiring the run to end by emitting a machine-readable verdict block:

```
<!-- keep-run-verdict: {"status":"pass","critical":0,"high":0} -->
```

`ReviewClean` parses that block (structured JSON, fixed schema) — never the surrounding prose. The four review command files are **not** modified. Detection is fail-safe: a missing or malformed block, or `status != "pass"`, counts as not-clean, so the orchestrator iterates (`run → fix with review_fix_prompt → re-run`) until the block reports pass.

Additionally, for the **review-code** phase, `VerifyPhase` enforces objective gates in pure Go — `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -l` must all be clean — so a subjective "pass" can never let broken or unformatted code through. (3 of the 4 review commands already persist a report to `.codexspec/specs/<slug>/review-<phase>.md`; that report stays a useful human artifact but is not relied upon for the verdict.)

**Rationale**: The signal lives in exactly one place (the injected instruction) — not "scattered" — and is read as structured JSON with fail-safe defaults — not "fragile prose parsing." The review commands stay untouched. The review-code objective gates remove the only case where a subjective pass could ship a real defect.

**Trade-offs**: The verdict still depends on the LLM emitting the block when instructed; the fail-safe (missing ⇒ iterate) makes that safe but can cost an extra review cycle if the LLM omits it. If absolute rigidity is ever needed, the block can be upgraded to a structured tool call (Option 3 from the design discussion) without touching the review commands.

### Decision 9: Review Execution Mode (`subagent` vs `direct`)

**Context**: A review phase (`/codexspec:review-*`) can be executed two ways; spec FR-008's `review_mode` selects between them. This is an *intra-phase* execution choice and is independent of Decision 2 (which governs *inter-phase* context) — it does not carry another phase's conversation, so it does not conflict with the per-phase model.

**Decision**: For review phases, `PhaseRunner` dispatches on `Config.ReviewMode`:
- `"subagent"` (default) — Run the review command in an isolated **subagent** via the existing fork-mode execution path (`internal/slash` `Executor` → `internal/subagent`; `ExecutionResult.Fork`). The reviewer sees only the on-disk artifacts and returns an independent report. The orchestrator's review-iterate loop then, if the verdict is not clean, performs a fix via a main-loop `engine.Run` (using `review_fix_prompt` + the findings) and re-reviews. Reviewer and fixer are separate agents.
- `"direct"` — Run the review command inline as a single `engine.Run` in the current loop; the same agent reviews and fixes across iterations.

Either way the review emits the verdict block (Decision 8), which `ReviewClean` parses identically regardless of mode.

**Rationale**: `subagent` yields an unbiased review (the reviewer did not author the code/artifact under review) at the cost of one extra isolated run; `direct` is cheaper and simpler. Both fit the per-phase model.

**Trade-offs**: `subagent` mode requires the real `PhaseRunner` (`internal/tui`) to drive fork-mode execution and then a consuming `engine.Run`; `direct` is a plain `RunRestricted`. The orchestrator's iterate loop is mode-agnostic; only `PhaseRunner` differs.

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Review-clean detection misjudges "clean" (Decision 8) | Low | Medium | Injected verdict block (structured JSON, fixed schema) read by `ReviewClean`; fail-safe (missing/malformed ⇒ iterate); review-code additionally gated by objective `build`/`vet`/`test`/`gofmt`. No review-command edits. A wrong "not clean" only costs an extra cycle. |
| Driving multi-turn engine runs from a built-in (new plumbing) | Medium | Medium | Reuse the existing async run lifecycle (events channel, cancelation, reporter); keep the control core in `internal/keeprun` (testable) with only a thin adapter in `internal/tui`; cover with integration tests. |
| Permanently failing phase retries forever (no cap, FR-007) | Low | Medium | Rate-limited backoff prevents hot-spin; by design no escape hatch. Operator can Ctrl+C; state file resumes later. Revisit only as a product decision. |
| Artifact gate wrong (false pass/fail in `VerifyPhase`) | Low | Medium | Explicit per-phase gate table; full unit-test coverage of `VerifyPhase` / `ReviewClean`. |
| Context window growth across phases | Medium | Medium | One accumulating session (Decision 2); the engine's automatic compaction bounds context as it nears the threshold; artifacts on disk are the durable handoff. |
| Worktree rooted at wrong base (was HEAD) | Low | Medium | `worktree.Create` takes an explicit `baseRef` from `DefaultBranch`, resolved once at startup. |
| Merge prohibition enforcement | Low | Critical | Enforced by construction: orchestrator never merges; a bash-command guard (`MergeGuard` + `keeprun.MergeProhibited`) denies any git/gh/glab merge command in any phase (FR-010); asserted by TC-013. |
| Slug collision with existing branches | Low | Low | `DeduplicateSlug` + `ListBranches` checked in Go before `Create`. |
| Config file missing or malformed | Low | Low | `LoadConfig` returns defaults for missing file; malformed JSON falls back to defaults with a logged warning. |
| Network failures during remote operations | Medium | Medium | Go retry/backoff around the phase + existing within-phase recovery. |
| BACKLOG.md format variations | Medium | Low | `ParseBacklog` tolerates whitespace / missing fields; covered by tests. |
| Stale worktrees from interrupted runs | Low | Low | Phase-level resume (FR-002): existing worktrees are reused; cleanup only after task reaches `done`. |

## Implementation Phases

### Phase 1: Foundation — Core Parsing Packages

Deliverables: Testable Go packages for backlog parsing, state file management, config loading, and slug generation.

- [ ] Write tests for `backlog.ParseBacklog` — valid format, multiple tasks, empty file, missing fields, edge cases (FR-001)
- [ ] Implement `backlog.go` to pass tests
- [ ] Write tests for `backlog.UpdateStatus` — pending→done, already done, invalid line
- [ ] Implement status update logic
- [ ] Write tests for state file read/write — valid state, empty completed_phases, partial progress, missing file, resume phase computation (FR-002)
- [ ] Implement state file parsing and `NextPhase` computation in `backlog.go`
- [ ] Write tests for `config.LoadConfig` — valid config, missing file, partial fields, defaults (FR-008, FR-009)
- [ ] Implement `config.go` with defaults matching spec FR-008 table
- [ ] Write tests for `slug.GenerateSlug` — all cases from FR-005, edge cases (unicode, long titles, special chars)
- [ ] Implement `slug.go` with exact algorithm from FR-005
- [ ] Write tests for `slug.DeduplicateSlug` — collision scenarios
- [ ] Implement deduplication with numeric suffix
- [ ] Write `doc.go` with package documentation
- [ ] Verify all tests pass: `go test ./internal/keeprun/...`

### Phase 2: Worktree and Phase Management

Deliverables: Git worktree lifecycle management and SDD phase definitions.

- [ ] Write tests for `phase.PipelinePhases` — correct ordering, 12 phases, properties (FR-003)
- [ ] Implement `phase.go` with all 12 SDD phases
- [ ] Write tests for `worktree.Manager.Create` — successful creation, branch naming, directory structure (FR-005)
- [ ] Implement `worktree.go` with `git worktree add` command
- [ ] Write tests for `worktree.Manager.Remove` — successful cleanup, already removed
- [ ] Implement cleanup with `git worktree remove`
- [ ] Write tests for `worktree.Manager.ListBranches` — existing keep-run-* branches
- [ ] Implement branch listing
- [ ] Verify all tests pass: `go test ./internal/keeprun/...`

### Phase 3: Orchestrator, PhaseRunner Seam, Verifier, and Built-in Wiring

Deliverables: The deterministic Go control plane and its TUI integration. *(Replaces the former "Prompt Command" phase; `keep-run.md` is removed.)*

- [ ] Revise `worktree.Create` to take an explicit `baseRef`; add `DefaultBranch`. Update tests.
- [ ] Define the `PhaseRunner` interface + `PhaseRequest`/`PhaseOutcome` in `runner.go`.
- [ ] TDD `verify.go`: `VerifyPhase` per the gate table (incl. review-code objective gates) + `ReviewClean` parsing the injected verdict block.
- [ ] TDD `orchestrator.go` against a **fake** `PhaseRunner` (no real LLM): task selection, slug + dedup, worktree create/resume, `next_phase` from state, ordered phase loop, review-iteration loop, artifact gate, `WriteState` after each phase, `UpdateStatus` to done, cleanup, exit, progress events (FR-002/003/012/013, NFR-006).
- [ ] TDD the retry/backoff policy: rate-limited, no cap (FR-007); retries until the gate passes.
- [ ] Implement the real `PhaseRunner` in `internal/tui/keeprun_runner.go`: resolve `/codexspec:<command>` via the `Executor`, call `runner.RunRestricted`, install the `MergeGuard` bash middleware (FR-010), map `RunResult` → `PhaseOutcome`.
- [ ] Register `/keep-run` as a `CommandBuiltin` in `internal/tui/keeprun_builtin.go`: start the orchestrator goroutine, bridge `ProgressSink` → TUI events, wire Ctrl+C.
- [ ] Manual test: sample `BACKLOG.md`, run `/keep-run`, verify ordered execution, resume, and that a `git merge` attempt is denied by the guard.

### Phase 4: Testing and Acceptance Validation

Deliverables: Comprehensive test coverage and acceptance test validation.

- [ ] Verify all unit tests pass: `go test ./internal/keeprun/... -v` and `go test ./internal/tui/... -run KeepRun`
- [ ] Confirm the orchestrator suite runs with a fake `PhaseRunner` (no real LLM) — proves NFR-006
- [ ] Run `gofmt -l ./internal/keeprun/` — no formatting issues
- [ ] Validate against acceptance test cases from spec:
  - [ ] TC-001: Basic pipeline execution (single pending task)
  - [ ] TC-002: Multi-task sequential processing
  - [ ] TC-003: Resume after interruption (one done, one pending)
  - [ ] TC-003b: Phase-level resume (existing worktree with state file showing phases 1-6 done, resume from phase 7)
  - [ ] TC-004: Worktree isolation (no cross-task artifacts)
  - [ ] TC-005: No remote operations when disabled
  - [ ] TC-006: Merge prohibition enforcement
  - [ ] TC-007: Error self-healing — network failure simulation
  - [ ] TC-008: Error self-healing — context window (compact + continue)
  - [ ] TC-009: Config file defaults (no config file)
  - [ ] TC-010: Exit condition (all tasks done)
  - [ ] TC-011: Phase-completion verification gate (artifact missing → not recorded, retried)
  - [ ] TC-012: Deterministic ordering + resume via a mocked runner (phases 7-12 only, in order)
  - [ ] TC-013: merge commands denied by the bash guard
- [ ] Validate edge cases from spec:
  - [ ] Empty BACKLOG.md
  - [ ] Missing BACKLOG.md
  - [ ] All tasks already done
  - [ ] Task with invalid type prefix
  - [ ] Config with missing fields
  - [ ] Worktree creation failure
  - [ ] Remote push rejected
  - [ ] SDD phase produces empty output — retry phase as error condition
  - [ ] Agent runs out of API quota mid-task — wait for quota refresh and continue
- [ ] Performance: measure single-task pipeline duration, ensure acceptable for developer workflow

## Security Considerations

- **Input validation**: `backlog.ParseBacklog` validates markdown structure; `config.LoadConfig` validates JSON format and field values
- **Shell injection**: `worktree.Manager` sanitizes slug before passing to `git` commands via `exec.Command` (argument array, not shell string)
- **Path traversal**: Slug generation only produces `[a-z0-9-]` characters, preventing directory traversal
- **No hardcoded secrets**: Config file stores no credentials; remote operations use existing git/gh authentication
- **Merge prohibition**: Enforced by construction — the orchestrator never invokes a merge, and a bash-command guard denies any git/gh/glab merge command in every phase (FR-010); not reliant on prompt instruction

## Performance Considerations

- **I/O bound**: Pipeline is dominated by LLM API calls (seconds per turn) and git operations (milliseconds). Go code performance is not a bottleneck.
- **Context compaction**: Phases are consecutive runs in one session (Decision 2); the engine's automatic compaction bounds context growth as it nears the token threshold. State file + on-disk artifacts ensure recovery after interruption.
- **Memory**: Each task's worktree is cleaned up after completion, preventing disk space accumulation.
- **Concurrency**: Tasks are strictly sequential (spec requirement). No concurrent execution needed.
