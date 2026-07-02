# Implementation Plan: backlog-add (`/codexspec:backlog` skill)

<!--
Language: document: en (per .codexspec/config.yml).
-->

**Related Spec**: `.codexspec/specs/2026-0630-210373-backlog-add/spec.md`
**Confirmed Requirements**: `.codexspec/specs/2026-0630-210373-backlog-add/requirements.md`
**Created**: 2026-07-01
**Status**: Draft

## Context

autodev drains `BACKLOG.md` and treats each item's `Description` as confirmed input, but today adding an item means hand-editing the file in the exact parse format (no append path exists) and the text is not user-confirmed. This feature ships a fox-owned skill `/codexspec:backlog` that runs a `/codexspec:specify`-style discovery + stage-summary confirmation and appends the **confirmed** content as one backlog item — giving autodev a genuinely confirmed starting point. Per DEC-002, v1 is a pure prompt-command skill (zero product Go logic); the deterministic `backlog_append` Go tool is deferred (OPEN-001).

## Goals / Non-Goals

**Goals**:
- One new skill `/codexspec:backlog` that converts a seed need into one user-confirmed, autodev-parseable `BACKLOG.md` entry.
- Honor the specify-family command conventions (frontmatter, `$ARGUMENTS`, language preference).
- Verify the discoverable aspects with a Go test (TDD where Go is touched).

**Non-Goals** (inherited from spec Out-of-Scope):
- No `fox backlog add` CLI wrapper (OUT-001).
- No editing/listing/reordering/dedup (OUT-002).
- No modification to `/codexspec:specify` (OUT-003).
- No `backlog_append` Go tool in v1 (OPEN-001, deferred).

## Tech Stack

- **Skill**: Markdown prompt-command (`.claude/commands/codexspec/backlog.md`), executed by the fox agent; reuses existing `ask_user_question`, `read_file`, `write_file`, `edit_file` tools.
- **Test**: Go (`internal/slash`), standard `go test`.
- No new dependencies, no new Go product code in v1.

## Architecture Overview

The skill is invoked in the TUI (`/codexspec:backlog [seed]`). Its body drives a linear flow with a human confirmation gate:

```
/codexspec:backlog "$ARGUMENTS"
   │
   ├─ 1. Resolve target file: read backlog_file from .foxharness/autodev.yml
   │      (default BACKLOG.md); if absent, create with a `# Backlog` header.
   ├─ 2. Discovery (specify-style, via ask_user_question): elicit
   │      Title, full confirmed Description, Priority, Type.
   ├─ 3. Stage-summary confirmation (explicit; silence ≠ consent).
   ├─ 4a. On confirm → append ONE entry via write_file/edit_file using the
   │       literal template (see Component C-1). Stop.
   └─ 4b. On no interactive asker / user decline → append nothing. Stop.
```

**Covers**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007

There is no feature-workspace, branch, `requirements.md`, or auto-advance (REQ-004).

## Component Structure

```
.claude/commands/codexspec/
└── backlog.md                     # C-1: the new skill (frontmatter + body)
internal/slash/
└── discovery_test.go              # C-2: add a case asserting /codexspec:backlog is discovered
```

### C-1: `.claude/commands/codexspec/backlog.md`

The skill file. Frontmatter mirrors `clarify.md` (description + argument-hint; **no `scripts`** because no feature-workspace is created — PDR-002). Body is self-contained (PDR-001) with these sections:

1. **Language Preference** (identical convention to sibling commands).
2. **User Input** — `$ARGUMENTS` (the seed need; may be empty).
3. **Goal** — produce one confirmed backlog entry; explicitly: do NOT create a feature workspace, branch, or `requirements.md`; do NOT auto-advance.
4. **Backlog Target Resolution** — read `backlog_file` from `.foxharness/autodev.yml` if present, else `BACKLOG.md`; create the file with a `# Backlog` header if it does not exist.
5. **Discovery & Confirmation** — one material question at a time, structured choices via `ask_user_question`, stage-summary confirmation; elicit Title, the full confirmed requirement substance (Description), Priority (`high`/`medium`/`low`), Type (default `[feature]`).
6. **Entry Format Template** — the literal template the agent fills and appends:

   ```markdown
   ## [<type>] <Title>

   **Priority**: <high|medium|low>
   **Status**: pending
   **Description**: <full confirmed requirement substance>
   ```

   Format rules (enforce NFR-001): `Priority` MUST be exactly `high`, `medium`, or `low` (else `parsePriority` silently defaults to `low`); `Status` MUST be exactly `pending`; append after existing content; never rewrite or reorder existing entries.

