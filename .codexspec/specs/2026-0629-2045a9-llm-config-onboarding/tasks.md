# Tasks: Interactive LLM Provider Onboarding

<!--
Language: Generated in English per .codexspec/config.yml (language.document: en).
-->

**Input**: Design documents from `.codexspec/specs/2026-0629-2045a9-llm-config-onboarding/`
**Prerequisites**: `plan.md` (PASS/100), `spec.md`, `requirements.md`

**Tests**: The project constitution mandates Test-Driven Development. Every code task below is therefore test-first: write the failing test (Red), implement the minimum to pass (Green), then refactor with tests green. Documentation/configuration tasks are direct edits.

**Organization**: Tasks are grouped by the approved plan's technical phases. User-story traceability is recorded in each task's `Covers:` line.

## Format: `[ID] [P?] [Phase] Description`

- **[P]**: Can run in parallel (different files, no inter-dependency).
- Each task has exactly one verifiable outcome and lists exact paths.
- `Covers: REQ-xxx; Plan: <component/phase>` preserves traceability.

---

## Phase 1 — Onboarding Error Path

**Purpose**: Replace the bare `missing LLM protocol` empty-configuration error with actionable guidance.

- [x] **T1** [P] [P1] Add empty-configuration detection to `internal/llmconfig`.
  - Test-first in `internal/llmconfig/config_test.go` (or a new `empty_config_test.go`): assert `Resolve(Settings{}, EnvOverrides{}, CLIOverrides{}, lookup)` returns `ErrNoProviderConfigured` (via `errors.Is`); assert a present-but-incomplete profile still returns its existing field error (e.g. `missing LLM base_url`), proving the guard intercepts only the entirely-empty case.
  - Implement `var ErrNoProviderConfigured = errors.New(...)` and `hasNoConfigInput(s, env, cli)`; return it at the top of `Resolve` before provider selection.
  - Verify `TestResolveValidatesRequiredFields` and the other existing `config_test.go` cases still pass unchanged.
  - **Covers**: REQ-003, REQ-011; Plan: Phase 1, Decision 2.

- [x] **T2** [P1] Add the onboarding message and wire it into `cmd/fox`.
  - Test-first in `internal/configcmd/onboarding_test.go`: assert `OnboardingMessage()` contains `fox config` and mentions no vendor or `ZHIPU_API_KEY`.
  - Implement `func OnboardingMessage() string` in `internal/configcmd/onboarding.go`.
  - In `cmd/fox/main.go`, after `resolveLLMConfig` errors, branch on `errors.Is(err, llmconfig.ErrNoProviderConfigured)` and print `OnboardingMessage()` to stderr before exiting. (Extract a small testable helper if needed so the branch is unit-tested in `cmd/fox/main_test.go`.)
  - **Covers**: REQ-003; Plan: Phase 1.

**Checkpoint**: A foxharness with no configuration prints actionable guidance naming `fox config` instead of the bare error.

---

## Phase 2 — Settings Persistence Helpers

**Purpose**: Provide the upsert/default helpers the wizard needs, reusing the existing atomic `0600` raw-preserving `Save`.

- [x] **T3** [P] [P2] Add profile-write helpers to `internal/settings`.
  - Test-first in `internal/settings/settings_test.go`: `SetProvider` upserts a profile and creates `Providers` if nil; `SetDefaultProvider` sets `llm.default_provider` and rejects an unknown id; a write through `Save` preserves unrelated fields and creates the file at `0600` when missing.
  - Implement `func SetProvider(s *Settings, id string, profile llmconfig.Profile) error` and `func SetDefaultProvider(s *Settings, id string) error` (validate existence, mirroring `SetProviderModel`).
  - **Covers**: REQ-008, REQ-009, REQ-012; Plan: Phase 2, Decision 6.

---

## Phase 3 — Catalog, Prompter, and Add Flow

**Purpose**: The preset catalog, the interactive prompt abstraction, and the add-provider wizard flow.

