# Feature Specification: TUI Interaction Compatibility

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Feature ID**: `2026-0709-2113sj`
**Feature Branch**: `main-latest-20260709`
**Created**: 2026-07-09
**Status**: Draft
**Input**: `.codexspec/specs/2026-0709-2113sj-tui-interaction-compatibility/requirements.md`

## Context

foxharness already has an interactive TUI with fox-specific workflows such as checkpoint rewind, autodev, a sidebar, file-based slash commands, Plan Mode, provider/profile configuration, and an ask-user overlay. This feature upgrades the overlapping TUI interaction surface by using Codex CLI as the primary visual and interaction baseline while using Claude Code as a supplemental compatibility reference where it fits the current foxharness architecture.

The first phase intentionally avoids features that require changes to the core agent, provider, permission, remote, plugin, MCP, account, or background-task systems.

## Goals

- Align shared TUI behavior with the Codex CLI baseline while preserving foxharness-specific semantics.
- Add first-phase `/status`, `/statusline`, and `/theme` behavior with persisted TUI preferences.
- Improve markdown, tool call, shell output, queue, slash menu, file mention, footer, and status presentation without broad core-system changes.
- Keep the first phase implementable through TUI-layer changes and focused runtime adapters.

## User Scenarios & Testing

### User Story 1 - Use A Codex-Aligned TUI Without Losing Fox Workflows (Priority: P1)

As a foxharness TUI user, I want overlapping UI behavior to feel closer to Codex CLI while foxharness-specific workflows keep working, so that future TUI customization starts from a consistent baseline without removing current productivity features.

**Why this priority**: This is the primary objective of the feature.

**Independent Test**: Start the TUI and verify the updated shared presentation while exercising `/rewind`, `/checkpoint`, `/autodev`, `/sidebar`, file-based slash commands, Plan Mode, current `/model` semantics, and fox Esc behavior.

**Acceptance Scenarios**:

1. **Given** the upgraded TUI is running, **When** the user views standard transcript, footer, slash menu, file mention, markdown, queue, and status areas, **Then** the presentation follows the Codex-oriented baseline for overlapping behavior.
2. **Given** the upgraded TUI is running, **When** the user invokes fox-specific features, **Then** those features remain available with their existing fox semantics.
3. **Given** the user presses Esc while idle or while a run is active, **When** foxharness's existing Esc conditions apply, **Then** the foxharness Esc behavior is preserved.

### User Story 2 - Inspect Session State With `/status` (Priority: P1)

As a TUI user, I want `/status` to show a concise Codex-style overview, so that I can inspect the session, model, runtime, UI, and available capabilities without opening separate commands.

**Why this priority**: `/status` becomes the canonical overview command and `/session` aliases to it.

**Independent Test**: Run `/status` and `/session` in the TUI and compare that both display the same grouped overview.

**Acceptance Scenarios**:

1. **Given** the TUI is idle, **When** the user runs `/status`, **Then** the overview includes `Session`, `Model`, `Runtime`, `UI`, and `Capabilities` groups.
2. **Given** the TUI is idle, **When** the user runs `/session`, **Then** it behaves as an alias for `/status`.
3. **Given** a field is unavailable, **When** `/status` renders, **Then** it displays an explicit unavailable/disabled state or omits only fields whose absence is expected by the spec.

### User Story 3 - Configure Persistent TUI Appearance (Priority: P1)

As a TUI user, I want `/theme` and `/statusline` settings to persist, so that my preferred TUI appearance survives restarts.

**Why this priority**: Persistent TUI customization is a confirmed first-phase behavior.

**Independent Test**: Select a built-in theme and statusline item configuration, restart the TUI, and verify both choices are restored from `~/.foxharness/settings.json`.

**Acceptance Scenarios**:

1. **Given** the user selects a built-in theme, **When** the selection is confirmed, **Then** the theme is persisted to `~/.foxharness/settings.json` and applied in later TUI sessions.
2. **Given** the user configures `/statusline`, **When** the selection is confirmed, **Then** the declarative item list is persisted to `~/.foxharness/settings.json` and used in later TUI sessions.
3. **Given** the user starts a theme or statusline configuration but does not confirm a valid selection, **When** they return to the main TUI, **Then** no persisted configuration change is made.

### User Story 4 - Read Tool Activity Efficiently (Priority: P2)

As a TUI user, I want tool calls, shell output, and queued prompts to be summarized and folded consistently, so that I can scan long runs without losing access to details.

**Why this priority**: Tool activity presentation is a major overlap area with Codex CLI and Claude Code, but the first phase must avoid a broad lifecycle model rewrite.

