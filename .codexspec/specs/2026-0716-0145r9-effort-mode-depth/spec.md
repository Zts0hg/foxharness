# Feature Specification

## ADDED Requirements

### Requirement: REQ-001 Interactive effort selector
foxharness SHALL provide a `/effort` slash command in the TUI that opens an interactive selector for the reasoning effort used by user-initiated model calls. The selector SHALL use the same vertical interaction pattern as the `/permissions` selector.
<!-- Sources: NEED-001, CON-005, DEC-001, DEC-002 -->

#### Scenario: Open selector from slash command

- **WHEN** the user enters `/effort` in the TUI
- **THEN** foxharness opens an effort selector instead of submitting the text as a prompt

#### Scenario: Selector-only command

- **WHEN** the user enters `/effort <value>` in the TUI
- **THEN** foxharness does not set effort from the typed value and informs the user to open `/effort`

### Requirement: REQ-002 Protocol-specific effort choices
foxharness SHALL show only the effort values valid for the active provider protocol.
<!-- Sources: NEED-002, NEED-003, NEED-004, CON-004 -->

#### Scenario: OpenAI options

- **WHEN** the active provider protocol is `openai`
- **THEN** the selector shows `auto`, `none`, `minimal`, `low`, `medium`, `high`, and `xhigh`

#### Scenario: Claude options

- **WHEN** the active provider protocol is `claude`
- **THEN** the selector shows `auto`, `low`, `medium`, `high`, `xhigh`, and `max`

### Requirement: REQ-003 Per-protocol persistence
foxharness SHALL persist user-selected effort independently for each supported provider protocol. Selecting `auto` SHALL clear the explicit value for the active protocol so provider defaults apply.
<!-- Sources: NEED-005, DEC-003, DEC-004 -->

#### Scenario: Restore protocol preference

- **WHEN** the user selects `high` for `openai`, switches to `claude`, selects `max`, and later returns to `openai`
- **THEN** foxharness restores `high` for `openai` and keeps `max` for `claude`

#### Scenario: Clear explicit preference

- **WHEN** the user selects `auto`
- **THEN** foxharness removes the persisted explicit effort value for the active protocol

### Requirement: REQ-004 CLI effort override
foxharness SHALL support an `-effort <value>` CLI flag for non-interactive runs and TUI launches. The value SHALL be validated against the resolved provider protocol before the run begins.
<!-- Sources: NEED-006, CON-004, DEC-005 -->

#### Scenario: Valid CLI effort

- **WHEN** the user runs `fox exec -protocol openai -effort minimal "task"`
- **THEN** foxharness uses `minimal` as the session effort override for user model calls

#### Scenario: Invalid CLI effort

- **WHEN** the user runs `fox exec -protocol claude -effort minimal "task"`
- **THEN** foxharness exits before model execution with an error naming the invalid value and protocol

### Requirement: REQ-005 Effort resolution precedence
foxharness SHALL resolve effective user-run effort in this order: prompt command frontmatter `effort`, CLI/session override, persisted protocol effort, then `auto`.
<!-- Sources: NEED-007, DEC-005 -->

#### Scenario: Frontmatter overrides broader settings

- **WHEN** a prompt command has `effort: high`, the session override is `low`, and the persisted protocol effort is `medium`
- **THEN** the prompt-command-initiated user run uses `high`

#### Scenario: Invalid frontmatter effort

- **WHEN** a prompt command has an `effort` value invalid for the active provider protocol
- **THEN** foxharness rejects the command run before model execution with a validation error

### Requirement: REQ-006 Provider-native request mapping
foxharness SHALL send explicit user-run effort through provider-native request fields. For `openai`, explicit effort SHALL map to `reasoning_effort`. For `claude`, explicit effort SHALL map to `output_config.effort`. When effective effort is `auto`, foxharness SHALL send no explicit effort value.
<!-- Sources: DEC-004, DEC-006 -->

#### Scenario: OpenAI explicit effort

