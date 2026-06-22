# Feature Specification: auto-memory

<!--
Language: Generate this document in the language specified in .codexspec/config.yml
If not configured, use English.
-->

**Feature Branch**: `2026-0621-18248t-auto-memory`
**Created**: 2026-06-21
**Status**: Draft
**Input**: Confirmed requirements record `requirements.md` (Feature ID `2026-0621-18248t`).

## Context and Goals

foxharness is a Go-based agent harness positioned as a Claude Code-equivalent. Today
it has **no cross-session persistent memory**: when a session ends, everything the
agent learned about the user, their feedback, and the project is lost. The only
session-spanning file is a flat, structure-less project `MEMORY.md`, and the
session-scoped `working_memory.md` is effectively dormant (its write methods are dead
code and no prompt drives its maintenance).

This feature adds a **cross-session persistent memory layer** — typed, file-based
memories indexed by an always-injected `MEMORY.md`, written both inline by the main
agent and by an asynchronous post-run extraction hook — while leaving `PLAN.md`,
`TODO.md`, compaction, and `AGENTS.md` instruction loading untouched. It also activates
`working_memory.md` as a session-scoped scratchpad.

**Goals**:
- The agent accumulates durable knowledge across sessions without being re-told.
- Memories are typed and structured so they can be targeted, deduplicated, and bounded.
- Capture happens both when the user explicitly asks ("remember this") and automatically
  as a backstop, without polluting or crowding out the main agent's context.
- Existing session/plan/compaction mechanisms are not disturbed.

## User Scenarios & Testing

### User Story 1 — Cross-session recall (Priority: P1)

A user tells the agent their role and preferences in one session. Days later, in a new
session in the same project, the agent already knows — without being asked — because the
relevant memory appears in the injected index and the agent reads its full content when
it becomes relevant.

**Why this priority**: Cross-session recall is the core value of the feature and the gap
foxharness most lacks. It is independently shippable.

**Independent Test**: Start a session, have the agent save a `user` memory, end the
session, start a new session in the same project, and verify the memory's index line is
injected and its full content readable. Delivers visible cross-session persistence.

**Acceptance Scenarios**:
1. **Given** a saved `user` memory in the user-global scope, **When** a new session starts
   in any project, **Then** the memory's index line is present in the injected system prompt.
2. **Given** a saved `project` memory in project A's scope, **When** a new session starts in
   project A, **Then** the index line is injected; **When** a session starts in project B,
   **Then** the project-A line is not injected.
3. **Given** the injected index, **When** the agent decides a memory is relevant, **Then** it
   can read that memory's full file via `read_file`.

---

### User Story 2 — Explicit "remember this" (Priority: P1)

A user says "remember that I prefer tests first" or "forget that note". The agent saves
or forgets the memory inline, immediately, using the existing file tools, classifying it
into the best-fit type when saving.

**Why this priority**: Explicit capture is the primary, deterministic write path and is
required for the extraction hook's mutual-exclusion logic to be meaningful.

**Independent Test**: In a session, ask the agent to remember a fact; verify a typed memory
file with correct frontmatter is created and the index updated. Then ask it to forget it;
verify the memory file is emptied, becomes non-loadable, and drops out of the index.

**Acceptance Scenarios**:
1. **Given** a user request to remember a preference, **When** the agent handles it inline,
   **Then** a memory file is created with frontmatter `name`/`description`/`type` and the
   `MEMORY.md` index gains a one-line entry.
2. **Given** a `feedback` or `project` memory, **When** it is saved, **Then** its body contains
   a `Why` and a `How to apply` structure.
3. **Given** a user request to forget a memory, **When** the agent handles it, **Then** the
   memory file content is emptied and its index line is removed.

---

### User Story 3 — Automatic capture backstop (Priority: P2)

During a run the user corrects the agent's approach ("no, don't mock the database"). The
agent, focused on the task, does not save it. After the run ends, the extraction hook
reviews the conversation and saves a `feedback` memory capturing the correction — without
the user asking and without touching the main agent's context.

