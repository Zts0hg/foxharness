# Implementation Plan: /keep-run — Autonomous SDD Pipeline Runner

**Related Spec**: `.codexspec/specs/2026-0531-23020o-keep-run-sdd-pipeline/spec.md`
**Created**: 2026-06-01
**Status**: Draft

## Context

foxharness-go is an AI agent harness with a mature slash command infrastructure. Users invoke `/codexspec:*` commands sequentially to drive the Spec-Driven Development (SDD) pipeline. Currently this is a manual process — the user must invoke each command, review output, and trigger the next phase.

`/keep-run` automates this by reading a `BACKLOG.md` file and processing each `pending` task through all 12 SDD phases in isolated git worktrees. The agent runs autonomously, self-heals errors, and produces one commit (and optionally one Issue + PR) per task without ever merging to main.

The implementation builds on three existing systems:
- **Slash command infrastructure** (`internal/slash/`) — command discovery, execution pipeline, fork/inline modes
- **Engine loop** (`internal/engine/`) — turn-based LLM execution with tool calls
- **Error recovery** (`internal/recovery/`) — failure detection and recovery prompt injection

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
| No new external dependencies required | | | |

## Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| 1. TDD | ✅ | All Go packages developed test-first. Prompt command tested through acceptance scenarios. |
| 2. Code Quality | ✅ | Single-responsibility packages. Injectable dependencies. Clear interfaces. |
| 3. Go Documentation Standards | ✅ | Block comments on all exported identifiers. `doc.go` for package docs. No teaching comments. |
| 4. Testing Standards | ✅ | Table-driven unit tests. Edge case coverage. Error path testing. |
| 5. Architecture | ✅ | New `internal/keeprun/` package with focused responsibilities. Interfaces before implementations. |
| 6. Performance | ✅ | Pipeline is I/O bound (LLM calls, git operations). No hot paths. |
| 7. Security | ✅ | Input validation at boundaries (BACKLOG.md, config.json). No hardcoded secrets. Shell arguments sanitized. |

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                        User types /keep-run                       │
└──────────────────────────────┬───────────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────────┐
│                     TUI (bubbletea)                               │
│               handleSlashCommand → lookup                         │
│               executePromptCommand → engine.Run()                 │
└──────────────────────────────┬───────────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────────┐
│               Engine Loop (single long conversation)              │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │  keep-run.md prompt → LLM reads BACKLOG.md                  │ │
│  │                                                               │ │
│  │  Read BACKLOG.md → re-read after each task completes          │ │
│  │                                                               │ │
│  │  FOR each pending task:                                       │ │
│  │    1. Check existing worktree + state file (FR-002 resume)    │ │
│  │    2. If exists: reuse worktree, resume from next phase       │ │
│  │    3. If not: generate slug → create worktree                 │ │
│  │    4. Create .codexspec/specs/{slug}/ in worktree             │ │
│  │    5. FOR each remaining SDD phase:                           │ │
│  │       a. Invoke /codexspec:* via Skill tool                   │ │
│  │       b. Append completed phase to state file                 │ │
│  │       c. On error: retry with backoff / compress context      │ │
│  │    6. Commit (mandatory)                                      │ │
│  │    7. If remote_enabled: push + create Issue + PR             │ │
│  │    8. Update BACKLOG.md status → done                         │ │
│  │    9. Clean up worktree → re-read BACKLOG.md                  │ │
│  │                                                               │ │
│  │  EXIT when all tasks are done                                 │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  Supporting Infrastructure:                                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐│
│  │ Error Recovery│  │  Compaction  │  │  Skill Tool (slash exec) ││
│  │ (existing)    │  │  (existing)  │  │  (existing)              ││
│  └──────────────┘  └──────────────┘  └──────────────────────────┘│
└──────────────────────────────────────────────────────────────────┘
```

## Component Structure

```
.claude/commands/codexspec/
└── keep-run.md                  # Prompt command: LLM pipeline driver

