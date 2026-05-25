# Task Breakdown: Slash Commands System

**Input**: `.codexspec/specs/2026-0524-2343mf-slash-commands/plan.md`
**Related Spec**: `.codexspec/specs/2026-0524-2343mf-slash-commands/spec.md`
**Created**: 2026-05-25
**Status**: Complete with post-review fixes (2026-05-25)

## Implementation Status

All 12 phases complete, plus a Phase 13 batch of post-review fixes (see end of file). Test results: `go test ./...` passes (88.6% coverage on `internal/slash`, 69.0% on `internal/slash/skilltool`). New code is in `internal/slash/`, `internal/slash/skilltool/`; integrations live in `internal/tui/slash_registry.go`, `internal/app/runner.go`, `internal/app/tui.go`, `internal/context/prompt.go`, `internal/engine/{config,loop}.go`. The `doublestar/v4` dependency was added for glob path matching.

### Spec TC → Test Mapping

- TC-001 → `TestDiscoverCommands_SingleFile`, `TestIntegration_DiscoverRegisterLookupExecute`
- TC-002 → `TestDiscoverCommands_SkillDirectory`, `TestIntegration_DiscoverRegisterLookupExecute`
- TC-003 → `TestDiscoverCommands_NamespaceMapping`, `TestIntegration_DiscoverRegisterLookupExecute`
- TC-004 → `TestParseFrontmatter_AllFields`
- TC-005 → `TestParseFrontmatter_NoFrontmatter`
- TC-006 → `TestSubstituteArguments_AllPlaceholderTypes` ($ARGUMENTS full)
- TC-007 → `TestSubstituteArguments_AllPlaceholderTypes` ($0 $1 shorthand)
- TC-008 → `TestSubstituteArguments_AllPlaceholderTypes` (named placeholders)
- TC-009 → `TestSubstituteArguments_AllPlaceholderTypes` (auto-append)
- TC-010 → All existing TUI tests (no regressions) + `TestModel_BuiltinCommandsUnaffectedByRegistry`
- TC-011 → `TestIntegration_BuiltinAndFileBasedCoexist`, `TestModel_FileBasedCommandAppearsInAutocomplete`
- TC-012 → `TestModel_FuzzyAutocomplete`, `TestFilterCommands_OrderedByScore`
- TC-013 → `TestSkillTool_Execute_Valid`
- TC-014 → `TestSkillTool_Execute_DisableModelInvocation`
- TC-015 → `TestSkillTool_Execute_UserInvocableFalseStillModelInvocable`
- TC-016 → `TestConditionalSkills_AddAndCheck`, `TestRegistry_CheckConditional_ActivatesAndMoves`
- TC-017 → `TestExecuteEmbeddedShell_Success`
- TC-018 → `TestExecuteEmbeddedShell_Failure`, `TestExecuteEmbeddedShell_NonexistentCommand`
- TC-019 → `TestReplaceVariables`, `TestExecutor_VariableReplacement`
- TC-020 → `TestExecutor_ForkMode_CallsRunner`
- TC-021 → `TestFilteredRegistry_ExecuteAllowed`, `TestFilteredRegistry_ExecuteBlocked`
- TC-022 → `TestIntegration_ProjectOverridesUser`, `TestRegistry_Precedence`
- TC-023 → `TestDiscoverCommands_UserVsProject`, `TestIntegration_DiscoverRegisterLookupExecute`
- TC-024 → `TestEdge_SymlinkHandled`
- TC-025 → `TestExecuteHooks_BeforeRuns`, `TestExecuteAfterHook_Runs`
- TC-026 → `TestProgressiveHint`
- TC-027 → `TestParseFrontmatter_InvalidYAML`
- TC-028 → `TestEdge_NoFoxharnessDirectory`
- TC-029 → `TestRegistry_LookupAlias`
- TC-030 → `TestConditionalSkills_MultiplePatternsOR`
- TC-031 → `TestIntegration_RefreshPicksUpNewFiles`
- TC-032 → `TestRegistry_All_CacheReused`

## Overview

- Total tasks: 54
- Parallelizable tasks: 20
- Estimated phases: 12

> **TDD Mandatory**: Per project constitution, all tests MUST be written BEFORE implementation. Every implementation task is preceded by its corresponding test task.

## Phase 1: Core Types & Frontmatter

**Purpose**: Define foundational types (Command, Frontmatter) and YAML frontmatter parsing. All subsequent phases depend on these types.

### Task 1.1: Create Package Structure and Add doublestar Dependency

- **Type**: Setup
- **Files**: `internal/slash/doc.go`, `go.mod`, `go.sum`
- **Description**: Create `internal/slash/` package directory with a minimal `doc.go` placeholder. Run `go get github.com/bmatcuk/doublestar/v4@v4.8.1` to add the glob matching dependency.
- **Dependencies**: None
- **Est. Complexity**: Low

### Task 1.2: Write Tests for Core Types

- **Type**: Testing
- **Files**: `internal/slash/command_test.go`
- **Description**: Write table-driven tests for `Command` construction and methods: `IsUserInvocable()` (default true, false when `user-invocable: false`), `IsModelInvocable()` (true unless `disable-model-invocation: true`), `MatchesAlias()` (exact alias match, case-sensitive). Test `CommandType` and `CommandSource` enums. Test `Frontmatter` struct default values for all 15 fields. Cover EC-001 scenario (empty content command is valid).
- **Dependencies**: Task 1.1
- **Est. Complexity**: Low

### Task 1.3: Implement Core Types

- **Type**: Implementation
- **Files**: `internal/slash/command.go`
- **Description**: Define `CommandType` (CommandBuiltin, CommandPrompt), `CommandSource` (SourceBuiltin, SourceUser, SourceProject), `Frontmatter` struct with all 15 fields from REQ-003 (description, arguments, argument-hint, allowed-tools, model, effort, user-invocable, disable-model-invocation, when_to_use, context, agent, paths, aliases, hooks, version). Define `Command` struct with methods `IsUserInvocable() bool`, `IsModelInvocable() bool`, `MatchesAlias(string) bool`. Exported types with block comments per Go documentation standards.
- **Dependencies**: Task 1.2
- **Est. Complexity**: Low

