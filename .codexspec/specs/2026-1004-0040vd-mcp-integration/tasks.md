# Implementation Tasks: MCP Integration

**Feature**: `2026-1004-0040vd-mcp-integration`
**Authority**: requirements.md → spec.md → design.md → plan.md
**Created**: 2026-10-04
**Status**: Reviewed — PASS (review-tasks.md)

## Execution Rules

All code tasks use Red → Green → Refactor. Write and run the scenario test before the implementation; a compile failure for a missing new API is acceptable initial Red evidence, followed by behavioral assertions once the API exists. Record each scenario's actual test name and command/outcome in implementation-log.md before marking the task complete. Format changed Go files with `gofmt -w`. Each dependency/documentation change updates `docs/package-dependencies.md` and `internal/architecturetest` in the same completed unit. Do not edit vendor, immutable characterization fixtures, or unrelated changes.

The paths below marked new are proposed by design.md. Tests sit with their package. No parallel marker is used because shared schema and architecture files overlap across phases; logical independence does not authorize concurrent edits.

## P01 — Additive Content and Artifact Foundation

### T001 — Carry rich results through the existing execution and persistence contracts

- [x] **Outcome**: Optional content parts and structured data reach committed messages and persisted replay with defensive-copy and text compatibility guarantees.
- **Paths**: `internal/schema/message.go`, `internal/toolprotocol/protocol.go`, `internal/tools/registry.go`, `internal/registryexec/adapter.go`, `internal/engine/target.go`, `internal/toolruntime/executor.go`, `internal/runtime/context_controller.go`, `internal/session/message_log.go`, corresponding tests and architecture documentation/checks.
- **Dependencies**: None.
- **Covers**: REQ-017, REQ-020, REQ-022, NFR-001, NFR-002, NFR-006; **Plan**: P01; **Design**: C06.
- **Test Scenarios**:
  - T001-S01: Text-only records retain their existing JSON and load/execute unchanged.
  - T001-S02: Rich text/image-reference/structured results retain call ID, order, and error status through registry, executor, and conversation commit.
  - T001-S03: Mutating caller-owned content slices or nested structured values after admission does not mutate snapshots, history, or outgoing messages.
  - T001-S04: Text preview truncation preserves non-text parts and structured data.
  - T001-S05: Persisting/reloading a message retains optional content fields; absent optional fields remain compatible.
- **Verification**: Focused schema/toolprotocol/tools/registryexec/engine/toolruntime/runtime/session tests and architecture tests.

### T002 — Persist and resolve bounded image artifacts

- [x] **Outcome**: Valid images are stored before message references are committed; replay resolves confined, verified references and reports missing images explicitly.
- **Paths**: new `internal/media`; `internal/toolruntime/executor.go`; content contracts from T001; corresponding tests.
- **Dependencies**: T001.
- **Covers**: REQ-017, REQ-020, REQ-021, REQ-022, NFR-006; **Plan**: P01; **Design**: C06, DD06, DD08.
- **Test Scenarios**:
  - T002-S01: Valid PNG/JPEG content produces a stable content-addressed artifact/reference and round-trips exact bytes.
  - T002-S02: Invalid MIME, malformed image, oversized image, traversal key, or escaped filesystem target is rejected with an explicit error.
  - T002-S03: Missing/corrupt referenced images yield an unavailable-image notice while text history remains usable.
  - T002-S04: Persistence failure does not commit a dangling image reference or claim successful storage.
  - T002-S05: Repeated identical content reuses its artifact; caller mutation does not alter persisted bytes.
- **Verification**: `go test ./internal/media ./internal/toolruntime ./internal/session` and affected architecture checks.

## P02 — Model Image Projection and Fallback

### T003 — Encode actual image input and explicit unsupported-model fallback

