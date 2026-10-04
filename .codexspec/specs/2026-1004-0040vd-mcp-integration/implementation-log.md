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

## T007–T008 — Protocol transport and content conversion

- Red: initial transport tests failed to compile before Connect/Session existed. Behavioral regressions cover malformed discovery, authorization classification, and bounded stdio output. Green: `go test -race ./internal/mcpclient ./internal/mcpintegration ./internal/runtime` passed; final expanded package checks run again below.
- T007-S01/S07: mcpclient.TestAllTransportsAndProtocolVersions exercises three transports, five requested versions, actual calls, cancellation, retained owner, and idempotent close. SSE's 2026 request negotiates 2025-11-25; 2026 HTTP uses stateless semantics.
- T007-S02: TestPaginatedDiscoveryResourceReadAndUncertainCallNeverReplays; TestMalformedDiscoveryAndUnsupportedProtocolAreRejected.
- T007-S03: TestHTTPFallbackExcludesAuthorizationFailures.
- T007-S04: TestCredentialsReachBothHTTPTransports; mcpauth.TestOAuthLoginRefreshRestartLogoutAndScope and TestCredentialsNeverFollowCrossOriginRedirect assert refresh and origin confinement.
- T007-S05: TestStartupTimeoutIsBounded; TestStdioLimitsAndFailedStartupReapDescendants; cancellation cases in TestAllTransportsAndProtocolVersions.
- T007-S06: TestPaginatedDiscoveryResourceReadAndUncertainCallNeverReplays counts the single dispatched uncertain call.
- T008-S01/S02/S03: TestRichConversionPreservesOrderStructuredDataAndErrors; TestUnsupportedImageFormatsAndOversizedPayloadsAreExplicit. Correlation and persistence additionally use T001/T002 tests.
- T008-S04: TestPaginatedDiscoveryResourceReadAndUncertainCallNeverReplays checks supported resource reads and unavailable resources; embedded image decoding is asserted by TestRichConversionPreservesOrderStructuredDataAndErrors.

## T009–T010 — Runtime services, borrowing, and permissions

- Red: new runtime services APIs were absent. TestSlowReconnectDoesNotBlockHealthyServer exposed a global lock held during network work; reconnect now builds outside that lock. TestCloseCancelsActiveCallsAndCleansAllServers exposed close timeout before canceling active calls; host shutdown now cancels ownership and starts all connection cleanup. TestTransportFailureUpdatesVisibleStatusWithoutRetry exposed invisible call failure status. TestUnattendedOAuthStatusIsActionable exposed erased authorization diagnostics between credential transport and SDK error conversion.
- T009-S01: runtime.TestOwnedServicesInitializeOnCreateAndOpenAndExtendBeforeNarrowing plus existing nil-factory lifecycle tests.
- T009-S02: runtime.TestPartialServiceFailureClosesAndReleasesSessionLease.
- T009-S03: mcpintegration.TestHostOwnsIsolatedSessionsBorrowersAndGeneration; childruntime.TestChildBorrowsParentBrowserAndHydratesLocalImages; existing runtime.TestChildRunnerPropagatesCancellationAndCleansBeforeReturning pins child terminal cleanup.
- T009-S04/S05: TestServerQueueCancellationDoesNotDispatchAndReconnectDrains; TestHostOwnsIsolatedSessionsBorrowersAndGeneration.
- T009-S06: TestCloseCancelsActiveCallsAndCleansAllServers; runtime.TestOwnedServicesInitializeOnCreateAndOpenAndExtendBeforeNarrowing; mcpclient process-tree/idempotent-close tests.
- T009-S07: TestSlowReconnectDoesNotBlockHealthyServer; TestTransportFailureUpdatesVisibleStatusWithoutRetry; tui.TestMCPNewSessionExplainsBrowserStateBoundary; isolated failure in TestHostOwnsIsolatedSessionsBorrowersAndGeneration.
- T010-S01: TestNamesAndExternalPermissionAssessment and collision assertion in TestKnownLinksRequireOriginCapabilityAndUnknownMetadataNeverGrantsPermission.
- T010-S02/S03: runtime.TestOwnedServicesInitializeOnCreateAndOpenAndExtendBeforeNarrowing; TestExternalChildCapabilitiesPreserveReadOnlyAndEmptyRestrictions; childruntime.TestMCPChildReadOnlyAndExplicitEmptyCannotExecuteExternalTools.
- T010-S04: childruntime.TestChildBorrowsParentBrowserAndHydratesLocalImages; TestMCPChildReadOnlyAndExplicitEmptyCannotExecuteExternalTools.
- T010-S05: TestNamesAndExternalPermissionAssessment; cmd/fox.TestTUIMCPDenialPreservesStateAcrossTurns; TestTUIMCPReconnectAndNewSessionPreserveOwnership; remote actual approval tests in both remote roots; CLI/Autodev success cases preserve their existing policy.
- T010-S06: TestKnownLinksRequireOriginCapabilityAndUnknownMetadataNeverGrantsPermission asserts unknown URI, origin restriction, cross-server rejection, and authorized read.
- A composition test caught use of the static profile snapshot for fallback child ceilings. Child composition now resolves the extended profile before obtaining allowed names.

