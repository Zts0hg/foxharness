# Feature: /keep-run — Autonomous SDD Pipeline Runner

## Overview

`/keep-run` is a TUI slash command that automates the full Spec-Driven Development (SDD) pipeline across a prioritized backlog of development tasks. It reads a `BACKLOG.md` file from the project root, processes each `pending` task sequentially through the complete SDD lifecycle in isolated git worktrees, and exits only when all tasks are marked `done`.

The feature enables unattended, 24/7-capable autonomous development: the agent self-heals all errors, never skips tasks, and produces one Issue + PR per task for human review — strictly without merging.

## Goals

- Provide a single command (`/keep-run`) that drives the entire SDD pipeline autonomously
- Maintain a human-editable backlog file (`BACKLOG.md`) as the single source of truth for task priorities and status
- Guarantee code isolation between tasks via mandatory git worktrees
- Ensure all failures are self-diagnosed and self-healed (retry, backoff, context compression, batch requests)
- Enforce strict merge prohibition — only humans may merge feature branches into main
- Support both remote (GitHub/GitLab) and local-only (no remote) workflows

## User Stories

### Story 1: Start autonomous development session
**As a** project maintainer
**I want** to run `/keep-run` in the TUI and have the agent process all pending tasks in my backlog
**So that** I can focus on code review while the agent handles implementation

**Acceptance Criteria:**
- [ ] Running `/keep-run` starts processing the first `pending` task in `BACKLOG.md`
- [ ] Each task is processed in its own git worktree with branch `keep-run-{task-slug}`
- [ ] The agent reports progress as it moves through the SDD pipeline phases
- [ ] The agent automatically exits when all tasks are `done`

### Story 2: Add tasks to the backlog
**As a** developer
**I want** to edit `BACKLOG.md` to add new feature requests, bug fixes, or refactoring tasks
**So that** the agent picks them up on the next `/keep-run` invocation

**Acceptance Criteria:**
- [ ] `BACKLOG.md` follows a flat-list format with `[type]` prefix, Priority, Status, and Description
- [ ] Tasks are processed in file order (top to bottom)
- [ ] New tasks can be added while the agent is idle between runs

### Story 3: Resume after interruption
**As a** developer
**I want** to re-run `/keep-run` after an interruption (crash, OOM, manual stop) and have it resume from where it left off
**So that** no work is lost and no completed tasks are repeated

**Acceptance Criteria:**
- [ ] On startup, the agent scans `BACKLOG.md` and finds the first `pending` task
- [ ] Previously completed tasks (`done`) are skipped automatically
- [ ] For each `pending` task, the agent checks for an existing state file and worktree
- [ ] If a state file exists with completed phases, the agent resumes from the first incomplete phase, reusing the existing worktree and its artifacts
- [ ] If no state file exists, the agent starts the SDD pipeline from phase 1

### Story 4: Review and merge PRs manually
**As a** project maintainer
**I want** to review each PR created by the agent before it reaches the main branch
**So that** I maintain quality control over the codebase

**Acceptance Criteria:**
- [ ] The agent creates one Issue + one PR per task (when remote is enabled)
- [ ] The agent **never** merges any branch into main
- [ ] PRs are created via `/codexspec:pr` command
- [ ] Commits are created via `/codexspec:commit-staged` command

### Story 5: Use project without remote repository
**As a** developer working on a local-only project
**I want** to use `/keep-run` without a remote repository
**So that** the agent commits locally and preserves work on feature branches

**Acceptance Criteria:**
- [ ] When `remote_enabled` is `false` in config, the agent skips push/issue/PR operations
- [ ] Code is committed to the worktree branch via `/codexspec:commit-staged`
- [ ] The worktree branch is preserved as the task's artifact
- [ ] Task is marked `done` after successful local commit

## Functional Requirements

### FR-001: Backlog File Format
The system shall read `BACKLOG.md` from the project root directory. Each task entry shall follow this format:

```markdown
## [type] Task title

**Priority**: high | medium | low
**Status**: pending | done
**Description**: Detailed description of the task...
```

Where `type` is one of: `feature`, `fix`, `refactor`, `docs`, `chore`, or `test`.

Tasks are delimited by `## [type]` headings. Each heading marks the start of a new task; all fields between two consecutive `## [type]` headings belong to the preceding task.

Example with multiple tasks:

