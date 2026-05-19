# Feature: Model Switch Persistence

## Overview

Persist the user's model selection across sessions so that when a user switches
models via `/model <name>` in the TUI, the new model is saved to a settings file
and automatically used as the default for subsequent sessions. This eliminates
the need to re-specify the model on every launch.

The design follows the priority system used by Claude Code: CLI flag > env var >
settings file > built-in default.

## Goals

- Remember the user's last model choice across fox sessions
- Provide a four-level model selection priority system
- Persist model changes immediately on `/model` command execution
- Keep the implementation focused on model persistence only (no other settings)

## User Stories

### Story 1: Model persists across sessions

**As a** fox TUI user
**I want** my `/model` selection to persist across sessions
**So that** I don't have to switch models every time I start fox

**Acceptance Criteria:**
- [ ] After running `/model glm-4-plus` in one session, starting a new session uses `glm-4-plus` by default
- [ ] The model name is saved to `~/.foxharness/settings.json`
- [ ] If settings.json doesn't exist, it is created automatically
- [ ] If settings.json exists but has no `model` field, the built-in default is used

### Story 2: CLI flag overrides persisted model

**As a** fox user
**I want** the `--model` flag to override my persisted model choice
**So that** I can temporarily use a different model without changing my saved preference

**Acceptance Criteria:**
- [ ] `fox --model glm-4-flash` uses `glm-4-flash` even if settings.json has `glm-4-plus`
- [ ] The `--model` flag does NOT overwrite the persisted settings.json model
- [ ] The flag value takes the highest priority in model selection

### Story 3: Environment variable override

**As a** fox user in a CI/CD or scripted environment
**I want** to set `FOX_MODEL` to override the persisted model
**So that** I can control the model via environment without changing settings files

**Acceptance Criteria:**
- [ ] `FOX_MODEL=glm-4-flash fox` uses `glm-4-flash` even if settings.json has `glm-4-plus`
- [ ] `FOX_MODEL` is lower priority than `--model` flag
- [ ] `FOX_MODEL` is higher priority than settings.json

### Story 4: View current model

**As a** fox TUI user
**I want** to see which model is currently active
**So that** I can verify my model selection

**Acceptance Criteria:**
- [ ] `/model` (no arguments) displays the current model name
- [ ] The status bar shows the active model name (existing behavior preserved)

### Story 5: Switch model in session

**As a** fox TUI user
**I want** to switch models during a session
**So that** I can use different models for different tasks

**Acceptance Criteria:**
- [ ] `/model glm-4-plus` switches to the new model immediately
- [ ] The new model is persisted to settings.json immediately
- [ ] Subsequent runs within the same session use the new model
- [ ] A confirmation message is shown after switching

## Functional Requirements

- [REQ-001] **Settings file location**: `~/.foxharness/settings.json`
- [REQ-002] **Settings file format**: JSON with a `model` string field, e.g. `{"model": "glm-4-plus"}`
- [REQ-003] **Model priority resolution** (highest to lowest):
  1. CLI `--model` flag (explicit command-line argument)
  2. `FOX_MODEL` environment variable
  3. `model` field in `~/.foxharness/settings.json`
  4. Built-in default: `glm-4.5-air`
- [REQ-004] **Persistence trigger**: Model is saved to settings.json immediately when `/model <name>` is executed in the TUI
- [REQ-005] **Persistence scope**: Only the model name is persisted. Provider, thinking, plan mode, and other flags are NOT persisted
- [REQ-006] **File creation**: If `~/.foxharness/settings.json` does not exist, create it with the model field
- [REQ-007] **Atomic writes**: Settings file writes should be atomic (write to temp file, then rename) to avoid corruption
- [REQ-008] **Graceful degradation**: If settings.json is unreadable or malformed, fall back to the built-in default without crashing
- [REQ-009] **CLI flag non-persistence**: The `--model` CLI flag does NOT overwrite settings.json; only `/model` command in TUI triggers persistence
- [REQ-010] **New package**: Create `internal/settings` package for settings file read/write operations. Expected exported API:
  - `Load(homeDir string) (*Settings, error)` — reads and parses `~/.foxharness/settings.json`; returns zero-value `Settings` if file missing or malformed
  - `Save(homeDir string, s *Settings) error` — writes settings atomically (temp file + rename); creates `~/.foxharness/` directory if needed
  - `ResolveModel(cliFlag, envVar string, s *Settings) string` — applies the four-level priority: returns `cliFlag` if non-empty, else `envVar` if non-empty, else `s.Model` if non-empty, else built-in default `"glm-4.5-air"`
  - `Settings` struct with a `Model string` field; JSON round-trips preserve unknown fields for forward compatibility

## Non-Functional Requirements

- [NFR-001] **Performance**: Settings file read adds < 5ms to startup time
- [NFR-002] **Reliability**: Atomic file writes prevent corruption on crash
- [NFR-003] **Compatibility**: Existing sessions and CLI behavior remain unchanged when no settings.json exists
- [NFR-004] **Security**: Settings file uses standard file permissions (0600 for user-only access)

## Acceptance Criteria (Test Cases)

- [TC-001] No settings.json exists → model defaults to `glm-4.5-air`
- [TC-002] settings.json has `{"model": "glm-4-plus"}` → model resolves to `glm-4-plus`
- [TC-003] `--model glm-4-flash` + settings.json has `glm-4-plus` → model resolves to `glm-4-flash`, settings.json unchanged
- [TC-004] `FOX_MODEL=glm-4-flash` + settings.json has `glm-4-plus` → model resolves to `glm-4-flash`
- [TC-005] `--model glm-4-flash` + `FOX_MODEL=glm-4-air` → model resolves to `glm-4-flash` (CLI wins)
- [TC-006] `/model glm-4-plus` in TUI → settings.json written with `{"model": "glm-4-plus"}`
- [TC-007] settings.json is malformed JSON → graceful fallback to `glm-4.5-air`, no crash
- [TC-008] settings.json has empty `{"model": ""}` → treated as unset, falls back to default
- [TC-009] New session after `/model glm-4-plus` → starts with `glm-4-plus`
- [TC-010] Settings file is created with correct directory structure if `~/.foxharness/` doesn't exist
- [TC-011] Concurrent writes to settings.json do not corrupt the file (atomic rename)

## Edge Cases

- **Malformed JSON**: If `~/.foxharness/settings.json` contains invalid JSON, log a warning and fall back to the built-in default. Do not crash.
- **Missing directory**: If `~/.foxharness/` doesn't exist, create it before writing settings.json.
- **Empty model field**: If `{"model": ""}` is in settings.json, treat it as unset and use the default.
- **Permission denied**: If settings.json cannot be written (e.g., read-only filesystem), log a warning and continue with in-memory model. Do not crash.
- **Settings file with extra fields**: Preserve unknown fields when rewriting the file (read-modify-write pattern) so future extensions don't lose data.
- **Concurrent processes**: Two fox processes running simultaneously may write to settings.json. Atomic rename prevents corruption, but last-write-wins is acceptable.

## Output Examples

### settings.json format
```json
{
  "model": "glm-4-plus"
}
```

### `/model` output (no arguments)
```
Current model: glm-4-plus
Usage: /model <model_name>
```

### `/model glm-4-plus` output
```
Switched model to glm-4-plus
```

## Out of Scope

- Provider protocol persistence (remains CLI flag only)
- Thinking mode persistence
- Plan mode persistence
- Max turns persistence
- Project-level settings (only global user-level settings)
- Model alias system (e.g., short names that map to full model IDs)
- Model validation against an allowlist
- Legacy model migration/remapping
- Session-level model overrides (model is always global)
