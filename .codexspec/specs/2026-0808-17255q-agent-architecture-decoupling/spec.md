# Feature Specification: Agent Architecture Decoupling

**Feature Branch**: `2026-0808-17255q-agent-architecture-decoupling`
**Created**: 2026-08-09
**Status**: Draft
**Input**: Confirmed requirements in `requirements.md` through `DEC-041`

## Context and Goals

Fox currently combines agent orchestration, runtime state, application entry behavior, and presentation concerns across boundaries that expose implementation details and amplify unrelated change. This refactor reorganizes those responsibilities inside the existing repository and Go module. It is a non-functional architecture change: all existing user interactions, runtime profiles, module behavior, persisted session data, remote entry points, benchmark behavior, and Autodev behavior remain compatible.

The target is a headless, reusable agent runtime with an infrastructure-independent turn engine, one recoverable session-state owner, explicit immutable Runtime Profiles, focused application and presentation adapters, independently replaceable concrete capabilities, and an executable package dependency contract. A complete hermetic characterization baseline must exist before any production architecture moves.

### Goals

- Establish small, stable, responsibility-focused package boundaries with an enforced acyclic dependency graph.
- Make the core agent runtime reusable without TUI, Bubble Tea, terminal, or remote-transport dependencies.
- Preserve all current behavior and persisted-data compatibility across seven explicit Runtime Profiles.
- Give recoverable session state, prompt rendering, runtime context lifecycle, engine turns, observation, artifacts, and telemetry unambiguous owners.
- Enable isolated future development of different modules through stable contracts, while performing this refactor sequentially on one integration branch.
- Make every migration boundary testable, traceable, reviewable, bisectable, and revertible.

## User Scenarios and Testing

### User Story 1 - Existing behavior remains unchanged (Priority: P1)

As a Fox user or runtime client, I want the refactored build to behave exactly like the corrected pre-refactor baseline so that I can upgrade without changing workflows, permissions, tools, sessions, automation, or integrations.

**Independent Test**: Execute every applicable shared, profile-specific, presentation, transport, evaluation, child-invocation, and Autodev-control characterization scenario against the frozen baseline and the migrated implementation using the same authoritative expectations and immutable fixtures.

**Acceptance Scenarios**:

1. **Given** an existing CLI, TUI, Feishu, AgentOps, benchmark, child-run, or Autodev workflow, **when** the same controlled inputs are executed before and after migration, **then** observable output, ordered facts, interaction behavior, outcomes, artifacts, and persisted state match the confirmed baseline.
2. **Given** a versioned session or output fixture from the frozen baseline, **when** the refactored implementation opens, continues, compacts, rewinds, or reports it, **then** the fixture remains usable and its compatibility-significant fields retain their semantics.
3. **Given** an unverified behavior difference or residual risk, **when** characterization is prepared, **then** the implementation records current behavior without changing it unless the separate defect workflow proves and explicitly approves a correction.

### User Story 2 - Modules evolve behind stable boundaries (Priority: P1)

As a maintainer, I want engine, runtime, application, presentation, persistence, prompt, tool, provider, telemetry, benchmark, child-run, and Autodev responsibilities to have explicit contracts so that one module can be changed or replaced without modifying unrelated modules.

**Independent Test**: Run automated import-architecture tests, package contract tests, and repository import analysis after each migration boundary.

**Acceptance Scenarios**:

1. **Given** the target package graph, **when** imports are analyzed, **then** every import follows the allowed DAG or a composition-root-only exception.
2. **Given** a concrete provider, tool executor, compactor, session store, or telemetry implementation, **when** it is replaced with a contract-compatible test implementation, **then** engine and unrelated adapters require no source change.
3. **Given** two future module changes in isolated worktrees, **when** they target different package owners, **then** stable contracts minimize shared implementation-file changes and neither module depends on the other's concrete internals.

### User Story 3 - Every entry resolves an explicit runtime contract (Priority: P1)

As an entry-adapter or runtime-control-client developer, I want each execution to resolve a flat immutable Runtime Profile and a typed per-run specification so that intentional differences remain explicit and security ceilings cannot drift.

**Independent Test**: Snapshot all seven resolved profiles and verify the applicable catalog for each profile using controlled `RunSpec` values.

**Acceptance Scenarios**:

1. **Given** one of the seven profile names, **when** it is resolved, **then** session lifecycle, workspace, model scope, budget, scheduling, tools, interactions, permissions, state, context, compaction, observation, and completion policy form one immutable snapshot.
2. **Given** a `RunSpec`, **when** it selects or narrows behavior, **then** it cannot expand a profile capability or security ceiling.
3. **Given** a model-visible tool definition snapshot, **when** a call is authorized, scheduled, and executed, **then** definitions, aliases, permission assessment, parallel-safety lookup, and executable tools agree for that immutable snapshot.

### User Story 4 - User interfaces remain adapters over a headless runtime (Priority: P2)

As a presentation maintainer, I want TUI, non-interactive CLI, Feishu, and AgentOps behavior to use UI-neutral application capabilities so that presentation can evolve without becoming runtime logic.

