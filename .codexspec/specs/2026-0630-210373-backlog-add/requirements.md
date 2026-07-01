# Confirmed Requirements: backlog-add

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0630-210373`
**Status**: Discovery Complete
**Last Confirmed**: 2026-07-01

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Backlog items must carry user-confirmed requirements

- **Status**: confirmed
- **Statement**: Provide a convenient way to produce a BACKLOG.md entry whose content is a user-confirmed requirement, so that autodev's Requirements-first SDD starts from genuinely confirmed input. autodev treats each backlog item's Description as the confirmed user input and materializes the worktree `requirements.md` from it; raw or hand-written entries are unconfirmed and undermine that guarantee. The value is the user-confirmation step, not raw text capture.
- **Rationale**: A raw `fox backlog add "title" "description"` cannot satisfy this — the confirmation/refinement of the requirement is the whole point, because autodev consumes the backlog content as authoritative.
- **User Evidence**: "现在的核心其实在于 autodev 使用的 Requirements first SDD 需要有由用户确认过的Requirements作为起点。`fox backlog add` 的方式无法满足这个需求。"
- **Confirmed At**: 2026-07-01

## Constraints

### CON-001: Appended entry must match autodev's Parse format

- **Status**: confirmed
- **Statement**: The appended entry MUST match `internal/autodev/backlog.go`'s `Parse` contract: a `## [type] Title` heading followed by `**Priority**:`, `**Status**:`, `**Description**:` field lines. `Parse` silently defaults an unknown Priority to low, so exact format matters — a malformed entry is silently mis-consumed by autodev.
- **User Evidence**: Verified from code (`backlog.go:13-90`, `parsePriority` at `92-95`) and accepted as a constraint during discovery.

### CON-002: Do not modify autodev's materialization

- **Status**: confirmed
- **Statement**: backlog-add only writes the BACKLOG.md entry. autodev's existing flow (Parse backlog → materialize the worktree `requirements.md` from the item Description) remains unchanged.
- **User Evidence**: Confirmed in the final stage summary ("不动 autodev 既有 materialization").

### CON-003: Write to autodev's configured backlog source

- **Status**: confirmed
- **Statement**: The skill appends to the same backlog autodev reads: `BACKLOG.md` by default, or the `backlog_file` configured in `.foxharness/autodev.yml` when set.
- **User Evidence**: Confirmed (paired with autodev, same source file).

## Decisions

### DEC-001: Mechanism — sibling skill `/codexspec:backlog` (R1)

- **Status**: confirmed
- **Decision**: Create a new fox-owned skill `/codexspec:backlog` (`.claude/commands/codexspec/backlog.md`) that runs a `/codexspec:specify`-style discovery + stage-summary confirmation, but skips feature-workspace/branch creation, does NOT write `requirements.md`, and does NOT auto-advance; on confirmation it appends a BACKLOG.md entry derived from the confirmed content. Accepts an optional `$ARGUMENTS` seed as the initial need.
- **Alternatives Rejected**:
  - R2 — wrap and invoke `/codexspec:specify`, then convert its output. Rejected: specify hardcodes feature-workspace + branch creation and writes `requirements.md` (plus auto_next), so "calling specify then not writing requirements" contradicts the intent and is heavy.
  - R3 — add a `--to-backlog` output mode to `/codexspec:specify` itself. Rejected: DRY but forces conditional branching across specify's feature-workspace / confirmation-output / auto_next sections, risking the core command.
- **Reason**: R1 does not touch specify (lowest risk), matches the "don't write requirements, write backlog" intent exactly, and reuses specify's discovery/confirmation semantics as a sibling.
- **User Evidence**: "如果我们新建一个自带的skill，这个skill调用 /codexspec:specify 然后将生成的内容不写入 requirements 而是写入 backlog 中" → "落地选 R1".
- **Confirmed At**: 2026-07-01

### DEC-002: v1 determinism — strict template + agent write_file

