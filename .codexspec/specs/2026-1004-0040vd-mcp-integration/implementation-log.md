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

## Terminal cleanup during persistence recovery

- The next review session was interrupted before producing a valid final envelope; its provisional observations are not a final verdict. Independently reproduced a cleanup omission with `TestCloseReleasesServicesEvenWhenRunFinishNeedsRecovery`: repeated durable-finish failure caused Close to return before external services were released (zero cleanup calls).
- Close now performs bounded external-service cleanup once even when durable finish requires recovery. The persistence recovery error and lease remain intact, cleanup errors remain visible, and subsequent durable recovery/close does not repeat remote cleanup. Existing recovery/admission behavior for sessions without services is preserved.
- The regression asserts cleanup count/context, joined cleanup and recovery errors, retained identity lease, successful durable recovery, and subsequent reopen. The runtime/app/child/MCP and all production entry packages pass their race tests. Clean-copy `go test ./...`, `go test -race ./...`, `go vet ./...`, package-test gate, empty gofmt check and `git diff --check` all passed before final independent review.


## Explicit OAuth completion and preregistered issuer binding

- Independently reproduced a plain initial HTTP 403 returning success without a prompt or stored credentials. The SDK intentionally returns nil for 403 without `insufficient_scope`; explicit Fox login now additionally requires successful credential persistence. `TestExplicitLoginDoesNotClaimSuccessOnPlainForbidden` failed before this change and passes afterward.
- `TestConfidentialLoginBindsConfiguredIssuer` independently reproduced missing/mismatched issuer accepting preregistered fixture credentials, and changing issuer reusing cached credentials. Configuration now requires trusted `auth.issuer` and `auth.client_id` for `client_secret_env`, passes that issuer to the SDK preregistered-client check, and includes it in credential identity. Missing issuer fails before discovery; a mismatch fails before prompt/token exchange. Public clients retain optional issuer binding.
- The new tests cover missing/mismatched/matched issuers, successful insufficient-scope authorization, credential isolation, and invalid issuer configuration. Targeted auth/config/parser/integration/TUI/CLI tests pass.

## Configured live-model visual acceptance

- Located the existing user-local Fox default provider `zhipu`, model `glm-5.2`, Claude-compatible protocol, with usable credentials. No credentials were printed or committed.
- First live run failed before sending a request: the test supplied unsupported auth value `bearer`. Changed the fixture to the existing `llmconfig.AuthAPIKey` contract, then reran.
- Command: `python3 /tmp/fox-mcp-run-live.py`, which reads the existing default profile without printing secrets, supplies `FOX_MCP_PLAYWRIGHT=1`, `FOX_MCP_LIVE_PROTOCOL`, `FOX_MCP_LIVE_BASE_URL`, `FOX_MCP_LIVE_MODEL`, and `FOX_MCP_LIVE_API_KEY`, and invokes `go test ./internal/mcpintegration -run '^TestPlaywrightAcceptance$' -count=1 -v`.
- Result: **FAIL** for T013-S05 / SC-007. The live response was `The background color of the "VISUAL BADGE" in the image is **blue**.` The controlled page is orange; the screenshot was independently inspected in a disposable diagnostic copy and is orange. All four actual-provider request encoding/fallback subtests and browser/child workflow assertions passed. Raw redacted evidence is at `/tmp/fox-mcp-live-model.log`; diagnostic screenshot is `/tmp/fox-mcp-controlled-page.png`.
- Requested an image-capable model configuration from the user. Final visual acceptance and T015 completion remain blocked pending successful model analysis. Earlier NOT RUN entries above record the earlier state, not the current result.

- Post-repair deterministic verification: clean-copy `go test ./...`, `go test -race ./...`, `go vet ./...`, `sh scripts/check-package-tests.sh`, empty `gofmt -l cmd internal benchmarks`, and `git diff --check` all passed. The verification mirror was restored to current repository source before these commands; diagnostic-only screenshot output was not included.
- The latest valid isolated complete-feature report remains `/tmp/fox-mcp-review-completion.md` (FAIL on its earlier target); its two code findings were independently reproduced and repaired, while the live-model evidence obligation remains unresolved. No new PASS review is claimed. T013 was reopened for the unsuccessful visual demonstration; T015 remains open. Fresh complete-feature review follows successful live-model acceptance.


