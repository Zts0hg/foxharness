# Implementation Plan: MCP Integration

**Related Spec**: [spec.md](spec.md)
**Related Design**: [design.md](design.md)
**Confirmed Requirements**: [requirements.md](requirements.md)
**Created**: 2026-10-04
**Status**: Reviewed — PASS (review-plan.md)

## Context and Scope

Implement the reviewed C01–C09 design and DD01–DD09 decisions. Deliver all three transports, authenticated remote access, all applicable runtime entries, stateful parent/child browser use, CLI/TUI management, and actual image input with compatible session persistence. Standalone Resources browsing, Prompts template entry points, and exposing Fox as an MCP server remain excluded.

This is one Go module using Go 1.25.0, existing OpenAI/Anthropic adapters, Bubble Tea TUI, and file-backed session state. The pinned MCP SDK and Playwright versions are defined in design.md; the plan does not select alternatives. Production code must follow TDD and block-level documentation. Do not edit vendor or regenerate immutable characterization fixtures. Each new dependency edge must update `docs/package-dependencies.md` and architecture enforcement together.

## Plan-Level Decisions

### PLD01 — Complete rich-result plumbing before entry rollout

- **Decision**: First establish optional content contracts/artifacts and verify old history, then provider projection, before mounting external tools into actual entry points.
- **Rationale**: A transport-only rollout would appear to support screenshot tools while discarding their images. Shared additions must remain inert for old text-only execution.
- **Alternatives**: Wiring transport into each entry first would multiply partial paths and obscure image loss.
- **Covers**: REQ-017, REQ-019, REQ-020, REQ-021; **Design**: C06, C07.

### PLD02 — Prove lifecycle with fixtures before the browser

- **Decision**: Use controlled subprocess, loopback HTTP/SSE/OAuth servers and scripted providers to establish failure, no-replay, refresh, and ownership behavior. Run pinned Playwright acceptance after those paths and entry compositions work.
- **Rationale**: Live browser/model variability must not mask lifecycle defects or make the normal Go suite network-dependent.
- **Covers**: REQ-002, REQ-008, REQ-014, REQ-016, REQ-023, REQ-024; **Design**: C02, C03, C04, C09, DD09.

### PLD03 — Migrate all entries through the same integration factory

- **Decision**: Implement the reusable C04/C05 integration and runtime ports first, then wire each verified composition root, testing policy and close behavior for each.
- **Rationale**: Avoid independent MCP clients or image/policy decisions in TUI, remote, and benchmark code. Entry-specific changes remain concrete wiring and presentation only.
- **Covers**: REQ-010, REQ-011, REQ-012; **Design**: C04, C05, C08, C09.

## Implementation Phases

### P01 — Additive content and artifact foundation

- **Outcome**: Optional rich results survive registry, execution, runtime commit, persisted history, and replay without changing old text JSON.
- **Paths**: `internal/schema/message.go`, `internal/toolprotocol/protocol.go`, `internal/tools/registry.go`, `internal/registryexec/adapter.go`, `internal/engine/target.go`, `internal/toolruntime/executor.go`, `internal/runtime/context_controller.go`, `internal/session/message_log.go`; new `internal/media`; corresponding tests and architecture documentation/enforcement.
- **Verification**: TDD for deep copies, content/error correlation, valid/invalid images, confined artifact references, replay and missing artifact notices; unchanged old history fixtures.
- **Dependencies**: None.
- **Covers**: REQ-017, REQ-020, REQ-022, NFR-001, NFR-002, NFR-006; **Design**: C06, DD06.

### P02 — Model image projection and fallback

- **Outcome**: Both providers receive actual image input; text-only model fallback and retained artifact/history behavior are explicit.
- **Paths**: `internal/modelinvoke`, `internal/provider/openai.go`, `internal/provider/claude.go`, `internal/llmconfig/config.go`, `internal/compaction`, `internal/runtime/context_controller.go`, relevant presentation DTOs.
- **Verification**: Inspect encoded outbound requests, full tool-result batch ordering, explicit capability settings, verified unsupported-image fallback, no fallback on generic errors, no repeated screenshot call, preserved compaction/replay/rewind artifacts.
- **Dependencies**: P01.
- **Covers**: REQ-019, REQ-020, REQ-021, NFR-002, NFR-006; **Design**: C07, DD06.

