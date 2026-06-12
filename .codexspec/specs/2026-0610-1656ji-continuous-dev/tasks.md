# Task Breakdown: Continuous Development / Backlog Autopilot (`autodev`)

> Spec: `spec.md` · Plan: `plan.md` (this dir)
> Convention: Go internal packages; each code file `X.go` is preceded by its test
> `X_test.go` (constitution Test-First). Pure declaration files (interfaces, value
> types, package doc) are `Setup` and need no preceding test. `[P]` = parallelizable
> with sibling `[P]` tasks once dependencies are met.

## Overview
- **Total tasks**: 41
- **Parallelizable tasks**: 19 (`[P]`)
- **Phases**: 6 (Foundation → Core → Local Integration → Remote/Adapter → Interface → Testing & Docs)
- **New package**: `internal/autodev` (+ `internal/app/autodev.go`, `internal/tui/autodev_*`, `cmd/fox/main.go`)

---

## Phase 1: Foundation

### Task 1.1: Package doc skeleton
- **Type**: Setup
- **Files**: `internal/autodev/doc.go`
- **Description**: Create `internal/autodev` with a package block comment describing the two-plane orchestrator and its cycle-free dependency rule (no import of `internal/app`/`internal/tui`).
- **Dependencies**: None
- **Est. Complexity**: Low

### Task 1.2: Value types [P]
- **Type**: Setup
- **Files**: `internal/autodev/item.go`
- **Description**: Declare `Priority` (high/medium/low), `Status` (pending/in-progress/done), `Item`, `Worktree`, `GateResult`, `PublishResult`, `StageContext` value types with block comments.
- **Dependencies**: Task 1.1
- **Est. Complexity**: Low

### Task 1.3: Port interfaces [P]
- **Type**: Setup
- **Files**: `internal/autodev/ports.go`
- **Description**: Declare injectable boundaries: `CoreRunner`, `CoreRunnerFactory`, `EngineerAgent`, `GitRunner`, `ExecRunner`, `Clock` (imports `internal/engine`, `internal/tools`). (Spec REQ-007/008/013/016)
- **Dependencies**: Task 1.1, Task 1.2
- **Est. Complexity**: Low

### Task 1.4: Config tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/config_test.go`
- **Description**: Table tests for `Load`: full file, missing file→defaults, partial keys, **gate floor** (Test forced true), `auto_merge=false` default. (REQ-018/023, TC-015)
- **Dependencies**: Task 1.2
- **Est. Complexity**: Low

### Task 1.5: Config loader
- **Type**: Implementation
- **Files**: `internal/autodev/config.go`
- **Description**: `AutodevConfig` + `Load(repoRoot)` via `yaml.v3` with defaults and gate-floor enforcement.
- **Dependencies**: Task 1.4
- **Est. Complexity**: Low

### Task 1.6: Slug tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/slug_test.go`
- **Description**: Tests for kebab slug derivation and collision disambiguation. (Edge Cases)
- **Dependencies**: Task 1.1
- **Est. Complexity**: Low

### Task 1.7: Slug
- **Type**: Implementation
- **Files**: `internal/autodev/slug.go`
- **Description**: `Slug(title, taken)`.
- **Dependencies**: Task 1.6
- **Est. Complexity**: Low

### Task 1.8: Backlog parser tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/backlog_test.go`
- **Description**: Tests for `Parse`: well-formed items (TC-001), missing Status→pending, missing Priority→lowest, empty file. (REQ-001, Edge Cases)
- **Dependencies**: Task 1.2
- **Est. Complexity**: Medium

### Task 1.9: Backlog parser
- **Type**: Implementation
- **Files**: `internal/autodev/backlog.go`
- **Description**: `Parse(path) ([]Item, error)` for the `## [type] Title` / Priority / Status / Description format.
- **Dependencies**: Task 1.8
- **Est. Complexity**: Medium

### Task 1.10: Ledger tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/ledger_test.go`
- **Description**: Tests for load/save, **seeding & precedence** (TC-023/REQ-028), `Pending` ordered by priority (TC-002), skip-done (TC-003), `Mark`. Uses a fake `Clock`.
- **Dependencies**: Task 1.2, Task 1.3
- **Est. Complexity**: Medium

