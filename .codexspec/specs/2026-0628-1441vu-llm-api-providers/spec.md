# Feature Specification: User-Configured LLM API Providers

<!--
Language: Generated in English per .codexspec/config.yml (language.document: en).
-->

**Feature Branch**: `2026-0628-1441vu-llm-api-providers`
**Created**: 2026-06-28
**Status**: Draft
**Input**: `requirements.md` (all entries `Status: confirmed`)

## Context

foxharness currently starts from a Zhipu-centered LLM setup: the default model path assumes Zhipu's API, the required credential is `ZHIPU_API_KEY`, and the existing `-provider` flag represents a wire protocol choice (`openai` or `claude`) rather than a user-selected LLM supplier. The confirmed goal is to replace that implicit Zhipu default with user-configured provider profiles, while keeping the scope limited to providers that are compatible with either the OpenAI-style API protocol or the Claude/Anthropic Messages protocol.

This specification treats "provider profile" and "protocol" as separate concepts:

- **Provider profile**: a user-named configuration entry such as `openrouter`, `deepseek`, `local`, or `zhipu`.
- **Protocol**: the request/response adapter used for the selected endpoint, currently `openai` or `claude`.
- **Auth mode**: the profile's credential requirement. Provider profiles default to `auth: "api-key"` and require a resolvable API key source unless the profile explicitly declares `auth: "none"`.

## Goals

- Let users connect foxharness to any OpenAI-compatible or Claude-compatible LLM provider by configuring provider connection fields.
- Replace the implicit Zhipu-only startup behavior with explicit user configuration and clear configuration errors.
- Support named provider profiles so users can switch LLM suppliers without re-entering API key source, base URL, protocol, and model every time.
- Keep provider support protocol-based instead of hardcoding vendor-specific implementations.

## Non-Goals

- Support non-OpenAI-compatible or non-Claude-compatible model APIs.
- Maintain `-provider` as a primary flag or compatibility alias.
- Keep Zhipu as an implicit default provider or required environment-variable path.
- Define a fixed vendor catalog or require foxharness to know every possible LLM provider by name.

## User Scenarios & Testing

### User Story 1 - Configure and run a compatible provider (Priority: P1)

As a foxharness user, I want to configure an LLM provider's API base URL, model id, protocol, and API key source when the provider requires one, so that I can use the provider I already pay for or operate.

**Why this priority**: This is the core feature. Without it, foxharness remains bound to the current implicit Zhipu behavior.

**Independent Test**: Configure a provider profile in `~/.foxharness/settings.json` with protocol, base URL, model, and API key source when required; launch `fox exec` without inline provider fields; verify the resolved provider sends requests through the configured protocol adapter.

**Acceptance Scenarios**:

1. **Given** `~/.foxharness/settings.json` contains a default provider profile with `protocol`, `base_url`, `model`, and any required API key source, **When** the user runs `fox exec "hello"`, **Then** foxharness uses that profile instead of assuming Zhipu.
2. **Given** the selected provider profile declares `protocol: "openai"`, **When** foxharness builds the LLM provider, **Then** it uses the OpenAI-compatible adapter with the configured base URL, credential, and model.
3. **Given** the selected provider profile declares `protocol: "claude"`, **When** foxharness builds the LLM provider, **Then** it uses the Claude/Anthropic Messages-compatible adapter with the configured base URL, credential, and model.
4. **Given** the selected provider profile omits `auth`, **When** foxharness resolves credentials, **Then** it treats the profile as `auth: "api-key"` and requires a resolvable API key source.
5. **Given** the selected provider profile declares `auth: "none"`, **When** foxharness resolves credentials, **Then** it does not require an API key source for that profile.

---

### User Story 2 - Switch among named LLM suppliers (Priority: P1)

As a user with multiple LLM suppliers, I want to switch by provider profile name, so that I do not have to retype credentials and endpoint details for each run.

**Why this priority**: Convenient provider switching was explicitly confirmed as a required workflow.

**Independent Test**: Define at least two provider profiles in settings, set one as `llm.default_provider`, and run commands with and without `-llm-provider`; verify that the selected profile changes while each profile keeps its own base URL, protocol, model, and credential source.

**Acceptance Scenarios**:

1. **Given** profiles `primary` and `local` exist, **When** the user runs `fox exec -llm-provider local "task"`, **Then** foxharness uses `llm.providers.local`.
2. **Given** `llm.default_provider` is `primary`, **When** the user runs `fox exec "task"` without `-llm-provider`, **Then** foxharness uses `llm.providers.primary`.
3. **Given** the user overrides `-model` while selecting a profile, **When** foxharness resolves the effective configuration, **Then** the selected profile supplies the base URL, protocol, and credential source while the CLI model override wins for that run.

---

### User Story 3 - Override configuration for one run or environment (Priority: P2)