### Task 1.4: Write Tests for Frontmatter Parsing

- **Type**: Testing
- **Files**: `internal/slash/frontmatter_test.go`
- **Description**: Write table-driven tests for `ParseFrontmatter()`: valid YAML with all 15 fields parsed correctly (TC-004), missing frontmatter uses defaults and returns full body (TC-005), invalid YAML logs warning and uses defaults (TC-027), empty file, missing closing `---` delimiter treats entire file as body with warning (EC-006), frontmatter with only `description` field, multi-line content after frontmatter preserved.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 1.5: Implement Frontmatter Parsing

- **Type**: Implementation
- **Files**: `internal/slash/frontmatter.go`
- **Description**: Implement `ParseFrontmatter(content []byte) (Frontmatter, string, error)`. Split content on `---` delimiters. Parse YAML with `gopkg.in/yaml.v3`. Return parsed Frontmatter and body content. Invalid YAML returns zero-value Frontmatter with defaults and a non-fatal error. No closing delimiter treats entire input as body content.
- **Dependencies**: Task 1.4
- **Est. Complexity**: Medium

**Checkpoint**: `go test ./internal/slash/...` passes. Types and frontmatter parsing are solid foundations.

---

## Phase 2: File Discovery & Loading

**Purpose**: Implement directory traversal to discover and load `.md` command/skill files from `.foxharness/` directories.

### Task 2.1: Write Tests for File Discovery

- **Type**: Testing
- **Files**: `internal/slash/discovery_test.go`
- **Description**: Write tests using temporary directories for: single-file format (`commands/review.md` loads as `review`), directory format (`skills/go-test/SKILL.md` loads as `go-test`) (TC-002), namespace mapping (`commands/db/migrate.md` → `db:migrate`) (TC-003), user-level vs project-level returned separately, file dedup by device+inode (TC-024), missing directories return empty slices without error (TC-028), loose `.md` files in skills/ ignored with log (EC-008), files >1MB skipped (EC-004), symlink handling.
- **Dependencies**: Task 1.5
- **Est. Complexity**: Medium

### Task 2.2: Implement File Discovery

- **Type**: Implementation
- **Files**: `internal/slash/discovery.go`
- **Description**: Implement `DiscoverCommands(workDir string, userHome string) (userCmds []*Command, projectCmds []*Command, err error)`. Traverse `.foxharness/commands/` and `.foxharness/skills/` at user-level (`~/.foxharness/`) and project-level (search from cwd to git root). Map filenames to command names with colon-separated namespace from subdirectories. Deduplicate by `os.Stat` device+inode. Skip files > 1MB. Handle directory format (look for `SKILL.md`). Log warnings for skipped files, continue on errors.
- **Dependencies**: Task 2.1
- **Est. Complexity**: High

**Checkpoint**: File discovery works end-to-end with temp directories.

---

## Phase 3: Command Registry & Cache

**Purpose**: Build the unified command registry with precedence rules, lookup, filtering, and caching.

### Task 3.1: Write Tests for Cache [P]

- **Type**: Testing
- **Files**: `internal/slash/cache_test.go`
- **Description**: Write tests for cache operations: cache miss on first `Get()`, cache hit on second `Get()` with same key (TC-032), `Set()` stores value, `Invalidate()` clears all entries, `InvalidateKey()` clears specific key. Test concurrent access safety.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Low

### Task 3.2: Implement Cache [P]

- **Type**: Implementation
- **Files**: `internal/slash/cache.go`
- **Description**: Implement `Cache` struct with mutex-protected `map[string][]*Command`. Methods: `Get(key string) ([]*Command, bool)`, `Set(key string, cmds []*Command)`, `Invalidate()`, `InvalidateKey(key string)`. Simple and generic for use by the registry.
- **Dependencies**: Task 3.1
- **Est. Complexity**: Low

### Task 3.3: Write Tests for Command Registry

- **Type**: Testing
- **Files**: `internal/slash/registry_test.go`
- **Description**: Write table-driven tests for: `Register()` adds builtin/user/project commands, `Lookup()` by name (TC-001), `Lookup()` by alias (TC-029), precedence rules — project overrides user overrides builtin (TC-022), `All()` returns all commands, `UserInvocable()` filters correctly, `ModelInvocable()` filters correctly, duplicate name triggers warning log, `Refresh()` re-discovers and reloads files, `Load()` integrates discovery.
- **Dependencies**: Task 3.2, Task 2.2
- **Est. Complexity**: Medium

### Task 3.4: Implement Command Registry

- **Type**: Implementation
- **Files**: `internal/slash/registry.go`
- **Description**: Implement `NewRegistry(workDir string) *Registry` and methods: `Register(cmd *Command)`, `Lookup(name string) (*Command, bool)`, `All() []*Command` (cached), `UserInvocable() []*Command` (cached), `ModelInvocable() []*Command`, `Load()` (calls DiscoverCommands + registers all), `Refresh()` (invalidates cache + re-loads). Enforce precedence: project > user > builtin. Log warning on name override. Wire `Cache` for `All()`/`UserInvocable()` results. Invalidate cache on `Register()` and `Refresh()`.
- **Dependencies**: Task 3.3
- **Est. Complexity**: High

**Checkpoint**: Registry fully functional. Can load, register, and look up commands.

---

## Phase 4: Argument Substitution [P with Phases 2-3]

**Purpose**: Parse user arguments and substitute placeholders in command content.

### Task 4.1: Write Tests for Argument Substitution [P]

