# Tasks: backlog-add (`/codexspec:backlog` skill)

<!--
Language: document: en (per .codexspec/config.yml).
-->

**Input**: `.codexspec/specs/2026-0630-210373-backlog-add/{requirements.md, spec.md, plan.md}`
**Prerequisites**: plan.md (required), spec.md (required for acceptance scenarios)

**Tests**: The constitution mandates TDD for new code. The only new code in v1 is the discovery test (Go); test-first ordering applies to it (Plan Phase 1 → Phase 2). The skill body is Markdown (not unit-testable in v1 — see plan PDR-005) and is verified by behavioral acceptance (T003).

**Organization**: Grouped by the approved plan's technical phases (Phase 1 red test → Phase 2 skill → Phase 3 acceptance).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (none here — the sequence is TDD-ordered then acceptance).
- **[Story]**: maps to spec user stories (US1 add, US2 cancel, US3 backlog_file).
- `Covers: REQ-xxx; Plan: <component/phase>` preserves traceability.

---

## Phase 1 — Discovery test (Red)

- [x] **T001** [US1] Add a failing discovery test for `/codexspec:backlog` in `internal/slash/discovery_test.go`, mirroring `TestDiscoverCommands_ClaudeCommandsLoadedByDefault` (discovery_test.go:105-122): write `.claude/commands/codexspec/backlog.md` into a temp work dir and assert it is discovered with `Name == "codexspec:backlog"` and `Source == SourceClaudeProject`.
  - **Covers**: REQ-001; Plan: C-2 / Phase 1
  - **Outcome**: the test exists and FAILS (the skill file does not exist yet).
  - **Verify**: `go test ./internal/slash/... -run <NewTestName>` fails for the expected reason (command not discovered).

---

## Phase 2 — Skill (Green)

- [x] **T002** [US1] Create `.claude/commands/codexspec/backlog.md` per plan Component C-1: frontmatter `description` + `argument-hint` only (NO `scripts`, per PDR-002); self-contained body (PDR-001) with sections — Language Preference; User Input (`$ARGUMENTS`); Goal (explicitly: NO feature-workspace, NO branch, NO `requirements.md`, NO auto-advance); Backlog Target Resolution (read `backlog_file` from `.foxharness/autodev.yml` if present else `BACKLOG.md`; create with `# Backlog` header if absent; on any config parse trouble default to `BACKLOG.md`); Discovery & Confirmation (specify-style, `ask_user_question`, stage-summary confirmation; elicit Title, full confirmed Description, Priority, Type); Entry Format Template (the literal `## [<type>] <Title>` + `**Priority**:`/`**Status**: pending`/`**Description**:` template, with Priority ∈ {high, medium, low} exactly and "append after existing content; never rewrite/reorder" rules); Abort Conditions (if no interactive asker, stop and append nothing); Completion.
  - **Covers**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, NFR-001, NFR-003, NFR-004; Plan: C-1 / Phase 2
  - **Depends**: T001
  - **Outcome**: the skill file exists and is discoverable; T001's test now passes.
  - **Verify**: `go test ./internal/slash/... -run <NewTestName>` passes; `go build ./...`; `gofmt -l .` clean.

---

## Phase 3 — Behavioral acceptance

- [ ] **T003** [US1, US2, US3] Manual behavioral acceptance per plan Phase 3, against a scratch backlog file (do not mutate the real `BACKLOG.md`):
  - **Automatable verification (done)**: full `go test ./...` green; NFR-001 guarded by the pre-existing `TestParseWellFormedItems` (the skill's template matches that format exactly); REQ-001 discoverability confirmed both by `TestDiscoverCommands_CodexspecBacklog` and by fox live-discovering `/codexspec:backlog` from the real file.
  - **Manual TUI verification (deferred to user)**: the interactive `/codexspec:backlog` run (US1 confirm/append, US2 cancel, US3 `backlog_file` honor) requires a human-driven TTY and cannot be executed in this environment — see `issues.md`.
  1. Run `/codexspec:backlog "test need"` in the TUI, confirm → exactly one entry appended; `Status=pending`; no feature-workspace/branch/`requirements.md` created (SC-001, SC-003).
  2. Cancel before confirm → backlog file byte-identical (US2).
  3. With `backlog_file` set in a temp `.foxharness/autodev.yml` → entry lands in that file; remove the temp file after (US3).
  4. Feed the emitted entry through `internal/autodev.Parse` (quick Go check or test) → well-formed item with recognized Priority (NFR-001, SC-002); confirm autodev needs no code change to drain it (SC-002).
  - **Covers**: SC-001, SC-002, SC-003, US1, US2, US3, NFR-001, NFR-002; Plan: Phase 3
  - **Depends**: T002
  - **Outcome**: all acceptance checks pass; the scratch backlog file is cleaned up.
  - **Verify**: the Phase 3 checklist above.

---

## Dependencies & Execution Order

```
T001 (red test) → T002 (skill, makes T001 green) → T003 (acceptance)
```

Acyclic; strictly sequential (TDD ordering, then acceptance). No parallel tasks.

## Requirements Coverage

| Plan Component / Requirement | Task | Notes |
|------------------------------|------|-------|
| C-2 / REQ-001 (discoverability) | T001, T002 | TDD pair: test first, then the skill that satisfies it |
| C-1 / REQ-001..REQ-007, NFR-001, NFR-003, NFR-004 | T002 | The skill file |
| SC-001..003, US1..US3, NFR-001, NFR-002 (acceptance) | T003 | Behavioral verification; NFR-002 via check #4 (SC-002) — no task touches internal/autodev |

## Unmapped / Out of Plan

- None. No documentation, polish, abstraction, or hardening tasks are added (not required by the plan; `backlog_append` Go tool is deferred per OPEN-001).