7. **Abort Conditions** — if interactive confirmation is unavailable (no `ask_user_question` asker, e.g., non-TUI), stop and append nothing.
8. **Completion** — report the appended entry and that autodev can now drain it.

**Covers**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, NFR-001, NFR-003, NFR-004

### C-2: `internal/slash/discovery_test.go` (new test case)

Add a test asserting `/codexspec:backlog` is discovered from `.claude/commands/codexspec/backlog.md`, mirroring the existing `codexspec:generate-spec` discovery test. This is the only Go code and the TDD hook for the feature.

**Covers**: REQ-001 (discoverability); necessary implementation support (regression guard on file location/name).

## Plan-Level Decisions

### PDR-001: Self-contained skill body (resolves OPEN-002)

**Context**: OPEN-002 asked whether the body is a thin delegate ("follow `/codexspec:specify`") or self-contained.
**Options Considered**: (1) thin-delegate — instruct the agent to follow specify's process and override only the output; (2) self-contained — restate the essential discovery/confirmation rules inline.
**Decision**: Self-contained.
**Rationale**: A thin-delegate relies on the agent faithfully loading and following another command — fragile. A self-contained body is explicit and robust. Accepted trade-off: minor drift risk if specify's discovery methodology changes (low — the methodology is stable); revisit if it changes.
**Covers**: REQ-002, REQ-003
**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

### PDR-002: Frontmatter omits `scripts`

**Context**: Whether `backlog.md` needs the `scripts:` frontmatter that `specify.md` carries.
**Decision**: Omit `scripts`. Only commands that create a feature workspace need it (`specify.md`, `checklist.md`, `tasks-to-issues.md`); `clarify.md` is the no-`scripts` precedent. This skill creates no workspace (REQ-004).
**Covers**: REQ-004
**Decision Level**: Plan-level; follows repository convention.

### PDR-003: Backlog target resolution via direct file read

**Context**: How the skill finds the target backlog file (CON-003).
**Decision**: The agent reads `.foxharness/autodev.yml`; if it sets `backlog_file`, use it, else `BACKLOG.md` (verified key/default: `internal/autodev/config.go:53,146`). No Go code. If the file is absent, create it with a `# Backlog` header before appending.
**Covers**: REQ-006, CON-003
**Decision Level**: Plan-level; no product-intent change.

### PDR-004: Field defaults for a new entry (resolves spec Assumption A1)

**Context**: Spec Assumption A1 deferred the default field values to the plan stage.
**Decision**: `Status` = `pending` (fixed — a new item is unstarted; the ledger, not the backlog `Status`, is authoritative); `Priority` defaults to `medium`; `Type` defaults to `[feature]`. Both are overridable during the discussion.
**Rationale**: `medium` is a neutral queue position (autodev drains high→medium→low) — it neither pre-empts existing high-priority work nor sinks to the bottom; `[feature]` is the commonest item type.
**Covers**: REQ-005, NFR-001
**Decision Level**: Plan-level implementation default; does not redefine confirmed product intent (the values were unspecified upstream).

### PDR-005: Verification fits v1 (no Go product logic)

