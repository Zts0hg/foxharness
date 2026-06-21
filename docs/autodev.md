# Autodev — Continuous Development / Backlog Autopilot

`autodev` drains a backlog of development requirements with zero human
intervention. For each pending item it drives the CodexSpec SDD + TDD
workflow inside an isolated git branch and worktree, then commits, pushes,
opens a linked GitHub issue + PR, records progress in a durable ledger, and
moves on to the next item by priority until the backlog is empty.

## Entry points

```bash
# Terminal: stream the full interaction to stdout
fox autodev                 # uses backlog_file from .foxharness/autodev.yml (default BACKLOG.md)
fox autodev WORK.md         # override the backlog path
fox autodev -C /path/repo   # run against another repository
```

Inside the TUI, the builtin command `/autodev [backlog-path]` runs the same
orchestrator and renders the identical event stream in the session area.
Both surfaces share one orchestrator and differ only in the reporter.

Exit codes (CLI): `0` backlog drained · `2` precondition failure (not a git
repository, or `gh` missing/unauthenticated) · `1` unexpected error.

## Two-plane architecture

- **Control plane (Go, deterministic).** A Go state machine owns flow
  control only: backlog parsing and selection, worktree lifecycle, driving
  the ordered steps, **read-only** ground-truth verification per step, the
  state ledger, and cleanup. A step advances only when its Go-evaluated
  `Verify` predicate observes the artifact on disk / in git / via `gh` —
  the LLM's claim of "done" is never trusted, and the LLM can never skip a
  step or stop the pipeline early.
- **Execution plane (LLM).** The core Agent (the normal foxharness engine,
  scoped to the item's worktree) performs all development work and all repo
  mutations — implement, `git add`, commit, `git push`, `gh issue create`,
  `gh pr create`. An engineer Agent (an LLM with a senior-engineer persona,
  same model as the core Agent) supervises each run: it answers the core
  Agent's `ask_user_question` calls and, whenever verification finds a gap,
  feeds a corrective instruction back as the next user message.

## The fixed requirements-first pipeline

Per item, the stages run in fixed order with these completion checks:

| Step | Driven by | Go `Verify` (ground truth) |
|------|-----------|----------------------------|
| `materialize-requirements` | Go control plane | a CodexSpec feature workspace with confirmed `requirements.md` |
| `generate-spec` | `/codexspec:generate-spec <feature-dir>/requirements.md` | non-empty `spec.md` and `review-spec.md` with `PASS` or `PASS_WITH_WARNINGS` |
| `spec-to-plan` | `/codexspec:spec-to-plan <feature-dir>/spec.md` | non-empty `plan.md` and `review-plan.md` with `PASS` or `PASS_WITH_WARNINGS` |
| `plan-to-tasks` | `/codexspec:plan-to-tasks <feature-dir>/plan.md` | non-empty `tasks.md` and `review-tasks.md` with `PASS` or `PASS_WITH_WARNINGS` |
| `implement-tasks` | `/codexspec:implement-tasks <feature-dir>/tasks.md` | all task checkboxes complete, gates green **and** non-empty worktree diff |
| stage → commit | `git add` + `/codexspec:commit-staged` | HEAD advanced + clean worktree |
| push | `git push -u <remote> <branch>` | `git ls-remote` tip == local tip |
| issue | `gh issue create` | issue found via `gh issue list --json` |
| PR | `/codexspec:pr` + `gh pr create` | PR found via `gh pr view --json`, body contains `Closes #N` |

Autodev treats each backlog item as the confirmed user input for unattended
development. The control plane creates `.codexspec/specs/YYYY-MMDD-HHMMxx-<slug>/requirements.md`
directly and does not call `/codexspec:specify`, avoiding interactive
confirmation and CodexSpec branch switching inside the isolated `auto/<slug>`
worktree.

The completion gate runs inside the worktree: `go build ./...`,
`go test ./...`, `gofmt -l .`. The **test gate cannot be disabled**; build
and gofmt may be toggled but disabling them warns prominently. PRs are
**never auto-merged** — review stays with humans/CI.

## Configuration — `.foxharness/autodev.yml`

The file is optional; every key has a sensible default.

```yaml
backlog_file: BACKLOG.md                  # requirements list (path configurable)
worktree_dir: ../foxharness-go-worktrees  # sibling dir, one isolated worktree per item
base_branch: main
remote: origin
concurrency: serial                       # v1 serial; parallel reserved

model: ""                                 # empty = global default; engineer & core share it
engineer_prompt: ""                       # inline engineer persona; empty = default
engineer_prompt_file: ""                  # custom engineer persona (.md); empty = default

gates: { build: true, test: true, gofmt: true }   # test is mandatory (cannot be disabled)

remote_flow:
  create_issue: true                      # create issue before the PR
  open_pr: true                           # /codexspec:pr + gh pr create
  link_issue: true                        # PR body: Closes #N
  auto_merge: false                       # never auto-merge (not supported)
```

## Backlog format — `BACKLOG.md`

```markdown
# Backlog

## [feature] Engine writes durable discoveries to MEMORY.md during runs

**Priority**: high
**Status**: pending
**Description**: During an agent run, the Engine should ... (free text; this
is treated as confirmed input for the generated requirements.md)
```

- `Priority`: `high` / `medium` / `low` (missing → lowest bucket).
- `Status` in the backlog is advisory/initial only — the ledger is the
  authoritative processing status. Editing it never overrides recorded
  progress.

## State ledger — `.foxharness/autodev-state.json`

The ledger is the authoritative progress source. Per item it records the
slug, status (`pending`/`in-progress`/`done`), branch, current stage, issue
number, PR number, and the bound CodexSpec feature directory. On restart,
`done` items are skipped and `in-progress` items resume from their recorded
stage on their existing branch/worktree; remote steps that already happened
(commit, push, recorded issue) are detected from ground truth and skipped.

## Preconditions

Before any item is processed, autodev validates that the workdir is a git
repository and that `gh` is installed and authenticated (`gh auth status`),
failing fast with exit code 2 otherwise. It relies on your existing git and
`gh` credentials; nothing is stored in `autodev.yml`.

## Observability

Everything is streamed: the engineer Agent's simulated-user answers and
corrections, the core LLM's output, every tool/git/gh call, per-step verify
results, gate outcomes, and ledger transitions. Nothing material happens
silently — a stage that is not converging is visible live in the stream.