- **Type**: Testing
- **Files**: `internal/slash/arguments_test.go`
- **Description**: Write table-driven tests for: `ParseArguments()` — simple space-split, double-quoted strings as single arg, mixed quoted and unquoted. `SubstituteArguments()` — `$ARGUMENTS` full replacement (TC-006), `$0`/`$1`/`$2` indexed access (TC-007), `$ARGUMENTS[0]`/`$ARGUMENTS[1]` bracket notation, named params like `$file` (TC-008), auto-append when no placeholder exists (TC-009), missing named arg → empty string (EC-010), special characters in arguments (EC-005). `ProgressiveHint()` — shows all params, shows remaining after fill (TC-026), custom hint overrides auto-generated.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 4.2: Implement Argument Substitution [P]

- **Type**: Implementation
- **Files**: `internal/slash/arguments.go`
- **Description**: Implement `ParseArguments(input string) []string` with shell-style double-quote grouping (split on unquoted spaces). Implement `SubstituteArguments(content string, args []string, argNames []string) string` replacing `$ARGUMENTS`, `$ARGUMENTS[N]`, `$N` (shorthand), and `$name` (named) placeholders. Auto-append `\n\nARGUMENTS: {args}` when no `$` placeholder exists. Implement `ProgressiveHint(argNames []string, filledCount int, customHint string) string` for autocomplete hints.
- **Dependencies**: Task 4.1
- **Est. Complexity**: Medium

**Checkpoint**: Argument parsing and substitution working independently.

---

## Phase 5: Shell Embedding & Special Variables [P with Phases 2-4]

**Purpose**: Execute embedded shell commands and replace special variables in content.

### Task 5.1: Write Tests for Shell Embedding [P]

- **Type**: Testing
- **Files**: `internal/slash/shell_test.go`
- **Description**: Write tests for: shell embedding regex extracts `` !`cmd` `` correctly, successful execution replaces with stdout (TC-017), command failure produces `[ERROR: ...]` message (TC-018), timeout cancels long-running commands, multiple embeddings in single content all executed sequentially, empty stdout → empty string replacement (EC-009), no embeddings → content unchanged, shell runs in specified workDir.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 5.2: Implement Shell Embedding [P]

- **Type**: Implementation
- **Files**: `internal/slash/shell.go`
- **Description**: Implement `ExtractAndExecuteShell(content string, workDir string, timeout time.Duration) (string, error)`. Parse `` !`cmd` `` syntax using regex. Execute via `exec.Command("sh", "-c", cmd)` with `context.WithTimeout`. Replace each embedding with trimmed stdout. On failure, replace with `[ERROR: command failed: exit code N]`. Process multiple embeddings sequentially left-to-right.
- **Dependencies**: Task 5.1
- **Est. Complexity**: Medium

### Task 5.3: Write Tests for Variable Replacement [P]

- **Type**: Testing
- **Files**: `internal/slash/variables_test.go`
- **Description**: Write tests for `ReplaceVariables()`: replace `${FOXHARNESS_SKILL_DIR}` with directory path (TC-019), replace `${FOXHARNESS_SESSION_ID}` with session ID (TC-019), both variables in single content, variables not present → content unchanged, empty variable value → replacement with empty string.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Low

### Task 5.4: Implement Variable Replacement [P]

- **Type**: Implementation
- **Files**: `internal/slash/variables.go`
- **Description**: Implement `ReplaceVariables(content string, vars map[string]string) string`. Replace `${FOXHARNESS_SKILL_DIR}` and `${FOXHARNESS_SESSION_ID}` using `strings.ReplaceAll`. Accept a generic `vars` map keyed by variable name (without `${}` wrapper) for extensibility.
- **Dependencies**: Task 5.3
- **Est. Complexity**: Low

**Checkpoint**: Shell embedding and variable replacement working independently.

---

## Phase 6: Executor, Hooks & Tool Filtering

**Purpose**: Orchestrate the full execution pipeline and implement tool filtering for allowed-tools.

### Task 6.1: Write Tests for Hooks [P]

- **Type**: Testing
- **Files**: `internal/slash/hooks_test.go`
- **Description**: Write tests for `ExecuteHooks()`: before hook runs and succeeds (TC-025), after hook runs and succeeds (TC-025), hook failure logs error but does not block execution, nil hooks → no-op (no error), hook with non-zero exit code is non-fatal, hook with timeout.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Low

### Task 6.2: Implement Hooks [P]

- **Type**: Implementation
- **Files**: `internal/slash/hooks.go`
- **Description**: Implement `ExecuteHooks(ctx context.Context, before string, after string, phase string, workDir string) error`. Execute shell command via `exec.Command("sh", "-c", cmd)` with timeout context. Log errors via `log.Printf` but always return nil (non-blocking). `phase` parameter is `"before"` or `"after"`. Export `HookResult` type for testability.
- **Dependencies**: Task 6.1
- **Est. Complexity**: Low

### Task 6.3: Write Tests for Tool Filtering [P]

- **Type**: Testing
- **Files**: `internal/slash/filter_test.go`
- **Description**: Write tests for `FilteredRegistry`: `GetAvailableTools()` only returns tools in the allowed list, `Execute()` blocks disallowed tools with error, empty allowed list means no tools available, base registry with no filter returns all tools (TC-021), `IsParallelSafe()` delegates correctly, `Register()` is not exposed on filtered view.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 6.4: Implement Tool Filtering [P]

- **Type**: Implementation
- **Files**: `internal/slash/filter.go`
- **Description**: Implement `NewFilteredRegistry(base tools.Registry, allowed []string) tools.Registry`. Return a struct implementing `tools.Registry` that filters `GetAvailableTools()` to only the named tools and blocks `Execute()` for disallowed tools with a descriptive error. Delegate `IsParallelSafe()` to the base registry for matching tools.
- **Dependencies**: Task 6.3
- **Est. Complexity**: Medium

### Task 6.5: Write Tests for Executor