**Context**: DEC-002 makes v1 a pure skill, so deterministic unit-testing of the emitted format is not possible in v1 (the agent writes the entry).
**Decision**: The only Go code is the discovery test (C-2, written TDD-first). Format fidelity (NFR-001) and end-to-end behavior are verified by manual acceptance — run the skill, then confirm the appended entry parses via `internal/autodev.Parse`. Deterministic formatter unit tests arrive with OPEN-001's `backlog_append` Go tool.
**Covers**: NFR-001, NFR-004, SC-001, SC-002, SC-003
**Decision Level**: Plan-level verification approach; consistent with DEC-002.

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| v1 format fidelity depends on the agent following the template (RA-001) | Medium | Medium | Strict literal template + format rules in C-1; NFR-001 Parse round-trip acceptance; OPEN-001 Go tool as hardening |
| Non-TUI / no-asker context appends unconfirmed content (RA-002) | Low | High | Explicit "abort, append nothing" instruction in C-1 §7 when `ask_user_question` is unavailable |
| Self-contained body drifts from specify (PDR-001) | Low | Low | Discovery methodology is stable; revisit if specify's rules change |

## Implementation Phases

TDD where Go is touched; the skill body itself is verified by acceptance.

### Phase 1 — Discovery test (Red)
- [ ] Add a case to `internal/slash/discovery_test.go` asserting `/codexspec:backlog` is discovered from `.claude/commands/codexspec/backlog.md` (mirror the `codexspec:generate-spec` case).
- [ ] Run `go test ./internal/slash/...` → MUST fail (file not found yet).

### Phase 2 — Skill (Green)
- [ ] Create `.claude/commands/codexspec/backlog.md` per Component C-1 (frontmatter without `scripts`; self-contained body; literal format template; abort conditions).
- [ ] Run `go test ./internal/slash/...` → discovery test passes; run `go build ./...` and `gofmt -l .`.

### Phase 3 — Behavioral acceptance
- [ ] In the TUI, run `/codexspec:backlog "test need"`, confirm, and verify: exactly one entry appended; `Status=pending`; no workspace/branch/`requirements.md` created (SC-001, SC-003).
- [ ] Cancel before confirm → file unchanged (US2).
- [ ] With `backlog_file` set in a temp `.foxharness/autodev.yml` → entry lands in that file (US3); remove the temp file after.
- [ ] Feed the emitted entry through `internal/autodev.Parse` (quick Go check or `go test`) → well-formed item, recognized Priority (NFR-001, SC-002).

## Verification Strategy

- **Unit (Go)**: discovery test (C-2) — the only automated test in v1.
- **Acceptance (manual)**: SC-001/002/003, US1–3, and the edge cases (file absent → created; malformed existing → only append; not writable → nothing written; empty seed → open prompt; non-TUI → abort).
- **Fidelity gate (NFR-001)**: the actual emitted entry must round-trip through `internal/autodev.Parse` without silent mis-fielding.
- Deterministic formatter tests are deferred to OPEN-001.

## Requirements Coverage

| Spec Requirement | Plan Coverage | Reference |
|------------------|---------------|-----------|
| REQ-001 | Full | C-1 (frontmatter/skill), C-2 (discovery test), PDR-002 |
| REQ-002 | Full | C-1 §5 (discovery), PDR-001 |
| REQ-003 | Full | C-1 §5 (stage-summary confirmation) |
| REQ-004 | Full | C-1 §3 (no workspace/requirements.md/auto-advance), PDR-002 |
| REQ-005 | Full | C-1 §6 (format template), PDR-004 |
| REQ-006 | Full | C-1 §4 (target resolution), PDR-003 |
| REQ-007 | Full | C-1 §7 (abort on no-confirmation) |
| NFR-001 | Full | C-1 §6 format rules; Phase 3 fidelity gate; PDR-005 |
| NFR-002 | Full | Non-Goals + C-1 writes only the backlog file |
| NFR-003 | Full | C-1 is a checked-in local `.md`; C-2 verifies discovery |
| NFR-004 | Full | C-1 template + write_file; PDR-005 |