**Independent Test**: Instantiate application and runtime capabilities without a presentation observer, then exercise each adapter through its own black-box presentation or transport catalog.

**Acceptance Scenarios**:

1. **Given** a headless run with no presentation observer, **when** it executes, **then** runtime and engine write no terminal, ANSI, Bubble Tea, Lark, or adapter-formatted output.
2. **Given** a synchronous permission, question, or plan interaction, **when** the runtime pauses, **then** a correlated request/response port handles response, cancellation, and timeout rather than a one-way notification bus.
3. **Given** TUI or CLI startup, **when** composition completes, **then** `cmd/fox` only selects and starts `tui.Run` or `cli.Run`; presentation workflows stay in their adapters.

### User Story 5 - Runtime control clients share the real harness (Priority: P2)

As a benchmark, subagent, or Autodev maintainer, I want direct access to the shared runtime contracts without presentation indirection or independent engine assembly so that evaluation and automation remain faithful to product behavior.

**Independent Test**: Resolve and run `BenchmarkEval`, `ChildRun`, and `AutodevPipeline` through `RuntimeHarness` and verify their profile plus control-plane or invocation-adapter catalogs.

**Acceptance Scenarios**:

1. **Given** a benchmark case, **when** it runs, **then** controlled providers, tools, sessions, budgets, validations, and fidelity metadata use the real runtime contracts while declared benchmark differences remain explicit.
2. **Given** root-level `delegate_task` or fork-skill invocation, **when** it creates a child, **then** runtime creates one isolated depth-one child and rejects every descendant-creation path.
3. **Given** an Autodev item, **when** its agent stages execute, **then** core runs use the shared runtime while ledger, worktree, stages, gates, Engineer supervision, and publication remain Autodev control-plane responsibilities.

## Functional Requirements

### REQ-001: Headless runtime execution

The system MUST provide a core runtime that can create or open sessions and execute runs without importing or requiring the TUI, Bubble Tea, terminal presentation, Feishu, or AgentOps transport. CLI, TUI, Feishu, AgentOps, benchmark, child-run, and Autodev execution MUST reuse this runtime according to their confirmed boundaries.

- **Sources**: NEED-003, DEC-001, DEC-004, DEC-014, DEC-017

### REQ-002: Explicit Runtime Profiles and per-run inputs

The runtime MUST expose exactly seven named behavior profiles: `TUIInteractive`, `CLIExec`, `FeishuRemote`, `AgentOpsTask`, `BenchmarkEval`, `ChildRun`, and `AutodevPipeline`. Each MUST resolve to a flat immutable snapshot. Profiles define defaults, permitted variation, and non-relaxable ceilings; `RunSpec` or equivalent typed input carries dynamic run values and may only select or narrow permitted behavior.

- **Sources**: NEED-005, DEC-018, DEC-019

### REQ-003: Runtime lifecycle ownership

`internal/runtime` MUST own `RuntimeHarness`, `AgentSession`, `RunSpec`, `RunScope`, `Profile`, `ContextController`, `ChildRunner`, and the consumer-owned `SessionStore` port as one cohesive package. `RuntimeHarness` contains only immutable configuration, concurrency-safe shared dependencies, and factories. `AgentSession` exclusively coordinates recoverable session state and session operations. `RunScope` owns run-scoped mutable collaborators and state. These lifetimes MUST NOT overlap or leak state across runs or sessions.

- **Sources**: DEC-010, DEC-013, DEC-025

### REQ-004: Infrastructure-independent turn engine

`internal/engine` MUST contain the readable run/turn coordinator and its consumer-owned contracts: `AgentEngine`, `RunInput`, `RunOutcome`, `RunContext`, `ModelInvoker`, `ToolExecutor`, `TurnPolicy`, `Conversation`, and `Observer`. The engine owns turn and phase transitions, terminal states, and hard turn-limit enforcement, retains no mutable state across runs, and does not own provider mechanics, tool implementation, context lifecycle, compaction, persistence, or telemetry sinks.

- **Sources**: DEC-009, DEC-015, DEC-026

### REQ-005: Exclusive engine collaborator responsibilities

`ModelInvoker` MUST own provider invocation, streaming, fallback, and normalized model-call facts. `ToolExecutor` MUST operate on one immutable constrained tool snapshot and return ordered structured results without persisting session messages. `TurnPolicy` MUST decide completion, TODO, reminder, and recovery behavior without selecting tools or mutating persistence. `RunContext` MUST be an immutable model-visible snapshot for one invocation. `Conversation`, implemented by runtime context control, MUST request ordered context changes without becoming persistence authority.

- **Sources**: DEC-009, DEC-015, DEC-026

### REQ-006: Recoverable session and persistence boundary

Runtime `AgentSession` MUST be the only coordinator that commits state needed for restore, continuation, rewind, and compaction. `internal/session` MUST remain the persistence package and provide `FileStore`, `StoredSession`, `StoredRun`, `ID`, `RunID`, `MessageRecord`, `MessageLog`, `CompactState`, `TranscriptEvent`, and `TranscriptLog` with their confirmed semantics and compatible encodings. Storage implementations perform storage mechanics only. The duplicate `session.WorkingMemory` MUST be removed in favor of `memory.Store`.

