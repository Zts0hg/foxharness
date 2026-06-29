# Implementation Plan: Interactive LLM Provider Onboarding

<!--
Language: Generated in English per .codexspec/config.yml (language.document: en).
-->

**Related Spec**: `.codexspec/specs/2026-0629-2045a9-llm-config-onboarding/spec.md`
**Confirmed Requirements**: `.codexspec/specs/2026-0629-2045a9-llm-config-onboarding/requirements.md`
**Created**: 2026-06-29
**Status**: Draft

## Context

The provider-configuration feature removed the implicit Zhipu default, so a foxharness with no configured provider now fails at startup with `resolve LLM configuration: missing LLM protocol`. That error is accurate but offers no remediation, and there is no way to create a configuration without hand-editing `~/.foxharness/settings.json`. This plan adds an interactive `fox config` subcommand and replaces the bare empty-configuration error with actionable guidance.

All implementation is built on top of existing, verified machinery:

- `internal/llmconfig.Resolve` already resolves and validates provider configuration from settings, environment, and CLI inputs, returning field-specific errors (`missing LLM protocol`, `missing LLM base_url`, ...).
- `internal/settings.Load` / `Save` already read and atomically write `~/.foxharness/settings.json` at `0600`, preserving unknown fields via a raw-merge path; `SetProviderModel` shows the existing profile-mutation helper pattern.
- `internal/llmresolve.FromUserSettings` is the bridge used by `cmd/fox`; it wraps resolution errors as `resolve LLM configuration: %w`.
- `internal/provider.NewProvider(llmconfig.ResolvedConfig)` builds a protocol-based (`openai`/`claude`) provider from a resolved config — reused unchanged for the connectivity probe.
- `cmd/fox/main.go` dispatches subcommands on `args[0]` (`exec`, `autodev`) and resolves LLM config unconditionally before launching.

## Goals / Non-Goals

**Goals:**

- Add a `fox config` subcommand with a guided wizard for adding a provider profile, plus list and set-default actions.
- Replace the bare empty-configuration error with an onboarding message that names `fox config`.
- Ship a curated twelve-provider preset catalog that pre-fills connection fields, plus a fully-custom entry.
- Catch mistakes before the wizard exits via env-var preflight and an optional connectivity probe.

**Non-Goals:**

- Non-interactive / scriptable configuration mode (OUT-002).
- Editing or removing existing profiles (OUT-001).
- Wizard localization (OUT-003).
- A separate credential store; secrets stay in `~/.foxharness/settings.json`.

## Tech Stack

- **Language**: Go (module `github.com/Zts0hg/foxharness`).
- **TUI/prompt**: line-based I/O via `bufio` for the wizard; `golang.org/x/term` (already a transitive dependency) for TTY detection and no-echo secret input.
- **Persistence**: existing `internal/settings` JSON store (atomic write, `0600`, unknown-field-preserving).
- **LLM probe**: existing `internal/provider.NewProvider` + `LLMProvider.Generate`.

## Architecture Overview

The feature is a thin interactive layer over existing resolution, persistence, and provider-construction code. It introduces one new package, `internal/configcmd`, and small additive changes to `internal/llmconfig`, `internal/settings`, and `cmd/fox`.

```
                 ┌─────────────────────────── cmd/fox/main.go ───────────────────────────┐
                 │  args[0]=="config" ──► launchConfig ──► configcmd.Run(...)  (no LLM   │
                 │                                            resolve needed)             │
                 │  otherwise ──► resolveLLMConfig ──► on ErrNoProviderConfigured print  │
                 │                                  onboarding message (fox config)       │
                 └──────────────┬────────────────────────────────────┬───────────────────┘
                                │                                     │
              ┌─────────────────▼───────────────┐      ┌──────────────▼──────────────────┐
              │       internal/configcmd        │      │      internal/llmconfig         │
              │  catalog.go   12 Presets (data) │      │  ErrNoProviderConfigured        │
              │  prompter.go  Prompter iface    │      │  + hasNoConfigInput detection   │
              │  wizard.go    add/list/default, │      │   in Resolve (empty input only) │
              │               preflight, probe  │      └─────────────────────────────────┘
              └──────┬──────────────┬───────────┘
                     │              │  reads/writes
        injects      │              ▼
   ProviderFactory ──┤     internal/settings
   (for probe)       │     SetProvider (upsert) / SetDefaultProvider
                     ▼     Save (existing, 0600, raw-preserve)
   internal/provider.NewProvider  ──►  LLMProvider.Generate (probe)
```