As a user in a shell, CI job, or container, I want CLI flags and environment variables to override persistent settings, so that I can run temporary configurations without editing `settings.json`.

**Why this priority**: The confirmed resolution order requires CLI and environment inputs to take precedence over persistent configuration.

**Independent Test**: Set conflicting values through settings, environment variables, and CLI flags; verify the effective resolved configuration follows `CLI flag > environment variables > settings file > no built-in provider default`.

**Acceptance Scenarios**:

1. **Given** settings define provider profile `primary`, **When** the user passes CLI override flags for protocol, base URL, credential source, or model, **Then** the CLI value wins for that run and does not need to rewrite settings.
2. **Given** environment variables provide LLM configuration overrides, **When** no conflicting CLI flag is present, **Then** the environment values win over `~/.foxharness/settings.json`.
3. **Given** both a CLI flag and environment variable are set for the same LLM field, **When** the effective configuration is resolved, **Then** the CLI flag wins.

---

### User Story 4 - Fail clearly when configuration is missing or invalid (Priority: P1)

As a user, I want foxharness to tell me exactly which LLM setting is missing or invalid, so that I can fix configuration problems without guessing whether Zhipu was assumed.

**Why this priority**: Replacing the implicit Zhipu default requires an explicit error path when no usable provider configuration exists.

**Independent Test**: Run foxharness with no complete provider configuration, with an unknown provider profile, with an unsupported protocol, and with a missing credential source; verify each failure reports the actionable missing or invalid field.

**Acceptance Scenarios**:

1. **Given** no complete LLM configuration is available from CLI, environment, or settings, **When** the user starts foxharness, **Then** startup fails with a clear error that identifies the missing configuration and does not mention `ZHIPU_API_KEY` as a required default.
2. **Given** the user passes `-llm-provider missing`, **When** no profile named `missing` exists and no inline configuration completes the provider, **Then** startup fails with an error naming the unknown profile id.
3. **Given** the user passes `-provider openai` in flag position, **When** CLI flags are parsed, **Then** foxharness treats it as a standard unknown flag because no `-provider` flag is registered.
4. **Given** the user writes positional prompt text containing `-provider`, **When** the text appears after the first positional argument or after `--`, **Then** foxharness preserves it as prompt text and does not treat it as a removed flag.

### Edge Cases

- **No default provider**: If `llm.default_provider` is missing and no CLI or environment input identifies or completes a provider, fail with a configuration error.
- **Unknown provider profile**: If the selected provider id is absent from `llm.providers` and the remaining CLI/environment fields do not form a complete inline provider config, fail with an error naming the missing profile id. If the remaining fields do form a complete inline provider config, resolve and run it, but do not treat the provider id as settings-backed for persistence.
- **Missing base URL**: If the effective provider config lacks an API base URL, fail before sending a model request.
- **Missing model**: If the effective provider config lacks a model id, fail before sending a model request.
- **Missing or empty API key source for API-key auth**: If a profile uses the default `auth: "api-key"` mode or explicitly declares `auth: "api-key"`, fail with an error that identifies the credential source problem and redacts secret values.
- **No-key compatible endpoint**: If a profile explicitly declares `auth: "none"`, do not fail solely because the API key source is absent.
- **Unsupported protocol**: If the effective protocol is not `openai` or `claude`, fail with an error listing the supported protocol values.
- **Zhipu example profile**: If the user configures a profile named `zhipu`, treat it exactly like any other compatible provider profile.
- **Existing settings fields**: Updating LLM settings must not discard unrelated existing `settings.json` fields.

## Requirements

### Functional Requirements

- **REQ-001**: foxharness MUST support user-configured LLM providers that use either the OpenAI-compatible adapter or the Claude/Anthropic Messages-compatible adapter.
  - Sources: NEED-001, CON-001, DEC-001

- **REQ-002**: An effective LLM provider configuration MUST include a protocol, API base URL, and model id before a model request is sent. When the effective auth mode is `api-key`, it MUST also include a resolvable API key source.
  - Sources: NEED-002, CON-001, DEC-007

- **REQ-003**: foxharness MUST support named provider profiles stored under `llm.providers` in `~/.foxharness/settings.json`, with `llm.default_provider` selecting the default profile when no higher-priority input selects another provider.
  - Sources: NEED-004, DEC-003, DEC-005

- **REQ-004**: Effective LLM configuration MUST resolve in the following priority order: CLI flags, then environment variables, then `~/.foxharness/settings.json`, then no built-in provider default.
  - Sources: DEC-002, NEED-002, NEED-004

- **REQ-005**: The CLI MUST use `-llm-provider` to select a named provider profile and `-protocol` to select the OpenAI-compatible or Claude-compatible adapter.
  - Sources: DEC-004, DEC-005