- **Sources**: CON-001, DEC-010, DEC-021

### REQ-007: Pure prompt rendering and runtime context lifecycle

`internal/context` MUST become `internal/prompt`. The target package MUST only represent, order, and deterministically render side-effect-free prompt fragments. Runtime `ContextController` MUST own complete-context collection, selection, injection decisions, projection, and compaction coordination; `AgentSession` MUST commit recoverable context changes in compatibility-preserving order.

- **Sources**: DEC-020, DEC-025

### REQ-008: Application capability boundary

`internal/app` MUST remain the application package and contain only user-entry use cases, UI-neutral commands and DTOs, ordered runtime-notification adaptation, and explicit interaction ports. TUI, CLI, Feishu, and AgentOps adapters MUST use small application capabilities and MUST NOT operate concrete engine, runtime, session, checkpoint, compaction, provider, tool, memory, or persistence implementations.

- **Sources**: DEC-004, DEC-011, DEC-023

### REQ-009: Bidirectional interactions and ordered observation

One-way typed notifications MAY carry progress. Permission approval, user questions, and plan review MUST use explicit correlated request/response ports with cancellation and timeout semantics. Engine facts MUST be emitted synchronously in canonical order, mapped once through runtime observation and application DTOs, and consumed by adapters. Session artifacts and best-effort telemetry MUST remain distinct from authoritative recovery state and from each other.

- **Sources**: DEC-011, DEC-012, DEC-026, DEC-027

### REQ-010: Presentation entry ownership

The TUI presentation workflow MUST have one public `tui.Run` entry in `internal/tui`. Non-interactive `fox exec` and `fox -p` presentation MUST use `cli.Run` in `internal/cli`. `cmd/fox` MAY parse process input, resolve configuration, construct dependencies, and dispatch modes, but MUST NOT own presentation or runtime workflows. CLI output MUST preserve its staged output-before-extraction-drain and exit-after-drain behavior.

- **Sources**: DEC-017, DEC-024, DEC-035

### REQ-011: Runtime control clients

Benchmark, child delegation, and Autodev MUST use `RuntimeHarness` directly as runtime control clients and MUST NOT pass through the user-facing application layer or independently assemble an engine. Benchmark evaluation, model-facing child invocation protocols, and Autodev deterministic orchestration MUST remain in their focused packages outside runtime.

- **Sources**: DEC-005, DEC-006, DEC-014, DEC-025

### REQ-012: Single-level child execution

Every child creation path MUST use runtime `ChildRunner`. A root run may create one depth-one child. A child MUST neither expose delegation capabilities nor create a descendant through any tool, skill, adapter, or internal caller. Runtime depth validation and child capability filtering MUST both enforce this ceiling. Child cancellation, permission evidence, capability ceilings, budgets, and result correlation MUST follow the confirmed `ChildRun` contract.

- **Sources**: DEC-006, DEC-007, DEC-016, DEC-039

### REQ-013: Benchmark runtime fidelity

Benchmark MUST remain a privileged evaluation and feedback client of the core runtime. It MAY inject benchmark-specific dependencies and controls through harness contracts, but MUST share runtime security and capability invariants. Runtime-fidelity metadata MUST derive from the resolved runtime specification and declared differences rather than independently maintained claims.

- **Sources**: DEC-005, DEC-014, DEC-038

### REQ-014: Profile behavior bundles

Each Runtime Profile MUST preserve the exact lifecycle, persisted source, workspace and model scope, budget, scheduling, capability surface, interaction and permission semantics, recoverable state, memory, compaction, observation, completion, and cleanup behavior defined by the Confirmed Runtime Profile Matrix and the profile's confirmed characterization catalogs in `requirements.md`.

- **Sources**: NEED-005, CON-001, CON-006, DEC-018, DEC-034, DEC-035, DEC-036, DEC-037, DEC-038, DEC-039, DEC-040

### REQ-015: Composition and concrete dependency injection

Concrete providers, tool executors and catalogs, compactors, session stores, memory, and telemetry implementations MUST remain in focused responsibility packages and be injected through consumer-owned contracts. Composition roots may import both sides only to construct, connect, select, and start an entry; they MUST NOT execute or retain workflow state after construction.

- **Sources**: DEC-001, DEC-022, DEC-027

## Non-Functional Requirements

### NFR-001: No functional or persisted-data change

The final refactor MUST preserve every externally observable user interaction and module behavior, including CLI and TUI behavior, permission semantics, tool sets and behavior, session persistence, rewind, compaction, remote entry points, benchmark, child-run, and Autodev behavior. Existing persisted sessions MUST remain readable and usable. Internal Go APIs MAY change only when all repository consumers migrate and behavioral and persisted-data compatibility remains proven.

- **Sources**: NEED-004, CON-001, CON-006, DEC-003, OUT-002

