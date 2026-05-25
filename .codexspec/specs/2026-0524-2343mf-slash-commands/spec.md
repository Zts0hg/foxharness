# Feature: Slash Commands System

## Overview

Implement a file-based custom slash commands system for foxharness-go, modeled after Claude Code's slash commands architecture. Users place `.md` files in `.foxharness/commands/` or `.foxharness/skills/` directories to create new commands that can be invoked from the TUI with `/command-name` or autonomously by the LLM agent via a new `skill` tool.

This feature transforms the current hardcoded 10-command system into an extensible, file-driven command registry that supports frontmatter configuration, argument substitution, conditional activation, and model-side invocation.

## Goals

- **Extensibility**: Allow users to add custom commands by creating `.md` files without modifying Go source code
- **Parity**: Match the core capabilities of Claude Code's slash commands system
- **Unification**: Merge built-in and file-based commands into a single registry with consistent behavior
- **Model Integration**: Enable the LLM agent to discover and invoke skills autonomously
- **Backward Compatibility**: Preserve all existing built-in command behavior

## User Stories

### Story 1: Create a Custom Command

**As a** developer using foxharness
**I want** to create a `.md` file in `.foxharness/commands/review.md` and have it appear as `/review` in the TUI
**So that** I can define reusable prompts without writing Go code

**Acceptance Criteria:**
- [ ] Placing a `.md` file in `.foxharness/commands/` makes it available as a slash command
- [ ] The command name derives from the filename (without `.md` extension)
- [ ] The command content is sent as the user prompt when invoked
- [ ] The command appears in autocomplete when typing `/`

### Story 2: Configure Command Behavior with Frontmatter

**As a** developer
**I want** to add YAML frontmatter to my `.md` files to configure description, arguments, allowed tools, and model
**So that** each command can have tailored behavior

**Acceptance Criteria:**
- [ ] YAML frontmatter between `---` delimiters is parsed correctly
- [ ] `description` field controls what appears in autocomplete
- [ ] `arguments` field defines named parameters for substitution
- [ ] `allowed-tools` field restricts which tools the command can use
- [ ] `model` field overrides the default model for that command
- [ ] `effort` field sets reasoning effort level
- [ ] Invalid frontmatter produces a descriptive error, not a crash

### Story 3: Pass Arguments to Commands

**As a** developer
**I want** to invoke `/review pr-123` and have `pr-123` substituted into the command content
**So that** commands can accept dynamic input

**Acceptance Criteria:**
- [ ] `$ARGUMENTS` is replaced with the full argument string
- [ ] `$ARGUMENTS[0]`, `$ARGUMENTS[1]` access arguments by index
- [ ] `$0`, `$1` are shorthand for indexed access
- [ ] Named parameters (e.g., `$file`) map from frontmatter `arguments` field
- [ ] If no placeholder exists in content but user provides args, args are appended
- [ ] Progressive argument hints show remaining unfilled parameter names

### Story 4: Organize Commands with Namespaces

**As a** developer
**I want** to organize commands into subdirectories like `.foxharness/commands/db/migrate.md`
**So that** related commands are grouped and invoked as `/db:migrate`

**Acceptance Criteria:**
- [ ] Subdirectory path maps to colon-separated command name
- [ ] `db/migrate.md` becomes `/db:migrate`
- [ ] `db/seed.md` becomes `/db:seed`
- [ ] Both appear under a `db` namespace in autocomplete

### Story 5: Fuzzy Search Commands

**As a** developer
**I want** to type `/rev` and see `review` suggested even if it's not an exact prefix match
**So that** I can quickly find commands with partial or approximate names

**Acceptance Criteria:**
- [ ] Typing `/` lists all available commands
- [ ] Typing partial text after `/` filters commands with fuzzy matching
- [ ] Results are weighted: exact name match > name prefix > description match
- [ ] Commands are grouped: builtin → user-level → project-level
- [ ] Within each group, commands are sorted alphabetically

### Story 6: Model-side Skill Invocation

**As a** the LLM agent
**I want** to invoke a skill via the `skill` tool
**So that** I can use specialized prompts for specific tasks without user intervention

