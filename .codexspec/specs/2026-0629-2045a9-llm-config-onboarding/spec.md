# Feature Specification: Interactive LLM Provider Onboarding

<!--
Language: Generated in English per .codexspec/config.yml (language.document: en).
-->

**Feature Branch**: `2026-0629-2045a9-llm-config-onboarding`
**Created**: 2026-06-29
**Status**: Draft
**Input**: `requirements.md` (all entries `Status: confirmed`; OPEN-001 resolved by DEC-005)

## Context

After the provider-configuration feature shipped, foxharness no longer assumes a built-in LLM vendor: when no provider is configured, startup fails with `resolve LLM configuration: missing LLM protocol`. That message is accurate but unhelpful to a first-time user — it names no remediation path and offers no way to create a configuration without hand-editing `~/.foxharness/settings.json`.

This feature adds an **interactive onboarding path**. A new `fox config` subcommand runs a guided wizard that adds a provider profile, and the empty-configuration error is replaced with an actionable message that points the user to that wizard. The wizard is built on top of the existing provider resolution, validation, and persistence machinery; it introduces no new vendor code paths and no new persistence location.

Three concepts carry over from the provider-configuration feature and are reused unchanged:

- **Provider profile**: a user-named entry under `llm.providers.<id>` with protocol, base URL, model, auth mode, and API key source.
- **Protocol adapter**: `openai` or `claude`, selected by the profile's `protocol`.
- **API key source**: `api_key_env` (an environment-variable reference) or `api_key` (an inline value).

## Goals

- Let a first-time user go from "no configuration" to "a working provider" entirely through an interactive command, without editing JSON by hand.
- Replace the terse `missing LLM protocol` empty-configuration error with guidance that names the next action (`fox config`).
- Make adding a common provider fast by shipping a small, curated preset catalog that pre-fills connection details.
- Catch configuration mistakes before the user leaves the wizard (env-var preflight and an optional live connectivity probe).

## Non-Goals

- Provide a non-interactive / scriptable configuration mode.
- Edit or remove existing provider profiles.
- Localize the wizard prompts (English for v1).
- Maintain an exhaustive vendor catalog; support remains protocol-based, and presets are convenience templates only.
- Introduce a separate credential store; secrets continue to live in `~/.foxharness/settings.json` per the existing design.

## User Scenarios & Testing

### User Story 1 - First-run onboarding from a bare error (Priority: P1)

As a new user who just installed foxharness and ran `fox`, I want to be told what to do next and then be guided through adding a provider, so that I am never stuck staring at `missing LLM protocol`.

**Why this priority**: This is the exact pain that motivated the feature. The empty-configuration error is the first thing a new user sees.

**Independent Test**: Start `fox` with no `~/.foxharness/settings.json`, no `FOXHARNESS_LLM_*` environment variables, and no CLI provider flags; verify the output directs the user to `fox config`. Then run `fox config`, complete the wizard with a preset, and verify the next `fox` startup resolves the saved provider.

**Acceptance Scenarios**:

1. **Given** no LLM configuration exists from any source, **When** the user starts `fox`, **Then** foxharness prints an actionable message that names `fox config` as the way to add a provider, instead of the bare `missing LLM protocol`.
2. **Given** the user completed `fox config` and saved a default provider, **When** the user starts `fox` again, **Then** startup resolves the saved provider and no longer prints the onboarding message.

---

### User Story 2 - Add a common provider from a preset (Priority: P1)

As a user who wants to use a well-known provider, I want to pick it from a list and have base URL, default model, and the suggested API key environment variable pre-filled, so that I only supply the credential and confirm.

**Why this priority**: Pre-filling known-vendor details is the largest convenience lever and the explicit motivation for the catalog.

**Independent Test**: Run `fox config`, select a preset (for example `deepseek`), confirm the pre-filled base URL / model / suggested `api_key_env`, complete the credential step, and verify the persisted profile matches the resolved values.

**Acceptance Scenarios**:

1. **Given** the wizard presents the preset catalog, **When** the user selects `deepseek`, **Then** protocol, base URL, default model, and a suggested `api_key_env` (`DEEPSEEK_API_KEY`) are pre-filled and editable.
2. **Given** the user selects the `ollama` preset, **Then** the wizard pre-fills a local base URL and `auth: "none"`, and does not require an API key.
3. **Given** the user selects the `anthropic` preset, **Then** the wizard selects the `claude` protocol adapter rather than `openai`.

---

### User Story 3 - Add a fully custom provider (Priority: P2)

As a user with a self-hosted or less common OpenAI-/Claude-compatible endpoint, I want to enter every field myself, so that I am not limited to the preset list.

**Why this priority**: The catalog must not become a ceiling; custom entry guarantees any compatible endpoint can be configured.