### NFR-002: Strong, independently evolvable package boundaries

Packages MUST have one cohesive responsibility, explicit state ownership and failure policy, and small stable contracts so modules can be changed or replaced independently. The design MUST avoid redundant forwarding wrappers, overlapping owners, package-per-interface proliferation, a generic event bus, and abstractions introduced only to reduce file size.

- **Sources**: NEED-001, NEED-002, CON-004, DEC-001, DEC-015, DEC-025

### NFR-003: Enforced package dependency DAG

The target import graph MUST be acyclic and follow DEC-027. Presentation adapters may depend on `app`; `app` may depend on `runtime`; `runtime` may depend on `engine`, persistence records in `session`, and pure `prompt`; `engine` may depend only on the standard library and narrow protocol values in `schema`. Runtime control clients may depend directly on runtime. `subagent.Tool` depends on a consumer-owned runner port, and composition maps it to runtime `ChildRunner` without a package cycle.

- **Sources**: NEED-001, NEED-006, DEC-026, DEC-027

### NFR-004: Single repository and delivery unit

The implementation MUST remain in one Git repository and one Go module, be developed sequentially on the current integration branch with module-focused commits, and be delivered as one final PR to `main`.

- **Sources**: NEED-002, CON-003, DEC-002, OUT-001

### NFR-005: Complete Phase 0 baseline before production movement

Before any production package, dependency direction, runtime, application boundary, or entry point moves, Phase 0 MUST implement and trace every applicable confirmed scenario, complete every `DV-*` result, incorporate separately approved defect fixes, pass the full suite against the corrected current implementation, and freeze the source commit as `B00`. Coverage percentage, skipped scenarios, document-only claims, or narrow unit approximations cannot satisfy this gate.

- **Sources**: NEED-004, NEED-007, CON-005, DEC-030, DEC-033, DEC-034, DEC-035, DEC-036, DEC-037, DEC-038, DEC-039, DEC-040, DEC-041

### NFR-006: Hermetic mandatory tests

Every commit, phase, PR, and compatibility gate MUST be deterministic, isolated, repeatable, self-contained, and offline. Mandatory tests MUST use scripted providers, fixed clocks and identifiers, test-owned temporary files or local repositories, bounded process fixtures, and test-owned local servers as needed. They MUST NOT require real external services, credentials, ambient user state, fixed ports, background processes, uncontrolled time or randomness, test order, or prior test state, and MUST NOT skip because external configuration is absent.

- **Sources**: CON-007, DEC-031

### NFR-007: Normative characterization catalog coverage

Every row in these confirmed `requirements.md` sections is a mandatory acceptance scenario and is incorporated into this specification by its stable ID and exact expected behavior: Shared Runtime (`RT-001..007`, `ST-001..006`, `TL-001..008`, `CX-001..008`, `PL-001..005`, `RS-001..007`); TUI (`PF-TUI-001..018`, `UI-TUI-001..006`); CLI (`PF-CLI-001..014`, `UI-CLI-001..004`); Feishu (`PF-FEI-001..018`, `UI-FEI-001..007`); AgentOps (`PF-AOP-001..019`, `UI-AOP-001..006`); Benchmark (`PF-BEN-001..016`, `EV-BEN-001..011`); Child (`PF-CHD-001..022`, `IA-CHD-001..006`); and Autodev (`PF-AUT-001..016`, `CP-AUT-001..025`, `UI-AUT-001..006`). A summary in this specification MUST NOT weaken an individual catalog row.

- **Sources**: NEED-005, NEED-007, DEC-032, DEC-033, DEC-034, DEC-035, DEC-036, DEC-037, DEC-038, DEC-039, DEC-040

### NFR-008: Shared old/new black-box contracts

Shared runtime scenarios MUST use one implementation-neutral authority for controlled input, expected facts, outcomes, and artifacts. The current and target implementations MUST use separate test-only adapters against that same suite. Entry-specific presentation and transport tests retain their own exact formatting and process behavior. The old test adapter remains only until all migrated profiles have parity; no old internal API becomes a permanent production compatibility requirement.

- **Sources**: NEED-007, DEC-003, DEC-032, DEC-041

### NFR-009: Immutable compatibility fixtures

Persisted-data and output compatibility MUST use versioned immutable fixtures generated once from `B00`, committed with source commit, profile or entry source, and expected semantics. Tests copy fixtures before mutation and never regenerate them with the implementation under test. Updating a fixture requires a separately approved behavior or format change.

- **Sources**: CON-001, CON-006, CON-007, DEC-031

### NFR-010: Residual-defect verification and separation

Every `DV-FEI-001..010`, `DV-AOP-001..006`, `DV-BEN-001..007`, `DV-CHD-001..006`, and `DV-AUT-001..010` item MUST receive comprehensive hermetic verification before its profile baseline freezes. A proven defect blocks `B00` until correction semantics are separately confirmed, a behavior-sensitive regression test demonstrates Red, an independent defect commit reaches Green, and all affected scenarios pass. An unproven risk records current behavior and authorizes no change. Repairs already merged through PRs `#61` and `#63` MUST NOT be duplicated.

