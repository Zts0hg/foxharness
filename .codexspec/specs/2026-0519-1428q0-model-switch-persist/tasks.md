# Task Breakdown: Model Switch Persistence

## Overview
Total tasks: 15
Parallelizable tasks: 6
Estimated phases: 4

## Phase 1: Foundation — `internal/settings` package (TDD)

### Task 1.1: Create settings package with doc.go
- **Type**: Setup
- **Files**: `internal/settings/doc.go`
- **Description**: Create the `internal/settings` package directory and `doc.go` with package-level block comment describing the package's responsibility: reading, writing, and resolving user settings from `~/.foxharness/settings.json`.
- **Dependencies**: None
- **Est. Complexity**: Low

### Task 1.2: Write tests for Load()
- **Type**: Testing (Red)
- **Files**: `internal/settings/settings_test.go`
- **Description**: Write table-driven tests for `Load(homeDir string) (*Settings, error)` covering:
  - Missing settings file → returns zero-value `Settings`, nil error
  - Valid `{"model": "glm-4-plus"}` → returns `Settings{Model: "glm-4-plus"}`
  - Malformed JSON → returns zero-value `Settings`, nil error (graceful)
  - Empty model field `{"model": ""}` → returns `Settings{Model: ""}`
  - File with extra unknown fields → `Settings.Model` parsed correctly
  - Non-existent directory → returns zero-value `Settings`, nil error
- **Dependencies**: Task 1.1
- **Est. Complexity**: Medium

### Task 1.3: Implement Load()
- **Type**: Implementation (Green)
- **Files**: `internal/settings/settings.go`
- **Description**: Implement `Settings` struct with `Model string` field and `json.RawMessage` for forward compatibility. Implement `Load(homeDir string) (*Settings, error)` to read `~/.foxharness/settings.json`, parse with graceful error handling (returns zero-value on any error). All tests from Task 1.2 must pass.
- **Dependencies**: Task 1.2
- **Est. Complexity**: Medium

### Task 1.4: Write tests for Save()
- **Type**: Testing (Red)
- **Files**: `internal/settings/settings_test.go`
- **Description**: Write table-driven tests for `Save(homeDir string, s *Settings) error` covering:
  - Creates `~/.foxharness/` directory if missing
  - Writes valid JSON with correct `model` field
  - Output file has 0600 permissions
  - Preserves unknown fields from existing file (read-modify-write)
  - Atomic write: temp file used, then renamed
  - Write to read-only directory → returns error, does not crash
- **Dependencies**: Task 1.3
- **Est. Complexity**: Medium

### Task 1.5: Implement Save()
- **Type**: Implementation (Green)
- **Files**: `internal/settings/settings.go`
- **Description**: Implement `Save(homeDir string, s *Settings) error` with:
  - Create `~/.foxharness/` directory if missing (`os.MkdirAll` with 0755)
  - Read-modify-write pattern to preserve unknown fields
  - Atomic write: write to temp file in same directory, then `os.Rename`
  - Output file permissions set to 0600 via `os.Chmod` after rename
  - All tests from Task 1.4 must pass.
- **Dependencies**: Task 1.4
- **Est. Complexity**: Medium

### Task 1.6: Write tests for ResolveModel() [P]
- **Type**: Testing (Red)
- **Files**: `internal/settings/settings_test.go`
- **Description**: Write table-driven tests for `ResolveModel(cliFlag, envVar, defaultModel string, s *Settings) string` covering all 4 priority levels:
  - TC-001: All empty + default → returns `defaultModel`
  - TC-002: Settings has model, others empty → returns `s.Model`
  - TC-003: `cliFlag` non-empty → returns `cliFlag` regardless of others
  - TC-004: `envVar` non-empty, `cliFlag` empty → returns `envVar`
  - TC-005: `cliFlag` + `envVar` both set → returns `cliFlag`
  - TC-008: Settings model is empty string → falls back to `defaultModel`
- **Dependencies**: Task 1.1
- **Est. Complexity**: Low

### Task 1.7: Implement ResolveModel()
- **Type**: Implementation (Green)
- **Files**: `internal/settings/settings.go`
- **Description**: Implement `ResolveModel(cliFlag, envVar, defaultModel string, s *Settings) string` with four-level cascade: return first non-empty value from `cliFlag`, `envVar`, `s.Model`, `defaultModel`. All tests from Task 1.6 must pass.
- **Dependencies**: Task 1.6, Task 1.3 (needs Settings struct)
- **Est. Complexity**: Low

## Phase 2: Integration — Wire into CLI startup

### Task 2.1: Wire settings into cmd/fox/main.go
- **Type**: Implementation
- **Files**: `cmd/fox/main.go`
- **Description**: Modify `main()` and `parseArgs()` to:
  1. Import `internal/settings`
  2. After `parseArgs()`, call `settings.Load(os.UserHomeDir())`
  3. Read `FOX_MODEL` from `os.Getenv("FOX_MODEL")`
  4. Call `settings.ResolveModel(cfg.Model, foxModelEnv, "glm-4.5-air", loadedSettings)`
  5. Set `cfg.Model` to the resolved model
  6. Keep `--model` flag default as `"glm-4.5-air"` for help text, but the resolved value overrides it
