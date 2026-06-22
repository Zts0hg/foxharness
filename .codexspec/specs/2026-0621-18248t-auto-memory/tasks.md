# Tasks: auto-memory

<!--
Language: Generate this document in the language specified in .codexspec/config.yml
If not configured, use English.
-->

**Input**: Design documents from `.codexspec/specs/2026-0621-18248t-auto-memory/`
**Prerequisites**: plan.md (required), spec.md (required for user stories)
**Constitution**: TDD is mandatory — every implementation task below is **test-first**: write the failing test in the named `*_test.go`, confirm it fails for the expected reason, implement the minimum code to pass, then refactor with tests green.

**Organization**: Tasks are grouped by the approved plan's technical phases (the plan's organization). User-story tags (`US1`…`US5`) are shown for traceability; infrastructure tasks are tagged `[INFRA]`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on each other).
- **[Story]**: `US1` cross-session recall · `US2` explicit remember/forget · `US3` automatic capture · `US4` session scratchpad · `US5` ignore memory · `INFRA` foundational.
- Each task is test-first (constitution) and produces one verifiable outcome.
- `Covers: REQ-xxx; Plan: <component/phase>` preserves traceability.

---

## Phase 1: Typed memory storage layer (foundational)

**Purpose**: The persistent memory data layer all user stories depend on. No story work is meaningful until this exists.

- [x] T001 [P] [INFRA] Export the project-path encoder for reuse (PLD-2).
  - Test-first in `internal/session/session_test.go`: assert `EncodeProjectPath` (new exported name) reproduces the existing `encodeProjectPath` behavior for sample paths. Then rename/export `encodeProjectPath` → `EncodeProjectPath` in `internal/session/session.go`, keeping the unexported alias if any internal caller prefers.
  - **Covers**: REQ-001; **Plan**: PLD-2 / Phase 1
  - **Deps**: none

- [x] T002 [P] [INFRA] Define memory types, scopes, and frontmatter parsing.
  - Test-first in `internal/automemory/types_test.go`: valid frontmatter parses; invalid/missing fields error; `type` restricted to `user|feedback|project|reference`; `feedback`/`project` require `Why` and `How to apply` in the body; content length rejects > ~40,000 chars. Then implement `internal/automemory/types.go` (`MemoryType`, `Scope`, `Memory`, frontmatter parse/validate).
  - **Covers**: REQ-002, REQ-003, REQ-004; **Plan**: Phase 1 types.go
  - **Deps**: none

- [x] T003 [INFRA] Resolve two-tier memory directory paths and type→scope mapping.
  - Test-first in `internal/automemory/scope_test.go`: `user` → `~/.foxharness/memory/`; `project|feedback|reference` → `~/.foxharness/projects/{key}/memory/` using `session.EncodeProjectPath`; idempotent directory creation; traversal-safe slug rejection. Then implement `internal/automemory/scope.go`.
  - **Covers**: REQ-001, REQ-002; **Plan**: Phase 1 scope.go / PLD-2
  - **Deps**: T001

- [x] T004 [INFRA] Implement the memory Store (load/save/list).
  - Test-first in `internal/automemory/store_test.go`: save→load roundtrip preserves frontmatter+body; listing skips empty or malformed files (orphan-safe, no panic); save writes atomically (temp + rename). Then implement `internal/automemory/store.go`.
  - **Covers**: REQ-001, REQ-003, REQ-004; **Plan**: Phase 1 store.go
  - **Deps**: T002, T003

- [x] T005 [INFRA] Build the per-scope index, regenerated from on-disk files (PLD-9).
  - Test-first in `internal/automemory/index_test.go`: index entries are one line, `< 150` chars, `- [Title](file.md) — hook`; seeding >200 files truncates at ~200 lines with a visible notice; ~25 KB byte cap enforced; rebuilding always reflects actual files (no drift after add/remove). Then implement `internal/automemory/index.go`.
  - **Covers**: REQ-005, REQ-007; **Plan**: Phase 1 index.go / PLD-9
  - **Deps**: T004