**Covers**: REQ-001, REQ-003, REQ-006, REQ-008, REQ-011, REQ-012

## Component Structure

```
cmd/fox/
  main.go                 # add launchConfig; dispatch; onboarding message; non-TTY guard
  main_test.go            # extend: config dispatch + onboarding path
internal/configcmd/       # NEW package
  doc.go                  # package doc
  catalog.go              # Catalog: twelve Presets as plain data + custom entry
  catalog_test.go
  prompter.go             # Prompter interface + bufio-backed implementation
  wizard.go               # Run, addFlow, listFlow, setDefaultFlow, preflight, probe
  wizard_test.go
  onboarding.go           # OnboardingMessage() string  (actionable guidance text)
  onboarding_test.go
internal/llmconfig/
  config.go               # add ErrNoProviderConfigured + hasNoConfigInput guard in Resolve
  empty_config_test.go    # NEW: empty vs partial input behavior
internal/settings/
  settings.go             # add SetProvider (upsert) and SetDefaultProvider (validates id)
  providers_test.go       # extend (or settings_test.go)
internal/provider/        # unchanged; NewProvider reused for the probe
```

## Data Models

### Preset (template data, `internal/configcmd/catalog.go`)

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| ID | string | preset and default profile id | non-empty; ASCII kebab |
| Protocol | string | adapter selector | `openai` or `claude` only |
| BaseURL | string | pre-filled API base URL | non-empty; finalized per vendor at implementation (A-1) |
| Model | string | pre-filled default model | non-empty; finalized per vendor at implementation (A-1) |
| Auth | string | auth mode | `api-key` (default) or `none` (`ollama`) |
| APIKeyEnv | string | suggested env-var name | empty when `Auth == "none"` |

The catalog is the twelve confirmed providers (DEC-005): `openai`, `anthropic`, `xai`, `mistral`, `groq`, `openrouter`, `zhipu`, `deepseek`, `moonshot`, `qwen`, `minimax`, `ollama`. `anthropic` uses protocol `claude`; `ollama` uses `auth: "none"`; the rest use protocol `openai`. Exact base URLs and default models are not fixed by the confirmed requirements (A-1) and are populated during implementation.

### Provider Profile (existing `llmconfig.Profile`)

The wizard persists `llmconfig.Profile` under `llm.providers.<id>`: `Protocol`, `BaseURL`, `Model`, `Auth`, `APIKeyEnv`, and (only when the user opts in) `APIKey`. No schema change is required.

## Internal Contracts

### `llmconfig.Resolve` empty-input guard

```go
// ErrNoProviderConfigured is returned when no provider configuration is
// supplied from settings, environment, or CLI inputs.
var ErrNoProviderConfigured = errors.New("no LLM provider configured")

// hasNoConfigInput reports whether the resolution inputs are entirely empty.
func hasNoConfigInput(s Settings, env EnvOverrides, cli CLIOverrides) bool {
    return s.DefaultProvider == "" && len(s.Providers) == 0 &&
        env == EnvOverrides{} && cli == CLIOverrides{}
}
```

At the top of `Resolve`, before provider selection: if `hasNoConfigInput(...)` → return `ErrNoProviderConfigured`. All other paths (including a present-but-incomplete profile that still has an empty protocol) keep their existing field-specific errors, satisfying REQ-003 and the "partial keeps specific error" edge case.

### `cmd/fox` onboarding path

```go
// launchConfig joins launchTUI/launchPrint/launchAutodev.
if mode == launchConfig {
    deps := configcmd.Deps{
        HomeDir:     homeDir,
        Env:         os.Getenv,
        Stdin:       os.Stdin,
        Stdout:      os.Stdout,
        Stderr:      os.Stderr,
        Interactive: term.IsTerminal(int(os.Stdin.Fd())),
    }
    if err := configcmd.Run(ctx, deps, subArgs); err != nil {
        exitWithError(err)
    }
    return
}
// ... resolveLLMConfig ...
if err != nil {
    if errors.Is(err, llmconfig.ErrNoProviderConfigured) {
        fmt.Fprintln(os.Stderr, configcmd.OnboardingMessage())
        os.Exit(1)
    }
    exitWithError(err)
}
```

`configcmd.Run` is called before `resolveLLMConfig`, because the wizard does not need a resolved provider. `interactive` is computed in `main` via `term.IsTerminal(int(os.Stdin.Fd()))` and passed in, keeping the wizard free of TTY I/O for testability.

