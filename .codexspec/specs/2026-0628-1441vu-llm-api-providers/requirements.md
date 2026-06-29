# Confirmed Requirements: llm-api-providers

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0628-1441vu`
**Status**: Requirements Confirmed
**Last Confirmed**: 2026-06-28 15:14:35 CST

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: User-configured compatible LLM providers

- **Status**: confirmed
- **Statement**: foxharness MUST support user-configured LLM providers that are compatible with either OpenAI-compatible APIs or Claude/Anthropic Messages-compatible APIs.
- **Rationale**: Users should be able to bring any compatible LLM vendor instead of being limited to a built-in vendor.
- **User Evidence**: "Any LLM provider that supports the OpenAI protocol or Claude protocol can be supported."
- **Confirmed At**: 2026-06-28 15:04:52 CST

### NEED-002: Configurable provider connection fields

- **Status**: confirmed
- **Statement**: Users MUST be able to configure the API key source, API base URL, model id, and protocol for their selected LLM provider.
- **Rationale**: These fields are the minimum required inputs for connecting to arbitrary OpenAI-compatible and Claude-compatible endpoints.
- **User Evidence**: "Users can set their LLM vendor API KEY, API BASE URL, and model id."
- **Confirmed At**: 2026-06-28 15:04:52 CST

### NEED-003: Replace the implicit Zhipu default

- **Status**: confirmed
- **Statement**: The current implicit Zhipu-only default MUST be replaced. Missing required LLM configuration MUST produce a clear configuration error instead of assuming Zhipu.
- **Rationale**: The feature goal is to remove the current Zhipu-centered startup behavior and make provider choice explicit.
- **User Evidence**: "The current default uses ZHIPU, and I want to replace this behavior."
- **Confirmed At**: 2026-06-28 15:04:52 CST

### NEED-004: Named provider profiles

- **Status**: confirmed
- **Statement**: foxharness MUST support named LLM provider profiles so users can switch providers without re-entering API key source, base URL, protocol, and default model each time.
- **Rationale**: Users need a convenient workflow for switching among their own LLM suppliers.
- **User Evidence**: "I want users to conveniently switch their own LLM suppliers."
- **Confirmed At**: 2026-06-28 15:04:52 CST

## Constraints

### CON-001: Protocol scope

- **Status**: confirmed
- **Statement**: This feature is limited to LLM APIs compatible with OpenAI-compatible protocols or Claude/Anthropic Messages-compatible protocols.
- **User Evidence**: "Any LLM provider that supports the OpenAI protocol or Claude protocol can be supported."

## Decisions

### DEC-001: Protocol compatibility over vendor hardcoding

- **Status**: confirmed
- **Decision**: Provider support MUST be based on protocol compatibility rather than hardcoded vendor-specific implementations.
- **Alternatives Rejected**: Maintaining Zhipu as the only implicit default provider path.
- **Reason**: The user wants arbitrary compatible LLM vendors, not a fixed vendor list or Zhipu-specific startup path.
- **User Evidence**: "Refer to openclaw so users can set their own LLM vendor API key, API base URL, and model id."

### DEC-002: Configuration resolution priority

- **Status**: confirmed
- **Decision**: Effective LLM configuration MUST resolve in this order: CLI flag, then environment variables, then `~/.foxharness/settings.json`, then no built-in provider default.
- **Alternatives Rejected**: Keeping Zhipu as the fallback default when no user configuration is present.
- **Reason**: This allows one-off overrides, CI/container configuration, persistent user defaults, and explicit failure when required configuration is missing.
- **User Evidence**: The user accepted the recommended priority: CLI flag > environment variables > config file > no default.

### DEC-003: Persistent settings location and shape

- **Status**: confirmed
- **Decision**: Persistent LLM provider configuration MUST use the existing `~/.foxharness/settings.json` file under `llm.default_provider` and `llm.providers`.
- **Alternatives Rejected**: Introducing a separate provider config file for this feature.
- **Reason**: foxharness already uses `~/.foxharness/settings.json` for user settings, and a user-level file avoids encouraging project-local secret configuration.
- **User Evidence**: The user accepted the complete summary that placed persistent config in `~/.foxharness/settings.json`.

### DEC-004: Provider profile and protocol CLI flags

- **Status**: confirmed
- **Decision**: The CLI MUST use `-llm-provider` for named provider profile selection and `-protocol` for OpenAI/Claude compatibility selection. The old `-provider` flag MUST NOT be retained as either a primary flag or compatibility alias. It MUST NOT receive special parsing or migration handling; when used in flag position, it is treated like any other unknown flag by the standard parser, while occurrences inside positional prompt text are preserved as prompt text.
- **Alternatives Rejected**: Reusing `-provider` for either protocol selection or provider profile selection.
- **Reason**: Keeping `-provider` would preserve ambiguity because it historically meant protocol, while the new design distinguishes provider identity from protocol compatibility.
- **User Evidence**: The user questioned why `-provider` should remain when `-llm-provider` exists, accepted the `-llm-provider` / `-protocol` split, and later clarified that `-provider` should behave as if it never existed rather than receiving targeted rejection logic.

### DEC-005: Separate provider identity from protocol compatibility

- **Status**: confirmed
- **Decision**: Provider identity and protocol compatibility MUST be modeled separately: provider profile id selects the configured supplier, while protocol selects the OpenAI-compatible or Claude-compatible adapter.
- **Alternatives Rejected**: Continuing to use one `Provider` concept for both supplier selection and wire protocol selection.
- **Reason**: Separating the concepts supports convenient provider switching and avoids ambiguity in configuration and runtime metadata.
- **User Evidence**: The user adopted the named provider profile recommendation and the `-llm-provider`/`-protocol` split.

### DEC-008: Inline config may run without a settings-backed profile

- **Decision**: A complete inline CLI/environment configuration MAY run even if the requested provider id is absent from settings. Such a run MUST NOT be treated as settings-backed for persistence; model changes should remain in memory unless the resolved provider id maps to an existing settings profile.
- **Alternatives Rejected**: Requiring every `-llm-provider` value to pre-exist in settings even when protocol, base URL, model, and auth fields are complete.
- **Reason**: Users can reasonably launch one-off or local providers without first editing `~/.foxharness/settings.json`, especially for TUI startup with inline connection fields.
- **User Evidence**: The user challenged rejecting complete inline config with an unknown provider id and asked how standalone `fox` TUI startup should choose a provider.

### DEC-006: Zhipu as example only

- **Status**: confirmed
- **Decision**: Zhipu MAY remain as ordinary documentation/example configuration, but it MUST NOT remain a special default code path or required environment variable.
- **Alternatives Rejected**: Keeping `ZHIPU_API_KEY` as the required startup credential.
- **Reason**: The user specifically wants to replace the current Zhipu-default behavior while still allowing Zhipu to be configured like any other compatible provider.
- **User Evidence**: The user asked to replace the current Zhipu default behavior and accepted treating Zhipu as a normal configurable example.

### DEC-007: API key authentication default

- **Status**: confirmed
- **Decision**: Provider profiles MUST default to key-based authentication (`auth: "api-key"`). A resolvable API key source is required for that default mode. API key source MAY be omitted only when the profile explicitly declares `auth: "none"`.
- **Alternatives Rejected**: Requiring an API key source for every provider profile, including local or internally authenticated compatible endpoints; silently allowing missing API keys for cloud-style providers.
- **Reason**: Most cloud LLM providers require an API key, so missing keys must fail clearly, while local or internally authenticated OpenAI-compatible endpoints must remain supported.
- **User Evidence**: The user accepted the clarification: provider profiles default to API-key auth, and API key source can be omitted only with explicit `auth: "none"`.

## Out of Scope

### OUT-001: Non-compatible provider protocols

- **Status**: confirmed
- **Statement**: LLM APIs that are not compatible with OpenAI-compatible or Claude/Anthropic Messages-compatible protocols are out of scope for this feature.
- **Reason**: The requested feature is explicitly bounded by OpenAI and Claude protocol compatibility.
- **User Evidence**: "Any LLM provider that supports the OpenAI protocol or Claude protocol can be supported."

## Open Questions

None.

## Superseded Entries

None.

## Confirmation Log

### Session 2026-06-28 15:04:52 CST

- **Summary Presented**: Support user-configured OpenAI-compatible or Claude-compatible LLM providers; replace the implicit Zhipu default; support named provider profiles and complete inline config; resolve config by CLI, environment, settings file, then no default; store persistent profile config in `~/.foxharness/settings.json`; use `-llm-provider` and `-protocol`; do not retain or specially handle old `-provider`; keep Zhipu only as an ordinary example.
- **User Confirmation**: "确认写入"
- **Entries Confirmed**: NEED-001, NEED-002, NEED-003, NEED-004, CON-001, DEC-001, DEC-002, DEC-003, DEC-004, DEC-005, DEC-006, OUT-001

### Session 2026-06-28 15:14:35 CST

- **Summary Presented**: Provider profiles default to `auth: "api-key"`; API key source is required in that mode; API key source may be omitted only when a profile explicitly sets `auth: "none"` for local or internally authenticated compatible endpoints.
- **User Confirmation**: "采纳"
- **Entries Confirmed**: DEC-007