- **Sources**: CON-005, DEC-030

### NFR-011: Strict TDD and commit verification

Every new package, port, state coordinator, behavior implementation, and defect correction MUST follow verifiable Red-Green-Refactor. Red evidence MUST identify the command and expected behavior-sensitive failure. Green adds only enough production code to pass while relevant characterization, package, and architecture tests remain green. Refactor may improve structure only while behavior and fixtures remain unchanged. Pure mechanical moves need no artificial Red. Every final module commit must be complete, green, independently reviewable, bisectable, revertible, and pass affected package and profile tests; the final PR must pass `go test ./...` and full compatibility verification.

- **Sources**: CON-002, CON-008, DEC-002, DEC-041

### NFR-012: Authoritative dependency documentation and enforcement

`docs/package-dependencies.md` MUST be the single human-readable package-dependency authority and use Mermaid plus concise tables or prose for responsibilities, allowed and forbidden imports, composition exceptions, interaction flow, observation mapping, and injection points. Boundary changes MUST update this document and automated import tests in the same commit. Architecture tests begin with the exact `B00` violation allowlist, never add or broaden entries, remove resolved entries during migration, and finish with an empty allowlist.

- **Sources**: NEED-006, DEC-027, DEC-028, DEC-029

### NFR-013: Profile-atomic migration

Production migration MUST follow `B00` and `M01` through `M27` in the confirmed bottom-up order. Before a profile switches, old and target test adapters pass the same applicable scenarios. A profile cutover commit switches all production construction and invocation paths and removes obsolete production wiring, leaving exactly one production path. Temporary facades exist only for unmigrated consumers, never become extension points, identify their deletion boundary, and never expand the allowlist.

- **Sources**: CON-002, DEC-002, DEC-041

## Runtime Profile Contract

The detailed Confirmed Runtime Profile Matrix in `requirements.md` is normative under REQ-014. The following condensed matrix identifies the planning boundary and does not replace its individual cells.

| Profile | Lifecycle | Distinct capability and interaction rules | State and observation distinction |
|---|---|---|---|
| `TUIInteractive` | Long-lived CLI-source session; serialized runs; launch-fixed workspace; mutable future-run model and effort. | Full product-root tools, delegation, run restrictions, Formal Plan, questions, and interactive permission modes. | Memory, automemory, checkpoints, rewind, automatic/manual compaction, ordered deltas and lifecycle presentation. |
| `CLIExec` | One synchronous CLI-source run with current session-selection behavior. | Product-root tools and delegation; no question, Formal Plan, or permission coordinator. | Checkpoints but no rewind entry; automatic compaction; output before extraction drain and exit after drain. |
| `FeishuRemote` | Chat-and-sender Feishu-source continuation; same-session serialization; bounded global scheduling. | No skill/question/plan; remote `ModeAsk` approval; root delegation. | No checkpoint/rewind; asynchronous extraction; remote progress and final messages without deltas. |
| `AgentOpsTask` | Fresh Feishu-source session per task; bounded global scheduling. | Incident tools plus `log_search`; remote `ModeAsk`; no skill/question/plan. | No checkpoint/rewind; asynchronous extraction; task result and artifact reporting. |
| `BenchmarkEval` | Fresh CLI-source session and fixture workspace per serial repeat. | Fixed evaluation tool set; no delegation or human interaction. | Session memory only; structured runtime and validation results with derived fidelity metadata. |
| `ChildRun` | Fresh child session with parent lineage; synchronous depth-one execution. | Read and Bash; conditional write/edit; allowed-tool intersection; inherited permission ceiling; no delegation or direct interaction. | Isolated read-only memory inputs; child-local compaction; one high-density result to parent. |
| `AutodevPipeline` | Fresh CLI-source core session per item attempt; serial items; durable control-plane resume. | Product-root runtime tools; Engineer answers questions; deterministic SDD operations remain outside runtime. | Runtime memory/checkpoints plus separate durable ledger; automatic compaction; line-oriented control reporting. |

## Package and Dependency Contract

### Required responsibilities

| Package or classification | Required responsibility |
|---|---|
| `internal/schema` | Narrow model protocol values only: messages, usage, tool definitions, calls, and results. |
| `internal/engine` | Infrastructure-independent turn coordinator and consumer-owned turn contracts. |
| `internal/runtime` | Harness construction, immutable profile resolution, session/run lifecycle, context-injection decisions, recoverable-state coordination, and child-run control. |
| `internal/app` | User-entry commands, DTOs, notification mapping, and interaction ports. |
| `internal/tui`, `internal/cli`, Feishu, AgentOps | Presentation or transport adaptation over application capabilities. |
| `internal/prompt` | Pure deterministic prompt-fragment representation, ordering, and rendering. |
| `internal/session` | Stored records, identifiers, transcript/message artifacts, and concrete file storage mechanics. |
| Provider, tool, compaction, checkpoint, memory, automemory, metrics, tracing packages | Focused concrete mechanisms injected through consumer-owned contracts. No aggregate `internal/infrastructure` package. |
| Benchmark, subagent, Autodev | Runtime control, invocation adaptation, or deterministic control-plane behavior outside user presentation and generic runtime ownership. |
| `cmd/*` | Process configuration, composition, entry selection, and startup only. |

