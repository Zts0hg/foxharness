# Implementation Plan: Configurable LLM API Providers

**Feature**: `.codexspec/specs/2026-0628-1441vu-llm-api-providers`  
**Spec**: `.codexspec/specs/2026-0628-1441vu-llm-api-providers/spec.md`  
**Date**: 2026-06-28  
**Status**: Draft

## Summary

Foxharness currently constructs Zhipu-backed providers from hardcoded base URLs, the `ZHIPU_API_KEY` environment variable, and the built-in `glm-4.5-air` model fallback. This plan replaces that behavior with a generic LLM configuration layer that resolves a selected provider profile from CLI flags, environment variables, and `~/.foxharness/settings.json`, then constructs either an OpenAI-compatible or Claude-compatible provider from the resolved protocol, base URL, model, and auth settings.

Zhipu remains only as an example provider profile in documentation. It is not a runtime default, not a special constructor path, and not inferred from `ZHIPU_API_KEY`.

## Goals

- Support any OpenAI-compatible or Claude/Anthropic Messages-compatible endpoint through configuration.
- Let users switch named provider profiles through a stable `-llm-provider` flag or environment variable.
- Resolve settings with deterministic priority: CLI > environment > `~/.foxharness/settings.json` > no built-in provider default.
- Remove runtime dependence on hardcoded Zhipu API key, base URL, and model fallback.
- Preserve existing unrelated settings fields when reading or writing `~/.foxharness/settings.json`.
- Keep existing model-changing workflows working against the currently selected LLM profile.

## Non-Goals

- Support non-OpenAI and non-Claude protocols.
- Validate vendor-specific model availability before calls.
- Add an in-session interactive provider switch command.
- Migrate legacy `-provider`, `FOX_MODEL`, top-level `model`, or Zhipu-only settings into the new schema.
- Store secrets in project-local files.

## Technical Context

### Existing Behavior

- `cmd/fox/main.go` registers `-model` and `-provider`; `-provider` currently means protocol (`openai` or `claude`).
- `settings.ResolveModel` resolves model as CLI `-model`, `FOX_MODEL`, top-level settings `model`, then fallback `glm-4.5-air`.
- `internal/provider/factory.go` exposes `NewZhipuProvider(protocol, model)`.
- `internal/provider/openai.go` and `internal/provider/claude.go` hardcode Zhipu base URLs and read `ZHIPU_API_KEY`.
- `internal/app/runner.go`, `internal/app/autodev.go`, `cmd/agentops`, `cmd/feishu`, and `cmd/bench` construct Zhipu providers directly.
- `settings.Save` already preserves unknown JSON fields and writes with mode `0600`.

### Constraints

- Go files must be formatted with `gofmt -w`.
- All tests should pass with `go test ./...`.
- Do not edit files under `vendor/`.
- Runtime errors must not log or display API key values.
- Tests must not require live LLM network calls.

## Configuration Contract

### Settings File

`~/.foxharness/settings.json` will store LLM provider profiles under `llm`:

```json
{
  "llm": {
    "default_provider": "openai-main",
    "providers": {
      "openai-main": {
        "protocol": "openai",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-4.1",
        "auth": "api-key",
        "api_key_env": "OPENAI_API_KEY"
      },
      "local-openai": {
        "protocol": "openai",
        "base_url": "http://127.0.0.1:11434/v1",
        "model": "qwen2.5-coder",
        "auth": "none"
      },
      "anthropic-main": {
        "protocol": "claude",
        "base_url": "https://api.anthropic.com",
        "model": "claude-sonnet-4-20250514",
        "auth": "api-key",
        "api_key_env": "ANTHROPIC_API_KEY"
      }
    }
  }
}
```

Field rules:

- `llm.default_provider`: selected profile id when no CLI or environment profile override is supplied.
- `llm.providers`: map of profile id to provider profile.
- `protocol`: required effective value, allowed values `openai` and `claude`.
- `base_url`: required effective value.
- `model`: required effective value.
- `auth`: optional in stored profiles; defaults to `api-key` when omitted.
- `api_key_env`: optional name of an environment variable containing the API key.
- `api_key`: optional direct API key value. Supported for completeness, but docs should prefer `api_key_env`.
- For `auth: "api-key"`, either `api_key` or a resolvable `api_key_env` must be present after all overrides.
- For `auth: "none"`, key fields may be omitted and no auth header may be sent.

**Covers**: REQ-002, REQ-003, REQ-010, REQ-012, REQ-013, NFR-001, NFR-004

### CLI Flags