- **REQ-006**: The old `-provider` flag MUST NOT be accepted as a primary flag or compatibility alias and MUST NOT receive targeted migration parsing. If `-provider` appears in flag position, foxharness MUST let the standard flag parser report it as an unknown flag. If `-provider` appears in positional prompt text, foxharness MUST preserve it as prompt text.
  - Sources: DEC-004

- **REQ-007**: CLI and environment overrides MUST be available for the configurable LLM connection fields: provider profile id, protocol, API base URL, model id, and API key source. The exact environment-variable names are implementation details, but they MUST be stable, documented, and scoped to foxharness.
  - Sources: NEED-002, DEC-002, DEC-004

- **REQ-008**: foxharness MUST NOT assume Zhipu, `ZHIPU_API_KEY`, or `glm-4.5-air` as a built-in fallback when required LLM configuration is missing.
  - Sources: NEED-003, DEC-002, DEC-006

- **REQ-009**: Zhipu MAY appear in documentation or examples only as a normal provider profile with explicit protocol, base URL, API key source, and model id; it MUST NOT have a special default code path.
  - Sources: DEC-006, DEC-001

- **REQ-010**: Provider profile identity and protocol compatibility MUST be represented separately in configuration, CLI parsing, and runtime metadata. A provider id selects a configured supplier; a protocol selects the adapter.
  - Sources: DEC-005, DEC-004

- **REQ-011**: Provider construction MUST be driven by the resolved protocol and connection fields rather than vendor-specific constructors tied to a single supplier.
  - Sources: NEED-001, NEED-002, DEC-001, DEC-006

- **REQ-012**: Reads and writes of `~/.foxharness/settings.json` MUST preserve unrelated existing fields when updating LLM provider settings.
  - Sources: DEC-003

- **REQ-013**: Provider profiles MUST default to `auth: "api-key"` when `auth` is omitted. API key source MAY be omitted only when the provider profile explicitly declares `auth: "none"`.
  - Sources: DEC-007, NEED-002

- **REQ-014**: Model changes in interactive runs MUST persist only when the resolved provider maps to an existing settings profile. Complete inline configurations, including inline configs paired with an unknown provider id, MUST remain runtime-only and MUST NOT attempt to update a missing settings profile.
  - Sources: DEC-008, NEED-004

### Non-Functional Requirements

- **NFR-001** (security): API key values and resolved secret values MUST NOT be logged, displayed in normal error messages, or written to project-local files. User-level settings MUST avoid encouraging committed secrets.
  - Sources: NEED-002, DEC-003, constitution Security

- **NFR-002** (testability): Configuration resolution and provider construction MUST be testable without real network calls by injecting settings, environment values, CLI values, and local fake endpoints or provider factories.
  - Sources: DEC-001, DEC-002, constitution Test-Driven Development

- **NFR-003** (extensibility): Adding a new compatible vendor MUST NOT require a new vendor-specific provider constructor when that vendor can be represented by protocol, base URL, model id, and API key source.
  - Sources: NEED-001, DEC-001, DEC-005

- **NFR-004** (compatibility of settings storage): Existing `~/.foxharness/settings.json` files that contain unrelated fields MUST remain readable and must not be truncated or rewritten to only the new `llm` object.
  - Sources: DEC-003

### Key Entities

- **LLM Provider Profile**: A user-named configuration entry under `llm.providers.<id>` containing provider-specific connection fields such as protocol, base URL, credential source, and default model.
- **Default Provider**: The `llm.default_provider` value that selects a provider profile when CLI and environment inputs do not select another provider.
- **Resolved LLM Configuration**: The final configuration after applying CLI, environment, and settings priorities. This is the only configuration used to construct the runtime provider.
- **Protocol Adapter**: The adapter selected by `protocol`, currently limited to `openai` and `claude`.
- **API Key Source**: The configured source from which foxharness obtains a credential when one is required, such as an environment variable reference or direct user-provided value.
- **Auth Mode**: The profile setting that determines whether a key is required. `api-key` is the default; `none` explicitly disables API key source requirements for compatible endpoints that do not require an LLM API key.

## Success Criteria

- **SC-001**: A user can run foxharness against a configured OpenAI-compatible provider without setting `ZHIPU_API_KEY`.
- **SC-002**: A user can run foxharness against a configured Claude-compatible provider without setting `ZHIPU_API_KEY`.
- **SC-003**: A user can define at least two provider profiles and switch between them using `-llm-provider` without re-entering base URL, protocol, credential source, and default model.
- **SC-004**: Conflicting CLI, environment, and settings values resolve according to `CLI flag > environment variables > settings file`.
- **SC-005**: Starting foxharness without a complete LLM configuration fails with an actionable error instead of silently selecting Zhipu.
- **SC-006**: Passing `-provider` in flag position fails as a standard unknown flag, while prompt text containing `-provider` remains valid positional text.
- **SC-007**: A provider profile using default `api-key` auth fails clearly when its API key source is missing or unresolved.
- **SC-008**: A provider profile with `auth: "none"` can be resolved without an API key source.
- **SC-009**: A complete inline configuration with an unknown provider id can start foxharness, and interactive model changes do not try to write that unknown id into settings.

