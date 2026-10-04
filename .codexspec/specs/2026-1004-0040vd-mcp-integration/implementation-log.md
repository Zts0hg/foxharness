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

## T003 — Actual image requests and fallback

- Red: outbound provider fixtures showed only text and no image batch; modelinvoke/configuration tests failed for missing capability and resolver APIs.
- Green: provider/modelinvoke/llmconfig/settings package tests passed.
- T003-S01/S02: provider.TestProviderRequestsContainCorrelatedImageBytes asserts native encoded image data and tool-call batch ordering over actual HTTP requests.
- T003-S03/S04/S05/S06: modelinvoke.TestImageHydrationFallbackAndMissingArtifacts covers disabled, automatic single fallback, explicit enabled rejection, generic errors, missing artifacts, and request-copy hydration. provider.TestImageRejectionRequiresExplicitBadRequest excludes auth/server/encoding errors.
- Configuration: llmconfig.TestImageInputProfileDefaultsAndValidation and settings.TestImageInputSurvivesSettingsSaveAndReload.

## T004 — Context and replay

- Red: image estimator treated image as zero cost; compaction clone aliased rich history. Green: compaction/runtimecompaction/runtime/session tests passed.
- T004-S01: compaction.TestImageArtifactsHaveBudgetAndExplicitSummaryBoundary; runtimecompaction.TestCompactionClonePreservesOwnedImageReferences.
- T004-S02: runtimecompaction.TestCompactingImageHistoryKeepsReplayArtifact.
- T004-S03: session.TestImageReplayAfterMessageRewindKeepsArtifact.
- T004-S04: unchanged existing compaction, runtime, and session tests; immutable fixture checks remain in the full suite.

## T005 — Scoped configuration

- Red: missing configuration APIs; a later validation test rejected accepting an invalid new definition. Green: `go test ./internal/mcpconfig` passed.
- T005-S01/S02/S06: TestScopedConfigWholeOverridesAndSnapshots; TestInvalidDefinitionsAndSecretsAreIsolated also checks redaction.
- T005-S03: TestInvalidDefinitionsAndSecretsAreIsolated.
- T005-S04/S05: TestScopedMutationsPreserveOthersAndRejectInvalidFiles; TestMutationLockAndInvalidNewDefinition.
- Edits use an exclusive lock directory and synced temporary file plus rename; stale locks remain explicit errors instead of silently permitting conflicting writers.

## T006 — Authentication

- Red: authentication tests failed for missing manager, credential, and identity APIs after installing pinned SDK dependencies.
- Green: `go test -race ./internal/mcpauth ./internal/mcpconfig ./internal/media` passed after strict validation of explicitly advertised malformed OAuth resource metadata. SDK fallback discovery otherwise silently accepted the malformed advertised document.
- T006-S01/S06: TestUnattendedAndStaticCredentials.
- T006-S02/S04/S05: TestOAuthLoginRefreshRestartLogoutAndScope asserts PKCE, actual loopback callback, persistent refresh, concurrent serialized refresh, project identity, and invalidation after logout.
- T006-S03: TestInvalidOAuthCallbacksAndCancellationDoNotStoreCredentials covers state/issuer/denied/malformed metadata/cancellation and closed callback listener.
- T006-S07: permissions assertion in login test; TestRefreshFailureAndStoreFailureAreExplicit covers locked store and redacted refresh rejection.
- TestCredentialsNeverFollowCrossOriginRedirect verifies no credential dispatch to another origin.
- Full T003–T006 verification: `go test ./...` passed in the clean verification copy (including SDK transitive upgrades).
