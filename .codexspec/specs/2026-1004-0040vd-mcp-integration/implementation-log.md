# MCP Implementation Evidence

## Baseline

- Original checkout: `go test ./...` failed in architecture scans of nested historical worktrees; other packages passed. See issues.md.
- Clean archive of original HEAD, `/tmp/fox-mcp-check.xqMuIv`: `go test ./...` passed. No production edits preceded this baseline.

## T001 — Rich result contracts

- Red: schema/registry tests failed on missing content fields and Clone API; engine/runtime/session tests likewise failed on missing rich contracts.
- Green: `go test ./internal/schema ./internal/toolprotocol ./internal/tools ./internal/registryexec ./internal/engine ./internal/toolexec ./internal/toolruntime ./internal/runtime ./internal/session` passed.
- Structured JSON uses `json.RawMessage` with defensive byte copies, retaining exact nested JSON without type conversion. Transient image bytes are excluded from serialized records. Legacy text error records retain their existing encoding; rich results additionally persist their error status.

| Scenario | Implemented assertions |
|---|---|
| T001-S01 | schema.TestRichMessageCloneAndCompatibleJSON; existing immutable history and execution tests |
| T001-S02 | registryexec.TestRichRegistryExecutionPreservesContentAcrossTruncationAndHookMutation; engine.TestEngineCommitsOwnedRichResult |
| T001-S03 | above tests plus runtime.TestContextCloneOwnsRichMessage and schema.TestRichMessageCloneAndCompatibleJSON |
| T001-S04 | registryexec.TestRichRegistryExecutionPreservesContentAcrossTruncationAndHookMutation |
| T001-S05 | session.TestMessageLogPersistsRichContentAndReturnsOwnedRecord; schema JSON compatibility test |

- T001 full verification: `go test ./...` passed in the clean verification copy, including architecture and immutable fixture checks.

## T002 — Image artifacts

- Red: media tests failed for missing Store API; toolruntime test failed for missing image-store injection.
- Green: `go test ./internal/media ./internal/toolruntime ./internal/registryexec ./internal/session` passed.

| Scenario | Implemented assertions |
|---|---|
| T002-S01 | media.TestStoreRoundTripContentAddressingAndOwnership |
| T002-S02 | media.TestStoreRejectsInvalidImagesAndEscapes |
| T002-S03 | media.TestStoreMissingCorruptAndWriteFailureAreExplicit (projection notice covered additionally by T003) |
| T002-S04 | media.TestStoreMissingCorruptAndWriteFailureAreExplicit; toolruntime.TestExecutorPersistsImageBeforeReturningReference |
| T002-S05 | media.TestStoreRoundTripContentAddressingAndOwnership |

- T002 full verification: `go test ./...` passed in the clean verification copy.