**Acceptance Criteria:**
- [ ] A `skill` tool is registered in the tool registry
- [ ] Available skills are listed in the system prompt within token budget
- [ ] Skills with `disable-model-invocation: true` are excluded from model invocation
- [ ] Skills with `user-invocable: false` are hidden from user autocomplete but visible to the model
- [ ] Model invocation follows the same argument substitution rules as user invocation

### Story 7: Conditional Skill Activation

**As a** developer
**I want** to define a skill with `paths: ["*_test.go"]` that only activates when the engine operates on test files
**So that** specialized skills don't clutter the command list until they're relevant

**Acceptance Criteria:**
- [ ] Skills with `paths` frontmatter are hidden from autocomplete initially
- [ ] When the engine's `read_file` or `write_file` operates on a matching file, the skill activates
- [ ] Activated skills appear in autocomplete and the model's skill list for the rest of the session
- [ ] Glob patterns support `*`, `**`, and `?` wildcards
- [ ] Multiple `paths` patterns use OR logic (any match activates)

### Story 8: Shell Command Embedding

**As a** developer
**I want** to embed `` !`git log --oneline -5` `` in my `.md` file and have the shell output injected into the prompt
**So that** commands can include dynamic runtime information

**Acceptance Criteria:**
- [ ] `` !`command` `` syntax executes the shell command and replaces with its stdout
- [ ] Shell command failures produce an error message in the prompt, not a crash
- [ ] Multiple shell embeddings in a single `.md` file are all executed
- [ ] Shell execution timeout prevents hanging (configurable, default 30s)

### Story 9: Fork Mode Execution

**As a** developer
**I want** to set `context: fork` and `agent: "general-purpose"` in frontmatter
**So that** a skill runs as an isolated sub-agent instead of inline in the main conversation

**Acceptance Criteria:**
- [ ] `context: fork` launches the skill as a sub-agent via the existing delegate_task mechanism
- [ ] `agent` field specifies the agent type for the sub-agent
- [ ] Fork-mode results are injected back into the main conversation as a tool result
- [ ] Default context (no `context` field) runs inline as a regular prompt

### Story 10: User-level Global Commands

**As a** developer
**I want** to place commands in `~/.foxharness/commands/` that work across all my projects
**So that** I have personal commands available everywhere

**Acceptance Criteria:**
- [ ] `~/.foxharness/commands/*.md` files are loaded as global user commands
- [ ] Project-level commands take precedence over user-level commands with the same name
- [ ] User-level commands are labeled as "user" in autocomplete grouping

## Functional Requirements

### REQ-001: File Discovery and Loading

The system must discover and load `.md` command/skill files from:

1. **User-level directories**: `~/.foxharness/commands/` and `~/.foxharness/skills/`
2. **Project-level directories**: `.foxharness/commands/` and `.foxharness/skills/` (relative to project root)

**Loading order**: User-level loaded first, then project-level. When names conflict, project-level takes precedence.

**Search scope**: Project-level search traverses from `cwd` up to the git root (detected via `.git` directory). Only the nearest `.foxharness/` directory is used.

**File deduplication**: Files with the same device ID and inode number (via `os.Stat`) are treated as duplicates. This prevents symlink-induced duplication.

### REQ-002: File Format Support

**Commands directory** (`.foxharness/commands/`):
- Single-file format: `my-command.md` → command name `my-command`
- Directory format: `my-command/SKILL.md` → command name `my-command`
- When a directory contains `SKILL.md`, only that file is loaded; other `.md` files in the directory are ignored

**Skills directory** (`.foxharness/skills/`):
- Directory format only: `my-skill/SKILL.md` → command name `my-skill`
- Single `.md` files directly under `skills/` are NOT loaded

**Namespacing**: Subdirectories map to colon-prefixed names:
- `commands/db/migrate.md` → `db:migrate`
- `skills/testing/go-test/SKILL.md` → `testing:go-test`

### REQ-003: YAML Frontmatter Parsing

Each `.md` file may contain a YAML frontmatter block delimited by `---` at the start of the file:

```yaml
---
description: "Command description text"
arguments: "file message branch"
argument-hint: "[file] [message]"
allowed-tools:
  - read_file
  - bash
model: "glm-4.5-air"
effort: "high"
user-invocable: true
disable-model-invocation: false
when_to_use: "Use when reviewing pull requests"
context: "inline"
agent: "general-purpose"
paths:
  - "src/**/*.go"
aliases:
  - "r"
hooks:
  before: "echo 'starting'"
  after: "echo 'done'"
version: "1.0"
---
```

**Frontmatter fields**:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `description` | string | First line of content | Displayed in autocomplete and help |
| `arguments` | string | "" | Space-separated parameter names for named substitution |
| `argument-hint` | string | Auto-generated | Custom hint text shown during progressive argument input |
| `allowed-tools` | []string | All tools | Restrict which tools the command can use |
| `model` | string | Session default | Override the LLM model for this command |
| `effort` | string | Session default | Set reasoning effort ("low", "medium", "high") |
| `user-invocable` | bool | true | Whether users can invoke this command |
| `disable-model-invocation` | bool | false | Prevent the LLM from auto-invoking this skill |
| `when_to_use` | string | "" | Context description for the model to decide when to use |
| `context` | string | "inline" | Execution mode: "inline" or "fork" |
| `agent` | string | "" | Agent type for fork mode |
| `paths` | []string | [] | File globs for conditional activation |
| `aliases` | []string | [] | Alternative names for the command |
| `hooks` | object | nil | Before/after shell hooks |
| `version` | string | "" | Command version metadata |

**Parsing rules**:
- Frontmatter is optional; files without it use defaults
- The `---` delimiters must be the first line of the file
- Content after the closing `---` is the command body
- Invalid YAML produces a warning log and falls back to defaults

### REQ-004: Unified Command Registry

All commands (built-in and file-based) are managed through a unified `CommandRegistry`:

```go
type CommandType int

const (
    CommandBuiltin CommandType = iota
    CommandPrompt
)

type Command struct {
    Type        CommandType
    Name        string
    Description string
    Aliases     []string
    Source      CommandSource
    Hidden      bool

    // Prompt-command specific fields
    Frontmatter Frontmatter
    Content     string
    FilePath    string
    SkillDir    string

    // Builtin-command specific fields
    Handler     func(args []string) (tea.Model, tea.Cmd)
}

type CommandSource int

const (
    SourceBuiltin CommandSource = iota
    SourceUser
    SourceProject
)

type CommandRegistry struct {
    commands map[string]*Command
}
```

**Registry operations**:
- `Register(cmd *Command)` — add a command; project-level overwrites user-level with same name
- `Lookup(name string) (*Command, bool)` — find by name or alias
- `All() []*Command` — return all commands
- `UserInvocable() []*Command` — return commands with `user-invocable: true`
- `ModelInvocable() []*Command` — return commands without `disable-model-invocation: true`
- `ActivateConditional(name string)` — move a conditional skill into the active set

**Precedence rules** (from lowest to highest):
1. Built-in commands
2. User-level file-based commands (from `~/.foxharness/`)
3. Project-level file-based commands (from `.foxharness/`)

When a higher-priority command shares a name with a lower-priority one, the higher-priority version replaces it and a warning is logged. For example, a file-based `/help` command overrides the built-in `/help`.

**Built-in command migration**: The existing 10 hardcoded commands (`/help`, `/clear`, `/model`, etc.) are registered as `CommandBuiltin` type with their handler functions. The `handleSlashCommand()` switch statement is replaced by registry lookups.

### REQ-005: Argument Substitution

When a prompt command is invoked with arguments, the system performs placeholder substitution on the content:

**Placeholder types**:

| Placeholder | Example | Replacement |
|-------------|---------|-------------|
| `$ARGUMENTS` | `/cmd hello world` | `hello world` |
| `$ARGUMENTS[0]` | `/cmd hello world` | `hello` |
| `$ARGUMENTS[1]` | `/cmd hello world` | `world` |
| `$0`, `$1`, `$2` | `/cmd a b c` | `a`, `b`, `c` |
| `$name` (named) | `/cmd foo` (with `arguments: "name"`) | `foo` |