- [x] **T4** [P] [P1] Add the preset catalog as plain data.
  - Test-first in `internal/configcmd/catalog_test.go`: the catalog has exactly the twelve confirmed ids (`openai`, `anthropic`, `xai`, `mistral`, `groq`, `openrouter`, `zhipu`, `deepseek`, `moonshot`, `qwen`, `minimax`, `ollama`); `anthropic`→`claude`, `ollama`→`auth:"none"`, others→`openai`; each non-`none` preset has a non-empty `APIKeyEnv`. (Create `internal/configcmd/doc.go` for the package comment.)
  - Implement `type Preset` and `var Catalog` in `internal/configcmd/catalog.go`, populated with each vendor's OpenAI-/Claude-compatible base URL and a default model (per plan A-1).
  - **Covers**: REQ-006, REQ-012, NFR-003; Plan: Phase 3, Decision 1.

- [x] **T5** [P] [P2] Add the `Prompter` abstraction and its `bufio`/`term` implementation.
  - Test-first in `internal/configcmd/prompter_test.go` using a fake `Prompter`: `ReadLine`, `ReadSecret` (no echo), `Select`, and `Confirm` behave per injected input.
  - Implement the `Prompter` interface and a `bufio`-backed implementation (`golang.org/x/term` for no-echo) in `internal/configcmd/prompter.go`.
  - **Covers**: REQ-010, NFR-001, NFR-002; Plan: Phase 3, Decision 3.

- [x] **T6** [P1] Implement the wizard add flow.
  - Test-first in `internal/configcmd/wizard_test.go` (fake `Prompter`): selecting a preset pre-fills protocol/base URL/model/`api_key_env` and all fields remain editable; the fully-custom entry collects each field and rejects an unsupported protocol (listing `openai`/`claude`); a duplicate profile id triggers overwrite confirmation (plan A-2); on confirm the profile persists via `settings.SetProvider` (+ optional `SetDefaultProvider`) and `Save`.
  - Implement the add flow in `internal/configcmd/wizard.go` (source selection, editable field collection, protocol validation, duplicate confirm, persistence). Use `llmconfig.Profile` and the `settings` helpers.
  - **Covers**: REQ-002 (add), REQ-006, REQ-008, REQ-010; Plan: Phase 3.
  - **Depends**: T3, T4, T5.

**Checkpoint**: A provider can be added end-to-end through the wizard and persists to `~/.foxharness/settings.json`.

---

## Phase 4 — Key Handling and Preflight

**Purpose**: Default env-var key storage with preflight, and the opt-in inline path.

- [x] **T7** [P2] Add the API-key step with preflight validation.
  - Test-first in `internal/configcmd/wizard_test.go`: the default key source is `api_key_env`; when the env var is unset (via the injected lookup), preflight warns and offers inline entry; inline entry is accepted only after a warning + confirm and the entered secret is not echoed back; declining inline still saves the env-var reference flagged as unset; `auth:"none"` profiles skip the key step.
  - Implement the key step and preflight in the add flow; use `ReadSecret` for the inline value.
  - **Covers**: REQ-004, REQ-005, NFR-001; Plan: Phase 4.
  - **Depends**: T6.

---

## Phase 5 — Connectivity Probe

**Purpose**: An optional, skippable live probe that catches misconfiguration before exit.

- [x] **T8** [P2] Add the connectivity probe with an injected factory.
  - Test-first in `internal/configcmd/wizard_test.go`: with a fake `ProviderFactory`, the probe reports success; with a failing factory it reports the reason; the probe is skippable and a failure does not block saving.
  - Implement `probe(ctx, cfg, factory)` (minimal `Generate` with `schema.RoleUser`, short timeout) and wire it into the add flow as an opt-in/skippable step. Default factory = `provider.NewProvider`.
  - **Covers**: REQ-007, NFR-002; Plan: Phase 5, Decision 4.
  - **Depends**: T6.

---

## Phase 6 — List, Set-Default, Dispatch, Non-TTY

**Purpose**: Complete the v1 action set and wire the `fox config` subcommand.