## T011–T012 — Management and all entry compositions

- Red: management tests failed for missing application DTO/manager/parser APIs. Entry tests failed on missing external tools and image-policy forwarding. TUI tests first failed for absent management injection; probe-cleanup and visible artifact tests independently reproduced hidden errors/output. Green: all targeted entry/provider/management tests passed.
- T011-S01: mcpcmd.TestParseRequiresScopeAndPreservesArguments; TestSplitQuotedArguments; mcpintegration.TestManagementScopesProbeCleanupAndNoCrossProcessReconnect; mcpconfig scoped mutation tests. Both presentations use the same parser and management port.
- T011-S02: host ready/disabled/failure/authorization tests and runtime close assertions; mcpauth/mcpconfig redaction tests; TestTransportFailureUpdatesVisibleStatusWithoutRetry.
- T011-S03: TestManagementScopesProbeCleanupAndNoCrossProcessReconnect; TestProbeReportsCleanupFailure; cmd/fox.TestMCPManagementRunsWithoutLLMConfiguration.
- T011-S04: cmd/fox.TestTUIMCPReconnectAndNewSessionPreserveOwnership plus host active-call drain and stale-generation tests.
- T011-S05: tui.TestMCPCommandUsesSharedPortAndDisplaysLogin; TestMCPCancelEndsPendingManagementAndShowsFailure; mcpauth callback/logout tests.
- T011-S06: mcpconfig malformed/isolated validation tests and host isolated-failure tests; management commands run independently of LLM configuration.
- T012-S01: cmd/fox.TestCLIMCPCompositionCallsAndHydratesScreenshot.
- T012-S02: cmd/fox.TestTUIMCPDenialPreservesStateAcrossTurns; TestTUIMCPReconnectAndNewSessionPreserveOwnership.
- T012-S03: cmd/feishu.TestRemoteMCPApprovalAndDenial; TestFeishuMCPRequiresRemoteApproval; existing Feishu session-selection/profile tests.
- T012-S04: cmd/agentops.TestRemoteMCPApprovalAndDenial; TestAgentOpsMCPTaskOwnsConnection.
- T012-S05: cmd/fox.TestAutodevMCPUsesItemWorkspace; TestAutodevProviderForwardsImageCapability.
- T012-S06: cmd/bench.TestBenchmarkMCPFreezesFidelityAndIsolatesRepeats. Effective external names enter the frozen case run spec and fidelity.
- T012-S07: childruntime.TestChildBorrowsParentBrowserAndHydratesLocalImages; TestMCPChildReadOnlyAndExplicitEmptyCannotExecuteExternalTools.
- T012-S08: cmd/fox.TestCLIActualProvidersReceiveMCPImagesAndTextFallback sends real PNG bytes through the full CLI runtime and both actual HTTP provider adapters, covering enabled/disabled modes and absence of base64 in final presentation.
- toolruntime.TestExecutorPersistsImageBeforeReturningReference additionally asserts a user-visible artifact reference after persistence.

## T013 — Live Playwright acceptance

- Command: `FOX_MCP_PLAYWRIGHT=1 go test ./internal/mcpintegration -run TestPlaywrightAcceptance -count=1 -v` — PASS on 2026-10-04 with Node 22.18.0, npm 10.9.3, Playwright MCP 0.0.83, installed Chrome.
- T013-S01: TestPlaywrightAcceptance asserts navigate → inline snapshot → type → click → `Hello Ada` on a controlled loopback page. Initial test assumptions were corrected against the pinned server's discovered schema: automatic snapshots may be file links, and version 0.0.83 accepts `target` rather than `ref`.
- T013-S02: The test runs actual childruntime with borrowed services, checks the same page text, then calls the parent snapshot again.
- T013-S03: PNG bytes are persisted by the normal executor, resolved for replay, and matched against actual outbound HTTP request bodies from both provider adapters.
- T013-S04: Both disabled-mode subtests assert absent image request data, explicit limitation, and retained readable artifact.
- T013-S05: Live-model subtest implemented but **NOT RUN**: no explicit live-model endpoint/model/credentials were supplied. Its skip reason is visible; browser/request-encoding acceptance does not claim visual reasoning by a live model.

## T014 — Documentation

- Added docs/mcp.md and README link, with scope precedence, supported commands/transports, OAuth lifecycle, permission/ownership boundaries, screenshot capability/fallback, exact pins, and acceptance reproduction.
- Dependency document and architecture tests updated together. Runtime and presentation retain their existing inward dependency boundaries.
- Documented SDK transport-specific negotiation, Playwright's pinned argument schema, and remote live-task/browser-state boundaries.

## Pre-review verification

- `go test ./...` passed in `/tmp/fox-mcp-check.xqMuIv`, a clean archive overlaid with all tracked/nonignored current feature files; ordinary live-browser test reports skipped by default.
- Original checkout `go test ./...` still fails only architecture scans over pre-existing nested historical worktrees; see issues.md. Those trees and all immutable fixture payloads remain untouched.
- Focused race suite passed for mcpconfig, mcpauth, mcpclient, mcpintegration, runtime, childruntime, media, and modelinvoke.
- Final scenario-to-test self-check and isolated complete-feature review are the remaining T015 gate; no PASS review verdict is claimed yet.