- **Dependencies**: Task 1.7, Task 1.5
- **Est. Complexity**: Medium

### Task 2.1b: Write tests for SetModel callback behavior [P]
- **Type**: Testing (Red)
- **Files**: `internal/app/runner_test.go`
- **Description**: Write tests for the `OnModelChange` callback behavior in `SetModel()`:
  - Nil callback → `SetModel` succeeds, no panic
  - Callback returns error → `SetModel` still succeeds (model switched), error is logged
  - Callback succeeds → `SetModel` succeeds, callback was called with correct model name
- **Dependencies**: None (tests against existing runner behavior)
- **Est. Complexity**: Low

### Task 2.2: Add OnModelChange callback to AgentRunnerConfig [P]
- **Type**: Implementation
- **Files**: `internal/app/runner.go`
- **Description**: Add `OnModelChange func(model string) error` field to `AgentRunnerConfig`. Store it as `onModelChange` in `AgentRunner`. Modify `SetModel()` to call `r.onModelChange(model)` after a successful provider switch (after lines 237-239). If `onModelChange` is nil, skip the call. If it returns an error, log a warning but do not fail the model switch.
- **Dependencies**: Task 2.1b (tests must exist first per TDD)
- **Est. Complexity**: Low

### Task 2.3: Pass OnModelChange through RunTUI [P]
- **Type**: Implementation
- **Files**: `internal/app/tui.go`
- **Description**: Modify `RunTUI` signature to accept an `onModelChange func(string) error` parameter (or add it to a config struct). Pass it through to `NewAgentRunner` via `agentRunnerConfigFromCLI` or a new field.
- **Dependencies**: Task 2.2
- **Est. Complexity**: Low

## Phase 3: Integration — Wire persistence into TUI `/model`

### Task 3.1: Wire Save callback in cmd/fox/main.go
- **Type**: Implementation
- **Files**: `cmd/fox/main.go`
- **Description**: Create the `onSave` closure in `main()` that:
  1. Loads current settings from file (to preserve unknown fields)
  2. Updates the `Model` field with the new model name
  3. Calls `settings.Save(homeDir, updatedSettings)`
  4. Logs a warning on error but does not crash
  Pass this closure to `app.RunTUI`. This completes the wiring so `/model <name>` in the TUI triggers persistence.
- **Dependencies**: Task 2.1, Task 2.3
- **Est. Complexity**: Medium

## Phase 4: Verification

### Task 4.1: Run full test suite
- **Type**: Testing
- **Files**: All packages
- **Description**: Run `go test ./...` and verify all existing tests still pass. Fix any regressions caused by the signature changes to `RunTUI` and `AgentRunnerConfig`.
- **Dependencies**: Task 3.1
- **Est. Complexity**: Low

### Task 4.2: Manual verification — startup priority
- **Type**: Testing
- **Files**: N/A (manual)
- **Description**: Verify manually or via integration test:
  - No settings file → model is `glm-4.5-air`
  - Settings file has `glm-4-plus` → model is `glm-4-plus`
  - `--model glm-4-flash` → model is `glm-4-flash`, settings unchanged
  - `FOX_MODEL=glm-4-flash` → model is `glm-4-flash`
  - `--model` + `FOX_MODEL` → `--model` wins
- **Dependencies**: Task 4.1
- **Est. Complexity**: Low

### Task 4.3: Manual verification — persistence
- **Type**: Testing
- **Files**: N/A (manual)
- **Description**: Verify manually:
  - `/model glm-4-plus` in TUI → `~/.foxharness/settings.json` written
  - New fox session → starts with `glm-4-plus`
  - Malformed `settings.json` → graceful fallback, no crash
- **Dependencies**: Task 4.1
- **Est. Complexity**: Low

## Execution Order

```
Phase 1: Task 1.1 ──► ┌─► Task 1.2 ──► Task 1.3 ──► Task 1.4 ──► Task 1.5 ──┐
                       │                                                     │
                       └─► Task 1.6 [P] ──► Task 1.7 ──────────────────────────┤
                                                                               │
Phase 2: ┌────────────────────────────────────────────────────────────────────┘
         │
    Task 2.1 ─────────────────────────────┐
         │                               │
    Task 2.1b [P] ──► Task 2.2 [P]       │
                          │               │
                     Task 2.3 [P]         │
                          │               │
Phase 3: Task 3.1 ◄──────┴───────────────┘
              │
Phase 4: ┌────┴────────┐
         │             │
    Task 4.1     Task 4.2 [P]
         │             │
         │        Task 4.3 [P]
         │
    (fix regressions if any)
```

## Checkpoints

- [x] **Checkpoint 1**: After Phase 1 — `go test ./internal/settings/...` passes; all Load/Save/ResolveModel tests green
- [x] **Checkpoint 2**: After Phase 2 — `go build ./cmd/fox` compiles; `OnModelChange` callback wired
- [x] **Checkpoint 3**: After Phase 3 — `go test ./...` passes; `/model` writes to settings.json
- [ ] **Checkpoint 4**: After Phase 4 — Full manual verification complete; no regressions