**Independent Test**: Run prompts that trigger bash, read, write, edit, todo, delegated task, queued prompt, successful tool result, and failed tool result entries, then verify summaries, collapse behavior, and error presentation.

**Acceptance Scenarios**:

1. **Given** a tool call is recorded, **When** the transcript renders, **Then** the entry uses a concise, user-readable summary for known tools and a safe fallback for unknown tools.
2. **Given** a tool result or shell output is long, **When** it renders in collapsed mode, **Then** only the configured summary/preview is shown with a visible indication that more content exists.
3. **Given** a tool or shell command fails, **When** the entry renders, **Then** the failure state is visually distinct from successful output.
4. **Given** messages are queued while a run is active, **When** the queue preview renders, **Then** it shows a compact preview and summarizes overflow rather than expanding indefinitely.

## Requirements

### Functional Requirements

- **REQ-001**: The TUI MUST use Codex CLI as the primary baseline for overlapping visual and interaction behavior while using Claude Code only as a supplemental compatibility reference.
  - Sources: NEED-001, NEED-002, DEC-001

- **REQ-002**: The TUI MUST preserve foxharness-specific features and semantics, including `/rewind`, `/checkpoint`, `/autodev`, `/sidebar`, file-based slash commands, ask-user overlays, Plan Mode, provider/profile behavior, current `/model` semantics, and current fox Esc behavior.
  - Sources: CON-001, DEC-002

- **REQ-003**: The slash command registry and dispatcher MUST apply the confirmed command conflict resolutions: `/clear` aliases `/new`, `/session` aliases `/status`, `/status` is added as the canonical overview, `/model` keeps current fox semantics, and `/review`, `/vim`, and `/keymap` are not first-phase commands.
  - Sources: DEC-002, OUT-002

- **REQ-004**: `/status` MUST render a Codex-style overview with these groups and fields:
  - `Session`: session id, session directory, working directory, git branch.
  - `Model`: provider/profile, model, Plan Mode.
  - `Runtime`: current run state, queued prompts, context usage.
  - `UI`: theme, enabled statusline items, sidebar visibility.
  - `Capabilities`: checkpoint/rewind availability, file slash registry availability, ask_user_question availability.
  - Sources: DEC-002, DEC-005

- **REQ-005**: `/statusline` MUST use Codex-style declarative item configuration and MUST NOT use a Claude-style shell command hook in the first phase.
  - Sources: DEC-003, DEC-006, OUT-001

- **REQ-006**: `/statusline` MUST support the initial item set `model`, `project`, `git-branch`, `run-state`, `plan-mode`, `context-used`, `queued`, `session-id`, `theme`, and `sidebar`.
  - Sources: DEC-006

- **REQ-007**: The default enabled `/statusline` items MUST be `model`, `project`, `git-branch`, `context-used`, and `plan-mode`; `run-state` MUST be available but not enabled by default.
  - Sources: DEC-006

- **REQ-008**: `/theme` MUST provide a built-in theme collection in the first phase and MUST NOT support custom theme files in the first implementation.
  - Sources: DEC-004, DEC-007, OUT-004

- **REQ-009**: `/theme` and `/statusline` preferences MUST persist to `~/.foxharness/settings.json`.
  - Sources: CON-003, DEC-003, DEC-006, DEC-007

- **REQ-010**: The TUI MUST align overlapping rendering behavior with the Codex baseline, including theme/palette application, markdown style, tool call summary/collapse/error presentation, shell output collapse/truncation, queued prompt preview, slash command menu hints, file mention suggestions, footer layout, and contextual status indicators.
  - Sources: DEC-004

- **REQ-011**: Tool call presentation MUST be implemented in the first phase by enhancing the existing entry-based TUI rendering model rather than introducing a complete per-tool lifecycle state model.
  - Sources: DEC-004, DEC-008

- **REQ-012**: First-phase implementation MUST exclude Claude/Codex account, remote session, IDE integration, MCP, plugin marketplace, app integration, hooks, usage-limit, background-task, and multi-agent ecosystem features unless later explicitly re-scoped.
  - Sources: CON-002, OUT-003

### Non-Functional Requirements

- **NFR-001**: Implementation SHOULD prefer TUI/UI-layer changes that do not require modifying core agent, provider, permission, remote, plugin, MCP, or account systems.
  - Sources: CON-002, DEC-008

- **NFR-002**: New code MUST follow the project constitution's TDD workflow, including failing tests before implementation, minimal passing implementation, and refactoring with tests still passing.
  - Sources: Project Constitution 2.0.0, Core Principle 1

- **NFR-003**: Settings persistence MUST preserve existing and unknown settings fields where feasible so unrelated user configuration is not dropped when writing TUI preferences.
  - Sources: CON-003

