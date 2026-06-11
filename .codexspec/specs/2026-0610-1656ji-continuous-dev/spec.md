# Feature: Continuous Development / Backlog Autopilot (`autodev`)

## Overview

`autodev` is a continuous-development driver for foxharness. Given a backlog of
development requirements (default `BACKLOG.md`), it autonomously drives the
CodexSpec **SDD + TDD** workflow for each requirement — one at a time — inside an
isolated git branch and worktree. When a requirement passes its completion gate,
`autodev` commits, pushes, opens a linked GitHub issue + PR, records progress in a
durable state ledger, then advances to the next requirement by priority until the
backlog is drained.

The defining design principle is a **two-plane architecture**:

- **Control plane (Go, deterministic).** A Go state machine owns *flow control only* and
  performs **no** development action: parsing/selecting backlog items, worktree lifecycle
  (its only repo writes), **driving the ordered sequence of steps and launching the next
  one**, **read-only** ground-truth verification that a step actually completed, the
  state ledger, and cleanup. Flow is driven by code — **not** by an LLM "remembering"
  what to do next — which is what prevents a step from being silently skipped, prematurely
  ended, or stalled (the failure we hit with skill-driven pipelines: stopping after one
  step instead of continuing to the next).
- **Execution plane (LLM, agentic).** The **core Agent** (the existing engine loop, scoped
  to the item's worktree) performs **all** the work and **all** repo actions — implement,
  stage, commit, `git push`, open the issue and the PR — via its own `bash`/`gh` tools and
  the CodexSpec skills. An **engineer Agent** (an LLM in a senior-engineer persona)
  supervises each core run the way a human user would: reviewing the result and steering
  corrections.

The two planes mesh through the existing `ask_user_question` tool: in `autodev`
mode the human asker is replaced by an `EngineerAsker` that answers the core
Agent's questions on behalf of the simulated engineer.

## Goals

- Drain a backlog of requirements with zero human intervention, one requirement
  per isolated branch + worktree, ending each with a pushed branch + linked
  issue/PR.
- Guarantee **deterministic, non-skippable** progression through the SDD stages by
  controlling flow in Go rather than in a self-driving skill — the primary
  reliability requirement.
- Keep the actual engineering intelligent and adaptive by having an engineer-role
  LLM converse with the core LLM at every decision point.
- Make the entire run **fully observable** — every simulated-user message, core LLM
  output, and tool/git/gh action is streamed to the terminal (CLI) or session area
  (TUI).
- Remain **constitution-compliant**: the Go orchestrator is TDD-testable, the
  default engineer persona embodies the constitution's values, and "PR without
  auto-merge" preserves the human/CI code-review gate.

## User Stories

### Story 1: Drain a backlog unattended

**As a** maintainer of foxharness
**I want** to point `autodev` at a backlog and walk away
**So that** every pending requirement is developed, verified, pushed, and turned
into a linked issue + PR without me babysitting each SDD step.

**Acceptance Criteria:**
- [ ] Running the entry point processes pending items by priority until none remain.
- [ ] Each item is developed in its own branch + isolated worktree.
- [ ] Each completed item results in a pushed branch, a GitHub issue, and a PR whose
      body links the issue (`Closes #N`).
- [ ] No human approval or input is required during the run.

### Story 2: Reliable, non-skippable SDD progression

**As a** maintainer
**I want** the SDD stages to advance only when each stage's output truly exists
**So that** the loop can never silently skip a stage or stop halfway the way a
self-driving skill might.

**Acceptance Criteria:**
- [ ] A stage advances only when a Go-evaluated done-condition is satisfied.
- [ ] If the core LLM claims completion but the artifact is absent, the loop does
      not advance and continues the engineer↔core dialogue.
- [ ] The loop cannot reach commit/push/PR until the completion gate is green.

### Story 3: Engineer Agent answers the core Agent

**As a** maintainer
**I want** an engineer-role LLM to analyze and answer the core LLM's questions
**So that** interactive SDD commands (which normally wait on a human) run to
completion unattended, with sensible engineering decisions.

**Acceptance Criteria:**
- [ ] When the core LLM calls `ask_user_question`, the engineer Agent supplies the
      answer (a valid option label, multi-selection, or "Other" free text).
- [ ] When the core LLM ends a turn with an open prose question, the engineer
      Agent's reply is fed back as the next user message.
- [ ] The engineer and core Agents use the same model; the engineer persona is
      configurable, defaulting to a senior engineer who prioritizes stability,
      consistent code style, testability, and readability.
- [ ] After each core run, the engineer Agent reviews the result (incl. tool errors) and,
      when a step's Go `Verify` hasn't passed, returns a corrective instruction (e.g.
      stage-and-retry on "nothing to commit").

### Story 4: Resume after interruption

**As a** maintainer
**I want** progress recorded durably
**So that** an interrupted run resumes without redoing finished items or losing
its place — even though PRs are not auto-merged and `BACKLOG.md` on `main` is not
updated immediately.

**Acceptance Criteria:**
- [ ] A committed state ledger records per-item status, branch, issue#, PR#, and the
      current SDD stage.
- [ ] On restart, done items are skipped and in-progress items resume from their
      recorded stage.

### Story 5: Two observable entry points

**As a** maintainer
**I want** both a terminal subcommand and a TUI command
**So that** I can launch `autodev` in whichever surface I'm using and watch the
whole interaction live.

**Acceptance Criteria:**
- [ ] `fox autodev [backlog-path]` runs in the terminal and streams the full
      interaction to stdout.
- [ ] A TUI built-in command `/autodev` renders the same interaction in the session
      area.
- [ ] Both share one orchestrator, differing only in the reporter implementation.

## Functional Requirements

### Backlog parsing & selection
- **REQ-001** Parse a backlog file (default `BACKLOG.md`, configurable via
  `backlog_file`) into ordered items, each with: type, title, `Priority`
  (`high`/`medium`/`low`), `Status` (`pending`/`in-progress`/`done`), and a free-text
  `Description`. The backlog `Status` field is advisory/initial only; the ledger
  (REQ-021) is the authoritative processing status (see REQ-028).
- **REQ-002** Select items whose authoritative status is `pending` (from the ledger;
  see REQ-021 and REQ-028), ordered by `Priority` `high → low`, breaking ties by
  document order.
- **REQ-003** Process items **strictly serially**: an item must complete fully
  (through PR creation and ledger update) before the next item begins. Only then is
  a new branch + worktree created.
- **REQ-004** The loop terminates when no selectable `pending` items remain.

### Git isolation (branch + worktree)
- **REQ-005** For each item, create a new git branch named `auto/<item-slug>` and an
  isolated git worktree under a **sibling directory** of the repository (default
  `../<repo>-worktrees/<item-slug>`, configurable via `worktree_dir`). `<item-slug>`
  is a kebab-case slug derived from the item title.
- **REQ-006** After a successful PR, remove the local worktree while retaining the
  remote branch and PR. If an item does not complete, retain its worktree for
  inspection.

### Two-plane orchestration
- **REQ-007** A deterministic Go control plane sequences the steps. A step advances to
  the next **only when its Go-evaluated `Verify`** (REQ-029) is satisfied, and Go then
  **drives** the next step. The LLM cannot cause advancement, skipping, or early
  termination; a bare claim of "done" by the LLM is not trusted.
- **REQ-008** Within each stage, the engineer Agent (LLM + persona) interacts with
  the core Agent (the existing engine loop, scoped to the item's worktree) to produce
  the stage's artifacts.

### SDD pipeline (lean)
- **REQ-009** Per item, run the lean SDD pipeline in fixed order:
  `generate-spec → spec-to-plan → plan-to-tasks → implement-tasks`. The corresponding
  CodexSpec command bodies are reused as the **prompt source** for each stage and are
  invoked programmatically by the control plane — never as a single self-driving
  meta-skill. (`clarify`, `analyze`, and the `review-*` commands are skipped in v1.)
- **REQ-010** The backlog item's `Description` is supplied as the already-clarified
  requirement input to `generate-spec` (`specify`/`clarify` are intentionally skipped).
- **REQ-011** The control plane detects the spec directory created by `generate-spec`
  (via a before/after directory diff under `.codexspec/specs/`) and binds it to the
  item, threading it through `spec-to-plan`, `plan-to-tasks`, and `implement-tasks`.
- **REQ-012** Stage `Verify` conditions:
  - `generate-spec`: a non-empty `spec.md` exists in the bound spec directory.
  - `spec-to-plan`: `plan.md` exists.
  - `plan-to-tasks`: `tasks.md` exists.
  - `implement-tasks`: all tasks are completed, the completion gate (REQ-018) is green,
    **and** the worktree diff is non-empty (real changes were produced; see REQ-029).

### Engineer ↔ core interaction channels
- **REQ-013** *Structured channel.* The control plane installs an `EngineerAsker`
  implementing `tools.UserAsker` onto the core Agent's runner via
  `AgentRunner.SetUserAsker`, so the merged `ask_user_question` tool is answered by
  the engineer Agent (selecting an option `Label`, multiple labels, or "Other" free
  text) instead of a human. Because an asker is present, the tool is registered and
  the `WithInteractiveAsk` prompt guidance steers the core LLM to prefer it.
- **REQ-014** *Result-review & correction channel.* After every core Agent run, the
  control plane routes the run's outcome — its final message **and any surfaced tool
  errors**, together with the Go-computed verification gap (REQ-029) — to the engineer
  Agent, which either approves (advance) or returns a corrective instruction fed back as
  the next user message. This is the engineer supervising the core Agent the way a human
  user would (e.g. on a `commit-staged` "nothing to commit", it tells the core Agent to
  `git add` and retry). It also covers any open prose question the core Agent ends a turn
  with.
- **REQ-015** *Stage-driving channel.* The control plane injects each stage's command
  prompt as the initiating user message for that stage.
- **REQ-016** The engineer and core Agents use the **same model**. The engineer
  persona is configurable (`engineer_prompt` inline or `engineer_prompt_file` path);
  when unset, a default persona is used: a senior engineer who prioritizes system
  stability, code-style consistency, testability, and readability. The engineer Agent
  is read-only with respect to the workspace (it may read artifacts to inform answers
  but performs no writes).

### Autonomy & approvals
- **REQ-017** The run is fully autonomous: the engineer Agent answers all
  clarifications and confirmations, and tool-permission approvals are auto-granted by
  policy in `autodev` mode so no human-approval prompt can block the loop.

### Completion gate
- **REQ-018** Before pushing, an item must pass the completion gate: `go build ./...`
  succeeds, `go test ./...` passes, and `gofmt -l .` produces no output. These gates run
  inside the item's worktree. **Gate floor (constitution-safe):** the `test` gate is
  mandatory and cannot be disabled; `build` and `gofmt` may be toggled via `gates`, but
  disabling any gate emits a prominent warning. All gates default to on, per the
  constitution's TDD/quality mandate.

### Remote integration
- **REQ-019** After the completion gate passes, the **core Agent** performs, in fixed
  order, all of: (1) **stage** changes (`git add`); (2) **commit** via the
  `/codexspec:commit-staged` skill (authoring a Conventional Commit message over the
  staged changes); (3) **push** the branch to the remote (`git push`); (4) **create a
  GitHub issue** (`gh issue create`) **before** the PR; (5) **open a PR** via the
  `/codexspec:pr` skill (`gh pr create`) whose body links the issue with `Closes #N`. All
  five are core-Agent actions through its `bash`/`gh` tools — **the control plane runs no
  git/gh mutation itself**. After each, the control plane **read-only**-verifies the
  ground truth (REQ-029: commit→HEAD advanced; push→`git ls-remote`; issue/PR→`gh … --json
  number`), steers engineer corrections on failure (REQ-014/030), then **drives the next
  step**; finally (6) Go **records** issue#, PR#, branch, and `status=done` in the ledger
  from the verified ground truth.
- **REQ-020** No auto-merge. Merging the PR remains a human/CI responsibility,
  preserving the constitution's code-review gate.

### State ledger & resumability
- **REQ-021** Maintain a state ledger at `.foxharness/autodev-state.json`, committed to
  the base branch (`main`), as the **authoritative** progress source. Per item it
  records: status (`pending`/`in-progress`/`done`), branch, issue#, PR#, and current
  SDD stage. It is authoritative even though `BACKLOG.md` on `main` is not updated
  immediately (because PRs are not auto-merged).
- **REQ-022** The loop is resumable: on restart, `done` items are skipped and
  `in-progress` items resume from their recorded stage.
- **REQ-028** Ledger seeding & precedence: when a backlog item is absent from the
  ledger (first run, or a newly added backlog entry), seed a ledger entry for it with
  status `pending`. Thereafter the ledger status is authoritative for selection and the
  backlog `Status` field is advisory only and never overrides the ledger. Division of
  truth: the backlog supplies the item set, the ordering input (`Priority`), and the
  `Description`; the ledger supplies the processing status, branch, issue#, PR#, and
  current stage.

### Configuration
- **REQ-023** Read configuration from `.foxharness/autodev.yml`, supporting at least:
  `backlog_file`, `worktree_dir`, `base_branch`, `remote`, `concurrency`, `model`,
  `engineer_prompt`/`engineer_prompt_file`, `pipeline`, `gates`, `remote_flow`. The
  file is optional; sensible defaults apply when it or any key is absent.

### Entry points & observability
- **REQ-024** Provide a CLI subcommand `fox autodev [backlog-path]` that runs in the
  terminal and streams the entire interaction — the engineer Agent's simulated-user
  messages, core LLM output, and all tool/git/gh operations — to the terminal.
- **REQ-025** Provide a TUI built-in command `/autodev` that renders the same
  interaction in the conversation/session area.
- **REQ-026** Both entry points construct the **same** orchestrator, differing only in
  the reporter implementation (`TerminalReporter` vs `TUIReporter`). The full
  engineer↔core dialogue and all control-plane actions are observable in both.

### Failure philosophy
- **REQ-027** No abandonment budget and no failure state machine. The loop relies on
  the engineer↔core dialogue plus the Go ground-truth verification (REQ-029) to converge
  each step. (This is an explicit product constraint: trust the SDD process; do not
  over-engineer failure scenarios.) Genuine *preconditions* (e.g., remote/`gh`
  availability) are validated up front (see Edge Cases), which is distinct from
  speculative in-loop failure handling.

### Per-step verification & supervised recovery
- **REQ-029** Per-step ground-truth verification (the hard backstop). Every step — each
  SDD stage **and** each remote step (commit/push/issue/PR) — has a deterministic
  `Verify` predicate that **read-only**-observes ground-truth state (filesystem / `git` /
  `gh` query commands; the control plane performs no mutation here), never the LLM's
  self-report, plus a `VerifyGap` describing precisely what is still missing. A step
  advances **only** when its `Verify` passes; if the engineer Agent approves but `Verify`
  still fails, the control plane does not advance. Concrete signals: artifact file present
  & non-empty (generate-spec/plan/tasks); gates green **and** non-empty worktree diff
  (implement); **HEAD advanced + clean worktree** (commit-staged); remote tip == local tip
  via `git ls-remote` (push); issue/PR present via `gh … --json` (issue/PR).
- **REQ-030** Unified per-step supervised loop, **driven by Go**. Go owns the ordered
  outer loop over all steps; for each step: core Agent run → Go **read-only** `Verify`
  (REQ-029); if passed, **Go launches the next step** — the LLM never decides the pipeline
  is "done" and stops; Go drives continuation, the exact failure of skill-driven pipelines
  that halt after, e.g., `spec-to-plan` without continuing to `plan-to-tasks`. If `Verify`
  fails, the control plane computes `VerifyGap` and hands it to the engineer Agent
  (REQ-014), whose corrective instruction drives the next core run — repeating until
  `Verify` passes. Every step (incl. commit/push/issue/PR) is a core Agent run; the control
  plane performs no development action. No abandonment budget (REQ-027); the genuine
  zero-change case is fenced upstream by the implement `Verify` (non-empty diff), so the
  commit loop always has real work and converges. A subagent completion-checker is
  explicitly **not** used (it would re-introduce an LLM as the gate).

## Non-Functional Requirements

- **NFR-001 — Determinism / reliability.** Flow control is deterministic Go. Stage
  progression is impossible to skip or prematurely end via the LLM. This is the
  primary motivation for choosing a Go orchestrator over an integrated skill.
- **NFR-002 — Testability (constitution).** All control-plane components
  (backlog store, worktree manager, stage machine, gate runner, remote publisher,
  ledger) are interfaces, unit-testable with fakes. `CoreSession` and `EngineerAgent`
  are interfaces with injectable deterministic stubs, enabling pure-Go tests of stage
  progression, the "claimed-done but artifact-absent" guard, and the question→answer
  loop. The feature itself is developed test-first.
- **NFR-003 — Isolation.** Each item runs in a separate worktree, branch, and engine
  session with no cross-item interference. The project-level `MEMORY.md` is shared
  read-only.
- **NFR-004 — Observability.** The full interaction is streamed to terminal/TUI via a
  pluggable reporter; nothing material happens silently.
- **NFR-005 — Documentation standards (constitution).** All new Go uses block-level
  doc comments on exported identifiers; no teaching line comments.
- **NFR-006 — Security.** Relies on the user's existing git/`gh` authentication; no
  secrets are stored in `autodev.yml`; `gh` operations use the authenticated CLI.
- **NFR-007 — Performance.** Serial execution; per-item wall-clock is dominated by LLM
  calls. Worktree create/remove complete in < 5s each (excluding network); git/`gh`
  operations are bounded by network latency and are not otherwise rate-limited by the
  orchestrator.

## Constitution Alignment

- **TDD** — the Go orchestrator is fully unit-testable (NFR-002); a skill-based
  orchestrator would not be. Implementation follows Red→Green→Refactor.
- **Code review** — opening a PR without auto-merge (REQ-020) keeps the mandated human/
  CI review gate intact.
- **Documentation** — block-level Go doc comments only (NFR-005).
- **Default engineer persona** — stability, consistent style, testability, readability
  (REQ-016) directly mirror constitutional principles, so the simulated engineer makes
  constitution-aligned decisions.

## Acceptance Criteria (Test Cases)

- **TC-001** Backlog parser extracts items with correct type/title/priority/status/
  description from a representative `BACKLOG.md`.
- **TC-002** Selection orders items by priority `high→low`, stable by document order,
  and excludes non-`pending` items.
- **TC-003** Given an item marked `done` in the ledger, selection skips it (resumability).
- **TC-004** `WorktreeManager.Create` produces a worktree in the configured sibling dir
  on branch `auto/<slug>`; `Remove` deletes the worktree while the remote branch remains.
- **TC-005** With a fake `CoreSession` that never emits the stage artifact, the stage
  machine does **not** advance and keeps routing questions to the engineer.
- **TC-006** With a fake `CoreSession` that claims "done" while the artifact is absent,
  the stage machine does **not** advance (LLM "done" is not trusted).
- **TC-007** `EngineerAsker.Ask` routes `[]Question` to the `EngineerAgent` stub and
  returns `[]Answer` whose `QuestionText` matches and `Value` is a valid option label.
- **TC-008** Installing `EngineerAsker` via `SetUserAsker` causes `ask_user_question` to
  be registered and answered with no human present.
- **TC-009** Free-form fallback: a core turn ending with a prose question and no tool
  call yields an engineer reply that becomes the next user message.
- **TC-010** When `go test ./...` fails, the control plane does **not** proceed to
  commit/push (gate enforced).
- **TC-011** When the gate is green, the remote sequence executes in order:
  add → commit-staged → push → issue → PR.
- **TC-012** The created PR body contains `Closes #<issue-number>` linking the issue
  created in the prior step.
- **TC-013** After a successful PR, the ledger entry is `done` with branch, issue#, PR#.
- **TC-014** The local worktree is removed after a successful PR; the remote branch is
  retained (assert via `git worktree list` / `git ls-remote`).
- **TC-015** Config loader reads `.foxharness/autodev.yml`; a missing file applies
  defaults; `engineer_prompt_file` overrides the default persona.
- **TC-016** `fox autodev` delivers Core/Engineer/Git events to a `TerminalReporter`.
- **TC-017** `/autodev` routes the same events to a `TUIReporter` rendered in the session
  area.
- **TC-018** With multiple pending items, item N+1 starts only after item N's ledger entry
  is `done` (strict serialization); the loop exits when none remain.
- **TC-019** Spec-directory binding: `generate-spec` creates a new dir, the control plane
  binds it, and subsequent stages write `plan.md`/`tasks.md` into the same dir.
- **TC-020** Under the autodev approval policy, a `bash`/`write` tool call from the core
  Agent proceeds without any human-approval prompt (REQ-017).
- **TC-021** The remote sequence calls `gh pr create` and never issues any merge command
  (REQ-020 — no auto-merge).
- **TC-022** The engineer and core runners both resolve to the same configured model
  (REQ-016 — same model).
- **TC-023** Ledger seeding: an item present in the backlog but absent from the ledger is
  seeded as `pending`; a backlog `Status` value never overrides an existing ledger entry
  (REQ-028).
- **TC-024** On a `commit-staged` "nothing to commit" (changes left unstaged), the engineer
  Agent instructs the core Agent to `git add` and retry; the commit then succeeds and Go's
  `Verify` (HEAD advanced + clean tree) passes (REQ-014/029/030).
- **TC-025** When the engineer Agent approves but Go `Verify` fails (e.g. HEAD did not
  advance), the control plane does **not** advance to the next step (REQ-029).
- **TC-026** `implement-tasks` `Verify` fails when the worktree diff is empty even though
  the gates are green (REQ-012/029).

## Edge Cases

- **Empty backlog / no pending items** → the loop is a clean no-op and exits.
- **Malformed item** → missing `Status` defaults to `pending`; missing `Priority`
  defaults to the lowest bucket; document order breaks ties.
- **Item-slug collision** (two items with the same title) → disambiguate the slug
  (append a short index/hash) so branch and worktree paths stay unique.
- **Leftover worktree / existing branch from a prior interrupted run** → if the ledger
  shows the item `in-progress`, resume on the existing branch/worktree; otherwise create
  a uniquely-suffixed path.
- **`gh` not installed or not authenticated; remote unreachable** → validated as a
  **precondition at startup**, failing fast with a clear message before any item is
  processed (a precondition check, not in-loop failure handling).
- **Issue created but PR creation interrupted** → the ledger records the issue#; on
  resume, the recorded issue is reused rather than creating a duplicate.
- **Core LLM asks a question whose options don't fit** → the engineer Agent answers via
  the auto-appended "Other" free-text entry.
- **`ask_user_question` cancellation** → the `EngineerAsker` never cancels; if the
  engineer cannot otherwise decide, it selects the recommended/first option.
- **Run interrupted (Ctrl-C / crash) mid-item** → the ledger's `in-progress` + stage
  drives resume; the worktree is retained.

## Output Examples

### `.foxharness/autodev.yml`
```yaml
backlog_file: BACKLOG.md                  # requirements list (rename/path configurable)
worktree_dir: ../foxharness-go-worktrees  # sibling dir, one isolated worktree per item
base_branch: main
remote: origin
concurrency: serial                       # v1 serial; parallel reserved

model: ""                                 # empty = global default; engineer & core share it
engineer_prompt_file: ""                  # custom engineer persona (.md); empty = default

pipeline: lean                            # generate-spec → spec-to-plan → plan-to-tasks → implement-tasks
gates: { build: true, test: true, gofmt: true }

remote_flow:
  create_issue: true                      # create issue before the PR
  open_pr: true                           # /codexspec:pr + gh pr create
  link_issue: true                        # PR body: Closes #N
  auto_merge: false                       # never auto-merge
```

### `.foxharness/autodev-state.json`
```json
{
  "items": [
    {
      "slug": "engine-writes-durable-discoveries-to-memory-md-during-runs",
      "title": "Engine writes durable discoveries to MEMORY.md during runs",
      "priority": "high",
      "status": "done",
      "branch": "auto/engine-writes-durable-discoveries-to-memory-md-during-runs",
      "stage": "implement-tasks",
      "issue": 31,
      "pr": 32,
      "spec_dir": ".codexspec/specs/2026-0610-1702xx-engine-memory-writes"
    }
  ]
}
```

### Terminal stream (CLI `fox autodev`)
```
[autodev] item 1/1  high  engine-writes-durable-discoveries-...
[autodev] worktree  ../foxharness-go-worktrees/engine-writes-...  branch auto/engine-writes-...
[stage] generate-spec
  core   → calls ask_user_question: "Where should discoveries be appended?"
  engineer → "Append under a '## Discoveries' section in MEMORY.md (Other)"
  core   → wrote .codexspec/specs/.../spec.md
[stage] generate-spec  DONE (spec.md present)
...
[stage] implement-tasks  gate: build ✓  test ✓  gofmt ✓  DONE
[remote] git add → commit-staged → push → issue #31 → PR #32 (Closes #31)
[ledger] engine-writes-... = done (issue #31, pr #32)
[autodev] backlog drained.
```

### PR body excerpt
```
## Summary
Implements MEMORY.md durable-discovery writes ...

Closes #31
```

## Out of Scope

- **Parallel worktrees / concurrent items** (v1 is strictly serial; parallelism is
  reserved for a future version).
- **Auto-merging PRs** (merge stays human/CI-gated).
- **Cross-repository operation.**
- **Redesigning the session-level `working_memory.md`** (unchanged).
- **The full SDD `clarify` / `analyze` / `review-*` steps in the loop** (lean pipeline
  only in v1).
- **Rewriting `BACKLOG.md` status on `main`** (the ledger is authoritative; direct
  backlog-status writeback to `main` is not required in v1).
