# Implementation Plan: auto-memory

<!--
Language: Generate this document in the language specified in .codexspec/config.yml
If not configured, use English.
-->

**Related Spec**: `.codexspec/specs/2026-0621-18248t-auto-memory/spec.md`
**Confirmed Requirements**: `.codexspec/specs/2026-0621-18248t-auto-memory/requirements.md`
**Created**: 2026-06-21
**Status**: Draft

## Context

foxharness is a Go-based agent harness with session-scoped workspaces under
`~/.foxharness/projects/{encoded-workdir}/sessions/{id}/`. Today it has **no cross-session
persistent memory**: the only cross-session artifact is a flat, structure-less
`{workDir}/MEMORY.md`, and the session-scoped `working_memory.md` is dormant (its
`Append`/`Replace` methods are dead code; no prompt drives its maintenance).

This plan implements the confirmed `auto-memory` feature: a typed, two-tier, file-based
persistent memory layer with inline plus extraction writing, plus activation of
`working_memory.md` as a session scratchpad.

Relevant verified repository facts (used to anchor the design):

- `provider.LLMProvider.Generate(ctx, []schema.Message, []schema.ToolDefinition)` is the LLM
  primitive (`internal/provider/interface.go`).
- `tools.Registry` supports `Use(middleware.Middleware)`; `middleware.BeforeExecute` returns
  a `Decision` with `DecisionDeny` — the mechanism for narrowing tool permissions
  (`internal/tools/registry.go`, `internal/middleware`).
- `slash.NewFilteredRegistry(registry, allowedTools)` filters a registry by an allowlist
  (`internal/app/runner.go:343`).
- `subagent.Manager.buildRegistry(readOnly bool, allowedTools)` already builds a read-only
  capable registry (`internal/subagent/manager.go:55`) — a reuse target for extraction.
- `engine.NewAgentEngine(...)` + `Run/RunWithReporter` drive the main loop, and the loop
  appends to the session message log via `messageLog.Append` (`internal/engine/loop.go`).
  **Therefore the main engine must not be reused for extraction** — it would append to the
  session log and violate CON-006.
- `prompt.Composer.WithMemory(path).Compose()` injects sections, including the legacy
  "Project Memory from MEMORY.md" and "Session Working Memory" (`internal/context/prompt.go`).
- `session.encodeProjectPath` (unexported) computes the project key used in
  `~/.foxharness/projects/{key}/` (`internal/session/session.go`).
- `memory.Store` / `NewSessionStore` / `EnsureFiles` manage PLAN/TODO and the legacy
  `{workDir}/MEMORY.md` (`internal/memory/store.go`).

## Goals / Non-Goals

**Goals:**
- Add a typed (`user`/`feedback`/`project`/`reference`) persistent memory layer stored under
  `~/.foxharness/`, two-tier (user-global + project-level), indexed by an always-injected
  `MEMORY.md`.
- Write memories both inline (main agent, existing file tools) and via an asynchronous,
  context-isolated post-run extraction hook (mutual-exclusion guarded, tool-narrowed).
- Encode the full lifecycle guardrails (what-NOT-to-save, surprising/non-obvious, drift,
  verify-before-recommend, ignore, dedup-first) in the shared memory prompt.
- Activate `working_memory.md` as a session-scoped scratchpad.
- Stop using the legacy flat `{workDir}/MEMORY.md` (fresh start).

**Non-Goals (REQ-018 / OUT-*):**
- Do NOT alter `PLAN.md`/`TODO.md` behavior, context compaction, or `AGENTS.md` instruction
  loading.
- No Team Memory, KAIROS/dream, CLAUDE.md instruction hierarchy, per-subagent memory,
  per-turn AI relevance filtering, or `@include` (all OUT-* ).

## Tech Stack

- **Language**: Go 1.25+ (per README badge).
- **Existing modules reused**: `internal/provider`, `internal/tools`, `internal/middleware`,
  `internal/context` (prompt composer), `internal/session`, `internal/memory`, `internal/schema`.
- **New package**: `internal/automemory`.
- **No new external dependencies.**

## Architecture Overview