- [ ] **Outcome**: Both existing provider adapters receive actual screenshot data, with same-model text fallback only for explicit capability/verified unsupported-image conditions.
- **Paths**: `internal/modelinvoke`, `internal/provider/openai.go`, `internal/provider/claude.go`, `internal/llmconfig/config.go`, settings/config presentation where the capability is exposed; corresponding tests.
- **Dependencies**: T002.
- **Covers**: REQ-019, REQ-020, REQ-021, NFR-002, NFR-006; **Plan**: P02; **Design**: C07.
- **Test Scenarios**:
  - T003-S01: Anthropic request contains decoded-image source content in the correctly correlated tool-result block.
  - T003-S02: OpenAI request contains image content after all results in the tool batch, with source call IDs and no outstanding-call ordering violation.
  - T003-S03: Disabled image input yields a visible limitation, retained user artifact, and usable text projection without model switching.
  - T003-S04: Auto mode makes at most one same-model text fallback on a verified image-input rejection and never reexecutes the tool.
  - T003-S05: Generic network/auth/model errors do not silently strip images; old text-only requests remain unchanged.
  - T003-S06: Artifact hydration changes a request copy rather than persisted messages; missing artifact behavior remains explicit.
- **Verification**: Provider HTTP fixture request assertions and modelinvoke/llmconfig/settings tests.

### T004 — Preserve image semantics across context preparation and replay

- [ ] **Outcome**: Compaction, context budget, rewind, and replay retain correct image references and do not count image input as free or claim omitted images were viewed.
- **Paths**: `internal/compaction`, `internal/runtime/context_controller.go`, `internal/runtimecompaction`, `internal/session`; corresponding tests.
- **Dependencies**: T003.
- **Covers**: REQ-020, REQ-021, NFR-002, NFR-006; **Plan**: P02; **Design**: C07.
- **Test Scenarios**:
  - T004-S01: Retained image-bearing turns preserve references through context cloning/projection and contribute a nonzero image budget.
  - T004-S02: Compacting an earlier image turn does not delete its durable artifact; transcript replay still resolves it.
  - T004-S03: Rewind removes/restores message references according to sequence without deleting artifacts still needed by history.
  - T004-S04: Existing text-only compaction and rewind fixtures remain unchanged.
- **Verification**: Compaction/runtime/runtimecompaction/session tests and immutable fixture checks.

## P03 — Configuration and Authentication

### T005 — Implement scoped configuration and atomic management mutations

- [ ] **Outcome**: Effective server definitions resolve predictably with validated inputs and explicit scope.
- **Paths**: new `internal/mcpconfig`; `go.mod`, `go.sum` for the pinned SDK as needed; dependency documentation/checks.
- **Dependencies**: None.
- **Covers**: REQ-003, REQ-004, REQ-005, REQ-006, REQ-015; **Plan**: P03; **Design**: C01, DD03.
- **Test Scenarios**:
  - T005-S01: Missing scopes are empty; user-only/project-only/different-name entries resolve correctly.
  - T005-S02: Project entry replaces the entire same-named user definition, including absent credentials and disabled state.
  - T005-S03: Invalid transport/URL/command/timeout or missing environment secret identifies only the affected server as unavailable.
  - T005-S04: Invalid JSON is reported and is not overwritten by a management mutation.
  - T005-S05: Add/remove/enable/disable mutate only the specified scope, preserve unrelated definitions, and reject conflicting updates safely.
  - T005-S06: Resolved snapshots remain unchanged after configuration edits or caller-map mutation; secret values are redacted from display/errors.
- **Verification**: Configuration tests with temporary homes/workspaces and concurrency/atomic-write checks.

### T006 — Implement persisted credentials, explicit OAuth login/logout, and refresh

