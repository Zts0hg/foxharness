# Feature: Durable Memory Writes — Engine Persists Discoveries to MEMORY.md

## Overview

The foxharness engine currently reads `MEMORY.md` into context at session start but never writes to it. This feature adds a dedicated `memory` tool that the LLM agent can invoke during a run to persist durable, project-level discoveries — stable conventions, architecture facts, build/test commands, recurring pitfalls, key file locations, and explicit user preferences — to `MEMORY.md` via the existing `internal/memory.Store`.

The tool enforces de-duplication (will not re-state what is already recorded), keeps the file concise, and avoids transient or trivial data. This enables knowledge accumulation across sessions without any engine-level "reflection" step: the model decides what is memory-worthy and calls the tool explicitly.

## Goals

- Provide a `memory` tool the LLM can call to append durable facts to `MEMORY.md`
- Enforce de-duplication so existing entries are never restated
- Keep `MEMORY.md` concise (budget-capped) so it remains useful as context
- Integrate seamlessly with the existing `internal/memory.Store` and tool registry
- Preserve current read-only `MEMORY.md` loading in the system prompt (`internal/context.Composer`)
- Remain transparent: the tool output shows what was written (or skipped due to duplication)

## User Stories

### Story 1: Agent records a project convention
**As a** developer running an agent session
**I want** the agent to notice and record that "this project uses table-driven tests" during its work
**So that** future sessions automatically know this convention without re-discovery

**Acceptance Criteria:**
- [ ] The agent invokes the `memory` tool with a fact like "Go table-driven tests are the standard pattern"
- [ ] `MEMORY.md` is updated with the new entry
- [ ] The tool output confirms the write with a summary of what was added

### Story 2: Agent avoids duplicating an existing fact
**As a** developer whose `MEMORY.md` already says "Build with `go build ./...`"
**I want** the agent to skip re-recording that fact
**So that** `MEMORY.md` stays concise and doesn't grow with redundant entries

**Acceptance Criteria:**
- [ ] The agent invokes the `memory` tool with "Build with `go build ./...`"
- [ ] The tool detects this is already recorded
- [ ] `MEMORY.md` is unchanged
- [ ] The tool output says "Already recorded, skipped"

### Story 3: Agent records multiple facts in one call
**As a** developer
**I want** the agent to batch-record several discoveries at once
**So that** tool overhead is minimized

**Acceptance Criteria:**
- [ ] The agent calls `memory` with an array of 3 facts
- [ ] All non-duplicate facts are appended
- [ ] Duplicate facts are skipped individually
- [ ] The tool output summarizes how many were added vs skipped

### Story 4: MEMORY.md stays within size budget
**As a** developer
**I want** `MEMORY.md` to never exceed a reasonable size limit
**So that** it doesn't consume excessive context window tokens

**Acceptance Criteria:**
- [ ] When a write would push `MEMORY.md` past the budget, the tool returns an error
- [ ] The error message suggests pruning older or less valuable entries
- [ ] The write is rejected (no partial append)

### Story 5: Agent records a fact during an autonomous keep-run
**As a** developer running `/keep-run`
**I want** the autonomous agent to record project discoveries to `MEMORY.md` just like an interactive session
**So that** knowledge accumulates even during unattended runs