- **Type**: Testing
- **Files**: `internal/slash/executor_test.go`
- **Description**: Write tests for `Execute()`: full pipeline with arguments → shell → variables → hooks in correct order, inline mode returns processed content, command with all frontmatter fields processed, ForkRunner interface mock verifies fork dispatch (TC-020), nil ForkRunner returns error for fork commands, arguments with no placeholders auto-append, empty content command produces valid output (EC-001), before hook runs before content processing, after hook runs after.
- **Dependencies**: Task 4.2, Task 5.2, Task 5.4, Task 6.2
- **Est. Complexity**: High

### Task 6.6: Implement Executor

- **Type**: Implementation
- **Files**: `internal/slash/executor.go`
- **Description**: Implement `Executor` struct with `ForkRunner` interface for dependency isolation. Implement `NewExecutor(opts ...ExecutorOption) *Executor` with functional options pattern. Implement `Execute(ctx context.Context, cmd *Command, rawArgs string, sessionID string) (string, error)` orchestrating: parse arguments → substitute placeholders → execute shell embeddings → replace variables → run before hook → dispatch (inline returns content, fork calls ForkRunner) → run after hook. Define `ForkRunner` interface: `Run(ctx context.Context, task string, agentType string) (string, error)`.
- **Dependencies**: Task 6.5
- **Est. Complexity**: High

**Checkpoint**: Full execution pipeline works. Hooks, filtering, inline mode functional.

---

## Phase 7: Fuzzy Search [P with Phases 2-6]

**Purpose**: Implement weighted scoring for autocomplete command filtering.

### Task 7.1: Write Tests for Fuzzy Search [P]

- **Type**: Testing
- **Files**: `internal/slash/fuzzy_test.go`
- **Description**: Write table-driven tests for `Score()`: exact name match = 100, name prefix match = 80, name contains match = 60, alias exact match = 50, description contains match = 20, case-insensitive matching, no match = 0. Test `FilterCommands()`: filters by query and sorts by score descending, empty query returns all commands unsorted, query matching no commands returns empty list, ties broken alphabetically. Cover TC-012 (`/rev` matches `review`).
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 7.2: Implement Fuzzy Search [P]

- **Type**: Implementation
- **Files**: `internal/slash/fuzzy.go`
- **Description**: Implement `Score(query string, name string, description string, aliases []string) int` with weighted scoring: exact=100, prefix=80, contains=60, alias=50, description=20. First match wins (highest priority checked first). Case-insensitive via `strings.EqualFold` and `strings.ToLower`. Implement `FilterCommands(query string, commands []*Command) []*Command` that scores each command, filters score > 0, sorts by score descending then name alphabetically.
- **Dependencies**: Task 7.1
- **Est. Complexity**: Medium

**Checkpoint**: Fuzzy search scoring and filtering working independently.

---

## Phase 8: TUI Integration

**Purpose**: Wire the slash command system into the existing TUI, replacing the hardcoded command dispatch.

### Task 8.0: Write Characterization Tests for Existing Slash Commands

- **Type**: Testing
- **Files**: `internal/tui/slash_test.go`
- **Description**: Write characterization tests capturing the current behavior of the 10 built-in slash commands before refactoring. Test that `handleSlashCommand()` dispatches each command correctly, `matchingSlashCommands()` returns expected results for various inputs, and `completeSlashCommand()` cycles through matches. These tests serve as a safety net — if they break during Tasks 8.1-8.6, the refactoring introduced a regression.
- **Dependencies**: Task 1.3 (needs Command types for test assertions)
- **Est. Complexity**: Medium

### Task 8.1: Add Registry to TUI Model

- **Type**: Implementation
- **Files**: `internal/tui/model.go`
- **Description**: Add `registry *slash.Registry` field to `Model` struct. Update `NewModel()` to accept `registry *slash.Registry` parameter and store it. Keep the existing `slashCommand` struct temporarily for reference but mark it deprecated. The registry field is the new source of truth for commands.
- **Dependencies**: Task 3.4, Task 7.2
- **Est. Complexity**: Medium

### Task 8.2: Refactor Command Matching for Fuzzy Search

- **Type**: Implementation
- **Files**: `internal/tui/model.go`
- **Description**: Replace `matchingSlashCommands()` to use `m.registry.UserInvocable()` + `fuzzy.FilterCommands()`. Update `completeSlashCommand()` to work with `*slash.Command` results from registry. Support tab cycling through filtered results. Remove direct references to hardcoded `slashCommands` slice.
- **Dependencies**: Task 8.1
- **Est. Complexity**: Medium

### Task 8.3: Refactor Command Dispatch to Registry

- **Type**: Implementation
- **Files**: `internal/tui/model.go`
- **Description**: Replace `handleSlashCommand()` switch statement with `m.registry.Lookup(commandName)`. Dispatch by `CommandType`: `CommandBuiltin` calls `cmd.Handler(args)`, `CommandPrompt` calls a new `executePromptCommand()` method that creates the user message with processed content. Remove all 10 hardcoded case branches. Unknown commands show error message.
- **Dependencies**: Task 8.1
- **Est. Complexity**: Medium

### Task 8.4: Update Slash Suggestions Rendering

- **Type**: Implementation
- **Files**: `internal/tui/view.go`
- **Description**: Update `renderSlashSuggestions()` to display commands grouped by source (builtin → user → project), sorted alphabetically within groups. Show command name, description, aliases in parentheses, and argument hints. Use `registry.UserInvocable()` and group by `CommandSource`. Replace hardcoded rendering with registry-driven data.
- **Dependencies**: Task 8.1
- **Est. Complexity**: Medium

### Task 8.5: Add Progressive Argument Hints

- **Type**: Implementation
- **Files**: `internal/tui/view.go`
- **Description**: Implement progressive argument hint display in the input area. When user types `/commit main.go `, call `ProgressiveHint()` to show remaining args `[message] [branch]`. Parse current input to extract command name and typed arguments. Integrate hint text into the existing input rendering below the input field.
- **Dependencies**: Task 8.4, Task 4.2
- **Est. Complexity**: Medium

### Task 8.6: Initialize Registry in App Runner

