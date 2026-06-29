# Implementation Tasks: Configurable LLM API Providers

**Feature**: `.codexspec/specs/2026-0628-1441vu-llm-api-providers`
**Plan**: `.codexspec/specs/2026-0628-1441vu-llm-api-providers/plan.md`
**Date**: 2026-06-28
**Status**: Complete

## Execution Notes

- Follow the project constitution's TDD workflow for all Go code tasks: write the failing test first, implement the smallest passing change, then refactor while keeping tests green.
- Use `gofmt -w` for changed Go files.
- Run `go test ./...` before considering implementation complete.
- Do not edit files under `vendor/`.
- Keep API key values out of logs, normal error output, and project-local files.

## Phase 1: Resolution Tests and `internal/llmconfig`

### [x] T001 - Add LLM config resolution tests

- **Outcome**: Failing tests define provider profile selection priority, field override priority, inline CLI/env config, missing config errors, unsupported protocol errors, `api-key` credential requirements, `auth: "none"` behavior, and redacted errors.
- **Paths**: `internal/llmconfig/config_test.go`
- **Dependencies**: None
- **Traceability**: Covers: REQ-002, REQ-003, REQ-004, REQ-007, REQ-008, REQ-010, REQ-013, NFR-001, NFR-002; Plan: Phase 1, Component `internal/llmconfig`

### [x] T002 - Implement `internal/llmconfig`

- **Outcome**: `internal/llmconfig` exposes schema types, foxharness-scoped env override loading, deterministic config resolution, validation, API key source resolution, and redacted error messages that satisfy T001.
- **Paths**: `internal/llmconfig/config.go`, `internal/llmconfig/doc.go`, `internal/llmconfig/config_test.go`
- **Dependencies**: T001
- **Traceability**: Covers: REQ-002, REQ-003, REQ-004, REQ-007, REQ-008, REQ-010, REQ-013, NFR-001, NFR-002; Plan: Phase 1, Component `internal/llmconfig`, PLD-001, PLD-002, PLD-004

### [x] T003 - Verify `internal/llmconfig`

- **Outcome**: `go test ./internal/llmconfig` passes.
- **Paths**: `internal/llmconfig`
- **Dependencies**: T002
- **Traceability**: Covers: REQ-002, REQ-003, REQ-004, REQ-007, REQ-008, REQ-010, REQ-013, NFR-001, NFR-002; Plan: Phase 1, Validation Strategy

## Phase 2: Settings Schema and Persistence

### [x] T004 - Add settings schema and persistence tests

- **Outcome**: Failing tests cover loading `llm.default_provider`, loading `llm.providers`, preserving unrelated top-level settings fields, preserving unknown fields inside existing `llm.providers` entries during profile model updates, updating the selected provider profile model, and avoiding any built-in provider default for missing or malformed settings.
- **Paths**: `internal/settings/settings_test.go`
- **Dependencies**: T002
- **Traceability**: Covers: REQ-003, REQ-008, REQ-012, NFR-004; Plan: Phase 2, Component `internal/settings`

### [x] T005 - Implement settings LLM schema and update helpers

- **Outcome**: `internal/settings` stores `LLM llmconfig.Settings`, preserves existing top-level settings data and unknown provider-profile fields on save/update, provides LLM profile update helpers, and leaves legacy top-level `model` outside the new LLM resolution contract.
- **Paths**: `internal/settings/settings.go`, `internal/settings/doc.go`, `internal/settings/settings_test.go`
- **Dependencies**: T004
- **Traceability**: Covers: REQ-003, REQ-008, REQ-012, NFR-004; Plan: Phase 2, Component `internal/settings`

### [x] T006 - Verify settings package

- **Outcome**: `go test ./internal/settings` passes.
- **Paths**: `internal/settings`
- **Dependencies**: T005
- **Traceability**: Covers: REQ-003, REQ-008, REQ-012, NFR-004; Plan: Phase 2, Validation Strategy

## Phase 3: Generic Provider Factory

### [x] T007 - Add generic provider factory tests

- **Outcome**: Failing tests cover constructing OpenAI and Claude providers from resolved protocol and connection fields, unsupported protocol errors, and factory behavior that does not read `ZHIPU_API_KEY` as an implicit credential source.
- **Paths**: `internal/provider/factory_test.go`
- **Dependencies**: T002
- **Traceability**: Covers: REQ-001, REQ-002, REQ-008, REQ-011, REQ-013, NFR-002, NFR-003; Plan: Phase 3, Component `internal/provider`, PLD-003

