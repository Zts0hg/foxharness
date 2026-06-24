# Tasks: Subagent Default Turn Budget

<!--
Language: Generated in English per .codexspec/config.yml (language.document: en).
Organization follows the approved plan.md (a single TDD Red → Green → Verify cycle
on one package), not the multi-story enterprise template — this feature is one
small, cohesive change with no foundational infrastructure or independent MVP slices.
-->

**Input**: `.codexspec/specs/2026-0623-17230n-runtime-limits-tuning/` (requirements.md, spec.md, plan.md)
**Prerequisites**: plan.md (approved, review-plan PASS)
**Test policy**: Test-first is **mandated** by the constitution (Core Principle 1), `CON-002`, and plan.md Phase 1 (Red) → Phase 2 (Green). T1 MUST precede T2.

## Format

`[ID] [P?] [Phase] Description` — with `Covers: REQ-xxx; Plan: <component/phase>` and exact paths.
- `[P]` = can run in parallel with sibling tasks after dependencies. None apply here: the three tasks form a strict linear TDD chain.
- All paths are in `internal/subagent/`.

---

## Phase R (Red): Failing tests

**Purpose**: Write the tests that define the new behavior and prove they fail before any implementation (constitution TDD Red phase).

- [x] **T1** [R] Add failing tests to `internal/subagent/manager_test.go` (white-box `package subagent`, reusing the existing `finalReportProvider` fake-provider pattern):
  - `TestDefaultMaxTurnsIs200` — asserts `DefaultMaxTurns == 200`.
  - `TestNewManagerDefaultsMaxTurnsTo200` — asserts a `Manager` from `NewManager(&finalReportProvider{}, t.TempDir())` has `maxTurns == DefaultMaxTurns`.
  - `TestWithMaxTurnsOverridesDefault` — asserts `NewManager(...).WithMaxTurns(3).maxTurns == 3`.
  - `TestRunHonorsInjectedMaxTurnsAndPreservesExhaustion` — build a `Manager` with `WithMaxTurns(1)` backed by a fake provider whose `Generate` returns an assistant message containing a `schema.ToolCall` (forcing the engine to loop past the budget); assert `Run` returns a non-nil error whose message contains `超过最大 Turn 数限制: 1`.
  - **Outcome**: `go test ./internal/subagent/... -run 'DefaultMaxTurns|NewManagerDefaults|WithMaxTurns|RunHonorsInjected'` compiles and **FAILS** (constant/field/setter absent; `Run` still uses literal `8`).
  - Covers: NFR-001; Plan: Phase 1, C2; CON-002 (TDD)
  - Deps: none
  - Note: Per review-plan DO-1, if crafting the looping fake provider for the exhaustion test proves impractical, that single test may fall back to the white-box field assertions (REQ-005 is structurally guaranteed because the engine exhaustion code in `engine/loop.go` is not modified). The other three tests are required as written.

**Checkpoint**: Red confirmed — the four tests exist and fail for the expected reason.

---

## Phase G (Green): Minimal implementation

**Purpose**: Make the Red tests pass with the smallest change (constitution TDD Green phase). All edits are in `internal/subagent/manager.go`.

- [x] **T2** [G] Implement the change in `internal/subagent/manager.go`:
  - **C1**: add `const DefaultMaxTurns = 200` with a godoc block comment.
  - **C2**: add `maxTurns int` field to `Manager`; set `maxTurns: DefaultMaxTurns` in `NewManager` (signature unchanged); add `func (m *Manager) WithMaxTurns(n int) *Manager` (fluent setter matching the `Composer.WithXxx` convention).
  - **C3**: in `Run`, replace the literal `MaxTurns: 8` with `MaxTurns: m.maxTurns`.
  - **Outcome**: `go test ./internal/subagent/...` — the T1 tests now **PASS**. No other package is edited; `NewManager`'s signature is unchanged, so all four construction sites (`app/runner.go:235`, `app/runner.go:894`, `feishu/runner.go:178`, `agentops/runner.go:211`) compile untouched.
  - Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, NFR-002, NFR-003; Plan: C1, C2, C3, Phase 2, Decision 1
  - Deps: T1

**Checkpoint**: Green confirmed — T1 tests pass; no call site changed.

---

## Phase V (Verify): Repo gates and regression checks

**Purpose**: Confirm the whole repository is healthy and that no scope was silently expanded (plan Phase 3).

- [x] **T3** [V] Run the verification suite:
  - `gofmt -w .` then `gofmt -l .` returns clean (constitution formatting).
  - `go build ./...` succeeds (NFR-003 — all four construction sites compile).
  - `go test ./internal/subagent/... -v` and `go test ./...` both green.
  - `grep -rnE "MaxTurns:\s*8" --include="*.go" internal/` returns nothing (the only subagent literal is gone).
  - `grep -rnE "subagent\.Manager\{|&subagent\.Manager" --include="*.go" internal/ cmd/ | grep -v _test.go` returns nothing (REQ-002 regression guard — every production subagent goes through `NewManager`).
  - Inspect the diff: no new CLI flag registration, no `os.Getenv`, no config-struct field was added (NFR-002 / REQ-003).
  - **Outcome**: all gates green; uniformity and no-config-surface confirmed.
  - Covers: NFR-002, NFR-003, REQ-002 (regression grep), REQ-003 (no-surface inspection); Plan: Phase 3
  - Deps: T2

**Checkpoint**: Verified — feature complete and repository healthy.

---

## Dependencies & Execution Order

```
T1 (Red) ──► T2 (Green) ──► T3 (Verify)
```

Strictly linear (TDD-mandated). No parallel `[P]` opportunities — each task's outcome is the precondition for the next.

## Requirements & Plan Coverage

| Plan Component / Requirement | Task | Result |
|------------------------------|------|--------|
| Phase 1 (Red tests) / NFR-001 / CON-002 | T1 | Covered |
| C1 (`DefaultMaxTurns` constant) / REQ-001 | T2 | Covered |
| C2 (field + default + `WithMaxTurns`) / REQ-001, REQ-002, REQ-004, NFR-001, NFR-003 | T2 | Covered |
| C3 (wire `m.maxTurns` into `Run`) / REQ-001, REQ-005 | T2 | Covered |
| REQ-003 (no config surface) / NFR-002 | T2 (implements none), T3 (inspects) | Covered |
| Phase 3 (Verify) / NFR-002, NFR-003, REQ-002 regression | T3 | Covered |

Spec user stories are covered transitively: US1 (real subtask runs past 8 turns) and US2 (uniformity across construction sites) by T2; US3 (test injection) by T1/T2.

## Unmapped Tasks

_None._ No polish, documentation, monitoring, abstraction, or hardening tasks are added — none are required by the approved plan or repository policy for a one-package constant change.

## Notes

- Test-first ordering is mandatory here (constitution + CON-002 + plan); do not implement T2 before T1's tests fail.
- Commit after each checkpoint (T1 Red, T2 Green, T3 Verified) per the repo's per-task commit habit.
