# Confirmed Requirements: runtime-limits-tuning

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0623-17230n`
**Status**: Confirmed
**Last Confirmed**: 2026-06-23

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Subagent turn budget must support real coding subtasks

- **Status**: confirmed
- **Statement**: The subagent's default maximum turn budget must be large enough to complete real-world coding subtasks (codebase exploration, multi-step refactors, debugging) rather than the demo-sized limit in place today.
- **Rationale**: The project has been repositioned from a demo harness into a practical Agent Coding Tool; the current default of 8 turns is adequate for demos but starves genuine engineering subtasks.
- **User Evidence**: "The default max-turn limits (main agent and subagent) are too low — fine for a demo, but the project is now positioned as a real Agent Coding Tool, so these defaults must be reconsidered."
- **Confirmed At**: 2026-06-23

## Constraints

### CON-001: All subagent entry points must behave identically

- **Status**: confirmed
- **Statement**: Every code path that constructs and runs a subagent (`internal/app/runner.go`, `internal/feishu/runner.go`, `internal/agentops/runner.go`) MUST use the same fixed default turn budget. No entry point may diverge.
- **Rationale**: Uniformity is achieved only by a package-level constant shared by all three constructors; a per-entry-point override would reintroduce inconsistency.
- **User Evidence**: "Just use the constant default 200 uniformly — would that be better?" (confirmed: yes)

### CON-002: Test-Driven Development

- **Status**: confirmed
- **Statement**: Implementation MUST follow the TDD cycle mandated by the project constitution — write a failing test first (asserting the subagent uses the new default), then implement, then refactor.
- **Rationale**: Project constitution (Core Principle 1) makes TDD mandatory for all new code.
- **User Evidence**: Constitution compliance is a HIGHEST PRIORITY, non-negotiable constraint of this project.

## Decisions

### DEC-001: Default subagent turn budget is 200

- **Status**: confirmed
- **Decision**: Set the subagent default maximum turns to **200**.
- **Alternatives Rejected**:
  - 50 (a middle ground that bounds cost more tightly) — rejected as too low for real engineering subtasks.
  - 0 / unlimited — rejected because an unbounded subagent risks runaway cost without user oversight.
- **Reason**: 200 aligns with Claude Code's published subagent default and is sufficient for real coding subtasks; foxharness runs on GLM models whose cost profile makes 200 affordable.
- **User Evidence**: "200 (align with Claude Code)."
- **Confirmed At**: 2026-06-23

### DEC-002: No user-facing configuration entry — package constant only

- **Status**: confirmed
- **Decision**: Expose the default as a single package-level constant (`subagent.DefaultMaxTurns = 200`) consumed identically by all three subagent entry points. Do NOT add any user-facing configuration surface — no CLI flag, no environment variable, no per-invocation parameter. The `Manager` may still accept the value internally so tests can inject it.
- **Alternatives Rejected**:
  - Package constant + `--subagent-max-turns` CLI flag override — rejected because the flag could only reach the fox CLI path (not feishu/agentops), creating inconsistency; and cost control is better served by the main agent's existing `--max-turns` flag or model choice, not by starving subagents.
  - Package constant + environment variable override — rejected for the same uniformity reason and to avoid premature configurability.
- **Reason**: Maximizes simplicity (KISS, constitution Core Principle 5), guarantees deterministic, identical behavior across all entry points (matches the user's stated preference for deterministic control), and eliminates flag-semantics ambiguity. Configurability can be added later if a real need emerges (YAGNI).
- **User Evidence**: "Don't provide a configuration entry — wouldn't using the constant default 200 uniformly be better?"
- **Confirmed At**: 2026-06-23

## Out of Scope

### OUT-001: Main agent maximum turns

- **Status**: confirmed
- **Statement**: The main agent's `--max-turns` default remains `0` (unlimited). No change.
- **Reason**: It already matches both Claude Code and Codex (unlimited main agent) and is therefore not "too low".
- **User Evidence**: Confirmed in scoping — only the subagent default is in scope.

### OUT-002: Context / compaction thresholds

- **Status**: confirmed
- **Statement**: The compaction thresholds (`internal/compaction/thresholds.go`) remain unchanged.
- **Reason**: They already mirror Claude Code's design (20K reserved for summary, 13K auto-compact buffer, 20K warning buffer, 3K blocking buffer) and are therefore already best-practice, not "too low".
- **User Evidence**: Confirmed in scoping — context thresholds are out of scope.

### OUT-003: Other hard limits

- **Status**: confirmed
- **Statement**: Other hard limits (output token caps, tool-call concurrency, bash timeout, file-read size, retry limits) are not changed in this feature.
- **Reason**: Narrowed scope to the subagent turn budget only.
- **User Evidence**: Confirmed scoping decision: "only subagent default turns".

### OUT-004: Any user-facing configuration entry

- **Status**: confirmed
- **Statement**: No CLI flag, environment variable, or per-invocation parameter is introduced to override the subagent turn budget.
- **Reason**: Explicit decision (DEC-002) to keep behavior uniform and simple.
- **User Evidence**: "Don't provide a configuration entry."

## Open Questions

_None._ No blocking open questions remain.

## Superseded Entries

_None in this revision._ (An earlier proposal — "constant + CLI flag override" — was revised to "constant only" during discovery before being written down, so no superseded record is retained.)

## Confirmation Log

### Session 2026-06-23

- **Summary Presented**: Revised stage summary after the user proposed dropping the configuration entry: NEED-001; DEC-001 (default 200, aligns with Claude Code); DEC-002 (package constant only, no user-facing config entry); CON-001 (all entry points identical); CON-002 (TDD); OUT-001–004 (main agent turns, context thresholds, other hard limits, and any config entry all excluded).
- **User Confirmation**: "Confirmed. You may state the alignment with Claude Code, but do not expose the reference file forkSubagent.ts:65."
- **Entries Confirmed**: NEED-001, CON-001, CON-002, DEC-001, DEC-002, OUT-001, OUT-002, OUT-003, OUT-004