`cmd/fox` will use these LLM flags:

- `-llm-provider`: named profile id to select from `llm.providers`.
- `-protocol`: protocol override, `openai` or `claude`.
- `-base-url`: API base URL override.
- `-model`: model id override.
- `-auth`: auth mode override, `api-key` or `none`.
- `-api-key-env`: environment variable name to read the API key from.
- `-api-key`: direct API key override.

The old `-provider` flag will not be registered as a normal flag. Argument parsing will pre-detect `-provider` and `--provider` and return a clear error instructing users to use `-llm-provider` for provider profile selection or `-protocol` for OpenAI/Claude protocol selection.

**Covers**: REQ-004, REQ-005, REQ-006, REQ-007, REQ-010, REQ-013

### Environment Variables

Use foxharness-scoped environment variables for stable overrides:

- `FOXHARNESS_LLM_PROVIDER`
- `FOXHARNESS_LLM_PROTOCOL`
- `FOXHARNESS_LLM_BASE_URL`
- `FOXHARNESS_LLM_MODEL`
- `FOXHARNESS_LLM_AUTH`
- `FOXHARNESS_LLM_API_KEY_ENV`
- `FOXHARNESS_LLM_API_KEY`

The resolver will not use `ZHIPU_API_KEY` as an implicit default. Users may still set a profile's `api_key_env` to `ZHIPU_API_KEY` explicitly.

The resolver will not use `FOX_MODEL` as part of the new LLM resolution contract. This keeps the new behavior explicit and avoids a partial legacy fallback that can provide only a model without protocol and base URL.

**Covers**: REQ-004, REQ-007, REQ-008, REQ-009, NFR-001

## Architecture

### Component: `internal/llmconfig`

Add a small shared package responsible for schema, resolution, and validation. It should not construct SDK clients and should not perform network calls.

Proposed responsibilities:

- Define `Protocol`, `AuthMode`, `Profile`, `Settings`, `CLIOverrides`, `EnvOverrides`, and `ResolvedConfig`.
- Load environment overrides from `os.Getenv` through an injectable lookup function for tests.
- Resolve the effective config using:
  1. settings-selected profile from `llm.default_provider`
  2. environment-selected profile override
  3. CLI-selected profile override
  4. environment field overrides
  5. CLI field overrides
- Validate required fields after merge.
- Resolve API key values only when `auth` is `api-key`.
- Return redacted error details that identify missing field names and provider ids without exposing secrets.

The profile selection rule is: CLI `-llm-provider` overrides `FOXHARNESS_LLM_PROVIDER`, which overrides `llm.default_provider`. Field-level CLI and environment overrides then modify the selected or inline config according to the same CLI > environment > settings priority. If no provider profile exists but CLI/env provide a complete inline config, resolution succeeds without requiring a profile id.

**Covers**: REQ-001, REQ-002, REQ-003, REQ-004, REQ-007, REQ-008, REQ-010, REQ-013, NFR-001, NFR-002

### Component: `internal/settings`

Extend the current settings model:

- Add `LLM llmconfig.Settings` with JSON key `llm`.
- Keep preserving unknown top-level fields through the existing raw JSON merge strategy.
- Preserve unknown fields inside provider profiles by limiting writes to known `llm` fields without rewriting unrelated top-level data.
- Replace model-only helpers with LLM-aware save/update helpers for the selected provider profile.
- Keep `Load` behavior tolerant of missing settings files. Missing or malformed settings still return an empty settings object; the LLM resolver then produces a clear missing-configuration error if no CLI/env config completes the requirement.

The legacy top-level `model` field remains preserved as unknown data if already present, but it is not part of the new LLM resolution contract.

**Covers**: REQ-003, REQ-004, REQ-008, REQ-012, NFR-004

### Component: `internal/provider`

Replace Zhipu-specific construction with generic provider construction from resolved config:

- Add a generic provider factory, for example `NewProvider(config llmconfig.ResolvedConfig) (interfaces.LLMProvider, error)`.
- For `protocol: "openai"`, construct `OpenAIProvider` with the resolved `base_url`, `model`, and auth behavior.
- For `protocol: "claude"`, construct `ClaudeProvider` with the resolved `base_url`, `model`, and auth behavior.
- Remove or stop using `NewZhipuProvider`, `NewZhipuOpenAIProvider`, `NewZhipuClaudeProvider`, and `zhipuAPIKeyFromEnv` in production paths.
- Keep provider package tests at HTTP-request level so they verify base URL, model, and headers without live network.

Auth behavior:

- `auth: "api-key"` applies the resolved API key using the relevant SDK option.
- `auth: "none"` must avoid SDK environment credential fallbacks and must not send `Authorization` or `X-Api-Key`. Use SDK header deletion options where available and prove behavior through tests that capture outgoing requests.

**Covers**: REQ-001, REQ-002, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002, NFR-003

### Component: `cmd/fox`

Update CLI parsing and app startup:

- Register the new LLM flags listed in this plan.
- Reject old `-provider` / `--provider` with a targeted error before normal flag parsing.
- Load settings from `~/.foxharness/settings.json`.
- Resolve effective LLM configuration via `internal/llmconfig`.
- Pass the resolved LLM config into `app.RunTUI` and non-TUI app startup instead of passing a protocol string and model fallback separately.
- Surface missing or invalid config as actionable CLI errors, for example missing profile, missing `base_url`, missing `model`, unsupported protocol, or missing API key for `auth: "api-key"`.

**Covers**: REQ-002, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-010, REQ-013

### Component: `internal/app`

Update application wiring to carry resolved LLM config:

- Replace `AgentRunnerConfig.Provider` and model-only provider construction with a resolved LLM config field.
- `NewAgentRunner` constructs providers only through the generic provider factory.
- `AgentRunner.SetModel(model)` updates the model in the current resolved config and rebuilds the provider with the same selected protocol, base URL, and auth settings.
- Keep `/model` as a model-only command. It should not switch provider profiles.
- When the current config came from a settings-backed named profile, TUI model changes should persist to that profile's `model` field under `llm.providers[profile_id]`. If the current config is fully inline from CLI/env without a settings profile id, the model change remains in memory for that session.
- Update autodev wiring so `.foxharness/autodev.yml` model overrides apply by copying the resolved LLM config with a different `model`, not by reconstructing a Zhipu provider.

**Covers**: REQ-003, REQ-004, REQ-008, REQ-010, REQ-011, REQ-012, NFR-002, NFR-003

### Component: Secondary Commands

Update `cmd/agentops`, `cmd/feishu`, and `cmd/bench`:

- Remove hardcoded `glm-4.5-air`, `ZHIPU_API_KEY`, and `NewZhipuOpenAIProvider` usage.
- Resolve LLM config from `~/.foxharness/settings.json` plus `FOXHARNESS_LLM_*` environment variables.
- Produce the same clear missing-configuration errors as `cmd/fox`.

These commands do not need the full `cmd/fox` CLI flag set unless they already expose LLM-related flags.

**Covers**: REQ-001, REQ-002, REQ-004, REQ-008, REQ-011, REQ-013

### Component: Documentation

Update public docs that currently describe Zhipu as the default:

- `README.md`
- `README.zh-CN.md`
- `README.zh-TW.md`
- `README.ja.md`

Documentation should:

- Show the new `llm.default_provider` / `llm.providers` settings shape.
- Explain config priority: CLI > env vars > `~/.foxharness/settings.json` > no built-in provider default.
- Explain `-llm-provider` versus `-protocol`.
- Explain that `-provider` is no longer accepted.
- Include Zhipu only as an ordinary example profile, with explicit `protocol`, `base_url`, `model`, `auth`, and `api_key_env`.
- Include an `auth: "none"` local endpoint example.
- Prefer `api_key_env` over direct `api_key` in examples.

**Covers**: REQ-003, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-013, NFR-001

## Data Flow

1. CLI parses flags and rejects old `-provider`.
2. The app loads `~/.foxharness/settings.json`.
3. `llmconfig` collects environment overrides.
4. `llmconfig` selects a profile id from CLI, env, or settings.
5. `llmconfig` merges profile values and field overrides.
6. `llmconfig` validates required fields and resolves API key according to auth mode.
7. The app constructs an LLM provider through the generic provider factory.
8. Runtime model changes update the current resolved config and rebuild the provider without changing provider identity or protocol.
9. If applicable, settings save writes the changed model back to the selected profile while preserving unrelated settings data.

**Covers**: REQ-002, REQ-003, REQ-004, REQ-010, REQ-011, REQ-012, REQ-013

## Plan Decisions

### PLD-001: Shared Resolver Package

Use `internal/llmconfig` for schema and resolution instead of putting resolution logic in `cmd/fox` or `internal/provider`.

Reasoning: provider config is needed by multiple entrypoints and by app/autodev wiring. Keeping resolution separate from SDK construction makes tests deterministic and prevents CLI/env parsing from leaking into provider code.

**Covers**: REQ-004, REQ-011, NFR-002, NFR-003