## Expected Error Behavior

- Missing required field errors MUST name the missing field and the configuration source being evaluated when possible.
- Unknown provider profile errors MUST include the requested provider profile id.
- Unsupported protocol errors MUST include the unsupported value and the supported values `openai` and `claude`.
- Credential-related errors MUST identify the missing or unresolved source but MUST NOT print secret values.
- Profiles that omit `auth` MUST be validated as `auth: "api-key"` profiles.
- Profiles that explicitly set `auth: "none"` MUST NOT fail solely because no API key source is configured.
- Removed `-provider` handling MUST rely on standard unknown-flag behavior in flag position and MUST NOT scan positional prompt text for targeted rejection.

## Constraints

- Supported protocols are limited to OpenAI-compatible and Claude/Anthropic Messages-compatible LLM APIs.
- Persistent provider configuration must live in `~/.foxharness/settings.json` under `llm.default_provider` and `llm.providers`.
- There is no built-in provider default after CLI, environment, and settings are exhausted.
- Provider profiles default to API-key authentication unless they explicitly declare `auth: "none"`.
- Zhipu is not special at runtime; it is allowed only as an ordinary user-configured example.
- Future implementation must follow the project constitution's TDD requirement.

## Assumptions

- **A-1**: Exact environment-variable names for LLM-specific overrides are not fixed by the confirmed requirements. They are implementation details as long as they preserve the confirmed resolution priority, cover the confirmed fields, are documented, and are scoped to foxharness.
- **A-2**: Direct API key values may be treated as one possible API key source for `api-key` auth, but implementations should prefer environment-variable references in documentation to reduce accidental secret persistence.

## Dependencies

- Existing user settings storage under `~/.foxharness/settings.json`.
- Existing OpenAI-compatible and Claude-compatible provider adapters, or equivalent adapters that satisfy the same internal provider interface.
- Existing CLI/TUI runner configuration paths that currently resolve model and protocol values.

## Out of Scope

- **Non-compatible LLM protocols**: APIs that are not OpenAI-compatible or Claude/Anthropic Messages-compatible are excluded. (OUT-001)
- **Vendor catalog management**: Maintaining a built-in list of all supported vendors is excluded; provider support is compatibility-based.
- **Retaining `-provider`**: Keeping `-provider` as either a protocol flag or compatibility alias is excluded.
- **Implicit Zhipu defaults**: Any code path that silently falls back to Zhipu, `ZHIPU_API_KEY`, or a Zhipu model is excluded.

## Open Questions

None. No open items block later planning or implementation.

## Requirements Traceability

| Confirmed Requirement | Spec Coverage | Notes |
|-----------------------|---------------|-------|
| NEED-001 | REQ-001, REQ-011, NFR-003, User Story 1 | Supports any compatible LLM provider |
| NEED-002 | REQ-002, REQ-004, REQ-007, REQ-011, REQ-013, NFR-001, User Story 1, User Story 3 | Covers configurable API key source, base URL, model id, protocol, and auth behavior |
| NEED-003 | REQ-008, User Story 4, SC-005, Out of Scope | Replaces implicit Zhipu default |
| NEED-004 | REQ-003, REQ-004, REQ-014, User Story 2, SC-003, SC-009 | Named provider profiles, inline configs, and switching |
| CON-001 | REQ-001, REQ-002, Constraints, Out of Scope | Protocol scope preserved |
| DEC-001 | REQ-001, REQ-009, REQ-011, NFR-002, NFR-003 | Protocol compatibility over vendor hardcoding |
| DEC-002 | REQ-004, REQ-007, REQ-008, User Story 3 | Configuration priority preserved |
| DEC-003 | REQ-003, REQ-012, NFR-001, NFR-004, Constraints | Uses existing settings file and preserves unrelated fields |
| DEC-004 | REQ-005, REQ-006, REQ-007, REQ-010, SC-006 | `-llm-provider` and `-protocol`; no `-provider` |
| DEC-005 | REQ-003, REQ-005, REQ-010, NFR-003 | Provider identity separated from protocol |
| DEC-008 | REQ-014, SC-009, Edge Cases | Inline config may run without a settings-backed profile |
| DEC-006 | REQ-008, REQ-009, REQ-011, Out of Scope | Zhipu as example only |
| DEC-007 | REQ-002, REQ-013, User Story 1, Edge Cases, SC-007, SC-008 | API-key auth is default; `auth: "none"` is the explicit no-key exception |
| OUT-001 | Out of Scope, Constraints | Non-compatible protocols excluded |
