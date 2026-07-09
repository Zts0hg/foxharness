# Confirmed Requirements: tui-interaction-compatibility

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0709-2113sj`
**Status**: Discovery Complete
**Last Confirmed**: 2026-07-09 21:37 CST

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Balanced TUI Compatibility Upgrade

- **Status**: confirmed
- **Statement**: Upgrade the foxharness TUI by aligning overlapping behavior with Codex CLI while retaining foxharness-specific interaction features.
- **Rationale**: The user wants smoother future TUI customization without discarding existing foxharness workflows.
- **User Evidence**: "Check the overlap between Codex CLI and foxharness TUI; overlapping functions follow Codex CLI, foxharness unique functions are retained."
- **Confirmed At**: 2026-07-09 21:24 CST

### NEED-002: Include Claude Code As A Compatibility Reference

- **Status**: confirmed
- **Statement**: Evaluate Claude Code CLI/TUI interaction patterns alongside Codex CLI and use them as supplemental compatibility input where they fit foxharness without core system changes.
- **Rationale**: The user wants this upgrade to balance Codex CLI and Claude Code rather than copying only one tool.
- **User Evidence**: "Balance trade-offs and compatibility between codex-cli and claude code."
- **Confirmed At**: 2026-07-09 21:24 CST

## Constraints

### CON-001: Preserve Foxharness-Specific Semantics

- **Status**: confirmed
- **Statement**: Keep foxharness-specific features and interaction semantics, including `/rewind`, `/checkpoint`, `/autodev`, `/sidebar`, file-based slash commands, ask-user overlays, Plan Mode, provider/profile behavior, and the current fox Esc behavior.
- **User Evidence**: "Foxharness unique functions should be retained, for example `/rewind`; Esc interaction keeps fox semantics."

### CON-002: Avoid Core Capability Changes In This Phase

- **Status**: confirmed
- **Statement**: Prefer TUI/UI-layer improvements that do not require changing the core agent, provider, permission, remote, plugin, MCP, or account systems.
- **User Evidence**: "Full TUI functionality may require core capability changes, so full TUI completeness is unnecessary."

### CON-003: Persist TUI Preferences In Foxharness Settings

- **Status**: confirmed
- **Statement**: Persist `/theme` and `/statusline` configuration in `~/.foxharness/settings.json`.
- **User Evidence**: "`/theme`, `/statusline` configuration persists to `~/.foxharness/settings.json`."

## Decisions

### DEC-001: Codex CLI Is The Primary TUI Baseline

- **Status**: confirmed
- **Decision**: Use Codex CLI as the primary visual and interaction baseline for overlapping TUI behavior; use Claude Code as a supplemental compatibility reference.
- **Alternatives Rejected**: Fully replacing foxharness TUI with a Codex clone; copying Claude Code as the primary baseline.
- **Reason**: This preserves foxharness workflows while standardizing the shared interaction surface around Codex CLI.
- **User Evidence**: "Follow your suggested classification."

### DEC-002: Confirmed Slash Command Conflict Resolutions

- **Status**: confirmed
- **Decision**: Apply these command-level resolutions:
  - `/clear` and `/new` are merged; `/clear` is an alias for `/new`.
  - `/session` becomes an alias of `/status`.
  - Add `/status` as a Codex-style overview.
  - Keep `/model` with the current foxharness interaction semantics.
  - Do not implement `/review` in this phase.
  - Do not implement `/vim` or `/keymap` in this phase.
- **Alternatives Rejected**: Keeping `/clear` as only a visible transcript clear; copying Codex/Claude `/model`; adding `/review`, `/vim`, or `/keymap` now.
- **Reason**: These choices resolve the known conflicts while keeping current foxharness semantics where the user explicitly prefers them.
- **User Evidence**: User listed the conflict resolutions directly, including `/clear` aliasing `/new`, `/session` aliasing `/status`, fox Esc semantics, fox `/model`, and excluding `/review`, `/vim`, `/keymap`.

### DEC-003: Statusline Uses Codex-Style Declarative Items First

- **Status**: confirmed
- **Decision**: Implement `/statusline` first as Codex-style declarative item configuration rather than as a Claude-style shell command hook.
- **Alternatives Rejected**: Supporting a Claude-style statusline shell command hook in the first phase.
- **Reason**: Declarative items fit the Codex baseline and avoid command-hook execution complexity for the initial scope.
- **User Evidence**: "`/statusline` first follows Codex-style declarative items; do not support Claude-style shell command hook for now."

### DEC-004: Overlapping UI Behaviors To Align

- **Status**: confirmed
- **Decision**: Align these overlapping TUI behaviors with the Codex baseline while using Claude Code as supplemental reference:
  - theme/palette selection and persistence;
  - markdown rendering style;
  - tool call summary, collapse, progress, and error presentation;
  - shell command output collapse and truncation presentation;
  - queued prompt preview and overflow summary;
  - slash command menu and argument hints;
  - file mention suggestions;
  - status/footer layout and contextual indicators.
- **Alternatives Rejected**: Treating these as foxharness-only UI and leaving them unchanged.
- **Reason**: These are shared interaction surfaces across foxharness, Codex CLI, and Claude Code.
- **User Evidence**: User accepted the proposed classification of overlapping functions to align.

### DEC-005: Status Overview Field Groups

- **Status**: confirmed
- **Decision**: Implement `/status` as a Codex-style overview with these initial groups:
  - `Session`: session id, session directory, working directory, git branch.
  - `Model`: provider/profile, model, Plan Mode.
  - `Runtime`: current run state, queued prompts, context usage.
  - `UI`: theme, enabled statusline items, sidebar visibility.
  - `Capabilities`: checkpoint/rewind availability, file slash registry availability, ask_user_question availability.
- **Alternatives Rejected**: Keeping `/status` as only session paths; using Claude Code's Settings Status page as the `/status` behavior.
- **Reason**: This gives `/status` a concise overview surface while keeping `/session` as its alias.
- **User Evidence**: User agreed to the proposed `/status` field grouping.

### DEC-006: Statusline Declarative Items

- **Status**: confirmed
- **Decision**: Implement `/statusline` with this initial declarative item set:
  - `model`
  - `project`
  - `git-branch`
  - `run-state`
  - `plan-mode`
  - `context-used`
  - `queued`
  - `session-id`
  - `theme`
  - `sidebar`
- **Default Enabled Items**: `model`, `project`, `git-branch`, `context-used`, `plan-mode`.
- **Alternatives Rejected**: Enabling `run-state` by default.
- **Reason**: The item set covers the useful first-phase statusline fields while keeping the default line compact.
- **User Evidence**: User agreed to the proposed item set and specified that the default enabled set should add `project` and remove `run-state`.

### DEC-007: Theme Scope Is Built-In Themes Only

- **Status**: confirmed
- **Decision**: Implement `/theme` in the first phase using a built-in theme collection only.
- **Alternatives Rejected**: Supporting custom theme files in the first phase.
- **Reason**: Built-in themes provide the required TUI customization while avoiding custom file parsing, validation, and failure recovery scope.
- **User Evidence**: User confirmed: "Do the built-in theme collection."

### DEC-008: Tool Call Rendering Uses Enhanced Entry-Based Model

- **Status**: confirmed
- **Decision**: In the first phase, improve tool call presentation using the existing entry-based TUI rendering model rather than introducing a complete per-tool lifecycle state model.
- **Alternatives Rejected**: Introducing full per-tool lifecycle state in the first phase.
- **Reason**: Enhanced entry-based rendering can deliver Codex/Claude-style summaries, folding, progress, and error presentation with lower implementation risk and without broad core event model changes.
- **User Evidence**: User confirmed: "First use enhanced entry-based."

## Out of Scope

### OUT-001: Claude-Style Statusline Shell Hook

- **Status**: confirmed
- **Statement**: Do not support a statusline shell command hook in the first phase.
- **Reason**: The first phase uses Codex-style declarative statusline items.
- **User Evidence**: "Temporarily no Claude-style shell command hook."

### OUT-002: Review, Vim, And Keymap Commands

- **Status**: confirmed
- **Statement**: `/review`, `/vim`, and `/keymap` are not required in this phase.
- **Reason**: The user explicitly deferred these commands.
- **User Evidence**: "`/review` is not needed for now; `/vim` and `/keymap` are also not needed for now."

### OUT-003: Core-System-Dependent Ecosystem Features

- **Status**: confirmed
- **Statement**: Do not implement Claude/Codex account, remote session, IDE integration, MCP, plugin marketplace, app integration, hooks, usage-limit, background-task, or multi-agent ecosystem features in this phase unless later explicitly re-scoped.
- **Reason**: These features depend on systems outside the TUI layer or conflict with foxharness's current architecture.
- **User Evidence**: Full TUI completeness is unnecessary when it requires core capability changes.

### OUT-004: Custom Theme Files

- **Status**: confirmed
- **Statement**: Do not support custom theme files in the first `/theme` implementation.
- **Reason**: The confirmed first phase uses only built-in themes.
- **User Evidence**: User confirmed built-in themes for the first version.

## Open Questions

No open questions remain for the first-phase specification.

## Superseded Entries

No superseded entries yet.

## Confirmation Log

### Session 2026-07-09 21:24 CST

- **Summary Presented**: Proposed using Codex CLI as the primary visual/interaction baseline, using Claude Code as supplemental compatibility input, preserving foxharness-specific features, aligning overlapping UI behaviors, and deciding whether `/statusline` should support Codex declarative items or Claude shell hooks.
- **User Confirmation**: User accepted the suggested classification and specified that `/statusline` should first use Codex-style declarative item configuration, without Claude-style shell command hook support for now.
- **Entries Confirmed**: NEED-001, NEED-002, CON-001, CON-002, CON-003, DEC-001, DEC-002, DEC-003, DEC-004, OUT-001, OUT-002, OUT-003

### Session 2026-07-09 21:26 CST

- **Summary Presented**: Proposed `/status` groups: Session, Model, Runtime, UI, and Capabilities, with concrete fields under each group.
- **User Confirmation**: User agreed to the proposed `/status` field grouping.
- **Entries Confirmed**: DEC-005

### Session 2026-07-09 21:30 CST

- **Summary Presented**: Proposed `/statusline` declarative items and default enabled items.
- **User Confirmation**: User agreed to the item set and changed the default enabled set to include `project` and exclude `run-state`.
- **Entries Confirmed**: DEC-006

### Session 2026-07-09 21:32 CST

- **Summary Presented**: Proposed limiting `/theme` first phase to built-in themes only and deferring custom theme files.
- **User Confirmation**: User confirmed the built-in theme collection approach.
- **Entries Confirmed**: DEC-007, OUT-004

### Session 2026-07-09 21:37 CST

- **Summary Presented**: Proposed improving tool call display through enhanced entry-based rendering in the first phase rather than introducing a full per-tool lifecycle state model.
- **User Confirmation**: User confirmed the enhanced entry-based approach.
- **Entries Confirmed**: DEC-008