- **Type**: Implementation
- **Files**: `internal/app/runner.go`
- **Description**: Create registry initialization in `AgentRunner` startup: `registry := slash.NewRegistry(workDir)`, call `registry.Load()` to discover files, register all 10 built-in commands (help, clear, model, exit, compact, plan, thinking, debug, tools, quit) as `CommandBuiltin` with their existing handler functions. Pass registry to `tui.NewModel()`. Create executor with `slash.NewExecutor()`.
- **Dependencies**: Task 8.1, Task 6.6
- **Est. Complexity**: Medium

**Checkpoint**: TUI uses registry for autocomplete and dispatch. All built-in commands work. Manual test: `go run ./cmd/fox` shows `/` autocomplete with grouped commands.

---

## Phase 9: Model-side Skill Tool

**Purpose**: Implement the `skill` tool that enables the LLM agent to invoke skills autonomously.

### Task 9.1: Create SkillTool Package Structure

- **Type**: Setup
- **Files**: `internal/slash/skilltool/doc.go`
- **Description**: Create `internal/slash/skilltool/` package with minimal `doc.go` placeholder.
- **Dependencies**: Task 6.6
- **Est. Complexity**: Low

### Task 9.2: Write Tests for Skill Prompt Formatting [P]

- **Type**: Testing
- **Files**: `internal/slash/skilltool/prompt_test.go`
- **Description**: Write tests for `FormatSkillsWithinBudget()`: no truncation when total ≤ budget (all descriptions shown), normal truncation shrinks non-builtin descriptions evenly when budget is tight, extreme truncation shows name-only (`- skillname`) when budget can't fit descriptions, builtin skills always get full descriptions regardless of budget pressure, 250 char max per description before truncation, empty skills list returns empty string, single skill within budget shows full entry, unknown context window → 8000 char fallback.
- **Dependencies**: Task 9.1, Task 1.3
- **Est. Complexity**: Medium

### Task 9.3: Implement Skill Prompt Formatting [P]

- **Type**: Implementation
- **Files**: `internal/slash/skilltool/prompt.go`
- **Description**: Implement `FormatSkillsWithinBudget(commands []*Command, contextWindowTokens int) string`. Calculate character budget: `charBudget = contextWindowTokens × 4 × 0.01`, fallback 8000. Each description capped at 250 chars. 3-level truncation: (1) no truncation when total ≤ budget, (2) normal — evenly distribute remaining budget across non-builtin descriptions (min 20 chars each), (3) extreme — non-builtin skills show name only. Builtin skills always get full descriptions. Format as Markdown list: `- name: description (when_to_use) [args]`.
- **Dependencies**: Task 9.2
- **Est. Complexity**: High

### Task 9.4: Write Tests for Skill Tool [P]

- **Type**: Testing
- **Files**: `internal/slash/skilltool/tool_test.go`
- **Description**: Write tests for `SkillTool`: `Name()` returns "skill", `Definition()` returns valid JSON schema with `name` and `arguments` fields. `Execute()` with valid skill processes content (TC-013), invalid skill name returns error, `disable-model-invocation: true` skill returns error (TC-014), arguments passed through to executor, `user-invocable: false` skill is still model-invocable (TC-015). Use mock registry and executor.
- **Dependencies**: Task 9.1, Task 3.4, Task 6.6
- **Est. Complexity**: Medium

### Task 9.5: Implement Skill Tool [P]

- **Type**: Implementation
- **Files**: `internal/slash/skilltool/tool.go`
- **Description**: Implement `SkillTool` struct satisfying `tools.BaseTool`. Constructor: `NewSkillTool(registry *slash.Registry, executor *slash.Executor, sessionID func() string) *SkillTool`. `Name()` returns "skill". `Definition()` returns `tools.ToolDefinition` with JSON schema `{name: string, arguments: string}`. `Execute()` parses input, looks up skill via `registry.Lookup()`, checks model-invocable, calls `executor.Execute()`, returns result as tool output string.
- **Dependencies**: Task 9.4
- **Est. Complexity**: Medium

### Task 9.6: Register SkillTool in Tool Registry

- **Type**: Implementation
- **Files**: `internal/app/runner.go`
- **Description**: Add SkillTool creation to `buildRegistry()`: create `skilltool.NewSkillTool(registry, executor, sessionIDFunc)` and append to tools slice alongside existing tools. Ensure the tool is available to the LLM.
- **Dependencies**: Task 9.5, Task 8.6
- **Est. Complexity**: Low

### Task 9.7: Inject Skill List into System Prompt

- **Type**: Implementation
- **Files**: `internal/engine/loop.go`
- **Description**: After system prompt construction, call `FormatSkillsWithinBudget(registry.ModelInvocable(), contextWindowTokens)` and append the formatted skill list to the system prompt. Determine the appropriate injection point in the prompt composition flow. The skill list should be clearly delimited so the model knows these are available skills.
- **Dependencies**: Task 9.3, Task 9.6
- **Est. Complexity**: Medium

**Checkpoint**: LLM can invoke skills via the `skill` tool. Skill list appears in system prompt within token budget.

---

## Phase 10: Conditional Activation

**Purpose**: Implement conditional skill activation based on file path glob matching.

### Task 10.1: Write Tests for Conditional Activation [P]

- **Type**: Testing
- **Files**: `internal/slash/conditional_test.go`
- **Description**: Write tests for: `Add()` stores conditional skill, `CheckAndActivate()` with matching glob pattern returns skill name (TC-016), non-matching path returns empty slice, `**` wildcard matches nested paths, `*` matches single path component, `?` matches single character, multiple patterns with OR logic — any match activates (TC-030), already-activated skill not returned again, pattern evaluated relative to project root.
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 10.2: Implement Conditional Activation [P]

