# Implementation Plan: Slash Commands System

**Related Spec**: `.codexspec/specs/2026-0524-2343mf-slash-commands/spec.md`
**Created**: 2026-05-25
**Status**: Draft

## Context

foxharness-go currently has 10 hardcoded slash commands in `internal/tui/model.go` defined as a `slashCommand` struct slice and dispatched via a `handleSlashCommand()` switch statement. The goal is to transform this into an extensible, file-based command system where users create `.md` files to add new commands — modeled after Claude Code's slash commands architecture.

## Goals / Non-Goals

**Goals:**

- Implement file-based command discovery from `.foxharness/commands/` and `.foxharness/skills/`
- Support YAML frontmatter with 15 configurable fields
- Replace hardcoded command dispatch with a unified registry
- Enable the LLM agent to invoke skills autonomously via a new `skill` tool
- Support argument substitution, shell embedding, conditional activation, and fork mode

**Non-Goals:**

- Runtime subdirectory skill discovery (deferred)
- Usage tracking and dynamic ranking (deferred)
- File watching for cache invalidation (using explicit refresh)
- MCP remote skills, plugin skills, managed/policy commands

## Tech Stack

| Category | Technology | Version | Notes |
|----------|------------|---------|-------|
| Language | Go | 1.25.0 | Per go.mod |
| TUI Framework | bubbletea | v1.3.10 | Existing |
| YAML Parsing | gopkg.in/yaml.v3 | v3.0.1 | Already imported |
| Glob Matching | github.com/bmatcuk/doublestar | v4.8.1 | For `**` support in paths (new dependency) |
| Testing | Go testing + table-driven | stdlib | Per constitution |
| Caching | In-memory with explicit refresh | N/A | No fsnotify for MVP |

## Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| 1. TDD | ✅ | Each phase starts with test files. 32 test cases from spec drive implementation. |
| 2. Code Quality | ✅ | Interfaces defined before implementations (Registry, Executor). Dependencies injectable via constructor functions. |
| 3. Go Documentation | ✅ | Block comments on all exported identifiers. `doc.go` for multi-file packages. No teaching line comments. |
| 4. Testing Standards | ✅ | Test files mirror package structure. Table-driven tests for multi-scenario requirements. Edge cases tested explicitly. |
| 5. Architecture | ✅ | `internal/slash/` has single responsibility. Public API limited to Registry interface and types. Internal details not leaked. |
| 6. Performance | ✅ | NFR targets addressed: 100ms loading, 10ms autocomplete, 1ms substitution. Caching reduces repeated work. |
| 7. Security | ✅ | Shell embedding validated, timeout enforced, allowed-tools restriction at registry level, path traversal prevention. |

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────────┐
│                          TUI (bubbletea)                          │
│  ┌──────────────┐  ┌──────────────────┐  ┌─────────────────────┐ │
│  │ Input Handler │─▶│ Registry Lookup  │─▶│ Command Dispatch    │ │
│  │ + Fuzzy Match │  │ (autocomplete)   │  │ (builtin/prompt)    │ │
│  └──────────────┘  └──────────────────┘  └──────────┬──────────┘ │
│                                                       │            │
└───────────────────────────────────────────────────────┼────────────┘
                                                        │
┌───────────────────────────────────────────────────────┼────────────┐
│  internal/slash/                                      │            │
│  ┌─────────────────────────────────────────────────┐  │            │
│  │              CommandRegistry                     │◀─┘            │
│  │  ┌──────────┐ ┌───────────┐ ┌────────────────┐  │               │
│  │  │ Built-in │ │ Prompt    │ │ Conditional    │  │               │
│  │  │ Commands │ │ Commands  │ │ Skills (dorm.) │  │               │
│  │  └──────────┘ └─────┬─────┘ └───────┬────────┘  │               │
│  │                     │               │            │               │
│  │  ┌──────────────────┼───────────────┘            │               │
│  │  │ Executor         │                            │               │
│  │  │ ┌──────────────┐ │                            │               │
│  │  │ │ Arg Subst    │ │                            │               │
│  │  │ │ Shell Embed  │ │                            │               │
│  │  │ │ Var Replace  │ │                            │               │
│  │  │ │ Hooks        │ │                            │               │
│  │  │ └──────────────┘ │                            │               │
│  │  └──────────────────┘                            │               │
│  └─────────────────────────────────────────────────┘               │
│                                                                     │
│  ┌──────────────────┐    ┌──────────────────┐                      │
│  │ Discovery        │    │ Fuzzy            │                      │
│  │ (file loader)    │    │ (autocomplete)   │                      │
│  └──────────────────┘    └──────────────────┘                      │
└─────────────────────────────────────────────────────────────────────┘
                        │
