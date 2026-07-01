# Feature Specification: backlog-add (`/codexspec:backlog` skill)

<!--
Language: Generate this document in the language specified in .codexspec/config.yml (document: en).
-->

**Feature Branch**: `2026-0630-210373-backlog-add`
**Created**: 2026-07-01
**Status**: Draft
**Input**: Confirmed requirements at `.codexspec/specs/2026-0630-210373-backlog-add/requirements.md`

## Context and Goals

### Context

`autodev` drains `BACKLOG.md` autonomously. For each pending item it treats the item's `Description` as the **confirmed user input** and materializes a worktree `requirements.md` directly from it (it deliberately does not call `/codexspec:specify`). Today the only way to add an item is to hand-edit `BACKLOG.md` in the exact parse format — there is no append/write path anywhere in the codebase — and that hand-written text is **not** user-confirmed. So autodev's Requirements-first SDD can start from raw, unrefined input.

### Goals

Provide a convenient way to produce a `BACKLOG.md` entry whose content is a **user-confirmed requirement**, by running a `/codexspec:specify`-style discovery + stage-summary confirmation and appending the confirmed content as one backlog item. This gives autodev a genuinely confirmed starting point and closes the "convenient add" gap.

The mechanism is a new fox-owned sibling skill `/codexspec:backlog` (Decision R1) — not a CLI subcommand, not a modification to `/codexspec:specify`.

## User Scenarios & Testing

### User Story 1 — Add a confirmed backlog item (Priority: P1)

As a developer using fox/autodev, I run `/codexspec:backlog` with a short seed need, answer the clarifying questions, confirm the stage summary, and get exactly one well-formed entry appended to the backlog that autodev can subsequently drain.

**Why this priority**: This is the entire feature — a convenient, confirmed-requirements path into `BACKLOG.md`.

**Independent Test**: Invoke `/codexspec:backlog "seed need"`; after confirmation, assert `BACKLOG.md` contains one new entry that `internal/autodev.Parse` reads as a well-formed item.

**Acceptance Scenarios**:

1. **Given** a clean `BACKLOG.md`, **When** the user runs `/codexspec:backlog "support dark mode"` and confirms the stage summary, **Then** exactly one new `## [feature] ...` entry is appended with `**Status**: pending` and a non-empty `**Description**` carrying the confirmed requirement.
2. **Given** the run has started, **When** the discovery has not yet been confirmed, **Then** `BACKLOG.md` is unchanged.
3. **Given** a confirmed run completes, **Then** no feature-workspace directory, git branch, or `requirements.md` was created for the item.

### User Story 2 — Cancel without side effects (Priority: P2)

As a developer, I can abandon the discovery at any time and be certain nothing was written.

**Independent Test**: Start `/codexspec:backlog`, decline/abort before confirmation, assert `BACKLOG.md` byte-identical to before.

**Acceptance Scenarios**:

1. **Given** an in-progress discovery, **When** the user declines the stage summary or aborts, **Then** no entry is appended and no file is created.

### User Story 3 — Honor autodev's configured backlog file (Priority: P2)

As a developer whose `.foxharness/autodev.yml` sets a custom `backlog_file`, my confirmed entry is written to the same file autodev reads.

**Independent Test**: Set `backlog_file: MYLOG.md`; run and confirm; assert the entry lands in `MYLOG.md`, not `BACKLOG.md`.

**Acceptance Scenarios**:

1. **Given** `.foxharness/autodev.yml` with `backlog_file: MYLOG.md`, **When** a run confirms, **Then** the entry is appended to `MYLOG.md`.
2. **Given** no `backlog_file` configured, **When** a run confirms, **Then** the entry is appended to `BACKLOG.md`.

### Edge Cases

- **Backlog file absent**: create it with a leading `# Backlog` header before appending the first entry.
- **Existing backlog file malformed**: still append one well-formed entry; do not rewrite or "repair" existing content.
- **Backlog file not writable**: emit a clear error and write nothing (no partial entry).
- **`$ARGUMENTS` empty**: begin discovery with an open prompt rather than failing.
- **Run outside the TUI** (e.g., a non-interactive context): the discovery cannot proceed (no human `ask_user_question` asker is available); fail clearly rather than appending unconfirmed content. *(Verified repo fact: only the TUI installs a `UserAsker`.)*

## Requirements

### Functional Requirements

- **REQ-001**: The feature MUST be delivered as a fox-owned prompt-command skill `/codexspec:backlog`, discovered locally from `.claude/commands/codexspec/backlog.md`, invocable as a slash command in the TUI, accepting an optional `$ARGUMENTS` seed.
  - Sources: DEC-001, DEC-003
- **REQ-002**: The skill MUST conduct a `/codexspec:specify`-style requirement discovery — one material question at a time, structured choices where there are 2–4 options — seeded by `$ARGUMENTS`, producing a concise Title and a confirmed requirement Description.
  - Sources: NEED-001, DEC-001
- **REQ-003**: The skill MUST obtain explicit user confirmation via a stage summary before writing anything; silence or lack of objection MUST NOT be treated as confirmation.
  - Sources: NEED-001, DEC-001
- **REQ-004**: The skill MUST NOT create a CodexSpec feature workspace, MUST NOT create a git branch, MUST NOT write `requirements.md`, and MUST NOT auto-advance to `/codexspec:generate-spec`.
  - Sources: DEC-001
- **REQ-005**: On confirmation the skill MUST append exactly one backlog entry formatted to `internal/autodev/backlog.go`'s `Parse` contract: a `## [type] Title` heading followed by `**Priority**:`, `**Status**:`, and `**Description**:` field lines; `**Status**` MUST be `pending`.
  - Sources: NEED-001, CON-001, DEC-001