```markdown
# Backlog

## [feature] Add dark mode

**Priority**: high
**Status**: pending
**Description**: Add dark mode support with theme toggle and system preference detection.

## [fix] Login timeout bug

**Priority**: medium
**Status**: done
**Description**: Fix timeout on slow network connections when authenticating.

## [refactor] Clean up utility functions

**Priority**: low
**Status**: pending
**Description**: Consolidate duplicate helper functions in the utils package.
```

### FR-002: Task State Machine
Each task shall have exactly two states:
- `pending` — task needs to be processed
- `done` — task has completed the full SDD pipeline and has been committed (and optionally pushed)

The agent shall scan tasks in file order and process the first `pending` task found. The exit condition is: all tasks in `BACKLOG.md` are `done`.

#### Phase-Level Resume via State File
Each task's pipeline progress shall be tracked via a state file (`.keep-run-state.json`) stored in the worktree. The state file shall have the following schema:

```json
{
  "task_slug": "add-dark-mode",
  "worktree_path": ".claude/worktrees/add-dark-mode",
  "completed_phases": [1, 2, 3, 4, 5, 6],
  "remote_enabled": true,
  "last_phase_at": "2026-06-01T10:45:00Z"
}
```

- `completed_phases` — Array of 1-indexed phase numbers that have fully completed (artifacts written, review clean if applicable)
- `task_slug` — The slug derived from the task title (matches FR-005 algorithm)
- `worktree_path` — Absolute path to the task's worktree directory
- `remote_enabled` — Cached from `keep-run.config.json` at task start
- `last_phase_at` — ISO 8601 timestamp of the most recent phase completion

**Resume logic**: When processing a `pending` task, the agent shall:
1. Check if a worktree with branch `keep-run-{task-slug}` already exists
2. If yes, read `.keep-run-state.json` from the worktree
3. Compute `next_phase = max(completed_phases) + 1` (or phase 1 if empty or no state file)
4. Reuse the existing worktree and resume from `next_phase`
5. If no worktree exists, create one and start from phase 1

**Phase completion**: A phase shall only be appended to `completed_phases` after its artifacts are successfully produced and, for review phases, all issues/warnings/suggestions are resolved.

### FR-003: SDD Pipeline Execution
For each `pending` task, the agent shall execute the following SDD phases in order, invoking each as a slash command. The sequence follows the logical SDD progression: clarify requirements first, then generate the specification document, then review the generated document:

1. `/codexspec:specify` — Requirement clarification (non-interactive, LLM decides)
2. `/codexspec:clarify` — Further clarification
3. `/codexspec:generate-spec` — Generate `spec.md`
4. `/codexspec:review-spec` — Review spec, fix all issues/warnings/suggestions
5. `/codexspec:spec-to-plan` — Generate `plan.md`
6. `/codexspec:review-plan` — Review plan, fix all issues/warnings/suggestions
7. `/codexspec:plan-to-tasks` — Generate task breakdown
8. `/codexspec:review-tasks` — Review tasks, fix all issues/warnings/suggestions
9. `/codexspec:implement-tasks` — TDD implementation
10. `/codexspec:review-code` — Code review, fix all issues/warnings/suggestions
11. `/codexspec:commit-staged` — Commit code (mandatory)
12. (If `remote_enabled`) Push + `/codexspec:pr` — Create PR (mandatory when remote enabled)

Each review phase shall iterate until all issues, warnings, and suggestions are resolved.

### FR-004: Non-Interactive Mode
All SDD commands shall run in non-interactive mode during `/keep-run`. The LLM shall autonomously make decisions using guidance from `keep-run.config.json`:

- `clarify_prompt` — System prompt guiding LLM decisions during requirement clarification
- `review_fix_prompt` — System prompt guiding LLM decisions during review fix cycles

### FR-005: Git Worktree Isolation (Mandatory)
Each task shall be developed in its own git worktree to ensure code isolation between tasks. Requirements:

- A new worktree is created for each `pending` task
- Branch naming: `keep-run-{task-slug}` where `task-slug` is derived as follows:
  1. Take the task title text (e.g., `Add dark mode support`)
  2. Strip the `[type]` prefix if present (e.g., `[feature] Add dark mode support` → `Add dark mode support`)
  3. Convert to lowercase
  4. Replace any character not in `[a-z0-9]` with a hyphen (`-`)
  5. Collapse consecutive hyphens into a single hyphen
  6. Strip leading and trailing hyphens
  7. Truncate to a maximum of 60 characters, breaking at the last hyphen boundary if possible
  8. On collision with an existing branch, append a numeric suffix: `-2`, `-3`, etc.
  - Example: `[feature] Add dark mode support` → `add-dark-mode-support`
  - Example: `[fix] Fix timeout on slow connections!!!` → `fix-timeout-on-slow-connections`