```
                         Main agent run (per user prompt)
                         ┌───────────────────────────────────────────────┐
   system prompt ◄──────┤  Composer.Compose()                            │
   (merged 2-tier       │   ├─ Persistent Memory section                 │
   MEMORY.md index      │   │    = merged index + guardrails (NEW)        │
   + guardrails)        │   ├─ Session Working Memory (working_memory.md) │
                        │   └─ AGENTS.md / Skills / … (UNCHANGED)         │
                        │                                                │
                        │  Agent acts with file tools (read/write/edit)  │
                        │   ├─ inline write/update/remove to memory dir  │── REQ-009
                        │   └─ maintain working_memory.md                │── REQ-015
                        │                                                │
                        │  memory-write tracker middleware (flag)        │── REQ-011 detect
                        └─────────────────────┬───────────────────────────┘
                                              │ run ends
                                   ┌──────────▼──────────┐
                                   │  Extraction hook    │  async, fire-and-forget (DEC-007)
                                   │  (internal/         │  • skip if tracker flagged (mutual excl)
                                   │   automemory)       │  • isolated loop over provider.Generate
                                   │                     │     with OWN message slice (CON-006)
                                   │                     │  • narrowed registry (middleware deny)
                                   │                     │  • pre-injected manifest → dedup
                                   └──────────┬──────────┘
                                              ▼
              ~/.foxharness/memory/  (user-global: user type)
              ~/.foxharness/projects/{key}/memory/  (project: project/feedback/reference)
                ├── MEMORY.md            (generated index, always injected next turn — PLD-9)
                ├── user_role.md         (frontmatter: name/description/type)
                ├── feedback_testing.md  (Why / How to apply)
                └── project_*.md
```

**Covers**: REQ-001, REQ-006, REQ-009, REQ-010, REQ-011, NFR-001

## Component Structure

```
internal/automemory/                 # NEW package
├── doc.go                           # package doc (block comment)
├── types.go                         # MemoryType, Memory record, frontmatter struct
├── scope.go                         # Scope (user-global/project), path resolution
├── store.go                         # load/save/list/validate memory files
├── index.go                         # MEMORY.md index build + truncation (CON-005)
├── prompt.go                        # shared prompt text: guardrails + index section
├── tracker.go                       # memory-write tracker (mutual-exclusion flag)
├── extraction.go                    # async post-run extraction hook (isolated loop)
└── *_test.go                        # TDD per file (NFR-002)

internal/context/prompt.go           # MODIFIED: add Persistent Memory section,
                                     #   remove legacy "Project Memory from MEMORY.md",
                                     #   add working_memory.md maintenance guidance
internal/session/session.go (or      # MODIFIED: export EncodeProjectPath for reuse
  shared util)                       #   (PLD-2)
internal/memory/store.go             # MODIFIED: stop creating/injecting legacy MEMORY.md
internal/app/runner.go               # MODIFIED: wire Composer with automemory store;
                                     #   fire async extraction at run end; attach tracker
internal/middleware/ (new file)      # NEW: memory-dir write-narrowing middleware
```

## Data Models

### Memory file (on disk, Markdown + YAML frontmatter)

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| `name` | string (frontmatter) | Short kebab-case slug / title | Required, non-empty |
| `description` | string (frontmatter) | One-line relevance signal | Required, `< 150` chars |
| `type` | enum (frontmatter) | `user` \| `feedback` \| `project` \| `reference` | Required |
| body | Markdown | The memory content | `< ~40,000` chars (CON-005) |
| `Why` / `How to apply` | Markdown lines | Required for `feedback`/`project` | Enforced by validation |

### In-memory entities (Go)

| Entity | Fields | Notes |
|--------|--------|-------|
| `MemoryType` | `user, feedback, project, reference` | Enum; maps to scope (REQ-002) |
| `Scope` | `UserGlobal`, `Project` | Determines directory (REQ-001) |
| `Memory` | `Name, Description, Type, Body` | Parsed frontmatter + body |
| `Index` | `[]IndexEntry{Title, File, Hook}` | Rendered to one-line `< 150` char entries |

No API contracts (CLI/library harness, not a service). No database — file system only.

## Decisions

### PLD-1: New package `internal/automemory`

**Context**: `internal/memory` currently owns PLAN/TODO and the legacy MEMORY.md. Persistent
typed memory is a distinct concern.

**Decision**: Implement the new subsystem in a new `internal/automemory` package.

**Rationale**: Separation of concerns (constitution principle 5); keeps the PLAN/TODO code
(REQ-018: unchanged) untouched.

**Covers**: REQ-001, REQ-018
**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