- [ ] **Outcome**: Static and OAuth authentication work with user-local secret storage and bounded interactive/noninteractive behavior.
- **Paths**: new `internal/mcpauth`, new OAuth helpers under `internal/testsupport/mcpfixture`; SDK pin in `go.mod`/`go.sum`; corresponding architecture updates.
- **Dependencies**: T005.
- **Covers**: REQ-007, REQ-008, REQ-009, NFR-004, NFR-005; **Plan**: P03; **Design**: C02, DD07.
- **Test Scenarios**:
  - T006-S01: No-auth/static-header/bearer sources resolve without exposing credentials in diagnostics.
  - T006-S02: Authorization-code flow completes against a loopback fixture with valid state/issuer/PKCE and persists credentials/refresh configuration.
  - T006-S03: Invalid state/issuer, denied authorization, malformed discovery, or cancelled login returns an error and closes the listener without storing a token.
  - T006-S04: Restart/reload reuses and refreshes a token; concurrent refresh is serialized and updated credentials are persisted atomically.
  - T006-S05: Endpoint/client/scope identity changes do not reuse another service's credential; local logout removes and invalidates the selected credential.
  - T006-S06: Unattended authorization requirements return promptly without invoking a browser prompt; refresh rejection is actionable.
  - T006-S07: Credential files are user-local, protected, and excluded from project configuration; lock/write failure is explicit.
- **Verification**: Hermetic OAuth/credential tests and race checks; no live account login in ordinary tests.

## P04 — Protocol Transports and Results

### T007 — Connect stdio, Streamable HTTP, and legacy SSE with bounded lifecycle

- [ ] **Outcome**: The pinned SDK adapter negotiates supported protocols, discovers all tool pages, and owns process/HTTP cleanup without uncertain call replay.
- **Paths**: new `internal/mcpclient`, fixture transport/server code under `internal/testsupport/mcpfixture`; consume `internal/processtree`.
- **Dependencies**: T002, T006.
- **Covers**: REQ-001, REQ-002, REQ-007, REQ-014, REQ-015, REQ-016, REQ-024, NFR-003; **Plan**: P04; **Design**: C03, DD01, DD02, DD08.
- **Test Scenarios**:
  - T007-S01: Each transport completes initialize/discovery/call/close with correct call correlation and every documented negotiated protocol version.
  - T007-S02: Paginated discovery returns the complete catalog; malformed pages and unsupported versions identify the failing server.
  - T007-S03: HTTP automatic negotiation falls back only during eligible handshake failures; authorization failure does not switch transports.
  - T007-S04: Static/refreshed credentials reach the intended HTTP endpoint for both variants and are not leaked across redirects/origins.
  - T007-S05: Startup timeout, call cancellation, frame overflow, and stderr overflow remain bounded and explicit; subprocess descendants are reaped on owner close/failure.
  - T007-S06: A server executes a call then drops the response; Fox reports uncertainty and does not send that call again.
  - T007-S07: Closing twice is safe; ending a call context does not cancel an otherwise healthy owning session.
- **Verification**: Transport fixtures, subprocess process-tree tests, short injected deadlines, race checks.

### T008 — Convert rich results and authorized resource payloads

- [ ] **Outcome**: Protocol results map to the rich Fox contracts without false success or silent loss.
- **Paths**: `internal/mcpclient` result conversion/read support, existing rich contracts and fixtures.
- **Dependencies**: T007.
- **Covers**: REQ-017, REQ-018, REQ-022; **Plan**: P04; **Design**: C03, C05, C06.
- **Test Scenarios**:
  - T008-S01: Text, structured JSON, image, embedded text/image resources, and links preserve order, metadata, and originating call.
  - T008-S02: MCP tool-level isError and transport errors remain distinguishable and are never advertised as successful results.
  - T008-S03: Unsupported audio/unknown content and invalid/oversized image payloads produce explicit indications while preserving usable supported parts where available.
  - T008-S04: Resource reads use the originating session and decode supported text/image contents; unknown/unavailable resources return explicit errors.
- **Verification**: Table-driven rich-result fixture tests and media persistence integration.

## P05 — Runtime Ownership and Capabilities