**Why this priority**: Automatic capture is the headline differentiator but depends on the
P1 storage/index/inline foundation. It is the higher-cost, higher-complexity layer.

**Independent Test**: Run a conversation containing an unambiguous correction that the main
agent does not save; after run end, verify a feedback memory was written and that the main
session message log was not appended to by extraction.

**Acceptance Scenarios**:
1. **Given** a run whose messages contain a saveable signal the main agent did not write,
   **When** the run ends, **Then** an asynchronous extraction pass writes an appropriate memory.
2. **Given** a run during which the main agent already wrote to a memory directory, **When**
   the run ends, **Then** the extraction pass is skipped (mutual exclusion).
3. **Given** an existing memory covering the same topic, **When** extraction runs, **Then** it
   updates the existing file rather than creating a duplicate.

---

### User Story 4 — Session scratchpad (Priority: P2)

Within a session, the agent maintains `working_memory.md` (Goal / Known Facts / Current Plan
/ Next Step) as a living scratchpad it updates as it works, helping it stay oriented across
turns. This state is scoped to the session and does not survive it.

**Why this priority**: Activates an existing-but-dormant mechanism and is logically independent
of the persistent layer; bundled per the confirmed scope but secondary to cross-session recall.

**Independent Test**: Have the agent work a multi-step task and verify `working_memory.md` is
updated through the session and that a new session starts with a fresh scratchpad.

**Acceptance Scenarios**:
1. **Given** an active session, **When** the agent progresses through a task, **Then** it updates
   `working_memory.md` via the existing `write_file`/`edit_file` tools (no new tool).
2. **Given** a session whose `working_memory.md` has accumulated notes, **When** a new session
   starts, **Then** the new session's `working_memory.md` is fresh and does not contain the
   prior session's notes.

---

### User Story 5 — Ignore memory (Priority: P3)

A user says "ignore memory for this". The agent proceeds as if the `MEMORY.md` index were
empty — it does not apply, cite, compare against, or mention remembered facts.

**Why this priority**: A safety/determinism control; low cost (prompt-level) but only
occasionally needed.

**Independent Test**: With memories present, instruct the agent to ignore memory and verify it
neither cites nor relies on remembered content for the rest of the request.

**Acceptance Scenarios**:
1. **Given** a non-empty injected index, **When** the user instructs the agent to ignore memory,
   **Then** the agent neither cites nor acts on remembered facts for that request.

---

### Edge Cases

- **Extraction crash mid-write**: If extraction writes a memory file but fails before updating
  the index, the file must not corrupt the index and existing memories must remain intact; a
  subsequent extraction must be able to reconcile (e.g., by re-encountering the same content).
- **Index overflow**: When a scope exceeds the file/line/byte limits, the injected index is
  truncated with a visible truncation notice rather than silently dropping content.
- **Extraction LLM failure**: A failed extraction must not affect the main run's outcome (it is
  async and out-of-band) and must not corrupt existing memory files or the index.
- **Concurrent inline + extraction**: Because extraction is skipped when the main agent wrote
  during the run (mutual exclusion), simultaneous writes to the same memory are avoided by
  construction; should they nonetheless race, the last writer wins and the index must remain
  consistent with the files on disk.
- **Empty memory directories**: A scope with no memories injects no index section (or an empty
  one) without error.
- **Malformed frontmatter**: A memory file with invalid/missing frontmatter is skipped during
  index building rather than crashing the injection.

## Requirements

### Functional Requirements

- **REQ-001**: The system MUST store persistent memory under two tiers within the user home
  directory only — user-global `~/.foxharness/memory/` and project-level
  `~/.foxharness/projects/{encoded-workdir}/memory/` — and MUST NOT store persistent memory
  inside the repository.
  - Sources: NEED-001, NEED-003, CON-001, DEC-003