**Acceptance Criteria:**
- [ ] The `memory` tool is available in the tool registry during keep-run sessions
- [ ] The tool writes to the project-level `MEMORY.md` (not the session worktree's)
- [ ] Writes succeed identically to interactive sessions

## Functional Requirements

### FR-001: `memory` Tool Definition
The system shall register a tool named `memory` in the tool registry (`internal/tools`). The tool definition shall be:

```json
{
  "name": "memory",
  "description": "Save durable project facts to MEMORY.md. These facts persist across sessions and are loaded into context at session start. Use this tool when you discover stable, reusable knowledge about the project — conventions, architecture, build commands, pitfalls, key file locations, or user preferences. Do NOT store transient, session-specific, or trivial details.",
  "input_schema": {
    "type": "object",
    "properties": {
      "facts": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "category": {
              "type": "string",
              "description": "Category for the fact, e.g. 'convention', 'architecture', 'build', 'testing', 'pitfall', 'location', 'preference'"
            },
            "fact": {
              "type": "string",
              "description": "The durable fact to record. Must be a self-contained, concise statement."
            }
          },
          "required": ["category", "fact"]
        },
        "minItems": 1,
        "maxItems": 10
      }
    },
    "required": ["facts"]
  }
}
```

### FR-002: Tool Registration
The `memory` tool shall be registered in `AgentRunner.buildRegistry()` alongside the existing tools (`read_file`, `write_file`, `bash`, `edit_file`, `read_todo`, `update_todo`). The tool shall receive the project directory path (the `Store.projectDir`) to locate `MEMORY.md`.

### FR-003: Append Semantics
When invoked, the tool shall:

1. Read the current contents of `MEMORY.md` (via `Store.MemoryPath()`)
2. For each fact in the `facts` array:
   a. Normalize the fact text: trim whitespace, collapse internal whitespace
   b. Check for de-duplication (FR-004)
   c. If not a duplicate, format the entry (FR-005) and append to a write buffer
3. If the write buffer is non-empty:
   a. Check the size budget (FR-006)
   b. Append all new entries to `MEMORY.md`
4. Return a structured result (FR-007)

### FR-004: De-duplication
The tool shall compare each incoming fact against existing `MEMORY.md` content using the following algorithm:

1. Extract all existing line-level entries from `MEMORY.md` (lines starting with `- `)
2. Normalize both the existing entries and the incoming fact: lowercase, trim whitespace, collapse internal whitespace
3. If the incoming fact's normalized form is a substring of any existing normalized entry (or vice versa), consider it a duplicate
4. Substring matching (rather than exact match) catches rephrasings like "uses table-driven tests" vs "table-driven tests are standard"

This is intentionally fuzzy. False positives (skipping a novel fact that happens to share a substring) are acceptable to avoid clutter. The model can always rephrase and retry.

### FR-005: Entry Format
Each new entry appended to `MEMORY.md` shall follow this format:

```markdown
- **{category}**: {fact}
```

Example resulting `MEMORY.md`:

```markdown
# MEMORY

- **convention**: Go table-driven tests are the standard pattern for unit tests
- **build**: Build with `go build ./...`
- **testing**: Run all tests with `go test ./...`
- **architecture**: Tool registry is in `internal/tools/registry.go`; all tools implement `BaseTool` interface
- **pitfall**: `internal/context/prompt.go` uses Chinese error messages — do not change without reason
```

The `# MEMORY` header is preserved. New entries are appended after all existing content.

### FR-006: Size Budget
The tool shall enforce a maximum `MEMORY.md` size of **4000 characters** (approximately 1000 tokens). This limit is configurable via a constant in the tool implementation.

When appending new entries would push the file past this budget:
- The tool shall reject the entire write (no partial append)
- Return an error: "MEMORY.md is at capacity (N/M characters). Consider consolidating or pruning entries."
- The model may respond by using `edit_file` to prune or consolidate existing entries before retrying

### FR-007: Tool Output
The tool shall return a JSON-formatted result string:

**On success (some facts added, some skipped):**
```json
{
  "added": 2,
  "skipped": 1,
  "details": [
    {"fact": "Go table-driven tests...", "status": "added"},
    {"fact": "Build with go build", "status": "skipped", "reason": "similar to existing entry"},
    {"fact": "Run tests with go test", "status": "added"}
  ]
}
```

**When all facts are duplicates:**
```json
{
  "added": 0,
  "skipped": 3,
  "details": [
    {"fact": "...", "status": "skipped", "reason": "similar to existing entry"}
  ]
}
```

**On budget exceeded:**
```json
{
  "error": "MEMORY.md is at capacity (3980/4000 characters). Consider consolidating or pruning entries.",
  "would_add": 2
}
```

### FR-008: Tool File Location
The tool implementation shall live in `internal/tools/memory.go` with tests in `internal/tools/memory_test.go`, following the existing pattern (`bash.go`, `edit_file.go`, etc.).

### FR-009: System Prompt Guidance
The `memoryInstructions()` function in `internal/context/prompt.go` shall be updated to include guidance on when and how to use the `memory` tool:

```
- Use the `memory` tool to persist durable project facts to MEMORY.md.
- Invoke it when you discover stable, reusable knowledge: conventions, architecture, build/test commands, pitfalls, key file locations, or user preferences.
- Do NOT store transient details, raw logs, or session-specific observations.
- Prefer recording fewer high-value facts over many trivial ones.
```

### FR-010: No Automatic Writing
The engine shall NOT automatically write to `MEMORY.md` without an explicit tool invocation. There shall be no "end-of-run reflection" step, no background writer, and no implicit memory capture. All writes are model-initiated through the `memory` tool. This keeps the model in control of what is memory-worthy.

### FR-011: Worktree-Aware Path Resolution
When running inside a git worktree (e.g., during `/keep-run`), the tool shall write to the **project root's** `MEMORY.md`, not the worktree's. The tool receives the project directory path at registration time (via `AgentRunner.workDir`), which is always the main project root regardless of worktree context.

### FR-012: Error Handling
The tool shall handle the following error scenarios:

| Scenario | Behavior |
|----------|----------|
| `MEMORY.md` does not exist | Create it with the default header `# MEMORY\n\n` then proceed with the append |
| File permission denied | Return error: "Cannot write to MEMORY.md: permission denied" |
| Concurrent write (file changed between read and write) | Read → append → write atomically (write full content, not append-only I/O) to reduce race window |
| Invalid input (empty fact, missing category) | Return validation error per-fact, skip invalid entries |

### FR-013: Parallel Safety
The `memory` tool shall NOT be marked as parallel-safe (`ParallelSafe()` returns false). Concurrent writes to the same file from parallel tool batches could cause data loss. The tool must execute sequentially.

## Non-Functional Requirements

### NFR-001: Performance
The `memory` tool shall complete in under 50ms for typical operations (read + de-dup check + write). File I/O on `MEMORY.md` (max 4KB) is trivially fast.

### NFR-002: Reliability
Writes shall be atomic: the tool reads the full file, appends in memory, and writes the complete result. This avoids partial writes on crash. The existing `MEMORY.md` is never left in a corrupted state.

### NFR-003: Testability
The tool shall be unit-testable with a mocked filesystem (an interface matching the pattern used by `internal/toolresult`'s `FileSystem` interface). All de-duplication, formatting, budget enforcement, and error handling logic shall be testable without real file I/O.

### NFR-004: Compatibility
The tool shall not change the existing `MEMORY.md` loading behavior in `internal/context.Composer`. The file format produced by the tool must be compatible with the current read path (plain Markdown with `- ` list items).

### NFR-005: Determinism
Given the same `MEMORY.md` contents and the same input facts, the tool shall always produce identical output. No randomness, no timestamps in entries.

## Acceptance Criteria (Test Cases)

### TC-001: Basic append
Given a `MEMORY.md` containing only `# MEMORY\n\n`
When the `memory` tool is called with `facts: [{"category": "build", "fact": "Build with go build ./..."}]`
Then `MEMORY.md` shall contain `- **build**: Build with go build ./...`
And the tool output shall show `added: 1, skipped: 0`

### TC-002: De-duplication — exact match
Given a `MEMORY.md` containing `- **build**: Build with go build ./...`
When the `memory` tool is called with `facts: [{"category": "build", "fact": "Build with go build ./..."}]`
Then `MEMORY.md` shall be unchanged
And the tool output shall show `added: 0, skipped: 1`

### TC-003: De-duplication — substring match
Given a `MEMORY.md` containing `- **convention**: table-driven tests are standard`
When the `memory` tool is called with `facts: [{"category": "testing", "fact": "uses table-driven tests"}]`
Then `MEMORY.md` shall be unchanged (substring match detected)
And the tool output shall show `added: 0, skipped: 1`

### TC-004: Mixed add and skip
Given a `MEMORY.md` containing `- **build**: go build ./...`
When the `memory` tool is called with 3 facts, one of which is a duplicate
Then 2 facts shall be appended, 1 skipped
And the tool output shall detail each fact's status

### TC-005: Budget enforcement
Given a `MEMORY.md` that is 3900 characters
When the `memory` tool is called with facts that would push the total past 4000 characters
Then the write shall be rejected
And the tool output shall show the capacity error with current and max sizes

### TC-006: Budget not exceeded
Given a `MEMORY.md` that is 3900 characters
When the `memory` tool is called with facts totaling 50 characters (within budget)
Then the write shall succeed
And `MEMORY.md` shall be 3950 characters

### TC-007: Empty MEMORY.md creation
Given no `MEMORY.md` file exists
When the `memory` tool is called
Then `MEMORY.md` shall be created with the `# MEMORY` header followed by the new entries

### TC-008: Invalid input handling
When the `memory` tool is called with an empty fact string
Then the tool shall skip that entry and return a validation error for it
And other valid facts in the same call shall still be processed

### TC-009: Too many facts in one call
When the `memory` tool is called with more than 10 facts
Then the tool shall return an error: "too many facts (max 10 per call)"

### TC-010: Tool is registered
Given a standard `AgentRunner` session
When `buildRegistry()` is called
Then the returned registry shall include a tool named `memory`

### TC-011: Parallel safety
The `memory` tool's `ParallelSafe()` method shall return `false`

### TC-012: System prompt includes memory tool guidance
When the system prompt is composed via `Composer.Compose()`
Then the "Persistent File Memory" section shall mention the `memory` tool and when to use it

### TC-013: Atomic write
Given a `MEMORY.md` with existing content
When the `memory` tool appends new entries
Then the file shall be written as a complete replacement (read-modify-write), not appended via O_APPEND
This ensures the file is never in a partial state

## Edge Cases

- **MEMORY.md is read-only**: Tool returns permission error, model can inform the user
- **Concurrent tool calls**: Not parallel-safe, so the engine serializes them. If two calls somehow race, the read-modify-write pattern means the second call's read sees the first call's write (acceptable)
- **Fact with special characters**: Facts containing backticks, quotes, or markdown syntax shall be preserved as-is (no escaping beyond normal markdown)
- **Category with special characters**: Categories shall be sanitized — only `[a-z0-9-]` allowed; others replaced with `-`
- **Very long single fact**: A single fact exceeding 500 characters shall be rejected with a validation error
- **All facts are duplicates**: Tool returns success with `added: 0`, not an error
- **Empty facts array**: Tool returns a validation error (minItems: 1 enforced by schema)

## Output Format Examples

### MEMORY.md Before Tool Call
```markdown
# MEMORY

- **build**: Build with `go build ./...`
- **testing**: Run all tests with `go test ./...`
```

### Tool Input
```json
{
  "facts": [
    {"category": "convention", "fact": "Go table-driven tests are the standard pattern"},
    {"category": "build", "fact": "Build with go build ./..."},
    {"category": "architecture", "fact": "Tools implement BaseTool interface in internal/tools"}
  ]
}
```

### MEMORY.md After Tool Call
```markdown
# MEMORY

- **build**: Build with `go build ./...`
- **testing**: Run all tests with `go test ./...`
- **convention**: Go table-driven tests are the standard pattern
- **architecture**: Tools implement BaseTool interface in internal/tools
```

### Tool Output
```json
{
  "added": 2,
  "skipped": 1,
  "details": [
    {"fact": "Go table-driven tests are the standard pattern", "status": "added"},
    {"fact": "Build with go build ./...", "status": "skipped", "reason": "similar to existing entry"},
    {"fact": "Tools implement BaseTool interface in internal/tools", "status": "added"}
  ]
}
```

## Out of Scope

- Automatic / end-of-run reflection writing to MEMORY.md (FR-010 explicitly prohibits this)
- Redesigning or modifying the session-level working memory system
- Memory versioning or history tracking
- Memory search or query capabilities beyond de-duplication
- User confirmation / approval workflow before writes
- Migration of existing empty MEMORY.md files
- Memory pruning or summarization strategies (model handles this manually via `edit_file`)
- Changes to how MEMORY.md is loaded into context (read path unchanged)