## T015 — Regression repairs and scenario self-check

- Independently reproduced all four admitted review defects: the package-test script rejected the new fixture package; real post-connect HTTP 401/403 lost login guidance on both tools and resources; active timeout/resource disconnect retained ready status; FIFO and symlink-to-FIFO configurations blocked both scopes. Reproduction tests were run and failed before the production repairs.
- Authentication classification now survives the SDK boundary. `mcpintegration.TestPostConnectAuthorizationFailureIsActionableWithoutReplay` checks real 401/403 responses, status, login guidance, and exactly one request. `mcpclient.TestPostConnectRefreshFailureRetainsLoginHint` checks sanitized refresh failures on both HTTP transports and both operations, complementing the real refresh rejection fixture in mcpauth.
- `TestActiveTimeoutAndResourceFailureChangeStatus` asserts unavailable status, redacted reasons, and removal from advertised capabilities. `TestActiveCallerCancellationKeepsHealthyConnection` and the existing queue test preserve caller/queue cancellation semantics. Existing generation and close tests now explicitly assert connecting/closed status.
- `mcpconfig.TestLoadRejectsFIFOAndSymlinkToFIFOWithoutBlocking` covers both scopes. Regular-file checks run before and after opening; Unix opens are nonblocking to avoid a substituted FIFO between checks. Existing malformed/missing/regular configuration tests remain green.
- `mcpfixture.TestFixtureServesImageAndTracksSessionLifetime` verifies real session counts, exact image bytes, consecutive calls and close. The package-level CI gate now passes.
- Scenario self-check also reproduced missing design command forms and empty-content MCP failure semantics. Red tests: `TestListNamedStatusAndBareStatus`, `TestListRedactsEffectiveConfigurationWithoutConnecting`, `TestNamedStatusSelectsOneServerAndReportsUnknownName`, and `TestEmptyToolErrorRetainsRichErrorContent`. They pass after adding nonconnecting redacted list, selected-server probes, bare status, and explicit rich content for empty tool errors. `tui.TestBareMCPShowsStatusThroughSharedPort` confirms actual slash dispatch.
- All 76 enumerated scenarios have substantive assertion mappings above. Strengthened T011-S02 with connecting/closed and runtime failure assertions, and T012-S01/S04/S05 with explicit zero-active-session assertions after CLI/AgentOps drain and Autodev close. T013-S05 remains explicitly **NOT RUN**, as allowed for absent live-model inputs; the scenario's skip is not a visual-reasoning pass.
- Green repair verification in the clean current-source copy: `go test ./...`, `go test -race ./...`, `go vet ./...`, `sh scripts/check-package-tests.sh`, `gofmt -l cmd internal benchmarks` (empty), and `git diff --check` all passed. Ordinary tests remain hermetic.
- `npm_config_offline=true FOX_MCP_PLAYWRIGHT=1 go test ./internal/mcpintegration -run TestPlaywrightAcceptance -count=1 -v` passed again, including browser continuity, persisted screenshot, and enabled/disabled request assertions for both providers. Live-model subtest remains explicitly skipped.
- Complete-feature independent re-review remains pending; T015 is not yet marked complete.

## OAuth resource binding regression

- A second complete independent review verified the previous four obligations and identified one additional OAuth refresh defect. Independently strengthened the existing loopback OAuth fixture to require the authorized `resource` in refresh requests: login succeeded, then the regression failed with `invalid_target` and missing resource. The pinned generic oauth2 TokenSource did not retain the SDK's code-exchange resource option.
- Login now persists the actual resource selected by the SDK authorization URL. Refresh reuses that resource through explicit grant parameters, preserving the existing OAuth client-authentication behavior, serialized store updates, cancellation, redacted failures, and refresh-token rotation. Older credential records without the field fall back to their bound server endpoint.
- `TestOAuthLoginRefreshRestartLogoutAndScope` now checks the resource at the strict token endpoint, persistence, one concurrent refresh, and isolation across endpoint/client/scope changes. `TestRefreshRetainsAuthorizedResourceAcrossRotationAndRestart` checks a canonical resource different from the endpoint path, restart, confidential client authentication, omission of the old redirect URI, and both rotated and unchanged refresh tokens. `TestBothHTTPTransportsUseResourceBoundRefreshedCredentials` verifies an already-connected Streamable HTTP/SSE session calls a real MCP tool with the refreshed credential.
- Relevant protocol obligation: [MCP resource parameter implementation](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization#resource-parameter-implementation).
- Verification after repair: `go test -race ./internal/mcpauth ./internal/mcpclient ./internal/mcpintegration`, clean-copy `go test ./...`, `go test -race ./...`, `go vet ./...`, package-test gate, empty gofmt check, `git diff --check`, and `go mod verify` all passed. No live-browser behavior changed; the previous fixed-version browser evidence remains applicable. Fresh complete-feature review remains required before T015 completion.