### Allowed dependency flow

```text
presentation adapters -> app -> runtime -> engine -> schema
runtime              -> session records, prompt, and injected capability contracts
benchmark/autodev    -> runtime
subagent.Tool        -> subagent.Runner <-composition-> runtime.ChildRunner
cmd/*                -> relevant adapters, runtime constructors, and concrete implementations for wiring only
```

Any reverse import, runtime-to-presentation callback, engine-to-concrete implementation import, runtime-to-`app` import, direct model-facing subagent-to-runtime binding, or post-construction workflow in `cmd/*` is forbidden.

## Acceptance Catalog Contract

The scenario IDs in NFR-007 are normative references to the exact rows in `requirements.md`. Planning and tasks MUST preserve the following ownership split:

| Catalog | Verification owner | Required evidence |
|---|---|---|
| `RT`, `ST`, `TL`, `CX`, `PL`, `RS` | Shared runtime black-box contract | Controlled inputs, ordered facts, outcomes, and persisted or artifact effects against old and target adapters. |
| `PF-*` | Resolved profile plus runtime/application wiring | Immutable snapshot, capability ceilings, lifecycle, runtime outcome, persistence, cleanup, and profile-specific behavior. |
| `UI-TUI`, `UI-CLI`, `UI-FEI`, `UI-AOP`, `UI-AUT` | Presentation or transport adapter | Exact input, terminal/transport output, formatting, interaction state, process behavior, and exit semantics. |
| `EV-BEN` | Benchmark evaluation control plane | Case loading, fixtures, validation, repeats, aggregation, provenance, reports, and process status. |
| `IA-CHD` | Child invocation adapters | `delegate_task` and fork-skill schema, assessment, normalization, invocation, ceilings, and result adaptation. |
| `CP-AUT` | Autodev control plane | Backlog, ledger, worktree, stages, verification, gates, Engineer review, publication, recovery, cleanup, and reporting. |
| `DV-*` | Pre-baseline defect verification | A deterministic proof result; if defective, a separate approved requirement, Red evidence, defect commit, and corrected baseline. |

## Migration and Commit Contract

| Boundary | Required result before proceeding |
|---|---|
| `B00` | Complete corrected characterization baseline, fixtures, `DV-*` conclusions, trace report, and initial exact architecture allowlist. |
| `M01` | Pure `internal/prompt`; temporary `internal/context` facade only. |
| `M02` | Stored-session vocabulary and `FileStore`; persistence fixtures unchanged. |
| `M03` | `memory.Store` is the only working-memory owner. |
| `M04` | Target engine contracts established through TDD without a production profile cutover. |
| `M05` | Target model-turn, thinking/action, streaming, fallback, limits, usage, and provider outcomes pass `RT`/`ST`. |
| `M06` | Target `ToolExecutor` behavior passes every applicable `TL` contract. |
| `M07` | Turn policies and run-scoped policy state pass applicable `PL` and isolation contracts. |
| `M08` | Engine boundary excludes context ownership, persistence, compaction, telemetry, and concrete infrastructure. |
| `M09` | Seven immutable profiles, `RunSpec`, ceiling validation, and snapshots. |
| `M10` | Runtime `AgentSession`, `RunScope`, and `SessionStore`; runtime owns recoverable live state. |
| `M11` | Runtime `ContextController`; context, compaction, resume, and rewind parity. |
| `M12` | Shared `RuntimeHarness`; complete target shared-runtime suite. |
| `M13` | Runtime `ChildRunner`; parent snapshot, lineage, depth, capability, cancellation, and cleanup parity. |
| `M14` | `BenchmarkEval` and `cmd/bench` cut over; independent benchmark engine assembly removed. |
| `M15` | Child invocation consumers cut over; legacy child engine construction removed. |
| `M16` | Narrow typed `internal/app`; temporary facade only for unmigrated user-entry profiles. |
| `M17` | `CLIExec` cut over atomically; old CLI path removed. |
| `M18` | `AutodevPipeline` cut over; direct `app.AgentRunner` and independent engine assembly removed. |
| `M19` | `FeishuRemote` cut over atomically; old remote runtime assembly removed. |
| `M20` | `AgentOpsTask` cut over atomically and reuse migrated Feishu transport/approval mechanisms. |
| `M21` | TUI runtime-facing session, model, memory, compaction, checkpoint, and rewind state use application contracts. |
| `M22` | TUI interactions, permissions, plan review, notifications, cancellation, and queueing use application ports. |
| `M23` | TUI entry cut over atomically to `tui.Run`; `cmd/fox` composition only; old TUI production path removed. |
| `M24` | Delete `app.AgentRunner`, `app.RunCLI`, `app.RunTUI`, old application assembly, and entry facades. |
| `M25` | Delete old engine, differential adapter, duplicate reporter pipeline, and old mutable engine state. |
| `M26` | Delete context facade, temporary session aliases/wrappers, and duplicate memory owners. |
| `M27` | Final dependency documentation, empty architecture allowlist, complete compatibility report, and full repository verification. |