## User-selected glm-5.3-flash visual acceptance

- User selected `glm-5.3-flash` for the live model call. Reused the existing `zhipu` profile's Claude-compatible endpoint and credential source, overriding only the model for this test invocation; did not change persisted user settings.
- Exact command: `FOX_MCP_LIVE_MODEL=glm-5.3-flash python3 /tmp/fox-mcp-run-live.py`. The wrapper loads the local default profile and invokes `go test ./internal/mcpintegration -run '^TestPlaywrightAcceptance$' -count=1 -v` with the documented `FOX_MCP_PLAYWRIGHT` and four `FOX_MCP_LIVE_*` variables. Equivalent direct command: `FOX_MCP_PLAYWRIGHT=1 FOX_MCP_LIVE_MODEL=glm-5.3-flash FOX_MCP_LIVE_PROTOCOL=claude go test ./internal/mcpintegration -run '^TestPlaywrightAcceptance$' -count=1 -v`, with `FOX_MCP_LIVE_BASE_URL` and `FOX_MCP_LIVE_API_KEY` supplied from the existing profile.
- Result: **PASS** on 2026-10-04. The model answered `The background color of the VISUAL BADGE is **orange**.` The screenshot came from the actual pinned Playwright MCP workflow, and the prompt contained no expected color. Navigation/snapshot/type/click, authorized child continuation, persisted screenshot, both provider image encodings and disabled-mode fallback also passed.
- Redacted raw test evidence: `/tmp/fox-mcp-live-glm-5.3-flash.log`. Configuration identity: local profile `zhipu`, protocol `claude`, overridden model `glm-5.3-flash`. Endpoint credentials remain user-local. This resolves T013-S05 / SC-007's live evidence gap and re-completes T013. No production code changed after the previous green full/race/vet/package/format checks; final independent review remains pending.


## Terminal error propagation and independent OAuth refresh

- The complete-feature review at `44042e0` reached a valid FAIL with three P2 defects and verified all three prior follow-up obligations, including an independent successful `glm-5.3-flash` visual demonstration. Caller validation accepted the schema-v2 result at `/tmp/fox-mcp-final-gate.md` and retained all new neutral obligations.
- Independently reproduced all three defects in a separate mirror with external Go overlays (`/tmp/fox-mcp-root-probes.log`), then installed regression tests and observed the same failures before production repairs (`/tmp/fox-mcp-repairs-red.log`).
- CLI now joins application Drain failures with run failures. `TestCLIReportsMCPCleanupFailure` uses the actual CLI composition and a real MCP HTTP server that disconnects DELETE, asserting the cleanup error reaches the caller. `TestRunPreservesRunFailureAlongsideDrainFailure` asserts both causes survive; successful output ordering remains covered by the existing suite.
- Autodev now gives persistence recovery and session close separate fresh five-second contexts after waiting/cancelling extraction. `TestAutodevClosesMCPAfterExtractionTimeout` uses the real factory, cancellable extraction and MCP session fixture; it asserts the extraction deadline remains observable and active sessions are zero after Close.
- OAuth refresh now holds only a per-credential cross-process lock during network work. The shared credential-file lock protects short reads and atomic merge/write operations. Login/save/logout use the same identity lock, preventing a refresh from restoring removed credentials. `TestUnrelatedCredentialSurvivesSlowRefresh` asserts a second Manager reads a healthy credential during a stalled independent refresh. `TestConcurrentRefreshMergesRecordsAndCannotUndoLogout` exercises concurrent independent refresh, a third saved record and logout during refresh, then verifies persisted results after reopening. Existing concurrent same-credential refresh, resource binding, rotation, issuer, origin and logout tests remain green.
- Verification: relevant CLI/auth/composition race tests; clean current-source copy `go test ./...`, `go test -race ./...`, `go vet ./...`; `sh scripts/check-package-tests.sh`, empty gofmt check and `git diff --check` all passed. No browser/image projection code changed; current actual `glm-5.3-flash` acceptance evidence remains applicable. T015 still requires a fresh complete independent review.


