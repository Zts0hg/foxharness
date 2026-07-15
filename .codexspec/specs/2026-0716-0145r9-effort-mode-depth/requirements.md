# Confirmed Requirements: effort-mode-depth

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0716-0145r9`
**Status**: Discovery Complete
**Last Confirmed**: 2026-07-16

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Interactive effort control

- **Status**: confirmed
- **Statement**: foxharness MUST provide a user-facing `/effort` workflow that lets users choose the reasoning effort used for model calls.
- **Rationale**: Users need direct control over reasoning depth, matching the Claude Code user-facing capability while adapting to foxharness provider protocols.
- **User Evidence**: "Replicate Claude Code's /effort feature so users can independently render the thinking depth used during call mode."
- **Confirmed At**: 2026-07-16

### NEED-002: Protocol-specific effort choices

- **Status**: confirmed
- **Statement**: The `/effort` UI MUST show only the effort values valid for the active provider protocol.
- **Rationale**: Protocol-specific options avoid presenting impossible choices and remove the need for cross-protocol incompatibility handling in the normal UI path.
- **User Evidence**: "Use an interactive selector; each protocol only shows its legal values, so the incompatibility case does not exist."
- **Confirmed At**: 2026-07-16

### NEED-003: OpenAI effort values

- **Status**: confirmed
- **Statement**: For the `openai` protocol, foxharness MUST support `auto`, `none`, `minimal`, `low`, `medium`, `high`, and `xhigh`.
- **Rationale**: The OpenAI protocol exposes reasoning effort values beyond the Claude Code visible set.
- **User Evidence**: "Support extended values outside Claude Code, including OpenAI/Anthropic values such as none, minimal, and xhigh."
- **Confirmed At**: 2026-07-16

### NEED-004: Claude effort values

- **Status**: confirmed
- **Statement**: For the `claude` protocol, foxharness MUST support `auto`, `low`, `medium`, `high`, `xhigh`, and `max`.
- **Rationale**: The Claude-compatible Messages API supports Claude-specific `output_config.effort` values, including `max`.
- **User Evidence**: "Support extended values outside Claude Code, including OpenAI/Anthropic values such as none, minimal, and xhigh."
- **Confirmed At**: 2026-07-16

### NEED-005: Persisted effort preferences

- **Status**: confirmed
- **Statement**: foxharness MUST persist effort settings per provider protocol so switching protocols restores the effort value for that protocol.
- **Rationale**: Per-protocol persistence avoids storing invalid cross-protocol values and keeps each provider's effort semantics independent.
- **User Evidence**: User selected "Save per protocol" for persistence.
- **Confirmed At**: 2026-07-16

### NEED-006: CLI effort override

- **Status**: confirmed
- **Statement**: Non-interactive runs MUST support an `-effort <value>` CLI flag that applies to that process or session and is validated against the resolved provider protocol.
- **Rationale**: Scripted and one-shot runs need an explicit non-TUI way to control reasoning effort.
- **User Evidence**: User selected "CLI flag support" for non-TUI usage.
- **Confirmed At**: 2026-07-16

### NEED-007: Prompt command frontmatter effort

- **Status**: confirmed
- **Statement**: The existing slash command frontmatter `effort` field MUST participate in effort resolution for prompt-command-initiated user runs.
- **Rationale**: foxharness already parses `effort` in slash command frontmatter, and command authors need a way to request a specific reasoning depth.
- **User Evidence**: Confirmed scope includes full user surface and Claude Code-style behavior unless unsupported by foxharness core capabilities.
- **Confirmed At**: 2026-07-16

## Constraints

### CON-001: TDD implementation

- **Status**: confirmed
- **Statement**: Implementation MUST follow the project constitution's TDD workflow: write failing tests first, implement the minimum behavior, then refactor while keeping tests green.
- **User Evidence**: Project constitution mandates TDD for all new code.

### CON-002: No spec generation in specify phase

- **Status**: confirmed
- **Statement**: This command records confirmed requirements only and MUST NOT generate `spec.md`.
- **User Evidence**: `$codexspec:specify` requires requirements-only output.

### CON-003: Backend calls excluded from user effort

- **Status**: confirmed
- **Statement**: User-selected effort MUST NOT affect permission reviewer, compaction, automatic memory extraction, configuration probe, or other background provider calls.
- **User Evidence**: User selected "User run only" for the effort scope.

### CON-004: Supported provider protocols

- **Status**: confirmed
- **Statement**: The feature scope is limited to foxharness' existing `openai` and `claude` provider protocols.
- **User Evidence**: User clarified that foxharness currently supports only OpenAI and Claude protocols.

### CON-005: Interactive choices must be vertical

- **Status**: confirmed
- **Statement**: The `/effort` selector MUST present user choices in a vertical layout consistent with the previously improved `/permissions` selector style.
- **User Evidence**: User referenced the prior interactive selector work and requested the same selector approach.

## Decisions

### DEC-001: Use an interactive selector as the primary `/effort` interface

- **Status**: confirmed
- **Decision**: `/effort` with no arguments MUST open a TUI selector instead of requiring users to type an effort value.
- **Alternatives Rejected**: Parameter-only command style.
- **Reason**: The selector can show only valid options for the active protocol and match the interactive style established for `/permissions`.
- **User Evidence**: "It should use an interactive selector."

### DEC-002: Do not support parameter-style effort setting

- **Status**: confirmed
- **Decision**: `/effort <value>` MUST NOT set effort in the first version; setting effort in TUI is done through the selector.
- **Alternatives Rejected**: Keep `/effort <value>` for Claude Code-style direct setting.
- **Reason**: The user chose "selector only" for command arguments.
- **User Evidence**: User selected "Only use selector."

### DEC-003: Persist effort per protocol

- **Status**: confirmed
- **Decision**: Persist OpenAI and Claude effort values separately under user settings.
- **Alternatives Rejected**: One global effort value; session-only effort.
- **Reason**: Protocol-specific persistence avoids incompatible saved values and preserves user preferences when switching protocols.
- **User Evidence**: User selected "Save per protocol."

### DEC-004: Use `auto` as the clear/default state

- **Status**: confirmed
- **Decision**: `auto` means no explicit effort value is sent for that protocol, allowing the model or endpoint default to apply.
- **Alternatives Rejected**: Mapping `auto` to a concrete provider value.
- **Reason**: `auto` should represent default provider behavior and a cleared explicit override.
- **User Evidence**: The confirmed value sets include `auto` for both protocols.

### DEC-005: Effort resolution precedence

- **Status**: confirmed
- **Decision**: Effective user-run effort MUST resolve in this order: prompt command frontmatter `effort` > CLI/session override > persisted protocol effort > `auto`.
- **Alternatives Rejected**: Persisted setting overriding per-command or per-run choices.
- **Reason**: More local and explicit inputs should override broader defaults.
- **User Evidence**: Confirmed scope includes CLI flag support, prompt command frontmatter effort, and persisted protocol preferences.

### DEC-006: Provider request mapping

- **Status**: confirmed
- **Decision**: OpenAI user-run requests MUST send effort through `reasoning_effort`; Claude user-run requests MUST send effort through `output_config.effort`.
- **Alternatives Rejected**: Simulating effort with prompt text or the existing legacy `-thinking` mode.
- **Reason**: The SDKs expose protocol-native request fields for effort/reasoning effort.
- **User Evidence**: User requested control over the "thinking depth used during call mode."

## Out of Scope

### OUT-001: Model picker effort cycling

- **Status**: confirmed
- **Statement**: The first version will not implement Claude Code-style model picker effort cycling.
- **Reason**: foxharness does not currently have the same model picker core capability.
- **User Evidence**: User allowed omissions for features that depend on special system core capabilities foxharness lacks.

### OUT-002: Environment variable override

- **Status**: confirmed
- **Statement**: The first version will not add a `FOXHARNESS_LLM_EFFORT` or similar environment variable override.
- **Reason**: The confirmed non-TUI surface is the `-effort` CLI flag.
- **User Evidence**: User selected "CLI flag support" rather than "CLI + environment variable."

### OUT-003: Legacy thinking mode replacement

- **Status**: confirmed
- **Statement**: The feature will not replace or rename the existing `-thinking` legacy two-phase execution mode.
- **Reason**: Effort controls provider-native reasoning depth, while `-thinking` controls foxharness' existing two-phase planning/action behavior.
- **User Evidence**: Confirmed requirements focus on `/effort` provider call depth, not removal of legacy thinking.

## Open Questions

None.

## Superseded Entries

None.

## Confirmation Log

### Session 2026-07-16

- **Summary Presented**: The final plan specified an interactive `/effort` selector, protocol-specific value sets, per-protocol persistence, CLI `-effort`, prompt frontmatter participation, user-run-only scope, provider-native request fields, TDD verification, and exclusions for model picker cycling and environment variable override.
- **User Confirmation**: "Use `$codexspec:specify` to write the content we confirmed into requirements."
- **Entries Confirmed**: NEED-001, NEED-002, NEED-003, NEED-004, NEED-005, NEED-006, NEED-007, CON-001, CON-002, CON-003, CON-004, CON-005, DEC-001, DEC-002, DEC-003, DEC-004, DEC-005, DEC-006, OUT-001, OUT-002, OUT-003