**Substitution rules**:
1. Parse the user input after the command name using shell-style quoting (respect double-quoted strings)
2. Replace all placeholders in the content
3. If the content contains no `$` placeholders but the user provided arguments, append `\n\nARGUMENTS: {args}` to the content
4. If a named parameter references an index beyond the provided arguments, replace with empty string

**Progressive argument hints**:
- When `arguments: "file message branch"` is set, the autocomplete shows `[file] [message] [branch]`
- After the user types one argument, the hint updates to `[message] [branch]`
- Custom `argument-hint` overrides the auto-generated hint

### REQ-006: TUI Autocomplete with Fuzzy Search

**Trigger**: When the user types `/` in the input field.

**Behavior**:
1. `/` alone: show all `user-invocable` commands, grouped and sorted
2. `/partial`: show commands matching `partial` with fuzzy scoring

**Scoring weights**:
- Exact name match: 100
- Name prefix match: 80
- Name contains match: 60
- Alias match: 50
- Description contains match: 20

**Display grouping** (in order):
1. Built-in commands (alphabetical)
2. User-level commands (alphabetical)
3. Project-level commands (alphabetical)

**UI**: Reuses the existing `renderSlashSuggestions()` pattern in `internal/tui/view.go` but draws from the registry instead of the hardcoded `slashCommands` slice.

**Selection**: Tab key cycles through matches; Enter selects and completes the command name.

### REQ-007: Shell Command Embedding

Prompt content may contain embedded shell commands using the syntax:

```
!`command args`
```

**Processing rules**:
- Execute the command via `exec.Command("sh", "-c", command)`
- Capture stdout; stderr is ignored
- Replace the `!`command`` with the captured stdout (trimmed)
- If the command fails (non-zero exit), replace with `[ERROR: command failed: exit code N]`
- Enforce a timeout (default 30 seconds, configurable)
- Multiple embeddings in a single file are executed sequentially

### REQ-008: Special Variable Replacement

After argument substitution and shell embedding, the following special variables are replaced:

| Variable | Replacement |
|----------|-------------|
| `${FOXHARNESS_SKILL_DIR}` | Absolute path to the directory containing the `.md` file |
| `${FOXHARNESS_SESSION_ID}` | Current session UUID |

### REQ-009: Model-side Skill Tool

A new `skill` tool implementing `BaseTool` is registered in the tool registry:

```go
type SkillTool struct {
    registry *CommandRegistry
    runner   tui.Runner
}
```

**Tool definition**:
- Name: `skill`
- Description: "Invoke a named skill with arguments"
- Input schema: `{ "name": string, "arguments": string }`

**Execution flow**:
1. Look up the skill by name in the registry
2. Check `disable-model-invocation` is false
3. Perform argument substitution on the skill content
4. If `context: "fork"`, delegate to sub-agent
5. If `context: "inline"` (default), inject the processed content as a user message

**System prompt injection**: The `PromptComposer` includes a formatted list of model-invocable skills in the system prompt, respecting a character budget derived from the model's context window. Each skill entry shows: name, description, when_to_use (if present), argument_hint (if present).

**Token budget mechanism** (modeled after Claude Code):

The skill list is allocated **1% of the context window** in characters:
```
charBudget = contextWindowTokens × 4 (chars/token) × 0.01
fallback   = 8,000 characters (when context window size is unknown)
```

Each skill description is capped at 250 characters before truncation. The system uses a 3-level truncation strategy:

| Level | Condition | Behavior |
|-------|-----------|----------|
| No truncation | Total description length ≤ budget | Full descriptions for all skills |
| Normal truncation | Budget exceeds names but descriptions must shrink | Evenly distribute remaining budget across non-bundled skill descriptions (min 20 chars each) |
| Extreme truncation | Budget cannot fit any descriptions | Non-bundled skills show name only (`- skillname`) |

Built-in skills are never truncated — they always get full descriptions regardless of budget pressure.

### REQ-010: Conditional Skill Activation

