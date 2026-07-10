# Implementation Plan: TUI Interaction Compatibility

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Feature ID**: `2026-0709-2113sj`
**Feature Branch**: `main-latest-20260709`
**Input**: `.codexspec/specs/2026-0709-2113sj-tui-interaction-compatibility/spec.md`
**Created**: 2026-07-09
**Status**: Draft

## Fidelity Check

`requirements.md` and `spec.md` are aligned for the first-phase scope:

- Codex CLI remains the primary baseline and Claude Code is supplemental only. Covers: REQ-001
- Foxharness-only workflows and fox Esc semantics remain preserved. Covers: REQ-002
- Confirmed slash command conflict resolutions are represented without adding deferred commands. Covers: REQ-003, REQ-012
- `/status`, `/statusline`, `/theme`, persistence, and entry-based rendering are fully specified. Covers: REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011
- The implementation remains bounded to TUI, app wiring, and settings persistence. Covers: NFR-001, NFR-004

No open requirement blocks planning.

## Existing Repository Constraints

- `internal/tui/model.go` owns the Bubble Tea model, slash command dispatch, queueing, Esc handling, sidebar state, plan mode state, and runtime snapshots. Covers: REQ-002, REQ-003, REQ-004, REQ-010, NFR-004
- `internal/tui/view.go` owns global lipgloss styles, status/footer rendering, transcript entry rendering, tool result folding, shell output folding, queued prompt previews, slash suggestions, and file mention suggestions. Covers: REQ-006, REQ-007, REQ-010, REQ-011
- `internal/tui/reporter.go` already converts tool events into entry-based `tool` transcript rows and has safe summary fallback logic. Covers: REQ-011
- `internal/tui/markdown.go` uses a cached glamour renderer keyed only by width today; theme-aware markdown will need theme-aware cache invalidation or cache keys. Covers: REQ-008, REQ-010
- `internal/settings/settings.go` already reads and atomically writes `~/.foxharness/settings.json` while preserving unknown raw JSON fields for existing LLM settings. Covers: REQ-009, NFR-003
- `internal/app/tui.go` is the narrow app-to-TUI construction point and can pass home directory and resolved provider display metadata without changing the agent runner core. Covers: REQ-004, REQ-009, NFR-001
- `internal/tui/model_test.go` already has fake runner coverage for slash commands, queueing, Esc behavior, model switching, plan mode, sidebar, tool rendering, and shell output folding. Covers: REQ-002, REQ-003, REQ-010, REQ-011, NFR-004
- Project constitution 2.0.0 requires TDD: failing tests first, minimal implementation, then refactor with tests passing. Covers: NFR-002

## Plan-Level Decisions

### PLD-001: Keep The Feature In TUI, App Wiring, And Settings Packages

**Decision**: Limit code changes to `internal/tui`, `internal/app/tui.go`, and `internal/settings` unless tests reveal an unavoidable local adapter need.
**Evidence**: Current TUI has all visible interaction state, settings already owns `~/.foxharness/settings.json`, and `RunTUI` already constructs `tui.Config`.
**Rationale**: This satisfies first-phase compatibility without modifying core agent, provider, permission, remote, plugin, MCP, or account systems.
**Trade-off**: `/status` can show provider/profile display metadata from the resolved CLI config, but it will not become a live provider-management UI.
Covers: REQ-004, REQ-009, REQ-012, NFR-001, NFR-004

### PLD-002: Persist TUI Preferences Under A Nested `tui` Settings Object

**Decision**: Store first-phase preferences as:

```json
{
  "tui": {
    "theme": "codex",
    "statusline": ["model", "project", "git-branch", "context-used", "plan-mode"]
  }
}
```

**Evidence**: Existing settings already has a top-level legacy `model` and an `llm` object; adding a nested `tui` object avoids collisions with existing top-level unknown fields such as `theme`.
**Rationale**: The nested object gives future TUI settings a stable namespace while preserving unknown top-level and nested fields.
**Trade-off**: Existing ad hoc top-level `theme` values remain preserved but are not treated as first-phase TUI configuration.
Covers: REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, NFR-003

