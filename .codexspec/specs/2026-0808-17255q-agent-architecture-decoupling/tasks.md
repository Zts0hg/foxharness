# Tasks: Agent Architecture Decoupling

**Input**: `requirements.md`, `spec.md`, and approved `plan.md` in this feature directory
**Prerequisites**: `review-spec.md` and `review-plan.md` both PASS
**Workflow**: Sequential execution on the current integration branch; strict Red-Green-Refactor for new production behavior and defect corrections

## Task Format and Gate Rules

- Every task has one verifiable outcome, exact or bounded paths, dependencies, and upstream coverage.
- A production task records its Red command and expected failure, minimal Green result, Refactor result, and final Green commands.
- A mechanical move uses existing green characterization and does not create an artificial Red.
- Every dependency-changing task updates `docs/package-dependencies.md`, its Mermaid graph, the architecture test, and the decreasing allowlist in the same commit.
- No production architecture task (`T047` onward) may start before `T046` freezes `B00`.
- If a `DV-*` task proves a defect, execution stops for a separately confirmed correction requirement and independent TDD defect commit. The corrected implementation is rerun before `T045` and `T046`.
- Each `Mxx` production boundary is one independently green, reviewable, bisectable, and revertible commit unless the confirmed narrow mechanical-combination exception applies.

## Phase 0A: Characterization Infrastructure

- [x] **T001** Create `.codexspec/specs/2026-0808-17255q-agent-architecture-decoupling/characterization-trace.md` with one row for every required `RT`, `ST`, `TL`, `CX`, `PL`, `RS`, `PF-*`, `UI-*`, `EV-BEN`, `IA-CHD`, `CP-AUT`, and `DV-*` ID, including test, fixture/expected outcome, command, status, and source commit fields. Verify the ID set exactly matches `requirements.md`. Dependencies: none. Covers: NFR-005, NFR-007, NFR-010; Plan: Phase 0 trace authority.
- [x] **T002** Create the initial authoritative `docs/package-dependencies.md` with the target Mermaid DAG, responsibilities, forbidden edges, composition exceptions, interaction/observation flow, and injection points; implement the standard-library Go import parser and exact current-violation allowlist in `internal/architecturetest/imports_test.go` and `internal/architecturetest/allowlist.json`; verify new/broadened violations fail. Dependencies: T001. Covers: REQ-015, NFR-003, NFR-006, NFR-012; Plan: PLD-002 and Phase 0 architecture baseline.
- [x] **T003** Implement immutable fixture manifest types, SHA-256 verification, copy-to-temp helpers, deterministic clocks/IDs, and self-tests in `internal/testsupport/entryfixture/` and `testdata/characterization/v1/manifest.json`. Dependencies: T001. Covers: NFR-006, NFR-009; Plan: PLD-006 and Phase 0 fixtures.
- [x] **T004** Define the implementation-neutral runtime scenario DSL and adapter contract in `internal/testsupport/runtimecontract/`, with self-tests proving ordered fact, outcome, artifact, and fixture assertions. Dependencies: T001, T003. Covers: NFR-007, NFR-008; Plan: PLD-001 and Shared Characterization Harness.
- [x] **T005** Add a current-production test adapter in the closest existing engine/app test packages and prove one tool-free contract scenario can execute without exporting a production compatibility API. Dependencies: T004. Covers: NFR-005, NFR-008; Plan: Phase 0 current adapter.
- [x] **T006** Add deterministic scripted model, streaming, tool, interaction, filesystem, process, transport, messenger, local-Git, and fake-GitHub collaborators under `internal/testsupport/`; prove they use no external network, credentials, ambient HOME, fixed ports, uncontrolled clocks, or sleep-based concurrency. Dependencies: T003, T004. Covers: NFR-006, NFR-007, NFR-010; Plan: Hermetic collaborators.

**Checkpoint P0A**: Infrastructure tests pass and T001 rows identify every mandatory scenario, but no row may be marked baseline-passing without its executable test.

## Phase 0B: Residual Defect Verification and Fixture Authority