### T009 — Add owned/borrowed session services and safe lifecycle transitions

- [ ] **Outcome**: New and reopened sessions own services, child sessions borrow, and reconnect/close preserve correct generations and state boundaries.
- **Paths**: `internal/runtime/runtime_harness.go`, `session_lifecycle.go`, `child_runner.go`; new `internal/mcpintegration` host; existing architecture enforcement/documentation.
- **Dependencies**: T008.
- **Covers**: REQ-006, REQ-012, REQ-013, REQ-014, REQ-015, REQ-016, NFR-001, NFR-003; **Plan**: P05; **Design**: C04, DD04.
- **Test Scenarios**:
  - T009-S01: Both CreateSession/OpenSession initialize services and keep connections across normal runs; nil factory preserves old behavior.
  - T009-S02: Partial factory failure closes created resources and releases session admission/identity leases.
  - T009-S03: Independent root sessions use distinct connections; child completion/cancellation leaves the parent's borrowed connection alive.
  - T009-S04: Concurrent calls to the same server serialize; queue wait observes timeout/cancellation without dispatching cancelled work.
  - T009-S05: Config edits do not alter established connections; explicit reconnect waits for active calls, applies the whole new definition, and rejects stale adapters.
  - T009-S06: Closing the root ends owned connections/process trees under a fresh bounded cleanup context; errors are visible and repeat-close is safe.
  - T009-S07: One failed server leaves healthy servers/internal tools usable; replacing a session reports potential browser-state loss and never replays uncertain calls.
- **Verification**: Runtime/host fixture integration and race tests.

### T010 — Mount external tools within existing capability and permission boundaries

- [ ] **Outcome**: Discovered tools and linked-resource helpers are callable only within effective host/profile/child restrictions, with frozen matching definitions and dispatch.
- **Paths**: `internal/mcpintegration/tools.go`, `internal/runtime/profile.go`, `internal/runtime/child_runner.go`, `internal/childruntime/runner.go`; `internal/toolpolicy` and registry adapters as required.
- **Dependencies**: T009.
- **Covers**: REQ-001, REQ-010, REQ-011, REQ-012, REQ-018, NFR-002; **Plan**: P05; **Design**: C04, C05, DD05.
- **Test Scenarios**:
  - T010-S01: Host-generated names preserve originating-server routing, fit provider constraints, and reject collisions with existing or external tools.
  - T010-S02: External capabilities extend the ceiling before narrowing; unregistered arbitrary names never become executable.
  - T010-S03: Nil restriction preserves allowed defaults while explicit empty intersections advertise/execute no tools, including child/persona restrictions.
  - T010-S04: Writable authorized children continue the parent's browser state with their own permission checks; unknown-effect tools are denied to read-only children.
  - T010-S05: Server read-only metadata does not grant fast approval; full-access and existing no-coordinator policy behavior remains unchanged.
  - T010-S06: Linked-resource reads require a known origin link and originating-tool permission; unknown URI, cross-server origin, and forbidden caller are denied before dispatch.
- **Verification**: Exact advertised/executable tool matrix, permission fixture tests, child lineage/ceiling regression tests.

## P06 — Management and Entry Rollout

### T011 — Add shared management ports, CLI commands, and TUI interactions

- [ ] **Outcome**: Users can manage configuration scopes, inspect redacted actual/probed state, login/logout, and reconnect the active TUI session.
- **Paths**: new `internal/app/mcp.go`, `internal/mcpcmd`, `cmd/fox/mcp.go`, `internal/tui/mcp.go`; existing main/slash/application dispatch and corresponding tests.
- **Dependencies**: T010.
- **Covers**: REQ-005, REQ-006, REQ-008, REQ-009, REQ-014, REQ-015, NFR-001; **Plan**: P06; **Design**: C08.
- **Test Scenarios**:
  - T011-S01: CLI and TUI actions add/remove/enable/disable only the explicitly selected scope and display effective precedence.
  - T011-S02: Status distinguishes disabled/connecting/ready/auth-required/failed/closed and redacts credentials.
  - T011-S03: CLI probes close temporary connections and say they are probes; no command claims cross-process reconnect.
  - T011-S04: TUI reconnect applies the new generation after active calls finish and surfaces failures/state-loss notices.
  - T011-S05: Login URL/callback/cancel/logout is presented through the shared service; a missing interaction returns an actionable error.
  - T011-S06: Unavailable or malformed configurations produce useful errors without blocking unrelated UI/CLI operations.