- SDD artifacts are stored in `.codexspec/specs/{task-slug}/` within the worktree
- The worktree is preserved between runs to support phase-level resume (FR-002)
- After a task reaches `done` status, the worktree is cleaned up
- Worktree creation and cleanup are mandatory regardless of remote configuration

### FR-006: Remote Operations (Optional)
Remote operations (push, issue, PR) shall be controlled by the `remote_enabled` field in `keep-run.config.json`:

**When `remote_enabled: true`:**
- Push the feature branch to the remote repository
- Create a GitHub/GitLab Issue with LLM-generated content (enriched by SDD context)
- Create a PR via `/codexspec:pr` referencing the issue (`Closes #N`)
- **Strictly prohibited** from merging the PR or the feature branch into main

**When `remote_enabled: false`:**
- Commit code to the local worktree branch via `/codexspec:commit-staged`
- Preserve the branch as the task artifact
- Skip all remote operations

### FR-007: Error Self-Healing
All failures during the SDD pipeline shall be self-diagnosed and self-healed. The agent shall never skip a task or abort due to errors.

| Failure Scenario | Recovery Strategy |
|---|---|
| Network connectivity failure | Retry with exponential backoff |
| Context window exceeded | Compress context, then retry |
| Unknown disconnection | Reduce request/response size via batch/chunk processing, then retry |
| API rate limit exceeded | Exponential backoff, wait for rate limit reset |
| API quota exhausted | Wait for quota refresh, then retry |
| Test failure | Treat as code implementation error feedback, fix code |
| Review issues/warnings/suggestions | Fix all findings |

The agent shall leverage existing project infrastructure (error recovery system, system reminders) to prevent infinite loops and execution drift.

**Design principle: No safety limits.** The agent shall have no maximum retry count, no timeout, and no task-level safety limit on recovery attempts. The system relies entirely on the existing error recovery system, system reminders, and context compaction to prevent runaway execution. There is no escape hatch that marks a task as failed or blocked — every task must be completed.

### FR-008: Configuration File
The system shall read `keep-run.config.json` from the project root directory with the following structure:

```json
{
  "remote_enabled": true,
  "review_mode": "subagent",
  "clarify_prompt": "System prompt for LLM decision-making during requirement clarification",
  "review_fix_prompt": "System prompt for LLM decision-making during review fix cycles",
  "retry_policy": {
    "backoff": "exponential"
  }
}
```

The `review_mode` field controls how review phases are executed:
- `"subagent"` — Spawn a dedicated sub-agent to perform reviews, providing isolation between implementation and review context (default)
- `"direct"` — The main agent performs reviews directly, sharing the full implementation context

All fields shall have sensible defaults if the file is missing or fields are omitted:

| Field | Default | Description |
|-------|---------|-------------|
| `remote_enabled` | `true` | Most projects use a remote repository |
| `review_mode` | `"subagent"` | Isolated review context by default |
| `clarify_prompt` | `"Make decisions that prioritize correctness, simplicity, and alignment with project conventions."` | Guides LLM during requirement clarification |
| `review_fix_prompt` | `"Fix all issues, warnings, and suggestions. Prioritize correctness and code quality. Follow project constitution and TDD principles."` | Guides LLM during review fix cycles |
| `retry_policy.backoff` | `"exponential"` | Exponential backoff for all retry scenarios |

### FR-009: SDD Artifact Storage
Each task's SDD artifacts (spec.md, plan.md, tasks.md, and review reports) shall be stored under `.codexspec/specs/{task-slug}/` within the task's worktree.

### FR-010: Merge Prohibition (Strict)
The agent shall be strictly prohibited from merging any feature branch into the main branch. This applies under all circumstances:

- When `remote_enabled: true`: PRs are created for human review; the agent never merges them
- When `remote_enabled: false`: Feature branches remain separate; the agent never merges locally

Merging to main shall only be performed by humans after code review.

### FR-011: Slash Command Registration
`/keep-run` shall be registered as a TUI slash command within the existing slash command infrastructure (`internal/slash/`). It shall be invocable from the interactive TUI session.

### FR-012: Progress Reporting
The agent shall report progress during execution:
- Log which task is currently being processed
- Log which SDD phase is currently active
- Log phase transitions (e.g., "Moving from review-spec to spec-to-plan")
- Log task completion with a summary of artifacts produced
- Update `BACKLOG.md` status from `pending` to `done` upon task completion