### `configcmd.Run` entry shape

```go
type Deps struct {
    HomeDir   string
    Env       llmconfig.EnvLookup
    Stdin     io.Reader
    Stdout    io.Writer
    Stderr    io.Writer
    Interactive bool
    NewProvider ProviderFactory // injectable for tests (default: provider.NewProvider)
}

type ProviderFactory func(llmconfig.ResolvedConfig) (provider.LLMProvider, error)

func Run(ctx context.Context, deps Deps, subArgs []string) error
```

`subArgs` selects the action: `add`, `list`, or `default [id]`. With no recognized sub-arg, the wizard presents an action menu (PD-5).

### Connectivity probe

```go
func probe(ctx context.Context, cfg llmconfig.ResolvedConfig, factory ProviderFactory) error {
    p, err := factory(cfg)
    if err != nil { return err }
    ctx, cancel := context.WithTimeout(ctx, probeTimeout) // ~20s
    defer cancel()
    _, err = p.Generate(ctx, []schema.Message{{Role: schema.RoleUser, Content: "ping"}}, nil)
    return err
}
```

### New `internal/settings` helpers

```go
// SetProvider upserts a provider profile under llm.providers.
func SetProvider(s *Settings, id string, profile llmconfig.Profile) error

// SetDefaultProvider sets llm.default_provider after verifying the id exists.
func SetDefaultProvider(s *Settings, id string) error
```

Both reuse the existing `Save` path (atomic, `0600`, raw-merge preservation).

## Decisions

### Decision 1: New `internal/configcmd` package for the wizard

**Context**: The wizard is a distinct concern from agent execution and should not enlarge `cmd/fox` or `internal/app`.

**Options Considered**: (1) a new `internal/configcmd` package; (2) implement directly inside `cmd/fox/main.go`.

**Decision**: New `internal/configcmd` package, with `cmd/fox` only dispatching to it.

**Rationale**: Keeps `cmd/fox` thin, isolates interactive logic and the catalog as plain data, and makes the wizard unit-testable without spinning up the CLI. Follows the constitution's separation-of-concerns principle.

**Covers**: REQ-001, REQ-006, REQ-012, NFR-002

**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

### Decision 2: Empty-config detection via a sentinel error in `llmconfig`

**Context**: The onboarding message must fire only when configuration is entirely empty; partially-configured profiles must keep their specific field errors.

**Options Considered**: (1) a sentinel `ErrNoProviderConfigured` returned by `Resolve` for empty input, detected in `cmd/fox`; (2) string-matching on `missing LLM protocol` in `cmd/fox`.

**Decision**: Sentinel error + `hasNoConfigInput` guard at the top of `Resolve`. `cmd/fox` matches it with `errors.Is` and prints `configcmd.OnboardingMessage()`.

**Rationale**: A sentinel is precise and testable; string-matching is fragile and would also catch the partial "protocol present but empty" path. The guard is placed before provider selection so it intercepts only the truly-empty case, preserving all existing field-specific errors unchanged.

**Trade-off / migration**: Existing tests that feed entirely-empty input and assert `missing LLM protocol` must be updated to expect `ErrNoProviderConfigured`. This is the intended, faithful replacement of the bare error (NEED-002); partial-input tests are unaffected.

**Covers**: REQ-003, REQ-011, NFR-002

**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

### Decision 3: Line-based prompter over `bufio`, behind an interface

**Context**: The wizard needs prompts, selects, confirms, and a no-echo secret read. The project already uses `bubbletea` for the TUI.

**Options Considered**: (1) a small `Prompter` interface backed by `bufio` + `golang.org/x/term`; (2) build the wizard in `bubbletea`.

**Decision**: A small `Prompter` interface (`ReadLine`, `ReadSecret`, `Select`, `Confirm`) with a `bufio`/`term` implementation. Tests inject a fake.

**Rationale**: The wizard is a short linear flow; `bubbletea` is heavier than needed and harder to drive in unit tests. An interface gives the testability required by NFR-002 without a new heavy dependency (`golang.org/x/term` is already transitive).

**Covers**: REQ-004, REQ-005, REQ-010, NFR-001, NFR-002

**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

### Decision 4: Connectivity probe through an injected provider factory

**Context**: The probe must send a real minimal request yet remain unit-testable without network calls.

**Decision**: The wizard takes a `ProviderFactory` (default `provider.NewProvider`). The probe builds the provider, calls `Generate` with a one-token user message and a short timeout, and returns/surfaces the error. Tests pass a fake factory.

