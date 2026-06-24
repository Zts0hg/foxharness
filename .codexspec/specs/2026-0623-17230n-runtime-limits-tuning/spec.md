# Feature Specification: Subagent Default Turn Budget

<!--
Language: Generated in English per .codexspec/config.yml (language.document: en).
-->

**Feature Branch**: `2026-0623-17230n-runtime-limits-tuning`
**Created**: 2026-06-23
**Status**: Draft
**Input**: `requirements.md` (all entries `Status: confirmed`)

## Context

The project has been repositioned from a demo harness into a practical Agent Coding Tool. The subagent's turn budget is currently hardcoded to **8** turns inside `subagent.Manager.Run`. That value is demo-sized and starves genuine engineering subtasks such as codebase exploration, multi-step refactors, and debugging — the exact work the tool is now expected to support.

Research performed during discovery (into Claude Code and Codex) established the reference baseline:

- **Main agent** turn budget: Claude Code and Codex both leave it unlimited. foxharness already defaults to `0` (unlimited) via `--max-turns`, so it already matches.
- **Context / compaction thresholds**: foxharness's `internal/compaction/thresholds.go` already mirrors Claude Code's design (20K reserved for summary, 13K auto-compact buffer, 20K warning buffer, 3K blocking buffer). Already best-practice.
- **Subagent** turn budget: Claude Code caps its subagent at **200** turns. foxharness's hardcoded **8** is the single mis-sized default.

Therefore the scope of this feature is exactly one thing: raise the subagent default turn budget to 200, uniformly across all entry points, without introducing any configuration surface.

## Goals

- Provide a subagent turn budget large enough to complete real-world coding subtasks.
- Guarantee identical subagent behavior across every code path that constructs a subagent.
- Achieve the above with the simplest possible change — a single package-level constant and no user-facing configuration.

## Non-Goals

- Changing the main agent turn budget (already unlimited; matches reference tools).
- Changing context/compaction thresholds (already mirror Claude Code).
- Changing any other hard limit (output tokens, tool concurrency, bash timeout, file-read size, retries).
- Introducing any user-facing way to override the subagent budget (CLI flag, env var, per-invocation parameter).

## User Scenarios & Testing

### User Story 1 - Real coding subtask runs to completion (Priority: P1)

As an agent developer, I want a delegated subagent to have enough turns to finish a real engineering subtask (explore a module, refactor across files, debug a failing path), so that delegation is a reliable tool rather than a truncated one.

**Why this priority**: This is the core value of the feature — without it, the tool cannot serve its new positioning as a practical coding agent. If only this story ships, the product is already improved.

**Independent Test**: Spawn a subagent on a multi-step read/refactor task that requires more than 8 tool-bearing turns; assert the subagent is not terminated at turn 8 and can continue toward the new default budget.

**Acceptance Scenarios**:

1. **Given** a subagent `Manager` built by any production entry point, **When** it runs a task, **Then** its engine's effective `MaxTurns` is 200 (not 8).
2. **Given** a subagent task that needs 12 tool-bearing turns to complete, **When** the subagent runs, **Then** it is not terminated before completion (i.e., it passes the old 8-turn cliff).

---

### User Story 2 - Uniform behavior across all entry points (Priority: P2)

As a maintainer, I want every code path that spawns a subagent (`internal/app/runner.go`, `internal/feishu/runner.go`, `internal/agentops/runner.go`) to use the same default budget, so that there is no per-entry-point divergence or surprise behavior.

**Why this priority**: Without uniformity the default would be inconsistent across the fox CLI, the Feishu integration, and the AgentOps runner — a subtle correctness hazard.

**Independent Test**: Construct a `Manager` through each entry point's construction path and assert the effective default budget equals the package constant (200) in every case.

**Acceptance Scenarios**:

1. **Given** the three subagent construction sites, **When** each builds its `Manager`, **Then** the resulting budget is `subagent.DefaultMaxTurns` (200) — identical in all three.

---

### User Story 3 - Test author can inject a small budget (Priority: P3)

As a test author, I want to inject a small turn budget into a subagent `Manager`, so that I can exercise the budget-exhaustion path deterministically and quickly without burning 200 real turns.

**Why this priority**: Required for constitution-compliant TDD coverage of the exhaustion path; supports but does not deliver end-user value on its own.

**Independent Test**: Build a `Manager` with an injected budget of 2; run a subagent whose task loops; assert it terminates at the injected budget with the existing exhaustion behavior.

**Acceptance Scenarios**:

1. **Given** a `Manager` configured with an injected budget of N, **When** the subagent runs, **Then** its engine's `MaxTurns` is N (not the 200 default).

---

### Edge Cases

- **Subagent still exhausts the budget (rare with 200)**: The engine returns `(RunResult, error)` with error `"超过最大 Turn 数限制: 200"`, the run is marked as an error, and `Manager.Run` propagates the error to the parent (the accumulated report is not returned). This is the **existing** behavior; only the threshold value changes from 8 to 200. See REQ-005 and Assumption A-1.
- **Caller signature stability**: Existing callers of `subagent.NewManager` must keep working without modification (they receive the new default automatically). See NFR-003.

## Requirements

### Functional Requirements