### [x] T008 - Add OpenAI provider request/auth tests

- **Outcome**: Failing tests prove OpenAI-compatible requests use the configured base URL, model id, API key auth header when `auth: "api-key"` is resolved, and no `Authorization` or `X-Api-Key` header when `auth: "none"` is resolved even if common SDK key env vars are set.
- **Paths**: `internal/provider/openai_test.go`
- **Dependencies**: T002
- **Traceability**: Covers: REQ-001, REQ-002, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002, NFR-003; Plan: Phase 3, Component `internal/provider`, Risk: SDK Default Environment Credentials Leak Into `auth: "none"`

### [x] T009 - Add Claude provider request/auth tests

- **Outcome**: Failing tests prove Claude-compatible requests use the configured base URL, model id, API key auth header when `auth: "api-key"` is resolved, and no `Authorization` or `X-Api-Key` header when `auth: "none"` is resolved even if common SDK key env vars are set.
- **Paths**: `internal/provider/claude_test.go`
- **Dependencies**: T002
- **Traceability**: Covers: REQ-001, REQ-002, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002, NFR-003; Plan: Phase 3, Component `internal/provider`, Risk: SDK Default Environment Credentials Leak Into `auth: "none"`

### [x] T010 - Implement generic provider factory and constructors

- **Outcome**: Provider construction uses resolved config for OpenAI and Claude protocols, applies `api-key` and `none` auth modes correctly, and removes production use of `NewZhipuProvider`, `NewZhipuOpenAIProvider`, `NewZhipuClaudeProvider`, and `zhipuAPIKeyFromEnv`.
- **Paths**: `internal/provider/factory.go`, `internal/provider/openai.go`, `internal/provider/claude.go`, `internal/provider/retry.go`, `internal/provider/factory_test.go`, `internal/provider/openai_test.go`, `internal/provider/claude_test.go`
- **Dependencies**: T007, T008, T009
- **Traceability**: Covers: REQ-001, REQ-002, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002, NFR-003; Plan: Phase 3, Component `internal/provider`, PLD-003, PLD-004

### [x] T011 - Verify provider package

- **Outcome**: `go test ./internal/provider` passes.
- **Paths**: `internal/provider`
- **Dependencies**: T010
- **Traceability**: Covers: REQ-001, REQ-002, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002, NFR-003; Plan: Phase 3, Validation Strategy

## Phase 4: Main CLI Parsing and Resolution Wiring

### [x] T012 - Add main CLI flag parsing tests

- **Outcome**: Failing tests cover `-llm-provider`, `-protocol`, `-base-url`, `-auth`, `-api-key-env`, `-api-key`, continued `-model` model override, standard unknown-flag behavior for `-provider` / `--provider` in flag position, and preservation of `-provider` inside positional prompt text.
- **Paths**: `cmd/fox/main_test.go`
- **Dependencies**: T002
- **Traceability**: Covers: REQ-004, REQ-005, REQ-006, REQ-007, REQ-010, REQ-013; Plan: Phase 4, Component `cmd/fox`, PLD-005

### [x] T013 - Implement main CLI flag parsing

- **Outcome**: `cmd/fox` parses the new LLM flags into app CLI config, does not register or pre-scan old `-provider`, and no longer assigns default protocol/model values during parsing.
- **Paths**: `cmd/fox/main.go`, `cmd/fox/main_test.go`, `internal/app/cli.go`
- **Dependencies**: T012
- **Traceability**: Covers: REQ-004, REQ-005, REQ-006, REQ-007, REQ-010, REQ-013; Plan: Phase 4, Component `cmd/fox`, PLD-005

### [x] T014 - Add main startup resolution tests

- **Outcome**: Failing tests cover resolving effective LLM config from CLI/env/settings in `cmd/fox`, surfacing missing or invalid config errors, and not using `FOX_MODEL`, `ZHIPU_API_KEY`, or `glm-4.5-air` as fallback.
- **Paths**: `cmd/fox/main_test.go`
- **Dependencies**: T005, T013
- **Traceability**: Covers: REQ-002, REQ-004, REQ-007, REQ-008, REQ-010, REQ-013, NFR-001, NFR-002; Plan: Phase 4, Component `cmd/fox`, PLD-002

## Phase 5: App and Autodev Wiring

### [x] T015 - Add app runner resolved-config tests