- [x] **T040** Execute comprehensive hermetic proofs for `DV-FEI-001..010` in Feishu/cmd tests, record each defect/not-defect result and evidence, and stop for separate requirements on any proven defect. Dependencies: T006. Covers: NFR-005, NFR-006, NFR-010; Plan: Feishu defect gate. **Correction stop cleared:** T074-T083 corrected all ten proven defects through independent Green commits; T041 may proceed after the complete post-correction gate.
- [x] **T074 (`D-FEI-001`)** TDD the authenticated bounded approval HTTP callback and deterministic status mapping; update `DV-FEI-001` only after Green. Dependencies: T040, DEC-042 confirmation. Covers: NFR-006, NFR-010.
- [x] **T075 (`D-FEI-002`)** TDD the atomic durable at-most-once Feishu message acceptance store, duplicate acknowledgement, restart behavior, and live rollback; update `DV-FEI-002` only after Green. Dependencies: T074. Covers: NFR-006, NFR-010.
- [x] **T076 (`D-FEI-003`)** TDD missing/blank sender rejection before reservation, lookup, and enqueue while preserving successful webhook acknowledgement; update `DV-FEI-003` only after Green. Dependencies: T075. Covers: NFR-006, NFR-010.
- [x] **T077 (`D-FEI-004`)** TDD acceptance-scoped timeout and cancellation-aware same-session waiting so expired work never starts; update `DV-FEI-004` only after Green. Dependencies: T076. Covers: NFR-006, NFR-010.
- [x] **T078 (`D-FEI-005`)** TDD per-session FIFO scheduling and global-permit fairness; update `DV-FEI-005` only after Green. Dependencies: T077. Covers: NFR-006, NFR-010.
- [x] **T079 (`D-FEI-006`)** TDD Runner drain/cancel behavior and signal-aware HTTP/task shutdown in `cmd/feishu`; update `DV-FEI-006` only after Green. Dependencies: T078. Covers: NFR-006, NFR-010.
- [x] **T080 (`D-FEI-007`)** TDD non-blocking exactly-once approval resolution and terminal cleanup; update `DV-FEI-007` only after Green. Dependencies: T079. Covers: NFR-006, NFR-010.
- [x] **T081 (`D-FEI-008`)** TDD frozen selected-model propagation to engine and compactor; update `DV-FEI-008` only after Green. Dependencies: T080. Covers: NFR-006, NFR-010.
- [x] **T082 (`D-FEI-009`)** TDD correlated panic terminal outcomes, bounded failure delivery, and cleanup; update `DV-FEI-009` only after Green. Dependencies: T081. Covers: NFR-006, NFR-010.
- [x] **T083 (`D-FEI-010`)** TDD typed delivery failures, bounded transport text, and production observation; update `DV-FEI-010` only after Green and clear the T040 correction stop. Dependencies: T082. Covers: NFR-006, NFR-010.
- [x] **T041** Execute proofs for `DV-AOP-001..006` plus reuse corrected `DV-FEI-001` and `DV-FEI-007` behavior without a second approval protocol; record each result and stop on proven defects. Dependencies: T006, T040, T074-T083. Covers: NFR-005, NFR-006, NFR-010; Plan: AgentOps defect gate. **Correction stop cleared:** T084-T089 corrected all six proven defects through independent Green commits; shared authenticated exactly-once approval reuse remains verified and T042 may proceed.
- [x] **T084 (`D-AOP-001`)** TDD composition of the shared durable Gateway `DeliveryStore` as AgentOps' sole task-acceptance authority, remove the process-local acceptance `Deduper`, and prove duplicate acknowledgement, rollback, concurrency, and restart behavior; update `DV-AOP-001` only after Green. Dependencies: T041, DEC-043. Covers: NFR-006, NFR-010.
- [x] **T085 (`D-AOP-002`)** TDD coordinated HTTP, bridge, two-channel, queued-task, and active-task shutdown with producer-safe close order, ordinary drain, cancellation outcomes, and one bounded process wait; update `DV-AOP-002` only after Green. Dependencies: T084. Covers: NFR-006, NFR-010.
- [x] **T086 (`D-AOP-003`)** TDD one Runner-owned typed terminal transition for success, ordinary failure, timeout, cancellation, and panic, including fresh bounded terminal delivery contexts and cleanup; update `DV-AOP-003` only after Green. Dependencies: T085. Covers: NFR-006, NFR-010.
- [x] **T087 (`D-AOP-004`)** TDD one immutable provider/model snapshot shared by engine, compactor, telemetry-relevant metadata, and child inheritance for each task; update `DV-AOP-004` only after Green. Dependencies: T086. Covers: NFR-006, NFR-010.
- [x] **T088 (`D-AOP-005`)** TDD typed delivery-failure observation, Runner-boundary text limits, separate task and delivery outcomes, and non-recursive terminal-send failure handling; update `DV-AOP-005` only after Green. Dependencies: T087. Covers: NFR-006, NFR-010.
- [x] **T089 (`D-AOP-006`)** TDD rooted regular-file access for `log_search` with traversal, separator, symlink, final-target, cancellation, 200-line, and one-MiB-line assertions; update `DV-AOP-006` only after Green and clear the T041 correction stop. Dependencies: T088. Covers: NFR-006, NFR-010.
- [x] **T042** Execute proofs for `DV-BEN-001..007`, distinguishing runtime, evaluation, infrastructure, cancellation, timeout, path, process-tree, output-bound, provenance, and status outcomes; record and stop on defects. Dependencies: T006, T084-T089. Covers: REQ-013, NFR-005, NFR-006, NFR-010; Plan: Benchmark defect gate. **Correction stop active:** all seven risks are proven defects; production migration and baseline freeze remain blocked until correction semantics are separately confirmed and expanded into independent TDD commits.
- [x] **T090 (`D-BEN-001`)** TDD one case-owned 600-second default deadline covering setup, runtime, validation, and reporting inputs, with distinct cancellation/timeout outcomes and synthetic ordered records for unstarted validations; prove an expired case context is not the cleanup authority and update `DV-BEN-001` only after Green. Dependencies: T042, DEC-044. Covers: REQ-013, NFR-006, NFR-010.
- [x] **T091 (`D-BEN-002`)** TDD typed repeat status, separate runtime/evaluation/infrastructure evidence, partial reporting, and process exit codes 0/1/2; update `DV-BEN-002` only after Green. Dependencies: T090. Covers: REQ-013, NFR-006, NFR-010.
- [x] **T092 (`D-BEN-003`)** TDD one immutable typed current `BenchmarkRuntimeSpec` used for construction and derived machine/human fidelity without introducing the future general profile layer; update `DV-BEN-003` only after Green. Dependencies: T091. Covers: REQ-013, NFR-006, NFR-010.
- [x] **T093 (`D-BEN-004`)** TDD rooted fixture copy and `file_contains`, source-symlink and unsupported-type rejection, source immutability, success retention, failed-workspace cleanup under one fresh 30-second context, and typed cleanup failure/timeout evidence; update `DV-BEN-004` only after Green. Dependencies: T092. Covers: REQ-013, NFR-006, NFR-010.
- [x] **T094 (`D-BEN-005`)** TDD strict repeat, case, turn-budget, YAML, fixture-resolution, and validation-field domains with deterministic error precedence; update `DV-BEN-005` only after Green. Dependencies: T093. Covers: REQ-013, NFR-006, NFR-010.
- [ ] **T095 (`D-BEN-006`)** TDD isolated validator process trees, independent one-MiB stdout/stderr bounds, overflow status, TERM-to-KILL cleanup, reaping, and ordered synthetic post-cancellation results; update `DV-BEN-006` only after Green. Dependencies: T094. Covers: REQ-013, NFR-006, NFR-010.
- [ ] **T096 (`D-BEN-007`)** TDD schema version, one-based repeat, run/case-definition/fixture identities, terminal cause, provider/model, deadline, runtime-spec provenance, normalized golden behavior, and corrected-schema stability; update `DV-BEN-007` only after Green and clear the T042 correction stop. Dependencies: T095. Covers: REQ-013, NFR-006, NFR-010.
- [ ] **T043** Execute proofs for `DV-CHD-001..006`, separating invocation-adapter and runtime-lifecycle behavior and preserving the one-level ceiling; record and stop on defects. Dependencies: T006, T090-T096. Covers: REQ-012, NFR-005, NFR-006, NFR-010; Plan: Child defect gate.
- [ ] **T044** Execute proofs for `DV-AUT-001..010` across ledger durability, stage validity, identity, materialization, paths, processes, publishing, extraction, concurrency configuration, and partial outcomes; record and stop on defects. Dependencies: T006. Covers: REQ-011, NFR-005, NFR-006, NFR-010; Plan: Autodev defect gate.
- [ ] **T045** After every proven defect has a separately approved and green correction commit, generate the immutable `testdata/characterization/v1` persisted-session and exact-output fixtures once from the corrected source commit, complete manifest semantics/hashes, and pass fixture read/copy/integrity tests. Dependencies: T040-T044 and all required defect commits. Covers: REQ-006, REQ-014, NFR-001, NFR-006, NFR-009, NFR-010; Plan: Baseline fixture authority.