┌───────────────────────┼─────────────────────────────────────────────┐
│  internal/slash/skilltool/                                          │
│  ┌───────────────────┐                                              │
│  │ SkillTool         │◀── LLM invokes via tool call                 │
│  │ (BaseTool impl)   │                                              │
│  └────────┬──────────┘                                              │
│           │ uses registry.Lookup() + executor                       │
└───────────┼─────────────────────────────────────────────────────────┘
            │
┌───────────┼─────────────────────────────────────────────────────────┐
│  internal/engine/loop.go                                            │
│  ┌───────────────────┐                                              │
│  │ After tool exec   │──▶ registry.CheckConditional(filepath)       │
│  └───────────────────┘                                              │
└─────────────────────────────────────────────────────────────────────┘
```

## Component Structure

```
internal/slash/
├── doc.go                 # Package documentation
├── command.go             # Command, CommandType, CommandSource, Frontmatter types
├── registry.go            # CommandRegistry: Register, Lookup, All, UserInvocable, ModelInvocable
├── discovery.go           # File discovery: directory traversal, .md loading, namespacing
├── frontmatter.go         # YAML frontmatter parsing from .md content
├── arguments.go           # Argument substitution: $ARGUMENTS, $0, $1, named params
├── executor.go            # Command execution: inline/fork modes, orchestrates pipeline
├── filter.go              # FilteredRegistry: allowed-tools restriction wrapper
├── shell.go               # Shell command embedding: !`cmd` syntax parsing and execution
├── variables.go           # Special variable replacement: ${FOXHARNESS_*}
├── fuzzy.go               # Fuzzy search scoring for autocomplete
├── conditional.go         # Conditional activation via paths glob matching
├── hooks.go               # Before/after shell hook execution
├── cache.go               # In-memory cache with explicit refresh
├── registry_test.go
├── discovery_test.go
├── frontmatter_test.go
├── arguments_test.go
├── executor_test.go
├── filter_test.go
├── shell_test.go
├── variables_test.go
├── fuzzy_test.go
├── conditional_test.go
├── hooks_test.go
└── skilltool/
    ├── doc.go
    ├── tool.go            # SkillTool implementing tools.BaseTool
    ├── prompt.go          # Skill list formatting with token budget
    ├── tool_test.go
    └── prompt_test.go
```

## Module Dependency Graph

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  discovery   │────▶│  frontmatter │     │    fuzzy     │
└──────┬───────┘     └──────────────┘     └──────┬───────┘
       │                                         │
       ▼                                         ▼
┌──────────────────────────────────────────────────────┐
│                   CommandRegistry                     │
│  ┌──────────┐  ┌────────────┐  ┌──────────────────┐ │
│  │ commands │  │ conditional │  │  cache           │ │
│  └──────────┘  └────────────┘  └──────────────────┘ │
└──────────────────────┬───────────────────────────────┘
                       │
           ┌───────────┼───────────┐
           ▼           ▼           ▼
    ┌────────────┐ ┌─────────┐ ┌──────────────┐
    │  executor  │ │  skill  │ │    hooks     │
    │            │ │  tool   │ │              │
    └─────┬──────┘ └────┬────┘ └──────────────┘
          │             │
    ┌─────┼─────────────┤
    ▼     ▼             ▼
┌────────┐ ┌──────────┐ ┌───────────┐
│arguments│ │  shell   │ │ variables │
└────────┘ └──────────┘ └───────────┘
```

**Dependency rules:**
- `skilltool/` depends on `slash/` (registry + executor) and `internal/tools` (BaseTool interface)
- `slash/` has NO dependency on `internal/tui/`, `internal/engine/`, or `internal/subagent/`
- `slash/` depends only on stdlib + `yaml.v3` + `doublestar`
- Fork mode uses a `ForkRunner` interface defined in `slash/`, with the concrete implementation injected from `app/`

## Module Specifications

### Module: `internal/slash/command.go`
- **Responsibility**: Define core types — `Command`, `CommandType`, `CommandSource`, `Frontmatter`
- **Dependencies**: None (pure types)
- **Interface**: All types are exported; `Command` has methods `IsUserInvocable()`, `IsModelInvocable()`, `MatchesAlias(string)`