### Task 1.11: Ledger
- **Type**: Implementation
- **Files**: `internal/autodev/ledger.go`
- **Description**: JSON ledger at `.foxharness/autodev-state.json`: `Load/Seed/Pending/Mark/Save/IsDone`. (REQ-021/022/028)
- **Dependencies**: Task 1.10
- **Est. Complexity**: Medium

**Checkpoint 1**: `go test ./internal/autodev/...` green for config/slug/backlog/ledger.

---

## Phase 2: Core orchestration logic (fakes for LLM)

### Task 2.1: Reporter interface
- **Type**: Setup
- **Files**: `internal/autodev/reporter.go`
- **Description**: `Reporter` interface embedding `engine.Reporter` + orchestration events (`OnItemStart/OnStageStart/OnEngineerReview/OnVerify/OnGate/OnIssue/OnPR/OnItemDone`). (REQ-024/026)
- **Dependencies**: Task 1.1, Task 1.2
- **Est. Complexity**: Low

### Task 2.2: Terminal reporter tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/terminal_reporter_test.go`
- **Description**: Assert events render to an `io.Writer` buffer (TC-016).
- **Dependencies**: Task 2.1
- **Est. Complexity**: Low

### Task 2.3: Terminal reporter
- **Type**: Implementation
- **Files**: `internal/autodev/terminal_reporter.go`
- **Description**: `TerminalReporter` writing the streamed interaction to stdout. (REQ-024)
- **Dependencies**: Task 2.2
- **Est. Complexity**: Low

### Task 2.4: Engineer tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/engineer_test.go`
- **Description**: `EngineerAsker.Ask` routes to a fake `EngineerAgent`, returns `[]tools.Answer` matching `QuestionText` with a valid label (TC-007); "no fitting option → Other"; never cancels (PLAN-005). `EngineerAgent.Review(result, gap)` returns a correction when a step isn't verified and "" on approval (TC-024).
- **Dependencies**: Task 1.3, Task 2.1
- **Est. Complexity**: Medium