**Checkpoint P0B**: Every `DV-*` item has a result, all required corrections are green, and immutable fixture bytes identify the corrected source commit.

## Phase 0C: Shared Runtime Characterization

- [ ] **T010** Implement and pass `RT-001..007` through the current adapter in `internal/testsupport/runtimecontract/` plus existing engine/app test adapters; update all seven trace rows. Dependencies: T005, T006, T045. Covers: REQ-004, REQ-005, NFR-005, NFR-007, NFR-008; Plan: Shared runtime turns.
- [ ] **T011** Implement and pass `ST-001..006`, including deterministic pre-delta fallback, post-delta failure, disabled-stream state, and cross-run cleanup; update trace rows. Dependencies: T010. Covers: REQ-004, REQ-009, NFR-005, NFR-007; Plan: Shared streaming.
- [ ] **T012** Implement and pass `TL-001..008` with synchronization barriers, immutable tool snapshots, ordering, aliases, errors, large artifacts, and cancellation; update trace rows. Dependencies: T010. Covers: REQ-005, REQ-014, NFR-005, NFR-007; Plan: Shared tool lifecycle.
- [ ] **T013** Implement and pass `CX-001..008` against fresh/resumed/compacted/rewound fixture copies and exact model-visible projections; update trace rows. Dependencies: T003, T010, T012. Covers: REQ-006, REQ-007, NFR-005, NFR-007, NFR-009; Plan: Shared context lifecycle.
- [ ] **T014** Implement and pass `PL-001..005` for recovery, reminder, completion, TODO, cooldown, suppression, and ordering; update trace rows. Dependencies: T010, T012. Covers: REQ-005, NFR-005, NFR-007; Plan: Shared policies.
- [ ] **T015** Implement and pass `RS-001..007` for lifecycle order, failures, cancellation, fatal/non-fatal persistence, serialization/isolation, exactly-once outcomes, and headless output; update trace rows. Dependencies: T010-T014. Covers: REQ-001, REQ-003, REQ-009, NFR-005, NFR-007; Plan: Shared run/session lifecycle.