- **Type**: Implementation
- **Files**: `internal/slash/conditional.go`
- **Description**: Implement `ConditionalSkills` struct with `NewConditionalSkills() *ConditionalSkills`, `Add(name string, patterns []string, cmd *Command)`, `CheckAndActivate(filePath string, projectRoot string) []string`. Use `doublestar.Match()` for glob matching. Evaluate patterns relative to project root using `filepath.Rel()`. Track activated set to prevent duplicate activation. Return names of newly activated skills.
- **Dependencies**: Task 10.1
- **Est. Complexity**: Medium

### Task 10.3: Wire Conditional Activation into Registry

- **Type**: Implementation
- **Files**: `internal/slash/registry.go`
- **Description**: Add `ConditionalSkills` to `Registry` struct. In `Load()`, when a command has non-empty `paths` frontmatter, store it in the conditional map instead of the active set. Add `CheckConditional(filePath string) []string` method that calls `conditional.CheckAndActivate()` and moves newly activated skills to the active command set. Invalidate cache on activation.
- **Dependencies**: Task 10.2, Task 3.4
- **Est. Complexity**: Medium

### Task 10.4: Hook Conditional Activation into Engine

- **Type**: Implementation
- **Files**: `internal/engine/loop.go`
- **Description**: After each `read_file` or `write_file` tool execution in the engine loop, extract the file path from the tool call arguments. Call `registry.CheckConditional(filePath)`. If new skills are activated, update the system prompt's skill list by re-calling `FormatSkillsWithinBudget()`.
- **Dependencies**: Task 10.3, Task 9.7
- **Est. Complexity**: Medium

**Checkpoint**: Conditional skills activate when matching files are read/written.

---

## Phase 11: Fork Mode

**Purpose**: Enable skills to run as isolated sub-agents via the ForkRunner interface.

### Task 11.1: Write Tests for Fork Mode Execution

- **Type**: Testing
- **Files**: `internal/slash/executor_test.go`
- **Description**: Add fork mode test cases to existing executor test file: `context: fork` calls `ForkRunner.Run()` with processed content and agent type (TC-020), agent type passed from frontmatter, `ForkRunner` returns result string that becomes the command output, nil `ForkRunner` with fork command returns descriptive error, inline mode never calls `ForkRunner`, fork command with before/after hooks still runs hooks.
- **Dependencies**: Task 6.6
- **Est. Complexity**: Medium

### Task 11.2: Implement Fork Mode in Executor

- **Type**: Implementation
- **Files**: `internal/slash/executor.go`
- **Description**: In `Execute()`, when `cmd.Frontmatter.Context == "fork"`, call `e.forkRunner.Run(ctx, processedContent, cmd.Frontmatter.Agent)` and return the fork result. When `ForkRunner` is nil and fork mode is requested, return error `"fork mode unavailable: no runner configured"`. Inline mode remains unchanged.
- **Dependencies**: Task 11.1
- **Est. Complexity**: Low

### Task 11.3: Implement SubagentForkRunner

- **Type**: Implementation
- **Files**: `internal/app/runner.go`
- **Description**: Define `SubagentForkRunner` struct in the `app` package that wraps `*subagent.Manager`. Implement `Run(ctx context.Context, task string, agentType string) (string, error)` by creating a subagent request and delegating to `Manager.Run()`. Map the agent type string to the subagent configuration.
- **Dependencies**: Task 11.2
- **Est. Complexity**: Low

### Task 11.4: Wire ForkRunner Injection

- **Type**: Implementation
- **Files**: `internal/app/runner.go`
- **Description**: In runner initialization, create `SubagentForkRunner` with the existing subagent manager. Pass it to the executor via `slash.NewExecutor(slash.WithForkRunner(&SubagentForkRunner{Manager: subManager}))`. Ensure the ForkRunner is optional — if no subagent manager is available, fork mode returns an error gracefully.
- **Dependencies**: Task 11.3
- **Est. Complexity**: Low

**Checkpoint**: Fork mode works end-to-end. Skills with `context: fork` delegate to sub-agent.

---

## Phase 12: Integration Testing & Polish

**Purpose**: End-to-end validation, edge cases, security testing, documentation.

### Task 12.1: Write Integration Test

- **Type**: Testing
- **Files**: `internal/slash/integration_test.go`
- **Description**: Create temp `.foxharness/commands/` and `.foxharness/skills/` directories with realistic `.md` test files. Test full flow: discovery → registry load → lookup → execute → result. Cover TC-001 (single-file discovery), TC-002 (directory-format skill), TC-003 (namespace mapping), TC-023 (user-level global commands). Test that built-in and file-based commands coexist (TC-011). Test registry refresh picks up new files.
- **Dependencies**: Task 11.4, Task 10.4
- **Est. Complexity**: High

### Task 12.2: Write Built-in Compatibility Tests

- **Type**: Testing
- **Files**: `internal/tui/slash_test.go`
- **Description**: Test that all 10 built-in commands (help, clear, model, exit, compact, plan, thinking, debug, tools, quit) are registered in the registry and behave identically after migration (TC-010). Test `/help` shows all commands, `/clear` resets conversation, `/model` switches model, `/exit` and `/quit` trigger shutdown. Verify backward compatibility — no behavior changes.
- **Dependencies**: Task 8.3
- **Est. Complexity**: Medium

### Task 12.3: Write Edge Case Tests

- **Type**: Testing
- **Files**: `internal/slash/edge_test.go`
- **Description**: Test edge cases: empty `.md` file registers but logs warning (EC-001), circular symlinks handled without infinite loop (EC-002), concurrent file modification doesn't crash (EC-003), very long content > 1MB is skipped (EC-004), special characters in arguments handled safely (EC-005), file-based command overrides built-in with warning log (EC-007), shell embedding with no output (EC-009), missing named arg → empty string (EC-010).
- **Dependencies**: Task 12.1
- **Est. Complexity**: Medium

### Task 12.4: Write Security Tests

