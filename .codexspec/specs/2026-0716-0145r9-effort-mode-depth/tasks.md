# Tasks: Effort Mode Depth

## 1. Effort Domain Rules

- [x] 1.1 Add failing table-driven tests in `internal/effort` for OpenAI and Claude option sets, accepted values, rejected values, `auto` normalization, and provider-send omission. Covers: REQ-002, REQ-006; Plan: `internal/effort`
- [x] 1.2 Implement `internal/effort` helpers for protocol options, validation, explicit provider values, and errors. Covers: REQ-002, REQ-006; Plan: `internal/effort`
- [x] 1.3 Add failing precedence tests in `internal/effort` for frontmatter, CLI/session override, persisted preference, and default `auto`. Covers: REQ-005; Plan: `internal/effort`
- [x] 1.4 Implement effort precedence resolution. Covers: REQ-005; Plan: `internal/effort`

## 2. Settings and CLI Override

- [x] 2.1 Add failing settings tests for `llm.effort` load/save, unknown-field preservation, protocol-specific values, and `auto` clearing. Covers: REQ-003; Plan: `internal/llmconfig` and `internal/settings`
- [x] 2.2 Implement persisted `llm.effort` schema and settings helpers for setting, reading, and clearing protocol effort. Covers: REQ-003; Plan: `internal/llmconfig` and `internal/settings`
- [x] 2.3 Add failing CLI tests for parsing `-effort`, accepting valid values for the resolved protocol, and rejecting invalid values before model execution. Covers: REQ-004; Plan: `cmd/fox` and `internal/app`
- [x] 2.4 Implement `-effort` parsing, post-resolution validation, and propagation into app/engine run configuration. Covers: REQ-004, REQ-005; Plan: `cmd/fox` and `internal/app`

## 3. Provider Request Mapping

- [x] 3.1 Add failing OpenAI provider tests proving explicit effort maps to `reasoning_effort` and `auto` sends no effort field. Covers: REQ-006; Plan: `internal/provider`
- [x] 3.2 Add failing Claude provider tests proving explicit effort maps to `output_config.effort` and `auto` sends no effort field. Covers: REQ-006; Plan: `internal/provider`
- [x] 3.3 Implement provider generation options and OpenAI/Claude request mapping while preserving default `Generate` behavior. Covers: REQ-006, REQ-007; Plan: `internal/provider`
- [x] 3.4 Add tests proving default/background generation paths do not carry user effort unless `GenerateWithOptions` is explicitly used. Covers: REQ-007; Plan: `internal/provider`

## 4. Engine and Prompt Command Integration

- [x] 4.1 Add failing engine tests with fake providers proving normal user turns receive resolved session/persisted effort and background-style `Generate` callers remain effort-free. Covers: REQ-005, REQ-007; Plan: `internal/engine`
- [x] 4.2 Add failing prompt command tests proving frontmatter `effort` overrides session/persisted effort and invalid frontmatter effort fails before model execution. Covers: REQ-005; Plan: `internal/engine` and slash command execution path
- [x] 4.3 Implement engine call-time effort propagation and prompt command frontmatter override validation. Covers: REQ-005, REQ-007; Plan: `internal/engine` and slash command execution path

## 5. TUI `/effort` Selector

- [x] 5.1 Add failing TUI form tests for OpenAI and Claude option sets, selected cursor, Enter selection, Esc cancellation, and `auto` selection. Covers: REQ-001, REQ-002, REQ-003; Plan: `internal/tui`
- [x] 5.2 Add failing TUI slash command tests proving `/effort` opens the selector and `/effort <value>` does not set effort directly. Covers: REQ-001; Plan: `internal/tui`
- [x] 5.3 Implement `effortForm`, `/effort` slash routing, settings save callback, and user-facing status feedback. Covers: REQ-001, REQ-002, REQ-003; Plan: `internal/tui`

## 6. Verification, Commit, and Review

- [x] 6.1 Run targeted package tests after each task group and `go test ./...` after all implementation tasks. Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007; Plan: Verification Strategy
- [x] 6.2 Run `gofmt` on changed Go files and `go vet ./...`. Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007; Plan: Verification Strategy
- [x] 6.3 Commit completed implementation changes. Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007; Plan: Verification Strategy
- [x] 6.4 Run `$codexspec:review-code` against changed source paths compared with the main branch, verify reported findings, fix true issues, and repeat until no review-code defects remain. Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007; Plan: Verification Strategy

## Dependencies

```
1.1 -> 1.2 -> 1.3 -> 1.4
                    |
                    v
2.1 -> 2.2 -> 2.3 -> 2.4
                    |
                    v
3.1 -> 3.2 -> 3.3 -> 3.4
                    |
                    v
4.1 -> 4.2 -> 4.3
                    |
                    v
5.1 -> 5.2 -> 5.3
                    |
                    v
6.1 -> 6.2 -> 6.3 -> 6.4
```

## Coverage Table

| Requirement | Tasks |
|-------------|-------|
| REQ-001 | 5.1, 5.2, 5.3, 6.1, 6.2, 6.3, 6.4 |
| REQ-002 | 1.1, 1.2, 5.1, 5.3, 6.1, 6.2, 6.3, 6.4 |
| REQ-003 | 2.1, 2.2, 5.1, 5.3, 6.1, 6.2, 6.3, 6.4 |
| REQ-004 | 2.3, 2.4, 6.1, 6.2, 6.3, 6.4 |
| REQ-005 | 1.3, 1.4, 2.4, 4.1, 4.2, 4.3, 6.1, 6.2, 6.3, 6.4 |
| REQ-006 | 1.1, 1.2, 3.1, 3.2, 3.3, 6.1, 6.2, 6.3, 6.4 |
| REQ-007 | 3.3, 3.4, 4.1, 4.3, 6.1, 6.2, 6.3, 6.4 |

## Notes

- The constitution requires Red-Green-Refactor for all implementation tasks.
- Tests must avoid network calls.
- Background provider call sites should remain on the existing effort-free `Generate` path unless explicitly identified as user-run engine calls.