### PLD-2: Reuse `session.encodeProjectPath` for the project key

**Context**: The project scope path needs the same `{encoded-workdir}` key already used for
sessions.

**Options Considered**: (1) export `session.EncodeProjectPath` and reuse it; (2) duplicate
the encoder in `automemory`.

**Decision**: Export the existing encoder (`session.EncodeProjectPath`) and reuse it.

**Rationale**: Single source of truth for the project key; avoids drift (constitution:
reuse before new abstraction).

**Covers**: REQ-001
**Decision Level**: Plan-level.

### PLD-3: Extraction is a dedicated, isolated loop over `provider.Generate`

**Context**: CON-006 forbids appending extraction turns to the main message log/system
prompt. The main `engine.AgentEngine` appends to `session.MessagesPath()` via
`messageLog.Append`, so reusing it with the main session would violate CON-006.

**Options Considered**: (1) reuse the full engine with a throwaway session; (2) a thin,
purpose-built extraction loop over `provider.LLMProvider.Generate` with its own
`[]schema.Message` slice and a narrowed registry.

**Decision**: Option 2 — a dedicated extraction loop that calls `provider.Generate`
directly, maintains its own message slice, and never calls `messageLog.Append` nor touches
the main composer/system prompt.

**Rationale**: Guarantees CON-006 by construction (no shared mutable session state); reuses
the provider + registry primitives (DEC-008/NFR-005); simplest architecture that satisfies
the requirement.

**Trade-off**: Does not reuse the full engine loop (e.g., its compaction/recovery). Accepted
— extraction is short (bounded turns) and must be isolated.

**Covers**: REQ-010, NFR-001, NFR-005
**Decision Level**: Plan-level.

### PLD-4: Tool-narrowing for extraction via middleware on a read-only registry

**Context**: REQ-013 narrows the extraction agent's tools to the memory directory.

**Decision**: Build the extraction registry from a read-only base (mirroring
`subagent.Manager.buildRegistry(readOnly=true, …)`), then layer a `middleware.Middleware`
that `Deny`s `write_file`/`edit_file` unless the target path is inside the memory directory,
and denies subagent/MCP/write-capable tools. Bash is allowed only in read-only form when the
harness can classify it; otherwise denied (a safe, more-restrictive refinement of REQ-013).

**Rationale**: Reuses the existing `middleware` deny mechanism and the read-only registry
pattern; no new permission system.

**Trade-off**: If foxharness cannot classify read-only bash reliably, bash is denied for
extraction (more restrictive than REQ-013 allows, never less). This is a refinement, not a
product-intent change.

**Covers**: REQ-013, CON-004
**Decision Level**: Plan-level.

### PLD-5: Mutual-exclusion detection via a memory-write tracker middleware

**Context**: REQ-011 requires skipping extraction when the main agent already wrote to a
memory dir during the run.

**Decision**: Add a "memory-write tracker" middleware to the **main** run's registry that
sets a boolean flag whenever a `write_file`/`edit_file` call targets a path inside a memory
directory during the run. The extraction hook checks this flag and skips when set.

**Rationale**: Deterministic, flag-based detection (NFR-004: no content-quality classifier);
reuses the middleware primitive; naturally scoped to one run.

**Covers**: REQ-011, NFR-004
**Decision Level**: Plan-level.

### PLD-6: `working_memory.md` activation is prompt-guidance + writability (no new tool)

**Context**: REQ-015 activates `working_memory.md`; DEC-005 reuses existing tools.

**Decision**: Add prompt guidance (a Composer section) instructing the agent to maintain
`working_memory.md` (Goal / Known Facts / Current Plan / Next Step) via the existing
`write_file`/`edit_file`. The file is already injected and already writable; no new tool and
no code writeback path is added (the dead `Append`/`Replace` stay unused or are removed as
dead code during refactor).

**Rationale**: Minimal change; faithful to "reuse existing tools".

**Covers**: REQ-015, DEC-005
**Decision Level**: Plan-level.

### PLD-7: Remove legacy `{workDir}/MEMORY.md` injection and creation

**Context**: CON-002 / REQ-017 / DEC-006 require a fresh start; the legacy flat
`{workDir}/MEMORY.md` must no longer be read/injected.