### PLD-003: Use Text Slash Commands For First-Phase `/theme` And `/statusline`

**Decision**: Implement command-driven configuration first:

- `/theme` lists current and built-in theme names.
- `/theme <name>` applies and persists a built-in theme.
- `/statusline` lists current/default/available declarative items and usage.
- `/statusline set <items>` persists an ordered item list; items may be comma-separated, space-separated, or both.
- `/statusline default` restores the default item list.

**Evidence**: The current TUI has text slash command dispatch but no general reusable picker framework for arbitrary settings.
**Rationale**: Text commands satisfy declarative configuration and persistence without adding an unrelated modal interaction system.
**Trade-off**: This phase does not copy an interactive theme/statusline picker; that remains compatible with the spec because persistence only occurs after a valid explicit command.
Covers: REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, NFR-001, NFR-004

### PLD-004: Built-In Theme Registry Defaults To A Codex-Oriented Theme

**Decision**: Introduce a small built-in registry with at least `codex`, `amber`, `mono`, and `light`; default to `codex` when no valid setting exists.
**Evidence**: The existing view has hard-coded amber color constants and global lipgloss styles.
**Rationale**: A registry provides the confirmed built-in collection while preserving the current amber look as a selectable compatibility theme.
**Trade-off**: Palette names become implementation choices; custom files remain out of scope.
Covers: REQ-001, REQ-008, REQ-010, REQ-012

### PLD-005: Keep Tool Activity Entry-Based And Patch Missing Presentation States

**Decision**: Retain the current `entry` model and enhance rendering locally: maintain concise tool call labels, folded tool/shell outputs, queue overflow previews, and add focused tests for malformed args and shell failure styling.
**Evidence**: `reporter.go`, `view.go`, and existing tests already implement summaries, tree-style tool results, `ctrl+o` folding, and queued prompt overflow.
**Rationale**: The confirmed approach is enhanced entry-based rendering, not a full lifecycle state model.
**Trade-off**: The TUI will not show a complete per-tool lifecycle timeline in this phase.
Covers: REQ-010, REQ-011, NFR-001, NFR-004

## Components And Interfaces

### C1: Settings Schema And Merge Support

Add `TUISettings` to `internal/settings`:

```go
type TUISettings struct {
    Theme      string   `json:"theme,omitempty"`
    Statusline []string `json:"statusline,omitempty"`
}
```

Extend `Settings` with `TUI TUISettings`, load `tui` from JSON, and merge `tui.theme` / `tui.statusline` into raw JSON while preserving unknown top-level fields, unknown `tui` fields, and existing `llm` merge behavior.
Covers: REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, NFR-003

### C2: TUI Runtime Configuration

Extend `tui.Config` with display and persistence inputs:

- `HomeDir string`
- `ProviderID string`
- `ProviderProfileID string`
- `ProviderProtocol string`

`internal/app/tui.go` obtains the home directory with `os.UserHomeDir()` and passes resolved provider metadata from `CLIConfig.ResolvedLLM`. The TUI uses empty values as unavailable display placeholders.
Covers: REQ-004, REQ-009, NFR-001, NFR-004

### C3: Theme Registry And Style Application

Create a TUI-local theme registry with palette fields for background, panel, accent, warning, primary text, muted text, dim text, divider, progress empty, selection background, and selection foreground. Rebuild the existing lipgloss style variables from the selected theme and make markdown style generation use the active palette. Clear or key markdown renderer cache by theme and width when the theme changes.
Covers: REQ-001, REQ-008, REQ-010, NFR-004

### C4: Statusline Item Registry

Represent statusline items as ordered declarative names with render functions:

- `model`: current model name
- `project`: project folder name
- `git-branch`: current git branch
- `run-state`: running, compacting/new-session state, or idle/status text
- `plan-mode`: plan mode on/off
- `context-used`: normalized context usage
- `queued`: queued prompt count
- `session-id`: current session id
- `theme`: selected theme name
- `sidebar`: shown/hidden/focused

Normalize settings and commands by trimming whitespace, rejecting unknown items, removing duplicates while preserving order, and falling back to defaults when the saved list is empty or fully invalid.
Covers: REQ-005, REQ-006, REQ-007, REQ-010, NFR-004

### C5: Slash Command Behavior

Update built-in commands:

- Add `/status`, `/theme`, and `/statusline` to `slashCommands`.
- Change `/session` to call the same status overview formatter as `/status`.
- Change `/clear` to call the same new-session path as `/new`.
- Keep `/model`, `/rewind`, `/checkpoint`, `/autodev`, `/sidebar`, file-based commands, ask-user overlays, Plan Mode, and Esc behavior unchanged except for statusline rendering of their state.
- Do not add `/review`, `/vim`, or `/keymap` as built-ins.

Covers: REQ-002, REQ-003, REQ-004, REQ-005, REQ-008, REQ-009, REQ-012, NFR-001

### C6: `/status` Overview Formatter

Add a grouped formatter used by `/status` and `/session`:

- `Session`: session id, session directory, working directory, git branch.
- `Model`: provider/profile display, model, Plan Mode.
- `Runtime`: current run state, queued prompt count, context usage.
- `UI`: theme, enabled statusline items, sidebar visibility.
- `Capabilities`: checkpoint/rewind availability, file slash registry availability, ask_user_question availability.

Unavailable provider/profile fields should be shown as `unavailable`, `inline`, or `disabled` according to the available metadata rather than causing an error.
Covers: REQ-003, REQ-004, REQ-012, NFR-004

### C7: Entry Rendering Alignment

Keep existing entry rendering and add targeted refinements:

- Preserve concise tool call labels and safe fallback summaries.
- Preserve long tool result and shell output folding with `ctrl+o` expansion.
- Ensure shell command failures use a visibly distinct error style based on `entry.err`.
- Preserve queued prompt preview truncation and overflow summary.
- Keep slash suggestions and file mentions foreground-only and width-bounded.

Covers: REQ-010, REQ-011, NFR-004

## Implementation Phases

### Phase 1: Settings Persistence TDD

Write failing tests in `internal/settings/settings_test.go` for loading, saving, and preserving unknown fields under the new nested `tui` object. Implement `TUISettings`, load parsing, raw merge support, and focused helper functions.
Covers: REQ-009, NFR-002, NFR-003

### Phase 2: Status And Command Conflict TDD

Write failing TUI model tests for:

- `/status` grouped overview.
- `/session` producing the same overview.
- `/clear` starting the same new-session command path as `/new`.
- `/review`, `/vim`, and `/keymap` remaining unsupported built-ins.
- Fox Esc, `/model`, `/rewind`, `/checkpoint`, `/autodev`, `/sidebar`, and file-based slash command behavior still passing existing tests.

Implement command registry/dispatcher updates and the status formatter.
Covers: REQ-002, REQ-003, REQ-004, REQ-012, NFR-002, NFR-004

### Phase 3: Theme And Statusline TDD

Write failing TUI/settings tests for:

- Default statusline items are `model`, `project`, `git-branch`, `context-used`, `plan-mode`.
- `run-state` is available but not default-enabled.
- `/statusline set ...` persists ordered valid items and rejects unknown/shell-hook-like input without writing.
- `/statusline default` restores defaults.
- `/theme <name>` applies and persists a built-in theme.
- Invalid theme names do not persist.
- New TUI models restore saved theme/statusline values from `~/.foxharness/settings.json`.

Implement theme registry, style rebuilding, markdown cache invalidation/keying, statusline item registry, command handlers, and persistence error reporting.
Covers: REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, NFR-002, NFR-003, NFR-004