### P03 — Configuration and authentication mechanisms

- **Outcome**: C01 effective configuration and C02 credentials/login exist with isolated errors, atomic scoped edits, explicit login/logout, and refresh.
- **Paths**: new `internal/mcpconfig`, `internal/mcpauth`, `internal/testsupport/mcpfixture`; `go.mod`, `go.sum`, dependency documentation/enforcement.
- **Verification**: Scope precedence and missing/malformed cases, secret reference redaction, whole-server replacement, credential identity/permissions/locking, OAuth loopback state/PKCE validation, refresh persistence, logout, cancellation and absent-interaction errors.
- **Dependencies**: None; SDK pin from DD01.
- **Covers**: REQ-003, REQ-004, REQ-007, REQ-008, REQ-009, REQ-015, NFR-004, NFR-005; **Design**: C01, C02, DD01, DD03, DD07.

### P04 — Protocol transports and typed external results

- **Outcome**: Narrow SDK adapter connects all transports, discovers paginated tools, returns rich calls/resources, and supervises local process trees.
- **Paths**: new `internal/mcpclient`, `internal/testsupport/mcpfixture`; existing `internal/processtree` consumed without broadening its responsibility.
- **Verification**: Three transports and each documented protocol version; authorization headers; legacy negotiation boundaries; pagination; tool-level versus transport errors; content conversion and unsupported formats; bounded frames/images/stderr; timeout/cancellation, complete process-tree reap, no ambiguous call replay.
- **Dependencies**: P01, P03.
- **Covers**: REQ-001, REQ-002, REQ-007, REQ-014, REQ-015, REQ-016, REQ-017, REQ-018, REQ-022, REQ-024, NFR-003; **Design**: C03, DD01, DD02, DD08.

### P05 — Runtime session ownership and capability integration

- **Outcome**: Session-owned servers, borrowed child views, external capability extension before narrowing, separate per-caller authorization, per-server serialization, explicit reconnect, and reliable close.
- **Paths**: new `internal/mcpintegration`; `internal/runtime/runtime_harness.go`, `session_lifecycle.go`, `profile.go`, `child_runner.go`; `internal/childruntime/runner.go`; `internal/toolpolicy` only as required by adapter assessment; architecture document/checks.
- **Verification**: Create/Open symmetry, failed construction cleanup, immutable catalog generations, nil/empty deny-all, child persona/read-only intersection, independent sessions, child completion/cancellation versus root close, state continuity, concurrent same-server calls, configuration edit/reconnect timing, linked-resource authorization, stale adapters and no-replay errors.
- **Dependencies**: P04.
- **Covers**: REQ-001, REQ-006, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, REQ-015, REQ-016, REQ-018, NFR-001, NFR-002, NFR-003; **Design**: C04, C05, DD04, DD05.

### P06 — Management and production entry rollout

- **Outcome**: All seven execution categories use C04; users can manage scopes/authentication/status via CLI and TUI, including current-session reconnect.
- **Paths**: new `internal/app/mcp.go`, `internal/mcpcmd`, `cmd/fox/mcp.go`, `internal/tui/mcp.go`; existing `cmd/fox/main.go`, `cli_runtime.go`, `tui_runtime.go`, `autodev_runtime.go`, `cmd/feishu/runtime_execution.go`, `cmd/agentops/runtime_execution.go`, `cmd/bench/main.go`, `internal/benchmark/runtime_spec.go`, `internal/childruntime/runner.go`; slash dispatch and application interfaces as required by C08.
- **Verification**: CLI parse/output and TUI message/update tests; exact scoped CRUD/status/login/logout behavior; temporary probe cleanup; no claimed cross-process reconnect; execution/denial/result/cleanup contract matrix for every entry, unchanged no-coordinator/full-access policies, benchmark fidelity and repeat isolation.
- **Dependencies**: P02, P05.
- **Covers**: REQ-005, REQ-006, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-014, REQ-015, REQ-019, REQ-020, REQ-021, NFR-001, NFR-002; **Design**: C08, C09.

### P07 — Browser acceptance, compatibility documentation, and final verification

