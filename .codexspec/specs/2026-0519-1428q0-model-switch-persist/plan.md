# Implementation Plan: Model Switch Persistence

## Context

The fox CLI currently hardcodes `"glm-4.5-air"` as the default model in `cmd/fox/main.go:97`. The `/model` command in the TUI switches models in-memory but the choice is lost when the session ends. This plan adds a settings persistence layer so the user's model selection survives across sessions, following a four-level priority system inspired by Claude Code.

## Goals / Non-Goals

**Goals:**

- Persist the user's model choice to `~/.foxharness/settings.json` on `/model <name>`
- Resolve model at startup via priority: CLI `--model` > `FOX_MODEL` env > settings.json > `"glm-4.5-air"`
- Add a new `internal/settings` package with testable, injectable API
- Maintain full backward compatibility (no settings file = existing behavior)

**Non-Goals:**

- Persisting provider, thinking, plan mode, or other flags
- Project-level settings
- Model alias system or allowlist
- Model validation beyond what the provider already does

## Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| 1. TDD | ✅ | Plan requires writing tests for `internal/settings` before implementation. All 11 test cases from spec are covered. |
| 2. Code Quality | ✅ | `internal/settings` has single responsibility (persistence). Dependencies (`homeDir` string) are injectable. |
| 3. Go Documentation | ✅ | New package needs `doc.go` with block comments on all exported identifiers. |
| 4. Testing Standards | ✅ | Tests mirror package structure. Table-driven tests for priority resolution. Edge cases covered. |
| 5. Architecture | ✅ | Clean separation: `internal/settings` handles I/O only, callers handle resolution logic. Small interface. |
| 6. Performance | ✅ | Single file read at startup (< 5ms). No hot-path impact. |
| 7. Security | ✅ | 0600 file permissions. Input validation for malformed JSON. |

## Decisions

### Decision 1: Use json.RawMessage for forward-compatible settings

**Context**: The spec requires preserving unknown fields when rewriting settings.json (REQ-002 edge case).
**Decision**: Use `json.RawMessage` to capture the full JSON object, unmarshal known fields into a typed struct, and re-marshal the original raw bytes with updated fields.
**Rationale**: `json.RawMessage` preserves whitespace, field order, and unknown fields without requiring a map-based approach. It integrates naturally with Go's `encoding/json`.

### Decision 2: Resolve model at config construction time, not at engine run time

**Context**: Model could be resolved in `NewAgentRunner()` or lazily in `Run()`.
**Decision**: Resolve once in `cmd/fox/main.go` before passing to `RunTUI`/`RunCLI`. The resolved model becomes the `cfg.Model` value.
**Rationale**: Keeps the settings package out of `internal/app`. The runner remains unaware of settings — it just receives a model string. Simpler dependency graph, easier to test.

### Decision 3: Default model constant lives in cmd/fox/main.go

**Context**: The spec's `ResolveModel` function needs a built-in default. Where should `"glm-4.5-air"` be defined?
**Decision**: Keep `"glm-4.5-air"` as the flag default in `cmd/fox/main.go`. `settings.ResolveModel` accepts it as a parameter.
**Rationale**: The default is a CLI concern, not a settings concern. This avoids coupling the settings package to a specific model name.

### Decision 4: Persistence happens via a callback in runner.SetModel

**Context**: The TUI calls `runner.SetModel()`. We need to also write to settings.json.
**Decision**: Add an `onModelChange func(model string) error` callback to `AgentRunnerConfig`. The TUI wiring passes `settings.Save` wrapped in a closure. `SetModel` calls this callback after a successful switch.
**Rationale**: Keeps `internal/app` decoupled from `internal/settings`. The runner doesn't know about settings files — it just calls a hook. Testable by injecting a mock callback.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     cmd/fox/main.go                       │
│                                                           │
│  parseArgs()                                              │
│      │                                                    │
│      ▼                                                    │
│  settings.Load(homeDir)  ──►  Settings{Model: "..."}      │
│      │                                                    │
│      ▼                                                    │
│  settings.ResolveModel(cliFlag, envVar, settings)         │
│      │                                                    │
│      ▼                                                    │
│  cfg.Model = resolvedModel                                │
│      │                                                    │
│      ├─► app.RunTUI(ctx, cfg, onSave)                     │
│      └─► app.RunCLI(ctx, cfg)                             │
└──────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────┐
│                   internal/settings                       │
│                                                           │
│  Settings struct { Model string }                         │
│  Load(homeDir)    → reads ~/.foxharness/settings.json     │
│  Save(homeDir, s) → writes atomically                     │
│  ResolveModel(cliFlag, envVar, defaultModel, s) → string  │
└──────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────┐
│                   internal/app/runner.go                  │
│                                                           │
│  AgentRunnerConfig.OnModelChange func(string) error       │
│                                                           │
│  SetModel(model):                                         │
│      1. Create new provider with model                    │
│      2. Update r.model and r.llmProvider                  │
│      3. Call r.onModelChange(model)  ◄── new callback     │
└──────────────────────────────────────────────────────────┘
```

### Module Dependency Graph

```
cmd/fox/main.go
    ├── internal/settings    (load + resolve model)
    └── internal/app         (run TUI/CLI)
            └── internal/provider
            └── internal/tui
                    └── Runner interface