- [x] T006 [INFRA] Compose the merged two-tier index and the extraction manifest.
  - Test-first in `internal/automemory/merge_test.go`: `MergedIndexString()` merges user-global + project scope indexes; `Manifest()` lists existing memories (`- [type] file: description`) for extraction dedup. Then implement `MergedIndexString`/`Manifest` (on Store, in `internal/automemory/merge.go` or `store.go`).
  - **Covers**: REQ-006 (builder), REQ-012; **Plan**: Phase 1 Store.MergedIndexString/Manifest
  - **Deps**: T005

**Checkpoint**: Storage layer complete — memories can be saved, listed, indexed, and merged. Cross-session persistence is testable in isolation (SC-002, SC-003).

---

## Phase 2: Prompt injection, working_memory.md activation, legacy removal

**Purpose**: Surface memories to the agent each turn and remove the legacy flat MEMORY.md path.

- [x] T007 [US1] Author the shared memory-system prompt text (guardrails + index section).
  - Test-first in `internal/automemory/prompt_test.go`: the guardrail text contains all six elements (what-NOT-to-save, surprising/non-obvious, drift caveat, verify-before-recommending, ignore directive, dedup-first); the same guardrail source is reused for the main-agent and extraction variants. Then implement `internal/automemory/prompt.go`.
  - **Covers**: REQ-014; **Plan**: Phase 2 prompt.go
  - **Deps**: T006

- [x] T008 [US1] Inject the merged index + guardrails; remove legacy MEMORY.md injection.
  - Test-first in `internal/context/prompt_test.go`: `Compose()` output contains a "Persistent Memory" section with the merged index + guardrails and does **not** contain the legacy "Project Memory from MEMORY.md" section; AGENTS.md/Skills/Plan sections remain unchanged. Then modify `internal/context/prompt.go` (add the section, remove the `loadProjectMemory` usage at the `Compose` call site) and give the Composer access to the automemory Store.
  - **Covers**: REQ-006, REQ-008, REQ-014, REQ-017, REQ-018; **Plan**: Phase 2 Composer / PLD-7
  - **Deps**: T007

- [x] T009 [US4] Add working_memory.md maintenance guidance; keep it session-scoped.
  - Test-first in `internal/context/prompt_test.go`: `Compose()` includes guidance to maintain `working_memory.md` (Goal / Known Facts / Current Plan / Next Step) via `write_file`/`edit_file`; the guidance states it is session-scoped and distinct from persistent memory. Then modify `internal/context/prompt.go` (PLD-6).
  - **Covers**: REQ-015, REQ-016; **Plan**: Phase 2 + PLD-6
  - **Deps**: T008 (same file)

- [x] T010 [P] [INFRA] Stop creating the legacy `{workDir}/MEMORY.md`.
  - Test-first in `internal/memory/store_test.go`: `EnsureFiles()` creates `PLAN.md`/`TODO.md` but **not** `{workDir}/MEMORY.md`; the Store's PLAN/TODO behavior is otherwise unchanged. Then modify `internal/memory/store.go` (`EnsureFiles`) per PLD-7; leave existing legacy files orphaned (CON-002).
  - **Covers**: REQ-017, REQ-018; **Plan**: PLD-7
  - **Deps**: none

**Checkpoint**: The agent sees the merged persistent-memory index + guardrails and working_memory guidance every turn; the legacy MEMORY.md path is gone. US1 (cross-session recall) and US4 (scratchpad) guidance are in place.

---

## Phase 3: Inline write path + mutual-exclusion tracker

**Purpose**: Let the agent write/update/remove memories inline and detect those writes for mutual exclusion.

- [x] T011 [US2] [INFRA] Implement the memory-write tracker middleware.
  - Test-first in `internal/automemory/tracker_test.go`: a `write_file`/`edit_file` call targeting a memory-directory path sets `WroteMemory()` true; a call outside the memory dir leaves it false; the middleware returns `Allow` (it observes, does not block). Then implement `internal/automemory/tracker.go` implementing `middleware.Middleware` (PLD-5).
  - **Covers**: REQ-011, NFR-004; **Plan**: Phase 3 tracker.go / PLD-5
  - **Deps**: T003

