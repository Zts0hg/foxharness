# Implementation Plan: Continuous Development / Backlog Autopilot (`autodev`)

> Spec: `.codexspec/specs/2026-0610-1656ji-continuous-dev/spec.md`
> This plan defines **how** the two-plane orchestrator is built on the existing
> foxharness Go codebase. New code is isolated in a new `internal/autodev` package
> plus thin entry points; it reuses `internal/engine`, `internal/app`,
> `internal/slash`, `internal/tools`, `internal/provider`, and `internal/tui`.

## 1. Tech Stack

| Category | Technology | Version | Notes |
|----------|------------|---------|-------|
| Language | Go | 1.25.0 (`go.mod`) | No new language/runtime. |
| Config format | YAML | `gopkg.in/yaml.v3` v3.0.1 (already in `go.mod`, used by `internal/slash`) | `.foxharness/autodev.yml`. |
| State format | JSON | stdlib `encoding/json` | `.foxharness/autodev-state.json` ledger. |
| Process exec | stdlib `os/exec` | — | `git` and `gh` invocations. |
| Core agent | `internal/app.AgentRunner` + `internal/engine` | existing | Per-item core loop via `AgentRunner.Run`. |
| Ask seam | `internal/tools.UserAsker` + `ask_user_question` | merged (PR #25) | Answered by `EngineerAsker`. |
| LLM | `internal/provider.LLMProvider` | existing | Same provider/model for engineer & core. |
| Slash | `internal/slash` Registry/Executor | existing | Materialize codexspec command bodies as stage prompts. |
| Testing | stdlib `testing` + table-driven | existing convention | Fakes for LLM/git/gh; no network in unit tests. |

**No new third-party dependencies.** Everything is stdlib + packages already in the tree.

## 2. Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| 1. Test-Driven Development | ✅ | Phases are Red→Green→Refactor. Every control-plane module is an interface unit-tested with fakes; `CoreRunner`, `EngineerAgent`, git/`gh`, and gates are injected so the orchestrator is deterministically testable (no LLM/network in unit tests). |
| 2. Code Quality | ✅ | Small, single-purpose modules; dependency injection via factories/interfaces; the two planes keep concerns separated. |
| 3. Go Documentation Standards | ✅ | Package `doc.go` for `internal/autodev`; block-level comments on every exported identifier; no teaching line comments. |
| 4. Testing Standards | ✅ | Tests mirror package structure (`*_test.go` per file); table-driven; deterministic via fakes; error paths (precondition failures, gate-red, resume) covered. |
| 5. Architecture | ✅ | `internal/autodev` depends on **abstractions** (`CoreRunner`/`CoreRunnerFactory`, `EngineerAgent`, `GitRunner`); `internal/app` supplies the concrete adapter — mirroring the existing `slash` ↔ `ForkRunner` delegation and avoiding an `app ↔ autodev` import cycle. |
| 6. Performance | ✅ | Serial by design; worktree create/remove < 5s (NFR-007); LLM/network dominate and are out of our control; no premature optimization. |
| 7. Security | ✅ | No secrets in `autodev.yml`; relies on the user's existing `git`/`gh` auth; all external inputs (backlog file, config, ledger) are validated at parse time; `gh`/`git` invoked with explicit args (no shell string interpolation of untrusted data). |

No principle conflicts. The plan strengthens TDD relative to a skill-based approach (which would be untestable).

## 3. Architecture Overview

```
                      ┌──────────────── Entry Points (thin) ─────────────────┐
   fox autodev ──▶ app.RunAutodev ─┐                  ┌─ tui /autodev (builtin) ─▶ TUIReporter
                                    │                  │        (channel → tea.Msg bridge)
                                    ▼                  ▼
                         ┌──────────────────────────────────────────┐
                         │      internal/autodev.Orchestrator        │  ← CONTROL PLANE (Go, deterministic)
                         │  outer loop: select pending by priority   │
                         │  per item: worktree → stages → gate →     │
                         │            commit → push → issue → PR →    │
                         │            ledger → cleanup → next         │
                         └───┬───────┬────────┬────────┬───────┬──────┘
              ┌──────────────┘       │        │        │       └──────────────┐
              ▼                      ▼        ▼        ▼                       ▼
       BacklogStore +          WorktreeMgr  StageMachine  GateRunner     RemotePublisher
       Ledger (seed/         (git worktree) (per-item     (build/test/   (add→commit-staged
       precedence, JSON)                     SDD stages)   gofmt)         →push→issue→PR)
                                                  │
                                                  │ drives, per stage
                                                  ▼
                    ┌───────────────────────────────────────────────────┐
                    │   EXECUTION PLANE (LLM)                            │
                    │   CoreRunner (adapter over app.AgentRunner,        │
                    │     WorkDir = item worktree)  ── Run(prompt) ─▶     │
                    │        ▲ ask_user_question                         │
                    │        │ (tools.UserAsker)                         │
                    │   EngineerAsker ─▶ EngineerAgent (same model +     │
                    │                     persona, structured + free)    │
                    └───────────────────────────────────────────────────┘
```

**Flow control — sequencing and advancement — lives entirely in `Orchestrator`/`StageMachine` (Go), which performs no development action.** The core LLM does all the work and all repo mutations (implement, stage, commit, `git push`, `gh` issue/PR); Go only **read-only**-verifies each step's ground truth and then **drives the next step** so the pipeline can't stop early. A step advances only when a Go `Verify` predicate holds (artifact present / gates green / HEAD advanced / remote tip / `gh --json`). Because `AgentRunner.Run` is **run-to-completion**
(not a step/yield API), the engineer↔core exchange is a sequence of `Run` calls —
questions are answered *mid-`Run`* via `EngineerAsker`, and the engineer Agent
**reviews each run's result between `Run`s**, turning the Go-computed `VerifyGap` into a
correction whenever the ground-truth `Verify` hasn't passed (see §6 `stage.go`).

## 4. Component Structure

```
internal/autodev/                 # NEW — control plane (no import of internal/app)
├── doc.go                        # package doc
├── config.go        config_test.go      # AutodevConfig + load .foxharness/autodev.yml + defaults
├── backlog.go       backlog_test.go     # BacklogStore: parse backlog → []Item
├── slug.go          slug_test.go        # title → kebab slug + collision disambiguation
├── ledger.go        ledger_test.go      # Ledger: JSON load/save, seeding (REQ-028), status updates
├── item.go                              # Item / Status / Priority value types
├── ports.go                             # interfaces: CoreRunner, CoreRunnerFactory, EngineerAgent, GitRunner, Clock
├── engineer.go      engineer_test.go    # EngineerAgent (provider-backed) + EngineerAsker (tools.UserAsker)
├── stage.go         stage_test.go       # Stage defs + StageMachine (engineer↔core loop, Done-gating)
├── gate.go          gate_test.go        # GateRunner: go build/test + gofmt in a worktree
├── worktree.go      worktree_test.go    # WorktreeManager: git worktree add/remove (via GitRunner)
├── remote.go        remote_test.go      # RemotePublisher: add→commit-staged→push→issue→PR(link)
├── reporter.go                          # autodev.Reporter (embeds engine.Reporter + orchestration events)
├── terminal_reporter.go  *_test.go      # TerminalReporter (stdout) for the CLI entry point
└── orchestrator.go  orchestrator_test.go# Orchestrator: outer loop + per-item state machine

internal/app/                     # MODIFIED — provides concrete adapters (app → autodev)
├── autodev.go       autodev_test.go     # RunAutodev(ctx, cfg): build Orchestrator; appCoreRunnerFactory; provider for EngineerAgent
└── runner.go                            # (unchanged seams reused: NewAgentRunner, Run, SetUserAsker, Slash*)

internal/tui/                     # MODIFIED — TUI entry point
├── autodev_reporter.go  *_test.go       # TUIReporter: channel → tea.Msg bridge (mirrors asker.go)
└── (model.go / slash builtin)           # register `/autodev` builtin command + handle its messages

cmd/fox/
└── main.go                              # MODIFIED — dispatch `autodev` subcommand
```

## 5. Module Dependency Graph

```
 cmd/fox/main.go
      │ (args[0]=="autodev")
      ▼
 internal/app.RunAutodev ───────────────┐ provides adapters
      │ constructs                       │  (appCoreRunnerFactory, providerEngineerLLM)
      ▼                                  ▼
 internal/autodev.Orchestrator ── depends on ──▶ ports.go interfaces
      │            │            │            │            │
      ▼            ▼            ▼            ▼            ▼
 BacklogStore   Ledger    WorktreeMgr   StageMachine   RemotePublisher
                              │              │   │
                              ▼              ▼   ▼
                          GitRunner   CoreRunner  EngineerAsker→EngineerAgent
                                          │              │
                          (adapter in app over           ▼
                           app.AgentRunner)        provider.LLMProvider

 internal/autodev imports: internal/engine (Reporter, RunResult),
   internal/tools (UserAsker, Question/Answer), internal/provider, internal/slash.
 internal/autodev MUST NOT import internal/app or internal/tui  (cycle-free; see Decision 2).
```

## 6. Module Specifications

### Module: `ports.go` (abstractions — the seam)
- **Responsibility**: Declare the interfaces the control plane depends on, so the LLM/git boundaries are injectable and testable.
- **Dependencies**: `internal/engine`, `internal/tools`.
- **Interface**:
  ```go
  type CoreRunner interface {
      Run(ctx context.Context, prompt string, r engine.Reporter) (*engine.RunResult, error)
      SetUserAsker(a tools.UserAsker)
      SetModel(model string) error
      WorkDir() string
      // StagePrompt materializes a codexspec command body (e.g. "codexspec:generate-spec")
      // with args, via the runner's slash Registry/Executor.
      StagePrompt(command, args string) (string, error)
  }
  type CoreRunnerFactory interface {
      New(ctx context.Context, workDir, model string) (CoreRunner, error)
  }
  type EngineerAgent interface {
      Decide(ctx context.Context, qs []tools.Question, c StageContext) ([]tools.Answer, error) // answer ask_user_question
      Reply(ctx context.Context, prompt string, c StageContext) (string, error)                 // free-form answer
      // Review supervises a finished core run like a human user: given the run result,
      // its surfaced tool errors, and the Go-computed VerifyGap, it returns "" to approve
      // or a corrective instruction to feed back to the core Agent.
      Review(ctx context.Context, res *engine.RunResult, gap string, c StageContext) (string, error)
  }
  type GitRunner interface { Run(ctx context.Context, dir string, args ...string) (string, error) } // worktree infra + READ-ONLY verification (git/gh queries); never commit/push/issue/PR — the core Agent does those
  type Clock interface { Now() time.Time }
  ```
- **Files**: `ports.go`.

### Module: `config.go`
- **Responsibility**: Load `.foxharness/autodev.yml` into `AutodevConfig`; apply defaults when the file or any key is missing (REQ-023).
- **Dependencies**: `gopkg.in/yaml.v3`, stdlib.
- **Interface**: `Load(repoRoot string) (AutodevConfig, error)`; `AutodevConfig` (see §7).
- **Files**: `config.go`, `config_test.go`.

### Module: `backlog.go`
- **Responsibility**: Parse the backlog markdown (default `BACKLOG.md`) into ordered `[]Item` with type/title/priority/status/description (REQ-001). Missing `Status`→`pending`, missing `Priority`→lowest bucket (Edge Cases).
- **Dependencies**: stdlib only.
- **Interface**: `Parse(path string) ([]Item, error)`.
- **Files**: `backlog.go`, `backlog_test.go`.

### Module: `slug.go`
- **Responsibility**: Derive a kebab-case slug from a title; disambiguate collisions with a short suffix (Edge Cases).
- **Interface**: `Slug(title string, taken map[string]bool) string`.
- **Files**: `slug.go`, `slug_test.go`.

### Module: `ledger.go`
- **Responsibility**: The authoritative progress store (REQ-021). Load/save `.foxharness/autodev-state.json`; **seed** unknown items as `pending` and enforce ledger precedence over backlog `Status` (REQ-028); update status/branch/issue/PR/stage; `Pending()` selection by priority (REQ-002), excluding `done` (REQ-022).
- **Dependencies**: `encoding/json`, `Clock`.
- **Interface**: `Load(path string) (*Ledger, error)`; `Seed(items []Item)`; `Pending() []LedgerItem`; `Mark(slug string, mut func(*LedgerItem))`; `Save() error`; `IsDone(slug string) bool`.
- **Files**: `ledger.go`, `ledger_test.go`.

### Module: `engineer.go`
- **Responsibility**: The simulated engineer. `EngineerAgent` makes a same-model `provider.LLMProvider` call with the configured/default persona to (a) pick option(s) for `ask_user_question` (`Decide`) or (b) answer a free-form prompt (`Reply`). `EngineerAsker` implements `tools.UserAsker` by delegating to `EngineerAgent.Decide` and emitting reporter events (REQ-013, REQ-016). When no offered option fits, `Decide` returns the auto-appended **"Other"** free-text answer; it **never cancels** (on genuine indecision it picks the recommended/first option) so the loop never stalls (PLAN-005). `EngineerAgent.Review` additionally supervises a finished core run (final message + surfaced tool errors + the Go `VerifyGap`) and returns either approval or a corrective instruction — the "user-view" result review (Decision 8).
- **Dependencies**: `internal/provider`, `internal/tools`, `Reporter`.
- **Interface**: `NewEngineerAgent(p provider.LLMProvider, model, persona string) EngineerAgent`; `NewEngineerAsker(EngineerAgent, Reporter) tools.UserAsker`.
- **Files**: `engineer.go`, `engineer_test.go`.

### Module: `stage.go`
- **Responsibility**: Define the lean pipeline stages and their Go done-conditions, and run the per-stage engineer↔core loop (REQ-007..014). The `generate-spec` stage prompt embeds the backlog item's `Description` as the clarified requirement (REQ-010). Binds the spec dir produced by `generate-spec` (REQ-011).
- **Dependencies**: `CoreRunner`, `EngineerAgent`/`EngineerAsker`, `Reporter`, `GateRunner` (the implement stage's `Verify` = gates green **and** non-empty worktree diff).
- **Loop semantics (run-to-completion, not a stepper — PLAN-001)**: `AgentRunner.Run` runs the agent to completion of one prompt and returns `RunResult.FinalMessage`. The stage loop is therefore a sequence of `Run` calls:
  ```go
  func (m *StageMachine) RunStep(ctx context.Context, core CoreRunner, sc *StageContext, st Stage) error {
      msg := core.StagePrompt(st.Command, sc.Args())   // ① seed: codexspec body (+ Description for generate-spec)
      for {
          res, err := core.Run(ctx, msg, m.reporter)   // ② within Run: ask_user_question answered by EngineerAsker
          if err != nil { return err }
          ok, gap := st.Verify(*sc)                     // ③ Go HARD backstop: ground truth, not the LLM's "done"
          if ok { return nil }
          msg, err = m.engineer.Review(ctx, res, gap, *sc) // ④ engineer turns the gap into a correction (REQ-014)
          if err != nil { return err }
      }
  }
  ```
  Channel ① (`StagePrompt`) seeds the loop; channel ② (`ask_user_question`→`EngineerAsker`) runs *inside* each `Run`; channel ③ (`EngineerAgent.Review` over the run result + Go `VerifyGap`) runs *between* `Run`s until the ground-truth `Verify` passes. The same `RunStep` loop serves SDD stages **and** remote steps (Decision 8).
- **Interface**: `type Stage struct { Name, Command string; Verify func(sc StageContext) (ok bool, gap string) }`; `func LeanPipeline() []Stage` (generate-spec, spec-to-plan, plan-to-tasks, implement-tasks). The same `Stage` shape (a `Verify`) is reused for remote steps so one `RunStep` loop serves all (Decision 8).
- **Files**: `stage.go`, `stage_test.go`.

### Module: `gate.go`
- **Responsibility**: Run `go build ./...`, `go test ./...`, `gofmt -l .` in a worktree; honor `gates` config with the **floor** (`test` mandatory; disabling warns — REQ-018).
- **Dependencies**: `os/exec` (or an injected command runner), `Reporter`.
- **Interface**: `Check(ctx, workDir string, cfg GateConfig) (GateResult, error)`.
- **Files**: `gate.go`, `gate_test.go`.

### Module: `worktree.go`
- **Responsibility**: `git worktree add` a new branch `auto/<slug>` in the sibling `worktree_dir`; remove the worktree after a successful PR; resume an existing worktree/branch when the ledger says `in-progress` (REQ-005, REQ-006, Edge Cases).
- **Dependencies**: `GitRunner`.
- **Interface**: `Create(ctx, item LedgerItem) (Worktree, error)`; `Remove(ctx, Worktree) error`.
- **Files**: `worktree.go`, `worktree_test.go`.

### Module: `remote.go`
- **Responsibility**: After the gate is green, **drives the core Agent** through the fixed sequence (REQ-019): stage → `/codexspec:commit-staged` → `git push` → `gh issue create` → `/codexspec:pr` (`gh pr create` with `Closes #N`). **The core Agent performs every git/gh action via its own tools; this module runs no git/gh mutation itself.** After each step it **read-only**-verifies ground truth (commit→HEAD advanced + clean tree; push→`git ls-remote` tip match; issue/PR→`gh … --json number`), routes failures to the engineer Agent (REQ-014/030), then drives the next step via `RunStep`. Never merges (REQ-020). **Idempotent on resume (PLAN-002)**: if the ledger already records this item's `Issue`/`PR`/completed push (all read from ground truth), the step is skipped. Records issue#/PR# from the verified `gh --json` output.
- **Dependencies**: `CoreRunner` (drives each commit/push/issue/PR step), `GitRunner` (**read-only** verification: `rev-parse`/`status`/`ls-remote`/`gh --json`), `EngineerAgent`, `Reporter`.
- **Interface**: `Publish(ctx, core CoreRunner, wt Worktree, item LedgerItem) (PublishResult, error)`.
- **Files**: `remote.go`, `remote_test.go`.

### Module: `reporter.go` / `terminal_reporter.go`
- **Responsibility**: `Reporter` embeds `engine.Reporter` (so it can be passed to `CoreRunner.Run`) and adds orchestration events (`OnItemStart/OnStageStart/OnEngineerDecision/OnGit/OnGate/OnIssue/OnPR/OnItemDone`). `TerminalReporter` writes a readable stream to stdout (REQ-024, REQ-026).
- **Dependencies**: `internal/engine`.
- **Files**: `reporter.go`, `terminal_reporter.go`, `terminal_reporter_test.go`.

### Module: `orchestrator.go`
- **Responsibility**: Wire everything. Validate preconditions (git repo, `gh` available/authed) up front (Edge Cases); seed ledger; loop pending by priority; per item: create worktree → new `CoreRunner` (WorkDir=worktree, `SetUserAsker(EngineerAsker)`) → run lean stages → gate → publish → ledger `done` → remove worktree → next. Strictly serial (REQ-003). No abandonment budget (REQ-027).
- **Dependencies**: all modules above + the injected `CoreRunnerFactory`, `EngineerAgent`, `GitRunner`, `Reporter`, `AutodevConfig`.
- **Interface**: `New(deps Deps) *Orchestrator`; `Run(ctx context.Context) error`.
- **Files**: `orchestrator.go`, `orchestrator_test.go`.

### Module: `internal/app/autodev.go` (adapter + CLI driver)
- **Responsibility**: `RunAutodev(ctx, cfg CLIConfig)` constructs the `Orchestrator` with concrete adapters: `appCoreRunnerFactory` (wraps `NewAgentRunner`, returns a `coreRunnerAdapter` over `*AgentRunner`), an `os/exec` `GitRunner`, a `provider`-backed `EngineerAgent`, and a `TerminalReporter`. Breaks the cycle: **app imports autodev, never the reverse** (Decision 2).
- **Files**: `internal/app/autodev.go`, `internal/app/autodev_test.go`.

### Module: `cmd/fox/main.go` + TUI `/autodev`
- **Responsibility**: CLI dispatch of `autodev` (mirrors the existing `exec` branch in `parseArgs`); TUI registers a `CommandBuiltin` `/autodev` whose handler launches the orchestrator with a `TUIReporter` that bridges events into the model via `tea.Msg` (mirrors `internal/tui/asker.go`) (REQ-024, REQ-025).
- **Files**: `cmd/fox/main.go`, `internal/tui/autodev_reporter.go`, TUI builtin registration.

## 7. Data Models

### `AutodevConfig` (`.foxharness/autodev.yml`)
| Field | Type | Default | Notes |
|-------|------|---------|-------|
| BacklogFile | string | `BACKLOG.md` | REQ-001 |
| WorktreeDir | string | `../<repo>-worktrees` | sibling dir (REQ-005) |
| BaseBranch | string | `main` | |
| Remote | string | `origin` | |
| Concurrency | string | `serial` | only `serial` honored in v1 |
| Model | string | "" → global default | engineer & core share (REQ-016) |
| EngineerPrompt / EngineerPromptFile | string | "" → default persona | REQ-016 |
| Pipeline | string | `lean` | |
| Gates | struct{Build,Test,Gofmt bool} | all true; Test forced true | floor (REQ-018) |
| RemoteFlow | struct{CreateIssue,OpenPR,LinkIssue,AutoMerge bool} | true,true,true,**false** | REQ-019/020 |

### `LedgerItem` (`.foxharness/autodev-state.json`)
| Field | Type | Notes |
|-------|------|-------|
| Slug | string | key |
| Title | string | from backlog |
| Priority | enum `high`/`medium`/`low` | ordering input |
| Status | enum `pending`/`in-progress`/`done` | authoritative (REQ-021/028) |
| Branch | string | `auto/<slug>` |
| Stage | string | current SDD stage (resume granularity) |
| Issue | int | GitHub issue # |
| PR | int | GitHub PR # |
| SpecDir | string | bound spec dir (REQ-011) |

### `Item` (parsed backlog), `Stage`, `StageContext`, `GateResult`, `Worktree`, `PublishResult` — plain Go structs (see module specs).

## 8. API Contracts

### CLI: `fox autodev [backlog-path]`
- **Arguments**: optional `backlog-path` (overrides `backlog_file`).
- **Options**: `-C/-workdir` (repo root), `-model`, `-config` (autodev.yml path).
- **Output**: streamed interaction (item/stage/engineer/core/tool/git/gate/issue/PR events) to stdout.
- **Exit Codes**: `0` backlog drained; `2` precondition failure (not a git repo / `gh` missing or unauthenticated / remote unreachable); `1` unexpected error.

### TUI: `/autodev [backlog-path]` (builtin)
- **Behavior**: launches the orchestrator; renders the same event stream in the conversation/session area; non-blocking to the rest of the TUI.

### External processes
- **Control plane (`GitRunner`, infra + read-only):** `git worktree add -b auto/<slug> <dir> <base>`, `git worktree remove`; verification queries `git rev-parse HEAD`, `git status --porcelain`, `git ls-remote --heads <remote> auto/<slug>`, `gh pr view --json number`, `gh issue view`.
- **Core Agent (its own `bash`/`gh` tools):** `git add`, `git push -u <remote> auto/<slug>`, `gh issue create …`, `gh pr create … --body "…\nCloses #<N>"`. Go reads the resulting issue#/PR# back via the read-only `gh --json` queries above.

## 9. Implementation Phases

### Phase 1: Foundation (pure, no LLM/git)
- [ ] `item.go` value types; `config.go` + tests (defaults, partial file, gate floor).
- [ ] `backlog.go` parser + tests (well-formed, missing fields, empty file).
- [ ] `slug.go` + tests (kebab, collision suffix).
- [ ] `ledger.go` + tests (load/save, **seeding & precedence REQ-028/TC-023**, priority selection TC-002, skip-done TC-003).

### Phase 2: Core orchestration logic (fakes for LLM)
- [ ] `ports.go` interfaces.
- [ ] `engineer.go` `EngineerAsker` + `EngineerAgent.Review` + tests with a fake `EngineerAgent` (TC-007: valid `[]Answer`; TC-024: `Review` returns a correction on a gap).
- [ ] `stage.go` `StageMachine.RunStep` + tests with fake `CoreRunner`+`EngineerAgent`: advance only when Go `Verify` passes (TC-005), **engineer-approved-but-`Verify`-fails does not advance (TC-006/TC-025)**, spec-dir binding (TC-019), engineer `Review` correction drives retry (TC-009/TC-024), implement `Verify` needs non-empty diff (TC-026).
- [ ] `reporter.go` + `terminal_reporter.go` + tests (events captured TC-016).

### Phase 3a: Local integration (git worktree + gates)
- [ ] `gate.go` + tests with an injected fake command runner (gate-red blocks: TC-010; gate floor).
- [ ] `worktree.go` + tests with a fake `GitRunner` (create on `auto/<slug>`; remove TC-004/TC-014; resume existing).

### Phase 3b: Remote + app adapter
- [ ] `remote.go` (**drives the core Agent** through commit/push/issue/PR; `GitRunner` is **read-only** verify) + tests with fakes (ordered sequence TC-011; per-step read-only `Verify` HEAD/ls-remote/`gh --json`; nothing-to-commit→stage+retry TC-024; `Closes #N` TC-012; never merges TC-021; resume idempotency PLAN-002).
- [ ] `internal/app/autodev.go`: `coreRunnerAdapter` over `*AgentRunner` (real `Run`/`SetUserAsker`/`StagePrompt`), `os/exec` `GitRunner`, provider-backed `EngineerAgent` (same model TC-022), auto-approval policy (no human gating middleware) TC-020.

### Phase 4: Orchestrator + entry points
- [ ] `orchestrator.go` + tests with all fakes: full per-item flow, strict serialization (TC-018), empty backlog no-op, resume from `in-progress`, precondition failure exit code.
- [ ] `cmd/fox/main.go`: dispatch `autodev` subcommand → `app.RunAutodev`.
- [ ] `internal/tui/autodev_reporter.go` (`TUIReporter` bridge) + `/autodev` builtin registration + tests (TC-017).

### Phase 5: Hardening & docs
- [ ] `doc.go` package documentation; block-comment pass on exported identifiers.
- [ ] End-to-end orchestrator test across ≥2 fake items (serial, ledger transitions, cleanup).
- [ ] `gofmt -l .` clean; `go test ./...` green; update root docs/README mention of `fox autodev` / `/autodev`.

## 10. Technical Decisions

### Decision 1: Two-plane (Go control + LLM execution)
- **Choice**: Deterministic Go owns flow; the LLM only produces content inside a stage.
- **Rationale**: REQ-007/NFR-001 — prevent a self-driving skill from skipping/early-stopping/stalling; make flow unit-testable.
- **Alternatives**: a single orchestrating skill (rejected: non-deterministic, untestable).
- **Trade-offs**: more Go code than a prompt, but vastly more reliable and testable.

### Decision 2: `autodev` depends on abstractions; `app` provides the adapter
- **Choice**: `internal/autodev` defines `CoreRunner`/`CoreRunnerFactory`; `internal/app` implements them over `AgentRunner`. `app → autodev` only.
- **Rationale**: avoids the `app ↔ autodev` import cycle; mirrors the established `slash` ↔ `ForkRunner` delegation; enables fakes for tests.
- **Alternatives**: `autodev` importing `app` directly (rejected: import cycle once `app.RunAutodev` exists; couples tests to the heavy runner).
- **Trade-offs**: one extra adapter layer.

### Decision 3: Reuse `ask_user_question` via `EngineerAsker`
- **Choice**: Implement `tools.UserAsker` and install it with `AgentRunner.SetUserAsker`; do not invent a new tool.
- **Rationale**: REQ-013 — the merged tool already gates registration on an asker and steers the model to use it (`WithInteractiveAsk`).
- **Alternatives**: a bespoke autodev ask tool (rejected: duplicates #25).
- **Trade-offs**: engineer answers are constrained to the tool's `Question`/`Answer` shape (acceptable; that is the desired bounded decision surface).

### Decision 4: `git`/`gh` via `os/exec` behind a `GitRunner` — for infra + read-only verification only
- **Choice**: `GitRunner` shells out to `git`/`gh`, but the control plane uses it **only** for worktree create/remove (infra) and **read-only** verification queries (`rev-parse`, `status`, `ls-remote`, `gh … --json`). The development mutations — commit, push, issue create, PR create — are performed by the **core Agent** via its own `bash`/`gh` tools (REQ-019/030).
- **Rationale**: keeps Go out of the development workflow (the LLM does it, like a user driving the agent) while still giving Go cheap, deterministic, network-free-testable ground-truth checks; `gh` reuses the user's existing auth (NFR-006).
- **Alternatives**: a Go GitHub SDK (rejected: extra dependency/auth); Go running the dev mutations directly (rejected by design — Go performs no development action).
- **Trade-offs**: requires `gh` installed/authed (used by both the core Agent and the verifier) — validated as a startup precondition.

### Decision 5: Go-evaluated per-step `Verify` (read-only)
- **Choice**: `Stage.Verify` is a **read-only** Go predicate over ground truth (artifact present / gates green / HEAD advanced / remote tip / `gh --json`); the implement step's `Verify` includes the gate and a non-empty diff.
- **Rationale**: REQ-007/029 — never trust the LLM's claim of completion (TC-006/025); see Decision 8 for the supervised loop.
- **Trade-offs**: each step needs an explicit, checkable ground-truth signal (true for spec/plan/tasks/code/commit/push/PR).

### Decision 6: JSON ledger as the source of truth; no `BACKLOG.md` writeback
- **Choice**: `.foxharness/autodev-state.json` on `main` is authoritative; backlog `Status` is advisory (REQ-028).
- **Rationale**: PRs are not auto-merged (REQ-020), so `BACKLOG.md` on `main` cannot reflect progress in real time; the ledger enables resumable selection.
- **Trade-offs**: two places describe items (backlog = source set; ledger = status) — disambiguated by explicit precedence.

### Decision 7: No abandonment budget / failure state machine
- **Choice**: Converge stages via the engineer↔core dialogue + Go `Done`; validate only true preconditions up front.
- **Rationale**: explicit product constraint (REQ-027) — trust SDD; don't over-engineer failure paths.
- **Trade-offs**: a genuinely non-converging stage would loop; accepted per direction, and visible via the streamed reporter.

### Decision 8: Engineer-driven correction + Go ground-truth backstop (no subagent checker)
- **Choice**: Go owns the ordered outer loop over **all** steps and drives the next one; per step the core Agent does the work (incl. every git/gh action), the engineer Agent reviews/corrects, and Go **read-only**-verifies ground truth to decide advancement. Go performs **no** development action — it only drives the sequence (so the LLM can't stop the pipeline early — the skill-mode bug) and verifies.
- **Rationale**: keeps determinism (an LLM's claim of "done" never advances a step — TC-025) while keeping *recovery* in the LLM/engineer plane (REQ-014/029/030).
- **Alternatives**: (a) **subagent completion-checker** — rejected: re-introduces an LLM as the gate (can hallucinate success), adds per-step cost/latency/flakiness, is hard to unit-test, and duplicates the engineer Agent; (b) engineer-only gate (no Go check) — rejected: pure LLM trust; (c) Go running the git/gh dev mutations — rejected: Go performs no development action; the core Agent does, like a user driving the agent (TC-024).
- **Trade-offs**: ~6 small per-step verifiers (reusing `GitRunner`/`ExecRunner`), in exchange for cheap, deterministic, fakeable gating.