### Phase 4: Rendering Alignment TDD

Write or extend focused view tests for:

- Malformed known tool arguments fall back to safe generic summaries.
- Failed tool and shell outputs are visually distinct.
- Existing tool/shell folding, queue overflow, slash menu, file mention, markdown, footer, and contextual status indicator tests still pass.

Implement only localized view/reporter refinements needed by the failing tests.
Covers: REQ-010, REQ-011, NFR-002, NFR-004

### Phase 5: Verification And Review Loop

Run `gofmt -w` on changed Go files, then `go test ./...`. After implementation, perform code review focused on regressions, missed requirements, persistence safety, and TDD coverage. Verify each finding against the code before fixing. Repeat review/fix/test until the review reports no verified defects.
Covers: NFR-002, NFR-003, NFR-004

## Verification Strategy

- Unit tests:
  - `go test ./internal/settings`
  - `go test ./internal/tui`
- Full suite:
  - `go test ./...`
- Manual inspection through rendered strings where appropriate:
  - Strip ANSI for semantic content checks.
  - Keep ANSI-aware checks only for presentation states that cannot be validated through plain text.
- Review loop:
  - Validate every review finding against repository facts before changing code.
  - Update `spec.md`, `plan.md`, or `tasks.md` if implementation uncovers a verified requirements or design mismatch.

Covers: NFR-002, NFR-003, NFR-004

## Risks And Trade-Offs

- Global lipgloss styles and markdown renderer cache can leak theme state between tests if not reset through `NewModel` or explicit test cleanup. Mitigation: apply the resolved theme during model construction and key or clear markdown cache on theme changes. Covers: REQ-008, REQ-010, NFR-004
- Provider/profile display is metadata-only in this phase. Mitigation: pass resolved config from `internal/app/tui.go` and render explicit unavailable/inline states when fields are absent. Covers: REQ-004, NFR-001
- Text command configuration is less rich than an interactive picker. Mitigation: keep commands declarative, persistent, testable, and compatible with a future picker. Covers: REQ-005, REQ-009, NFR-004
- Existing tests may assert old `/session` or `/clear` semantics. Mitigation: update tests to the confirmed new command conflict resolutions. Covers: REQ-003, NFR-002

## Requirements Coverage

| Requirement | Plan References |
|-------------|-----------------|
| REQ-001 | Fidelity Check, PLD-004, C3 |
| REQ-002 | Existing Repository Constraints, C5, Phase 2 |
| REQ-003 | Fidelity Check, C5, C6, Phase 2 |
| REQ-004 | PLD-001, C2, C6, Phase 2 |
| REQ-005 | PLD-002, PLD-003, C1, C4, C5, Phase 3 |
| REQ-006 | PLD-002, PLD-003, C1, C4, Phase 3 |
| REQ-007 | PLD-002, PLD-003, C1, C4, Phase 3 |
| REQ-008 | PLD-002, PLD-003, PLD-004, C1, C3, C5, Phase 3 |
| REQ-009 | PLD-001, PLD-002, PLD-003, C1, C2, C5, Phase 1, Phase 3 |
| REQ-010 | PLD-004, PLD-005, C3, C4, C7, Phase 3, Phase 4 |
| REQ-011 | PLD-005, C7, Phase 4 |
| REQ-012 | PLD-001, PLD-004, C5, C6, Phase 2 |
| NFR-001 | PLD-001, PLD-003, PLD-005, C2, C5, C6 |
| NFR-002 | Existing Repository Constraints, Phase 1, Phase 2, Phase 3, Phase 4, Phase 5 |
| NFR-003 | PLD-002, C1, Phase 1, Phase 3, Phase 5 |
| NFR-004 | Existing Repository Constraints, PLD-001, PLD-003, PLD-005, C2, C3, C4, C6, C7, Verification Strategy |

## Unresolved Items

None.