**Decision**: In `prompt.Composer.Compose()`, remove the "Project Memory from MEMORY.md"
section that reads `{workDir}/MEMORY.md`. In `memory.Store.EnsureFiles()`, stop creating the
legacy `MEMORY.md` for new projects (PLAN/TODO creation is unchanged). Existing legacy files
are left on disk (orphaned) per CON-002.

**Rationale**: Directly implements the confirmed fresh-start decision; REQ-018 preserved
(PLAN/TODO/AGENTS.md untouched).

**Trade-off**: Projects with hand-authored `{workDir}/MEMORY.md` content lose injection; the
user confirmed they will handle legacy content manually (DEC-006).

**Covers**: REQ-017, CON-002, REQ-018
**Decision Level**: Plan-level.

### PLD-8: Async extraction via detached-context goroutine

**Context**: DEC-007 requires fire-and-forget, non-blocking extraction at run end.

**Decision**: After `eng.RunWithReporter` returns in `AgentRunner.runInternal`, launch a
goroutine with a fresh, detached `context` (not the run's cancelable ctx) that runs the
extraction loop. Bounded turns; all failures are logged and swallowed; the goroutine never
propagates errors to the run result.

**Rationale**: Non-blocking; survives run-context cancellation; isolation-friendly.

**Covers**: REQ-010, DEC-007, NFR-001
**Decision Level**: Plan-level.

### PLD-9: The MEMORY.md index is system-generated from on-disk files (not hand-maintained)

**Context**: REQ-005 requires a `MEMORY.md` index of one-line entries pointing to typed files.
Two implementation models exist: (1) a hand-maintained file the agent updates alongside each
memory file (Claude Code style, risking file/index drift); (2) an index generated by the
system from the actual memory files on disk at injection time.

**Decision**: Model 2 — `Store.BuildIndex(scope)` / `MergedIndexString()` regenerate the
index from the memory files present on disk. The injected index is always the source of
truth and can never drift from the files. A `MEMORY.md` file MAY be written as a generated
cache but is not hand-maintained and is not the injection source.

**Rationale**: Removes the "agent forgot to update the index" failure mode and the
file/index-drift edge case entirely; simpler and more robust; fully satisfies REQ-005
(still one-line entries pointing to files). The agent's only inline responsibilities are
create/update/remove of memory files; the index follows automatically.

**Trade-off**: Index regeneration scans the memory dir each turn; cost is bounded by CON-005
(≤ ~200 files) and acceptable.

**Covers**: REQ-005, REQ-006, REQ-009
**Decision Level**: Plan-level.

## Components / Interfaces

- **`automemory.Store`** — `Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-007`
  - `Load(scope) ([]Memory, error)`, `Save(scope, Memory) error`, `Remove(scope, name) error`,
    `BuildIndex(scope) (string, error)` (regenerated from on-disk files per PLD-9; truncated
    per CON-005), `MergedIndexString() string` (two tiers merged — the injection source of
    truth), `Manifest() string` (existing-memory list for extraction dedup). Validates
    frontmatter + Why/How-to-apply.
- **`automemory.PromptText`** — `Covers: REQ-006, REQ-014`
  - Returns the "Persistent Memory" section (merged index + full guardrail set) for the main
    Composer, and the extraction prompt variant (same guardrails + dedup instruction +
    manifest). Shared guardrail source ensures inline and extraction use identical criteria.
- **`automemory.Tracker`** (`middleware.Middleware`) — `Covers: REQ-011, NFR-004`
  - `BeforeExecute` flags memory-dir writes; `WroteMemory() bool`.
- **`automemory.Extractor`** — `Covers: REQ-010, REQ-012, REQ-013, NFR-001, NFR-005`
  - `Run(ctx, runMessages, store)`: checks tracker (skip if flagged), builds extraction
    prompt, runs the isolated `provider.Generate` loop with a narrowed registry, writes
    memories via `Store.Save` (dedup-first). Owns its message slice; never touches the
    session message log.
- **`middleware.MemoryDirGuard`** — `Covers: REQ-013, CON-004`
  - Denies `write_file`/`edit_file` outside the memory dir; denies non-read-only tools.
- **`context.Composer` (modified)** — `Covers: REQ-006, REQ-008, REQ-014, REQ-015, REQ-016, REQ-017`
  - Adds Persistent Memory section (index + guardrails), working_memory.md guidance; removes
    legacy MEMORY.md section; leaves AGENTS.md/Skills/Plan sections unchanged.
- **`app.AgentRunner` (modified)** — `Covers: REQ-009, REQ-010, REQ-011`
  - Wires `automemory.Store` into the Composer; attaches `Tracker` to the main registry; fires
    `Extractor.Run` async at run end (PLD-8).

## Implementation Phases

### Phase 1: Typed memory storage layer (TDD)
- [ ] Export `session.EncodeProjectPath` (PLD-2).
- [ ] `automemory/types.go`: `MemoryType`, `Scope`, `Memory`, frontmatter parse/validate
      (tests first: valid/invalid frontmatter, Why/How-to-apply presence for feedback/project).
- [ ] `automemory/scope.go`: resolve `~/.foxharness/memory/` and
      `~/.foxharness/projects/{key}/memory/`; type→scope mapping (REQ-002).
- [ ] `automemory/store.go`: load/save/remove/list; idempotent dir creation; orphan-safe on
      malformed frontmatter (skip, don't crash).
- [ ] `automemory/index.go`: build one-line `< 150` char entries; truncate at ~200 lines /
      ~25 KB with a notice; enforce ~200 files / ~40,000 char content cap (CON-005).
- **Covers**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-007

### Phase 2: Prompt injection + working_memory.md activation + legacy removal
- [ ] `automemory/prompt.go`: shared guardrail text (what-NOT-to-save, surprising/non-obvious,
      drift, verify-before-recommend, ignore, dedup-first) + index section builder.
- [ ] Modify `context.Composer`: inject merged two-tier index + guardrails as "Persistent
      Memory"; add `working_memory.md` maintenance guidance; **remove** legacy
      "Project Memory from MEMORY.md" section (PLD-7).
- [ ] Modify `memory.Store.EnsureFiles`: stop creating legacy `{workDir}/MEMORY.md` (keep
      PLAN/TODO). Leave existing legacy files orphaned.
- [ ] Verify AGENTS.md/Skills/Plan injection paths are untouched (REQ-018).
- **Covers**: REQ-006, REQ-008, REQ-014, REQ-015, REQ-016, REQ-017, REQ-018

### Phase 3: Inline write path + mutual-exclusion tracker
- [ ] `automemory/tracker.go`: memory-write tracker middleware (flag on memory-dir
      write/edit); `WroteMemory()`.
- [ ] Wire `Tracker` into the main registry in `AgentRunner.buildRegistry`/`runInternal`.
- [ ] Ensure inline create/update/remove (remove = delete file + drop index line) works via
      existing `write_file`/`edit_file` + file removal; forget honored on explicit request.
- **Covers**: REQ-009, REQ-011 (detection side), NFR-004

### Phase 4: Extraction hook (isolated, async, narrowed)
- [ ] `middleware.MemoryDirGuard`: deny write/edit outside memory dir; deny non-read-only tools.
- [ ] `automemory/extraction.go`: `Extractor.Run` — mutual-exclusion skip via tracker;
      pre-inject manifest; isolated `provider.Generate` loop (own message slice, bounded turns);
      dedup-first writes; async goroutine with detached context (PLD-3, PLD-8).
- [ ] Fire `Extractor.Run` at the end of `AgentRunner.runInternal` (after engine run returns).
- **Covers**: REQ-010, REQ-011 (skip side), REQ-012, REQ-013, NFR-001, NFR-005

### Phase 5: Verification & hardening
- [ ] Edge-case tests: empty memory dirs, malformed frontmatter (skip), extraction crash
      (no corruption, no main-run effect), index overflow (truncation notice), concurrent
      inline+extraction (mutual exclusion holds).
- [ ] Success-criteria tests SC-001…SC-006 (see Verification Strategy).
- [ ] `gofmt`, `go vet`, `go test ./...`; block-comment docs on exported identifiers (NFR-003).
- **Covers**: NFR-002, NFR-003, SC-001…SC-006

## Verification Strategy

TDD throughout (NFR-002): each file gets `*_test.go` written first. Key verifications map to
the spec's success criteria:

- **SC-001** (cross-session): write a memory via `Store.Save(project)`; new session's
  `Composer.Compose()` includes the index line. (Phase 1/2 test)
- **SC-002** (cross-project user memory): save a `user` memory in global scope; assert it
  appears in a different project's merged index. (Phase 1 test)
- **SC-003** (bounded index): seed >200 memories; assert `BuildIndex` truncates at ~200 lines
  with a notice. (Phase 1 test)
- **SC-004** (mutual exclusion): set the tracker flag during a run; assert `Extractor.Run`
  skips. (Phase 3/4 test)
- **SC-005** (context isolation): run extraction; diff `session.MessagesPath()` before/after;
  assert no extraction turns appended. (Phase 4 test)
- **SC-006** (working_memory.md scoping): assert `working_memory.md` lives only under the
  session dir and is never referenced by `automemory.Store`. (Phase 1/2 test)

Extraction LLM behavior is tested with a fake `provider.LLMProvider` (deterministic) so tests
stay fast and flake-free (constitution: deterministic tests).

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Extraction goroutine leaks / panics affect the process | Low | High | `recover()` in the goroutine; bounded turns; detached context; all errors logged+swallowed (PLD-8). |
| Race: inline and extraction write the same memory | Low | Medium | Mutual-exclusion tracker (PLD-5) prevents concurrent runs; `Store.Save` writes whole files atomically (write-temp + rename). |
| Index grows beyond budget at two tiers (A1) | Medium | Low | Per-scope truncation (CON-005); advisory A1 documented; revisit if context budget suffers. |
| Bash read-only classification unavailable | Medium | Low | Deny bash for extraction (more restrictive; PLD-4). |
| Legacy MEMORY.md removal surprises users with authored content | Low | Low | Documented in DEC-006/CON-002; fresh start was user-confirmed. |
| Extraction prompt drift from inline criteria | Medium | Medium | Single shared `PromptText` guardrail source (PLD: one source of truth). |

## Security Considerations

- Extraction tool-narrowing (REQ-013/PLD-4) confines writes to the memory directory; all
  other paths and destructive tools denied.
- Memory paths resolved under `~/.foxharness/`; validate against traversal (no `..` escapes
  in memory file names; names restricted to safe slug charset).
- No secrets handling beyond existing harness behavior (Team Memory / secret scanning is
  OUT-001).

## Performance Considerations

- Index injection cost is bounded (CON-005: ~200 lines / ~25 KB) and read once per turn.
- Extraction runs out-of-band async; never blocks run completion (DEC-007).
- Extraction uses its own token budget, not the main agent's (CON-006/NFR-001).

## Monitoring & Observability

- Log extraction start/skip (mutual exclusion)/completion/failure via the existing `log`
  package (consistent with `[PlanMode]`/`[slash]` style).
- No new metrics required for v1; the existing `internal/metrics` remains available if
  extraction cost needs tracking later.

## Requirements Coverage

| Spec Requirement | Plan Coverage | Reference |
|------------------|---------------|-----------|
| REQ-001 | Full | PLD-2 / Phase 1 (scope.go) |
| REQ-002 | Full | Phase 1 (type→scope mapping) |
| REQ-003 | Full | Phase 1 (types.go frontmatter) |
| REQ-004 | Full | Phase 1 (validation) + Phase 2 (prompt) |
| REQ-005 | Full | Phase 1 (index.go) |
| REQ-006 | Full | Phase 2 (Composer) |
| REQ-007 | Full | Phase 1 (index.go truncation) |
| REQ-008 | Full | Phase 2 (Composer index-only) |
| REQ-009 | Full | Phase 3 (inline create/update/remove + tracker) |
| REQ-010 | Full | Phase 4 (Extractor) + PLD-8 |
| REQ-011 | Full | Phase 3 (tracker) + Phase 4 (skip) |
| REQ-012 | Full | Phase 4 (manifest + dedup) |
| REQ-013 | Full | Phase 4 (MemoryDirGuard) + PLD-4 |
| REQ-014 | Full | Phase 2 (prompt.go guardrails) |
| REQ-015 | Full | Phase 2 (working_memory guidance) + PLD-6 |
| REQ-016 | Full | Phase 1/2 (Store never references session file) |
| REQ-017 | Full | Phase 2 (remove legacy section) + PLD-7 |
| REQ-018 | Full | Non-Goals + Phase 2 constraint |
| NFR-001 | Full | PLD-3 / Phase 4 (isolated loop) |
| NFR-002 | Full | All phases (TDD) |
| NFR-003 | Full | Phase 5 (gofmt/docs/DI) |
| NFR-004 | Full | PLD-5 / Phase 3-4 (deterministic flag, no classifier) |
| NFR-005 | Full | PLD-3 / PLD-4 (reuse provider + registry + middleware) |