### Module: `internal/slash/registry.go`
- **Responsibility**: Unified command registry — store, lookup, filter, precedence
- **Dependencies**: `command.go`, `cache.go`, `conditional.go`
- **Interface**: `NewRegistry(workDir string) *Registry`, `Register(*Command)`, `Lookup(string) (*Command, bool)`, `All() []*Command`, `UserInvocable() []*Command`, `ModelInvocable() []*Command`, `CheckConditional(filePath string)`, `Refresh()`
- **Files**: `registry.go`, `registry_test.go`

### Module: `internal/slash/discovery.go`
- **Responsibility**: File discovery — directory traversal, `.md` loading, namespacing, dedup
- **Dependencies**: `command.go`, `frontmatter.go`
- **Interface**: `DiscoverCommands(workDir string, userHome string) ([]*Command, []*Command)` — returns user-level and project-level commands
- **Files**: `discovery.go`, `discovery_test.go`

### Module: `internal/slash/frontmatter.go`
- **Responsibility**: Parse YAML frontmatter from `.md` content, separate body
- **Dependencies**: `gopkg.in/yaml.v3`, `command.go` (Frontmatter type)
- **Interface**: `ParseFrontmatter(content []byte) (Frontmatter, string, error)`
- **Files**: `frontmatter.go`, `frontmatter_test.go`

### Module: `internal/slash/arguments.go`
- **Responsibility**: Parse user arguments (shell-style quoting) and substitute placeholders in content
- **Dependencies**: None
- **Interface**: `ParseArguments(input string) []string`, `SubstituteArguments(content string, args []string, argNames []string) string`, `ProgressiveHint(argNames []string, filledCount int, customHint string) string`
- **Files**: `arguments.go`, `arguments_test.go`

### Module: `internal/slash/executor.go`
- **Responsibility**: Orchestrate command execution pipeline — arguments → shell → variables → hooks; dispatch to inline or fork mode
- **Dependencies**: `arguments.go`, `shell.go`, `variables.go`, `hooks.go`, `command.go`
- **Interface**: `Execute(ctx context.Context, cmd *Command, args string, sessionID string) (string, error)`
- **Fork mode**: The executor defines a `ForkRunner` interface to avoid importing `internal/subagent`:
  ```go
  type ForkRunner interface {
      Run(ctx context.Context, task string, agentType string) (string, error)
  }
  ```
  The concrete `SubagentForkRunner` implementation is created in `app/runner.go` wrapping `subagent.Manager`. The executor receives it via constructor injection:
  ```go
  type Executor struct {
      forkRunner ForkRunner // nil means fork mode unavailable
      workDir    string
  }
  ```
- **Files**: `executor.go`, `executor_test.go`

### Module: `internal/slash/fuzzy.go`
- **Responsibility**: Fuzzy search scoring for autocomplete filtering
- **Dependencies**: None
- **Interface**: `Score(query string, name string, description string, aliases []string) int`, `FilterCommands(query string, commands []*Command) []*Command`
- **Files**: `fuzzy.go`, `fuzzy_test.go`

### Module: `internal/slash/filter.go`
- **Responsibility**: Implement `FilteredRegistry` that wraps `tools.Registry` and restricts available tools to the allowed set defined in frontmatter
- **Dependencies**: `internal/tools` (Registry interface)
- **Interface**: `NewFilteredRegistry(base tools.Registry, allowed []string) tools.Registry` — returns a `tools.Registry` implementation that only exposes the named tools. Commands without `allowed-tools` use the unfiltered base registry.
- **Files**: `filter.go`, `filter_test.go`

### Module: `internal/slash/conditional.go`
- **Responsibility**: Conditional skill activation via paths glob matching
- **Dependencies**: `github.com/bmatcuk/doublestar`, `command.go`
- **Interface**: `NewConditionalSkills() *ConditionalSkills`, `Add(name string, cmd *Command)`, `CheckAndActivate(filePath string) []string` — returns names of newly activated skills
- **Files**: `conditional.go`, `conditional_test.go`

### Module: `internal/slash/skilltool/tool.go`
- **Responsibility**: SkillTool implementing `tools.BaseTool` for model-side skill invocation
- **Dependencies**: `internal/slash/` (registry, executor), `internal/tools` (BaseTool interface)
- **Interface**: `NewSkillTool(registry *Registry, sessionID func() string) *SkillTool`
- **Files**: `tool.go`, `tool_test.go`