**Independent Test**: Run `fox config`, choose the fully-custom entry, enter protocol (`openai` or `claude`), base URL, model, auth mode, and API key source, and verify the persisted profile holds exactly the entered values.

**Acceptance Scenarios**:

1. **Given** the user chooses the fully-custom entry, **When** they enter `openai` as protocol, a base URL, a model, and an `api_key_env`, **Then** the wizard accepts and persists a profile with those exact values.
2. **Given** the user enters an unsupported protocol in custom mode, **When** the wizard validates input, **Then** it rejects the value and lists the supported protocols `openai` and `claude`.

---

### User Story 4 - Confidence before leaving the wizard (Priority: P2)

As a user finishing setup, I want the wizard to confirm my environment variable is actually set and optionally test the connection, so that a typo or missing export does not surface as a confusing error on my next `fox` run.

**Why this priority**: Preflight and the probe remove the "configured but still broken" gap that would otherwise just move the bad first-run experience one step later.

**Independent Test**: Run `fox config` with the chosen `api_key_env` unset in the shell; verify the wizard warns and offers inline entry. Complete the wizard and run the connectivity probe against a fake endpoint; verify success/failure is reported and the user may skip.

**Acceptance Scenarios**:

1. **Given** the chosen `api_key_env` is not set in the current shell, **When** the wizard reaches the save step, **Then** it warns prominently that the variable is unset and offers to enter the key inline instead.
2. **Given** the user declines inline entry for an unset env var, **Then** the wizard still allows saving the env-var reference but records that it is currently unset.
3. **Given** the wizard offers a connectivity probe, **When** the user accepts, **Then** it sends a minimal request with the resolved configuration and reports success or the failure reason.
4. **Given** the probe is offered, **When** the user skips it, **Then** the wizard proceeds to save without probing.

---

### User Story 5 - Manage the default provider (Priority: P3)

As a user with one or more saved profiles, I want to list them and choose which one is the default, so that `fox` starts against the provider I expect.

**Why this priority**: Set-default and list complete the minimum useful management surface for v1 and make onboarding repeatable.

**Independent Test**: Save two profiles via `fox config`, list them, set one as default, and verify `llm.default_provider` reflects the choice.

**Acceptance Scenarios**:

1. **Given** at least one profile is saved, **When** the user runs `fox config` list, **Then** the wizard prints the saved profile ids and marks the current default.
2. **Given** the user sets a saved profile as default, **Then** `llm.default_provider` in `~/.foxharness/settings.json` is updated to that profile id.

---

### Edge Cases

- **No configuration at all vs. partial configuration**: The onboarding message is shown only when configuration is entirely empty. If a profile exists but is incomplete (for example, missing base URL), foxharness keeps the existing specific field error (`missing LLM base_url`) rather than the generic onboarding message.
- **`api_key_env` unset at preflight**: The wizard warns and offers inline entry; declining keeps the env-var reference but flags it as currently unset.
- **Inline plaintext opt-in**: Choosing to store the key inline requires an explicit confirmation after a plaintext warning; the entered value is not echoed back in full.
- **Probe failure**: A failed probe reports the reason and does not block saving unless the user chooses to abort.
- **Probe against `auth: "none"` (local) endpoints**: The probe runs without an API key.
- **Missing `~/.foxharness/settings.json`**: The wizard creates the file with only the `llm` section rather than failing.
- **Existing unrelated settings fields**: Updating LLM settings preserves all unrelated existing fields.
- **Duplicate profile id on add**: When the entered profile id already exists, the wizard confirms overwrite before persisting (assumption A-2).
- **Non-interactive stdin**: If `fox config` is launched without an interactive terminal (for example, piped stdin in CI), the wizard exits with a clear message that interactive mode is required, because a non-interactive configuration mode is out of scope (OUT-002).

## Requirements

### Functional Requirements

- **REQ-001**: foxharness MUST provide a `fox config` subcommand, dispatched on `args[0]` consistent with the existing `exec` and `autodev` subcommands, that launches a guided interactive wizard for adding an LLM provider profile.
  - Sources: NEED-001, DEC-003

- **REQ-002**: The v1 `fox config` action set MUST be limited to add (the guided wizard), list, and set-default. Editing and removing existing profiles MUST NOT be offered.
  - Sources: NEED-001, DEC-003, DEC-004

- **REQ-003**: When LLM resolution yields an entirely empty configuration (no settings file content, no CLI provider flags, no `FOXHARNESS_LLM_*` environment overrides), foxharness MUST emit an actionable message that directs the user to run `fox config`, instead of the bare `missing LLM protocol` error.
  - Sources: NEED-002

- **REQ-004**: Before persisting a profile whose API key source is `api_key_env`, the wizard MUST verify that the named environment variable is currently set in the shell. If it is unset, the wizard MUST warn prominently and offer to enter the key inline as `api_key` instead.
  - Sources: NEED-003, DEC-001