Skills with a non-empty `paths` frontmatter field are conditionally activated:

**Loading behavior**:
- Conditional skills are loaded but not added to the active command set
- They are stored in a separate map: `conditionalSkills map[string]*ConditionalSkill`

**Activation trigger**:
- When the engine executes `read_file` or `write_file`, the operated file path is checked against all conditional skills' `paths` globs
- On match, the skill is moved to the active command set
- The model's skill list in the system prompt is updated
- The skill remains active for the rest of the session

**Glob matching**:
- Uses `filepath.Match` or a glob library supporting `**`
- Patterns are evaluated relative to the project root
- Multiple patterns use OR logic

### REQ-011: Execution Modes

**Inline mode** (default):
- The processed command content is injected as a user message in the current conversation
- The engine processes it as a normal prompt
- `allowed-tools` restricts the available tools for this turn

**Fork mode** (`context: fork`):
- The processed command content is sent to a sub-agent via the existing `delegate_task` mechanism
- The `agent` frontmatter field specifies the agent type
- The sub-agent runs in isolation; its result is returned as a tool result in the main conversation

### REQ-012: Skill Hooks

Frontmatter `hooks` field supports before/after shell commands:

```yaml
hooks:
  before: "echo 'Starting review'"
  after: "echo 'Review complete'"
```

- `before` hook runs before the command content is processed
- `after` hook runs after the command execution completes
- Hook failures are logged but do not block command execution

### REQ-013: Caching

**File loading cache**:
- Command files are loaded once when the registry is initialized
- A file watcher (or explicit refresh) invalidates the cache when `.md` files change
- Cache key: directory path + file path

**Registry cache**:
- The `All()` and `UserInvocable()` results are cached
- Cache is invalidated when commands are registered, activated, or files change

## Non-Functional Requirements

### NFR-001: Performance

- Command file loading must complete in under 100ms for up to 50 `.md` files
- Autocomplete filtering must respond in under 10ms for up to 100 commands
- Argument substitution must complete in under 1ms

### NFR-002: Security

- Shell command embedding is only executed for project-level commands, not user-level commands from remote sources
- Shell commands run with the user's current permissions (no privilege escalation)
- Shell execution timeout prevents indefinite hangs
- `allowed-tools` restriction is enforced at the tool registry level, not just advisory. The runtime MUST wrap the engine's tool registry in `FilteredRegistry` for the turn in which a restricted command is invoked. Both the per-call tool definitions returned to the model (`GetAvailableTools`) and the dispatch (`Execute`) MUST honor the allow-list. If the runtime cannot enforce this (e.g. a test mock without the restricted path), it MUST refuse to run the command rather than silently falling back to an unrestricted run.
- Frontmatter parsing must not execute arbitrary code
- File paths in `.md` content are validated to prevent path traversal

### NFR-003: Reliability

- Invalid `.md` files (malformed frontmatter, missing content) must not prevent other commands from loading
- File system errors during discovery must be logged and skipped, not crash the application
- The system must degrade gracefully: if no `.foxharness/` directory exists, built-in commands work normally
- Concurrent file access must be handled safely

### NFR-004: Compatibility

- All existing built-in commands must continue to work identically after migration to the unified registry
- The `Runner` interface must not change; the TUI interacts with commands through the registry
- Existing session format must not be affected by the new command system

## Acceptance Criteria (Test Cases)

### TC-001: Single-file command discovery
**Given** a file `.foxharness/commands/review.md` exists
**When** the TUI starts
**Then** `/review` appears in autocomplete and is invocable

### TC-002: Directory-format skill discovery
**Given** a directory `.foxharness/skills/go-test/SKILL.md` exists
**When** the TUI starts
**Then** `/go-test` appears in autocomplete and is invocable

### TC-003: Namespace command discovery
**Given** a file `.foxharness/commands/db/migrate.md` exists
**When** the TUI starts
**Then** `/db:migrate` appears in autocomplete

### TC-004: Frontmatter parsing
**Given** a `.md` file with valid YAML frontmatter
**When** the file is loaded
**Then** all frontmatter fields are correctly parsed into the Command struct