- **Verification**: CLI parsing/output tests and Bubble Tea message/update tests with injected management ports; architecture tests.

### T012 — Wire all production entries and image-aware presentation

- [ ] **Outcome**: All applicable entry compositions use shared MCP integration, preserve their policies, and pass rich results to the actual model path.
- **Paths**: `cmd/fox/cli_runtime.go`, `tui_runtime.go`, `autodev_runtime.go`; `cmd/feishu/runtime_execution.go`, `cmd/agentops/runtime_execution.go`, `cmd/bench/main.go`; `internal/benchmark/runtime_spec.go`, `internal/childruntime/runner.go`; app/remote result DTOs as required by C06/C09.
- **Dependencies**: T004, T011.
- **Covers**: REQ-010, REQ-011, REQ-012, REQ-014, REQ-015, REQ-019, REQ-020, REQ-021, NFR-001, NFR-002; **Plan**: P06; **Design**: C06, C07, C09.
- **Test Scenarios**:
  - T012-S01: CLIExec composition discovers/calls a permitted MCP tool, receives rich content, and closes the owned session.
  - T012-S02: TUIInteractive preserves MCP state across turns and new-session/reconnect boundaries while using its existing approval flow.
  - T012-S03: FeishuRemote permits/denies calls through existing remote approvals, reuses its session, and handles unavailable interaction explicitly.
  - T012-S04: AgentOpsTask isolates task-owned MCP state and preserves remote approval and terminal cleanup.
  - T012-S05: AutodevPipeline uses the item workspace and full-access/no-human policy, closing resources per attempt.
  - T012-S06: BenchmarkEval uses a frozen case/repeat MCP capability snapshot, records fidelity, and isolates repeats without requiring live services in ordinary tests.
  - T012-S07: ChildRun borrows parent state without recursive delegation or broader capabilities and preserves child-local artifact references.
  - T012-S08: Representative real entry runs pass image content to both configured provider families and expose the explicit non-image fallback; output never dumps base64.
- **Verification**: Entry composition contract tests, provider fixture assertions, existing entry/architecture regressions.

## P07 — Acceptance and Final Verification

### T013 — Add and run pinned Playwright acceptance

- [ ] **Outcome**: A separately runnable workflow proves the complete browser sequence and screenshot model-input behavior against a controlled local page.
- **Paths**: new `internal/mcpintegration/playwright_integration_test.go` and local fixture page; documented pinned dependency/prerequisite data.
- **Dependencies**: T012.
- **Covers**: REQ-023, REQ-024, NFR-005; **Plan**: P07; **Design**: C09, DD09.
- **Test Scenarios**:
  - T013-S01: Pinned Playwright in an isolated browser navigates a local page, returns a snapshot, enters text, clicks, and observes the expected result across consecutive turns.
  - T013-S02: Authorized writable child continues the same page; child completion leaves it available to the parent.
  - T013-S03: Screenshot bytes reach an image-capable scripted provider as actual image input and persist for replay.
  - T013-S04: Text-only model mode retains the screenshot for user access and explicitly reports no visual analysis.
  - T013-S05: The opt-in live-model demonstration receives the screenshot and reports the known visual page property; absence of credentials/prerequisites is reported as not run, never passed.
- **Verification**: Run the opt-in browser suite with prerequisites; record exact pins, commands, and results. Ordinary suite must still skip live dependencies by design.