- **REQ-005**: The wizard MUST default to storing the API key as `api_key_env`. Storing the key inline as `api_key` MUST be opt-in only and MUST be preceded by a warning that the value will be written in plaintext to `~/.foxharness/settings.json` and require explicit confirmation.
  - Sources: DEC-001

- **REQ-006**: The wizard MUST present a built-in preset catalog containing exactly the twelve confirmed providers (`openai`, `anthropic`, `xai`, `mistral`, `groq`, `openrouter`, `zhipu`, `deepseek`, `moonshot`, `qwen`, `minimax`, `ollama`). Selecting a preset MUST pre-fill protocol, base URL, default model, and a suggested `api_key_env` (or `auth: "none"` for `ollama`). A fully-custom entry MUST also be available.
  - Sources: NEED-004, CON-002, DEC-005

- **REQ-007**: Before finishing, the wizard MUST offer a live connectivity probe that sends a minimal request using the resolved configuration and reports success or the failure reason. The probe MUST be skippable and MUST NOT block saving on failure unless the user aborts.
  - Sources: NEED-005

- **REQ-008**: The wizard MUST persist a new or updated profile into `~/.foxharness/settings.json` under `llm.providers.<id>` and, when the user chooses, set `llm.default_provider` to that id. The wizard MUST NOT introduce a separate credential file.
  - Sources: DEC-002, NEED-001

- **REQ-009**: Reads and writes of `~/.foxharness/settings.json` during the wizard MUST preserve unrelated existing fields, and a missing file MUST be created containing only the `llm` section.
  - Sources: DEC-002

- **REQ-010**: The wizard MUST collect, at minimum, the profile id, protocol, base URL, model, auth mode, and API key source, pre-filled by the selected preset where applicable and all editable before saving.
  - Sources: NEED-001, NEED-004

- **REQ-011**: The wizard, the preset catalog, and the connectivity probe MUST be limited to providers compatible with the OpenAI Chat Completions protocol or the Claude/Anthropic Messages protocol.
  - Sources: CON-001

- **REQ-012**: The preset catalog MUST be ordinary template/example data (pre-filled connection fields) and MUST NOT introduce vendor-specific code paths or special-case provider resolution logic.
  - Sources: CON-002, DEC-005

### Non-Functional Requirements

- **NFR-001** (security): Inline API key values MUST be persisted only after an explicit plaintext warning and user confirmation. The wizard MUST NOT echo or log the full secret value, and MUST NOT write secrets to any project-local file.
  - Sources: DEC-001, constitution Security

- **NFR-002** (testability): Wizard steps, preset selection, the env-var preflight check, and the connectivity probe MUST be testable without real network calls by injecting an environment lookup, a settings store, and a probe target (fake endpoint or provider factory).
  - Sources: NEED-003, NEED-005, constitution Test-Driven Development

- **NFR-003** (extensibility): Adding a new preset or adjusting a preset's base URL / default model MUST require changing only template data and MUST NOT require new code paths or constructor changes.
  - Sources: CON-002, DEC-005

### Key Entities

- **Provider Preset**: A built-in template entry (id, protocol, base URL, default model, suggested `api_key_env` or `auth: "none"`) used to pre-fill the wizard. Presets are data, not runtime code paths.
- **Wizard Session**: The interactive flow that collects profile fields, runs the env-var preflight, optionally runs the connectivity probe, and persists the resulting profile.
- **Preflight Check**: The validation that an `api_key_env` key source resolves in the current environment before the profile is persisted.
- **Connectivity Probe**: A minimal live request built from the resolved configuration, used to confirm the endpoint and credential work before the wizard exits.
- **Onboarding Message**: The actionable empty-configuration message that directs the user to `fox config`, shown only when no provider configuration exists from any source.

## Success Criteria

- **SC-001**: A new user with no configuration who runs `fox` sees a message naming `fox config`, and can reach a working provider through the wizard alone (no manual JSON editing).
- **SC-002**: Selecting any of the twelve presets pre-fills protocol, base URL, default model, and suggested `api_key_env`; only the credential (or skip for `auth: "none"`) remains to confirm.
- **SC-003**: A user can configure a fully-custom OpenAI- or Claude-compatible endpoint not present in the preset list.
- **SC-004**: A profile whose `api_key_env` is unset is caught at preflight with a clear warning and an inline fallback, rather than failing on the next `fox` run.
- **SC-005**: The connectivity probe reports success or a failure reason and can be skipped; a failed probe does not silently corrupt or block a save.
- **SC-006**: Existing unrelated `settings.json` fields are preserved across wizard writes, and a missing file is created cleanly.

## Expected Error Behavior