- [x] T012 [US2] Wire the tracker into the main run and verify inline create/update/forget.
  - Test-first in `internal/app/runner_test.go` (or an automemory integration test): during a run, an inline `write_file` to a memory dir sets the tracker flag; an explicit "forget" writes empty content to the memory file, sets the tracker flag, and drops the memory from the regenerated index. Then attach the tracker in `internal/app/runner.go` (`buildRegistry`/`runInternal`) and ensure inline create/update/forget work via existing `write_file`/`edit_file`.
  - **Covers**: REQ-009, REQ-011; **Plan**: Phase 3
  - **Deps**: T004, T011

**Checkpoint**: US2 (explicit remember/forget) is functional inline; mutual-exclusion signal is available for the extraction hook.

---

## Phase 4: Extraction hook (isolated, async, narrowed)

**Purpose**: Backstop inline writes with a context-isolated, tool-narrowed, asynchronous post-run extraction.

- [x] T013 [P] [US3] [INFRA] Implement the memory-directory write-narrowing guard middleware.
  - Test-first in `internal/middleware/memorydirguard_test.go`: `write_file`/`edit_file` inside the memory dir → `Allow`; outside → `Deny`; bash → `Deny` (PLD-4 safe default when read-only classification is unavailable); subagent/MCP → `Deny`. Then implement `internal/middleware/memorydirguard.go` as a `middleware.Middleware` (location per plan component structure).
  - **Covers**: REQ-013, CON-004; **Plan**: Phase 4 + PLD-4
  - **Deps**: T003

- [x] T014 [US3] Implement the isolated extraction runner.
  - Test-first in `internal/automemory/extraction_test.go` using a fake `provider.LLMProvider`: (a) skips when the tracker flag is set (mutual exclusion); (b) writes an appropriate memory when the run messages contain a saveable signal; (c) updates an existing file rather than duplicating (dedup via pre-injected manifest); (d) **never appends to the supplied session message log** — assert by passing a recording writer and checking zero writes (CON-006/NFR-001); (e) failures are recovered and swallowed, not propagated. Then implement `internal/automemory/extraction.go` as a dedicated loop over `provider.Generate` with its own message slice and a narrowed registry (PLD-3).
  - **Covers**: REQ-010, REQ-011, REQ-012, REQ-013, NFR-001, NFR-005; **Plan**: Phase 4 extraction.go / PLD-3
  - **Deps**: T006 (manifest), T007 (extraction prompt), T011 (tracker), T013 (guard)

- [x] T015 [US3] Fire the extraction hook asynchronously at run end.
  - Test-first in `internal/app/runner_test.go`: after `runInternal` completes, the extraction hook is invoked; a panicking/failing extraction does not affect the `RunResult` and does not block run completion. Then modify `internal/app/runner.go` to launch the extractor in a goroutine with a detached `context` and `recover()` (PLD-8).
  - **Covers**: REQ-010, REQ-011, NFR-001; **Plan**: Phase 4 + PLD-8
  - **Deps**: T014

**Checkpoint**: US3 (automatic capture) is functional; extraction is isolated, narrowed, and non-blocking.

---

## Phase 5: Verification & hardening

**Purpose**: Success-criteria, edge-case, and quality-gate verification mandated by the plan and constitution.

- [x] T016 [INFRA] Add success-criteria and edge-case tests.
  - Add/extend tests covering: **SC-001** memory saved in one session appears in a new session's injected index (same project); **SC-002** a `user` memory appears in a different project's merged index; **SC-003** index truncation at the limits; **SC-004** mutual-exclusion skip; **SC-005** extraction leaves `session.MessagesPath()` unchanged (diff before/after); **SC-006** `working_memory.md` is fresh per session and never referenced by `automemory.Store`. Plus edge cases: empty memory dirs, malformed-frontmatter skip, extraction-crash no corruption, concurrent inline+exclusion holds.
  - **Covers**: REQ-016, NFR-002, SC-001…SC-006; **Plan**: Phase 5
  - **Deps**: T015

- [x] T017 [INFRA] Run quality gates and finalize documentation.
  - Run `gofmt -w .`, `go vet ./...`, `go test ./...` all green; ensure every exported identifier in `internal/automemory` and the modified `Composer`/`Store` has block-level documentation (constitution NFR-003); remove dead legacy code only where touched (e.g., unused `Store.Load`/`Bundle` memory path) without changing behavior.
  - **Covers**: NFR-003; **Plan**: Phase 5
  - **Deps**: T016

---