**Checkpoint P0C**: Every shared row passes the corrected current implementation with one authoritative expectation set.

## Phase 0D: Profile, Adapter, and Control-Plane Characterization

- [ ] **T020** Implement `PF-TUI-001..018` in app/TUI integration tests and update trace rows. Dependencies: T010-T015. Covers: REQ-002, REQ-010, REQ-014, NFR-005, NFR-007; Plan: TUIInteractive profile.
- [ ] **T021** Implement `UI-TUI-001..006` in `internal/tui/*_test.go` and `cmd/fox/main_test.go`, preserving exact terminal/input/render/overlay/process behavior; update trace rows. Dependencies: T020. Covers: REQ-008, REQ-010, REQ-014, NFR-005, NFR-007; Plan: TUI presentation.
- [ ] **T022** Implement `PF-CLI-001..014` in app/CLI contract tests and update trace rows. Dependencies: T010-T015. Covers: REQ-002, REQ-010, REQ-014, NFR-005, NFR-007; Plan: CLIExec profile.
- [ ] **T023** Implement `UI-CLI-001..004` in `cmd/fox/main_test.go` and current CLI tests, asserting exact stdout/stderr/exit and output-before-drain behavior; update trace rows. Dependencies: T022. Covers: REQ-008, REQ-010, REQ-014, NFR-005, NFR-007; Plan: CLI presentation.
- [ ] **T024** Implement `PF-FEI-001..018` in `internal/feishu/*_test.go` using scripted runtime and messenger boundaries; update trace rows only after affected DV expectations are classified. Dependencies: T010-T015, T006. Covers: REQ-002, REQ-014, NFR-005, NFR-007, NFR-010; Plan: FeishuRemote profile.
- [ ] **T025** Implement `UI-FEI-001..007` in Feishu/cmd tests with authenticated local HTTP, duplicate, correlation, approval, delivery, and shutdown fixtures; update trace rows subject to DV conclusions. Dependencies: T024. Covers: REQ-008, REQ-009, REQ-014, NFR-005, NFR-007, NFR-010; Plan: Feishu transport.
- [ ] **T026** Implement `PF-AOP-001..019` in `internal/agentops/*_test.go` and update trace rows subject to AgentOps and shared Feishu DV conclusions. Dependencies: T010-T015, T006. Covers: REQ-002, REQ-014, NFR-005, NFR-007, NFR-010; Plan: AgentOpsTask profile.
- [ ] **T027** Implement `UI-AOP-001..006` in AgentOps/cmd tests for task mapping, queues, deduplication, exact messages, approvals, delivery failures, and shutdown; update trace rows subject to DV conclusions. Dependencies: T026. Covers: REQ-008, REQ-009, REQ-014, NFR-005, NFR-007, NFR-010; Plan: AgentOps transport.
- [ ] **T028** Implement `PF-BEN-001..016` in `internal/benchmark/*_test.go` using fresh deterministic fixture workspaces and sessions; update trace rows subject to benchmark DV conclusions. Dependencies: T010-T015, T006. Covers: REQ-002, REQ-013, REQ-014, NFR-005, NFR-007, NFR-010; Plan: BenchmarkEval profile.
- [ ] **T029** Implement `EV-BEN-001..011` in benchmark and `cmd/bench/main_test.go` for case input, validators, repeats, reports, provenance, failure precedence, and status; update trace rows subject to DV conclusions. Dependencies: T028. Covers: REQ-013, REQ-014, NFR-005, NFR-007, NFR-010; Plan: Benchmark control plane.
- [ ] **T030** Implement `PF-CHD-001..022` in `internal/subagent/*_test.go` and shared runtime tests for lineage, ceilings, cancellation, partial outcomes, and cleanup; update trace rows subject to child DV conclusions. Dependencies: T010-T015, T006. Covers: REQ-002, REQ-012, REQ-014, NFR-005, NFR-007, NFR-010; Plan: ChildRun profile.
- [ ] **T031** Implement `IA-CHD-001..006` for `delegate_task` and fork-skill schemas, normalization, assessment, invocation, and result adaptation; update trace rows. Dependencies: T030. Covers: REQ-011, REQ-012, NFR-005, NFR-007; Plan: Child invocation adapters.
- [ ] **T032** Implement `PF-AUT-001..016` with a scripted core runner and deterministic item sessions; update trace rows subject to Autodev DV conclusions. Dependencies: T010-T015, T006. Covers: REQ-002, REQ-011, REQ-014, NFR-005, NFR-007, NFR-010; Plan: AutodevPipeline profile.
- [ ] **T033** Implement `CP-AUT-001..025` in `internal/autodev/*_test.go` using local repositories and fake command/GitHub boundaries; update trace rows subject to DV conclusions. Dependencies: T032. Covers: REQ-011, REQ-014, NFR-005, NFR-007, NFR-010; Plan: Autodev control plane.
- [ ] **T034** Implement `UI-AUT-001..006` in Autodev TUI/CLI adapter tests for exact reporting, cancellation, ownership, and process status; update trace rows. Dependencies: T032, T033. Covers: REQ-008, REQ-010, REQ-014, NFR-005, NFR-007; Plan: Autodev entry adapters.