- The empty-configuration path MUST print actionable guidance naming `fox config` and MUST NOT assume any provider or mention a vendor-specific required variable.
- A partially-configured profile (for example, present but missing base URL or model) MUST keep the existing specific field error rather than the generic onboarding message.
- An unsupported protocol entered in custom mode MUST be rejected with a message listing `openai` and `claude`.
- A missing required field at validation MUST name the missing field.
- An unset `api_key_env` at preflight MUST produce a prominent warning and offer inline entry; the secret MUST NOT be printed in full.
- A failed connectivity probe MUST report the failure reason and remain non-blocking unless the user aborts.

## Constraints

- Supported protocols are limited to OpenAI-compatible and Claude/Anthropic Messages-compatible LLM APIs (CON-001).
- The preset catalog is template/example data only; no vendor-specific code paths or special-case resolution (CON-002).
- Persistence stays in `~/.foxharness/settings.json` under `llm.providers` and `llm.default_provider`; no separate credential store (DEC-002).
- v1 actions are add, list, and set-default only; edit/remove and non-interactive mode are excluded (DEC-004).
- API key storage defaults to `api_key_env`; inline plaintext is opt-in with a warning (DEC-001).
- Implementation MUST follow the project constitution's TDD requirement.

## Assumptions

- **A-1**: The exact base URL and default model for each of the twelve presets are not fixed by the confirmed requirements beyond the membership list and suggested `api_key_env` names; they are finalized during implementation. They are template data and may be corrected without changing product intent (residual of resolved OPEN-001).
- **A-2**: When the user enters a profile id that already exists, the wizard confirms overwrite before persisting. This behavior is not otherwise specified by the confirmed requirements and is the minimal safe default for v1.
- **A-3**: The connectivity probe issues a minimal completion-style request using the resolved provider factory; any HTTP or model error is reported as a probe failure with a reason. The exact request shape is an implementation detail.

## Dependencies

- Existing provider configuration resolution and validation in `internal/llmconfig` (including the `missing LLM protocol` / field-specific error path the onboarding message intercepts).
- Existing user settings storage in `internal/settings` (`~/.foxharness/settings.json`, `llm.providers` / `llm.default_provider`, field-preserving writes).
- Existing protocol-based provider factory (`openai` / `claude`) reused for the connectivity probe.
- Existing `fox` subcommand dispatch pattern (`exec`, `autodev`) that `fox config` joins.

## Out of Scope

- **Editing or removing existing profiles**: deferred from v1 (OUT-001, DEC-004).
- **Non-interactive / scriptable configuration mode**: deferred from v1 (OUT-002, DEC-004).
- **Wizard internationalization**: wizard copy is English for v1 (OUT-003).
- **Full SecretRef model (env / file / exec)**: a richer secret-reference model like the openclaw reference project's is out of scope; only `api_key_env` and inline `api_key` are supported (DEC-001 rejected alternative).
- **Providers beyond the twelve confirmed presets**: additional presets may be added later as template data; the catalog is not exhaustive in v1.

## Open Questions

None. OPEN-001 (preset catalog membership) is resolved by DEC-005. Residual per-preset base URL / model values are an implementation detail captured in assumption A-1 and do not block planning or implementation.

## Requirements Traceability

| Confirmed Requirement | Spec Coverage | Notes |
|-----------------------|---------------|-------|
| NEED-001 | REQ-001, REQ-002, REQ-008, REQ-010, User Story 1, User Story 2 | `fox config` interactive wizard |
| NEED-002 | REQ-003, User Story 1, Expected Error Behavior, SC-001 | Empty-config onboarding message |
| NEED-003 | REQ-004, NFR-002, User Story 4, Edge Cases, SC-004 | `api_key_env` preflight |
| NEED-004 | REQ-006, REQ-010, REQ-012, User Story 2, SC-002 | Built-in preset catalog |
| NEED-005 | REQ-007, NFR-002, User Story 4, Edge Cases, SC-005 | Live connectivity probe |
| CON-001 | REQ-011, Constraints, Non-Goals | Protocol scope preserved |
| CON-002 | REQ-012, NFR-003, Constraints | Presets are template data only |
| DEC-001 | REQ-004, REQ-005, NFR-001, Edge Cases | Default `api_key_env`; inline opt-in with warning |
| DEC-002 | REQ-008, REQ-009, Constraints, Dependencies, SC-006 | Reuses `~/.foxharness/settings.json`; preserves fields |
| DEC-003 | REQ-001, REQ-002, Constraints | Subcommand form; v1 actions |
| DEC-004 | REQ-002, Out of Scope (OUT-001, OUT-002) | Edit/remove and non-interactive excluded |
| DEC-005 | REQ-006, REQ-012, NFR-003 | Twelve-provider v1 catalog |
| OUT-001 | Out of Scope | Edit/remove deferred |
| OUT-002 | Out of Scope | Non-interactive mode deferred |
| OUT-003 | Out of Scope, Non-Goals | Wizard i18n deferred |