## Configuration URL redaction on invalid definitions

- The next valid complete-feature review at `709d745` verified all three previous repair obligations and admitted one P2 display defect. Caller independently reproduced all bounded variants: malformed endpoint retaining query data, issuer userinfo on a rejected definition, and client-metadata URL credentials/query. Report: `/tmp/fox-mcp-release-gate.md`; strict caller validation passed.
- Red regressions (`/tmp/fox-mcp-redaction-root-red.log`) failed before the repair. `TestRedactedURLsHandleInvalidAndOAuthConfiguration` now covers all three URL fields with userinfo/query/fragment, malformed escape, opaque/relative URL and absent values, while asserting the original snapshot stays unchanged. `TestListRedactsSecretsEvenWhenConfigurationIsInvalid` verifies actual shared management formatting for both scopes and retained useful validation diagnostics, without connecting.
- Redacted snapshots now sanitize endpoint, issuer and client-metadata URLs through one helper. Valid HTTP(S) URLs retain non-sensitive origin/path while dropping userinfo, query and fragment. Invalid URLs produce an explicit redacted placeholder rather than their original value.
- Green verification: relevant config/integration/TUI/CLI race tests; clean current-source copy `go test ./...`, `go test -race ./...`, `go vet ./...`; package-test gate, empty gofmt check and `git diff --check` all passed. No image/protocol behavior changed. T015 remains pending a fresh complete-feature isolated review.

## Text resource preservation and Feishu session clarification

- The valid complete-feature review at `78285d9` verified the preceding URL-redaction obligation and admitted two P2 findings. Strict caller validation of `/tmp/fox-mcp-completion-gate.md` passed; both neutral follow-up obligations are retained in `/tmp/fox-mcp-review-next-obligations.json`. The report remains FAIL, not a completion approval.
- Independently reproduced text loss for embedded `application/json`, `application/xml`, `application/javascript`, textual `image/svg+xml`, and empty JSON resources, plus actual authorized JSON/XML resource reads. Repository regression tests failed before changing production code (`/tmp/fox-mcp-text-red.log`).
- Embedded resources now select text from the SDK's text/blob representation rather than a MIME whitelist. Text, MIME and URI are preserved, including empty text; binary images still receive existing validation and unsupported binary formats retain an explicit notice. `TestTextResourcesPreservePayloadAndMIME` and the extended `TestPaginatedDiscoveryResourceReadAndUncertainCallNeverReplays` cover both conversion entry paths and metadata preservation.
- Green verification: targeted MCP client/media/integration race tests; clean current-source copy `go test ./...`, `go test -race ./...`, `go vet ./...`; package-test gate, empty gofmt check and `git diff --check` all passed. Evidence: `/tmp/fox-mcp-text-{green,full,race,vet,package,format}.log`.
- Feishu currently reopens a persisted session record for each message and closes its live AgentSession after each task. The review interprets ordinary-message continuity as requiring the same live MCP/browser session across messages; the approved design separately distinguishes persisted history from live AgentSession lifetime. Asked the user to choose persistent browser state until explicit new session/service shutdown, or retain task-local connections with an explicit notice. No dependent lifecycle implementation or authoritative requirement change has been made while that decision is pending. T015 remains open.

## Confirmed Feishu live-session continuity