## Phase 0E: Baseline Freeze

- [ ] **T046** Freeze `B00`: complete every trace row with test, fixture/outcome, command, passing result, and source commit; verify no skips or unresolved `DV-*`; run architecture baseline tests and `go test ./...`; record the baseline evidence. Dependencies: T001-T045. Covers: REQ-001..015, NFR-001..013; Plan: Phase 0 baseline gate.

**Checkpoint B00**: Production migration is now permitted. If this checkpoint is not complete, all following tasks remain blocked.

## Phase 1: Pure and Persisted Endpoints

- [ ] **T047 (`M01`)** Move deterministic renderer code from `internal/context` to new `internal/prompt`, retain a one-way compatibility facade, update import documentation/allowlist, and pass prompt goldens plus applicable context/profile contracts. Treat the move as mechanical unless a new renderer contract requires Red evidence. Dependencies: T046. Covers: REQ-007, NFR-001, NFR-011, NFR-013; Plan: Phase 1 M01.
- [ ] **T048 (`M02`)** TDD the persistence vocabulary and storage boundary in `internal/session`: `StoredSession`, `StoredRun`, `FileStore`, `ID`, `RunID`, `TranscriptEvent`, and `TranscriptLog`; retain one-way aliases/wrappers; pass all immutable session/compaction/checkpoint/rewind fixtures and update docs/allowlist. Dependencies: T047. Covers: REQ-006, NFR-001, NFR-009, NFR-011, NFR-013; Plan: Phase 1 M02.
- [ ] **T049 (`M03`)** TDD migration of all active session working-memory behavior to `internal/memory.Store`, remove production use of duplicate `session.WorkingMemory`, and pass memory/session/compaction/checkpoint/rewind contracts without changing fixtures. Dependencies: T048. Covers: REQ-006, NFR-001, NFR-011, NFR-013; Plan: Phase 1 M03.

## Phase 2: Target Engine