### PLD-002: Explicit Foxharness Env Names

Use `FOXHARNESS_LLM_*` environment variables and do not carry forward `FOX_MODEL` or implicit `ZHIPU_API_KEY` behavior.

Reasoning: the new configuration needs a complete provider definition, not only a model string or vendor-specific key. Scoped names also avoid accidental credential pickup from unrelated SDK defaults.

**Covers**: REQ-004, REQ-007, REQ-008, NFR-001

### PLD-003: No Runtime Zhipu Special Case

Remove Zhipu-specific constructors from production paths. Zhipu is represented by the same profile shape as any other compatible endpoint.

Reasoning: this is the simplest way to satisfy protocol-based compatibility and prevent the old default from reappearing through a helper constructor.

**Covers**: REQ-001, REQ-008, REQ-009, REQ-011, NFR-003

### PLD-004: Auth-None Is an Explicit Mode

Treat `auth: "none"` as an explicit provider profile setting or override, not as "missing API key".

Reasoning: missing credentials for `api-key` should fail early, while local or gateway endpoints that intentionally do not require credentials must remain usable.

**Covers**: REQ-002, REQ-013, NFR-001

### PLD-005: Provider Switching Is Profile Selection, Not Protocol Aliasing

`-llm-provider` selects a named profile. `-protocol` overrides only protocol. The old `-provider` flag is rejected because its old meaning was protocol-like and conflicts with the new user-facing provider concept.

Reasoning: users need to switch full configurations, not just protocol strings. Separating profile id from protocol avoids ambiguous CLI behavior.

**Covers**: REQ-003, REQ-005, REQ-006, REQ-010

### PLD-006: Keep `/model` Narrow

Do not introduce provider switching through the existing `/model` command.

Reasoning: the spec requires convenient provider switching through configuration, especially named profiles and CLI/env selection. Extending in-session commands would be a separate interaction design and is not needed to replace the Zhipu default.

**Covers**: REQ-003, REQ-010, REQ-012

## Implementation Phases

### Phase 1: Resolution Tests and `internal/llmconfig`

- Add unit tests for profile selection priority.
- Add unit tests for field override priority.
- Add unit tests for complete inline CLI/env config with no settings profile.
- Add unit tests for missing config producing clear errors.
- Add unit tests for `auth: "api-key"` requiring a resolvable key.
- Add unit tests for `auth: "none"` allowing missing key.
- Implement `internal/llmconfig` types, env loading, merge, validation, and redacted error behavior.

**Covers**: REQ-002, REQ-003, REQ-004, REQ-007, REQ-008, REQ-010, REQ-013, NFR-001, NFR-002

### Phase 2: Settings Schema and Persistence

- Extend `internal/settings.Settings` with `LLM`.
- Update load tests for `llm.default_provider` and `llm.providers`.
- Update save tests to verify unknown top-level fields remain preserved.
- Add save/update tests for changing a selected provider profile's model.
- Ensure missing or malformed settings files do not silently create a built-in provider default.

**Covers**: REQ-003, REQ-008, REQ-012, NFR-004

### Phase 3: Generic Provider Factory

- Add provider tests for OpenAI-compatible base URL, model, and auth header behavior.
- Add provider tests for Claude-compatible base URL, model, and auth header behavior.
- Add provider tests proving `auth: "none"` sends no `Authorization` or `X-Api-Key` even when common SDK env vars are set.
- Implement generic provider factory from resolved LLM config.
- Remove production use of Zhipu-specific provider constructors.

**Covers**: REQ-001, REQ-002, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002, NFR-003

### Phase 4: Main CLI Wiring

- Update `internal/app.CLIConfig` and `cmd/fox` flag parsing.
- Add tests for `-llm-provider`, `-protocol`, `-base-url`, `-auth`, `-api-key-env`, and `-api-key`.
- Add tests for rejecting `-provider` and `--provider` with migration guidance.
- Replace model fallback resolution with LLM config resolution.
- Update errors surfaced by startup to identify missing required config fields.

**Covers**: REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-010, REQ-013

### Phase 5: App and Autodev Wiring

- Update `AgentRunnerConfig` to carry resolved LLM config.
- Update `NewAgentRunner` and `SetModel` to rebuild providers through the generic factory.
- Update TUI model persistence to write into the selected provider profile when available.
- Update autodev dependency construction to apply autodev model overrides on top of the resolved provider config.
- Update app tests that currently assert Zhipu or protocol-only construction.

**Covers**: REQ-003, REQ-004, REQ-008, REQ-010, REQ-011, REQ-012, NFR-002, NFR-003