- **REQ-002**: The system MUST assign memory types to scopes as follows: `user` → user-global;
  `project`, `feedback`, and `reference` → project-level.
  - Sources: NEED-002, NEED-003, DEC-003

- **REQ-003**: The system MUST store each memory as an individual Markdown file with YAML
  frontmatter containing `name`, `description`, and `type`, where `type` is one of
  `user`, `feedback`, `project`, `reference`.
  - Sources: NEED-002

- **REQ-004**: The bodies of `feedback` and `project` memories MUST include a `Why` line and a
  `How to apply` line (in addition to the rule/fact itself).
  - Sources: NEED-002

- **REQ-005**: The system MUST maintain a `MEMORY.md` entry-point index at each tier. The index
  MUST have no frontmatter, MUST contain only one-line entries (target `< 150` characters) of
  the form `- [Title](file.md) — hook`, and MUST NOT contain full memory bodies.
  - Sources: NEED-003, CON-005

- **REQ-006**: On every turn, the system MUST inject the merged two-tier `MEMORY.md` index
  (descriptions only) into the system prompt.
  - Sources: NEED-003, NEED-005, DEC-004

- **REQ-007**: The system MUST bound memory size: at most ~200 memory files per scope; the
  injected index truncated at ~200 lines / ~25 KB with a truncation notice; each index line
  under ~150 characters; memory file content (excluding frontmatter) capped at ~40,000 characters.
  - Sources: CON-005

- **REQ-008**: The system MUST NOT perform per-turn AI relevance filtering. The agent expands a
  specific memory's full content on demand via `read_file` when relevant.
  - Sources: NEED-005, DEC-004, OUT-005

- **REQ-009**: The main agent MUST be able to create, update, and forget memories inline,
  driven by the shared memory system prompt. Create and update use the existing
  `write_file`/`edit_file` tools; forget is represented by writing empty content to the memory
  file with the existing file tools, making it non-loadable and dropping its index line. When
  the user explicitly asks to forget a memory, the agent MUST persist that forget. This
  persistent forget is distinct from the
  temporary "ignore memory" directive (REQ-014), which only suppresses memory for the current
  request without removing anything.
  - Sources: NEED-004, NEED-008, DEC-002, DEC-009

- **REQ-010**: At the end of every run, the system MUST asynchronously trigger an LLM extraction
  pass (fire-and-forget, non-blocking on run completion) that reviews the run's messages and
  writes memories the main agent did not capture.
  - Sources: NEED-004, DEC-002, DEC-007

- **REQ-011**: The extraction pass MUST skip itself when the main agent already wrote to a memory
  directory during that run (mutual exclusion).
  - Sources: NEED-004, CON-004, DEC-007

- **REQ-012**: The extraction pass MUST be pre-injected with the existing memory manifest and
  MUST prefer updating an existing memory file over creating a duplicate.
  - Sources: NEED-004, NEED-006, CON-004

- **REQ-013**: The extraction agent's tool permissions MUST be narrowed to the memory directory:
  read-only file tools and read-only bash are allowed; `write_file`/`edit_file` are allowed only
  for paths inside the memory directory; all other tools (MCP, subagent, write-capable bash,
  `rm`) MUST be denied.
  - Sources: CON-004

- **REQ-014**: The memory system prompt — shared by the inline and extraction layers — MUST
  encode: (a) a "what NOT to save" list (code patterns, git history, fix recipes,
  AGENTS.md/CLAUDE.md-documented content, and ephemeral task state are excluded); (b) the
  "surprising / non-obvious" heuristic as the saving criterion; (c) a memory-drift caveat; (d) a
  verify-before-recommending rule (check files/functions/flags named in a memory before
  recommending); (e) an explicit "ignore memory" directive (proceed as if the index were empty);
  and (f) a dedup-first rule.
  - Sources: NEED-006

- **REQ-015**: The system MUST activate `working_memory.md` as a session-scoped scratchpad by
  connecting a writeback path and adding prompt guidance to maintain its sections (Goal / Known
  Facts / Current Plan / Next Step), reusing the existing `write_file`/`edit_file` tools (no new
  dedicated tool).
  - Sources: NEED-007, DEC-005