Adjacent boundaries may be combined only when they are purely mechanical and cross neither a profile cutover nor recoverable-state ownership boundary. Every profile cutover and state-owner change remains an independent commit.

## Expected Error and Failure Behavior

- A missing, skipped, externally dependent, nondeterministic, or failing mandatory scenario fails its commit or phase gate; production migration cannot begin before `B00` is complete.
- An invalid `RunSpec` or attempted ceiling expansion fails before model invocation or tool execution and cannot partially mutate a session.
- Provider, tool, persistence, compaction, cancellation, timeout, and observer outcomes retain the exact characterized ordering and fatal/non-fatal policy. Authoritative session-write failures remain fatal; transcript, metrics, and tracing failures retain current warning behavior.
- A child depth greater than one is rejected by runtime even if an invocation path or future tool attempts it; depth-one child capability snapshots contain no delegation operation.
- A profile cutover that fails parity is not activated. There is no runtime flag that exposes old and target production paths simultaneously.
- A characterization test that proves a residual defect stops baseline freeze. The correction is not folded into a refactor commit.
- Compatibility fixtures and expected outputs cannot be changed to make target code pass. Any intended behavior or format change requires a separate confirmed requirement.

## Key Entities

- **Runtime Profile**: Immutable defaults, allowed variation, behavior policy, and non-relaxable capability and security ceilings for one execution class.
- **RunSpec**: Typed dynamic input selecting or narrowing profile behavior for one run.
- **RuntimeHarness**: Immutable reusable construction boundary and factory for sessions and shared dependencies.
- **AgentSession**: Live runtime coordinator and sole committer of recoverable session state.
- **RunScope**: Per-run cancellation, observer, permission, budget, model snapshot, and mutable run state.
- **RunContext**: Immutable model-visible projection for one model invocation.
- **ContextController**: Runtime owner of complete-context preparation, injection decisions, projections, and ordered context-change requests.
- **StoredSession / StoredRun**: Persistence records that contain durable values but own no runtime lifecycle.
- **Engine Fact / Run Notification**: Canonically ordered runtime observation transformed once into application-owned UI-neutral DTOs.
- **Session Artifact**: Transcript or other non-authoritative run material with distinct persistence and failure semantics.
- **Telemetry Journal**: Best-effort metrics or tracing sink that cannot own recoverable behavior.

## Success Criteria

- **SC-001**: Every mandatory scenario ID in NFR-007 maps to at least one executable hermetic test, authoritative fixture or expected result, command, and passing `B00` result.
- **SC-002**: Every `DV-*` item has a recorded proof outcome; every proven defect has separate approval, Red evidence, a defect-focused commit, and a passing corrected baseline.
- **SC-003**: All seven Runtime Profiles have independently passing immutable snapshots and their exact applicable shared and profile-specific catalogs.
- **SC-004**: At every profile cutover, old and target test adapters pass the same scenario authority while production exposes exactly one path.
- **SC-005**: Existing versioned sessions and artifacts remain readable and behaviorally compatible through resume, continuation, compaction, checkpoint, rewind, and reporting as applicable.
- **SC-006**: Automated import tests report no forbidden target dependency at `M27`, and the temporary architecture allowlist is empty.
- **SC-007**: `docs/package-dependencies.md`, its Mermaid graphs, and executable architecture tests describe the same implemented boundaries at every dependency-changing commit.
- **SC-008**: Every module commit passes its relevant package, contract, profile, and architecture tests; final `go test ./...` and complete compatibility verification pass.
- **SC-009**: The final repository remains one Go module, contains no aggregate `internal/infrastructure` package or generic event bus, and has no obsolete production engine, runner, or compatibility facade.

## Confirmed Constraints and Decisions

- Internal Go APIs and package locations may evolve; external behavior, module behavior, and persisted data may not.
- The final user transition occurs once when the single PR is merged; temporary migration compatibility exists only inside the integration branch for unmigrated code and differential tests.
- Prompt rendering is not complete-context ownership; persisted session records are not live runtime sessions; benchmark and subagent are not presentation capabilities.
- The TUI remains Fox-specific. This work does not extract a generic terminal UI library.
- Infrastructure is a classification, not a package destination.
- Reference-project features that Fox does not currently provide do not become compatibility requirements.

## Open Questions

None. `OPEN-001` through `OPEN-004` are resolved by `DEC-027`, `DEC-017`, `DEC-041`, and `DEC-016` respectively.

## Out of Scope