**Rationale**: Reuses the existing protocol-based provider construction; the factory seam satisfies NFR-002. The probe is advisory and skippable, so a failure does not block saving.

**Covers**: REQ-007, NFR-002

**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

### Decision 5: Config sub-actions: explicit verbs with a no-arg menu

**Context**: `fox config` must support add, list, and set-default (REQ-002) without designing a nested parser.

**Decision**: `fox config [add|list|default [id]]`. A recognized first sub-arg runs that action directly; no recognized sub-arg opens an interactive action menu.

**Rationale**: Direct verbs are scriptable-friendly and discoverable for repeat use; the no-arg menu serves first-time users. No new product behavior is introduced beyond the confirmed v1 action set.

**Covers**: REQ-002

**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

### Decision 6: Persistence via new `SetProvider` / `SetDefaultProvider` helpers

**Context**: The wizard writes a new/updated profile and optionally the default, reusing the existing raw-preserving `Save`.

**Decision**: Add `settings.SetProvider` (upsert, creates the providers map if nil) and `settings.SetDefaultProvider` (validates the id exists, mirroring `SetProviderModel`). Overwrite-on-duplicate confirmation stays in the wizard (assumption A-2), not in the helper.

**Rationale**: Mirrors the existing `SetProviderModel` pattern; reuses `Save`'s `0600` + unknown-field preservation (so REQ-009 and RA-001 are satisfied by the existing write path).

**Covers**: REQ-008, REQ-009, NFR-001

**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

### Decision 7: Non-TTY guard via `golang.org/x/term`, flag passed into the wizard

**Context**: MIN-001 requires a clean failure when `fox config` runs without an interactive terminal (OUT-002 defers a scripted mode).

**Decision**: `cmd/fox` computes `interactive := term.IsTerminal(int(os.Stdin.Fd()))` and passes it to `configcmd.Run`. When false, the wizard exits with a clear "interactive terminal required" message.

**Rationale**: Promotes an already-transitive dependency to direct use (no new dependency); keeps the TTY check out of the pure wizard logic so it stays testable.

**Covers**: REQ-001, Edge Cases (non-interactive stdin)

**Decision Level**: Plan-level technical decision; does not change confirmed product scope.

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Existing tests assert `missing LLM protocol` for empty input | High | Low | Update those tests to expect `ErrNoProviderConfigured`; partial-input tests unchanged. Called out in Decision 2. |
| Preset base URL / model values are wrong | Medium | Medium | Populate from each vendor's published OpenAI-/Claude-compatible endpoint; the connectivity probe (Phase 5) will reveal mistakes before release. |
| Inline API key stored as plaintext | Low (opt-in only) | Medium | Only after explicit warning + confirm (NFR-001); file is already `0600` via `Save`; secret never echoed/logged. |
| Probe costs a real (tiny) request / hangs | Low | Low | Short timeout (~20s), skippable, advisory only. |
| `golang.org/x/term` import promotion | Low | Low | Already transitive; no version change expected. |

## Implementation Phases

Each phase follows Red → Green → Refactor per the constitution. Tests are written first and must fail for the expected reason before implementation.

### Phase 1 — Onboarding error path

- [ ] Test: `Resolve` returns `ErrNoProviderConfigured` for entirely-empty input; partial input still returns field-specific errors (`internal/llmconfig/empty_config_test.go`).
- [ ] Implement `ErrNoProviderConfigured` + `hasNoConfigInput` guard in `Resolve`.
- [ ] Update any existing tests that asserted `missing LLM protocol` for empty input.
- [ ] Test: `configcmd.OnboardingMessage()` names `fox config` and mentions no vendor.
- [ ] Wire `cmd/fox` to print the onboarding message on `errors.Is(err, ErrNoProviderConfigured)`.

**Covers**: REQ-003, REQ-011

### Phase 2 — Settings helpers + persistence

- [ ] Test: `SetProvider` upserts a profile and creates the providers map if nil (`internal/settings`).
- [ ] Test: `SetDefaultProvider` sets the default and rejects an unknown id.
- [ ] Test: writing a new profile via `Save` preserves unrelated fields and creates the file at `0600` when missing.
- [ ] Implement `SetProvider` and `SetDefaultProvider`.

**Covers**: REQ-008, REQ-009, REQ-012 (data path)

### Phase 3 — Catalog + add flow