- **Outcome**: Documented pinned Playwright workflow and comprehensive verification evidence establish the feature's user-visible behavior.
- **Paths**: opt-in `internal/mcpintegration/playwright_integration_test.go`, new `docs/mcp.md`, relevant README links, `docs/package-dependencies.md`, `internal/architecturetest`; execution log and task evidence in this feature workspace.
- **Verification**: Real controlled-page navigation/snapshot/type/click, child continuation and screenshots; provider request image assertion; optional credentialed model demonstration with results reported honestly. Run relevant package tests, full `go test ./...`, race checks on new shared-state/lifecycle packages, and required architecture/fixture checks. Verify every enumerated task scenario maps to a substantive assertion before the complete-feature code review.
- **Dependencies**: P06.
- **Covers**: REQ-023, REQ-024, NFR-001, NFR-002, NFR-005, NFR-006; **Design**: C09, DD09.

## Verification and Delivery Rules

For every testable implementation unit: write the scenario test, observe failure for the missing behavior, implement the minimum working change, and refactor while green. Use `gofmt -w` on changed Go files. Add executable architecture allowances for deliberate new dependency categories while retaining the forbidden engine/presentation directions and immutable migration baseline.

Keep each completed unit green before committing. Do not commit unrelated user work, binaries, credentials, temporary browser profiles, or downloaded module caches. Tests with real MCP/browser/model dependencies are opt-in and must state their prerequisites; do not label a skipped live run as passed. The implementation completion gate is the full-feature `codexspec:review-code` workflow, including the independent scenario-coverage self-check.

## Risks and Delivery Trade-offs

| Risk | Delivery implication | Verification |
|---|---|---|
| Broad additive message change | P01/P02 precede entry rollout to keep baseline intact | Old fixtures, clones, compaction and actual request encoding |
| OAuth and cancellation interact | Fixture-driven P03/P04 must pass before live browser tests | Expired tokens, cancelled callback, bounded refresh/cleanup |
| Static ceilings silently filter MCP | P05 precedes entry work | Exact advertised and executable capability intersections |
| Entry-specific wiring diverges | P06 needs a per-entry contract matrix | All seven categories, permissions, results, close |
| Live model/browser unavailable | Report exact missing prerequisite, not fabricated acceptance | Separate integration and model evidence |

## Requirements Coverage

| Spec requirement | Design component | Plan coverage |
|---|---|---|
| REQ-001 | C03, C05 | P04, P05 |
| REQ-002 | C03 | P04 |
| REQ-003 | C01 | P03 |
| REQ-004 | C01 | P03 |
| REQ-005 | C01, C08 | P06 |
| REQ-006 | C04, C08 | P05, P06 |
| REQ-007 | C02, C03 | P03, P04 |
| REQ-008 | C02, C08 | P03, P06 |
| REQ-009 | C02, C08 | P03, P06 |
| REQ-010 | C04, C05, C09 | P05, P06 |
| REQ-011 | C04, C05 | P05, P06 |
| REQ-012 | C04, C05 | P05, P06 |
| REQ-013 | C04 | P05 |
| REQ-014 | C03, C04, C08 | P04, P05, P06 |
| REQ-015 | C01, C03, C04, C08 | P03–P06 |
| REQ-016 | C03, C04 | P04, P05 |
| REQ-017 | C05, C06 | P01, P04 |
| REQ-018 | C03, C05, C06 | P04, P05 |
| REQ-019 | C06, C07 | P02, P06 |
| REQ-020 | C06, C07 | P01, P02, P06 |
| REQ-021 | C06, C07 | P02, P06 |
| REQ-022 | C05, C06 | P01, P04 |
| REQ-023 | C09 | P07 |
| REQ-024 | C03, C09 | P04, P07 |
| NFR-001 | C04, C07, C09 | P01, P05, P06, P07 |
| NFR-002 | C04, C06, C07, C09 | P01, P02, P05, P06, P07 |
| NFR-003 | C04, DD08 | P04, P05 |
| NFR-004 | C02 | P03 |
| NFR-005 | C09 | P03, P07 |
| NFR-006 | C06, C07 | P01, P02, P07 |

All nine design components have implementation coverage. No architecture change or product deferral is introduced by this plan. The implementation details delegated by requirements.md are resolved in design.md rather than silently selected in tasks.