- **Type**: Testing
- **Files**: `internal/slash/security_test.go`
- **Description**: Test NFR-002 security requirements: path traversal in content (e.g., `../../etc/passwd`) is not directly accessible, frontmatter YAML parsing does not execute code (no code execution during unmarshal), shell commands run in the configured workDir not arbitrary paths, allowed-tools restriction is enforced at the registry level (FilteredRegistry blocks disallowed tools), no privilege escalation in shell execution, symlink escape prevention.
- **Dependencies**: Task 12.1
- **Est. Complexity**: Medium

### Task 12.5: Run Full Test Suite

- **Type**: Verification
- **Files**: N/A
- **Description**: Run `go test ./...` to verify no regressions in existing tests. Run `go test -v ./internal/slash/...` for detailed output of all new tests. Run `go test -cover ./internal/slash/...` for coverage report. Verify all tests pass cleanly.
- **Dependencies**: Task 12.4
- **Est. Complexity**: Low

### Task 12.6: Code Formatting and Documentation

- **Type**: Documentation
- **Files**: `internal/slash/doc.go`, `internal/slash/skilltool/doc.go`
- **Description**: Run `gofmt -w ./internal/slash/` to format all files. Complete `doc.go` files with comprehensive package documentation (`// Package slash provides ...`). Verify all exported identifiers have godoc-compatible block comments. Remove any line-level teaching comments. Ensure comments start with the name being documented.
- **Dependencies**: Task 12.5
- **Est. Complexity**: Low

### Task 12.7: Verify All Spec Test Cases Pass

- **Type**: Verification
- **Files**: N/A
- **Description**: Verify all 32 spec test cases (TC-001 through TC-032) have corresponding passing tests. Create a traceability matrix mapping each TC to its test function(s). Run the full suite and confirm all pass. Report any gaps.
- **Dependencies**: Task 12.6
- **Est. Complexity**: Low

**Checkpoint**: All tests pass. All spec requirements verified. Code formatted and documented.

---

## Execution Order

```
Phase 1: 1.1 → 1.2 → 1.3 → 1.4 → 1.5
          │
          ├──────────────────────────────────────────────────────────┐
          │                                                          │
Phase 2: 2.1 → 2.2                                                 │
          │        │                                                │
Phase 3: 3.1 → 3.2─┤                                               │
          │        │                                                │
          │    3.3 ─┤ (needs 3.2 + 2.2)                            │
          │        │                                                │
          │    3.4 ─┘                                               │
          │         │                                               │
Phase 4: [P] 4.1 → 4.2                                             │
          │              │                                          │
Phase 5: [P] ┌─ 5.1 → 5.2 ─┐                                      │
          │  └─ 5.3 → 5.4 ─┤                                      │
          │                   │                                    │
Phase 7: [P] 7.1 → 7.2      │                                    │
          │              │   │                                    │
Phase 6: ┌─ 6.1 → 6.2 ─┐ │                                    │
         └─ 6.3 → 6.4 ─┤ │                                    │
                           │ │                                    │
                    6.5 ──┤ (needs 4.2 + 5.2 + 5.4 + 6.2 + 6.4)
                     │     │                                    │
                    6.6    │                                    │
                     │     │                                    │
Phase 8: 8.0 ←── (needs 1.3)
          │
          8.1 ◄──────┼─────┘ (needs 3.4 + 7.2)                  │
          │         │                                          │
          ├─ 8.2    │                                          │
          ├─ 8.3    │                                          │
          ├─ 8.4    │                                          │
          ├─ 8.5 ◄──┤ (needs 8.4 + 4.2)                       │
          └─ 8.6    │ (needs 8.1 + 6.6)                        │
               │    │                                          │
Phase 9: 9.1 ◄─────┘ (needs 6.6)                                │
          │                                                      │
          ├─ 9.2 [P] → 9.3 [P]                                 │
          ├─ 9.4 [P] → 9.5 [P]                                 │
          │               │                                     │
          │          9.6 ◄┘ (needs 9.5 + 8.6)                   │
          │           │                                         │
          │          9.7                                        │
          │           │                                         │
Phase 10: 10.1 [P] → 10.2 [P]                                  │
            │                                                    │
           10.3 (needs 10.2 + 3.4)                               │
            │                                                    │
           10.4 (needs 10.3 + 9.7)                               │
            │                                                    │
Phase 11: 11.1 → 11.2 → 11.3 → 11.4                            │
            │                                                    │
Phase 12: ┌─ 12.1 (needs 11.4 + 10.4)                           │
          │    │                                                  │
          │    ├─ 12.3 → 12.4 → 12.5 → 12.6 → 12.7             │
          │                                                      │
          └─ 12.2 [P] (needs 8.3)                               │
```

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1** (Foundation): No dependencies — start here
- **Phase 2** (Discovery): Depends on Phase 1 (frontmatter types)
- **Phase 3** (Registry): Depends on Phase 1 + Phase 2 (discovery results)
- **Phase 4** (Arguments): Depends on Phase 1 only — **parallel with Phases 2-3**
- **Phase 5** (Shell & Variables): Depends on Phase 1 only — **parallel with Phases 2-4**
- **Phase 6** (Executor): Depends on Phases 4 + 5 (arguments, shell, variables, hooks, filter)
- **Phase 7** (Fuzzy Search): Depends on Phase 1 only — **parallel with Phases 2-6**
- **Phase 8** (TUI): Depends on Phases 3 + 7 (registry + fuzzy)
- **Phase 9** (Skill Tool): Depends on Phase 6 (executor) + Phase 8 (runner integration)
- **Phase 10** (Conditional): Depends on Phase 3 (registry) + Phase 9 (system prompt)
- **Phase 11** (Fork Mode): Depends on Phase 6 (executor)
- **Phase 12** (Polish): Depends on Phases 10 + 11 (all features complete)

### Parallel Opportunities

- **Phases 4, 5, 7** can all run in parallel after Phase 1 completes
- **Phase 5 tasks 5.1-5.2** (shell) and **5.3-5.4** (variables) are independent within the phase
- **Phase 6 tasks 6.1-6.2** (hooks) and **6.3-6.4** (filter) are independent within the phase
- **Phase 9 tasks 9.2-9.3** (prompt) and **9.4-9.5** (tool) are independent within the phase
- **Phase 10 tasks 10.1-10.2** can start immediately after Phase 1 (no dependency on later phases)
- **Phase 12 task 12.2** (built-in compat) can run in parallel with 12.1