### TC-005: Frontmatter missing
**Given** a `.md` file with no frontmatter
**When** the file is loaded
**Then** description defaults to first line of content, all other fields use defaults

### TC-006: Argument substitution - full arguments
**Given** a command with content `Review this: $ARGUMENTS`
**When** invoked as `/review pr-123`
**Then** the processed content is `Review this: pr-123`

### TC-007: Argument substitution - indexed
**Given** a command with content `File: $0, Message: $1`
**When** invoked as `/commit main.go fix bug`
**Then** the processed content is `File: main.go, Message: fix`

### TC-008: Argument substitution - named
**Given** a command with `arguments: "file message"` and content `$file: $message`
**When** invoked as `/commit main.go fix bug`
**Then** the processed content is `main.go: fix`

### TC-009: Auto-append arguments
**Given** a command with content `Please review the code` (no `$` placeholders)
**When** invoked as `/review pr-123`
**Then** the processed content is `Please review the code\n\nARGUMENTS: pr-123`

### TC-010: Built-in commands preserved
**Given** the unified registry is active
**When** `/help`, `/clear`, `/model`, `/exit` are invoked
**Then** each behaves identically to the pre-migration implementation

### TC-011: Unified autocomplete
**Given** both built-in and file-based commands exist
**When** `/` is typed in the TUI
**Then** all commands appear in autocomplete grouped by source

### TC-012: Fuzzy search
**Given** a command `/review` exists
**When** `/rev` is typed
**Then** `/review` appears in the filtered suggestions

### TC-013: Model-side skill invocation
**Given** a skill `review` is registered with `disable-model-invocation: false`
**When** the LLM calls the `skill` tool with `{"name": "review", "arguments": "pr-123"}`
**Then** the skill content is processed and executed

### TC-014: Disable model invocation
**Given** a skill `internal` is registered with `disable-model-invocation: true`
**When** the model's skill list is generated
**Then** `internal` is not included

### TC-015: User-invocable false
**Given** a skill `helper` is registered with `user-invocable: false`
**When** the user types `/` in the TUI
**Then** `helper` does not appear in autocomplete
**When** the model's skill list is generated
**Then** `helper` IS included

### TC-016: Conditional activation
**Given** a skill `go-test` with `paths: ["*_test.go"]` is registered
**When** the engine reads `internal/engine/loop_test.go`
**Then** `go-test` becomes activated and appears in autocomplete

### TC-017: Shell command embedding
**Given** a command with content `Current branch: !`git branch --show-current``
**When** the command is invoked
**Then** the shell output replaces the `!`...`` syntax

### TC-018: Shell command failure
**Given** a command with content `!`nonexistent-command``
**When** the command is invoked
**Then** the content contains `[ERROR: command failed: ...]` instead of crashing

### TC-019: Special variable replacement
**Given** a command with content `Skill dir: ${FOXHARNESS_SKILL_DIR}, Session: ${FOXHARNESS_SESSION_ID}`
**When** the command is invoked in a session
**Then** both variables are replaced with actual values

### TC-020: Fork mode execution
**Given** a command with `context: fork` and `agent: "general-purpose"`
**When** invoked
**Then** execution delegates to a sub-agent via `delegate_task`

### TC-021: Allowed-tools restriction
**Given** a command with `allowed-tools: ["read_file", "bash"]`
**When** the command is executed
**Then** only `read_file` and `bash` are available during execution

### TC-022: Project-level overrides user-level
**Given** `~/.foxharness/commands/review.md` and `.foxharness/commands/review.md` both exist
**When** the registry is loaded
**Then** the project-level version is used

### TC-023: User-level global commands
**Given** `~/.foxharness/commands/my-global.md` exists
**When** the TUI starts in any project
**Then** `/my-global` appears in autocomplete as a user-level command

### TC-024: File deduplication
**Given** two directory entries pointing to the same file (same device+inode)
**When** the registry loads
**Then** the command is registered only once

### TC-025: Skill hooks
**Given** a command with `hooks: { before: "echo start", after: "echo done" }`
**When** the command is invoked
**Then** the before hook runs before processing, after hook runs after execution

