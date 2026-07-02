# Implementation Issues — backlog-add

## Issue: T003 manual TUI acceptance cannot run in this environment
- **Task**: T003 (behavioral acceptance)
- **Error**: The interactive `/codexspec:backlog` run (US1 confirm-and-append, US2 cancel, US3 `backlog_file` honor) requires a human-driven TTY session where the `ask_user_question` asker is installed (verified repo fact: only the TUI installs a `UserAsker`). This environment cannot drive an interactive TUI session.
- **Attempted**: All automatable verification was completed instead — full `go test ./...` green; NFR-001 guarded by the pre-existing `TestParseWellFormedItems` (`internal/autodev/backlog_test.go`), whose format the skill's template matches exactly; REQ-001 discoverability confirmed by `TestDiscoverCommands_CodexspecBacklog` and by fox live-discovering `/codexspec:backlog` from the real `.claude/commands/codexspec/backlog.md`.
- **Status**: Needs Discussion — the user should run the three manual TUI checks (against a scratch backlog file) before relying on the feature. This does not block the code review: the code change is a discovery test plus a Markdown skill; the fidelity-critical format is unit-tested upstream.

## Note: T001/TDD framing
- **Task**: T001
- **Note**: The discovery mechanism (`DiscoverCommands` + `pathToName`) pre-exists and is generic, so `TestDiscoverCommands_CodexspecBacklog` is a characterization/regression guard for the `codexspec:backlog` name/scope contract, not a red→green cycle for new logic. There is no new Go product logic in v1 (per DEC-002); the skill `.md` is a Non-Testable asset implemented directly and verified by the upstream Parse test + live discovery.
- **Status**: N/A (transparency note, not a blocker)