internal/keeprun/
├── doc.go                       # Package documentation
├── backlog.go                   # BACKLOG.md parser and task state management
├── backlog_test.go              # TDD tests for backlog parsing
├── config.go                    # keep-run.config.json loader with defaults
├── config_test.go               # TDD tests for config loading
├── slug.go                      # Task slug generation algorithm
├── slug_test.go                 # TDD tests for slug generation
├── worktree.go                  # Git worktree lifecycle management
├── worktree_test.go             # TDD tests for worktree operations
├── phase.go                     # SDD phase definitions and ordering
└── phase_test.go                # TDD tests for phase transitions

# Files managed at runtime (not in source control):
BACKLOG.md                      # User-created task backlog
keep-run.config.json            # Optional configuration overrides
```

## Module Dependency Graph

```
┌─────────────────────────────┐
│     keep-run.md              │     (prompt command, depends on LLM tools)
│     (prompt command)         │
└──────────┬──────────────────┘
           │ references (specification, not runtime import)
           ▼
┌─────────────────────────────────────────────────────────────┐
│                    internal/keeprun/                          │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐   │
│  │ backlog  │  │  config  │  │   slug   │  │  phase   │   │
│  └──────────┘  └──────────┘  └─────┬────┘  └──────────┘   │
│                                    │                         │
│                              ┌─────┴────┐                    │
│                              │ worktree │                    │
│                              └─────┬────┘                    │
│                                    │                         │
└────────────────────────────────────┼─────────────────────────┘
                                     │
                                     ▼
                           ┌──────────────────┐
                           │   os/exec         │
                           │   (stdlib git)    │
                           └──────────────────┘