- The user explicitly selected shared browser state until new session/service exit. Added confirmed DEC-010 and synchronized REQ-013, design C04, plan P06, T012-S03 and user documentation. This resolves the prior decision blocker without weakening the connection/configuration obligations.
- Repository regression tests first failed for per-message drain closing MCP, cancelled-task drain closing MCP, and service exit omitting factory cleanup (`/tmp/fox-feishu-continuity-red.log`). The composition factory now retains one live AgentSession per chat/user. Each prepared application binds its own permission coordinator/evidence, journals, compactor and extraction tracker through its run context; the retained session remains the sole live runtime owner. Task drain retries durable finish without closing the owner.
- Explicit new session closes the prior owner before replacement. Runner/service shutdown stops work and invokes factory close with a fresh bounded context, joining cleanup errors into the service result. Per-conversation preparation locks avoid serializing unrelated network startup; terminal session recovery and close use separate bounded cleanup contexts.
- T012-S03 mapping strengthened: `TestFeishuMessagesRetainLiveMCPAndFrozenConfiguration` checks the actual MCP session ID across messages after their contexts are cancelled, retained endpoint after config edits, new approvals and run/trace identities, independent users, explicit replacement/rejection of stale applications, actual serve→runner→factory cleanup and closed-factory admission. `TestFeishuCancelledAndFailedMessagesRetainSession` asserts cancellation/model failure retain the connection and a subsequent run succeeds. `TestFeishuServeClosesFactoryAndReportsCleanupError` verifies cleanup errors and a live cleanup context. Existing permission allow/deny, extraction, terminal-delivery and child-borrowing tests remain green.
- Green verification: targeted Feishu/runtime/MCP/child race suites; clean current-source copy `go test ./...`, `go test -race ./...`, `go vet ./...`; package-test gate, empty gofmt check, and `git diff --check`. Logs: `/tmp/fox-feishu-{full,full-race,vet,package,format}.log`. All 76 enumerated task scenarios retain substantive test mappings; T012-S03 now includes the explicitly clarified lifecycle assertions. T015 awaits a fresh complete-feature isolated review with both retained follow-up obligations.

## Final completion — 2026-10-04

- **Result: PASS; T001–T015 complete.** The seventh valid complete-feature review used a fresh isolated primary and a fresh independent specialist covering all ten activated risk profiles. Reviewed source commit: `498f186ab8409818f7af411f3d427cc04f811360`; target fingerprint: `sha256:c65d5afa0171ea8225f53ce178d9a882d93074ef4905a23b1639040bad89838e`. Report: `/tmp/fox-mcp-conversation-gate.md`; full inventory: `/tmp/fox-mcp-conversation-inventory.json`; specialist evidence: `/tmp/fox-mcp-conversation-specialist.json`.
- Caller independently validated the unique schema-v2 envelope, target identity, unchanged source bytes, 115 inventory members, isolated reviewer topology, 12 complete contracts and 22 complete partitions, zero P0–P3 findings, zero gaps and no outgoing obligations. Both retained neutral obligations (`FU-CG-001`, text resources; `FU-CG-002`, Feishu continuity) were independently verified with unchanged originating identities. No unresolved defect, decision or evidence blocker remains. Earlier FAIL/NOT RUN entries above are historical states superseded by this completion evidence.
- Final reviewer independently reran `go test ./...`, `go test -race ./...`, `go vet ./...`, package-test and read-only format checks in an exact archive mirror; all passed. Full base-to-HEAD and worktree whitespace checks passed. Additional disposable probes verified actual Benchmark MCP/image execution plus explicit deny-all, and Feishu second-message denial without another server call or connection loss. No source or immutable fixture was changed during review.
- Actual Playwright 0.0.83 plus user-selected `glm-5.3-flash` again passed navigation/snapshot/type/click, parent/child state, screenshot persistence, both provider request encodings, disabled-image fallback and live visual analysis. The model correctly identified the orange badge. Evidence: `/tmp/fox-mcp-conversation-live.log`. The model override was test-local; persisted user credentials/settings were not changed.
- Caller scenario self-check covers all 76 enumerated scenarios with substantive assertions; all 96 test names referenced by this log resolve to current test functions. The independent review does not replace that caller check.
- The only remaining environment limitation is the original checkout's ignored historical worktrees contaminating recursive architecture scans; identical clean source passes those unchanged checks. No worktrees were deleted and no gate was weakened.
- This final commit changes task completion and evidence metadata only; production code, tests, dependencies and confirmed behavior remain exactly the reviewed source. Automatic distillation checked the new delta once: lifecycle knowledge is already represented by existing profile rules or belongs to DEC-010/design C04, so there is nothing new to distill.