- [ ] **T050 (`M04`)** Write behavior-sensitive failing target-adapter tests, then add minimal engine `RunInput`, `RunContext`, `RunOutcome`, `ModelInvoker`, `ToolExecutor`, `Conversation`, `TurnPolicy`, and `Observer` contracts in `internal/engine`; prove tool-free completion, provider failure, and fact order while no production profile uses the target. Dependencies: T049. Covers: REQ-004, REQ-005, REQ-009, NFR-003, NFR-008, NFR-011; Plan: Phase 2 M04.
- [ ] **T051 (`M05`)** TDD the target model-turn loop for tool-free completion, thinking/action, streaming/fallback, limits, usage, and provider errors; pass `RT-001..007` and `ST-001..006` through both adapters. Dependencies: T050. Covers: REQ-004, REQ-005, NFR-007, NFR-008, NFR-011; Plan: Phase 2 M05.
- [ ] **T052 (`M06`)** TDD target `ToolExecutor` snapshots, correlated calls, parallel/exclusive scheduling, errors, large results, cancellation, and order; pass `TL-001..008` through both adapters. Dependencies: T051. Covers: REQ-005, REQ-014, NFR-007, NFR-008, NFR-011; Plan: Phase 2 M06.
- [ ] **T053 (`M07`)** TDD run-scoped recovery, reminder, completion, and TODO policies and remove target cross-run policy state; pass `PL-001..005` and related isolation scenarios through both adapters. Dependencies: T052. Covers: REQ-005, NFR-007, NFR-008, NFR-011; Plan: Phase 2 M07.
- [ ] **T054 (`M08`)** Complete the engine boundary by moving target context ownership, compaction, persistence, metrics, tracing, and presentation mapping outside engine; enforce only standard-library/schema imports; pass shared target contracts and decrease the allowlist. Dependencies: T053. Covers: REQ-004, REQ-005, REQ-009, NFR-002, NFR-003, NFR-012, NFR-013; Plan: Phase 2 M08.

## Phase 3: Runtime Lifecycle

- [ ] **T055 (`M09`)** TDD seven immutable `internal/runtime.Profile` resolvers, `RunSpec`, flat snapshots, validation, and non-relaxable ceiling intersection; pass every profile snapshot scenario. Dependencies: T054. Covers: REQ-002, REQ-014, NFR-003, NFR-011; Plan: Phase 3 M09.
- [ ] **T056 (`M10`)** TDD `RuntimeHarness` session creation/opening prerequisites, `AgentSession`, `RunScope`, and consumer-owned `SessionStore`; establish runtime as sole live recoverable-state owner and pass applicable `CX`, `RS`, and fixtures. Dependencies: T055. Covers: REQ-003, REQ-006, REQ-014, NFR-001, NFR-003, NFR-011; Plan: Phase 3 M10.
- [ ] **T057 (`M11`)** TDD `ContextController` collection, prompt injection, model-visible projection, compaction proposals, resume, and rewind with one `AgentSession` commit path; pass all applicable context/profile contracts. Dependencies: T056. Covers: REQ-003, REQ-006, REQ-007, NFR-001, NFR-007, NFR-011; Plan: Phase 3 M11.
- [ ] **T058 (`M12`)** TDD complete `RuntimeHarness` wiring for engine, model, tools, policies, observer, artifacts, persistence, compaction, memory, and telemetry; pass the full target shared-runtime suite without app/presentation imports. Dependencies: T057. Covers: REQ-001, REQ-003, REQ-009, REQ-015, NFR-003, NFR-007, NFR-011; Plan: Phase 3 M12.
- [ ] **T059 (`M13`)** TDD runtime `ChildRunner` with frozen parent snapshot, lineage, depth-one validation, capability intersection, inherited permission ceiling, cancellation, partial outcome, and cleanup; pass all `PF-CHD`, `IA-CHD`, and corrected DV expectations. Dependencies: T058. Covers: REQ-012, REQ-014, NFR-007, NFR-011; Plan: Phase 3 M13.

## Phase 4: Profile-Atomic Production Cutovers