- **NFR-004**: UI behavior MUST remain testable through focused TUI model/view tests without requiring real model calls for normal acceptance coverage.
  - Sources: CON-002, DEC-008

### Key Entities

- **TUI Settings**: User-facing TUI preferences persisted in `~/.foxharness/settings.json`, including the selected built-in theme and declarative statusline configuration.
- **Statusline Item**: A named declarative field that may be enabled, disabled, ordered, and rendered in the TUI statusline.
- **Built-In Theme**: A shipped palette/style preset selectable through `/theme`.
- **Status Overview**: The grouped `/status` output containing session, model, runtime, UI, and capability information.
- **Transcript Entry**: The existing TUI rendering unit used for user messages, assistant messages, command output, tool calls, and tool results.

## Acceptance Criteria And Expected Error Behavior

- `/clear` and `/new` start a fresh session through the same behavior path; `/clear` must not remain a visible-transcript-only clear command.
- `/session` and `/status` produce the same status overview.
- Running `/review`, `/vim`, or `/keymap` as built-ins is not required in this phase; if entered, they must not silently perform unsupported behavior.
- `/statusline` rejects or ignores shell-hook configuration in first-phase UI flows and must not execute a user-provided statusline command.
- `/theme` exposes only built-in choices; custom theme files are not searched, parsed, or loaded in the first phase.
- If `~/.foxharness/settings.json` cannot be written, the TUI must show a clear error and keep the in-memory session usable.
- If a statusline item cannot produce a value, the statusline must remain renderable and either omit that item or show an appropriate unavailable placeholder.
- If an enhanced tool summary cannot parse known tool arguments, it must fall back to a generic, safe summary rather than hiding the entry.

## Out of Scope

- Claude-style statusline shell command hooks.
  - Sources: OUT-001
- `/review`, `/vim`, and `/keymap` first-phase command implementations.
  - Sources: OUT-002
- Claude/Codex account, remote session, IDE integration, MCP, plugin marketplace, app integration, hooks, usage-limit, background-task, and multi-agent ecosystem features.
  - Sources: OUT-003
- Custom theme files.
  - Sources: OUT-004
- Complete per-tool lifecycle state modeling in the first phase.
  - Sources: DEC-008

## Assumptions

- The first phase may add small TUI-facing adapters or view models when needed, but those additions must remain subordinate to the confirmed constraint of avoiding broad core-system changes.
- The exact visual palette names for built-in themes are implementation details as long as the first-phase `/theme` behavior is built-in-only and persistent.

## Dependencies

- Existing foxharness TUI model/view/reporter code.
- Existing `internal/settings` persistence path for `~/.foxharness/settings.json`.
- Existing session, model, git branch, context usage, sidebar, checkpointer, slash registry, and ask-user availability signals.
- Existing Go test infrastructure and the project constitution's TDD requirements.

## Open Questions

No open questions remain for the first-phase specification.

## Requirements Traceability

| Confirmed Entry | Spec Coverage | Notes |
|-----------------|---------------|-------|
| NEED-001 | REQ-001, REQ-010, User Story 1 | Primary upgrade goal covered. |
| NEED-002 | REQ-001 | Claude Code is supplemental reference only. |
| CON-001 | REQ-002 | Foxharness-specific behavior preserved. |
| CON-002 | REQ-012, NFR-001, NFR-004 | First phase avoids broad core changes. |
| CON-003 | REQ-009, NFR-003 | TUI preferences persist to foxharness settings. |
| DEC-001 | REQ-001 | Codex is the primary baseline. |
| DEC-002 | REQ-002, REQ-003, REQ-004 | Slash conflict resolutions covered. |
| DEC-003 | REQ-005, REQ-009 | Declarative statusline and persistence covered. |
| DEC-004 | REQ-008, REQ-010, REQ-011 | Overlapping UI behaviors covered. |
| DEC-005 | REQ-004 | `/status` groups and fields covered. |
| DEC-006 | REQ-006, REQ-007, REQ-009 | Statusline item set and defaults covered. |
| DEC-007 | REQ-008, REQ-009 | Built-in theme scope covered. |
| DEC-008 | REQ-011, NFR-001, NFR-004 | Entry-based tool rendering approach covered. |
| OUT-001 | REQ-005, Out of Scope | Claude-style shell hook excluded. |
| OUT-002 | REQ-003, Out of Scope | Review, Vim, and keymap excluded. |
| OUT-003 | REQ-012, Out of Scope | Ecosystem features excluded. |
| OUT-004 | REQ-008, Out of Scope | Custom theme files excluded. |
