# Backlog

## [feature] Engine writes durable discoveries to MEMORY.md during runs

**Priority**: high
**Status**: pending
**Description**: During an agent run, the Engine should recognize information that is durably useful for accomplishing the user's development tasks and persist it to the project-level MEMORY.md (`internal/memory`, `Store.MemoryPath()` = `<projectDir>/MEMORY.md`). Today MEMORY.md is read into context but the Engine never writes to it, so it stays at its empty template.

Memory-worthy examples: stable project conventions, architecture/module facts, build & test commands, recurring pitfalls and their fixes, key file locations, and explicit user preferences.

The Engine should: (1) detect such facts as it works — e.g. via a dedicated memory tool the model can call, and/or an end-of-run reflection step; (2) append them to MEMORY.md in a structured, de-duplicated form (do not restate what is already recorded); (3) avoid storing transient, trivial, or session-only details; and (4) keep MEMORY.md concise so it stays useful when loaded as context on later runs.

Out of scope: redesigning the session-level `working_memory.md`. This is specifically the project-level MEMORY.md that accumulates across sessions.