### Module: `internal/slash/skilltool/prompt.go`
- **Responsibility**: Format skill list for system prompt with token budget (1% context window, 3-level truncation)
- **Dependencies**: `internal/slash/` (command types)
- **Interface**: `FormatSkillsWithinBudget(commands []*Command, contextWindowTokens int) string`
- **Files**: `prompt.go`, `prompt_test.go`

## Technical Decisions

### Decision 1: Glob Library — doublestar over filepath.Match
- **Choice**: `github.com/bmatcuk/doublestar`
- **Rationale**: `filepath.Match` does not support `**` (recursive wildcard). The spec requires `**` in `paths` patterns (e.g., `src/**/*.go`). `doublestar` is a well-maintained, zero-dependency library with ~1K GitHub stars.
- **Alternatives**: `filepath.Match` (no `**` support), `github.com/gobwas/glob` (larger API surface), manual recursive walking
- **Trade-offs**: One new dependency; accepted because `**` support is a hard requirement from the spec

### Decision 2: Caching — Explicit Refresh, No File Watcher
- **Choice**: In-memory cache invalidated by explicit `Refresh()` call
- **Rationale**: No file watching dependency (`fsnotify`) for MVP. The registry loads files once at initialization. A `Refresh()` method re-reads files on demand. This is simpler and sufficient for interactive use where files change rarely.
- **Alternatives**: `fsnotify`-based file watching (more responsive but adds complexity and a dependency)
- **Trade-offs**: Users must restart the TUI or use a refresh command to pick up `.md` file changes during a session