```

Key dependency rules:
- `backlog`, `config`, `slug`, `phase` — no internal dependencies (pure logic each)
- `worktree` depends on `slug` (for branch naming) and `os/exec` (for git commands)
- All packages depend only on Go stdlib and existing project dependencies
- No package depends on `internal/slash/` — the prompt command is the runtime bridge

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
- **Responsibility**: Create and remove git worktrees for task isolation
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

  // Create creates a new worktree at .claude/worktrees/<slug> with
  // branch keep-run-<slug>, rooted at the repo's default branch.
  func (m *Manager) Create(ctx context.Context, slug string) (worktreeDir string, err error)

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
  | 12 | `codexspec:pr` | no | yes | Push + create PR (only when remote_enabled) |

- **Files**: `phase.go`, `phase_test.go`

### Module: `keep-run.md` (Prompt Command)
- **Responsibility**: Drive the full SDD pipeline autonomously through LLM tool calls
- **Dependencies**: Existing slash command infrastructure, Skill tool, all project tools
- **Interface**: Registered as a TUI slash command via `.claude/commands/codexspec/keep-run.md`
- **Files**: `.claude/commands/codexspec/keep-run.md`

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
| review_mode | string | "subagent" | Review execution mode (subagent or direct) |
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

### Decision 1: Prompt Command as Primary Driver

**Context**: The `/keep-run` command needs to orchestrate 12 SDD phases, each of which is an LLM-driven slash command. The question is whether to build a Go-orchestrated pipeline or let the LLM drive via a prompt.

**Options Considered**:

1. **Built-in Go command** — Go code creates the pipeline, invokes engine.Run() per phase, validates results
2. **Prompt command (.md file)** — Detailed prompt instructs the LLM to drive the pipeline using Skill tool invocations
3. **Hybrid** — Go packages for orchestration logic, prompt for LLM interaction

**Decision**: Option 2 (Prompt command) with supporting Go packages for testable specifications.

**Rationale**: All existing SDD commands are prompt-based and LLM-driven. The LLM already has access to all necessary tools (bash, read_file, edit_file, Skill). A prompt command:
- Leverages the existing slash command execution pipeline
- Naturally chains SDD phases through Skill tool invocations
- Handles context management via the existing engine loop
- Requires minimal new infrastructure

The Go packages in `internal/keeprun/` define the expected formats and algorithms as testable specifications, ensuring the prompt's behavior can be validated independently.

**Trade-offs**: Less Go-level enforcement of pipeline ordering. Mitigated by detailed prompt with explicit phase checklist and state tracking.

### Decision 2: Inline Mode (Single Conversation)

**Context**: Slash commands support inline mode (prompt injected into current conversation) and fork mode (sub-agent with isolated context). `/keep-run` needs to invoke multiple SDD commands in sequence.

**Decision**: Inline mode — all phases run within a single engine loop conversation.

**Rationale**: Inline mode allows the LLM to accumulate context across phases. Each phase builds on the previous one's output. The engine loop naturally handles the multi-turn conversation. Fork mode would lose inter-phase context and add significant overhead.

**Trade-offs**: Context grows with each phase, requiring compaction. Mitigated by the existing compaction system and state file persistence.

### Decision 3: Phase-Level Resume via State File

**Context**: The pipeline may run for hundreds of turns across multiple tasks. Interruptions (Ctrl+C, OOM, crash) are inevitable. Context compaction may discard earlier phase details.

**Decision**: Track pipeline progress in `.keep-run-state.json` (schema defined in spec FR-002). After each phase completes, append its number to `completed_phases`. On resume, compute `next_phase = max(completed_phases) + 1` and skip already-completed phases. Re-read `BACKLOG.md` after each task completes (not during a task's execution).

**Rationale**: Phase-level resume avoids re-running completed phases, saving API calls and time. The state file serves two purposes: (1) intra-task context recovery after compaction, and (2) inter-run resume after interruption. Reusing the existing worktree preserves artifacts from completed phases. Re-reading `BACKLOG.md` between tasks ensures newly added or changed tasks are picked up.

**Trade-offs**: Additional file I/O per phase. The overhead is negligible compared to LLM API calls. Requires strict discipline: a phase must only be marked complete after artifacts are verified (review phases iterate until clean).

### Decision 4: Go Packages as Testable Specifications

**Context**: The prompt command handles runtime execution, but the formats and algorithms (backlog parsing, slug generation, config loading) benefit from testable Go implementations.

**Decision**: Create `internal/keeprun/` with full TDD coverage for backlog parsing, config loading, slug generation, worktree management, and phase definitions.

**Rationale**: These packages:
- Define the expected formats and algorithms as executable, tested specifications
- Provide confidence that the prompt's instructions align with correct behavior
- Can be used by a future built-in command if the prompt approach proves insufficient
- Follow the constitution's TDD mandate for all new Go code

**Trade-offs**: Go packages are not directly invoked at runtime by the prompt command. They serve as specifications and future-ready infrastructure. This is acceptable because the LLM is the execution engine, not Go code.

### Decision 5: Worktree Storage Location

**Context**: Git worktrees need a physical directory. The question is where to place them relative to the repository.

**Decision**: Worktrees are created at `.claude/worktrees/<slug>/` within the repository root.

**Rationale**: This location:
- Is already used by the Claude Code worktree system (consistent with existing patterns)
- Is typically gitignored (worktrees are temporary)
- Keeps all worktrees grouped under one directory
- Allows the LLM to easily reference worktree paths

**Trade-offs**: Worktree directories inside `.claude/` might surprise users. Mitigated by cleanup after task completion.

### Decision 6: Error Recovery Strategy

**Context**: The spec requires all failures to be self-healed with no safety limits.

**Decision**: Leverage the existing error recovery system (`internal/recovery/`) plus prompt-level retry instructions. No Go-level retry loop.

**Rationale**: The existing error recovery system already detects repeated tool failures and injects recovery prompts. Combined with detailed prompt instructions for retry strategies (exponential backoff, context compression, batch/chunk processing), this provides robust self-healing without additional Go infrastructure.

**Trade-offs**: Less deterministic than a Go retry loop. The LLM's behavior depends on the prompt and error recovery system working correctly.

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Context window exhaustion during long pipeline | High | High | Compaction system + state file persistence. Prompt instructs LLM to save progress after each phase. |
| LLM deviates from pipeline (skips phases, wrong order) | Medium | High | Detailed prompt with explicit numbered phase checklist. State file records completed phases. |
| Worktree path issues with SDD commands | Medium | Medium | Prompt instructs LLM to use absolute paths. Phase commands reference worktree directory explicitly. |
| Merge prohibition enforcement | Low | Critical | Strong prompt instructions. Constitution already prohibits autonomous merges. |
| Slug collision with existing branches | Low | Low | `DeduplicateSlug` function handles numeric suffixes. LLM checks with `git branch --list`. |
| Config file missing or malformed | Low | Low | `LoadConfig` returns defaults for missing file, logs warning for malformed fields. |
| Network failures during remote operations | Medium | Medium | Existing error recovery + exponential backoff prompt instructions. |
| BACKLOG.md format variations | Medium | Low | Parser handles common variations (extra whitespace, missing fields). Prompt includes format examples. |
| Runaway execution from "no safety limits" principle | Low | Medium | Spec mandates no retry cap. The existing error recovery system and context compaction serve as implicit circuit breakers. Prompt must not introduce artificial limits that contradict spec FR-007. |
| Stale worktrees from interrupted runs | Low | Low | Resolved by phase-level resume (FR-002): existing worktrees are reused, not treated as stale. Cleanup only after task reaches `done`. |

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

### Phase 3: Prompt Command

Deliverables: The `/keep-run` slash command prompt file.

- [ ] Create `.claude/commands/codexspec/keep-run.md` with frontmatter:
  ```yaml
  description: "Autonomous SDD pipeline runner — processes BACKLOG.md tasks sequentially"
  argument-hint: "[no arguments]"
  user-invocable: true
  disable-model-invocation: true
  ```
- [ ] Write prompt body covering:
  - [ ] Backlog reading strategy: read BACKLOG.md at startup, re-read after each task completes to pick up new/changed tasks (US-2)
  - [ ] Task state machine: skip `done`, process `pending`, exit when all `done` (FR-002)
  - [ ] Phase-level resume: check for existing worktree and state file, compute `next_phase = max(completed_phases) + 1`, skip completed phases (FR-002)
  - [ ] Worktree creation: branch naming using slug algorithm, directory path (FR-005)
  - [ ] Create `.codexspec/specs/{slug}/` directory inside the worktree for SDD artifacts (FR-009)
  - [ ] Sequential SDD phase execution via Skill tool invocations (FR-003, exact 12-phase order)
  - [ ] After each phase completes: append phase number to `completed_phases` in state file
  - [ ] Non-interactive decision-making using config prompts (FR-004)
  - [ ] Review phase iteration: loop until all issues/warnings/suggestions resolved
  - [ ] Commit via `/codexspec:commit-staged` (mandatory) (FR-003 step 11)
  - [ ] Conditional remote operations based on `keep-run.config.json` (FR-006)
  - [ ] Merge prohibition — explicit instruction never to merge (FR-010)
  - [ ] Error recovery: cover all 7 scenarios from FR-007 table plus empty output detection and API quota exhaustion
  - [ ] Progress reporting: task, phase, transitions (FR-012)
  - [ ] Exit condition: all tasks done (FR-002)
  - [ ] BACKLOG.md status update after task completion
- [ ] Manual test: create sample `BACKLOG.md`, run `/keep-run`, verify pipeline execution

### Phase 4: Testing and Acceptance Validation

Deliverables: Comprehensive test coverage and acceptance test validation.

- [ ] Verify all unit tests pass: `go test ./internal/keeprun/... -v`
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
- **Merge prohibition**: Prompt explicitly forbids merging; codebase constitution prohibits autonomous merges

## Performance Considerations

- **I/O bound**: Pipeline is dominated by LLM API calls (seconds per turn) and git operations (milliseconds). Go code performance is not a bottleneck.
- **Context compaction**: The engine's existing compaction system handles context growth. State file persistence ensures recovery after compaction.
- **Memory**: Each task's worktree is cleaned up after completion, preventing disk space accumulation.
- **Concurrency**: Tasks are strictly sequential (spec requirement). No concurrent execution needed.