- Splitting runtime and UI into separate repositories or Go modules, independent publication, or a cross-module version matrix. Sources: OUT-001.
- New or changed user-facing behavior, permissions, tools, persisted-session formats, or entry-point semantics unless separately specified and approved. Sources: OUT-002, CON-001.
- A generic `pi-tui`-style terminal UI library. Sources: DEC-017.
- Multi-level, background, resumable, worktree-isolated, or general agent-tree child execution. Sources: DEC-007, DEC-016, DEC-039.
- Reference-only Codex or Claude Code capabilities named as exclusions in `requirements.md`, including unrelated RPC/WebSocket, MCP environment, rollout, cloud-task, hook, microcompaction, context-collapse, budget, structured-output, team, and multi-worker features. Sources: DEC-033 through DEC-040.
- Repeating vulnerability fixes already merged in PRs `#61` and `#63`. Sources: DEC-030.

## Assumptions

None. This specification requires no unconfirmed assumption to compile the confirmed requirements.

## Requirements Traceability

| Confirmed entry | Spec coverage | Result |
|---|---|---|
| `NEED-001` | NFR-002, NFR-003 | Full |
| `NEED-002` | NFR-002, NFR-004 | Full |
| `NEED-003` | REQ-001 | Full |
| `NEED-004` | NFR-001, NFR-005 | Full |
| `NEED-005` | REQ-002, REQ-014, NFR-007 | Full |
| `NEED-006` | NFR-003, NFR-012 | Full |
| `NEED-007` | NFR-005, NFR-007, NFR-008 | Full |
| `CON-001` | REQ-006, REQ-014, NFR-001, NFR-009, Out of Scope | Full |
| `CON-002` | NFR-011, NFR-013 | Full |
| `CON-003` | NFR-004 | Full |
| `CON-004` | NFR-002 | Full |
| `CON-005` | NFR-005, NFR-010 | Full |
| `CON-006` | REQ-014, NFR-001, NFR-009 | Full |
| `CON-007` | NFR-006, NFR-009 | Full |
| `CON-008` | NFR-011 | Full |
| `DEC-001` | REQ-001, REQ-015, NFR-002, NFR-004 | Full |
| `DEC-002` | NFR-004, NFR-011, NFR-013 | Full |
| `DEC-003` | NFR-001, NFR-008 | Full |
| `DEC-004` | REQ-001, REQ-008 | Full |
| `DEC-005` | REQ-011, REQ-013 | Full |
| `DEC-006` | REQ-011, REQ-012 | Full |
| `DEC-007` | REQ-012, Out of Scope | Full |
| `DEC-009` | REQ-004, REQ-005 | Full |
| `DEC-010` | REQ-003, REQ-006 | Full |
| `DEC-011` | REQ-008, REQ-009 | Full |
| `DEC-012` | REQ-009 | Full |
| `DEC-013` | REQ-003 | Full |
| `DEC-014` | REQ-001, REQ-011, REQ-013 | Full |
| `DEC-015` | REQ-004, REQ-005, NFR-002 | Full |
| `DEC-016` | REQ-012, Out of Scope | Full |
| `DEC-017` | REQ-001, REQ-010, Out of Scope | Full |
| `DEC-018` | REQ-002, REQ-014 | Full |
| `DEC-019` | REQ-002 | Full |
| `DEC-020` | REQ-007 | Full |
| `DEC-021` | REQ-006 | Full |
| `DEC-022` | REQ-015 | Full |
| `DEC-023` | REQ-008 | Full |
| `DEC-024` | REQ-010 | Full |
| `DEC-025` | REQ-003, REQ-007, REQ-011, NFR-002 | Full |
| `DEC-026` | REQ-004, REQ-005, REQ-009, NFR-003 | Full |
| `DEC-027` | REQ-009, REQ-015, NFR-003, NFR-012 | Full |
| `DEC-028` | NFR-012 | Full |
| `DEC-029` | NFR-012 | Full |
| `DEC-030` | NFR-005, NFR-010, Out of Scope | Full |
| `DEC-031` | NFR-006, NFR-009 | Full |
| `DEC-032` | NFR-007, NFR-008 | Full |
| `DEC-033` | NFR-005, NFR-007, Out of Scope | Full |
| `DEC-034` | REQ-014, NFR-005, NFR-007 | Full |
| `DEC-035` | REQ-010, REQ-014, NFR-005, NFR-007 | Full |
| `DEC-036` | REQ-014, NFR-005, NFR-007 | Full |
| `DEC-037` | REQ-014, NFR-005, NFR-007 | Full |
| `DEC-038` | REQ-013, REQ-014, NFR-005, NFR-007 | Full |
| `DEC-039` | REQ-012, REQ-014, NFR-005, NFR-007 | Full |
| `DEC-040` | REQ-014, NFR-005, NFR-007 | Full |
| `DEC-041` | NFR-005, NFR-008, NFR-011, NFR-013, Migration and Commit Contract | Full |
| `OUT-001` | NFR-004, Out of Scope | Full |
| `OUT-002` | NFR-001, Out of Scope | Full |

`DEC-008` is intentionally absent from binding coverage because it is superseded by confirmed `DEC-009`. All resolved `OPEN-*` entries are represented in Open Questions and do not introduce requirements.