- [ ] **T060 (`M14`)** Prove old/target benchmark parity, atomically migrate `internal/benchmark` and `cmd/bench` to `RuntimeHarness`, derive fidelity from resolved profile data, and remove independent benchmark engine assembly. Dependencies: T059. Covers: REQ-011, REQ-013, REQ-014, NFR-001, NFR-008, NFR-013; Plan: Phase 4 BenchmarkEval cutover.
- [ ] **T061 (`M15`)** Prove old/target child parity, adapt `delegate_task` and fork skills through consumer-owned `subagent.Runner` to runtime `ChildRunner`, and remove every legacy child-engine construction path. Dependencies: T060. Covers: REQ-011, REQ-012, REQ-014, NFR-001, NFR-003, NFR-013; Plan: Phase 4 ChildRun cutover.
- [ ] **T062 (`M16`)** TDD narrow `internal/app` command, DTO, notification, and interaction contracts; map runtime values without reverse imports; retain only the temporary facade needed by unmigrated user-entry profiles. Dependencies: T061. Covers: REQ-008, REQ-009, NFR-002, NFR-003, NFR-011, NFR-013; Plan: Phase 4 application boundary.
- [ ] **T063 (`M17`)** Prove full CLI parity, add `internal/cli.Run`, atomically migrate every CLI production path through app/runtime, preserve exact output and extraction drain ordering, and remove old CLI wiring. Dependencies: T062. Covers: REQ-001, REQ-008, REQ-010, REQ-014, NFR-001, NFR-007, NFR-013; Plan: Phase 4 CLIExec cutover.
- [ ] **T064 (`M18`)** Prove Autodev parity, migrate the production `CoreRunnerFactory` to runtime-backed sessions, keep control-plane responsibilities in Autodev, and remove `app.AgentRunner` and independent engine dependencies from Autodev production wiring. Dependencies: T063. Covers: REQ-011, REQ-014, NFR-001, NFR-007, NFR-013; Plan: Phase 4 AutodevPipeline cutover.
- [ ] **T065 (`M19`)** Prove Feishu parity, atomically migrate typed task execution to app/runtime while retaining transport/scheduling/approval ownership in Feishu, and remove old Feishu engine assembly. Dependencies: T064. Covers: REQ-001, REQ-008, REQ-014, NFR-001, NFR-007, NFR-013; Plan: Phase 4 FeishuRemote cutover.
- [ ] **T066 (`M20`)** Prove AgentOps parity, migrate task execution to app/runtime, reuse migrated Feishu transport/approval mechanisms, retain incident/log/control responsibilities, and remove old AgentOps engine assembly. Dependencies: T065. Covers: REQ-001, REQ-008, REQ-014, NFR-001, NFR-007, NFR-013; Plan: Phase 4 AgentOpsTask cutover.
- [ ] **T067 (`M21`)** TDD TUI-facing app capabilities and migrate run/session/model/effort/memory/compaction/checkpoint/rewind state away from concrete runtime/engine/session implementations; pass affected shared, profile, and UI scenarios. Dependencies: T066. Covers: REQ-008, REQ-010, REQ-014, NFR-001, NFR-011; Plan: Phase 4 TUI runtime state.
- [ ] **T068 (`M22`)** TDD application interaction ports and migrate TUI permissions, questions, Formal Plan review, notifications, cancellation, and queue coordination without Bubble Tea types crossing inward; pass all affected ordering and stale-event scenarios. Dependencies: T067. Covers: REQ-008, REQ-009, REQ-010, REQ-014, NFR-001, NFR-011; Plan: Phase 4 TUI interactions.
- [ ] **T069 (`M23`)** Prove complete TUI parity, atomically replace `app.RunTUI` with the single public `tui.Run`, restrict `cmd/fox` to composition/dispatch/startup, and remove the old TUI production path. Dependencies: T068. Covers: REQ-001, REQ-008, REQ-010, REQ-014, NFR-001, NFR-007, NFR-013; Plan: Phase 4 TUIInteractive cutover.

## Phase 5: Cleanup and Final Compatibility Gate

- [ ] **T070 (`M24`)** Delete `app.AgentRunner`, old application assembly, `app.RunCLI`, `app.RunTUI`, and all now-unused entry facades; prove all seven profiles use confirmed runtime/application boundaries and pass relevant catalogs. Dependencies: T069. Covers: REQ-001, REQ-008, REQ-010, NFR-002, NFR-013; Plan: Phase 5 M24.
- [ ] **T071 (`M25`)** Delete the old engine, old differential adapter, Reporter chain, old mutable configuration, and cross-run state; prove target engine/runtime own the full shared suite and engine imports remain restricted. Dependencies: T070. Covers: REQ-004, REQ-005, REQ-009, NFR-003, NFR-008, NFR-013; Plan: Phase 5 M25.
- [ ] **T072 (`M26`)** Delete the `internal/context` facade, temporary session aliases/wrappers, and duplicate memory owner; prove repository-wide symbol absence and unchanged fixture/context behavior. Dependencies: T071. Covers: REQ-006, REQ-007, NFR-001, NFR-009, NFR-013; Plan: Phase 5 M26.
- [ ] **T073 (`M27`)** Finalize `docs/package-dependencies.md` Mermaid and tables against implemented imports, empty `internal/architecturetest/allowlist.json`, run all architecture/scenario/fixture tests and `go test ./...`, verify one production path per profile and no generated worktree artifact, and publish final compatibility evidence. Dependencies: T072. Covers: all REQ-001..015 and NFR-001..013; Plan: Phase 5 M27 final gate.

