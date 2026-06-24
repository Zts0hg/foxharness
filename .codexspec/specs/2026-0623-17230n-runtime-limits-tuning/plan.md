# Implementation Plan: Subagent Default Turn Budget

<!--
Language: Generated in English per .codexspec/config.yml (language.document: en).
Only sections that materially help implement or verify this feature are included.
Omitted template sections (Tech Stack block, Data Models, API Contracts, Security,
Monitoring) are not applicable: this is a single-package internal Go change with no
new data, API surface, trust boundary, or deployable artifact.
-->

**Related Spec**: `.codexspec/specs/2026-0623-17230n-runtime-limits-tuning/spec.md`
**Confirmed Requirements**: `.codexspec/specs/2026-0623-17230n-runtime-limits-tuning/requirements.md`
**Created**: 2026-06-23
**Status**: Draft

## Context

The subagent turn budget is a hardcoded literal today. In `internal/subagent/manager.go`, `Manager.Run` builds its engine with `engine.Config{EnableThinking: false, MaxTurns: 8}` (manager.go:113-116). The value `8` is demo-sized and too small for real coding subtasks (see `spec.md` Context).

Verified repository facts that constrain this plan:

- `subagent.Manager` (manager.go:46-52) currently has fields `provider`, `workDir`, `homeDir` — **no** turn-budget field. The `8` is an inline literal.
- `subagent.NewManager(p provider.LLMProvider, workDir string) *Manager` (manager.go:57-63) is the **single** constructor used by every entry point.
- Four construction sites across three packages build the subagent via that constructor and must keep working unchanged:
  - `internal/app/runner.go:235` (`currentSubagentManager`, the on-demand subagent path for fox CLI + TUI)
  - `internal/app/runner.go:894` (`buildRegistry`, registers the main agent's subagent tool for fox CLI + TUI — the primary fox path)
  - `internal/feishu/runner.go:178` (Feishu runner)
  - `internal/agentops/runner.go:211` (AgentOps runner)
  - (Note: the `MaxTurns: 20` / `MaxTurns: 24` literals in `feishu/runner.go:113` and `agentops/runner.go:123` are those runners' **main-loop** budgets, not the subagent's — out of scope.)
- `engine.Config.MaxTurns > 0` means "limited to N turns"; `<= 0` means unlimited (engine/loop.go:420). On exhaustion the engine returns `(RunResult, error)` with `"超过最大 Turn 数限制: %d"` and marks the run as an error (engine/loop.go:420-430); `Manager.Run` propagates that error and discards the accumulated report (manager.go:135-137). This behavior is **preserved**, only the threshold changes.
- The repo uses a fluent-builder convention for configurators: `func (c *Composer) WithXxx(...) *Composer` in `internal/context/prompt.go`. Existing subagent tests are white-box (`package subagent`) and use a fake `provider.LLMProvider` (`finalReportProvider` in manager_test.go) plus direct `&Manager{...}` construction.

## Goals / Non-Goals

**Goals:**

- Raise the subagent default turn budget from `8` to `200` via a single package-level constant.
- Apply the new default uniformly to all subagent construction sites (four sites across three packages) with **zero** changes at the call sites.
- Keep the budget injectable internally so tests can exercise the exhaustion path quickly.

**Non-Goals (inherited from spec):**

- No change to the main agent `--max-turns` (stays unlimited).
- No change to compaction/context thresholds.
- No change to exhaustion semantics (report still discarded on budget exhaustion — preserved, not fixed).
- No user-facing configuration surface (no CLI flag, env var, or per-invocation parameter).

## Architecture Overview

The entire change is confined to `internal/subagent`. Because `NewManager` is the only constructor all four construction sites use, defaulting the budget **inside** `NewManager` propagates the new value everywhere with no caller edits. Test injection rides on a fluent setter that mirrors the existing `Composer.WithXxx` convention.

**Covers**: REQ-001, REQ-002, REQ-004, NFR-003

```
fox CLI/TUI  (app/runner.go:235, :894) ─┐
Feishu       (feishu/runner.go:178)     ─┼─► subagent.NewManager(p, wd)
AgentOps     (agentops/runner.go:211)   ─┘            │
                                                       ▼
                                  Manager{ maxTurns: DefaultMaxTurns (200) }
                                                       │   ▲
                                                       │   └─ .WithMaxTurns(n)   ← tests only
                                                       ▼
                                Run() ─► engine.Config{ MaxTurns: m.maxTurns }
```

## Component Structure

All changes are in `internal/subagent/manager.go` (plus tests in `manager_test.go`):

- **C1 — Package constant** `DefaultMaxTurns = 200` with a godoc block comment.
  - Covers: REQ-001
- **C2 — `Manager.maxTurns int` field + defaulting + fluent setter**:
  - Add field `maxTurns int` to `Manager`.
  - `NewManager` sets `maxTurns: DefaultMaxTurns` (signature unchanged).
  - Add `func (m *Manager) WithMaxTurns(n int) *Manager` for internal/test injection.
  - Covers: REQ-001, REQ-002, REQ-004, NFR-001, NFR-003
- **C3 — Wire the field into `Run`**: replace the literal `MaxTurns: 8` with `MaxTurns: m.maxTurns` in the `engine.Config` built by `Run`.
  - Covers: REQ-001, REQ-005

No other packages are modified. No CLI flag, env var, config-struct field, or tool-parameter is added.

## Decisions

### Decision 1 (PLD-1): Inject the budget via a fluent setter matching the repo's Composer convention

**Context**: REQ-004 requires the `Manager` to accept the turn budget internally for test injection; NFR-003 requires `NewManager`'s signature to stay stable so all four construction sites compile unchanged; DEC-002 forbids any user-facing configuration surface.

**Options Considered**:

1. Functional-option variadic: `NewManager(p, wd, opts ...Option)` + a `WithMaxTurns` option.
2. Fluent method `WithMaxTurns(n int) *Manager`, `NewManager` signature unchanged (matches `Composer.WithReadOnlyMemory`).
3. Unexported field set only by direct struct construction in same-package tests.

**Decision**: Option 2 — an exported fluent `WithMaxTurns(n int) *Manager` method.

**Rationale**: It is the exact convention already used in this repository (`internal/context/prompt.go` exports `func (c *Composer) WithXxx(...) *Composer`), so it reuses an existing pattern rather than introducing functional options (constitution Core Principle 5: reuse patterns before new abstractions). It keeps `NewManager(p, workDir)` byte-for-byte stable, satisfying NFR-003 with the smallest surface. An exported method is acceptable because `internal/subagent` is not end-user-facing — "user-facing configuration surface" in DEC-002 means CLI flags, env vars, and tool parameters, not an internal Go API used by tests.

**Covers**: REQ-004, NFR-001, NFR-003

**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Budget exhaustion discards the partial report and returns an error to the parent (RA-1) | Low (rare at 200) | Medium | Accepted and **preserved** per REQ-005 / Assumption A-1. Changing it is a separate, out-of-scope feature. |
| Higher per-subagent token spend (200 vs 8) | Medium | Low | Accepted per DEC-001. Cost-control levers (main agent `--max-turns`, model choice) are unchanged and remain the intended knobs. |
| Existing tests that construct `&Manager{...}` directly get a zero-value `maxTurns` | Low | None | Harmless: those tests (`manager_test.go:94,120,147`) only call `buildComposer`, never `Run`. No behavior change. |

## Implementation Phases

The design is small enough for a single TDD cycle rather than a multi-phase build. Phases below follow the constitution's mandatory Red → Green → Refactor cycle.

### Phase 1: Red — failing tests (Covers: NFR-001, CON-002)

Add to `internal/subagent/manager_test.go` (white-box `package subagent`):

- `TestDefaultMaxTurnsIs200` — asserts `DefaultMaxTurns == 200`.
- `TestNewManagerDefaultsMaxTurnsTo200` — asserts a `Manager` from `NewManager(...)` has `maxTurns == DefaultMaxTurns`.
- `TestWithMaxTurnsOverridesDefault` — asserts `NewManager(...).WithMaxTurns(3).maxTurns == 3`.
- `TestRunHonorsInjectedMaxTurnsAndPreservesExhaustion` — build a `Manager` with `WithMaxTurns(1)` backed by a fake provider whose `Generate` returns an assistant message containing a tool call (forcing the engine to loop past the budget); assert `Run` returns a non-nil error whose message contains `超过最大 Turn 数限制: 1`. This proves the injected value governs the engine **and** that exhaustion behavior is preserved.

Run `go test ./internal/subagent/...` → all four fail (constant/field/setter do not exist yet, and `Run` still uses `8`).

### Phase 2: Green — minimal implementation (Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, NFR-001, NFR-002, NFR-003)

In `internal/subagent/manager.go`:

1. Add `const DefaultMaxTurns = 200` with a godoc block comment (C1).
2. Add `maxTurns int` to `Manager`; set `maxTurns: DefaultMaxTurns` in `NewManager` (C2).
3. Add `func (m *Manager) WithMaxTurns(n int) *Manager` (C2).
4. In `Run`, change `MaxTurns: 8` → `MaxTurns: m.maxTurns` (C3).

Run `go test ./internal/subagent/...` → all pass.

### Phase 3: Verify & refactor (Covers: NFR-002, NFR-003)

- `gofmt -w .` (constitution: formatted code).
- `go build ./...` — confirms all four construction sites compile unchanged (NFR-003).
- `go test ./...` — full suite green.
- `grep -rnE "subagent\.Manager\{|&subagent\.Manager" --include="*.go" internal/ cmd/ | grep -v _test.go` returns nothing — confirms every production subagent is built through `NewManager` and thus inherits the default (REQ-002 regression guard).
- Manual confirmation that no new flag, env-var read, or config-struct field was introduced (NFR-002); the only additions are the constant, the field, the setter, and the `Run` wiring.

## Verification Strategy

- **Unit (white-box)**: the four tests in Phase 1 directly cover the constant value, default wiring, override, and exhaustion preservation. This covers REQ-001 (value), REQ-002 (default via the shared `NewManager`), REQ-004/NFR-001 (injectability), and REQ-005 (exhaustion preserved).
- **Cross-construction-site uniformity (REQ-002)**: because all four construction sites call the same `NewManager` and none overrides the budget, the unit test on `NewManager` transitively proves uniformity. Phase 3 also greps to confirm (a) no other `MaxTurns:` literal reaches the subagent path, and (b) no production code constructs `subagent.Manager{}` directly (bypassing `NewManager`).
- **No-config-surface (REQ-003/NFR-002)**: verified by inspection — the diff adds no flag registration, no `os.Getenv`, and no tool-input schema field.
- **Repo gates**: `gofmt -l .` clean, `go build ./...` succeeds, `go test ./...` passes.

## Performance Considerations

- Raising the default from 8 to 200 can increase token spend per subagent on long tasks; this is the intended trade-off (DEC-001) and is bounded in practice by the main agent's overall `--max-turns` and by compaction. No hot-path code is altered, so no profiling is required for this change.

## Requirements Coverage

| Spec Requirement | Plan Coverage | Reference |
|------------------|---------------|-----------|
| REQ-001 (default = `DefaultMaxTurns` = 200) | Full | C1, C2, C3 / Phase 2 |
| REQ-002 (all 4 construction sites use the default) | Full | Architecture Overview / C2 (shared `NewManager`) / Phase 3 grep |
| REQ-003 (no user-facing config surface) | Full | C1–C3 add none / Verification Strategy / Phase 3 inspection |
| REQ-004 (`Manager` internally injectable; literal removed) | Full | C2, C3 / Decision 1 / Phase 1–2 |
| REQ-005 (exhaustion behavior preserved) | Full | C3 (only value changes) / Phase 1 exhaustion test |
| NFR-001 (testable via injection) | Full | C2 (`WithMaxTurns`) / Phase 1 tests |
| NFR-002 (no new config plumbing) | Full | C1–C3 / Phase 3 inspection |
| NFR-003 (`NewManager` signature stable) | Full | C2 (signature unchanged) / Phase 3 `go build` |