- [ ] Test: `Catalog` contains exactly the twelve confirmed ids; `anthropic`→`claude`, `ollama`→`auth:"none"`, others→`openai` (`internal/configcmd/catalog_test.go`).
- [ ] Test: `Prompter` interface with a fake; selecting a preset pre-fills protocol/base URL/model/`api_key_env` (all editable); custom entry collects each field and rejects an unsupported protocol.
- [ ] Implement `catalog.go`, `prompter.go`, and the add flow's field collection in `wizard.go`.

**Covers**: REQ-002 (add), REQ-006, REQ-010, NFR-003

### Phase 4 — Key handling + preflight

- [ ] Test: default key source is `api_key_env`; preflight warns when the env var is unset and offers inline entry.
- [ ] Test: inline entry is accepted only after a warning + confirm; the entered secret is not echoed back; declining inline still allows saving the env-var reference flagged as unset.
- [ ] Implement the key step, preflight, and no-echo secret read.

**Covers**: REQ-004, REQ-005, NFR-001

### Phase 5 — Connectivity probe

- [ ] Test: with an injected fake factory, the probe reports success; with a failing factory it reports the reason; the probe is skippable and a failure does not block save.
- [ ] Implement `probe` with a short timeout and wire it into the add flow as an opt-in/skippable step.

**Covers**: REQ-007, NFR-002

### Phase 6 — List, set-default, action wiring, non-TTY guard

- [ ] Test: `list` prints saved profile ids and marks the default; `default <id>` updates `llm.default_provider`.
- [ ] Test: no-arg `fox config` opens the action menu; `add`/`list`/`default` sub-args run the matching action.
- [ ] Test: when `Interactive` is false, `Run` exits with a clear "interactive terminal required" message.
- [ ] Implement list, set-default, the sub-arg/menu dispatch, and the non-TTY guard.

**Covers**: REQ-001, REQ-002 (list/default), Edge Cases (non-interactive stdin)

### Phase 7 — Integration, docs, polish

- [ ] End-to-end manual run of `fox config` against a preset and a custom provider.
- [ ] Update `README.*` to document `fox config`, the preset list, key storage, and the new first-run guidance.
- [ ] `gofmt -w .`, `go vet ./...`, `go test ./...` green. (Docs/config are direct edits per the conditional-TDD rule; code paths are test-first.)

**Covers**: REQ-001, REQ-006, NFR-001

## Security Considerations

- Inline `api_key` is persisted only after an explicit plaintext warning and confirmation (NFR-001, Decision 6). `~/.foxharness/settings.json` is already written with `0600` permissions by the existing `Save` (RA-001 mitigated).
- The wizard never echoes or logs the full secret; secret input uses no-echo (`term.ReadPassword`).
- The connectivity probe sends only a minimal `ping` message; the API key travels only through the existing provider transport, never through wizard logs.

## Performance Considerations

- The wizard is interactive; per-step latency is irrelevant.
- The connectivity probe is bounded by a short timeout and is skippable; it issues a single minimal request.
- Settings I/O is one small JSON read/write per wizard action.

## Requirements Coverage

| Spec Requirement | Plan Coverage | Reference |
|------------------|---------------|-----------|
| REQ-001 | Full | Architecture; Decision 1, 7; Phase 1, 6 |
| REQ-002 | Full | Decision 5; Phase 3 (add), Phase 6 (list/default) |
| REQ-003 | Full | Decision 2; Internal Contracts; Phase 1 |
| REQ-004 | Full | Decision 3; Phase 4 |
| REQ-005 | Full | Decision 3, 6; Phase 4 |
| REQ-006 | Full | Data Models (Preset); Decision 1; Phase 3 |
| REQ-007 | Full | Decision 4; Internal Contracts (probe); Phase 5 |
| REQ-008 | Full | Decision 6; Internal Contracts; Phase 2 |
| REQ-009 | Full | Decision 6 (reuses `Save` `0600` + raw-preserve); Phase 2 |
| REQ-010 | Full | Decision 3; Phase 3 |
| REQ-011 | Full | Decision 2 (guard); Constraints; Phase 1 |
| REQ-012 | Full | Data Models (catalog is plain data); Decision 1; Phase 2/3 |
| NFR-001 | Full | Decision 3, 6; Security; Phase 4 |
| NFR-002 | Full | Decision 1, 3, 4 (injected deps); Verification across all phases |
| NFR-003 | Full | Data Models (Preset); Decision 1; Phase 3 |