## Dependencies and Execution Order

- `T001-T006` establish Phase 0 infrastructure.
- Defect gates `T040-T044` run next against the current production paths. A proven defect introduces an explicit stop and a separately approved defect task/commit before continuation.
- `T045` generates immutable compatibility fixtures once from the corrected source commit.
- Shared contracts `T010-T015` then execute sequentially against those fixtures and one scenario authority.
- Profile tasks `T020-T034` depend on the applicable shared contracts and execute sequentially on the current integration branch.
- `T046` depends on every Phase 0 task and is the only `B00` freeze point.
- `T047-T073` form the confirmed sequential `M01-M27` production chain. No production task may bypass a predecessor or `B00`.
- Profile cutovers and recoverable-state ownership boundaries are never combined. Adjacent mechanical work may be combined only under DEC-041 and only when its independent evidence remains intact.

## Required Verification Record

For each checked task, record in the task/implementation log:

- exact commands and results;
- Red failure reason when TDD applies;
- Green and post-refactor results;
- affected scenario IDs and trace rows;
- fixture hashes or expected-output references;
- architecture allowlist before/after when dependencies change;
- commit hash for every `Mxx` boundary and separate defect correction.

## Coverage

| Requirement / plan deliverable | Task references | Result |
|---|---|---|
| `REQ-001` Headless runtime | T015, T058, T063, T065-T066, T069-T070, T073 | Full |
| `REQ-002` Runtime Profiles | T020, T022, T024, T026, T028, T030, T032, T055 | Full |
| `REQ-003` Runtime ownership | T015, T056-T058 | Full |
| `REQ-004` Engine | T010-T011, T050-T054, T071 | Full |
| `REQ-005` Collaborators | T010-T014, T050-T054, T071 | Full |
| `REQ-006` Session/persistence | T013, T045, T048-T049, T056-T057, T072 | Full |
| `REQ-007` Prompt/context | T013, T047, T057, T072 | Full |
| `REQ-008` Application/adapters | T021, T023, T025, T027, T034, T062-T070 | Full |
| `REQ-009` Interactions/observation | T011, T015, T025, T027, T050, T054, T058, T062, T068, T071 | Full |
| `REQ-010` Presentation entries | T021-T023, T034, T063, T067-T070 | Full |
| `REQ-011` Control clients | T029, T031-T034, T060-T064 | Full |
| `REQ-012` Child depth/capability | T030-T031, T043, T059, T061 | Full |
| `REQ-013` Benchmark fidelity | T028-T029, T042, T060 | Full |
| `REQ-014` Profile bundles | T020-T034, T045, T055-T069 | Full |
| `REQ-015` Composition/injection | T002, T054, T058, T062, T073 | Full |
| `NFR-001` Behavior compatibility | T010-T046, T047-T073 | Full |
| `NFR-002` Focused boundaries | T002, T050-T054, T062, T070 | Full |
| `NFR-003` Dependency DAG | T002, T050, T054-T062, T071, T073 | Full |
| `NFR-004` Single repo/module | T046, T073 | Full |
| `NFR-005` Complete Phase 0 | T001, T005-T046 | Full |
| `NFR-006` Hermetic tests | T002-T006, T010-T046 | Full |
| `NFR-007` Catalog coverage | T001, T004, T010-T046, affected migration tasks | Full |
| `NFR-008` Old/target contracts | T004-T005, T050-T071 | Full |
| `NFR-009` Immutable fixtures | T003, T013, T045-T049, T072-T073 | Full |
| `NFR-010` Defect separation | T024-T045 | Full |
| `NFR-011` TDD and gates | T046-T073 plus Required Verification Record | Full |
| `NFR-012` Dependency docs/allowlist | T002, every dependency-changing migration task, T073 | Full |
| `NFR-013` Atomic migration | T046-T073 | Full |
| Shared Characterization Harness | T001, T003-T015 | Full |
| Runtime Profile and adapter catalogs | T020-T034 | Full |
| Residual defect gates and corrections | T040-T044, T074-T089 | Full |
| `B00` baseline | T045-T046 | Full |
| Engine component | T050-T054 | Full |
| Runtime component | T055-T059 | Full |
| Runtime control clients | T060-T061, T064 | Full |
| Application/presentation | T062-T063, T065-T070 | Full |
| Package documentation/enforcement | T002, T073 | Full |
| Mandatory cleanup | T070-T073 | Full |

## Unmapped Tasks

None.