- **Outcome**: Failing tests cover `AgentRunnerConfig` carrying resolved LLM config, `NewAgentRunner` using the generic provider factory, and startup errors no longer mentioning Zhipu or `ZHIPU_API_KEY`.
- **Paths**: `internal/app/runner_test.go`
- **Dependencies**: T010
- **Traceability**: Covers: REQ-003, REQ-004, REQ-008, REQ-010, REQ-011, NFR-002, NFR-003; Plan: Phase 5, Component `internal/app`

### [x] T016 - Add `SetModel` provider rebuild tests

- **Outcome**: Failing tests prove `AgentRunner.SetModel` updates only the model while preserving selected provider id, protocol, base URL, and auth settings, then rebuilds the provider through the generic factory.
- **Paths**: `internal/app/runner_test.go`
- **Dependencies**: T015
- **Traceability**: Covers: REQ-003, REQ-010, REQ-011, REQ-012, NFR-002, NFR-003; Plan: Phase 5, Component `internal/app`, PLD-006

### [x] T017 - Add TUI model persistence tests

- **Outcome**: Failing tests cover persisting `/model` changes to `llm.providers[active_profile].model` when a settings-backed profile is active, and keeping inline CLI/env model changes in memory when no profile id is available.
- **Paths**: `internal/app/tui_test.go`, `internal/tui/model_test.go`
- **Dependencies**: T005, T013
- **Traceability**: Covers: REQ-003, REQ-010, REQ-012, NFR-002; Plan: Phase 5, Component `internal/app`, PLD-006, Risk: Existing `/model` Persistence Writes the Wrong Location

### [x] T018 - Add autodev resolved-config tests

- **Outcome**: Failing tests cover autodev building providers from resolved LLM config and applying `.foxharness/autodev.yml` model overrides by copying the resolved config with a different model instead of reconstructing a Zhipu provider.
- **Paths**: `internal/app/autodev_test.go`
- **Dependencies**: T010
- **Traceability**: Covers: REQ-003, REQ-004, REQ-008, REQ-010, REQ-011, NFR-002, NFR-003; Plan: Phase 5, Component `internal/app`

### [x] T019 - Implement app runner resolved-config wiring

- **Outcome**: `AgentRunnerConfig`, `NewAgentRunner`, runtime metadata, and `SetModel` carry and update resolved LLM config while constructing providers only through the generic factory.
- **Paths**: `internal/app/runner.go`, `internal/app/runner_test.go`, `internal/engine/config.go`, `internal/engine/reporter_test.go`
- **Dependencies**: T015, T016
- **Traceability**: Covers: REQ-003, REQ-004, REQ-008, REQ-010, REQ-011, REQ-012, NFR-002, NFR-003; Plan: Phase 5, Component `internal/app`

### [x] T020 - Implement TUI model persistence wiring

