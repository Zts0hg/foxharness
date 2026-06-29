# Confirmed Requirements: llm-config-onboarding

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0629-2045a9`
**Status**: Requirements Confirmed
**Last Confirmed**: 2026-06-29 21:05:00 CST

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Interactive provider configuration subcommand

- **Status**: confirmed
- **Statement**: foxharness MUST provide a `fox config` subcommand that launches a guided interactive wizard for adding an LLM provider profile, so users can configure a provider without hand-editing `~/.foxharness/settings.json`.
- **Rationale**: The current first-run experience emits a terse `missing LLM protocol` error with no onboarding path. A guided wizard makes provider setup convenient and discoverable.
- **User Evidence**: "我希望实现一个方便的交互式命令，让用户可以便捷添加llm provider配置。"
- **Confirmed At**: 2026-06-29 20:50:00 CST

### NEED-002: Friendly first-run guidance

- **Status**: confirmed
- **Statement**: When LLM resolution yields an entirely empty configuration (no settings file, no CLI flags, no environment overrides), foxharness MUST emit an actionable message that points the user to `fox config`, instead of the bare `missing LLM protocol` error.
- **Rationale**: The error that motivated this feature is unhelpful to a first-time user; the new message must guide them to the fix.
- **User Evidence**: "当前fox启动时只会给出如下简单的报错信息：resolve LLM configuration: missing LLM protocol"
- **Confirmed At**: 2026-06-29 20:50:00 CST

### NEED-003: API key preflight validation

- **Status**: confirmed
- **Statement**: The wizard MUST validate, before persisting a profile, that an `api_key_env`-based key source is currently resolvable (the named environment variable is set in the shell). When it is not set, the wizard MUST warn prominently and offer to enter the key inline as a fallback.
- **Rationale**: Recording an `api_key_env` that the user has not exported produces a confusing second error on the next run; preflight removes that gap. Inspired by the openclaw reference project's "preflight validation before saving" of secret references.
- **User Evidence**: The user selected "方案 1 (preflight) + 默认 api_key_env、内联作带警告的可选项."
- **Confirmed At**: 2026-06-29 20:50:00 CST

### NEED-004: Built-in provider preset catalog

- **Status**: confirmed
- **Statement**: The wizard MUST offer a built-in catalog of common provider presets (e.g., zhipu, openai, anthropic, deepseek, moonshot, qwen). Selecting a preset MUST pre-fill base URL, a default model, and a suggested `api_key_env` name. A fully-custom entry MUST also be available.
- **Rationale**: Pre-filling known-vendor connection details is the single largest convenience lever and is the explicit motivation for the feature.
- **User Evidence**: The user selected "内置多厂商预设目录."
- **Confirmed At**: 2026-06-29 20:50:00 CST

### NEED-005: Live connectivity probe at end of wizard

- **Status**: confirmed
- **Statement**: Before finishing, the wizard MUST offer a live connectivity probe that sends a minimal request using the resolved configuration and reports success or the failure reason. The probe MUST be skippable.
- **Rationale**: Confirming the configured provider actually works before exiting the wizard prevents a silent misconfiguration from being discovered on the next `fox` run.
- **User Evidence**: The user selected connectivity probe option A ("要 … 新增一次真实 LLM 调用").
- **Confirmed At**: 2026-06-29 20:50:00 CST

## Constraints

### CON-001: Protocol scope

- **Status**: confirmed
- **Statement**: The wizard is limited to providers compatible with the OpenAI Chat Completions protocol or the Claude/Anthropic Messages protocol, consistent with the existing provider feature boundary.
- **User Evidence**: Inherited from the confirmed `llm-api-providers` feature (CON-001 / OUT-001).

### CON-002: Presets are template data, not vendor code paths

- **Status**: confirmed
- **Statement**: The preset catalog MUST be ordinary template/example data (pre-filled connection fields). It MUST NOT introduce vendor-specific code paths or special-case resolution logic.
- **User Evidence**: Confirmed as compatible with the existing decisions DEC-001 (protocol compatibility over vendor hardcoding) and DEC-006 (Zhipu as example only).

## Decisions

### DEC-001: Default key storage is an environment-variable reference

- **Status**: confirmed
- **Decision**: The wizard MUST default to storing the API key as `api_key_env` (the name of an environment variable). Inline plaintext storage as `api_key` MUST be supported only as an explicit opt-in, accompanied by a warning that the value will be written in plaintext to `~/.foxharness/settings.json`.
- **Alternatives Rejected**: Storing the plaintext key by default; supporting only `api_key_env` with no inline fallback; adopting a full SecretRef model (env/file/exec) like the openclaw reference project.
- **Reason**: Defaulting to `api_key_env` keeps secrets off disk and reuses the existing provider resolution path; the opt-in inline path preserves convenience for users who prefer it, with informed consent. A full SecretRef model was judged out of scope for v1.
- **User Evidence**: The user selected "方案 1 (preflight) + 默认 api_key_env、内联作带警告的可选项."

### DEC-002: Reuse existing persistence location and shape

- **Status**: confirmed
- **Decision**: The wizard MUST persist profiles into the existing `~/.foxharness/settings.json` under `llm.providers` and `llm.default_provider`. A separate credential file MUST NOT be introduced.
- **Alternatives Rejected**: Splitting credentials into a dedicated store (as the openclaw reference project does with its auth-profiles store).
- **Reason**: foxharness already resolves configuration from `~/.foxharness/settings.json`; reusing it avoids a new secret-management surface and keeps the change small.
- **User Evidence**: Accepted in the stage summary ("都认可").

### DEC-003: Subcommand form and v1 action set

- **Status**: confirmed
- **Decision**: The feature MUST be exposed as a `fox config` subcommand, dispatched on `args[0]` consistent with the existing `exec` and `autodev` subcommands. The v1 action set MUST be: add (the guided wizard), list, and set-default.
- **Alternatives Rejected**: An auto-launched wizard that intercepts `fox` startup; a nested `fox config add` verb structure.
- **Reason**: The user chose a standalone subcommand plus an improved error over an auto-prompt, to avoid intercepting non-interactive/scripted startup.
- **User Evidence**: The user selected "独立子命令 + 改进报错" for the entry-point shape.

### DEC-004: v1 scope boundary

- **Status**: confirmed
- **Decision**: v1 MUST exclude editing and removing existing profiles and MUST exclude a non-interactive mode. These are deferred to later versions.
- **Alternatives Rejected**: Shipping edit/remove and `--non-interactive` in v1.
- **Reason**: Keep the first increment focused on the core onboarding pain (convenient add + friendly first-run guidance).
- **User Evidence**: The user approved the assumptions ("假设都认可") and chose to defer non-interactive mode ("v1不做非交互").

### DEC-005: v1 preset catalog membership

- **Status**: confirmed
- **Decision**: The v1 built-in preset catalog MUST contain exactly the following twelve providers. Each preset pre-fills protocol and a suggested `api_key_env` (names sourced from the openclaw reference project's catalog); base URL and default model are finalized during specification. A fully-custom entry remains available outside this list.

  | Provider id | Protocol | Suggested `api_key_env` |
  |---|---|---|
  | `openai` | openai | `OPENAI_API_KEY` |
  | `anthropic` | claude | `ANTHROPIC_API_KEY` |
  | `xai` | openai | `XAI_API_KEY` |
  | `mistral` | openai | `MISTRAL_API_KEY` |
  | `groq` | openai | `GROQ_API_KEY` |
  | `openrouter` | openai | `OPENROUTER_API_KEY` |
  | `zhipu` | openai | `ZHIPU_API_KEY` |
  | `deepseek` | openai | `DEEPSEEK_API_KEY` |
  | `moonshot` | openai | `MOONSHOT_API_KEY` |
  | `qwen` | openai | `DASHSCOPE_API_KEY` |
  | `minimax` | openai | `MINIMAX_API_KEY` |
  | `ollama` | openai | (none — local, `auth: "none"`) |

- **Alternatives Rejected**: Shipping the full 60+ provider catalog from the openclaw reference project; a smaller Zhipu-only set.
- **Reason**: Twelve covers the existing Zhipu example plus mainstream international, mainstream Chinese, and one local entry, keeping the catalog practical without over-growing v1.
- **User Evidence**: The user selected "就按这 12 个" for the proposed set (OpenAI, Anthropic, xAI, Mistral, Groq, OpenRouter, Zhipu, DeepSeek, Moonshot, Qwen, MiniMax, Ollama).

## Out of Scope

### OUT-001: Editing and removing existing profiles

- **Status**: confirmed
- **Statement**: Modifying or deleting an already-persisted provider profile is out of scope for v1.
- **Reason**: Deferred per DEC-004 to keep v1 focused.
- **User Evidence**: Approved in the stage summary assumptions.

### OUT-002: Non-interactive configuration mode

- **Status**: confirmed
- **Statement**: A scripted/CI non-interactive mode (e.g., `fox config add --provider … --non-interactive`) is out of scope for v1.
- **Reason**: Deferred per DEC-004.
- **User Evidence**: The user chose "v1不做非交互."

### OUT-003: Wizard internationalization

- **Status**: confirmed
- **Statement**: Wizard copy is English for v1; localization of the wizard prompts is out of scope.
- **Reason**: Match the existing codebase/documentation language; revisit i18n later.
- **User Evidence**: Approved in the stage summary assumptions.

## Open Questions

### OPEN-001: Exact preset catalog membership

- **Status**: resolved
- **Resolved By**: DEC-005
- **Note**: The v1 catalog membership is confirmed as the twelve providers in DEC-005. Per-preset base URL and default model remain to be finalized during specification.

## Superseded Entries

None.

## Confirmation Log

### Session 2026-06-29 20:50:00 CST

- **Summary Presented**: Standalone `fox config` subcommand (guided add wizard) + improved first-run guidance; default `api_key_env` key storage with opt-in inline plaintext (warned); preflight validation of the env var before saving; built-in multi-vendor preset catalog with a fully-custom entry; live connectivity probe at end of wizard (skippable); v1 actions = add + list + set-default; edit/remove and non-interactive mode deferred; wizard copy in English for v1; persistence reuses `~/.foxharness/settings.json`.
- **User Confirmation**: "① 都没问题；② 选 A，v1不做非交互；③ 假设都认可，写入吧"
- **Entries Confirmed**: NEED-001, NEED-002, NEED-003, NEED-004, NEED-005, CON-001, CON-002, DEC-001, DEC-002, DEC-003, DEC-004, OUT-001, OUT-002, OUT-003

### Session 2026-06-29 21:05:00 CST

- **Summary Presented**: Reviewed the openclaw reference project's provider catalog and proposed a curated twelve-provider v1 preset set (OpenAI, Anthropic, xAI, Mistral, Groq, OpenRouter, Zhipu, DeepSeek, Moonshot, Qwen, MiniMax, Ollama) covering mainstream international, mainstream Chinese, and one local entry.
- **User Confirmation**: "就按这 12 个"
- **Entries Confirmed**: DEC-005 (and OPEN-001 resolved)