### Phase 6: Secondary Commands

- Update `cmd/agentops`, `cmd/feishu`, and `cmd/bench` to use shared settings/env resolution.
- Remove Zhipu-specific comments and key checks from these commands.
- Add focused tests where command startup can be tested without network.

**Covers**: REQ-001, REQ-002, REQ-004, REQ-008, REQ-011, REQ-013

### Phase 7: Documentation and Examples

- Update README files with the new settings schema and CLI flags.
- Replace old `-provider` examples with `-llm-provider` and `-protocol` examples.
- Add Zhipu as a normal explicit profile example.
- Add a local `auth: "none"` example.
- Document security guidance for `api_key_env`.

**Covers**: REQ-003, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-013, NFR-001

## Validation Strategy

### Unit Tests

- `internal/llmconfig`: resolution, validation, errors, auth modes, env override names.
- `internal/settings`: load/save schema, unknown field preservation, provider profile update.
- `internal/provider`: request construction for OpenAI and Claude protocols, auth headers, base URLs, model ids.
- `cmd/fox`: flag parsing, old flag rejection, startup config resolution errors.
- `internal/app`: runner config propagation and `SetModel` provider rebuild.

### Integration-Level Tests

- Run app startup paths with temporary home directories and fake provider factories.
- Run provider calls against `httptest.Server` endpoints, not live LLM APIs.
- Verify no test depends on `ZHIPU_API_KEY`, OpenAI, Anthropic, or network availability.

### Final Verification

```bash
gofmt -w <changed-go-files>
go test ./...
```

**Covers**: REQ-001, REQ-002, REQ-004, REQ-008, REQ-011, REQ-013, NFR-001, NFR-002, NFR-003, NFR-004

## Risks and Mitigations

### Risk: SDK Default Environment Credentials Leak Into `auth: "none"`

OpenAI and Anthropic SDKs can read their own environment variables by default. If `auth: "none"` is configured, those defaults must not add auth headers.

Mitigation: add request-capturing tests with common SDK key env vars set. Use SDK header deletion options or a small transport wrapper to strip auth headers if needed.

**Covers**: REQ-013, NFR-001

### Risk: Partial Overrides Create Invalid Mixed Config

Users can override only some fields through CLI or environment variables, which may leave missing base URL, model, protocol, or API key.

Mitigation: centralize final validation in `llmconfig` after all merges and return actionable field-specific errors.

**Covers**: REQ-002, REQ-004, REQ-007

### Risk: Existing `/model` Persistence Writes the Wrong Location

The old top-level `model` field is no longer the effective model config. Persisting to it would make `/model` appear successful but not affect future provider resolution.

Mitigation: update model persistence to write `llm.providers[active_profile].model` when an active profile id exists.

**Covers**: REQ-003, REQ-012

### Risk: Old `-provider` Error Looks Like a Generic Flag Parse Failure

If left to Go's flag package, users may only see "flag provided but not defined".

Mitigation: pre-scan arguments and return a targeted error explaining `-llm-provider` and `-protocol`.

**Covers**: REQ-006

## Requirement Coverage Matrix

| Requirement | Planned Coverage |
| --- | --- |
| REQ-001 | Generic OpenAI/Claude provider factory; secondary command updates; provider tests |
| REQ-002 | `llmconfig` validation; auth mode handling; clear startup errors |
| REQ-003 | `llm.default_provider` and named `llm.providers`; `-llm-provider` switching |
| REQ-004 | Resolver priority CLI > env > settings > no default |
| REQ-005 | New `-llm-provider` and `-protocol` CLI flags |
| REQ-006 | Explicit old `-provider` rejection |
| REQ-007 | CLI/env overrides for provider id, protocol, base URL, model, auth, API key source |
| REQ-008 | Removal of implicit Zhipu key/base/model fallback |
| REQ-009 | Zhipu docs/examples only as ordinary profile |
| REQ-010 | Provider profile id separated from protocol |
| REQ-011 | Provider construction from resolved protocol and fields, no vendor-specific constructor path |
| REQ-012 | Settings raw merge preservation and profile model update helpers |
| REQ-013 | `auth: "api-key"` default and explicit `auth: "none"` behavior |
| NFR-001 | Redacted errors, no secret logging, env-var examples, auth-none header tests |
| NFR-002 | Injectable env/settings and fake HTTP/provider factories |
| NFR-003 | Generic protocol factory and config schema |
| NFR-004 | Existing settings file preservation and `0600` save behavior |

## Open Questions

None.