- **Outcome**: TUI startup and model-change callbacks persist model changes to the active settings-backed provider profile when available, keep inline-only provider changes in memory, and avoid writing obsolete top-level model values as effective LLM config.
- **Paths**: `internal/app/tui.go`, `internal/app/tui_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `cmd/fox/main.go`
- **Dependencies**: T017, T019
- **Traceability**: Covers: REQ-003, REQ-010, REQ-012, NFR-002; Plan: Phase 5, Component `internal/app`, PLD-006

### [x] T021 - Implement autodev resolved-config wiring

- **Outcome**: Autodev dependency construction and app-core runner factory use resolved LLM config and apply autodev model overrides without Zhipu-specific reconstruction.
- **Paths**: `internal/app/autodev.go`, `internal/app/autodev_test.go`
- **Dependencies**: T018, T019
- **Traceability**: Covers: REQ-003, REQ-004, REQ-008, REQ-010, REQ-011, NFR-002, NFR-003; Plan: Phase 5, Component `internal/app`

### [x] T022 - Implement main startup resolution wiring

- **Outcome**: `cmd/fox` loads settings, resolves LLM config via `internal/llmconfig`, passes resolved config into app startup, and surfaces actionable missing/invalid config errors.
- **Paths**: `cmd/fox/main.go`, `cmd/fox/main_test.go`, `internal/app/cli.go`
- **Dependencies**: T014, T019, T020
- **Traceability**: Covers: REQ-002, REQ-004, REQ-007, REQ-008, REQ-010, REQ-013, NFR-001, NFR-002; Plan: Phase 4, Component `cmd/fox`, Data Flow

### [x] T023 - Verify main CLI and app packages

- **Outcome**: `go test ./cmd/fox ./internal/app` passes.
- **Paths**: `cmd/fox`, `internal/app`
- **Dependencies**: T021, T022
- **Traceability**: Covers: REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-010, REQ-011, REQ-012, REQ-013, NFR-001, NFR-002, NFR-003; Plan: Phase 4, Phase 5, Validation Strategy

## Phase 6: Secondary Commands

### [x] T024 - Add secondary command startup tests

- **Outcome**: Failing tests cover `cmd/agentops`, `cmd/feishu`, and `cmd/bench` resolving LLM config from settings plus `FOXHARNESS_LLM_*`, producing clear missing-configuration errors, and not requiring `ZHIPU_API_KEY` or `glm-4.5-air`.
- **Paths**: `cmd/agentops/main_test.go`, `cmd/feishu/main_test.go`, `cmd/bench/main_test.go`
- **Dependencies**: T005, T010
- **Traceability**: Covers: REQ-001, REQ-002, REQ-004, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002; Plan: Phase 6, Component Secondary Commands

### [x] T025 - Implement secondary command LLM resolution

- **Outcome**: `cmd/agentops`, `cmd/feishu`, and `cmd/bench` use shared settings/env resolution and generic provider construction, with Zhipu-specific comments, key checks, and model defaults removed from production command paths.
- **Paths**: `cmd/agentops/main.go`, `cmd/agentops/main_test.go`, `cmd/feishu/main.go`, `cmd/feishu/main_test.go`, `cmd/bench/main.go`, `cmd/bench/main_test.go`
- **Dependencies**: T024
- **Traceability**: Covers: REQ-001, REQ-002, REQ-004, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002; Plan: Phase 6, Component Secondary Commands

### [x] T026 - Verify secondary commands

- **Outcome**: `go test ./cmd/agentops ./cmd/feishu ./cmd/bench` passes.
- **Paths**: `cmd/agentops`, `cmd/feishu`, `cmd/bench`
- **Dependencies**: T025
- **Traceability**: Covers: REQ-001, REQ-002, REQ-004, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002; Plan: Phase 6, Validation Strategy

## Phase 7: Documentation and Examples

### [x] T027 - Update provider configuration documentation

- **Outcome**: README files document the new `llm.default_provider` / `llm.providers` schema, config priority, `-llm-provider`, `-protocol`, absence of a `-provider` flag, Zhipu as an explicit ordinary profile, local `auth: "none"` profile, and `api_key_env` security guidance.
- **Paths**: `README.md`, `README.zh-CN.md`, `README.zh-TW.md`, `README.ja.md`
- **Dependencies**: T013, T022
- **Traceability**: Covers: REQ-003, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-013, NFR-001; Plan: Phase 7, Component Documentation

### [x] T028 - Verify documentation and hardcoded-default removal

- **Outcome**: Repository search confirms non-test production code no longer uses `NewZhipuProvider`, `NewZhipuOpenAIProvider`, `NewZhipuClaudeProvider`, `zhipuAPIKeyFromEnv`, `FOX_MODEL`, or `glm-4.5-air` as an LLM fallback; tests mention old names only as negative cases; docs mention Zhipu only as an explicit ordinary example profile.
- **Paths**: `cmd`, `internal`, `README.md`, `README.zh-CN.md`, `README.zh-TW.md`, `README.ja.md`
- **Dependencies**: T010, T022, T025, T027
- **Traceability**: Covers: REQ-008, REQ-009, REQ-011, NFR-001, NFR-003; Plan: Phase 3, Phase 4, Phase 6, Phase 7

## Final Verification

### [x] T029 - Format changed Go files

- **Outcome**: `gofmt -w` has been run on all changed Go files.
- **Paths**: `cmd`, `internal`
- **Dependencies**: T002, T005, T010, T013, T019, T020, T021, T022, T025
- **Traceability**: Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-010, REQ-011, REQ-012, REQ-013, NFR-002, NFR-003, NFR-004; Plan: Validation Strategy, Constitution

### [x] T030 - Run full test suite

- **Outcome**: `go test ./...` passes.
- **Paths**: repository root
- **Dependencies**: T003, T006, T011, T023, T026, T028, T029
- **Traceability**: Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, NFR-001, NFR-002, NFR-003, NFR-004; Plan: Final Verification, Constitution

## Dependency Summary

- Core config resolution must be implemented first: T001 -> T002 -> T003.
- Settings persistence depends on `internal/llmconfig`: T004 -> T005 -> T006.
- Provider factory work depends on resolved config types: T007, T008, T009 -> T010 -> T011.
- CLI parsing can start after `internal/llmconfig`: T012 -> T013.
- CLI startup resolution depends on settings and app wiring: T014 -> T022, with T022 also depending on T019 and T020.
- App wiring depends on generic provider construction: T015, T016, T018 -> T019/T021.
- TUI persistence depends on settings helpers and app runner config: T017 -> T020.
- Secondary command work depends on settings and provider factory: T024 -> T025 -> T026.
- Docs depend on finalized CLI/startup behavior: T027 -> T028.
- Final verification runs after all implementation and documentation tasks: T029 -> T030.

## Parallelization Notes

After T002 is complete, these test-design tasks can proceed independently if separate developers coordinate on shared files:

- T004 (`internal/settings`)
- T007, T008, T009 (`internal/provider`)
- T012 (`cmd/fox` parsing)

After T010 is complete, app and secondary-command test tasks can proceed in parallel if they avoid editing the same files:

- T015/T016 (`internal/app/runner_test.go`, sequential with each other)
- T018 (`internal/app/autodev_test.go`)
- T024 (`cmd/agentops`, `cmd/feishu`, `cmd/bench`)

No task is marked `[P]` because the implementation will likely be performed by one agent in a shared working tree, and several tasks touch overlapping test and wiring files.

## Coverage Table

| Requirement / Plan Item | Task References |
| --- | --- |
| REQ-001 | T007, T008, T009, T010, T011, T024, T025, T026, T030 |
| REQ-002 | T001, T002, T003, T007, T008, T009, T010, T011, T014, T022, T023, T024, T025, T026, T030 |
| REQ-003 | T001, T002, T003, T004, T005, T006, T015, T016, T017, T019, T020, T021, T023, T027, T030 |
| REQ-004 | T001, T002, T003, T012, T013, T014, T018, T021, T022, T023, T024, T025, T026, T030 |
| REQ-005 | T012, T013, T023, T027, T030 |
| REQ-006 | T012, T013, T023, T027, T030 |
| REQ-007 | T001, T002, T003, T012, T013, T014, T022, T023, T027, T030 |
| REQ-008 | T001, T002, T003, T004, T005, T006, T007, T008, T009, T010, T011, T014, T015, T018, T019, T021, T022, T023, T024, T025, T026, T027, T028, T030 |
| REQ-009 | T027, T028, T030 |
| REQ-010 | T001, T002, T003, T012, T013, T014, T015, T016, T017, T019, T020, T021, T022, T023, T030 |
| REQ-011 | T007, T008, T009, T010, T011, T015, T016, T018, T019, T021, T023, T024, T025, T026, T028, T030 |
| REQ-012 | T004, T005, T006, T016, T017, T019, T020, T023, T030 |
| REQ-013 | T001, T002, T003, T007, T008, T009, T010, T011, T012, T013, T014, T022, T023, T024, T025, T026, T027, T030 |
| REQ-014 | T001, T002, T003, T017, T020, T023, T030 |
| NFR-001 | T001, T002, T008, T009, T010, T011, T014, T022, T023, T024, T025, T026, T027, T028, T030 |
| NFR-002 | T001, T002, T003, T007, T008, T009, T010, T011, T014, T015, T016, T017, T018, T019, T020, T021, T022, T023, T024, T025, T026, T029, T030 |
| NFR-003 | T007, T008, T009, T010, T011, T015, T016, T018, T019, T021, T023, T028, T029, T030 |
| NFR-004 | T004, T005, T006, T029, T030 |
| Component `internal/llmconfig` | T001, T002, T003 |
| Component `internal/settings` | T004, T005, T006 |
| Component `internal/provider` | T007, T008, T009, T010, T011 |
| Component `cmd/fox` | T012, T013, T014, T022, T023 |
| Component `internal/app` | T015, T016, T017, T018, T019, T020, T021, T023 |
| Component Secondary Commands | T024, T025, T026 |
| Component Documentation | T027, T028 |
| Validation Strategy | T003, T006, T011, T023, T026, T028, T029, T030 |

## Unmapped Tasks

None. Every task maps to a confirmed requirement, the approved plan, constitution-required TDD/formatting, or repository verification policy.

## Checkpoints

1. **Config checkpoint**: After T006, settings and config resolution tests should pass without provider or CLI changes.
2. **Provider checkpoint**: After T011, generic provider construction should be testable without app startup.
3. **App checkpoint**: After T023, main CLI and TUI/app wiring should use resolved LLM config.
4. **Command checkpoint**: After T026, all production command entrypoints should avoid implicit Zhipu defaults.
5. **Release-readiness checkpoint**: After T030, full repository tests should pass.