- **WHEN** a user run resolves effort to `xhigh` under the `openai` protocol
- **THEN** the OpenAI-compatible request includes `reasoning_effort: xhigh`

#### Scenario: Claude explicit effort

- **WHEN** a user run resolves effort to `max` under the `claude` protocol
- **THEN** the Claude-compatible request includes `output_config.effort: max`

#### Scenario: Auto effort

- **WHEN** a user run resolves effort to `auto`
- **THEN** no explicit effort field is sent to the provider

### Requirement: REQ-007 Background calls excluded
foxharness SHALL NOT apply user-selected effort to permission reviewer, compaction, automatic memory extraction, configuration probe, or other background provider calls.
<!-- Sources: CON-003 -->

#### Scenario: Background provider call

- **WHEN** a user has selected or overridden effort
- **THEN** background provider calls still execute without user effort options

## Context

foxharness currently supports `openai` and `claude` provider protocols, has an interactive TUI selector pattern for `/permissions`, and already parses slash command frontmatter `effort`. The runtime does not yet expose a user-facing effort workflow or send effort through provider-native request fields.

## Goals

- Add a protocol-aware `/effort` selector for the TUI.
- Persist effort per provider protocol.
- Add a validated `-effort` CLI/session override.
- Apply prompt command frontmatter `effort` to prompt-command-initiated user runs.
- Send explicit effort only for user-run model calls through provider-native request fields.

## Non-Goals

- Do not implement Claude Code-style model picker effort cycling.
- Do not add an environment variable override.
- Do not replace or rename legacy `-thinking` mode.
- Do not add protocols beyond existing `openai` and `claude`.

## User Stories

### Story: TUI user selects effort

**As a** TUI user
**I want** to choose effort from an interactive selector
**So that** I can control reasoning depth without typing provider-specific values

**Acceptance Criteria:**

- [ ] `/effort` opens a vertical selector.
- [ ] The selector shows only active-protocol legal values.
- [ ] The selected value is persisted for that protocol.

### Story: CLI user overrides effort

**As a** CLI user
**I want** to pass `-effort <value>`
**So that** scripted and one-shot runs can control reasoning depth

**Acceptance Criteria:**

- [ ] Valid values are accepted for the resolved protocol.
- [ ] Invalid values fail before model execution.
- [ ] CLI/session override takes precedence over persisted preferences.

### Story: Prompt command author requests effort

**As a** prompt command author
**I want** frontmatter `effort` to participate in effort resolution
**So that** commands can request the reasoning depth they need

**Acceptance Criteria:**

- [ ] Frontmatter `effort` overrides CLI/session and persisted values.
- [ ] Frontmatter values are validated against the active protocol.

## Constraints

- Implementation must follow the project constitution's TDD workflow.
- Supported provider protocols are limited to `openai` and `claude`.
- User-selected effort must not affect background provider calls.
- The selector must use a vertical layout consistent with `/permissions`.

## Assumptions

- `auto` is a foxharness-level clear/default state and is not forwarded to provider SDKs.
- Existing provider SDK versions expose the request fields required by the confirmed provider mapping.

## Requirements Traceability

| Confirmed Requirement | Spec Coverage |
|-----------------------|---------------|
| NEED-001 | REQ-001 |
| NEED-002 | REQ-002 |
| NEED-003 | REQ-002 |
| NEED-004 | REQ-002 |
| NEED-005 | REQ-003 |
| NEED-006 | REQ-004 |
| NEED-007 | REQ-005 |
| CON-001 | Constraints |
| CON-002 | Generation boundary; no product requirement |
| CON-003 | REQ-007 |
| CON-004 | REQ-002, REQ-004 |
| CON-005 | REQ-001 |
| DEC-001 | REQ-001 |
| DEC-002 | REQ-001 |
| DEC-003 | REQ-003 |
| DEC-004 | REQ-003, REQ-006 |
| DEC-005 | REQ-004, REQ-005 |
| DEC-006 | REQ-006 |
| OUT-001 | Non-Goals |
| OUT-002 | Non-Goals |
| OUT-003 | Non-Goals |