- **REQ-006**: The skill MUST resolve the target file as `backlog_file` from `.foxharness/autodev.yml` when set, otherwise `BACKLOG.md`; if the file does not exist it MUST create it with a `# Backlog` header before appending.
  - Sources: CON-003
- **REQ-007**: If the user does not confirm, the skill MUST NOT append any entry and MUST leave the backlog file unchanged.
  - Sources: NEED-001

### Non-Functional Requirements

- **NFR-001** (Format fidelity): The emitted entry MUST round-trip through `internal/autodev.Parse` without silent mis-fielding — in particular `Priority` must be a recognized value, because `parsePriority` silently defaults unknowns to `low`.
  - Sources: CON-001
- **NFR-002** (No autodev regression): The feature changes only `BACKLOG.md` (or the configured backlog file); it MUST NOT alter `internal/autodev` parsing or materialization.
  - Sources: CON-002
- **NFR-003** (fox-owned & local): The skill MUST be a checked-in `.md` in this repository discovered by fox's local slash discovery; no external package is introduced.
  - Sources: DEC-001
- **NFR-004** (v1 determinism): For v1 the entry format MUST be governed by a strict, literal template in the skill body and written by the agent via `write_file`/`edit_file`; the deterministic `backlog_append` Go tool is explicitly deferred (OPEN-001).
  - Sources: DEC-002

### Key Entities

- **Backlog entry** (the artifact this feature produces): the unit `internal/autodev` consumes — `{Type, Title, Priority, Status, Description}`. `Status` is advisory/initial only (the ledger is authoritative); `Description` is free text that autodev treats as the confirmed requirement source.

## Success Criteria

- **SC-001**: A confirmed `/codexspec:backlog` run yields exactly one new backlog entry that `internal/autodev.Parse` reads as a well-formed item (correct Title, recognized Priority, `Status=pending`, non-empty Description).
- **SC-002**: After an item is added, `autodev` can drain it with **no** code changes to `internal/autodev`.
- **SC-003**: No `/codexspec:backlog` run creates a feature-workspace directory, a git branch, or a `requirements.md`.

## Confirmed Constraints

- **CON-001**: Entry format must match `internal/autodev/backlog.go`'s `Parse` (`## [type] Title` + `**Priority**`/`**Status**`/`**Description**`).
- **CON-002**: autodev's existing materialization is not modified.
- **CON-003**: The target file is `backlog_file` from `.foxharness/autodev.yml` if set, else `BACKLOG.md`.

## Confirmed Decisions

- **DEC-001**: Mechanism is a sibling skill `/codexspec:backlog` (R1) — not a wrapper that calls `/codexspec:specify` (R2) and not a `--to-backlog` mode on specify (R3).
- **DEC-002**: v1 uses a strict skill-body template + agent `write_file`; the `backlog_append` Go tool is deferred.
- **DEC-003**: Namespace is `/codexspec:backlog` (specify family).

## Out of Scope

- **`fox backlog add` CLI wrapper**: deferred (OUT-001); the skill is the v1 entry point.
- **Editing / listing / reordering / dedup** of existing backlog items (OUT-002); v1 is append-only.
- **Modifying `/codexspec:specify`** or adding a backlog mode to it (OUT-003 / R3).

## Assumptions

- **A1 (field defaults)**: `Status=pending` is forced by the backlog lifecycle (a new item is unstarted; the ledger — not the backlog `Status` — is authoritative per `internal/autodev/item.go`). `Priority` defaults to `medium` and `Type` defaults to `[feature]` when not chosen during discovery; these two defaults were discussed during discovery but are **not** separately recorded in `requirements.md` and should be confirmed at the plan stage.
- **A2 (surface)**: The skill is expected to run inside a TUI session (the only surface with the human `ask_user_question` asker installed). Non-interactive contexts cannot perform the confirmation step and should fail clearly rather than append unconfirmed content.

## Dependencies

- fox slash-command discovery (`internal/slash`) — to surface `/codexspec:backlog`.
- `internal/autodev/backlog.go` `Parse` — the contract the appended entry must satisfy.
- `.foxharness/autodev.yml` (optional) — `backlog_file` resolution.

## Open Questions

- **OPEN-001** (non-blocking): Whether/when to add the deterministic `backlog_append` Go tool to harden format fidelity beyond the v1 template approach.
- **OPEN-002** (non-blocking, plan-stage): Whether the `/codexspec:backlog` body is a thin delegate to `/codexspec:specify`'s discovery process or a self-contained restatement.

> Open items remain questions. They MUST NOT be rewritten as confirmed REQ items.

## Requirements Traceability

| Confirmed Requirement | Spec Coverage | Notes |
|-----------------------|---------------|-------|
| NEED-001 | REQ-002, REQ-003, REQ-005, REQ-007; SC-001 | Confirmed-requirements goal fully covered |
| CON-001 | REQ-005, NFR-001 | Parse format + fidelity |
| CON-002 | NFR-002, SC-002 | No autodev regression |
| CON-003 | REQ-006 | Backlog source resolution |
| DEC-001 | REQ-001..REQ-005, NFR-003 | R1 sibling skill mechanism |
| DEC-002 | NFR-004 | v1 template + write_file |
| DEC-003 | REQ-001 | `/codexspec:backlog` namespace |
| OUT-001 | Out of Scope | CLI wrapper deferred |
| OUT-002 | Out of Scope | Append-only |
| OUT-003 | Out of Scope | specify not modified |
| OPEN-001 | Open Questions | Non-blocking |
| OPEN-002 | Open Questions | Non-blocking, plan-stage |