```

`internal/app` does NOT depend on `internal/settings`. The wiring happens only in `cmd/fox/main.go`.

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| Malformed settings.json breaks startup | Low — graceful fallback to default | `Load()` returns zero-value Settings on any parse error; logs warning |
| Concurrent fox processes write settings | Low — last-write-wins is acceptable | Atomic rename prevents corruption; no locking needed |
| Future settings fields need migration | Low — RawMessage preserves unknowns | Read-modify-write pattern preserves all fields |

## Implementation Phases

### Phase 1: Foundation — `internal/settings` package (TDD)

**Red → Green → Refactor for each behavior.**

- [ ] Create `internal/settings/doc.go` with package documentation
- [ ] Write tests for `Load()` — missing file, valid file, malformed JSON, empty model field
- [ ] Write tests for `Save()` — creates directory, atomic write, preserves unknown fields, 0600 permissions, graceful handling of write permission failures
- [ ] Write tests for `ResolveModel()` — all 4 priority levels, empty string handling
- [ ] Implement `Settings` struct with `json.RawMessage` for forward compatibility
- [ ] Implement `Load()` — read and parse with graceful error handling
- [ ] Implement `Save()` — atomic write via temp file + `os.Rename`; output file permissions set to 0600; returns error on write failure (caller logs warning and continues)
- [ ] Implement `ResolveModel()` — four-level priority cascade; accepts `defaultModel` as parameter (not a package constant)

### Phase 2: Integration — Wire into CLI startup

- [ ] Modify `cmd/fox/main.go`: call `settings.Load()` + `settings.ResolveModel()` before passing config
- [ ] Read `FOX_MODEL` from `os.Getenv("FOX_MODEL")`
- [ ] Remove hardcoded `"glm-4.5-air"` flag default; use resolved model instead
- [ ] Ensure `--model` flag still works as highest-priority override

### Phase 3: Integration — Wire persistence into TUI `/model`

- [ ] Add `OnModelChange func(string) error` field to `AgentRunnerConfig`
- [ ] Modify `SetModel()` to call `onModelChange` callback after successful switch
- [ ] Wire the callback in `cmd/fox/main.go` to call `settings.Save()`
- [ ] Pass callback through `RunTUI` → `NewAgentRunner`
- [ ] Verify `/model <name>` persists immediately and `/model` (no args) shows current model

### Phase 4: Edge cases and verification

- [ ] Test: startup with no settings file → default model used
- [ ] Test: startup with settings file → persisted model used
- [ ] Test: `--model` flag → overrides persisted model, doesn't write settings
- [ ] Test: `FOX_MODEL` env → overrides settings, doesn't write settings
- [ ] Test: `/model` in TUI → writes settings, next session uses new model
- [ ] Test: malformed settings.json → graceful fallback
- [ ] Run `go test ./...` to verify no regressions

## Files to Create or Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/settings/doc.go` | Create | Package documentation |
| `internal/settings/settings.go` | Create | `Settings` struct, `Load`, `Save`, `ResolveModel` |
| `internal/settings/settings_test.go` | Create | Table-driven tests for all behaviors |
| `cmd/fox/main.go` | Modify | Wire settings load/resolve at startup; add `FOX_MODEL` env; add save callback |
| `internal/app/runner.go` | Modify | Add `OnModelChange` callback to config; call in `SetModel` |
| `internal/app/tui.go` | Modify | Pass callback through to runner |