- **REQ-001**: The subagent engine's default maximum turn budget MUST equal the package-level constant `subagent.DefaultMaxTurns`, and that constant MUST be `200`.
  - Sources: NEED-001, DEC-001, DEC-002

- **REQ-002**: Every subagent entry point — the fox CLI/TUI path (`internal/app/runner.go`), the Feishu runner (`internal/feishu/runner.go`), and the AgentOps runner (`internal/agentops/runner.go`) — MUST obtain the default budget of `subagent.DefaultMaxTurns` (200). No entry point may diverge.
  - Sources: CON-001, DEC-002

- **REQ-003**: The subagent turn budget MUST NOT be configurable via any user-facing surface. No CLI flag, no environment variable, and no per-invocation tool parameter may override the default. The package constant is the single source of truth.
  - Sources: DEC-002, OUT-004

- **REQ-004**: The `subagent.Manager` MUST accept the maximum-turns value internally (so the default is injectable), while all production callers use the default constant. The 8-turn literal currently in `Manager.Run` MUST be replaced by this value.
  - Sources: DEC-002, CON-002

- **REQ-005** (expected error behavior, preserved): When a subagent exhausts its turn budget, the behavior MUST be unchanged from the current 8-turn semantics — the engine returns its result together with the `"超过最大 Turn 数限制: %d"` error, the run is marked as an error, and the error is propagated to the parent agent. Only the threshold value changes.
  - Sources: NEED-001 (behavioral preservation)

### Non-Functional Requirements

- **NFR-001** (testability): The default budget MUST be unit-testable by injecting a value into the `Manager`, satisfying the constitution's testability principle and enabling a fast exhaustion-path test.
  - Sources: CON-002, DEC-002

- **NFR-002** (simplicity): The change MUST NOT introduce new configuration plumbing — no CLI flag parsing, no environment-variable reading, no new config-struct field. Only a package-level constant and an internal `Manager` field are permitted.
  - Sources: DEC-002 (KISS), constitution Core Principle 5

- **NFR-003** (backward compatibility): The `subagent.NewManager` constructor signature MUST remain stable so that the three existing callers compile and behave correctly without source changes; they receive the new default automatically.
  - Sources: DEC-002, CON-001

## Success Criteria

- **SC-001**: A subagent spawned on a multi-step coding subtask can execute up to 200 turns without premature termination at the old 8-turn cliff. (User Story 1)
- **SC-002**: All three subagent entry points produce a `Manager` whose default budget is 200, verified by a unit test. (User Story 2)
- **SC-003**: No new CLI flag, environment variable, or tool parameter is introduced for the subagent budget. (User Story / REQ-003)
- **SC-004**: Existing tests covering the three entry points continue to pass with no constructor-signature changes. (NFR-003)

## Constraints

- **TDD is mandatory** (constitution Core Principle 1; CON-002): a failing test asserting the new default (and injection) MUST be written before implementation.
- **Uniformity across all three subagent entry points** is required (CON-001).
- **No user-facing configuration surface** may be added (DEC-002).

## Assumptions

- **A-1**: The existing MaxTurns-exhaustion behavior (engine returns result + `"超过最大 Turn 数限制: %d"` error, run marked as error, error propagated to parent by `Manager.Run`) is acceptable to **preserve unchanged**. The requirements confirm only that the default *value* changes (8 → 200); they do not confirm any change to termination *semantics*. Preserving it is the faithful, non-expanding interpretation and is encoded in REQ-005. This assumption MUST be confirmed if the plan or implementation considers altering exhaustion behavior.
- **A-2**: The 200-turn default aligns with Claude Code's published subagent default, established during discovery research. (Per project convention, reference-tool internal source paths are intentionally not cited in this artifact.)

## Dependencies

- None external. The change is confined to the `internal/subagent` package and the three construction sites that already depend on it.

## Out of Scope

- **Main agent maximum turns** (OUT-001): remains `0` (unlimited); already matches Claude Code and Codex.
- **Context / compaction thresholds** (OUT-002): `internal/compaction/thresholds.go` already mirrors Claude Code's design; unchanged.
- **Other hard limits** (OUT-003): output token caps, tool-call concurrency, bash timeout, file-read size, retry limits — unchanged this round.
- **Any user-facing configuration entry** (OUT-004): no CLI flag, env var, or per-invocation override (see REQ-003).

## Open Questions

_None._ No open items block later planning or implementation. (A-1 is an assumption to confirm only if exhaustion semantics are later revisited.)

## Requirements Traceability

| Confirmed Requirement | Spec Coverage | Notes |
|-----------------------|---------------|-------|
| NEED-001 | REQ-001, REQ-005, User Story 1 | Core need; behavioral preservation in REQ-005 |
| CON-001 | REQ-002, NFR-003, User Story 2 | Uniformity across 3 entry points |
| CON-002 | NFR-001, Constraints | TDD + testability |
| DEC-001 | REQ-001 | Value = 200 |
| DEC-002 | REQ-001, REQ-003, REQ-004, NFR-002 | Constant-only, no config surface; internally injectable |
| OUT-001 | Out of Scope | Main agent turns unchanged |
| OUT-002 | Out of Scope | Context thresholds unchanged |
| OUT-003 | Out of Scope | Other hard limits unchanged |
| OUT-004 | Out of Scope, REQ-003 | No user-facing config entry |