### TC-026: Progressive argument hints
**Given** a command with `arguments: "file message branch"`
**When** the user types `/commit main.go` (one argument)
**Then** the hint shows `[message] [branch]` (remaining arguments)

### TC-027: Invalid frontmatter handling
**Given** a `.md` file with invalid YAML in frontmatter
**When** the file is loaded
**Then** a warning is logged, defaults are used, and the command is still registered

### TC-028: No .foxharness directory
**Given** no `.foxharness/` directory exists in the project or user home
**When** the TUI starts
**Then** only built-in commands are available (no errors)

### TC-029: Alias support
**Given** a command `review` with `aliases: ["r", "rev"]`
**When** `/r` is typed in the TUI
**Then** `review` is matched and can be invoked

### TC-030: Multiple paths conditional activation
**Given** a skill with `paths: ["*_test.go", "Makefile"]`
**When** the engine reads either a `_test.go` file or `Makefile`
**Then** the skill activates

### TC-031: Cache invalidation on file change
**Given** a cached command loaded from `.foxharness/commands/review.md`
**When** the file is modified on disk and the registry is refreshed
**Then** the updated content is returned on subsequent invocations

### TC-032: Cache hit on repeated queries
**Given** the registry cache is populated with all commands
**When** `All()` is called twice without any file changes or registrations
**Then** the same slice pointer is returned (no reload)

## Edge Cases

### EC-001: Empty .md file
A `.md` file with no content (only frontmatter or completely empty) should still register as a command but produce an empty prompt. Log a warning.

### EC-002: Circular symlinks
If symlink chains create cycles during file discovery, the system must not loop infinitely. Use `os.Lstat` and track visited inodes.

### EC-003: Concurrent file modification
If a `.md` file is modified while being loaded, the system should read a consistent snapshot. Use `os.ReadFile` which reads atomically on most filesystems.

### EC-004: Very long command content
A `.md` file with extremely long content (e.g., >1MB) should not crash the system. Enforce a configurable max file size (default 1MB).

### EC-005: Special characters in arguments
Arguments containing spaces (quoted), newlines, or shell metacharacters must be handled safely. Use proper shell-style quoting during argument parsing.

### EC-006: Missing frontmatter closing delimiter
A file with opening `---` but no closing `---` should treat the entire file as content (no frontmatter). Log a warning.

### EC-007: Command name conflicts with built-in
If a file-based command has the same name as a built-in (e.g., `/help`), the file-based version takes precedence. Log a warning about the override.

### EC-008: Skills directory with loose .md files
A `.md` file placed directly in `.foxharness/skills/` (not in a subdirectory) should be ignored with a debug log message.

### EC-009: Shell embedding with no output
If an embedded shell command succeeds but produces no stdout, replace with empty string (not an error).

### EC-010: Missing named argument
If frontmatter defines `arguments: "file message"` but user provides only one argument, `$message` should be replaced with empty string.

### EC-011: Conditional skill name collides with an active command
If a conditional skill's name matches an already-active command (built-in or file-based) and is then triggered by a path match, the registry MUST apply the same precedence rules used at load time (project > user > builtin). A lower-precedence conditional skill MUST NOT overwrite a higher-precedence active command on activation; it is suppressed and logged. Two conditional skills with the same name follow the same rule inside `ConditionalSkills.Add` itself.

### EC-012: Session or model swap during a session with fork-mode skills
If the user switches session (`/new`) or model (`/model X`) after a fork-mode skill has been registered, subsequent fork-mode invocations MUST use the new session id as `ParentSessionID` and the new model as the sub-agent's provider. The fork runner MUST NOT cache the session id or sub-agent manager captured at runner construction.

## Output Examples

### Example 1: Custom review command

**File**: `.foxharness/commands/review.md`
```markdown
---
description: "Review code for quality issues"
arguments: "scope"
argument-hint: "[scope: file or directory]"
aliases: ["r"]
---
You are a code reviewer. Review the following code for:

1. Bug risks and logic errors
2. Security vulnerabilities
3. Performance concerns
4. Style and readability

Scope: $ARGUMENTS
```