## Non-Functional Requirements

### NFR-001: Reliability
The system shall not lose work in progress. If interrupted, re-running `/keep-run` shall correctly identify and resume from the next `pending` task at the first incomplete SDD phase (via the state file, FR-002). Completed phases shall not be re-executed.

### NFR-002: Isolation
Each task's code changes shall be fully isolated via git worktrees. No artifacts from a previous task shall leak into a subsequent task's worktree.

### NFR-003: Idempotency
Running `/keep-run` multiple times shall produce the same result: all `pending` tasks processed, all `done` tasks skipped. No duplicate work.

### NFR-004: Compatibility
The system shall work with:
- Projects using GitHub or GitLab for remote repository management
- Projects with no remote repository (local-only)
- Existing foxharness infrastructure (error recovery, system reminders, context compaction)

### NFR-005: Observability
The agent's progress shall be observable through:
- TUI console output showing current task and phase
- `BACKLOG.md` status updates
- Session transcript and tracing files

## Acceptance Criteria (Test Cases)

### TC-001: Basic pipeline execution
Given a `BACKLOG.md` with one `pending` task
When `/keep-run` is invoked
Then the agent shall create a worktree, execute all 12 SDD phases, commit, push (if enabled), and mark the task `done`

### TC-002: Multi-task sequential processing
Given a `BACKLOG.md` with three `pending` tasks
When `/keep-run` is invoked
Then the agent shall process all three tasks sequentially, each in its own worktree, and mark all three `done`

### TC-003: Resume after interruption
Given a `BACKLOG.md` where task 1 is `done` and task 2 is `pending`
When `/keep-run` is invoked
Then the agent shall skip task 1 and start processing task 2

### TC-003b: Phase-level resume after interruption
Given a `BACKLOG.md` with one `pending` task and an existing worktree with state file showing phases 1-6 completed
When `/keep-run` is invoked
Then the agent shall reuse the existing worktree and resume from phase 7, skipping phases 1-6

### TC-004: Worktree isolation
Given two tasks being processed sequentially
When task 1 completes and task 2 starts
Then task 2's worktree shall contain no artifacts from task 1

### TC-005: No remote operations when disabled
Given `keep-run.config.json` with `remote_enabled: false`
When `/keep-run` processes a task
Then the agent shall commit locally and skip push/issue/PR operations

### TC-006: Merge prohibition enforcement
Given any task completing the SDD pipeline
When the task is finished
Then the agent shall not merge the feature branch into main under any circumstance

### TC-007: Error self-healing — network failure
Given a network failure during an SDD phase
When the agent detects the failure
Then it shall retry with exponential backoff until the operation succeeds

### TC-008: Error self-healing — context window exceeded
Given a context window overflow during an SDD phase
When the agent detects the overflow
Then it shall compress the context and retry the operation

### TC-009: Config file defaults
Given no `keep-run.config.json` file exists
When `/keep-run` is invoked
Then the system shall use sensible defaults (e.g., `remote_enabled: true`, default prompts, default retry policy)

### TC-010: Exit condition
Given all tasks in `BACKLOG.md` are `done`
When `/keep-run` is invoked
Then the agent shall report "All tasks completed" and exit immediately

## Edge Cases

- **Empty BACKLOG.md**: Agent shall report "No tasks found" and exit
- **BACKLOG.md missing**: Agent shall report error and exit
- **All tasks already done**: Agent shall exit immediately (same as TC-010)
- **Task with invalid type prefix**: Agent shall still process the task (type is informational)
- **keep-run.config.json with missing fields**: Agent shall use defaults for missing fields
- **Worktree creation failure**: Agent shall retry with backoff (treated as a transient error)
- **Remote push rejected (e.g., branch already exists)**: Agent shall handle by creating a unique branch name and retrying
- **SDD phase produces empty output**: Agent shall retry the phase, treating it as an error condition
- **Agent runs out of API quota mid-task**: Agent shall wait for quota refresh and continue

## Out of Scope

- Parallel task processing (tasks are strictly sequential)
- Automatic backlog management (adding/removing tasks via agent)
- Integration with project management tools beyond GitHub/GitLab Issues
- Interactive mode for SDD phases during keep-run (all decisions are autonomous)
- Task prioritization or reordering (strict file-order processing)
- Automatic dependency resolution between tasks