- **Status**: confirmed
- **Decision**: For v1 the skill body provides a strict, literal fill-in template and the agent appends the entry via `write_file`/`edit_file` — consistent with how `/codexspec:specify` itself has the agent write a formatted `requirements.md`. No Go code in v1.
- **Alternatives Rejected**: A dedicated `backlog_append` Go tool (deterministic formatter registered in `AgentRunner.buildRegistry`, gated so it does not leak into autodev's core agent). Deferred as future hardening — see OPEN-001.
- **Reason**: Simplest possible v1 (one skill file, zero Go code), consistent with specify's existing agent-writes-formatted-file model. Reconsider if malformed-entry rate proves non-trivial.
- **User Evidence**: AI-proposed default, accepted by user ("确认，写 requirements.md").

### DEC-003: Namespace — `/codexspec:backlog`

- **Status**: confirmed
- **Decision**: Name the skill `/codexspec:backlog`, placing it in the codexspec / specify command family.
- **Alternatives Rejected**: `/backlog` top-level (parity with `/autodev`). `/autodev` is a Go TUI builtin, not a prompt-command; this skill is a specify-variant prompt-command, so the codexspec family fits better.
- **Reason**: Semantic fit with specify.
- **User Evidence**: AI-proposed default, accepted by user.

## Out of Scope

### OUT-001: No `fox backlog add` CLI wrapper in v1

- **Status**: confirmed
- **Statement**: v1 ships only the skill. A `fox backlog add` CLI wrapper that launches the TUI and fires the skill via the prompt-command seam is deferred.
- **Reason**: Skill-as-entry is the minimal v1. The CLI wrapper needs a new programmatic entry point around `runInlinePromptCommand`/`RunWithDisplay` (the `cfg.Prompt`→`InitialPrompt` path only pre-fills the input buffer and does not auto-submit), so it is deferred.
- **User Evidence**: Confirmed in the final stage summary.

### OUT-002: No editing / listing / reordering / dedup of existing backlog items

- **Status**: confirmed
- **Statement**: v1 is append-only.
- **Reason**: Scope control.
- **User Evidence**: Confirmed.

### OUT-003: Do not modify `/codexspec:specify` (R3 rejected)

- **Status**: confirmed
- **Statement**: specify itself is not altered; the backlog target is realized as a separate sibling skill, not as a mode on specify.
- **Reason**: Avoid risk to the core specify command.
- **User Evidence**: "落地选 R1".

## Open Questions

### OPEN-001: `backlog_append` Go tool as future hardening

- **Status**: open
- **Why It Matters**: If agent-written entries misformat, autodev silently consumes a malformed item (Priority defaults to low on unknowns). A deterministic Go emitter (tool or post-step) removes that risk. Non-blocking for v1.
- **Owner**: Team — revisit after v1 usage.

### OPEN-002: Skill body — thin-delegate vs self-contained

- **Status**: open
- **Why It Matters**: The `/codexspec:backlog` body can either (a) instruct the agent to follow `/codexspec:specify`'s discovery/confirmation and override only the output, or (b) re-include the discovery rules self-contained. (a) avoids duplication but relies on the agent loading specify; (b) is more robust but risks drift. Plan-stage decision.
- **Owner**: Team — plan stage.

## Superseded Entries

### DEC-000: `fox backlog add` CLI as the entry (discussion-first + TUI)

- **Status**: superseded
- **Replaced By**: DEC-001
- **Historical Note**: Initially the feature was framed as a `fox backlog add` CLI subcommand that launches a TUI specify-style discussion. After the user clarified the core need ("confirmed requirements as the SDD starting point, not a raw add") and asked to keep it simple, the approach shifted to a specify-variant sibling skill (R1). The CLI wrapper moved to OUT-001.

## Confirmation Log

### Session 2026-07-01

- **Summary Presented**: Final stage summary — NEED-001 (confirmed-requirements goal); DEC-001 mechanism = R1 sibling skill `/codexspec:backlog`; AI-defaults DEC-002 (v1 template + write_file) and DEC-003 (`/codexspec:backlog` namespace); CON-001/002/003; OUT-001/002/003.
- **User Confirmation**: "确认，写 requirements.md"
- **Entries Confirmed**: NEED-001, CON-001, CON-002, CON-003, DEC-001, DEC-002, DEC-003, OUT-001, OUT-002, OUT-003
- **Open (non-blocking)**: OPEN-001, OPEN-002
