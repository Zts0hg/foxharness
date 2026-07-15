# Design Document: Effort Mode Depth

## Context

foxharness already resolves provider protocol through `internal/llmconfig`, constructs protocol-specific providers in `internal/provider`, dispatches user runs through `internal/engine`, and exposes TUI slash commands in `internal/tui`. The TUI already has a vertical `/permissions` form pattern. Slash command frontmatter already parses an `effort` field, but no runtime path resolves, persists, or sends effort to providers.

## Goals / Non-Goals

**Goals:**

- Add a protocol-aware effort rules layer. Covers: REQ-002, REQ-004, REQ-005, REQ-006
- Persist effort preferences per provider protocol. Covers: REQ-003
- Add `-effort` as a validated session override. Covers: REQ-004
- Add a selector-only `/effort` TUI workflow. Covers: REQ-001, REQ-002, REQ-003
- Send explicit effort only for user-run provider calls. Covers: REQ-006, REQ-007

**Non-Goals:**

- Model picker effort cycling.
- Environment variable effort overrides.
- Replacement or renaming of legacy `-thinking`.
- Provider protocols beyond `openai` and `claude`.

## Decisions

### Decision 1: Centralized effort domain helpers

**Context**: CLI validation, settings persistence, TUI options, prompt frontmatter, and provider mapping must agree on the same protocol-specific values.
**Decision**: Add `internal/effort` to define `auto`, supported values, validation, display options, provider-send normalization, and precedence resolution.
**Rationale**: Centralizing rules prevents the selector from accepting values the provider path rejects, and keeps `auto` semantics consistent.

### Decision 2: Persist explicit effort under `llm.effort`

**Context**: Effort is protocol-specific LLM behavior, not a visual TUI preference.
**Decision**: Extend `llmconfig.Settings` with an `Effort map[string]string` persisted as `llm.effort`, keyed by protocol. Store only explicit protocol-valid values; absence means `auto`.
**Rationale**: This matches per-protocol persistence and keeps values close to LLM provider settings while preserving existing settings merge behavior.

### Decision 3: Add call-time generation options while preserving default `Generate`

**Context**: Background provider callers use the same provider implementations and must not inherit user effort.
**Decision**: Keep `LLMProvider.Generate` effort-free and add an optional interface, for example `GenerateWithOptions(ctx, messages, tools, options)`, used only by engine user-run calls. Provider implementations support both paths.
**Rationale**: Existing background callers remain unchanged and therefore cannot receive effort by accident. The engine opts into effort explicitly.

### Decision 4: Resolve session and persisted effort before engine startup

**Context**: CLI/session override and persisted settings are known after provider resolution. Prompt command frontmatter is known later when a command is executed.
**Decision**: Add effort override fields to app/engine configuration. Resolve persisted/session effort for normal user turns at startup, then apply prompt command frontmatter override at prompt-command execution time.
**Rationale**: This preserves the confirmed precedence while keeping prompt-command-specific data scoped to that command run.

### Decision 5: Build `/effort` as a sibling of `/permissions`

**Context**: The user requested the same interactive selector approach and vertical choices.
**Decision**: Add an `effortForm` in `internal/tui` with vertical options from `internal/effort`, route `/effort` to the form, reject `/effort <value>` as selector-only, and save selections through a settings callback.
**Rationale**: Reusing the existing form pattern provides consistent interaction and avoids invalid typed values in normal TUI use.

## Architecture

```
settings llm.effort ─┐
CLI -effort ─────────┼─> internal/effort resolution ─┐
frontmatter effort ──┘                                │
                                                       v
TUI /effort selector ──> settings save          engine user-run call
                                                       │
                                                       v
                                      provider GenerateWithOptions(effort)
                                        │                         │
                                        v                         v
                              OpenAI reasoning_effort   Claude output_config.effort
```

<!-- Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007 -->

## Components

- `internal/effort`: value sets, validation, `auto` omission, and precedence. Covers: REQ-002, REQ-005, REQ-006
- `internal/llmconfig` and `internal/settings`: persisted `llm.effort` map and helpers for setting/clearing protocol values. Covers: REQ-003
- `cmd/fox` and `internal/app`: `-effort` flag parsing, post-resolution validation, and run configuration propagation. Covers: REQ-004, REQ-005
- `internal/provider`: call-time generation options and OpenAI/Claude request field mapping. Covers: REQ-006, REQ-007
- `internal/engine` and slash command execution path: user-run effort propagation and frontmatter override. Covers: REQ-005, REQ-007
- `internal/tui`: `/effort` selector form, slash command routing, save callback, and user feedback. Covers: REQ-001, REQ-002, REQ-003

## Verification Strategy

- Write failing tests before implementation for each code behavior, following the constitution.
- Use table-driven tests for protocol value sets and precedence.
- Use settings tests to verify persistence, unknown-field preservation, and `auto` clearing.
- Use CLI tests to verify valid and invalid `-effort` values after protocol resolution.
- Use provider tests that inspect constructed request parameters without network calls.
- Use engine/fake-provider tests to prove user-run calls receive effort and background callers remain effort-free by default.
- Use TUI model/form tests for option sets, selector-only command behavior, selection, cancellation, and save callback invocation.

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| Provider interface expansion could force many background call changes | User effort might leak into non-user calls | Preserve `Generate` unchanged and add opt-in options interface |
| SDK effort enum support may differ by provider library version | Compile failures or incorrect request fields | Verify through provider request-construction tests and compiler checks |
| Persisting effort under the wrong settings namespace could be hard to migrate | Confusing user settings schema | Store under `llm.effort` because effort is provider behavior |
| Prompt command frontmatter override happens outside normal CLI parsing | Invalid command effort could fail late | Validate frontmatter before model execution and surface a user-facing error |

## Implementation Notes

- Existing `.codexspec/memory/constitution.md` requires TDD for all code changes.
- `auto` is represented as an empty explicit provider value when sending requests.
- The implementation must not add a `FOXHARNESS_LLM_EFFORT` environment variable.
- The implementation must not change the behavior of `-thinking`.

## Requirements Coverage

| Spec Requirement | Plan Coverage |
|------------------|---------------|
| REQ-001 | Decision 5; `internal/tui` component |
| REQ-002 | Decision 1; `internal/effort`; `internal/tui` component |
| REQ-003 | Decision 2; `internal/llmconfig` and `internal/settings` component |
| REQ-004 | Decision 1; Decision 4; `cmd/fox` and `internal/app` component |
| REQ-005 | Decision 1; Decision 4; `internal/engine` component |
| REQ-006 | Decision 1; Decision 3; `internal/provider` component |
| REQ-007 | Decision 3; `internal/provider` and `internal/engine` components |