## Dependencies & Execution Order

```
T001 (export encoder) ──┐
T002 (types) ───────────┤
                        ├──► T003 (scope) ──► T004 (store) ──► T005 (index) ──► T006 (merge/manifest)
                        │                         │                                   │
T010 (stop legacy)      │                         │                                   ├──► T007 (prompt) ──► T008 (Composer) ──► T009 (working_mem)
                        │                         │                                   │
                        ├──► T011 (tracker) ──► T012 (wire inline)                  ├──► T014 (extractor) ──► T015 (wire async) ──► T016 (SC tests) ──► T017 (quality gates)
                        │              ▲                                            │
                        └──► T013 (guard) ──────────────────────────────────────────┘
```

- **Phase 1** is foundational; Phase 2–4 depend on it.
- **Parallel**: T001, T002, T010 are independent (`[P]`). T011 and T013 are independent after T003 (`[P]`).
- **Critical path**: T001 → T003 → T004 → T005 → T006 → T007 → T008 → (T009) and T006/T007/T011/T013 → T014 → T015 → T016 → T017.

## Execution Strategy (incremental delivery)

1. Phase 1 → storage layer testable in isolation (SC-002, SC-003).
2. + Phase 2 → US1 (cross-session recall) and US4 (scratchpad guidance) demonstrable.
3. + Phase 3 → US2 (explicit remember/forget) demonstrable.
4. + Phase 4 → US3 (automatic capture) demonstrable; US5 (ignore) is prompt-level and active from Phase 2.
5. Phase 5 → all SC-* verified; quality gates green.

## Coverage

| Requirement / Plan Item | Task References | Result |
|-------------------------|-----------------|--------|
| REQ-001 (two-tier storage) | T001, T003, T004 | Covered |
| REQ-002 (type→scope) | T002, T003 | Covered |
| REQ-003 (frontmatter) | T002, T004 | Covered |
| REQ-004 (Why/How to apply) | T002 | Covered |
| REQ-005 (index format) | T005, T006 | Covered |
| REQ-006 (inject merged index) | T006, T008 | Covered |
| REQ-007 (bounds) | T005 | Covered |
| REQ-008 (no AI filter, on-demand read) | T008 | Covered |
| REQ-009 (inline create/update/forget) | T012 | Covered |
| REQ-010 (async extraction) | T014, T015 | Covered |
| REQ-011 (mutual exclusion) | T011, T012, T014, T015 | Covered |
| REQ-012 (manifest + dedup) | T006, T014 | Covered |
| REQ-013 (tool narrowing) | T013, T014 | Covered |
| REQ-014 (guardrails) | T007, T008 | Covered |
| REQ-015 (activate working_memory) | T009 | Covered |
| REQ-016 (working_memory session-scoped) | T009, T016 (SC-006) | Covered |
| REQ-017 (drop legacy MEMORY.md) | T008, T010 | Covered |
| REQ-018 (PLAN/TODO/compaction/AGENTS.md unchanged) | T008, T010, T016 | Covered |
| NFR-001 (extraction isolation) | T014, T015, T016 (SC-005) | Covered |
| NFR-002 (TDD) | All tasks (test-first) | Covered |
| NFR-003 (Go docs/quality) | T017 (+ per-task) | Covered |
| NFR-004 (deterministic trigger) | T011 | Covered |
| NFR-005 (reuse primitives) | T014 | Covered |
| PLD-1 (new automemory package) | T002…T006, T011, T014 | Covered |
| PLD-2 (reuse EncodeProjectPath) | T001, T003 | Covered |
| PLD-3 (isolated extraction loop) | T014 | Covered |
| PLD-4 (middleware tool-narrowing) | T013 | Covered |
| PLD-5 (tracker mutual exclusion) | T011 | Covered |
| PLD-6 (working_memory via prompt) | T009 | Covered |
| PLD-7 (remove legacy MEMORY.md) | T008, T010 | Covered |
| PLD-8 (async detached goroutine) | T015 | Covered |
| PLD-9 (system-generated index) | T005, T006 | Covered |

## Unmapped Tasks
None. Every task maps to a requirement or to necessary implementation support (T001 path export, T011/T013 middleware infra, T016/T017 verification/quality gates required by the plan and constitution).