### Decision 3: Fuzzy Search — Custom Implementation
- **Choice**: Custom scoring function without external fuzzy search library
- **Rationale**: The spec defines exact scoring weights (exact=100, prefix=80, contains=60, alias=50, description=20). This is a weighted prefix/contains match, not a full fuzzy search with character-level tolerance. A custom implementation (under 100 lines) avoids an external dependency.
- **Alternatives**: `github.com/lithammer/fuzzysearch/fuzzy` (Fuse.js-like for Go, but overkill for this scoring model)
- **Trade-offs**: No character-level fuzzy matching (e.g., `/rvewi` won't match `/review`). Accepted because the spec defines specific scoring weights that don't include character-level fuzziness

### Decision 4: Argument Parsing — Shell-Style Quoting
- **Choice**: Custom shell-style argument parser supporting double-quoted strings
- **Rationale**: Simple implementation that handles `"hello world"` as a single argument. Full POSIX shell parsing is not needed — only space splitting with double-quote grouping.
- **Alternatives**: `github.com/kballard/go-shellquote` (more complete but adds a dependency for simple needs)
- **Trade-offs**: No single-quote support, no escape sequences. Can be enhanced later if needed

### Decision 5: Registry Injected into TUI via Constructor
- **Choice**: Add `*slash.Registry` field to `tui.Model`, initialized in the `NewModel()` constructor
- **Rationale**: The TUI needs registry access for autocomplete and command dispatch. Injecting via constructor follows existing dependency injection patterns (the `Runner` interface is already injected this way).
- **Alternatives**: Global registry singleton, interface-based injection
- **Trade-offs**: Direct struct dependency (not interface-based). Accepted because `slash.Registry` is a concrete type with a clear API, and the TUI is the sole consumer

### Decision 6: SkillTool Registered in buildRegistry()
- **Choice**: Add SkillTool creation to `AgentRunner.buildRegistry()` alongside existing tools
- **Rationale**: Consistent with how all other tools are registered. The SkillTool needs access to the registry and session ID, both available at registration time.
- **Alternatives**: Separate registration path, lazy initialization
- **Trade-offs**: SkillTool is created per-session, consistent with existing tools like `subagent.NewTool()`

### Decision 7: ForkRunner Interface for Dependency Isolation
- **Choice**: Define a `ForkRunner` interface in `slash/executor.go`, with concrete `SubagentForkRunner` in `app/runner.go`
- **Rationale**: The executor needs to delegate fork-mode execution to a sub-agent, but `slash/` must not depend on `internal/subagent/`. By defining the interface in `slash/` and injecting the implementation from `app/`, we maintain the clean dependency boundary. This follows the Go principle of "accept interfaces, return structs."
- **Alternatives**: Direct `internal/subagent` import (violates dependency rules), callback function (less type-safe)
- **Trade-offs**: One extra small type in `app/runner.go`. The interface is minimal (single method) and unlikely to need changes.

### Decision 8: FilteredRegistry for allowed-tools
- **Choice**: Implement `filter.go` as a `tools.Registry` wrapper that filters `GetAvailableTools()` and blocks `Execute()` for disallowed tools
- **Rationale**: The `allowed-tools` frontmatter field restricts which tools a command can use. Rather than modifying the existing registry, a wrapper provides a clean, testable boundary. The wrapper implements the same `tools.Registry` interface, so the engine loop needs no changes — it just receives a filtered registry for that turn.
- **Alternatives**: Tool-level permission checks (scattered logic), middleware-based filtering (more complex)
- **Trade-offs**: Wrapper adds a thin allocation per command execution. Negligible performance impact.

## Implementation Phases

### Phase 1: Core Types & Frontmatter (TDD)
**Deliverables**: Types, YAML parsing, frontmatter extraction

- [ ] Write `frontmatter_test.go`: test valid YAML, missing frontmatter, invalid YAML, empty content, missing closing delimiter
- [ ] Implement `frontmatter.go`: `ParseFrontmatter()` with `---` delimiter handling
- [ ] Write `command_test.go`: test Command construction, IsUserInvocable, IsModelInvocable, MatchesAlias
- [ ] Implement `command.go`: CommandType, CommandSource, Command, Frontmatter types
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 2: File Discovery & Loading (TDD)
**Deliverables**: Directory traversal, file loading, namespacing, dedup

- [ ] Write `discovery_test.go`: test single-file format, directory format, namespace mapping, user-level vs project-level, file dedup by inode, missing directories, loose skills/ files
- [ ] Implement `discovery.go`: `DiscoverCommands()` with directory traversal, `.md` loading, namespacing
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 3: Command Registry (TDD)
**Deliverables**: Unified registry with precedence, lookup, filtering

- [ ] Write `registry_test.go`: test Register, Lookup by name and alias, precedence (builtin < user < project), All, UserInvocable, ModelInvocable, duplicate name handling
- [ ] Implement `registry.go`: `NewRegistry()`, `Register()`, `Lookup()`, `All()`, `UserInvocable()`, `ModelInvocable()`
- [ ] Implement `cache.go`: in-memory caching for All/UserInvocable results
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 4: Argument Substitution (TDD)
**Deliverables**: Argument parsing, placeholder substitution, progressive hints

- [ ] Write `arguments_test.go`: test ParseArguments (simple, quoted, mixed), SubstituteArguments ($ARGUMENTS, $0/$1, named, auto-append, missing arg), ProgressiveHint
- [ ] Implement `arguments.go`: `ParseArguments()`, `SubstituteArguments()`, `ProgressiveHint()`
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 5: Shell Embedding & Special Variables (TDD)
**Deliverables**: Shell command execution, variable replacement

- [ ] Write `shell_test.go`: test shell embedding extraction, execution, failure handling, timeout, multiple embeddings, empty output
- [ ] Implement `shell.go`: shell command extraction regex, execution with timeout
- [ ] Write `variables_test.go`: test ${FOXHARNESS_SKILL_DIR}, ${FOXHARNESS_SESSION_ID} replacement
- [ ] Implement `variables.go`: `ReplaceVariables()`
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 6: Executor, Hooks & Tool Filtering (TDD)
**Deliverables**: Command execution pipeline, before/after hooks, allowed-tools filtering

- [ ] Write `hooks_test.go`: test before/after hook execution, hook failure handling
- [ ] Implement `hooks.go`: `ExecuteHooks()`
- [ ] Write `executor_test.go`: test full pipeline (arguments → shell → variables → hooks), inline mode
- [ ] Implement `executor.go`: `Execute()` orchestrating the pipeline, `ForkRunner` interface definition
- [ ] Write `filter_test.go`: test `FilteredRegistry` — verify that only allowed tools are exposed, unlisted tools return error, commands without `allowed-tools` get all tools
- [ ] Implement `filter.go`: `FilteredRegistry` — a lightweight wrapper around `tools.Registry` that filters `GetAvailableTools()` and `Execute()` to only the allowed set. The executor creates a `FilteredRegistry` when `cmd.Frontmatter.AllowedTools` is non-empty
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 7: Fuzzy Search (TDD)
**Deliverables**: Weighted scoring, filtered command list

- [ ] Write `fuzzy_test.go`: test exact match, prefix match, contains match, alias match, description match, scoring order, grouping
- [ ] Implement `fuzzy.go`: `Score()`, `FilterCommands()`
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 8: TUI Integration
**Deliverables**: Registry injected into TUI, autocomplete with fuzzy search, command dispatch refactor

- [ ] Add `registry *slash.Registry` field to `tui.Model`
- [ ] Initialize registry in `app/runner.go`: `slash.NewRegistry(workDir)`, call `registry.Load()`, register built-in commands
- [ ] Pass registry to `tui.NewModel()` constructor
- [ ] Refactor `matchingSlashCommands()` to use `registry.UserInvocable()` + `fuzzy.FilterCommands()`
- [ ] Refactor `handleSlashCommand()` to use `registry.Lookup()` + dispatch by CommandType
- [ ] Update `renderSlashSuggestions()` in `view.go` for grouped display (builtin → user → project)
- [ ] Add progressive argument hint display
- [ ] Verify: `go test ./internal/tui/...` passes, manual TUI test

### Phase 9: Model-side Skill Tool (TDD)
**Deliverables**: SkillTool, system prompt injection, token budget

- [ ] Write `skilltool/tool_test.go`: test Execute with valid/invalid skill, disable-model-invocation, argument passing
- [ ] Implement `skilltool/tool.go`: `SkillTool` struct implementing `tools.BaseTool`
- [ ] Write `skilltool/prompt_test.go`: test FormatSkillsWithinBudget — no truncation, normal truncation, extreme truncation, built-in preservation
- [ ] Implement `skilltool/prompt.go`: `FormatSkillsWithinBudget()` with 1% context window budget, 3-level truncation
- [ ] Register SkillTool in `app/runner.go` `buildRegistry()`
- [ ] Add skill list injection to system prompt in `engine/loop.go` or prompt composer
- [ ] Verify: `go test ./internal/slash/skilltool/...` passes

### Phase 10: Conditional Activation (TDD)
**Deliverables**: Conditional skill storage, glob matching, activation trigger

- [ ] Write `conditional_test.go`: test Add conditional skill, CheckAndActivate with matching path, non-matching path, multiple patterns, `**` wildcard
- [ ] Implement `conditional.go`: `ConditionalSkills` map, `CheckAndActivate()` using `doublestar`
- [ ] Hook into `engine/loop.go`: after each `read_file`/`write_file` execution, call `registry.CheckConditional(filePath)`
- [ ] Update system prompt to include newly activated skills
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 11: Fork Mode (TDD)
**Deliverables**: Sub-agent execution via injected ForkRunner

- [ ] Write fork mode tests in `executor_test.go`: test fork mode calls ForkRunner, agent type passed, result returned. Test with nil ForkRunner (fork unavailable) returns error
- [ ] Implement fork mode in `executor.go`: when `context: fork`, call `e.forkRunner.Run(ctx, processedContent, cmd.Frontmatter.Agent)`
- [ ] Implement `SubagentForkRunner` in `app/runner.go`: wraps `subagent.Manager.Run()` to satisfy the `ForkRunner` interface
- [ ] Wire ForkRunner injection: pass `&SubagentForkRunner{Manager: subManager}` to the executor constructor
- [ ] Verify: `go test ./internal/slash/...` passes

### Phase 12: Integration Testing & Polish
**Deliverables**: End-to-end tests, edge case coverage, cleanup

- [ ] Write integration test: create temp `.foxharness/commands/` with test `.md` files, verify full flow
- [ ] Test built-in command backward compatibility (TC-010)
- [ ] Test edge cases: empty files, circular symlinks, concurrent access, long content, special characters
- [ ] Test NFR-002 security: path traversal prevention (content with `../../etc/passwd` is sanitized), frontmatter does not execute code, shell commands run in sandboxed workDir
- [ ] Run `go test ./...` to verify no regressions
- [ ] Run `gofmt -l .` and fix formatting
- [ ] Add `doc.go` files with package documentation
- [ ] Verify all 32 spec test cases pass

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| doublestar dependency breaks | Low | Medium | Pin version in go.mod; API is stable |
| Registry refactoring breaks built-in commands | Medium | High | TC-010 validates backward compatibility; incremental migration |
| Shell embedding security (command injection) | Medium | High | Timeout enforced; NFR-002 restricts user-level commands |
| Fork mode complexity with subagent coupling | Medium | Medium | Fork mode is optional; inline mode works without subagent dependency |
| TUI autocomplete performance with many commands | Low | Low | Fuzzy search is O(n) with simple string ops; caching helps |