## Execution Strategy

### MVP Path (Minimum Viable Slash Commands)

1. Complete Phase 1 → Phase 2 → Phase 3 → Phase 7 → Phase 8
2. **STOP and VALIDATE**: Users can create `.md` files and see them in autocomplete
3. Manual test: `go run ./cmd/fox`, type `/`, verify commands appear

### Full Feature Path

1. MVP Path above
2. Phase 4 + Phase 5 + Phase 6 (executor pipeline)
3. Phase 9 (model-side skill tool)
4. Phase 10 (conditional activation) + Phase 11 (fork mode)
5. Phase 12 (integration & polish)

---

## Notes

- **[P]** = parallelizable (different files, no dependencies on each other)
- **TDD mandatory**: Every implementation task has a preceding test task
- Each task involves exactly one primary file
- Commit after each task or logical group
- Stop at any checkpoint to validate independently
- Run `go test ./internal/slash/...` frequently during development

---

## Phase 13: Post-review fixes

Added after a Codex review of the Phase 1–12 implementation surfaced three integration gaps (one P1 + two P2) plus one related issue. All four are tracked under `plan.md` **Revisions (post-implementation review)** — R1–R4. Each fix follows the same TDD cycle as the original phases.

### Task 13.1: Honor precedence in conditional activation

- **Type**: Implementation + tests
- **Files**: `internal/slash/conditional.go`, `internal/slash/registry.go`, `internal/slash/conditional_test.go`
- **Description**: `ConditionalSkills.Add` now performs `existing.Source > cmd.Source` precedence checks before storing; same-precedence overwrites log a warning. `Registry` factors out a shared `activateLocked(cmd) bool` helper used by both `registerLocked` and `CheckConditional`. Adds tests `TestConditionalSkills_Add_HigherPrecedenceWins`, `TestConditionalSkills_Add_LowerPrecedenceIgnored`, `TestRegistry_CheckConditional_PrecedenceProtectsActive`.
- **Related spec**: EC-011 (new)
- **Dependencies**: Task 10.3
- **Est. Complexity**: Low

### Task 13.2: Fork runner reads live session and provider

- **Type**: Implementation + test
- **Files**: `internal/app/runner.go`, `internal/app/fork_runner_test.go`
- **Description**: `subagentForkRunner` replaces the snapshot fields with `getManager func() *subagent.Manager` and `getSession func() string` callbacks. `AgentRunner` exposes `currentSubagentManager()` (builds a fresh manager bound to the current `llmProvider`) and `currentSessionIDLocked()` (returns the live session id). Construction of `slashExecutor` is moved to after the `AgentRunner` struct is allocated so the methods can be taken as values. Adds `TestSubagentForkRunner_UsesLiveGetters`.
- **Related spec**: EC-012 (new)
- **Dependencies**: Task 11.4
- **Est. Complexity**: Low

### Task 13.3: Enforce allowed-tools at the tool registry

- **Type**: Implementation + tests
- **Files**: `internal/slash/executor.go`, `internal/slash/executor_test.go`, `internal/slash/integration_test.go`, `internal/slash/edge_test.go`, `internal/slash/skilltool/tool.go`, `internal/tui/model.go`, `internal/tui/slash_registry.go`, `internal/tui/slash_registry_test.go`, `internal/app/runner.go`
- **Description**: `Executor.Execute` now returns `ExecutionResult{Content, AllowedTools, Fork}` instead of a plain string. The TUI's `executePromptCommand` routes results through a new `startPromptRestricted(text, allowedTools)` helper. When `allowedTools` is non-empty the TUI type-asserts the runner to an optional `restrictedRunner { RunRestricted(...) }` interface; `*AgentRunner` implements it and wraps the engine registry in `slash.NewFilteredRegistry(base, allowed)` for that single run. If the runner cannot enforce the restriction, the command is refused with an error entry. Tests: `TestExecutor_InlineMode_SurfacesAllowedTools`, `TestExecutor_ForkMode_OmitsAllowedTools`, `TestModel_AllowedTools_RoutesToRunRestricted`, `TestModel_NoAllowedTools_UsesRegularRun`, `TestModel_AllowedTools_UnsupportedRunner_ErrorsOut`.
- **Related spec**: REQ-011, NFR-002 (clarified)
- **Dependencies**: Task 8.6, Task 6.4, Task 6.6
- **Est. Complexity**: High

### Task 13.4: Show fork-mode results as assistant entries

- **Type**: Implementation
- **Files**: `internal/tui/model.go`
- **Description**: When `ExecutionResult.Fork` is true the TUI shows the sub-agent report as an assistant entry rather than starting a new turn — sending the report back as a user prompt would otherwise cause the model to act on its own output. Light behavioral correction surfaced while adapting the TUI to the new `ExecutionResult` shape.
- **Related spec**: REQ-011 (fork mode result handling)
- **Dependencies**: Task 13.3
- **Est. Complexity**: Low

### Phase 13 TC additions

- TC-021 → `TestModel_AllowedTools_RoutesToRunRestricted`, `TestModel_AllowedTools_UnsupportedRunner_ErrorsOut` (replaces the prior in-isolation `TestFilteredRegistry_*` mapping — those tests still exist but did not validate the integration)
- EC-011 → `TestConditionalSkills_Add_HigherPrecedenceWins`, `TestConditionalSkills_Add_LowerPrecedenceIgnored`, `TestRegistry_CheckConditional_PrecedenceProtectsActive`
- EC-012 → `TestSubagentForkRunner_UsesLiveGetters`

**Checkpoint**: `go test ./...` passes (88.6% / 69.0% coverage holds; no other package's tests regressed). `gofmt -l .` is clean.