**Invocation**: `/review internal/engine/loop.go`
**Processed content**:
```
You are a code reviewer. Review the following code for:

1. Bug risks and logic errors
2. Security vulnerabilities
3. Performance concerns
4. Style and readability

Scope: internal/engine/loop.go
```

### Example 2: Conditional Go test skill

**File**: `.foxharness/skills/go-test/SKILL.md`
```markdown
---
description: "Run and fix Go tests"
paths: ["**/*_test.go", "**/testdata/**"]
when_to_use: "Use when test files are being modified or when tests need fixing"
---
Analyze the failing Go tests and fix them:

1. Run the relevant test: go test -v $ARGUMENTS
2. Identify the root cause of failures
3. Apply minimal fixes
4. Re-run tests to verify
```

### Example 3: Fork-mode deployment skill

**File**: `.foxharness/commands/deploy.md`
```markdown
---
description: "Deploy the application"
context: "fork"
agent: "general-purpose"
allowed-tools: ["bash", "read_file"]
model: "glm-4.5-air"
---
Deploy the application to the specified environment:

Environment: $ARGUMENTS

Steps:
1. Run tests: go test ./...
2. Build: go build -o bin/app ./cmd/fox
3. Deploy: ./scripts/deploy.sh $ARGUMENTS
```

### Example 4: Shell embedding

**File**: `.foxharness/commands/pr-summary.md`
```markdown
---
description: "Generate PR summary"
---
Generate a summary of recent changes for a pull request.

Current branch: !`git branch --show-current`
Recent commits:
!`git log --oneline -10`

$ARGUMENTS
```

## Out of Scope

The following are explicitly excluded from this feature:

1. **Runtime subdirectory skill discovery** — No automatic discovery of `.foxharness/skills/` in project subdirectories when the engine operates on files. Skills load only from project root and user-level directories.
2. **Usage tracking and dynamic ranking** — No persistent usage counts. Autocomplete uses static grouping and alphabetical sorting.
3. **Managed/policy commands** — No enterprise-administered command directories.
4. **Plugin skills** — No plugin system for dynamically loaded skill packages.
5. **MCP remote skills** — No Model Context Protocol integration for remote skill loading.
6. **Git worktree fallback** — No special handling for git worktrees missing `.foxharness/` directories.
7. **Template rendering** — No Go template syntax in `.md` files beyond the defined substitution placeholders.
8. **Skill versioning enforcement** — The `version` field is metadata only; no version compatibility checks.

## Architecture Notes

### New Packages

- `internal/slash/` — Command registry, file discovery, frontmatter parsing, argument substitution
- `internal/slash/skilltool/` — Model-side skill tool implementation

### Modified Packages

- `internal/tui/model.go` — Replace `slashCommands` slice and `handleSlashCommand()` with registry-based dispatch
- `internal/tui/view.go` — Update `renderSlashSuggestions()` to use registry
- `internal/app/runner.go` — Initialize command registry, register skill tool
- `internal/engine/loop.go` — Hook conditional activation into tool execution

### Key Integration Points

1. **Registry initialization** in `app/runner.go`:
   ```go
   registry := slash.NewRegistry(workDir)
   registry.Load() // discovers and loads all .md files
   registry.RegisterBuiltin(...) // registers built-in commands
   ```

2. **TUI dispatch** in `tui/model.go`:
   ```go
   // Before: switch statement with 10 cases
   // After:
   cmd, ok := m.registry.Lookup(commandName)
   if !ok { /* unknown command */ }
   switch cmd.Type {
   case CommandBuiltin:
       return cmd.Handler(args)
   case CommandPrompt:
       return m.executePromptCommand(cmd, args)
   }
   ```

3. **Skill tool registration** in `app/runner.go`:
   ```go
   tools = append(tools, skilltool.New(registry, runner))
   ```

4. **Conditional activation hook** in `engine/loop.go`:
   ```go
   // After each tool execution:
   for _, tool := range result.Tools {
       m.registry.CheckConditionalActivation(tool.FilePath)
   }
   ```