- **REQ-016**: `working_memory.md` MUST remain session-scoped (it perishes with the session) and
  MUST be kept distinct from the cross-session persistent layer (its contents MUST NOT be merged
  into the persistent memory store).
  - Sources: NEED-007, DEC-001, DEC-005

- **REQ-017**: The system MUST stop reading and injecting the legacy flat `{workDir}/MEMORY.md`
  and MUST NOT automatically migrate its content into the new store.
  - Sources: CON-002, DEC-006

- **REQ-018**: The feature MUST NOT alter the behavior of `PLAN.md`/`TODO.md`, context
  compaction, or `AGENTS.md` instruction loading.
  - Sources: NEED-001, DEC-001

### Non-Functional Requirements

- **NFR-001**: The extraction hook MUST be context-isolated. It reads the run's main messages as
  read-only input only; its own turns occupy only its own message slice and MUST NEVER be
  appended to the main message log (`messages.jsonl`), the main transcript, or the main system
  prompt. It runs asynchronously and out-of-band. It consumes its own token budget, not the main
  agent's context window.
  - Sources: CON-006, DEC-007

- **NFR-002**: All new code MUST be developed test-first (Red → Green → Refactor), with error
  paths and edge cases covered by deterministic tests.
  - Sources: CON-003

- **NFR-003**: New code MUST follow Go block-level documentation standards (no teaching line
  comments; exported identifiers documented), use dependency injection for testability, and
  expose small focused interfaces.
  - Sources: CON-003

- **NFR-004**: The extraction trigger MUST be a Go-coded deterministic gate (a per-run
  end-of-run hook plus a "main agent already wrote" check), not a content-quality
  pre-classifier. The LLM decides only what to save, not whether to run.
  - Sources: DEC-002, DEC-007

- **NFR-005**: The extraction hook MUST be built on foxharness's existing engine/provider/
  subagent primitives, not a prompt-cache-sharing forked agent.
  - Sources: DEC-008

### Key Entities

- **Memory file**: An individual Markdown file with YAML frontmatter (`name`, `description`,
  `type`) and a typed body. The unit of read/write/delete. Lives in exactly one scope.
- **Memory type**: One of `user`, `feedback`, `project`, `reference`; determines scope and
  body structure.
- **Memory scope**: Either user-global (`~/.foxharness/memory/`) or project-level
  (`~/.foxharness/projects/{encoded-workdir}/memory/`). Each scope owns a `MEMORY.md` index.
- **`MEMORY.md` index**: The always-injected entry point for a scope; one-line pointers to
  memory files; bounded and truncatable.
- **Extraction hook**: The asynchronous, context-isolated, tool-narrowed post-run pass that
  backstops inline writes.
- **`working_memory.md`**: Session-scoped scratchpad (Goal / Known Facts / Current Plan / Next
  Step); separate from persistent memory.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A memory written in session 1 is observable in the injected index of a later
  session in the same project (cross-session persistence).
- **SC-002**: A `user` memory written while working in project A is observable in the injected
  index of project B (cross-project user memory).
- **SC-003**: The injected index for any scope never exceeds ~200 lines after truncation,
  regardless of the number of memory files.
- **SC-004**: A run during which the main agent writes a memory produces no second extraction
  write of the same content (mutual exclusion verifiable).
- **SC-005**: A run with extraction active leaves the main session `messages.jsonl` unchanged by
  extraction (no extraction turns appended) — verifiable by diffing the log before and after.
- **SC-006**: `working_memory.md` is fresh per session and its contents never appear in the
  persistent memory store or its index.

## Out of Scope

- **Team Memory**: Shared team-level memory with sync and sensitive-data scanning (OUT-001).
- **KAIROS daily-log mode and `/dream` nightly distillation**: Append-only daily logs and
  nightly consolidation (OUT-002).