### T014 — Document configuration, compatibility, and architecture

- [ ] **Outcome**: Users can reproduce MCP configuration/authorization/Playwright setup and understand state, permissions, image fallback, and supported versions.
- **Paths**: new `docs/mcp.md`, relevant README links, `docs/package-dependencies.md`, feature implementation evidence.
- **Dependencies**: T013.
- **Covers**: REQ-024, NFR-001; **Plan**: P07; **Design**: C09, DD01, DD09.
- **Verification**: Deterministically check referenced paths/commands/config examples against the implementation and pins. This documentation task has no artificial unit-test scenarios.

### T015 — Establish full verification and complete-feature review

- [ ] **Outcome**: All task scenarios map to substantive test assertions, required checks are green, and the complete-feature strict review reaches a valid terminal verdict.
- **Paths**: Whole feature diff and tests; `implementation-log.md` and `issues.md` in this workspace as needed.
- **Dependencies**: T014.
- **Covers**: REQ-001–024, NFR-001–006; **Plan**: P07; **Design**: C01–C09.
- **Verification**: `gofmt` checks, `go test ./...`, focused race checks for new shared-state/lifecycle packages, architecture and fixture checks; scenario-to-test self-check; exact `$codexspec:review-code --feature .codexspec/specs/2026-1004-0040vd-mcp-integration` with required isolated topology. Repair only verified defects using TDD and repeat the required checks. This verification task consumes the scenarios above and does not invent duplicate scenarios.

## Coverage and Dependencies

| Plan component | Task coverage | Scenario mapping |
|---|---|---|
| P01 / C06 | T001, T002 | T001-S01–05, T002-S01–05 |
| P02 / C07 | T003, T004 | T003-S01–06, T004-S01–04 |
| P03 / C01, C02 | T005, T006 | T005-S01–06, T006-S01–07 |
| P04 / C03 | T007, T008 | T007-S01–07, T008-S01–04 |
| P05 / C04, C05 | T009, T010 | T009-S01–07, T010-S01–06 |
| P06 / C08, C09 | T011, T012 | T011-S01–06, T012-S01–08 |
| P07 / C09 | T013, T014, T015 | T013-S01–05; documentation and aggregate verification |

| Requirement | Tasks |
|---|---|
| REQ-001 | T007, T010 |
| REQ-002 | T007 |
| REQ-003 | T005 |
| REQ-004 | T005 |
| REQ-005 | T005, T011 |
| REQ-006 | T005, T009, T011 |
| REQ-007 | T006, T007 |
| REQ-008 | T006, T011 |
| REQ-009 | T006, T011 |
| REQ-010 | T010, T012 |
| REQ-011 | T010, T012 |
| REQ-012 | T009, T010, T012 |
| REQ-013 | T009 |
| REQ-014 | T007, T009, T011, T012 |
| REQ-015 | T005, T007, T009, T011, T012 |
| REQ-016 | T007, T009 |
| REQ-017 | T001, T002, T008 |
| REQ-018 | T008, T010 |
| REQ-019 | T003, T012 |
| REQ-020 | T001, T002, T003, T004, T012 |
| REQ-021 | T002, T003, T004, T012 |
| REQ-022 | T001, T002, T008 |
| REQ-023 | T013 |
| REQ-024 | T007, T013, T014 |
| NFR-001 | T001, T009, T011, T012, T014 |
| NFR-002 | T001, T003, T004, T010, T012 |
| NFR-003 | T007, T009 |
| NFR-004 | T006 |
| NFR-005 | T006, T013 |
| NFR-006 | T001, T002, T003, T004 |

All implementation tasks are mapped. T015 is necessary aggregate verification and complete-feature review support. Each declared dependency precedes its dependent; T005 is independent of P01/P02, but no parallel agent execution is prescribed. No implementation task is marked complete at generation time.