- [x] **T9** [P3] Add the list and set-default actions.
  - Test-first in `internal/configcmd/wizard_test.go`: `list` prints saved profile ids and marks the default; `default <id>` updates `llm.default_provider` via `settings.SetDefaultProvider`.
  - Implement the list and set-default flows in `internal/configcmd/wizard.go`.
  - **Covers**: REQ-002 (list, set-default); Plan: Phase 6.
  - **Depends**: T3.

- [x] **T10** [P1] Add the `configcmd.Run` entry, action dispatch, non-TTY guard, and `cmd/fox` wiring.
  - Test-first: `Run` with no recognized sub-arg opens the action menu; `add`/`list`/`default` sub-args run the matching action; when `Interactive` is false, `Run` exits with a clear "interactive terminal required" message. Extend `cmd/fox/main_test.go` so `args[0] == "config"` selects `launchConfig` and the remaining args are passed as sub-args (not parsed as fox flags / prompt).
  - Implement `func Run(ctx, deps Deps, subArgs []string) error` (menu + sub-verb dispatch + non-TTY guard); add `launchConfig` and the `config` branch to `cmd/fox` `parseArgs`/`main` (construct `configcmd.Deps`, call `Run` before `resolveLLMConfig`).
  - **Covers**: REQ-001, REQ-002 (dispatch); Plan: Phase 6, Decision 5, 7.
  - **Depends**: T6, T7, T8, T9.

**Checkpoint**: `fox config` runs as a real subcommand: add / list / set-default all work, and it fails cleanly without a TTY.

---

## Phase 7 — Documentation and Verification

**Purpose**: User-facing docs and final gate (direct edits; conditional-TDD per repository policy).

- [x] **T11** Document `fox config` and the first-run guidance; finalize the build gate.
  - Update `README.*` to document the `fox config` subcommand, the preset list, key storage (default `api_key_env`, opt-in inline with warning), and the new first-run guidance message.
  - Run `gofmt -w .`, `go vet ./...`, `go test ./...`; all must be green.
  - **Covers**: REQ-001, REQ-006, NFR-001; Plan: Phase 7.
  - **Depends**: T10.

---

## Dependencies and Execution Order

```
T1 ──► T2
T3 ──► T6 ──► T7
          ──► T8
T4 ──► T6
T5 ──► T6
T3 ──► T9
T6, T7, T8, T9 ──► T10 ──► T11
```

- **Parallel (no deps, distinct files/packages)**: T1, T3, T4, T5 can start together.
- **Sequential after the parallel front**: T2 (needs T1); T6 (needs T3, T4, T5); then T7, T8 after T6; T9 after T3; T10 after T6/T7/T8/T9; T11 after T10.
- The dependency graph is acyclic; every task is ordered before its dependents.

## Requirements Coverage

| Requirement | Task(s) | Result |
|-------------|---------|--------|
| REQ-001 | T10 | Covered |
| REQ-002 | T6 (add), T9 (list/default), T10 (dispatch) | Covered |
| REQ-003 | T1, T2 | Covered |
| REQ-004 | T7 | Covered |
| REQ-005 | T7 | Covered |
| REQ-006 | T4, T6 | Covered |
| REQ-007 | T8 | Covered |
| REQ-008 | T3, T6 | Covered |
| REQ-009 | T3 | Covered |
| REQ-010 | T5, T6 | Covered |
| REQ-011 | T1 | Covered |
| REQ-012 | T3, T4 | Covered |
| NFR-001 | T5, T7 | Covered |
| NFR-002 | T5, T6, T8 (injected seams) | Covered |
| NFR-003 | T4 | Covered |

## Unmapped Tasks

None. Every task maps to a confirmed requirement or to necessary implementation support declared in `plan.md`.

## Notes

- **Test-migration clarification**: The plan's risk note about updating existing tests that assert `missing LLM protocol` for empty input is moot — `internal/llmconfig/config_test.go` contains no entirely-empty-input case (its validation tests always supply a present profile). T1 is therefore purely additive (Red→Green) and breaks no existing test. This corrects an over-cautious assumption in `plan.md` without changing its design; no plan decision is affected.
- TDD ordering is mandatory per the constitution; each code task is Red→Green→Refactor.
- Commit after each task or logical group (per repository convention).