- **CLAUDE.md instruction hierarchy and conditional rules**: Hierarchical instruction files with
  `@include` and frontmatter `paths` rules (OUT-003); foxharness already loads
  `AGENTS.md`/`.foxharness/`/`.claude/` instructions.
- **Per-subagent-type agent memory**: Memory scoped per subagent type (OUT-004).
- **Per-turn AI relevance filtering**: Claude Code-style `findRelevantMemories` per-turn
  selection (OUT-005); revisit when memory count grows.
- **`@include` directives for memory files** (OUT-006).

## Assumptions

- **ASSUME-001**: The existing `provider.LLMProvider` abstraction is sufficient for the
  extraction pass. (The exact call cadence and prompt are plan-level details.)
- **ASSUME-002**: A "run" corresponds to one user-submitted task/message, bounded by the existing
  runner run lifecycle (per the session package documentation: "each run represents one
  user-submitted task or message"). The extraction hook attaches to that run's end.

These assumptions aid readability only; they do not expand scope.

## Dependencies

- Existing `provider.LLMProvider` for extraction LLM calls.
- Existing tool registry and permission/middleware model (to narrow extraction tool permissions).
- Existing `session.Manager` and project-path encoding for the project scope key.
- Existing prompt composer (`prompt.Composer`) for system-prompt section injection.

## Requirements Traceability

| Confirmed Requirement | Spec Coverage | Notes |
|-----------------------|---------------|-------|
| NEED-001 | REQ-001, REQ-018 | Persistent layer; existing mechanisms unchanged |
| NEED-002 | REQ-003, REQ-004 | Four types + frontmatter + Why/How-to-apply |
| NEED-003 | REQ-002, REQ-005, REQ-006 | Two-tier always-injected index |
| NEED-004 | REQ-009, REQ-010, REQ-011, REQ-012, REQ-013 | Two-layer write + mutual exclusion + tool-narrowing |
| NEED-005 | REQ-006, REQ-008 | Full index injection + on-demand read |
| NEED-006 | REQ-014 | Prompt guardrails |
| NEED-007 | REQ-015, REQ-016 | Activate session-scoped scratchpad |
| NEED-008 | REQ-009 | Persistent forget/delete |
| CON-001 | REQ-001 | Storage under user home, two-tier |
| CON-002 | REQ-017 | Fresh start for legacy MEMORY.md |
| CON-003 | NFR-002, NFR-003 | TDD + Go doc/quality standards |
| CON-004 | REQ-011, REQ-012, REQ-013 | Extraction mutual exclusion + tool-narrowing |
| CON-005 | REQ-007 | Bounded index/file size |
| CON-006 | NFR-001 | Extraction context isolation |
| DEC-001 | REQ-016, REQ-018 | New persistent layer; keep PLAN/TODO/compaction/AGENTS.md |
| DEC-002 | REQ-009, REQ-010, NFR-004 | Inline + extraction hook |
| DEC-003 | REQ-001, REQ-002 | Two-tier type→scope mapping |
| DEC-004 | REQ-006, REQ-008 | Full index + on-demand read, no AI filtering |
| DEC-005 | REQ-015 | Activate working_memory.md, reuse existing tools |
| DEC-006 | REQ-017 | Legacy MEMORY.md fresh start |
| DEC-007 | REQ-010, REQ-011, NFR-001 | Async, per-run, mutual-exclusion skip |
| DEC-008 | NFR-005 | Reuse existing engine/provider/subagent primitives |
| DEC-009 | REQ-009 | Persistent forget/delete in scope |
| OUT-001 | Out of Scope | Team Memory |
| OUT-002 | Out of Scope | KAIROS daily-log + /dream |
| OUT-003 | Out of Scope | CLAUDE.md instruction hierarchy |
| OUT-004 | Out of Scope | Per-subagent-type memory |
| OUT-005 | Out of Scope (and REQ-008) | Per-turn AI relevance filtering |
| OUT-006 | Out of Scope | @include for memory files |