### Task 2.5: Engineer agent + asker
- **Type**: Implementation
- **Files**: `internal/autodev/engineer.go`
- **Description**: `EngineerAgent` (`Decide`/`Reply`/**`Review`**) over `provider.LLMProvider` (same model + persona) and `EngineerAsker` implementing `tools.UserAsker`. (REQ-013/014/016)
- **Dependencies**: Task 2.4
- **Est. Complexity**: Medium

### Task 2.6: Stage machine tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/stage_test.go`
- **Description**: With fake `CoreRunner`+`EngineerAgent`: advance only when Go `Verify` passes (TC-005); **engineer-approved-but-`Verify`-fails does not advance** (TC-006/TC-025); spec-dir binding (TC-019); engineer `Review(gap)` correction drives the retry (TC-009/TC-024); implement `Verify` needs non-empty diff (TC-026); `LeanPipeline` order. (REQ-007..014/029/030)
- **Dependencies**: Task 1.3, Task 2.1, Task 2.5
- **Est. Complexity**: High

### Task 2.7: Stage machine
- **Type**: Implementation
- **Files**: `internal/autodev/stage.go`
- **Description**: `Stage` (with `Verify`/`VerifyGap`), `LeanPipeline()`, `StageMachine.RunStep` (loop: seed→Run→Go `Verify`?→engineer `Review` correction). Generate-spec prompt embeds `Description` (REQ-010). The same `RunStep` is reused by remote steps (Decision 8).
- **Dependencies**: Task 2.6
- **Est. Complexity**: High

**Checkpoint 2**: core stage/engineer/reporter tests pass with fakes (no LLM).

---

## Phase 3a: Local integration (git worktree + gates)

### Task 3a.1: Gate runner tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/gate_test.go`
- **Description**: With a fake `ExecRunner`: all-green passes; test-red blocks (TC-010); gate-floor (Test cannot be disabled; disabling warns). (REQ-018)
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 3a.2: Gate runner
- **Type**: Implementation
- **Files**: `internal/autodev/gate.go`
- **Description**: `Check(ctx, workDir, GateConfig)` running `go build ./...`, `go test ./...`, `gofmt -l .` via `ExecRunner`.
- **Dependencies**: Task 3a.1
- **Est. Complexity**: Medium

### Task 3a.3: Worktree manager tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/worktree_test.go`
- **Description**: With a fake `GitRunner`: `Create` issues `git worktree add -b auto/<slug>` in the sibling dir (TC-004); `Remove` cleans up local worktree (TC-014); resume existing branch/worktree when `in-progress`. (REQ-005/006)
- **Dependencies**: Task 1.2, Task 1.3
- **Est. Complexity**: Medium

### Task 3a.4: Worktree manager
- **Type**: Implementation
- **Files**: `internal/autodev/worktree.go`
- **Description**: `Create`/`Remove` via `GitRunner`.
- **Dependencies**: Task 3a.3
- **Est. Complexity**: Medium

### Task 3a.5: Exec git runner tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/gitexec_test.go`
- **Description**: Verify command construction for the `os/exec`-backed `GitRunner` (worktree add/remove + **read-only** queries: `rev-parse`/`status`/`ls-remote`/`gh … --json`) and `ExecRunner` (`go build/test`, `gofmt`); returns raw output for callers to parse. No mutating git/gh here.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 3a.6: Exec git runner
- **Type**: Implementation
- **Files**: `internal/autodev/gitexec.go`
- **Description**: `os/exec` implementations of `GitRunner` (worktree add/remove + **read-only** git/gh queries) and `ExecRunner` (`go build/test`, `gofmt`). It runs **no** commit/push/issue/PR — the core Agent does those.
- **Dependencies**: Task 3a.5
- **Est. Complexity**: Medium

**Checkpoint 3a**: gate + worktree + gitexec tests pass with fakes/temp dirs.

---

## Phase 3b: Remote publishing

### Task 3b.1: Remote publisher tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/remote_test.go`
- **Description**: `RemotePublisher` **drives the core Agent** through add→commit-staged→push→issue→PR (core Agent runs all git/gh; `GitRunner` is **read-only** verify). With fakes+ledger: ordered sequence (TC-011); per-step read-only `Verify` (commit→HEAD; push→`ls-remote`; PR→`gh --json`); **nothing-to-commit → engineer steers `git add` + retry → commit `Verify` passes** (TC-024); PR body `Closes #N` (TC-012); never merges (TC-021); idempotent on resume (PLAN-002). (REQ-019/020/029/030)
- **Dependencies**: Task 1.3, Task 1.11, Task 2.1
- **Est. Complexity**: High

### Task 3b.2: Remote publisher
- **Type**: Implementation
- **Files**: `internal/autodev/remote.go`
- **Description**: `Publish(ctx, core, wt, item)` **drives the core Agent** through the sequence (it runs all git/gh) with per-step read-only `Verify` + engineer correction + idempotency; `GitRunner` used read-only only.
- **Dependencies**: Task 3b.1
- **Est. Complexity**: High

**Checkpoint 3b**: remote sequence + idempotency verified.

---

## Phase 4: Orchestrator + entry points

### Task 4.1: Orchestrator tests [P]
- **Type**: Testing
- **Files**: `internal/autodev/orchestrator_test.go`
- **Description**: With all fakes: precondition validation (git repo, `gh` present) fails fast; seed→select→per-item flow; **strict serialization** (item N+1 starts only after item N done — TC-018); empty backlog no-op; resume from `in-progress`; no abandonment budget (REQ-027).
- **Dependencies**: Task 1.5, 1.7, 1.9, 1.11, 2.1, 2.5, 2.7, 3a.2, 3a.4, 3b.2
- **Est. Complexity**: High

### Task 4.2: Orchestrator
- **Type**: Implementation
- **Files**: `internal/autodev/orchestrator.go`
- **Description**: `New(Deps)` + `Run(ctx)`: wire all modules, outer priority loop, per-item state machine, cleanup. (REQ-003/004/027)
- **Dependencies**: Task 4.1
- **Est. Complexity**: High

### Task 4.3: App adapter tests [P]
- **Type**: Testing
- **Files**: `internal/app/autodev_test.go`
- **Description**: `coreRunnerAdapter` exposes real `Run`/`SetUserAsker`/`StagePrompt`; engineer & core resolve the same model (TC-022); auto-approval policy installs no human-gating middleware (TC-020).
- **Dependencies**: Task 4.2, Task 1.3
- **Est. Complexity**: Medium

### Task 4.4: App adapter + RunAutodev
- **Type**: Implementation
- **Files**: `internal/app/autodev.go`
- **Description**: `RunAutodev(ctx, cfg, reporter)` builds the `Orchestrator` with `appCoreRunnerFactory` (wraps `NewAgentRunner`→`coreRunnerAdapter`), `gitexec` runners, provider-backed `EngineerAgent`. **app→autodev only** (Decision 2).
- **Dependencies**: Task 4.3, Task 3a.6, Task 2.5, Task 2.3
- **Est. Complexity**: Medium

### Task 4.5: CLI dispatch tests
- **Type**: Testing
- **Files**: `cmd/fox/main_test.go`
- **Description**: `parseArgs`/dispatch routes `fox autodev [path]` to the autodev path with correct workdir/config; exit-code mapping.
- **Dependencies**: Task 4.4
- **Est. Complexity**: Low

### Task 4.6: CLI subcommand
- **Type**: Implementation
- **Files**: `cmd/fox/main.go`
- **Description**: Add an `autodev` branch (mirroring `exec`) calling `app.RunAutodev` with a `TerminalReporter`. (REQ-024)
- **Dependencies**: Task 4.5
- **Est. Complexity**: Low

### Task 4.7: TUI reporter tests [P]
- **Type**: Testing
- **Files**: `internal/tui/autodev_reporter_test.go`
- **Description**: `TUIReporter` forwards events over a channel (mirrors `asker.go` bridge); non-blocking. (TC-017)
- **Dependencies**: Task 2.1
- **Est. Complexity**: Medium

### Task 4.8: TUI reporter bridge
- **Type**: Implementation
- **Files**: `internal/tui/autodev_reporter.go`
- **Description**: `TUIReporter` (channel→`tea.Msg`) rendering events into the session area. (REQ-025)
- **Dependencies**: Task 4.7
- **Est. Complexity**: Medium

### Task 4.9: TUI `/autodev` command tests [P]
- **Type**: Testing
- **Files**: `internal/tui/autodev_command_test.go`
- **Description**: Builtin `/autodev` is registered and its handler launches the orchestrator with a `TUIReporter`.
- **Dependencies**: Task 4.8, Task 4.4
- **Est. Complexity**: Medium

### Task 4.10: TUI `/autodev` command
- **Type**: Implementation
- **Files**: `internal/tui/autodev_command.go`
- **Description**: Register the `CommandBuiltin` `/autodev` and wire its handler to `app.RunAutodev(ctx, cfg, tuiReporter)`. (REQ-025/026)
- **Dependencies**: Task 4.9
- **Est. Complexity**: Medium

**Checkpoint 4**: `fox autodev` and `/autodev` launch the orchestrator end-to-end (with fakes/dry-run).

---

## Phase 5: End-to-end testing & documentation

### Task 5.1: Orchestrator end-to-end test
- **Type**: Testing
- **Files**: `internal/autodev/orchestrator_e2e_test.go`
- **Description**: ≥2 fake backlog items: serial transitions, ledger pending→in-progress→done, worktree create/remove, issue/PR recorded, backlog drained. (Stories 1, 2, 4)
- **Dependencies**: Task 4.2
- **Est. Complexity**: High

### Task 5.2: Package documentation pass
- **Type**: Documentation
- **Files**: `internal/autodev/doc.go`
- **Description**: Finalize the package doc + verify block-comment coverage on all exported identifiers (NFR-005, constitution §3).
- **Dependencies**: Task 4.2
- **Est. Complexity**: Low

### Task 5.3: User docs + sample config [P]
- **Type**: Documentation
- **Files**: `docs/autodev.md`
- **Description**: Document `fox autodev` / `/autodev`, the lean pipeline, and a sample `.foxharness/autodev.yml`; link from README.
- **Dependencies**: Task 4.6, Task 4.10
- **Est. Complexity**: Medium

### Task 5.4: Full verification gate
- **Type**: Testing
- **Files**: (repo-wide) `go build ./... && go test ./... && gofmt -l .`
- **Description**: Whole-repo green + format clean before opening the PR for this feature.
- **Dependencies**: Task 5.1, Task 5.2, Task 5.3
- **Est. Complexity**: Low

**Checkpoint 5**: end-to-end green; docs complete; `gofmt -l .` empty.

---

## Execution Order

```
Phase 1  1.1 ─┬─► 1.2 [P] ─┬─► 1.4[P]►1.5      ┌─► 1.8[P]►1.9
              ├─► 1.3 [P] ─┤   1.6[P]►1.7      └─► 1.10[P]►1.11
              │            └────────────────────────────────┐
Phase 2  2.1 ─┴─► 2.2[P]►2.3                                 │
              └─► 2.4[P]►2.5 ─► 2.6[P]►2.7  (needs 1.3,2.1)   │
Phase 3a  3a.1[P]►3a.2   3a.3[P]►3a.4   3a.5[P]►3a.6  (needs 1.3,1.2)
Phase 3b  3b.1[P]►3b.2                   (needs 1.3,1.11,2.1)
Phase 4   4.1►4.2 ─► 4.3[P]►4.4 ─► 4.5►4.6
                     └─► 4.7[P]►4.8 ─► 4.9[P]►4.10
Phase 5   4.2►5.1   4.2►5.2   {4.6,4.10}►5.3   {5.1,5.2,5.3}►5.4
```

## Checkpoints
- [x] **Checkpoint 1** — Foundation: config/slug/backlog/ledger tests green.
- [x] **Checkpoint 2** — Core: stage/engineer/reporter tests green (fakes).
- [x] **Checkpoint 3a/3b** — Integration: gate/worktree/gitexec/remote tests green.
- [x] **Checkpoint 4** — Interface: `fox autodev` + `/autodev` launch the orchestrator.
- [x] **Checkpoint 5** — E2E green, docs done, `gofmt -l .` empty.

## Implementation Notes (2026-06-11)

All 41 tasks completed test-first. Deviations from the original task sheet,
each made for a concrete reason discovered during implementation:

- **`Stage.Append` field added** (stage.go): the `codexspec:generate-spec`
  command body consumes no `$ARGUMENTS`, so the item Description (REQ-010)
  and per-step hard requirements (e.g. "body MUST contain `Closes #N`") are
  appended to the materialized prompt instead of passed as arguments.
- **`Stage.Skip`/`Stage.Prepare` hooks added**: `Skip` carries the remote
  steps' resume idempotency (PLAN-002); `Prepare` snapshots pre-existing
  spec dirs for the generate-spec directory diff (REQ-011).
- **Commit verification uses `git rev-list --count base..HEAD` + clean
  status** rather than a recorded BaseHead: monotonic across resumes, so
  the same predicate serves both `Verify` and `Skip` (stage/commit steps).
- **`Publish` takes a `record` callback** so the verified issue number is
  durably in the ledger before the PR step runs (Edge Case: interrupted
  between issue and PR).
- **`RemoteFlowConfig.AutoMerge` is forced false at Load** with a warning —
  a config cannot opt into auto-merge (REQ-020 hardened).
- **TUI wiring**: `tui.Config.Autodev` launcher injected by `app.RunTUI`
  keeps the dependency `app → {tui, autodev}` one-way (Decision 2).

## Notes
- TDD: every `*.go` with behavior is preceded by its `*_test.go` task; `Setup`
  declaration files (doc/item/ports/reporter interface) carry no test.
- All external boundaries (LLM, git, gh, go/gofmt) are faked in unit tests — no
  network or real `gh` required to make the suite green (constitution Testing Standards).
