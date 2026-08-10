# Confirmed Requirements: agent-architecture-decoupling

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0808-17255q`
**Status**: Discovery
**Last Confirmed**: 2026-08-09 20:11 +0800

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Strongly decoupled package boundaries

- **Status**: confirmed
- **Statement**: The agent project MUST separate modules through small, stable, responsibility-focused package interfaces so that a module can evolve or be replaced without requiring unrelated modules to change or altering their behavior.
- **Rationale**: The current orchestration, application, and TUI layers expose implementation details across boundaries and amplify change cost.
- **User Evidence**: "Modules should be flexibly iterated and replaced without affecting other modules."
- **Confirmed At**: 2026-08-08 17:52 +0800

### NEED-002: Independently developable modules

- **Status**: confirmed
- **Statement**: Package ownership and dependency boundaries MUST make it possible to develop different modules independently in isolated worktrees when future parallel development requires it, although this refactor itself will use one development worktree.
- **Rationale**: Independent module development requires stable contracts and minimal overlap in implementation files.
- **User Evidence**: "Different modules should support isolated development in different worktrees at the same time."
- **Confirmed At**: 2026-08-08 17:52 +0800

### NEED-003: Headless core agent runtime

- **Status**: confirmed
- **Statement**: The core agent runtime MUST run without depending on the TUI, Bubble Tea, or another concrete user-interface implementation, and MUST remain reusable by CLI, Feishu, AgentOps, benchmark, and subagent entry points.
- **Rationale**: User-interface replacement and runtime evolution should have independent change reasons.
- **User Evidence**: The user confirmed separation of the core agent runtime from the user-interaction layer while retaining all current entry points.
- **Confirmed At**: 2026-08-08 17:52 +0800

### NEED-004: Pre-refactor behavioral characterization

- **Status**: confirmed
- **Statement**: Before production code is reorganized, a complete and comprehensive functional and behavioral characterization test baseline MUST cover all system functions and user interactions affected by the refactor so that behavior equivalence can be verified before and after the change.
- **Rationale**: This refactor is driven exclusively by non-functional maintainability requirements and must not introduce functional changes.
- **User Evidence**: "Before implementing the refactor, complete functional and behavioral tests are needed to ensure functionality does not change before and after refactoring."
- **Confirmed At**: 2026-08-08 17:52 +0800

### NEED-005: Explicit runtime-profile behavior matrix

- **Status**: confirmed
- **Statement**: Every product entry point and runtime control client MUST resolve through an explicit runtime profile whose session lifecycle, workspace scope, capability surface, permission semantics, context and memory behavior, execution budgets, scheduling, and observation behavior are completely represented and snapshot-verifiable. The matrix in this record is the behavior-preservation authority for profile extraction, subject to separately confirmed pre-existing defect fixes under CON-005.
- **Rationale**: Shared runtime assembly is safe only when intentional differences remain explicit and cannot drift behind independent constructors.
- **User Evidence**: The user requested a complete, non-sampled Runtime Profile behavior matrix and confirmed the seven-profile matrix and its Profile/RunSpec boundary.
- **Confirmed At**: 2026-08-08 20:25 +0800

### NEED-006: Authoritative package-dependency documentation

- **Status**: confirmed
- **Statement**: The target module responsibilities, allowed dependency graph, forbidden dependencies, composition-only exceptions, bidirectional interaction flow, observation mapping flow, and concrete implementation injection points MUST be documented as an authoritative architecture contract maintained with the code.
- **Rationale**: Strong package boundaries cannot remain reliably reviewable or independently evolvable when their dependency rules exist only in implementation details or transient design discussion.
- **User Evidence**: The user explicitly required module dependencies to be documented and allowed either Markdown with Mermaid or draw.io as the representation.
- **Confirmed At**: 2026-08-09 12:25 +0800

### NEED-007: Complete pre-refactor characterization baseline

- **Status**: confirmed
- **Statement**: Before any production architecture is moved, a complete executable characterization baseline MUST cover and trace every behavior-preservation obligation in CON-001, every cell of the seven-profile runtime matrix, persisted-data compatibility, and the critical success, failure, cancellation, timeout, ordering, and recovery paths affected by the refactor. Code-coverage percentages MAY be reported as supporting evidence but MUST NOT substitute for behavioral traceability.
- **Phase 0 Acceptance Gate**: Phase 0 MUST implement every applicable shared `RT`, `ST`, `TL`, `CX`, `PL`, and `RS` scenario; every confirmed `PF-*` profile scenario; every `UI-*` presentation or transport scenario; and every `EV-BEN`, `IA-CHD`, and `CP-AUT` control or invocation scenario. Every scenario ID MUST map to one or more executable tests and its authoritative fixture or expected outcome. A document-only claim, unit-test approximation that omits the required black-box behavior, coverage percentage, skipped test, missing-credential skip, environment-specific exception, or unresolved scenario MUST NOT satisfy the gate.
- **Baseline Completion**: All mandatory tests MUST satisfy CON-007, pass against the current production implementation, and pass under `go test ./...`. Immutable persisted-data and output fixtures MUST satisfy DEC-031 and identify their source production commit, profile or entry source, and expected semantics. Every `DV-*` item MUST have a completed verification result. A proven defect blocks baseline freeze until its correction semantics are separately confirmed, its regression test demonstrates Red, its independent defect commit is Green, and every affected characterization scenario passes against the corrected implementation. An unproven risk records the verified current behavior without authorizing a change. The final Phase 0 evidence MUST provide a reviewable mapping from scenario ID to test, fixture when applicable, execution command, and passing result, tied to the frozen baseline commit.
- **Permitted Phase 0 Changes**: Phase 0 MAY add characterization and architecture tests, immutable fixtures, test-only current-implementation adapters, deterministic fakes and local harnesses, the exact initial architecture-violation allowlist required by DEC-029, and separately approved defect corrections under CON-005. It MUST NOT move production packages, change production dependency direction, introduce the target runtime or application structure, migrate an entry point, or combine baseline construction with the production architecture change being characterized. If a required behavior cannot be observed through a black-box or test-only adapter, any production observability change requires separate confirmation before implementation and MUST NOT become an implicit refactor.
- **Refactor Start Condition**: No production architecture refactor commit may begin until the complete Phase 0 gate is accepted. The required order is complete characterization coverage, comprehensive `DV-*` verification, any separately approved defect corrections, re-execution and freeze of the corrected baseline, and only then the first production architecture migration commit.
- **Rationale**: The refactor changes ownership and call paths across the entire agent harness; package tests alone cannot prove that cross-package user and module behavior remains equivalent.
- **User Evidence**: The user required all relevant tests to be completed before refactoring production code, reiterated that functionality and interaction behavior must remain unchanged, and explicitly confirmed that the complete characterization suite is a zero-stage gate because no observable standard exists for detecting refactor regressions without it.
- **Confirmed At**: 2026-08-09 13:38 +0800
- **Last Amended**: 2026-08-09 20:00 +0800

## Constraints

### CON-001: No functional or behavioral change

- **Status**: confirmed
- **Statement**: The final refactor merged into `main` MUST preserve every externally observable user interaction and module behavior, including CLI and TUI interactions, permission semantics, tool availability and behavior, session persistence, rewind, compaction, remote entry points, and benchmark behavior. Existing persisted sessions MUST remain readable and usable. Any intentional behavior change requires a separate, explicitly approved requirement.
- **User Evidence**: "The refactor only adjusts code organization for non-functional requirements and must not affect any functionality."

### CON-002: Verification at commit and PR boundaries

- **Status**: confirmed
- **Statement**: Every module-focused commit MUST pass the relevant package tests. The final PR MUST pass the complete test suite and explicit compatibility verification against the pre-refactor behavioral baseline.
- **User Evidence**: "Every module commit should pass the relevant package tests, and the final PR must pass complete testing and compatibility verification."

### CON-003: Single repository and Go module

- **Status**: confirmed
- **Statement**: The refactor MUST retain one Git repository and one Go module while enforcing strong package boundaries.
- **User Evidence**: "Use a single repository, a single Go module, and strong package boundaries."

### CON-004: Minimal, non-redundant abstractions

- **Status**: confirmed
- **Statement**: The target architecture MUST validate every proposed collaborator and package against a distinct responsibility, state ownership, failure policy, and reason to change. Redundant wrappers, overlapping ownership, package proliferation, generic event buses, and abstractions created only to reduce file size MUST NOT be introduced. The result MUST improve maintainability, extensibility, readability, and local testability.
- **User Evidence**: The user required the proposed decomposition to be checked for reasonableness and redundancy so that it serves the refactor's maintainability, extensibility, and readability goals.

### CON-005: Validate and separate pre-existing defect fixes from refactoring

- **Status**: confirmed
- **Statement**: Before production architecture migration begins, every potential pre-existing defect discovered during architecture review MUST be comprehensively verified rather than sampled. At minimum, verification MUST cover child capability inheritance, child compactor model configuration, rewind and compact-state consistency, benchmark runtime-fidelity drift, and child or benchmark termination behavior. A confirmed defect MUST receive a separate requirement, a regression test that fails for the defect, and an independent defect-focused commit before refactoring that affected behavior. The post-fix version becomes the behavior baseline for refactoring. An unconfirmed risk MUST NOT be used to change behavior during refactoring.
- **FeishuRemote Verification Gate**: The following potential defects MUST each receive a hermetic, deterministic verification test before the `FeishuRemote` behavior baseline is frozen. A test that proves a defect triggers the separate requirement, Red-Green-Refactor correction, and defect-focused commit workflow above. A test that does not prove a defect records the verified current behavior without authorizing a change.

| ID | Potential pre-existing defect to verify comprehensively |
|---|---|
| `DV-FEI-001` | The remote approval resolver exists, but no externally reachable and authenticated Feishu HTTP or event callback path appears to invoke it, potentially making a pending approval impossible to resolve through the production gateway. |
| `DV-FEI-002` | Repeated delivery of the same Feishu `message_id` is not visibly deduplicated and may create duplicate tasks, runs, side effects, and replies. Verification MUST cover sequential and concurrent duplicates in one process, duplicates after completion, duplicates after process restart, and the response or acknowledgement returned for a duplicate. |
| `DV-FEI-003` | A message event without a sender identifier is accepted and keyed with an empty sender value, potentially allowing unrelated senders in one chat to share a persisted session. |
| `DV-FEI-004` | Waiting for the same-session execution lock is not context-aware, potentially allowing wall-clock task execution to exceed the configured five-minute timeout and allowing already-expired work to start later. |
| `DV-FEI-005` | Same-session tasks are mutually exclusive but do not visibly have a FIFO guarantee, and tasks waiting for a session lock consume global concurrency permits, potentially reordering one conversation or blocking unrelated sessions. |
| `DV-FEI-006` | Runner cancellation or task-channel closure returns from the consumer without visibly draining already-started tasks, while the production entry point does not visibly coordinate HTTP shutdown, task cancellation, and in-flight task completion. |
| `DV-FEI-007` | Duplicate, late, cancelled, or concurrent approval responses do not visibly have a fully non-blocking, exactly-once terminal-state policy and may block or race with pending-request cleanup. |
| `DV-FEI-008` | Feishu automatic compaction does not visibly receive the selected runtime model, potentially using a model configuration inconsistent with the active run. |
| `DV-FEI-009` | A panic during task execution is recovered and releases the global concurrency permit but does not visibly produce a terminal failure reply; verification must also determine whether session locks, permission waits, run-scoped work, and other resources always reach one terminal cleanup state. |
| `DV-FEI-010` | Delivery errors for task receipts, session notices, final messages, and failure messages are mostly ignored or only logged, potentially allowing a task and its side effects to complete without a terminal user reply or a failure visible to the controlling adapter. |

- **AgentOpsTask Verification Gate**: The following AgentOps-specific potential defects MUST each receive the same hermetic proof and separate correction workflow before the `AgentOpsTask` baseline is frozen. Because AgentOps reuses the Feishu gateway and approval store, the results of `DV-FEI-001` and `DV-FEI-007` also govern AgentOps approval callback reachability and terminal-state handling and MUST NOT be reimplemented as a second approval protocol.

| ID | Potential pre-existing defect to verify comprehensively |
|---|---|
| `DV-AOP-001` | AgentOps deduplication is process-local and marks a message before successful bridge delivery or task completion. The gateway also rejects a missing message-ID pointer but does not visibly reject an empty message ID, allowing unrelated empty-ID events to share one deduplication key. Verification MUST cover missing and empty IDs, sequential and concurrent duplicates, the exact TTL boundary and expiry, bridge delivery failure, task failure, completion, duplicate acknowledgement and terminal lifecycle, and process restart to determine whether work can be duplicated or permanently lost and when acceptance becomes durable. |
| `DV-AOP-002` | The Feishu gateway, two capacity-64 task channels, bridge goroutine, and AgentOps runner do not visibly share one coordinated shutdown protocol, and runner return does not visibly drain in-flight tasks. Verification MUST distinguish work not yet accepted, accepted but queued work, and in-flight work; prove when new intake stops; and determine whether each accepted task is drained or cancelled into one correlated terminal state. |
| `DV-AOP-003` | A task panic releases the global concurrency permit and is logged but does not visibly produce a terminal failure reply. Verification MUST prove that panic, timeout, cancellation, and ordinary failure cannot produce a missing, duplicate, late, or cross-task terminal outcome and that session, approval, tool, extraction, and other run-scoped cleanup reaches one terminal state. |
| `DV-AOP-004` | AgentOps automatic compaction does not visibly receive the selected runtime model, potentially using a context-window configuration inconsistent with the active run. |
| `DV-AOP-005` | The initial session-notice delivery error is ignored; final delivery failure triggers a second failure notification whose own delivery error is ignored; and unbounded final model text may exceed the Feishu transport limit. Verification must determine the terminal outcome when work or side effects complete but result delivery fails. |
| `DV-AOP-006` | `log_search` rejects direct traversal syntax but follows the resolved `<logDir>/<service>.log` path and may follow a symlink outside the configured log directory. Verification MUST cover the final resolved target that is actually opened, not only the unresolved input path, and MUST confirm that the existing 200-line and one-MiB-per-line limits provide the intended resource bound. |

- **Cross-Validation**: Local Codex app-server source was checked for explicit request, thread, turn, and approval correlation plus a shutdown gate that stops new work and drains in-flight handlers. Local Claude Code remote-session source was checked for request-ID-keyed pending approvals, remote cancellation, terminal pending-state removal, and disconnect cleanup. These references validate the lifecycle dimensions that Fox must verify but do not add WebSocket, reconnection, RPC, or another reference-only behavior to Fox requirements.
- **User Evidence**: The user adopted the principle that confirmed existing defects are fixed and tested independently before refactoring and that refactoring must not silently change unverified behavior. The user explicitly adopted all ten `FeishuRemote` and all six AgentOps-specific verification items as pre-baseline `CON-005` gates, with shared Feishu approval conclusions reused by AgentOps.

- **BenchmarkEval Verification Gate**: The following benchmark-specific potential defects MUST each receive the same hermetic proof and separate correction workflow before the `BenchmarkEval` baseline is frozen. Evaluation failure, runtime failure, and infrastructure failure MUST remain distinguishable while these tests determine the authoritative post-verification behavior.

| ID | Potential pre-existing defect to verify comprehensively |
|---|---|
| `DV-BEN-001` | `RunCase` has no whole-case deadline, while only each command validation receives a two-minute timeout. Verification MUST cover cancellation and timeout during runtime execution, between runtime and validation, and during every validation stage, including whether all accepted work reaches a terminal state. |
| `DV-BEN-002` | An Agent run or validation can produce a failed benchmark result while `cmd/bench` still exits with status zero. Verification MUST cover runtime failure, partial result, validation failure, cancellation, timeout, mixed repeat outcomes, summary output, JSON output, and final process status. |
| `DV-BEN-003` | `RuntimeFidelity` is populated from manually maintained strings in the composition root rather than derived from the resolved runtime specification and may omit or misstate actual shared invariants and intentional differences. Verification MUST compare every resolved profile dimension with both machine-readable and human-visible fidelity reporting. |
| `DV-BEN-004` | Fixture copy opens the final target of a file symlink, `file_contains` joins unchecked relative or absolute paths, and setup failures can leave an unreported temporary workspace. Verification MUST cover traversal, absolute paths, file and directory symlinks, final resolved targets, source-fixture mutation, partial-copy and harness failures, successful-workspace retention, and failed-workspace cleanup. |
| `DV-BEN-005` | A non-positive `-repeat` can produce a zero-run successful report; negative `max_turns` becomes an unlimited run; and empty command, path, or contains fields can create invalid or vacuous validations. Verification MUST determine the accepted input domain and failure precedence for every case and process option. |
| `DV-BEN-006` | Command validation uses unbounded `CombinedOutput`, so an uncontrolled validator can exhaust memory, and `exec.CommandContext` may terminate only the immediate shell while leaving descendants alive. Verification MUST cover bounded stdout and stderr, truncation or overflow reporting, timeout, cancellation, process-group or process-tree termination, reaping, and subsequent-validation behavior. |
| `DV-BEN-007` | Results do not visibly record a repeat index, run identity, case-definition identity, fixture identity, or complete resolved runtime provenance. Reusing a `case_id` with different inputs or configuration can therefore produce reports that cannot be unambiguously correlated or reproduced. Verification MUST cover stable identity, volatile-field normalization for tests, terminal-state correlation, and compatibility of any corrected result schema. |

- **Cross-Validation**: Local Codex source contains no equivalent Fox benchmark runner, but its rollout trace records stable trace, rollout, and root-thread identities, distinguishes running, completed, failed, and aborted status, tests process-group cleanup, and checks canonical and symlink path boundaries. Local Claude Code source likewise contains no equivalent benchmark runner, but its VCR fixtures hash normalized inputs, fail closed in CI when committed fixtures are absent, normalize volatile paths, identifiers, timestamps, and durations, cap command output, terminate process trees, and map command failure to explicit status. These references strengthen Fox evaluation reproducibility and resource-lifecycle verification without importing Codex rollout bundles, Claude VCR recording, sandbox services, or reference-only protocols.
- **User Evidence**: The user accepted the complete BenchmarkEval scenario set, requested Codex and Claude Code source cross-validation, and explicitly confirmed the resulting terminal-state, process-tree, output-bound, fixture-authority, provenance, and reproducibility refinements plus `DV-BEN-001` through `DV-BEN-007`.

- **ChildRun Verification Gate**: The following child-run-specific potential defects MUST each receive the same hermetic proof and separate correction workflow before the `ChildRun` baseline is frozen. Tests MUST distinguish a model-facing child-invocation adapter defect from a runtime child-lifecycle defect and MUST preserve the confirmed one-level child ceiling.

| ID | Potential pre-existing defect to verify comprehensively |
|---|---|
| `DV-CHD-001` | A read-only child still receives arbitrary Bash and may mutate the workspace, repository, process environment, or external state. Verification MUST cover model-visible and runtime-executable surfaces, permission assessment, direct and indirect filesystem mutation, shell redirection and subprocesses, and the exact post-verification meaning of read-only. |
| `DV-CHD-002` | The child compactor is not visibly configured with the selected child model, potentially falling back to a default context-window assumption such as 128K that differs from the active model. Verification MUST compare invocation, trigger, token accounting, telemetry, and persisted compact state against the frozen child model snapshot. |
| `DV-CHD-003` | The child prompt can describe write, edit, or TODO capabilities that are absent from the actual child tool snapshot. Verification MUST prove that prompt fragments, model-visible definitions, runtime-executable calls, aliases, permission assessment, and parallel-safety lookup agree for read-only and writable children. |
| `DV-CHD-004` | Cancelling a Bash tool call may terminate only its immediate process while leaving descendants alive. Verification MUST cover parent cancellation, child timeout or turn exhaustion, process-group or process-tree termination, reaping, permission cleanup, and absence of later output or side effects. |
| `DV-CHD-005` | The fork-skill path accepts an `agent` selection but the current child execution may ignore it, conflicting with the existing slash-command contract. Verification MUST trace the processed task, selected agent, project instructions, provider and model snapshot, tool ceiling, and final child report before any compatibility expectation is frozen. |
| `DV-CHD-006` | The engine can return a partial result together with an error, while child wrappers may discard the result, including child session identity and report content. Verification MUST cover provider, tool, persistence, compaction, turn-limit, and cancellation failures and define which correlated partial outcome reaches the parent. |

- **Cross-Validation**: Local Codex source was checked for immutable parent configuration snapshots, explicit parent-thread and depth lineage, child-capacity accounting, terminal status, cancellation, and cleanup. Local Claude Code source was checked for child tool filtering, permission isolation, synchronous cancellation inheritance, separate transcripts, child compaction, partial results, and `finally`-style cleanup. These references strengthen Fox child capability, lineage, and resource-lifecycle verification without adding background children, resume or send-input protocols, worktree isolation, agent trees, MCP transport, or team orchestration.
- **User Evidence**: The user accepted the complete cross-validated `ChildRun` scenario set and explicitly confirmed `DV-CHD-001` through `DV-CHD-006` as pre-baseline `CON-005` gates.

- **AutodevPipeline Verification Gate**: The following Autodev-specific potential defects MUST each receive the same hermetic proof and separate correction workflow before the `AutodevPipeline` baseline is frozen. Tests MUST separate runtime-profile behavior, durable control-plane behavior, and CLI or TUI presentation behavior and MUST use local repositories, scripted providers, fake GitHub boundaries, deterministic clocks and identifiers, and bounded local process fixtures.

| ID | Potential pre-existing defect to verify comprehensively |
|---|---|
| `DV-AUT-001` | Ledger-save failures after initial seeding are logged but do not visibly stop subsequent stage, commit, push, issue, PR, done, or cleanup operations. Verification MUST inject failure before and after every irreversible transition and determine whether memory and disk state, remote side effects, reporting, retry, and restart can diverge or duplicate work. |
| `DV-AUT-002` | An unknown non-empty recorded stage currently maps past the entire SDD pipeline and may proceed directly to publishing. Verification MUST cover empty, every known SDD and publish stage, renamed, malformed, future-version, and unknown values and prove that no invalid state can bypass required verification. |
| `DV-AUT-003` | Backlog reconciliation matches by mutable title, retains ledger entries absent from the current backlog, and does not persist descriptions. Removing, renaming, reordering, duplicating, or editing an item may therefore execute stale work, create a second item, reuse the wrong identity, or resume with missing requirement text. Verification MUST define stable identity and authoritative reconciliation without letting advisory backlog status overwrite durable progress. |
| `DV-AUT-004` | Requirements materialization collapses the backlog description to one line and truncates it to 4,000 characters, potentially discarding confirmed input before specification generation. Verification MUST cover multiline Markdown, code blocks, Unicode, maximum scanner input, very long descriptions, empty descriptions, and exact traceability back to the source item. |
| `DV-AUT-005` | A persisted `FeatureDir` is joined with the worktree without a visible lexical and resolved-path containment check. Verification MUST cover absolute paths, traversal, file and directory symlinks, malformed ledger data, resumed stages, materialization, verification, and every artifact read or write. |
| `DV-AUT-006` | Completion gates and Git or GitHub queries use unbounded `CombinedOutput`; `exec.CommandContext` may terminate only the immediate process; and stages have no deadline beyond caller cancellation. Verification MUST cover bounded stdout and stderr, overflow reporting, hung commands, cancellation, process-group or process-tree termination and reaping, later-stage suppression, ledger state, and CLI signal handling. |
| `DV-AUT-007` | Issue verification searches at most twenty results and binds the first exact title, without a stable Autodev item marker. A pre-existing issue, duplicate backlog title, search truncation, or concurrent creation may bind the wrong issue or create duplicates. Verification MUST cover durable item-to-issue correlation, restart, closed issues, duplicate titles, stale results, and exactly-once reporting. |
| `DV-AUT-008` | Per-run automemory extraction remains asynchronous while the orchestrator can advance to another item or remove the completed item's worktree. Verification MUST determine ordering, cancellation, worktree lifetime, shared-memory isolation, provider failure, panic, process exit, and whether delayed extraction can affect a later item or lose intended memory. |
| `DV-AUT-009` | The `concurrency` configuration accepts arbitrary values while execution remains serial, potentially silently accepting a misspelling or an unsupported parallel mode. Verification MUST determine the accepted domain, warning or failure behavior, precedence, and prove that no value accidentally enables parallel item execution. |
| `DV-AUT-010` | When a core run returns a partial result together with an error, `StageMachine` returns the error before verification or Engineer review and may discard run, session, report, artifact, or side-effect correlation. Verification MUST cover provider, tool, persistence, compaction, turn-limit, and cancellation failures and define the durable control-plane outcome and retry behavior. |

- **Cross-Validation**: Local Codex cloud-task source was checked for stable task identity, explicit pending, ready, applied, and error states, per-attempt terminal status, apply preflight, concurrent-operation exclusion, stale-result rejection, and non-zero status behavior. Local Claude Code source was checked for explicit running, completed, failed, and killed states, parent-linked cancellation, registered cleanup, exactly-once terminal notification, session-storage flush before result delivery, and unconditional state finalization. These references strengthen Fox Autodev identity, durability, terminal-state, and cleanup verification without adding cloud execution, best-of-N attempts, background agents, resumable messaging, or multi-worker orchestration.
- **User Evidence**: The user confirmed the complete cross-validated Autodev runtime-profile, control-plane, entry-adapter, and defect-verification scenario set, including `DV-AUT-001` through `DV-AUT-010`.

### CON-006: Preserve unverified profile differences and persisted source metadata

- **Status**: confirmed
- **Statement**: Profile extraction MUST preserve current entry-specific differences unless a difference is confirmed and handled as a pre-existing defect under CON-005. In particular, existing persisted session source values and lookup behavior, including AgentOps sessions using the Feishu source and benchmark and Autodev sessions using the CLI source, MUST remain compatible and MUST NOT be normalized as part of the refactor alone.
- **User Evidence**: The user confirmed the profile matrix with these values identified as compatibility facts rather than automatically classified defects.

### CON-007: Mandatory verification is hermetic and environment-independent

- **Status**: confirmed
- **Statement**: Every test used as a commit, phase, PR, or compatibility gate MUST be self-contained, deterministic, isolated, repeatable, and runnable offline. Mandatory tests MUST NOT access a real LLM, Feishu, AgentOps, GitHub, or another external service; require external credentials; depend on user home contents, ambient configuration, existing sessions, fixed ports, background processes, wall-clock timing, uncontrolled randomness, test execution order, or state left by another test. Tests MAY use real filesystem operations in test-owned temporary directories, repository-declared local toolchain programs such as `git` against test-owned local repositories, scripted fakes, deterministic clocks and identifiers, and local test servers created and destroyed by the test. Missing external service configuration MUST NOT cause a mandatory test to skip or fail.
- **Rationale**: Behavioral equivalence is only enforceable when every developer and CI environment can reproduce the same results without external availability or mutable personal state.
- **User Evidence**: The user explicitly required tests to avoid real external dependencies and to use fixed persisted fixtures that can be rerun in any environment.

### CON-008: New implementation follows strict Red-Green-Refactor TDD

- **Status**: confirmed
- **Statement**: Every new production implementation and every behavior correction in this refactor MUST follow a verifiable Red-Green-Refactor cycle. In Red, a test for the target behavior MUST run before its production implementation and fail because the capability is missing or the current behavior is wrong, rather than because of invalid test code, unavailable fixtures, external state, or another unrelated setup error; an initial missing-API compile failure MAY establish that an API is absent but MUST be followed by a behavior-sensitive failing assertion. In Green, only the minimum production code required to satisfy the failing behavior MUST be added while all applicable characterization, package, and architecture tests remain green. In Refactor, naming, structure, duplication, and dependencies MAY be improved only while all tests remain green, behavior remains unchanged, and golden or compatibility fixtures are not rewritten to accommodate implementation drift. Shared contract scenarios MUST pass against the current implementation before migration and enter a genuine Red state against each new target implementation adapter before that implementation is completed. Pure mechanical moves or renames protected by existing characterization tests MUST NOT manufacture artificial failures, but any new package, port, state coordinator, behavior, or defect correction requires the complete cycle. Red evidence, including the command and expected failure reason, MUST be retained in the task or implementation record. Failing intermediate tests need not be committed to the integration branch; each final module commit MUST remain complete, green, reviewable, and independently revertible.
- **Rationale**: The project constitution already mandates TDD, but this cross-package refactor requires explicit evidence that new boundaries and implementations are driven by behavior contracts rather than tested only after construction.
- **User Evidence**: The user explicitly added a requirement that new code implementation strictly follow the TDD failure-pass-refactor process and confirmed the detailed operational definition.

## Decisions

### DEC-001: Logical package separation before physical project separation

- **Status**: confirmed
- **Decision**: Separate the core runtime, application services, runtime assembly, entry-point adapters, and TUI through Go package boundaries within the existing repository and module.
- **Alternatives Rejected**: Multiple Go modules and separate Git repositories for this refactor.
- **Reason**: Strong package boundaries provide independent evolution without introducing cross-module versioning and release coordination.
- **User Evidence**: "Use a single repository, a single Go module, and strong package boundaries."

### DEC-002: One integration branch and one final PR

- **Status**: confirmed
- **Decision**: Implement the refactor directly on the current feature/integration branch using clear module-focused commits, without creating additional topic branches or worktrees, and deliver it as one final PR to `main`.
- **Alternatives Rejected**: Mandatory per-module branches, additional worktrees for this implementation, and multiple incremental PRs to `main`.
- **Reason**: Development is sequential in this effort, while the final transition to the new architecture should be atomic for users.
- **User Evidence**: "Develop directly on the current integration branch, with clear independent commits by module. No additional branches or worktrees are needed."

### DEC-003: Internal Go APIs may evolve

- **Status**: confirmed
- **Decision**: The refactor MAY change `internal/...` Go types, interfaces, constructors, methods, and package locations when required to establish the target boundaries. Every in-repository caller and test MUST migrate atomically in the same final PR, and behavioral characterization MUST prove that user-visible behavior, module behavior, and persisted data remain compatible.
- **Alternatives Rejected**: Preserving source compatibility for every existing internal Go API through permanent transitional facades.
- **Reason**: Some current internal APIs expose implementation details across package boundaries; preserving them would retain the coupling that the refactor is intended to remove.
- **User Evidence**: The user explicitly allowed internal Go API changes under the stated behavioral and data compatibility constraints.

### DEC-004: User entry points depend on application capabilities

- **Status**: confirmed
- **Decision**: CLI, TUI, Feishu, and AgentOps MUST access the core agent runtime through small application capability interfaces and UI-neutral DTOs. These entry-point adapters MUST NOT directly operate on concrete engine, session, checkpoint, or compaction implementations. `cmd/*` packages MAY know both adapters and concrete implementations only in their role as composition roots.
- **Alternatives Rejected**: Allowing each user entry point to assemble or directly control core runtime subsystems independently, and replacing explicit capabilities with a generic event bus.
- **Reason**: A stable application boundary prevents UI and transport concerns from leaking into the runtime while keeping dependencies explicit and testable.
- **User Evidence**: The user explicitly confirmed the proposed application capability boundary for all user entry points.

### DEC-005: Benchmark belongs to the runtime harness

- **Status**: confirmed
- **Decision**: Benchmarking is an evaluation and feedback capability of the core agent runtime harness, not a user-interaction adapter. It MAY directly control core runtime modules through privileged harness APIs and inject benchmark-specific providers, sessions, reporters, tool sets, and budgets. It MUST reuse the real runtime contracts and shared security and capability invariants rather than duplicate runtime assembly. Every intentional difference from product execution MUST be represented explicitly and reported as runtime-fidelity metadata.
- **Alternatives Rejected**: Forcing benchmark execution through the user-facing application capability layer, and maintaining an independently assembled benchmark engine that can drift from the product runtime.
- **Reason**: Benchmarking exists to evaluate and provide feedback on core agent capabilities and therefore requires controlled access to runtime components without user-interface concerns.
- **User Evidence**: "Benchmark serves and tests core agent capabilities, is not part of the user interaction layer, and should be part of the runtime harness for evaluation and feedback with direct control of core modules."

### DEC-006: Subagent is nested runtime execution

- **Status**: confirmed
- **Decision**: Subagent delegation is a core runtime execution capability, not a user-interaction capability. The model-facing `delegate_task` tool is the invocation adapter for this capability; its implementation MUST delegate to a shared child-run factory and explicit child profile rather than independently assembling an engine. Nested runs MUST reuse runtime contracts, propagate cancellation, inherit parent permission evidence and security ceilings, and explicitly constrain tools, read/write mode, turn budget, and context budget.
- **Alternatives Rejected**: Routing subagent execution through the user-facing application layer, treating the tool adapter itself as the complete subagent runtime, and maintaining an independently assembled nested engine.
- **Reason**: Tool invocation is the model protocol surface, while isolated session creation and nested agent execution are runtime orchestration responsibilities that must share the same invariants as top-level runs.
- **User Evidence**: "Subagent is not a user-layer capability; it is one of the runtime execution capabilities."

### DEC-007: Subagent delegation remains single-level

- **Status**: confirmed
- **Decision**: The refactor MUST preserve the current single-level delegation topology: a parent agent may invoke `delegate_task` to create one child run, but the child run MUST NOT expose `delegate_task` or create further descendant runs.
- **Alternatives Rejected**: Adding recursive child-to-descendant delegation as part of this refactor.
- **Reason**: Multi-level delegation would be a functional and resource-management change, while this work is strictly behavior-preserving.
- **User Evidence**: The user explicitly required delegation to remain one level deep.

### DEC-008: AgentEngine is a turn state machine

- **Status**: superseded by DEC-009
- **Decision**: The top-level `AgentEngine` MUST retain only the readable run/turn state machine and coordination of injected collaborators. It MUST NOT directly implement model invocation mechanics, tool scheduling and result processing, context and compaction policy, turn policy, or journal internals. Each extracted capability MUST be independently injectable and testable.
- **Alternatives Rejected**: Keeping the current monolithic `RunWithReporter` orchestration and performing a file-only split without responsibility boundaries.
- **Reason**: These capabilities have distinct state, failure semantics, tests, and reasons to change; concentrating them in the engine prevents independent evolution.
- **User Evidence**: The user agreed to reduce the top-level engine to a pure turn state machine, subject to validating that the decomposition is non-redundant.

### DEC-009: AgentEngine is an infrastructure-independent turn coordinator

- **Status**: confirmed
- **Decision**: `AgentEngine` MUST own the readable run/turn transition flow, turn and phase state, terminal states, and hard turn-limit enforcement while remaining independent of UI packages, persisted-session implementations, and concrete infrastructure. It MAY invoke injected model, tool, policy, and context collaborators, so it is not required to be a mathematically pure state machine. It MUST NOT implement provider streaming and fallback mechanics, tool authorization/scheduling/result processing, compaction mechanics, persisted-state commits, or telemetry sink internals.
- **Alternatives Rejected**: A monolithic engine, a file-only split, and an effect-interpreter architecture introduced only to make the engine literally pure.
- **Reason**: A thin but meaningful coordinator preserves readable control flow without hiding I/O behind redundant abstractions.
- **User Evidence**: The user adopted the revised architecture after independent subagent review identified ambiguity in the phrase "pure turn state machine."

### DEC-010: AgentSession is the sole recoverable-state coordinator

- **Status**: confirmed
- **Decision**: The runtime `AgentSession` MUST be the sole coordinator and committer of all state required for session restore, continuation, rewind, and compaction compatibility. Storage repositories MUST only perform storage operations. `RunContext` MUST be a per-run in-memory projection of model-visible context and MUST NOT be an independent persisted-state authority. Compaction MUST return context and state-change proposals for `AgentSession` to commit in the behavior-compatible order.
- **Alternatives Rejected**: Allowing `RunContext`, compaction, the engine, and session repositories to independently mutate overlapping authoritative state.
- **Reason**: Message history, compact state, checkpoints, and PLAN/TODO state require one owner to prevent double writes and ambiguous recovery.
- **User Evidence**: The user adopted the revised ownership model produced by the architecture review.

### DEC-011: Application protocols separate notifications from interaction ports

- **Status**: confirmed
- **Decision**: User-facing application capabilities MUST expose UI-neutral commands and DTOs. One-way typed notifications MAY represent run progress, but synchronous bidirectional interactions such as permission approval, user questions, and plan review MUST use explicit request/response ports with correlation, cancellation, and timeout semantics. Runtime-owned event and result types MUST be mapped into application-owned DTOs without the runtime importing the application package.
- **Alternatives Rejected**: Modeling every interaction as a one-way event channel, exposing engine/session/checkpoint/compaction types to entry adapters, and introducing a generic event bus.
- **Reason**: Interactive runtime pauses have different control-flow semantics from progress notifications, and the dependency direction must remain acyclic in Go.
- **User Evidence**: The user adopted the revised application boundary after subagent review.

### DEC-012: Runtime observation, session artifacts, and telemetry remain distinct

- **Status**: confirmed
- **Decision**: A synchronous ordered `RunObserver` MUST carry user-observable run lifecycle and streaming facts. Session artifacts such as transcripts MUST remain distinct because they may be model-visible even when they are not authoritative recovery state. Metrics and tracing MUST use best-effort telemetry journals whose failures are surfaced according to existing warning semantics. A fact MUST be produced once by the runtime and adapted to these consumers without parallel Reporter/Event/Journal pipelines that independently define ordering.
- **Alternatives Rejected**: Treating reporters, transcripts, metrics, and traces as one uniform best-effort `RunJournal` and maintaining duplicate event pipelines.
- **Reason**: These outputs have different consumers, behavioral significance, and failure policies.
- **User Evidence**: The user adopted the revised observer and journal split after subagent review.

### DEC-013: RuntimeHarness, AgentSession, and RunScope have non-overlapping lifetimes

- **Status**: confirmed
- **Decision**: `RuntimeHarness` MUST contain only immutable configuration, concurrency-safe shared dependencies, and factories used to create or open sessions. It MUST NOT retain a current session, permission grants, engine state, compactor state, or another run-scoped mutable object. `AgentSession` MUST own session operations such as run, compact, rewind, and close. A per-run `RunScope` or `RunSpec` MUST own the observer, cancellation, permission scope, budget, model snapshot, and run-scoped collaborators. Profiles MUST describe immutable policy presets and capability ceilings rather than mutable dependencies or session state.
- **Alternatives Rejected**: Splitting the current runner into forwarding wrappers, sharing mutable engine/compactor/permission state across sessions, and creating a profile god object.
- **Reason**: Explicit lifetimes prevent cross-session state leakage while keeping harness assembly reusable.
- **User Evidence**: The user adopted the revised runtime lifecycle model after subagent review.

### DEC-014: Runtime control clients share the harness without using the application layer

- **Status**: confirmed
- **Decision**: Benchmark, subagent delegation, and Autodev MUST be runtime control clients. They MUST use the shared runtime harness and explicit profiles or run specifications without passing through the user-facing application layer or independently assembling engines. Benchmark fidelity MUST derive from the resolved runtime specification and explicitly declared differences. Child and benchmark session scope and workspace scope MUST be modeled independently.
- **Alternatives Rejected**: Treating these capabilities as presentation adapters, duplicating runtime assembly, and coupling runtime control clients to TUI or remote application DTOs.
- **Reason**: These capabilities control or evaluate agent execution rather than mediate a user-interface protocol.
- **User Evidence**: The user adopted the revised structure, including the previously omitted Autodev classification.

### DEC-015: Engine collaborators have exclusive operational responsibilities

- **Status**: confirmed
- **Decision**: `ModelInvoker` MUST own provider invocation, streaming and fallback mechanics, and normalized model-call facts. `ToolExecutor` MUST execute a turn-specific constrained tool snapshot and return an ordered structured batch without writing the session message log. `TurnPolicy` MUST decide completion, TODO, reminder, and recovery policy without selecting tools or persisting state. `RunContext` MUST hold only the current model-visible projection. A context or compaction coordinator MUST preserve the distinct initial-history, pre-turn, and reactive-compaction paths and their existing timing and failure semantics. These capabilities MAY be interfaces in their consumer package and MUST NOT each require a dedicated Go package.
- **Alternatives Rejected**: Overlapping ownership, one package per interface, policy-controlled tool assembly, and compaction hidden inside mutable run context.
- **Reason**: Exclusive responsibilities preserve behavior while avoiding both monoliths and package proliferation.
- **User Evidence**: The user adopted the revised collaborator boundaries after subagent review.

### DEC-016: Single-level delegation uses runtime and capability-surface enforcement

- **Status**: confirmed
- **Decision**: Every child-run creation path MUST pass through one runtime `ChildRunner` capability. The runtime MUST enforce a non-relaxable maximum delegation depth of one for this architecture: a root run may create a depth-one child, while a depth-one child MUST be rejected from creating another child regardless of which tool, skill, or internal caller requests it. The child profile MUST additionally exclude all delegation capabilities from its model-visible and callable tool surface. Entry points, profiles, and individual delegation requests MUST NOT raise this limit.
- **Alternatives Rejected**: Relying only on omission of `delegate_task` from the current child registry, adding a general configurable agent-tree subsystem, and allowing individual profiles to relax the depth ceiling.
- **Reason**: Central runtime enforcement preserves the confirmed single-level behavior across present and future child-creation paths, while capability-surface filtering prevents invalid model calls without introducing a complex multi-level runtime.
- **User Evidence**: After comparing Codex stable runtime depth enforcement and Claude Code child tool filtering and recursive-fork guards, the user adopted the minimal dual constraint.

### DEC-017: TUI remains a Fox-specific presentation adapter

- **Status**: confirmed
- **Decision**: The TUI MUST be separated from the core runtime as a Fox-specific presentation adapter over application capabilities, UI-neutral DTOs, runtime notifications, and explicit interaction ports. This refactor MUST NOT extract or publish a generic terminal UI library comparable to `pi-tui`. The existing TUI package MAY reorganize its internal components when required for the application boundary, but it MUST NOT introduce generic extension contracts or abstractions without a second concrete consumer.
- **Alternatives Rejected**: Extracting a reusable general-purpose terminal UI framework during this refactor and retaining direct TUI dependencies on engine, session, checkpoint, compaction, or concrete tool implementations.
- **Reason**: Runtime/UI decoupling is required, while a generic TUI library has no current second consumer and would add scope and abstractions unrelated to the behavior-preserving maintainability objective.
- **User Evidence**: The user adopted the recommendation to keep the TUI as the Fox application presentation adapter and not extract a `pi-tui`-style library.

### DEC-018: Seven explicit resolved runtime profiles

- **Status**: confirmed
- **Decision**: Runtime assembly MUST expose seven named behavior profiles: `TUIInteractive`, `CLIExec`, `FeishuRemote`, `AgentOpsTask`, `BenchmarkEval`, `ChildRun`, and `AutodevPipeline`. Implementations MAY compose shared typed policy fragments such as product-root defaults, but every selected profile MUST resolve to a flat, immutable, independently snapshot-testable specification. Runtime profile inheritance, mutable profile state, and one monolithic profile object with overlapping ownership MUST NOT be introduced.
- **Alternatives Rejected**: One profile per incidental run option, treating every entry point as identical hidden assembly, runtime inheritance hierarchies, and a mutable profile god object.
- **Reason**: The seven profiles correspond to materially different lifecycle, security, capability, and observation contracts while still permitting implementation reuse without hiding differences.
- **User Evidence**: The user explicitly adopted the proposed seven-profile catalog.

### DEC-019: Profiles define presets and ceilings while RunSpec carries per-run input

- **Status**: confirmed
- **Decision**: A runtime profile MUST define immutable defaults, permitted variation, and non-relaxable capability and security ceilings. Per-run values such as prompt and display prompt, session selection, collaboration mode, model and effort snapshots, thinking and budget overrides, read-only mode, allowed tools, benchmark case, parent session, delegation depth, task text, and observer instances MUST be carried by a per-run `RunSpec` or equivalent typed inputs. A `RunSpec` MAY select or narrow behavior permitted by its profile but MUST NOT expand the profile's capability or security ceilings.
- **Alternatives Rejected**: Creating a new profile for every model, mode, or tool allow-list; storing dynamic run state in profiles; and allowing run-time overrides to expand safety ceilings.
- **Reason**: This boundary prevents profile proliferation and state leakage while making dynamic execution inputs explicit.
- **User Evidence**: The user confirmed the proposed Profile/RunSpec boundary.

### DEC-020: Prompt fragments are pure renderers owned separately from context lifecycle

- **Status**: confirmed
- **Decision**: The current `internal/context` package MUST become `internal/prompt`. The target package MUST contain deterministic, side-effect-free prompt-fragment representation, rendering, and ordering only. It MUST NOT discover project instruction files or skills, read or write session or automatic memory, select collaboration mode, own conversation history, execute compaction, call a model provider, or decide when a fragment is injected. Runtime `ContextController` MUST collect and select resolved inputs and prepare the per-run `RunContext`; runtime `AgentSession` MUST coordinate any recoverable context-state commit and its behavior-compatible ordering.
- **Alternatives Rejected**: Retaining the misleading `context` package name despite its collision with Go's standard `context` package, making `prompt` the complete context owner, and distributing recoverable context ownership across prompt rendering, compaction, and session storage.
- **Reason**: Codex separates typed context fragments from session-owned context lifecycle, while Claude Code also keeps prompt generation separate from its runtime query pipeline even though its state ownership is more distributed. Fox requires the clearer single-owner runtime model already established by DEC-010 and DEC-015.
- **User Evidence**: The user explicitly confirmed `internal/context` to `internal/prompt` and required prompt to remain a pure fragment renderer while runtime owns context lifecycle and injection decisions.

### DEC-021: Runtime sessions and persisted session records use distinct names and owners

- **Status**: confirmed
- **Decision**: The refactor MUST retain the `internal/session` package path and distinguish persistence records from the live runtime coordinator through types and responsibilities rather than renaming the package to `sessionstore`. The live coordinator MUST be `runtime.AgentSession`; immutable per-run input and run-scoped state MUST use `runtime.RunSpec` and `runtime.RunScope`; and the persistence capability consumed by runtime MUST be represented by a consumer-owned `runtime.SessionStore` interface. The file-backed implementation MUST be `session.FileStore`, replacing the lifecycle-ambiguous `session.Manager`. Persisted metadata records MUST be named `session.StoredSession` and `session.StoredRun`, replacing `session.Session` and `session.Run`. Durable identifiers MUST use `session.ID` and `session.RunID` while preserving their existing string encodings. `session.MessageRecord`, `session.MessageLog`, and `session.CompactState` MUST retain their names and persistence responsibilities. The persisted transcript artifact types MUST be named `session.TranscriptEvent` and `session.TranscriptLog`, replacing the overly broad `session.Event` and `session.Transcript`. The duplicate `session.WorkingMemory` implementation MUST be removed in favor of the single `memory.Store` owner. Persisted record values MUST NOT directly perform runtime lifecycle or storage operations such as starting runs; `AgentSession` decides lifecycle and recoverable-state commit policy, while `SessionStore` implementations perform storage mechanics only.
- **Alternatives Rejected**: Renaming `internal/session` to `internal/sessionstore`, retaining ambiguous `Session`, `Run`, and `Manager` names for persistence types, allowing persisted records to own file I/O or runtime lifecycle decisions, and maintaining duplicate working-memory implementations.
- **Reason**: The naming follows the semantic distinction demonstrated by Codex between a live runtime `Session`, a storage boundary, and `Stored*` values, while preserving Fox terminology and avoiding package-path churn. Consumer-owned storage contracts permit replacement without making persistence the runtime state owner.
- **User Evidence**: The user explicitly required retaining `internal/session`, distinguishing persisted session records from `runtime.AgentSession` through types and responsibilities, and confirmed the proposed concrete naming set before the planning stage.

### DEC-022: Infrastructure remains a classification rather than an aggregate package

- **Status**: confirmed
- **Decision**: The target architecture MUST NOT create a unified `internal/infrastructure` package. Infrastructure is an architectural classification only. Concrete implementations MUST remain in responsibility-focused packages such as provider, tools, session, memory, and telemetry, with each package owning one cohesive capability and an independent reason to change. Package moves or renames MUST be justified by a clearer ownership boundary, not by grouping implementations under a generic technical-layer directory.
- **Alternatives Rejected**: Moving unrelated provider, tool, persistence, memory, and telemetry implementations into one `internal/infrastructure` package, and using a broad technical-layer package as a default destination for code that does not fit elsewhere.
- **Reason**: A broad infrastructure package would recreate the oversized and overlapping ownership that this refactor is intended to remove, weaken cohesion, increase coupling, and increase cross-module development conflicts.
- **User Evidence**: The user explicitly rejected a unified `internal/infrastructure` package to avoid repeating oversized package responsibilities that violate high cohesion and low coupling.

### DEC-023: The application layer retains the internal/app package path

- **Status**: confirmed
- **Decision**: The application layer MUST retain the concise Go package path `internal/app`; it MUST NOT be renamed to `internal/application` in this refactor. Its responsibilities MUST nevertheless be narrowed to user-entry use cases, UI-neutral commands and DTOs, ordered runtime-notification adaptation, and explicit interaction ports as established by DEC-004 and DEC-011. Runtime assembly and lifecycle ownership, concrete presentation behavior, and command composition MUST move to their corresponding runtime, adapter, or `cmd/*` owners rather than remain in `app` merely because of the package name.
- **Alternatives Rejected**: Renaming `internal/app` to `internal/application`, and interpreting the retained `app` name as permission to continue combining application use cases, runtime composition, and UI implementation.
- **Reason**: `app` is an accepted concise Go package name for the application layer. Responsibility and dependency boundaries, rather than a longer directory name, prevent the current ownership ambiguity.
- **User Evidence**: The user corrected the naming discussion and reaffirmed the previously selected decision to retain `internal/app` while improving its responsibility boundaries.

### DEC-024: TUI and non-interactive CLI runs belong to dedicated presentation adapters

- **Status**: confirmed
- **Decision**: The existing `app.RunTUI` entry flow MUST move to the existing `internal/tui` package and become its single public `tui.Run` adapter entry. Any current lower-level `tui.Run` implementation MUST become an unexported implementation detail or be merged so that two public TUI runners do not remain. The existing `app.RunCLI` entry flow MUST move to a dedicated `internal/cli` package and become `cli.Run`, representing the non-interactive `fox exec` and `fox -p` presentation adapter. `cmd/fox` MUST remain the composition root and launch-mode dispatcher: it may parse process inputs, resolve configuration, construct concrete stores, providers, runtime harnesses, and application capabilities, then invoke `tui.Run` or `cli.Run`; it MUST NOT own either presentation workflow. Both adapters MUST depend on application capabilities and UI-neutral types rather than assemble or directly operate runtime, engine, provider, tool, compaction, checkpoint, memory, or session-store implementations. `cli.Run` MUST own the compatibility-sensitive formatting and ordering of final output and artifact locations. As clarified by DEC-035, CLI completion MUST be staged: the completed run outcome becomes available to the adapter for output before tracked automemory extraction is drained, while the application invocation does not finish and the process does not exit until that drain completes.
- **Alternatives Rejected**: Keeping `RunTUI` or `RunCLI` in `internal/app`, implementing either presentation workflow directly in `cmd/fox`, allowing the adapters to assemble runtime dependencies, and retaining duplicate public TUI run entry points.
- **Reason**: TUI interaction mechanics and non-interactive terminal output have distinct presentation-level reasons to change and test surfaces. The composition root should close the dependency graph and dispatch modes without becoming another application or presentation package. This also follows the useful Codex separation between its CLI dispatcher, TUI runner, and non-interactive exec runner.
- **User Evidence**: The user requested one exact location for each runner, reviewed the responsibility-based rationale and Codex comparison, and explicitly adopted `app.RunTUI` to `tui.Run`, `app.RunCLI` to `cli.Run`, with `cmd/fox` limited to composition and dispatch.

### DEC-025: Runtime lifecycle types form one cohesive package without premature subpackages

- **Status**: confirmed
- **Decision**: A new `internal/runtime` package MUST own the cohesive runtime-lifecycle types and coordination represented by `RuntimeHarness`, `AgentSession`, `RunSpec`, `RunScope`, `Profile`, `ContextController`, `ChildRunner`, and the consumer-owned `SessionStore` persistence port. These types MAY be organized in focused files but MUST NOT be preemptively split into `runtime/session`, `runtime/profile`, `runtime/child`, or similar subpackages during this refactor. `internal/runtime` MUST remain limited to runtime construction, immutable profile resolution and enforcement, session and run lifecycle coordination, context-injection decisions, recoverable-state commit coordination, and nested-run control. The turn state machine, session persistence, prompt rendering, compaction mechanics, benchmark evaluation, the model-facing subagent adapter and child-result protocol, and Autodev orchestration MUST remain in the independent `engine`, `session`, `prompt`, `compaction`, `benchmark`, `subagent`, and `autodev` packages respectively.
- **Alternatives Rejected**: Moving the excluded mechanisms and clients into one broad runtime package, and creating a package for every runtime type before an independent ownership or dependency boundary exists.
- **Reason**: The selected runtime types share one lifecycle-coordination reason to change, while the excluded capabilities have independent algorithms, state, protocols, or client responsibilities. Focused files provide readability without introducing package proliferation or cycles.
- **User Evidence**: The user explicitly confirmed the proposed `internal/runtime` type set, independent package exclusions, and no-premature-subpackage rule.

### DEC-026: Engine ports preserve request consistency without owning runtime state

- **Status**: confirmed
- **Decision**: `internal/engine` MUST contain the infrastructure-independent turn state machine and its consumer-owned ports represented by `AgentEngine`, `RunInput`, `RunOutcome`, `RunContext`, `ModelInvoker`, `ToolExecutor`, `TurnPolicy`, `Conversation`, and `Observer`. `AgentEngine` MUST retain only immutable collaborators and MUST NOT retain mutable state across runs. Turn counters, completion-gate attempts, reminder and recovery tracking, streaming fallback state, and similar mutable values MUST have an explicit run, turn, model-invoker, or runtime-owner lifetime. `RunContext` MUST be an immutable model-visible snapshot for one model invocation rather than a mutable context owner. `Conversation` MUST be the engine-consumer interface implemented by runtime `ContextController` for preparing model-visible snapshots and requesting ordered conversation changes; runtime `AgentSession` remains the sole coordinator of recoverable-state commits. `ToolExecutor` MUST represent or operate on one immutable turn- or step-scoped capability snapshot whose model-visible definitions and executable tools are derived together, preserving ordering and parallel-safety semantics. `Observer` MUST synchronously emit typed engine facts in canonical order; runtime MUST adapt those facts into the single `RunObserver` pipeline rather than maintain a parallel reporter or event pipeline. `TurnPolicy` MUST remain a focused decision capability and MUST NOT become a container for context mutation, tool selection, persistence, or telemetry. Engine `RunOutcome` MUST contain only execution outcome information such as final message, turn count, finish reason, normalized usage, and failure facts; runtime `RunResult` MUST add session, run, artifact, and telemetry information. Turn-loop helper state and outcome types MUST remain unexported unless a confirmed cross-package contract requires them.
- **Allowed Dependencies**: `engine` MAY depend on the Go standard library and the narrow `schema` protocol-value package for messages, usage, tool definitions, calls, and results. `schema` MUST NOT become a general-purpose DTO or utility package.
- **Forbidden Dependencies**: `engine` MUST NOT directly depend on `runtime`, `app`, `tui`, `cli`, `session`, provider implementations, concrete tools, `compaction`, `checkpoint`, `memory`, `automemory`, `metrics`, `tracing`, `recovery`, `reminder`, or `toolresult`.
- **Alternatives Rejected**: A `ToolExecutor` disconnected from the definitions shown to the model, a mutable `RunContext` acting as conversation authority, direct engine access to session persistence, observation only after run completion, parallel Reporter and Observer pipelines, cross-run mutable engine state, and exporting turn-loop helper types preemptively.
- **Reason**: Local Codex source comparison showed useful invariants despite its more coupled and expansive `run_turn`: one live session owner, request-scoped `StepContext`, one `ToolRouter` for both advertised and executable tools, explicit model-client lifetimes, and session-mediated history and persistence updates. Fox preserves these invariants while using narrower Go package boundaries and avoiding Codex's direct turn-loop access to many session services.
- **User Evidence**: The user requested a Codex comparison before confirming the engine design and explicitly adopted the resulting corrected boundary after the identified defects and revised responsibilities were presented.

### DEC-027: Package dependencies form an enforced acyclic graph

- **Status**: confirmed
- **Decision**: The target package dependencies MUST form an explicit directed acyclic graph rather than a nominal linear layer chain. `tui`, `cli`, `feishu`, and `agentops` presentation adapters MAY depend on `app`; `app` MAY depend on `runtime`; `runtime` MAY depend on `engine`, persisted record types in `session`, and the pure `prompt` renderer; and `engine` MAY depend only on the standard library and the narrow `schema` protocol-value package as established by DEC-026. `benchmark` and `autodev` MAY depend directly on `runtime` as runtime control clients. The model-facing `subagent.Tool` MUST depend on a consumer-owned `subagent.Runner` or equivalent function port rather than a concrete runtime child runner; composition MUST map that tool protocol to the runtime-owned `ChildRunner` capability so that `runtime` and `subagent` do not import each other. Concrete providers, tool executors and catalogs, compactors, session stores, and telemetry implementations MUST be injected through their consumer contracts and MUST NOT be constructed or imported by the engine. Runtime MUST NOT import presentation adapters, `app`, or the model-facing subagent adapter. Composition roots MAY import the relevant adapters, runtime constructors, consumer contracts, and concrete implementations only to construct, connect, select, and start an entry point; after construction they MUST NOT execute turn, compaction, rewind, session-commit, or tool workflows or retain mutable workflow state. Bidirectional permission, user-question, and plan-review control MUST flow through runtime-owned interaction ports mapped by `app` to application-owned ports implemented by presentation adapters without reverse imports. Observable facts MUST follow one canonical mapping chain from typed `engine.Observer` facts through `runtime.RunObserver` and application notification DTOs to presentation adapters. Automated architecture import tests MUST enforce the allowed and forbidden package dependencies, including composition-only exceptions.
- **Alternatives Rejected**: Treating the architecture as the unrestricted chain `cmd/* -> adapters -> app -> runtime -> engine -> schema`, allowing `cmd/*` to operate any internal layer after construction, binding `subagent.Tool` directly to a concrete runtime implementation, permitting runtime-to-UI callbacks, maintaining adapter-specific event pipelines, and relying only on documentation or review to prevent forbidden imports.
- **Reason**: Local Codex inspection showed useful runtime invariants but also direct multi-agent access hidden inside one broad Rust crate. Local Claude Code inspection showed explicit caller injection and centralized types used to break concrete circular dependencies. Fox uses Go package boundaries, consumer-owned ports, composition mapping, and executable import constraints to preserve the useful invariants without retaining those hidden cycles or broad-package coupling.
- **User Evidence**: The user requested a second comprehensive Codex and Claude Code comparison of the proposed dependency relationships and explicitly confirmed the corrected graph and dependency rules produced by that review.

### DEC-028: Markdown and Mermaid are the package-dependency authority

- **Status**: confirmed
- **Decision**: `docs/package-dependencies.md` MUST be the single authoritative human-readable package-dependency document. It MUST use Mermaid diagrams together with concise tables or prose to define each package's responsibility, the allowed import DAG, forbidden reverse or cross-layer dependencies, composition-root-only exceptions, runtime interaction request flow, engine-to-presentation observation mapping, and concrete implementation injection points. Any change to a package boundary or dependency rule MUST update this document and the corresponding automated architecture import tests in the same commit. Existing or future draw.io diagrams MAY present a broader visual architecture and MAY link to this document, but they MUST NOT become a second independently maintained authority for package dependency rules.
- **Alternatives Rejected**: Making draw.io the sole dependency authority, maintaining equivalent dependency rules independently in both Markdown and draw.io, and treating dependency documentation as a one-time refactor artifact that may drift after the refactor.
- **Reason**: Text-backed Mermaid is reviewable in source diffs, searchable, renderable with ordinary repository tooling, and maintainable beside executable import constraints. A single authority avoids divergence while still allowing draw.io to serve broader presentation needs.
- **User Evidence**: The user explicitly confirmed the recommended Markdown and Mermaid authority with optional non-authoritative draw.io presentation.

### DEC-029: Architecture import enforcement uses a decreasing allowlist

- **Status**: confirmed
- **Decision**: Automated architecture import tests MUST begin with an exact allowlist of dependency violations that already exist at the frozen pre-refactor baseline. A refactor commit MUST NOT add or broaden an allowlist entry. Each module migration MUST remove every allowlist entry that the migration resolves, and the final PR MUST have an empty allowlist before merge. The allowlist is a temporary migration ledger rather than an exception mechanism for the target architecture.
- **Alternatives Rejected**: Requiring the current architecture to satisfy every target dependency rule before migration begins, postponing all architecture enforcement until the end, and retaining permanent allowlist exceptions in the final architecture.
- **Reason**: A decreasing allowlist makes the target rules executable from the first migration commit while preventing new violations and allowing the currently coupled system to remain buildable during sequential migration.
- **User Evidence**: The user explicitly agreed that architecture tests should use a decreasing allowlist.

### DEC-030: Merged vulnerability repairs define the pre-refactor starting point

- **Status**: confirmed
- **Decision**: The vulnerability and reliability defects confirmed from `docs/agent-architecture-maintainability-review.md` have already been repaired by the merged `#61` and `#63` work and MUST NOT be scheduled for duplicate repair in this refactor. Pre-refactor preparation MUST only establish comprehensive characterization coverage and verify residual risks that have not yet been classified as defects. If a residual-risk test proves an additional pre-existing defect, that defect MUST follow CON-005 through a separate approved requirement, failing regression test, and defect-focused commit before the behavioral baseline is frozen. If no defect is proven, refactoring proceeds without a defect-repair phase.
- **Alternatives Rejected**: Assuming the architecture report vulnerabilities remain unresolved, repeating the merged fixes, and silently correcting an unverified residual risk during architecture migration.
- **Reason**: The current `main` already contains the two dedicated vulnerability repair batches. Distinguishing completed repairs from conditional residual-risk verification keeps the refactor non-functional and prevents duplicate or hidden behavior changes.
- **User Evidence**: The user confirmed that the report vulnerabilities were already fixed and adopted the corrected interpretation that only unclassified residual risks require pre-refactor verification.

### DEC-031: Compatibility tests use immutable versioned baseline fixtures

- **Status**: confirmed
- **Decision**: Persisted-data and output compatibility tests MUST use versioned immutable fixtures generated once from the frozen pre-refactor production baseline and committed to the repository with their source commit, runtime profile or entry source, and expected semantics identified. Tests MUST copy mutable fixtures into test-owned temporary directories before use and MUST NOT regenerate compatibility fixtures at test runtime using the current writer. Fixtures MUST cover the persisted artifacts needed to verify session metadata, runs, messages, transcripts, compact state, checkpoints, memory state, continuation, compaction, rewind, and existing persisted source behavior. Time, identifiers, paths, provider responses, transport responses, and concurrent scheduling MUST be fixed or explicitly controlled; normalization MUST NOT erase fields whose compatibility is under test. Updating a compatibility fixture requires an explicitly approved behavior or format change and MUST NOT occur as part of this behavior-preserving refactor.
- **Alternatives Rejected**: Generating fixtures dynamically with the implementation under test, relying on developer-local sessions, requiring live external services, mutating committed fixtures in place, and broadly normalizing unstable output so that meaningful compatibility changes become invisible.
- **Reason**: A current reader successfully reading data written by the same current implementation does not prove backward compatibility. Immutable baseline artifacts provide independent, repeatable evidence across the entire refactor.
- **User Evidence**: The user explicitly confirmed the hermetic test definition and required fixed persisted fixtures that can be rerun in any environment.

### DEC-032: Shared black-box contracts drive old and new implementations

- **Status**: confirmed
- **Decision**: Pre-refactor characterization MUST combine a shared black-box contract suite with entry-specific tests. Shared scenarios MUST describe controlled model responses, tool behavior, run inputs, interaction responses, expected ordered facts, outcomes, and persisted artifacts without freezing current `AgentEngine`, `AgentRunner`, or another replaceable internal API. Entry-specific tests MUST retain ownership of presentation input, output formatting, terminal state, transport messages, and process exit behavior. During migration, a test-only adapter for the current implementation and a test-only adapter for the target runtime MUST execute the same applicable shared scenarios; both MUST remain green until parity is established and the migrated old implementation path is removed. Shared fixtures and scenario expectations MUST have one authority and MUST NOT be duplicated per implementation.
- **Alternatives Rejected**: Binding all characterization tests to current internal APIs, duplicating expected results for old and new implementations, relying only on entry-level end-to-end tests, and removing the old path before the target path passes the same contracts.
- **Reason**: Reusable black-box scenarios preserve behavioral authority across internal API migration, permit direct differential verification, and avoid turning obsolete internal types into permanent compatibility requirements.
- **User Evidence**: The user explicitly adopted the shared black-box contract suite, entry-specific tests, and migration-time old/new differential execution model.

### DEC-033: Shared runtime characterization has a minimum scenario catalog

- **Status**: confirmed
- **Decision**: The shared black-box characterization suite MUST implement every scenario in the Confirmed Shared Runtime Characterization Catalog below. Scenario identifiers are stable traceability labels rather than proposed production packages or types: `RT` means Runtime Turn, `ST` means Streaming, `TL` means Tool Lifecycle, `CX` means Context, `PL` means Policy, and `RS` means Run and Session. Existing unit tests MAY supply lower-level evidence but MUST NOT replace a missing end-to-end shared scenario. A scenario that exposes a previously unclassified defect MUST follow CON-005 rather than silently changing its expectation during refactoring. Features present only in reference projects, including Codex WebSocket, MCP dynamic environment, pending steering and rollout-budget behavior and Claude Code stop hooks, microcompaction, context collapse, cost budgets, and structured-output retries, MUST NOT become Fox compatibility requirements.
- **Alternatives Rejected**: Treating the existing engine unit tests as complete runtime characterization, copying reference-project features into Fox's baseline, binding scenarios to current internal engine APIs, and changing a baseline expectation to match target implementation drift.
- **Reason**: Codex integration suites demonstrate the need to test ordered tool concurrency, interruption history, turn-state lifetimes, and compaction-resume-rollback combinations. Claude Code source demonstrates explicit cleanup of failed streaming attempts, terminal tool-result completion, ordered concurrent tool output, and transcript flushing before completion. Fox needs the applicable invariants plus its own thinking, completion/TODO gate, reminder/recovery, and large-result behavior, without importing unrelated product features.
- **User Evidence**: The user requested a Codex and Claude Code source comparison and gap audit and explicitly confirmed the resulting `RT`, `ST`, `TL`, `CX`, `PL`, and `RS` minimum catalog.

### DEC-034: TUIInteractive has profile and presentation characterization catalogs

- **Status**: confirmed
- **Decision**: Before production architecture moves, the complete `TUIInteractive` behavior bundle MUST be covered by the Confirmed TUIInteractive Profile Characterization Catalog and the TUI-owned presentation catalog below. `PF-TUI` scenarios verify profile resolution, runtime and application wiring, and the observable and persisted effects of TUI-specific capabilities through the shared old/new black-box adapters. `UI-TUI` scenarios remain owned by the TUI adapter and verify terminal input, rendering, overlays, settings, and process interaction without being duplicated as runtime contracts. `TUIInteractive` MUST execute every shared `RT`, `ST`, `TL`, `CX`, and `PL` scenario and `RS-001` through `RS-006`; `RS-007` remains a global headless-runtime invariant and MUST NOT be duplicated as a TUI-profile scenario.
- **Cross-Validation**: Local Codex source was checked for queued-input dispatch, interruption, session replay, model switching, permission confirmation, plan transition, and backtracking behavior. Local Claude Code source was checked for guarded queue processing, query cleanup, cancellation and automatic input restoration, session resume, permission interaction, compaction projection, and stale-completion protection. This comparison strengthened explicit assertions for model-dependent context refresh, duplicate-free resume, overlay-gated queue dispatch, guarded cancel restoration, visible-history versus model-context projection, and stale prior-run completion. Reference-only behavior was not imported into Fox.
- **Alternatives Rejected**: Treating existing TUI unit tests as an untraced complete baseline, placing terminal rendering expectations in shared runtime contracts, taking a Cartesian product of every shared scenario and presentation state, and copying Codex or Claude Code features that Fox does not currently provide.
- **Reason**: The TUI is the broadest product-root profile and combines long-lived session behavior, mutable future-run controls, interactive permission and plan ports, rewind, manual compaction, streaming presentation, and queueing. Explicit profile and presentation catalogs preserve those boundaries without coupling the target runtime to Bubble Tea.
- **User Evidence**: The user reviewed and accepted the complete proposed TUI scenario set and requested an additional Codex and Claude Code source cross-validation and gap audit before recording it.

### DEC-035: CLIExec has profile and presentation characterization catalogs

- **Status**: confirmed
- **Decision**: Before production architecture moves, the complete `CLIExec` behavior bundle MUST be covered by the Confirmed CLIExec Profile Characterization Catalog and CLI-owned presentation catalog below. `PF-CLI` scenarios verify immutable profile resolution, one-shot runtime wiring, non-interactive capability boundaries, persisted effects, staged completion, and runtime outcomes through the shared old/new black-box adapters. `UI-CLI` scenarios verify process routing, prompt acquisition, configuration precedence, exact output bytes and stream ownership, errors, and exit status in the dedicated CLI presentation adapter. `CLIExec` MUST execute every shared `RT`, `ST`, `TL`, `PL`, and `RS` scenario plus `CX-001` through `CX-005` and `CX-008`; `CX-006` and `CX-007` are not repeated for this profile because it exposes neither manual compaction nor rewind.
- **Terminology**: A model-visible tool surface is the set of tool definitions supplied to a model request. A runtime-executable tool surface is the set of tool calls the same immutable run or turn snapshot can actually execute. These surfaces MUST agree; the documentation MUST use these terms rather than the unnatural literal translation "advertised tool surface."
- **Cross-Validation**: Codex exec source and tests were checked for prompt and stdin parsing, resume, human-output deduplication, failure exit status, artifact finalization, and separation of runtime events from formatting. Claude Code's non-interactive `QueryEngine` and `ask` wrapper were checked for the absence of interactive permission prompting, run-scoped configuration, result and partial-event behavior, and `finally`-based state finalization. Fox-specific output and permission behavior remains authoritative; Codex TTY/JSONL behavior and Claude Code stream-JSON, budget, hook, and structured-output features were not imported.
- **Alternatives Rejected**: Reusing the TUI adapter for print mode, allowing runtime code to print CLI labels, treating a slash-prefixed prompt as a TUI command, waiting for automemory extraction before showing the completed result, exiting before tracked extraction drains, and copying reference-project output modes that Fox does not currently expose.
- **Reason**: A one-shot CLI has a smaller interaction surface than the TUI but stronger process-level compatibility requirements. Explicitly separating runtime outcomes from stdout, stderr, and exit behavior preserves headless reuse and exact user-visible output while retaining the current extraction shutdown guarantee.
- **User Evidence**: The user confirmed the complete proposed `PF-CLI` and `UI-CLI` scenario set, the model-visible versus runtime-executable tool-surface terminology, and the output-first, exit-after-drain completion interpretation.

### DEC-036: FeishuRemote has profile and transport-adapter characterization catalogs

- **Status**: confirmed
- **Decision**: Before production architecture moves, the complete `FeishuRemote` behavior bundle MUST be covered by the Confirmed FeishuRemote Profile Characterization Catalog and Feishu-owned transport-adapter catalog below. `PF-FEI` scenarios verify immutable profile resolution, typed task execution, session and scheduling behavior, capability and permission boundaries, context, compaction, observation, outcomes, and cleanup through the shared old/new black-box adapters. `UI-FEI` scenarios verify process configuration, Feishu HTTP protocol handling, event translation, task delivery, exact outbound messages, delivery failures, and approval transport without making Lark SDK or Chinese message formatting runtime contracts. `FeishuRemote` MUST execute shared `RT-001` through `RT-003` and `RT-005` through `RT-007`, `TL-001` and `TL-004` through `TL-008`, `CX-001` through `CX-005` plus `CX-008`, and every `PL` and `RS` scenario. It MUST NOT execute `RT-004` because thinking is disabled, `ST-001` through `ST-006` because the current Feishu path uses no streaming provider or model-delta observer, `TL-002` or `TL-003` because the permission-decorated registry treats all calls as non-parallel, or `CX-006` and `CX-007` because the profile exposes neither manual compaction nor rewind. Expectations affected by `DV-FEI-001` through `DV-FEI-010` MUST be frozen only after their CON-005 verification and any separately approved defect corrections are complete.
- **Cross-Validation**: Local Codex app-server source was checked for request, thread, turn, item, and approval correlation; exactly-once pending-request removal on response or error; turn-abort cancellation; connection cleanup; and shutdown draining. Local Claude Code remote-session and print-mode source was checked for current-process and persisted-session UUID deduplication, duplicate acknowledgement, command lifecycle completion, request-ID-keyed approval state, cancellation and disconnect cleanup, stale-response rejection, and explicit outbound-send failure results. This comparison strengthened durable and concurrent duplicate verification, cross-task stale-notification isolation, unknown-event handling, approval terminal-state cleanup, and outbound-delivery failure verification. Codex or Claude Code WebSocket, reconnection, structured remote protocol, and other reference-only features remain outside Fox requirements.
- **Alternatives Rejected**: Binding runtime to Lark types or message text, treating remote transport behavior as engine behavior, freezing a suspected defect as compatibility before verification, relying on existing narrow Feishu unit tests, and importing reference-project transport features not currently provided by Fox.
- **Reason**: Feishu combines a headless agent runtime with durable conversation selection, concurrency, timeout, remote approval, and an externally retried webhook and message transport. Separate profile and adapter catalogs preserve complete behavior while keeping transport concerns outside runtime.
- **User Evidence**: The user accepted the complete proposed `PF-FEI` and `UI-FEI` scenario sets, requested Codex and Claude Code source cross-validation before recording them, and explicitly adopted the resulting duplicate-delivery, stale-correlation, unknown-event, approval-cleanup, and delivery-failure corrections.

### DEC-037: AgentOpsTask has profile and transport-adapter characterization catalogs

- **Status**: confirmed
- **Decision**: Before production architecture moves, the complete `AgentOpsTask` behavior bundle MUST be covered by the Confirmed AgentOpsTask Profile Characterization Catalog and AgentOps-owned transport-adapter catalog below. `PF-AOP` scenarios verify immutable profile resolution, fresh-session task execution, bounded scheduling, the incident-analysis prompt and tool surface, permission and child-run boundaries, context and memory, compaction, observation, outcomes, cleanup, and runtime isolation through the shared old/new black-box adapters. `UI-AOP` scenarios verify process configuration, the shared Feishu gateway mapping, deduplication, exact outbound messages, delivery failures, approval callbacks, and coordinated shutdown without making Lark SDK types or Chinese message formatting runtime contracts. `AgentOpsTask` MUST execute shared `RT-001` through `RT-003` and `RT-005` through `RT-007`, `TL-001` and `TL-004` through `TL-008`, the fresh-session branch of `CX-001`, `CX-003` through `CX-005`, `CX-008`, every `PL` scenario, `RS-001` through `RS-004`, and `RS-006` through `RS-007`. Only the distinct-fresh-session isolation and concurrency branch of `RS-005` applies. It MUST NOT execute `RT-004` because thinking is disabled, `ST-001` through `ST-006` because the current path invokes no streaming provider and emits no model deltas, `TL-002` or `TL-003` because the permission-decorated registry preserves non-parallel execution, or `CX-002`, `CX-006`, or `CX-007` because every task creates a fresh session and the profile exposes neither manual compaction nor rewind. Expectations affected by `DV-AOP-001` through `DV-AOP-006`, `DV-FEI-001`, or `DV-FEI-007` MUST be frozen only after their CON-005 verification and any separately approved defect corrections are complete.
- **Cross-Validation**: Local Codex app-server source was checked for request, connection, thread, turn, item, and approval correlation; pending-request removal; rejection of late work after close; and shutdown draining of accepted handlers. Local Claude Code source was checked for in-process and persisted-session UUID deduplication, duplicate command acknowledgement, request-ID-keyed approval cleanup, explicit outbound-send failure, durable remote-task metadata, guarded terminal transitions, stale-completion rejection, and output-before-terminal ordering. This comparison found no reason to change the AgentOps profile boundary, but strengthened empty message-ID coverage, task/session/run/terminal correlation, stale cross-task completion isolation, exactly-one terminal outcome, accepted-work shutdown semantics, and final-resolved-path containment. Codex RPC and Claude Code WebSocket, reconnection, polling, and resumable remote-task protocols remain outside Fox requirements.
- **Alternatives Rejected**: Treating AgentOps as a second FeishuRemote profile without its incident policy, placing incident prompt or `log_search` ownership in the generic runtime, freezing suspected deduplication and shutdown defects as compatibility, requiring general remote-task resume, or importing reference-project transport protocols that Fox does not currently expose.
- **Reason**: AgentOps is a headless runtime control client with a fixed incident-analysis policy, fresh isolated sessions, an additional bounded log capability, and a Feishu transport. Separate profile and adapter catalogs preserve its runtime behavior while keeping process queues, deduplication, message delivery, and Lark protocol concerns outside the core runtime.
- **User Evidence**: The user accepted the complete proposed `PF-AOP` and `UI-AOP` scenario sets and requested Codex and Claude Code source cross-validation before recording them. The user explicitly confirmed the resulting message-identity, task-correlation, terminal-state, shutdown, and resolved-log-path refinements.

### DEC-038: BenchmarkEval has profile and evaluation-control characterization catalogs

- **Status**: confirmed
- **Decision**: Before production architecture moves, the complete `BenchmarkEval` behavior bundle MUST be covered by the Confirmed BenchmarkEval Profile Characterization Catalog and benchmark evaluation-control catalog below. `PF-BEN` scenarios verify immutable profile resolution, case-controlled runtime inputs, fresh fixture workspaces and sessions, serial repeat isolation, exact capabilities, prompt and memory behavior, compaction, policies, runtime outcomes, evaluation handoff, termination, provenance, and runtime-control isolation through the shared old/new black-box adapters. `EV-BEN` scenarios verify case-file and process inputs, fixture materialization, harness construction, validations, repeat orchestration, summaries, JSON reports, failure precedence, and process status without treating benchmark as a user-interaction adapter. `BenchmarkEval` MUST execute shared `RT-001` through `RT-003` and `RT-005` through `RT-007`, `TL-001` through `TL-004` and `TL-006` through `TL-008`, the fresh-session branch of `CX-001`, `CX-003` through `CX-005`, `CX-008`, `PL-001`, `PL-002`, `PL-004`, `RS-001` through `RS-004`, and `RS-006` through `RS-007`. `TL-005` applies to tool definitions, aliases, invocation, and parallel-safety lookup but not permission assessment; only the fresh-repeat isolation and serial-execution branch of `RS-005` applies. It MUST NOT execute `RT-004` because thinking is disabled, `ST-001` through `ST-006` because the current path installs no reporter and emits no streaming deltas, `CX-002`, `CX-006`, or `CX-007` because every repeat creates a fresh session and exposes neither manual compaction nor rewind, or `PL-003` and `PL-005` because it installs neither a general completion gate nor a next-turn-reminder source. Expectations affected by `DV-BEN-001` through `DV-BEN-007` MUST be frozen only after their CON-005 verification and any separately approved defect corrections are complete.
- **Terminology**: `EV-BEN` means Benchmark Evaluation Control Plane. It covers evaluation-specific process and orchestration behavior and MUST NOT be named `UI-BEN`, because benchmark directly controls and evaluates the runtime harness and is not a presentation adapter or user-interaction layer.
- **Cross-Validation**: Local Codex and Claude Code source contain no runner equivalent to Fox `cmd/bench`, so their test frameworks are not treated as product architecture templates. Codex rollout-trace identity and terminal states, process-group cleanup, canonical path tests, and snapshot normalization and Claude Code normalized input-keyed VCR fixtures, fail-closed CI fixture policy, bounded shell output, process-tree cleanup, and explicit exit status were used only to strengthen reproducibility, provenance, terminal-state separation, path security, output bounds, and process cleanup. Codex rollout bundles, Claude VCR recording, sandbox services, and reference-only transport or protocol features remain outside Fox requirements.
- **Alternatives Rejected**: Routing benchmark through the user-facing application layer, classifying its control plane as a UI adapter, independently assembling a benchmark engine, treating a failed evaluation as a successful process, relying on manually maintained fidelity claims, conflating runtime completion with validation success, and copying reference-project fixture or rollout protocols.
- **Reason**: Benchmark is an evaluation and feedback client of the real runtime. Its results are credible only when runtime behavior is shared through the resolved profile, intentional differences are explicit, repeats are isolated, validators are bounded, and every result can be correlated and reproduced without coupling runtime to benchmark reporting.
- **User Evidence**: The user accepted `PF-BEN-001` through `PF-BEN-016`, `EV-BEN-001` through `EV-BEN-011`, and `DV-BEN-001` through `DV-BEN-007` after requesting Codex and Claude Code source cross-validation and explicitly confirming the resulting refinements.

### DEC-039: ChildRun has profile and invocation-adapter characterization catalogs

- **Status**: confirmed
- **Decision**: Before production architecture moves, the complete synchronous `ChildRun` behavior bundle MUST be covered by the Confirmed ChildRun Profile Characterization Catalog and child-invocation-adapter catalog below. `PF-CHD` scenarios verify immutable child profile resolution, parent lineage, fresh child sessions, synchronous one-shot execution, frozen provider and model scope, turn ceilings, exact capability and permission boundaries, one-level nesting enforcement, prompt and memory behavior, compaction, outcomes, cancellation, cleanup, and runtime ownership through the shared old/new black-box adapters. `IA-CHD` scenarios verify the model-facing `delegate_task` tool and fork-skill invocation protocols without classifying either as presentation UI. `ChildRun` MUST execute shared `RT-001` through `RT-003` and `RT-005` through `RT-007`, every `TL` scenario, `CX-003` through `CX-005` and `CX-008`, `PL-001` and `PL-002`, and `RS-001` through `RS-004`, `RS-006`, and `RS-007`. The fresh-session branch of `CX-001` and the independent-child-session plus synchronous-parent-wait branch of `RS-005` apply. It MUST NOT execute `RT-004` because thinking is disabled, any `ST` scenario because the child path emits no model-delta stream, `CX-002`, `CX-006`, or `CX-007` because it never resumes and exposes neither manual compaction nor rewind, or `PL-003` through `PL-005` because it installs no general completion gate, TODO gate, or next-turn-reminder source. Expectations affected by `DV-CHD-001` through `DV-CHD-006` MUST be frozen only after their CON-005 verification and any separately approved defect corrections are complete.
- **Cross-Validation**: Local Codex source was checked for live parent configuration snapshots, explicit parent and depth lineage, child-capacity limits, terminal state, cancellation, and resource cleanup. Local Claude Code source was checked for child tool filtering, permission isolation, synchronous cancellation inheritance, separate transcripts, child compaction, partial outcomes, and unconditional cleanup. The comparison strengthened Fox's capability intersection, lineage, terminal-correlation, process-tree, and partial-result requirements while preserving Fox's synchronous single-level child model. Background children, resume and send-input, worktree isolation, agent trees, MCP, and team orchestration remain outside scope.
- **Alternatives Rejected**: Treating `delegate_task` as the child runtime itself, allowing adapters to assemble their own engine, inheriting a mutable parent configuration, trusting prompt text as the nesting guard, introducing background or resumable children, adding an agent tree, or importing reference-project multi-agent and transport capabilities.
- **Reason**: Child execution is a privileged nested runtime capability, while `delegate_task` and the fork skill are only invocation adapters. Separate catalogs preserve the model-facing protocols and the runtime lifecycle independently, enforce one-level capability ceilings in code, and prevent child construction from leaking into tools, presentation, or application adapters.
- **User Evidence**: The user accepted the cross-validated `PF-CHD-001` through `PF-CHD-022`, `IA-CHD-001` through `IA-CHD-006`, and `DV-CHD-001` through `DV-CHD-006` set, including its explicit exclusions and one-level child constraint.

### DEC-040: AutodevPipeline has runtime-profile, control-plane, and entry-adapter characterization catalogs

- **Status**: confirmed
- **Decision**: Before production architecture moves, the complete `AutodevPipeline` behavior bundle MUST be covered by the Confirmed AutodevPipeline Profile Characterization Catalog, Autodev control-plane catalog, and Autodev entry-adapter catalog below. `PF-AUT` scenarios verify immutable core runtime resolution, per-item session lifecycle, model scope, exact capabilities, Engineer-mediated questions, context, memory, compaction, policies, outcomes, cleanup, and runtime-control isolation. `CP-AUT` scenarios verify configuration, backlog and ledger authority, scheduling, worktrees, fixed CodexSpec stages, deterministic verification, quality gates, GitHub publication, recovery, terminal state, cleanup, and observation. `UI-AUT` scenarios preserve the CLI command, TUI command, reporter mappings, cancellation, output ownership, and process exit behavior without making either presentation format a runtime contract. `AutodevPipeline` MUST execute shared `RT-001` through `RT-003` and `RT-005` through `RT-007`, `TL-001` through `TL-004` and `TL-006` through `TL-008`, `CX-003` through `CX-005` and `CX-008`, `PL-001`, `PL-002`, `PL-004`, `PL-005`, and every `RS` scenario. `TL-005` applies to definitions, aliases, invocation, and parallel-safety lookup but not permission assessment; the fresh-session and same-process existing-session branches of `CX-001` apply; and `RS-005` applies to serialization within one item plus isolation between distinct item sessions, not concurrent item execution. It MUST NOT execute `RT-004` because thinking is disabled, any `ST` scenario because the current path emits no model deltas, `CX-002`, `CX-006`, or `CX-007` because the pipeline does not reopen a core session and exposes neither manual compaction nor rewind, or `PL-003` because it installs no general runtime completion gate. Expectations affected by `DV-AUT-001` through `DV-AUT-010` MUST be frozen only after their CON-005 verification and any separately approved defect corrections are complete.
- **Terminology**: `CP-AUT` means Autodev Control Plane and covers deterministic workflow orchestration outside runtime. `UI-AUT` covers the `fox autodev` and `/autodev` presentation adapters. The Engineer Agent's question and review adaptation remains part of `CP-AUT`, not a human permission coordinator and not a presentation adapter.
- **Cross-Validation**: Local Codex has no equivalent requirements-first backlog pipeline, but its cloud-task implementation uses stable task and attempt identities, explicit terminal states, apply preflight, operation exclusion, stale-result rejection, and failure-sensitive exit status. Local Claude Code task and query implementations use explicit terminal transitions, parent-linked cancellation, cleanup registration, exactly-once notification, pre-result storage flush, and unconditional finalization. The comparison strengthened Fox's durable item identity, persisted transition, terminal correlation, cancellation, cleanup, and publication verification while leaving cloud tasks, background agents, best-of-N, resumable messaging, and multi-worker execution outside scope.
- **Alternatives Rejected**: Treating Autodev as presentation UI, placing its ledger or stage machine in runtime, allowing Autodev to assemble a separate engine, trusting the core Agent's completion claim instead of deterministic verification, conflating Engineer answers with permission approval, introducing parallel item execution, auto-merging PRs, or importing reference-project cloud and background-task protocols.
- **Reason**: Autodev is a privileged runtime control client with its own durable deterministic workflow and two presentation adapters. Separating these responsibilities preserves use of the real runtime while preventing backlog, ledger, worktree, quality-gate, and GitHub publication concerns from entering engine or runtime packages.
- **User Evidence**: The user explicitly confirmed the complete cross-validated `PF-AUT-001` through `PF-AUT-016`, `CP-AUT-001` through `CP-AUT-025`, `UI-AUT-001` through `UI-AUT-006`, and `DV-AUT-001` through `DV-AUT-010` scenario set.

### DEC-041: Production migration follows a bottom-up strangler sequence with profile-atomic cutovers

- **Status**: confirmed
- **Decision**: After the complete Phase 0 baseline is frozen, production code MUST migrate through the Confirmed Production Migration Sequence and Commit Boundaries below. The sequence first stabilizes pure rendering and persistence boundaries, then constructs and proves the target engine and runtime beside the current implementation, then cuts over one Runtime Profile at a time from the simplest control client to the most stateful presentation adapter, and finally removes every temporary facade and old implementation. Every `Mxx` identifier defines an independently reviewable, testable, bisectable, and revertible commit boundary unless the narrowly defined mechanical-combination exception below applies.
- **Cutover Invariant**: A profile MUST NOT have two selectable production execution paths. Before a profile cutover, its old and target test adapters MUST pass the same applicable shared and profile characterization scenarios. The cutover commit MUST switch every production path for that profile and remove that profile's obsolete production wiring. Test-only access to the old implementation MAY remain solely for differential verification until the final profile reaches parity.
- **Compatibility Invariant**: Temporary compatibility facades MAY exist only to keep unmigrated consumers compiling and behaving identically. Each facade MUST have an identified deletion boundary in this sequence, MUST NOT become a new extension point, and MUST NOT expand the architecture-test allowlist. The allowlist may only decrease after baseline freeze.
- **Alternatives Rejected**: A single big-bang rewrite; beginning with TUI or application entry movement before the runtime owner exists; splitting commits mechanically by file or package without a complete behavioral boundary; deleting the old implementation before all dependent profiles have migrated; and retaining old and new production paths behind a long-lived flag.
- **Reason**: Bottom-up movement follows dependency direction and establishes stable data and execution contracts before stateful consumers move. Shared old/new black-box scenarios make each profile cutover observable, while profile-atomic commits avoid half-migrated entry points and preserve reliable review, bisection, and rollback.
- **User Evidence**: The user explicitly confirmed the proposed baseline freeze, `M01` through `M27` production migration order, independent commit boundaries, profile-atomic cutover rule, decreasing architecture allowlist, and final compatibility gate.

### DEC-042: Proven Feishu defects are corrected before baseline freeze

- **Status**: confirmed
- **Decision**: The executable T040 proofs classified `DV-FEI-001` through `DV-FEI-010` as pre-existing defects. They MUST be corrected before T041 and before immutable fixture generation. Each correction MUST follow Red-Green-Refactor, retain its trace evidence, and land as an independently Green defect-focused commit rather than as architecture refactoring.
- **DV-FEI-001 correction**: `Gateway.Server` MUST expose `POST /webhook/approval`. It MUST authenticate `Authorization: Bearer <verification token>` using constant-time comparison, accept only bounded JSON containing `approval_id`, `approved`, and optional `reason`, call the shared approval store, and return deterministic `204`, `400`, `401`, `404`, or `409` outcomes without exposing approval existence to unauthenticated callers.
- **DV-FEI-002 correction**: Production message acceptance MUST use a project-independent user-state file as a durable at-most-once idempotency authority keyed by non-empty Feishu `message_id`. Reservation and persistence MUST be atomic and concurrency-safe. Sequential, concurrent, post-completion, and post-restart duplicates MUST be acknowledged successfully without enqueueing another task. A persistence or enqueue failure MUST be observable and MUST roll back the reservation when the process remains alive. This correction does not claim distributed exactly-once execution after an unclean crash between an external side effect and local terminal persistence.
- **DV-FEI-003 correction**: A message lacking a non-empty sender open ID MUST be acknowledged to Feishu but rejected before task construction, durable idempotency reservation, session lookup, or enqueue. The validation failure MUST be locally observable without causing retry amplification.
- **DV-FEI-004 correction**: The task timeout MUST begin when the Runner accepts a task, include all same-session queue or lock waiting, and prevent an expired or cancelled task from beginning execution later. Cancellation while waiting MUST release all queue/lock references and produce one correlated terminal outcome.
- **DV-FEI-005 correction**: Runner scheduling MUST preserve FIFO order per `(chat_id, sender_id)` session key. Waiting tasks MUST NOT consume global execution permits. A runnable task from another session may use available capacity, and completion of one session head MUST make only that session's next head eligible.
- **DV-FEI-006 correction**: Shutdown MUST stop HTTP intake first, then stop task acceptance, drain accepted work on ordinary channel closure, and cancel queued/in-flight work on process cancellation into one correlated terminal state. `cmd/feishu` MUST use signal-aware context, call HTTP shutdown, close the task channel only after intake stops, and wait for Runner completion subject to an explicit shutdown deadline.
- **DV-FEI-007 correction**: Approval resolution MUST be non-blocking and exactly once. The first valid resolution atomically claims the pending request; duplicate or concurrent resolution returns conflict immediately; unknown, late, expired, or cancelled IDs return not found; cancellation and timeout remove pending state without permitting a later success.
- **DV-FEI-008 correction**: Feishu MUST freeze the selected provider model once per task and pass the same model snapshot to engine metadata and `CompactionConfig.Model`; known models MUST use their registered context window instead of the fallback.
- **DV-FEI-009 correction**: A recovered task panic MUST release scheduling and run-scoped resources and emit exactly one correlated failed `TaskOutcome`. The Runner MUST attempt one bounded terminal failure delivery without using an already-cancelled task context; failure of that delivery is handled by the delivery-outcome rule rather than hidden.
- **DV-FEI-010 correction**: Receipt, session, lifecycle, final, ordinary-failure, panic-failure, and cancellation delivery errors MUST be captured as typed delivery failures associated with task, chat, stage, and cause. Runner and Reporter MUST report them to an injected non-blocking outcome observer; the production composition root MUST observe and log them. User-visible text remains bounded before transport invocation. A failed transport cannot be represented as a successful user delivery.
- **Alternatives Rejected**: An unauthenticated approval callback; process-local duplicate suppression; empty-sender fallback identity; mutex waiting outside timeout; global permits acquired before per-session eligibility; fire-and-forget shutdown; blocking duplicate approval sends; implicit 128K compaction fallback; panic-only logging; and silently ignored delivery errors.
- **Reason**: These corrections establish the safe post-fix behavior that the refactor must preserve while keeping transport, scheduling, approval, and delivery concerns outside the future core engine.
- **User Evidence**: After T040 proved all ten defects, the user explicitly confirmed the recommended correction principles: durable message idempotency, missing-sender rejection, context-aware FIFO session scheduling, coordinated shutdown and drain, non-blocking exactly-once approval, selected-model compaction, correlated panic terminal outcomes, and typed observable delivery failures.

### DEC-043: Proven AgentOps defects are corrected before baseline freeze

- **Status**: confirmed
- **Decision**: The executable T041 proofs classified `DV-AOP-001` through `DV-AOP-006` as pre-existing defects. They MUST be corrected before T042 and immutable fixture generation. Each correction MUST follow Red-Green-Refactor, retain its trace evidence, and land as an independently Green defect-focused commit rather than as architecture refactoring.
- **DV-AOP-001 correction**: The shared Feishu Gateway `DeliveryStore` MUST be the sole durable authority that decides whether a non-empty message ID is accepted for task delivery. `cmd/agentops` MUST compose the same project-independent file authority and MUST remove its second process-local acceptance `Deduper`. Transport-local echo or replay suppression MAY exist only when it has a distinct documented transport purpose, cannot reserve, reject, or complete a task accepted by the Gateway, and cannot replace durable acceptance. Sequential, concurrent, post-completion, and post-restart duplicates MUST retain the corrected Gateway acknowledgement and at-most-once enqueue behavior; a live enqueue failure MUST release the durable reservation.
- **DV-AOP-002 correction**: One coordinated lifecycle MUST own HTTP intake, the Feishu-to-AgentOps bridge, both capacity-64 task channels, queued tasks, and active Runner work. Shutdown MUST stop HTTP intake first, wait until no producer can reach the upstream channel, close and drain the bridge into the downstream channel, then close downstream intake. Ordinary channel closure MUST drain accepted tasks; process cancellation MUST cancel queued and active tasks into one correlated terminal outcome. The composition root MUST wait for listener, bridge, and Runner completion under one explicit deadline and MUST NOT close a channel while a live producer can send to it.
- **DV-AOP-003 correction**: Every accepted AgentOps task MUST reach exactly one typed terminal `TaskOutcome` through one Runner-owned terminal transition. The public terminal status set is `completed`, `failed`, or `cancelled`; panic and timeout remain typed terminal reasons or causes rather than additional success-like states. Panic, timeout, parent cancellation, ordinary failure, and success MUST release scheduling and run-scoped resources before completion is reported. Any terminal user notification that cannot use the task context MUST use one fresh background-derived context with a strict deadline. No late or duplicate completion may be attributed to another task.
- **DV-AOP-004 correction**: AgentOps MUST freeze provider protocol and selected model once per accepted task and pass that same immutable snapshot to engine metadata and `CompactionConfig.Model`. Compaction threshold, context-window selection, telemetry, model invocation, and child-run inheritance MUST not perform a second mutable model selection for the active task.
- **DV-AOP-005 correction**: Initial-session, final-result, ordinary-failure, panic-failure, timeout, and cancellation sends MUST report typed delivery failures correlated by task, chat, stage, and cause to an injected non-blocking observer with panic isolation. The production composition root MUST install an observer. AgentOps MUST bound every user-visible message at its Runner messenger boundary before invoking any fake or real transport. Runtime failure may make one failure-delivery attempt; failure of final or failure delivery MUST be observed and MUST NOT recursively invoke another fallback message through the same failing transport. Task outcome and delivery outcome remain separately inspectable.
- **DV-AOP-006 correction**: `log_search` MUST open only a regular log file whose final filesystem resolution remains beneath the configured log root. Validation and open MUST use a rooted filesystem operation or an equivalently race-resistant mechanism, not a lexical join or a check-then-unrestricted-open sequence. Direct traversal, separators, symlink escape, and non-regular targets MUST fail closed while valid in-root files preserve case-insensitive ordered matching, the default and maximum result limits, cancellation, the 200-line maximum, and the one-MiB-per-line scanner bound.
- **Cross-Validation**: Local Codex uses immutable per-turn model/provider state for both sampling and compaction, typed `Completed`/`Interrupted`/`Failed` turn outcomes, guaranteed delivery for terminal in-process notifications under backpressure, bounded shutdown with drain then abort, and canonical-root checks with explicit symlink-escape tests. Local Claude Code freezes `mainLoopModel` in the query/tool context and uses it for compaction, links cancellation through `AbortController`, waits for active sessions and pending cleanup during bridge shutdown, records typed terminal task states, and atomically suppresses duplicate terminal notifications. Claude Code also distinguishes outbound echo suppression from inbound replay suppression; these are transport safeguards rather than competing durable task-acceptance authorities. Its filesystem permission checks evaluate original and resolved paths and its output scanner excludes symlinks. These references support the six correction semantics without importing Codex RPC or Claude Code WebSocket, polling, reconnection, resumable-session, or multi-process protocols.
- **Alternatives Rejected**: A second process-local task-acceptance map; declaring every auxiliary transport dedupe forbidden; fire-and-forget bridge shutdown; closing channels while producers remain live; panic-only logging; terminal sends on an already-cancelled context; mutable or separately selected compactor models; recursive failure notification; unbounded transport text; lexical-only log validation; and check-then-open symlink handling.
- **Reason**: These corrections establish one acceptance authority, one accepted-task lifecycle, one terminal transition, one model snapshot, observable delivery semantics, and a race-resistant log boundary before characterization freezes behavior. They keep transport, process coordination, and incident-log policy outside the future core engine.
- **User Evidence**: After requesting an additional Codex and Claude Code source cross-validation and gap audit, the user authorized adoption of the six AgentOps correction principles when no conflicting design defect was found.

## Confirmed Production Migration Sequence and Commit Boundaries

This is the authoritative production migration order after NEED-007's complete Phase 0 gate. It is a controlled strangler migration inside one integration branch: the target implementation is proven through shared contracts while unmigrated profiles continue using the current production path. Parallel old and target implementations are permitted only through test adapters; each product profile has exactly one production path at every commit.

### Phase 0: Freeze the behavioral authority

| Boundary | Scope | Required gate |
|---|---|---|
| `B00` | Complete every shared, profile, presentation, transport, evaluation, child-invocation, and Autodev-control characterization test; materialize immutable fixtures; complete all `DV-*` verification and separately approved defect commits; establish the exact initial architecture-test allowlist; publish the scenario-to-test evidence and freeze the corrected baseline commit. Phase 0 MAY contain multiple test-only and defect-focused commits, but `B00` is the single baseline freeze point. | NEED-007 and CON-007 are fully satisfied; every mandatory scenario passes against the current production implementation and under `go test ./...`; no unresolved `DV-*` item remains; fixtures identify their source commit and semantics; no production architecture has moved. |

No `Mxx` production migration commit may begin before `B00` passes.

### Phase 1: Stabilize pure and persisted dependency endpoints

| Boundary | Production responsibility moved | Required gate and temporary compatibility |
|---|---|---|
| `M01` | Establish `internal/prompt` as the pure prompt-fragment renderer. Keep discovery, ordering, injection decisions, and complete-context lifecycle in the existing runtime path. | Prompt golden tests and every applicable profile context scenario pass. `internal/context` MAY temporarily forward to `internal/prompt` for unmigrated callers and is deleted at `M26`. |
| `M02` | Establish the `internal/session` persistence vocabulary and ownership for stored session and run records, identifiers, and `FileStore`, without making it the live runtime-session owner. | Versioned persisted-session fixtures plus resume, compaction, checkpoint, and rewind compatibility pass. Temporary wrappers or aliases MAY preserve unmigrated callers and are deleted at `M26`. |
| `M03` | Make `memory.Store` the sole session-working-memory abstraction and remove duplicate use of persistence-layer working-memory representations from active behavior. This commit does not yet transfer complete context ownership to the target runtime. | Memory, persisted-session, checkpoint, resume, compaction, and rewind characterization remains green. No fixture is rewritten to accommodate the new structure. |

### Phase 2: Build and prove the pure turn engine

| Boundary | Production responsibility moved | Required gate and temporary compatibility |
|---|---|---|
| `M04` | Introduce the target engine contracts: `RunInput`, `RunContext`, `RunOutcome`, `ModelInvoker`, `ToolExecutor`, `Conversation`, `TurnPolicy`, and `Observer`. Preserve an explicit compatibility path for the current engine while no profile uses the target engine in production. | TDD Red evidence covers the first tool-free turn, provider failure, and ordered observation contracts; Green proves contract behavior without migrating a production profile. |
| `M05` | Implement the target model-turn state machine: tool-free completion, thinking and action phases, streaming and fallback, turn-limit semantics, usage, and provider-error outcomes. | Every applicable `RT` and `ST` scenario passes through both current and target test adapters with one authoritative expectation set. |
| `M06` | Move tool-definition snapshots, calls, parallel-safe batching, exclusive boundaries, large-result handling, correlation, and cancellation behind `ToolExecutor`. | Every applicable `TL` scenario passes through both adapters, including deterministic concurrency barriers, ordering, cancellation, and artifact assertions. |
| `M07` | Move recovery, reminder, TODO, and completion behavior into injected turn policies and eliminate policy state shared across runs. | Every applicable `PL` scenario and related run-isolation scenario passes through both adapters; a later run cannot inherit mutable policy state. |
| `M08` | Enforce the target engine boundary: context preparation and mutation, compaction, session persistence, metrics, tracing, and presentation mapping stay outside the engine; the engine emits only typed outcomes and ordered facts. | The target `internal/engine` depends only on standard-library and approved schema/value packages. All shared runtime characterization scenarios applicable to the engine pass, and the architecture allowlist decreases or remains unchanged. |

The current engine MAY remain temporarily available only to test adapters and unmigrated profile wiring. No new production consumer may be added to it.

### Phase 3: Establish the runtime lifecycle owner

| Boundary | Production responsibility moved | Required gate and temporary compatibility |
|---|---|---|
| `M09` | Define the seven immutable Runtime Profiles, per-run `RunSpec`, capability-ceiling intersection, validation, and snapshot representation. | Every profile snapshot scenario passes, narrowing cannot expand a ceiling, and profile resolution has no entry-adapter dependency. |
| `M10` | Introduce runtime `AgentSession`, `RunScope`, and `SessionStore` collaboration; make runtime the sole owner of recoverable live session state while `internal/session` remains persistence records and storage. | Applicable `CX` and `RS` scenarios plus all persisted fixtures pass through target runtime tests; session and run state cannot leak across identities. |
| `M11` | Introduce the runtime `ContextController` that owns project-instruction, collaboration, session-memory, automemory, skill-list, context-projection, compaction, resume, and rewind injection decisions while calling pure `internal/prompt` renderers. | Every applicable context, compaction, resume, and rewind scenario passes old/new contracts without duplicate fragments or persisted-state drift. |
| `M12` | Introduce `RuntimeHarness` as the single assembly and driving boundary for engine, model, tools, policies, observer, artifacts, persistence, and telemetry. | The complete shared target-runtime suite passes; runtime does not depend on application or presentation adapters; no product profile has yet gained independent engine assembly. |
| `M13` | Introduce runtime `ChildRunner` with frozen parent snapshots, parent and child lineage, depth enforcement, capability intersection, inherited permission ceiling, cancellation, and cleanup. | Every `PF-CHD`, `IA-CHD`, and resolved `DV-CHD` expectation passes against the target runtime; single-level delegation is enforced by both runtime depth and child tool filtering. |

### Phase 4: Cut over Runtime Profiles one at a time

| Boundary | Profile-atomic production cutover | Required gate and obsolete path removal |
|---|---|---|
| `M14` | Migrate `BenchmarkEval` and `cmd/bench` to `RuntimeHarness`, retaining benchmark-owned fixture, validation, aggregation, and reporting control. | All applicable shared, `PF-BEN`, and `EV-BEN` scenarios pass old and target adapters before cutover. The commit removes benchmark's independent engine assembly and leaves one production path. |
| `M15` | Migrate `ChildRun` invocation consumers so `delegate_task` and fork skills adapt to the single runtime `ChildRunner` through consumer-owned `subagent.Runner` contracts. | All applicable shared, `PF-CHD`, and `IA-CHD` scenarios pass. The commit removes every legacy child-engine construction path and preserves one-level delegation. |
| `M16` | Establish `internal/app` as the typed command, DTO, notification, and interaction-port boundary used by user-facing adapters. Retain a temporary old-entry facade only for profiles not yet migrated. | Application contracts contain no runtime implementation, persistence record, provider, tool-registry, Bubble Tea, or transport types. The facade is deleted at `M24`. |
| `M17` | Migrate `CLIExec`: `internal/cli.Run` owns print-mode presentation and `cmd/fox` owns composition and invocation; execution flows through application and runtime. | All applicable shared, `PF-CLI`, and `UI-CLI` scenarios pass old and target adapters before cutover. Exact stdout, stderr, exit, final-message, and extraction-drain behavior passes; the old CLI production path is removed in the same commit. |
| `M18` | Migrate `AutodevPipeline` core-agent factory and per-item execution to runtime while keeping backlog, ledger, stages, gates, worktrees, Engineer supervision, and publication in the Autodev control plane. | All applicable shared, `PF-AUT`, `CP-AUT`, and `UI-AUT` scenarios pass. Direct dependency on `app.AgentRunner` and independent engine assembly are removed in the same commit. |
| `M19` | Migrate `FeishuRemote` typed task execution to application and runtime while retaining webhook, scheduling, outbound messaging, and approval transport in Feishu packages. | All applicable shared, `PF-FEI`, and `UI-FEI` scenarios pass. The old Feishu runtime assembly is removed in the same commit. |
| `M20` | Migrate `AgentOpsTask` to application and runtime while retaining its incident policy, log-search capability, queue, artifacts, and control plane and reusing the already migrated Feishu transport and approval mechanisms. | All applicable shared, `PF-AOP`, and `UI-AOP` scenarios pass. The old AgentOps runtime assembly is removed in the same commit. |
| `M21` | Migrate TUI runtime-facing state: run and session execution, model and effort snapshots, memory, compaction, checkpoint, and rewind flow through application commands and DTOs. | The affected shared, `PF-TUI`, and TUI presentation scenarios pass old and target adapters. The TUI cannot construct or mutate engine, runtime, session-store, model-provider, or tool-registry implementations directly. |
| `M22` | Migrate TUI bidirectional interactions: permission decisions, user questions, Formal Plan review, notifications, cancellation, and queued-input coordination through application ports and typed facts. | Every affected permission, question, plan, queue, cancellation, ordering, and stale-event scenario passes. No Bubble Tea type crosses into application or runtime. |
| `M23` | Complete the `TUIInteractive` entry cutover: `app.RunTUI` is replaced by `tui.Run`, and `cmd/fox` contains only composition, mode selection, and startup. | The complete shared, `PF-TUI`, and `UI-TUI` catalogs pass old and target adapters before cutover. The old TUI production path is removed in the same commit, leaving exactly one TUI production path. |

Every profile cutover `M14` through `M23` MUST obey all of these rules:

1. The current and target test adapters execute the same authoritative applicable scenario suite before the production switch.
2. The commit switches every production construction and invocation path for that profile; partial profile cutovers are forbidden.
3. The commit removes that profile's obsolete production wiring. The old implementation remains reachable only from the differential test adapter until every profile has migrated.
4. The architecture-test allowlist may decrease but cannot gain an exception.
5. A failure can be reverted by reverting that commit without selecting a second production implementation at runtime.

### Phase 5: Remove the old architecture and close the contract

| Boundary | Production responsibility removed or finalized | Required gate |
|---|---|---|
| `M24` | Delete `app.AgentRunner`, old application assembly, `app.RunCLI`, `app.RunTUI`, and every now-unused entry facade. Keep `internal/app` limited to application commands, DTOs, notifications, and interaction ports. | All seven profiles use runtime through their confirmed boundaries; no production or test entry requires an obsolete application facade. |
| `M25` | Delete the old engine implementation, its differential test adapter, parallel reporter chain, old mutable engine configuration, and old cross-run state. | The target engine and runtime own the complete shared suite; `internal/engine` contains only the pure turn-state-machine boundary and approved value dependencies. |
| `M26` | Delete the temporary `internal/context` facade, obsolete session names or aliases, compatibility wrappers, and every duplicate working-memory owner. | Repository-wide symbol and dependency checks find no compatibility consumer; all persisted fixtures and context scenarios remain unchanged and green. |
| `M27` | Finalize the authoritative package-dependency document and Mermaid diagrams, remove the architecture allowlist entirely, and publish the complete compatibility and traceability result for the refactor. | `go test ./...` passes; every mandatory scenario and immutable fixture passes; no forbidden dependency remains; package documentation matches code; all production entries have one path; and no generated worktree artifact is included. |

### Gate applied to every production migration commit

- New or changed behavior contracts MUST follow strict Red-Green-Refactor, and review evidence MUST retain the expected Red failure before implementation.
- Relevant package tests, the shared contract subset, and every affected profile or adapter scenario MUST pass before the commit is complete.
- The architecture allowlist MUST only decrease or remain unchanged, and dependency documentation MUST change in the same commit when the implemented boundary changes.
- Immutable fixtures and expected behavior MUST NOT be edited to make a migration pass. Any discovered discrepancy returns to CON-005 instead of being absorbed into refactoring.
- Each commit MUST compile independently and remain reviewable, bisectable, and revertible. It MUST NOT include production work assigned to the next boundary.
- Adjacent, purely mechanical, low-risk boundaries MAY be combined only when they cross neither a Runtime Profile cutover nor a recoverable-state ownership boundary and retain the same test and review evidence. Profile cutovers and state-owner changes MUST remain independent commits.

## Confirmed Shared Runtime Characterization Catalog

| ID | Required scenario |
|---|---|
| `RT-001` | A single tool-free turn completes with the expected model request, final message, turn count, ordered observer facts, and persisted records. |
| `RT-002` | A tool call, correlated tool result, follow-up model call, and final response occur exactly once and in order. |
| `RT-003` | An assistant response containing both text and tool calls preserves the current ordering and visibility of intermediate text, tool facts, and final text. |
| `RT-004` | With thinking enabled, the thinking request has no tools while the action request has the resolved immutable tool surface. |
| `RT-005` | Non-positive turn limits remain unlimited; positive limits preserve current boundary and partial-result or error behavior without an off-by-one change. |
| `RT-006` | Effort, provider protocol, model snapshot, and resolved tool definitions reach the model invocation unchanged. |
| `RT-007` | Empty responses, nil messages, and ordinary provider errors produce the current run, observer, metrics, and persistence outcomes. |
| `ST-001` | Successful text deltas concatenate to the final message without duplicate presentation or persistence. |
| `ST-002` | Unsupported or empty streaming before the first delta falls back to non-streaming according to current behavior. |
| `ST-003` | A retryable streaming-start error falls back without polluting subsequent requests. |
| `ST-004` | Failure after the first emitted delta does not perform a duplicate fallback call and preserves partial-delta and error ordering. |
| `ST-005` | After streaming is determined unsupported, later model calls in the same run preserve the current no-retry behavior. |
| `ST-006` | A streaming failure does not leak turn-scoped failure state into the next run in the same session. |
| `TL-001` | A successful tool's advertised definition, invocation input, structured result, observer facts, and persisted message agree. |
| `TL-002` | Parallel-safe tools demonstrably overlap under controlled barriers while their committed results retain model-call order. |
| `TL-003` | Mixed parallel-safe and exclusive calls execute as parallel batch, exclusive call, then parallel batch without overlap across the exclusive boundary. |
| `TL-004` | Unknown tools, invalid arguments, business failures, and infrastructure failures each produce a correlated structured failure and current continuation behavior. |
| `TL-005` | Aliases remain consistent across advertisement, invocation, permission assessment, and parallel-safety lookup. |
| `TL-006` | Large results preserve the distinct full artifact, model-visible preview, reporter preview, truncation, and persistence behavior. |
| `TL-007` | Cancellation during a tool batch gives every model-confirmed call a deterministic terminal state and leaves no executing work behind. |
| `TL-008` | Tool results complete before reminders, attachments, or ordinary user messages are inserted into the next model request. |
| `CX-001` | Fresh, existing uncompacted, and resumed sessions each produce the correct initial model-visible context. |
| `CX-002` | Initial-history compaction persists compact state and does not duplicate summaries or messages after reopen. |
| `CX-003` | Pre-turn automatic compaction keeps summary, boundary, observer facts, and the next model request consistent. |
| `CX-004` | Prompt-too-long reactive compaction retries according to the current bounded retry and failure policy. |
| `CX-005` | Compaction failure preserves current original-context fallback and circuit-breaker behavior, including reset after success. |
| `CX-006` | Manual compaction followed by continuation, reopen, and repeated compaction preserves the model-visible conversation. |
| `CX-007` | Rewind before, within, and after compact coverage never reintroduces truncated future content into the model-visible conversation. |
| `CX-008` | Tool-definition token overhead and context blocking decisions use the same resolved request tool snapshot. |
| `PL-001` | Repeated tool failures inject the recovery notice at the current threshold and frequency. |
| `PL-002` | Reminder triggers, cooldown, re-anchoring, and verification-based suppression preserve their current ordering. |
| `PL-003` | The completion gate blocks the first unsatisfied final response, injects its reminder, and preserves the current repeated-unsatisfied terminal behavior. |
| `PL-004` | The TODO gate requires a successful TODO update; a failed update cannot satisfy the gate. |
| `PL-005` | Next-turn reminders retain their current ordering relative to recovery and ordinary reminders. |
| `RS-001` | Successful runs emit one ordered start, turn/model/tool fact sequence, final fact, and completion fact. |
| `RS-002` | Composer, model, tool-result persistence, and message-log persistence failures emit and persist the current error and run-finish outcomes. |
| `RS-003` | Cancellation during model streaming and tool execution fully terminates run-scoped work and observation. |
| `RS-004` | Authoritative session-write failure remains fatal while transcript, metrics, and tracing failures retain their current non-fatal warning behavior. |
| `RS-005` | Runs in one session remain serialized while distinct sessions execute without mutable state, permission, or artifact leakage. |
| `RS-006` | Usage, finish reason, turn count, final message, and artifact paths are calculated and reported exactly once per run. |
| `RS-007` | A runtime without a presentation observer writes no user output directly; observation does not perform adapter-specific formatting. |

## Confirmed TUIInteractive Profile Characterization Catalog

The scenarios below supplement, rather than duplicate, the applicable shared runtime catalog. Each mandatory scenario uses a test-owned temporary home and workspace, scripted providers and interaction responses, deterministic identifiers and clocks, controlled event channels, and temporary copies of immutable baseline session fixtures. It MUST NOT require a real model, external service, ambient user state, or interactive terminal.

| ID | Required scenario |
|---|---|
| `PF-TUI-001` | Resolving `TUIInteractive` produces an immutable, flat snapshot containing the exact session, workspace, turn-limit, capability-ceiling, interaction-port, permission, memory, checkpoint, compaction, and observation policies. A `RunSpec` can select or narrow permitted behavior but cannot expand any ceiling. |
| `PF-TUI-002` | Default launch creates a CLI-source session; explicit-session, continue-latest CLI, and forced-new selection preserve their current lookup and error behavior, including every mutually exclusive flag combination. |
| `PF-TUI-003` | Opening an existing session restores visible messages, display content, tool history, compact and checkpoint state, PLAN/TODO state, and input history without duplicating compact summaries, base instructions, model-switch instructions, or history. Launch-fixed workspace and profile policies remain authoritative. |
| `PF-TUI-004` | Multiple runs retain one session and continuous conversation while receiving distinct run identifiers and run-scoped state. Turn, streaming, policy, tool, permission-request, and artifact state cannot leak between runs or sessions. |
| `PF-TUI-005` | Input submitted during an active run enters the existing FIFO queue without automatically cancelling that run. Success, failure, and explicit cancellation each permit the next item to start in order; collaboration mode is frozen at submission, queued model commands take effect in queue order, and a queued prompt uses the model and effort effective when it is dequeued. Queue dispatch waits while a blocking interaction overlay owns input. |
| `PF-TUI-006` | `/new` and `/clear` create a fresh CLI-source session; clear the visible transcript, queued input, Formal Plan selection, and session permission grants; refresh session paths, sidebar data, and project input history; and preserve workspace, model, effort, selected permission mode, and other process-scoped settings. They remain unavailable during an active run. |
| `PF-TUI-007` | `/model` and `/effort` preserve current validation, persistence, and future-run behavior. A model switch refreshes the provider, model snapshot, context-window estimate, compactor model, skill budget, child-run provider, and resolved tool request for later runs without duplicating prompt fragments; an effort switch exposes only protocol-valid choices, persists per protocol, and cannot mutate an active run's snapshot. |
| `PF-TUI-008` | The default model-visible and callable root surface agrees exactly for `read_file`, `write_file`, `edit_file`, `bash`, `read_todo`, `update_todo`, `delegate_task`, `skill`, and the TUI-enabled `ask_user_question`; aliases, permission assessment, and parallel-safety lookup resolve consistently. |
| `PF-TUI-009` | File-based prompt commands preserve arguments, display prompts, asynchronous preparation, before and after hooks, run-specific effort, conditional skill activation, fork execution, and `allowed-tools`. Restrictions only intersect the profile ceiling, invalid restrictions fail before model invocation, and fork skills still enter the single-level child runtime. |
| `PF-TUI-010` | Only the interactive TUI installs `ask_user_question`. Single choice, multiple choice, free text, multiple ordered questions, submission, dismissal, and context cancellation preserve current results; the originating tool call remains suspended and resumes exactly once after the response. |
| `PF-TUI-011` | Formal Plan mode starts with the exact formal tool phase, routes `submit_plan` to interactive review, keeps planning on revision or cancellation, moves an approved plan to the checklist phase in the same run, and exposes the default implementation surface only after a successful `update_todo`. Transitions occur only at turn boundaries, duplicate submissions remain rejected, and extraction remains gated until approval. |
| `PF-TUI-012` | Ask, Approve for me, and Full Access preserve provider-review start, retry, automatic approval, escalation, allow-once, allow-session, deny, deny-with-feedback, warning, confirm, remember, persistence, and visible-decision behavior. Failed persistence or cancelled confirmation cannot partially activate a mode; selected mode survives a new session while session grants clear on explicit clear, `/new`, and TUI exit; cancellation resolves any pending approval without a stale overlay. |
| `PF-TUI-013` | Every run receives the correct project instructions, collaboration fragment, session memory, automemory context, and skill list. Post-run automemory extraction is run-ID bounded, asynchronous, non-blocking for TUI readiness, and unable to alter the completed outcome on failure or panic. |
| `PF-TUI-014` | User-message state snapshots and file checkpoints produce selectable rewind targets and accurate diffs. Restore-both, conversation-only, code-only, no-op, cancellation, and each failure path preserve current ordering; conversation restore truncates future history, restores input and PLAN/TODO state, and remains consistent before, within, and after compact coverage. |
| `PF-TUI-015` | Automatic compaction uses the shared context contract. `/compact` preserves default and custom instructions, result statistics, insufficient-history and provider failures, active-run rejection, continuation, reopen, and repeated compaction. The full visible transcript remains distinct from the compacted model-visible projection and does not duplicate summaries. |
| `PF-TUI-016` | Run, thinking, compaction, tool-call, tool-result, assistant-delta, assistant-final, error, and completion facts map to TUI state in canonical order. Deltas concatenate once, the final replaces temporary streaming state, final content and persistence are not duplicated, and late deltas after terminal completion are ignored. |
| `PF-TUI-017` | `Esc`, `Ctrl+C`, and `/cancel` propagate through model streaming, tools, questions, plan review, approval, shell, and prompt-command preparation; terminate run-scoped work; close transient interaction state; and return the UI to its current idle or interrupted state. Early user cancellation restores the submitted input only under the current no-meaningful-output, empty-input, and empty-queue guards. A stale event or completion from a prior run cannot reset or terminate a later run, and queued work continues according to current behavior. |
| `PF-TUI-018` | Runtime and application capabilities write no terminal output, ANSI, or Bubble Tea state directly. The TUI remains the sole presentation mapper; runtime logs continue to redirect to the active session's `tui.log` and global logging state is restored on every exit path. |

## Confirmed TUI Presentation Characterization Catalog

| ID | Required scenario group |
|---|---|
| `UI-TUI-001` | Launch routing, default TUI selection, initial-prompt prefill, argument conflicts, help, and error text. |
| `UI-TUI-002` | Text editing, cursor movement, multiline and large paste, input history and drafts, slash completion, and file mentions. |
| `UI-TUI-003` | Built-in commands and aliases, file-based prompt commands, local `!` shell execution, Autodev handoff, help, cancellation, and exit dispatch. |
| `UI-TUI-004` | Transcript and Markdown rendering, tool output and expansion, sidebar documents and focus, scrolling, selection and copying, terminal focus, resizing, and stable ANSI and HTML snapshots. |
| `UI-TUI-005` | Question, plan-review, permission, approval, Full Access warning, effort, and rewind overlays, including keyboard navigation, exclusive input ownership, confirmation, dismissal, and constrained layout. |
| `UI-TUI-006` | Theme and statusline persistence, status and session information, context usage, running and queued notices, quit confirmation, and deterministic restoration after errors or interruption. |

## Confirmed CLIExec Profile Characterization Catalog

The scenarios below use the same hermetic runtime fixture discipline as the TUI profile. Process-level scenarios additionally use injected or captured stdout, stderr, stdin, exit status, and deterministic logging configuration; they MUST NOT depend on a real terminal or process-global state left by another test.

| ID | Required scenario |
|---|---|
| `PF-CLI-001` | Resolving `CLIExec` produces an immutable, flat snapshot containing the exact one-run lifecycle, CLI session source, workspace and model scope, turn budget, capability ceiling, absent interaction ports, memory, compaction, extraction, and observation policies. A `RunSpec` cannot expand any ceiling. |
| `PF-CLI-002` | Default selection creates a CLI-source session; explicit-session, continue-latest CLI, and forced-new selection preserve their current lookup and error behavior, including every mutually exclusive flag combination. |
| `PF-CLI-003` | Each invocation executes exactly one synchronous run with no queue or later user interaction. Workspace, model, effort, legacy thinking, and maximum turns are resolved and frozen for that run, including non-positive unlimited and positive boundary behavior. |
| `PF-CLI-004` | The model-visible and runtime-executable tool surfaces agree exactly for file read, write, and edit, Bash, TODO read and update, skill, and root-level delegation. Definitions, names and aliases, arguments, permission lookup, and parallel-safety lookup derive from the same immutable request snapshot. |
| `PF-CLI-005` | CLI execution installs no user-question port, plan-review port, `submit_plan`, Formal Plan phase, or permission coordinator. It never waits for unavailable user input and preserves the current undecorated-registry execution semantics. |
| `PF-CLI-006` | Model-invocable skills, conditional skill activation, and root delegation remain available; delegation enters the shared single-level child runtime. A slash-prefixed CLI prompt remains ordinary model input and is never routed through the TUI slash-command dispatcher. |
| `PF-CLI-007` | Fresh, existing uncompacted, compacted, and resumed sessions receive the correct project instructions, session memory, automemory context, skill list, and conversation without duplicate summaries, base instructions, or prompt fragments. |
| `PF-CLI-008` | CLI runs create the current file checkpoints and per-message PLAN/TODO state snapshots but expose no user rewind or manual restore entry. Their persisted records remain consumable by a later compatible TUI resume. |
| `PF-CLI-009` | Automatic compaction preserves the selected model, observer and compact-state ordering, retry and fallback behavior, and resumed model-visible projection. CLI exposes no manual `/compact` presentation behavior. |
| `PF-CLI-010` | Post-run automemory extraction is run-ID bounded and tracked. The completed final result and artifact locations become available for user output before the extraction drain finishes; the invocation then waits for all tracked extraction work before return and process exit. Extraction failure or panic cannot alter the run outcome or exit status. |
| `PF-CLI-011` | Internal model streaming still satisfies the shared streaming contract, but CLI presentation emits no model deltas and prints the completed final message at most once. Empty or failed runs cannot reuse a stale final message from resumed history. |
| `PF-CLI-012` | A successful runtime result contains the exact session ID, run ID, final message, transcript path, metrics path, trace path, usage, finish reason, and turn count, each calculated, persisted, and reported once. |
| `PF-CLI-013` | Provider, tool, persistence, turn-limit, and cancellation failures preserve the current partial-result, artifact, persistence, and error behavior. Runtime cleanup completes, no run-scoped work survives process completion, and an error cannot be converted into a successful outcome by presentation formatting. |
| `PF-CLI-014` | Runtime and application capabilities write no CLI labels, stdout, stderr, ANSI, or TUI state directly. The dedicated `internal/cli` adapter is the sole owner of human-readable print-mode formatting and remains independent of TUI and Bubble Tea. |

## Confirmed CLI Presentation Characterization Catalog

| ID | Required scenario group |
|---|---|
| `UI-CLI-001` | `fox exec`, `fox -p`, and `fox -print` routing; positional prompt, `-prompt`, absent prompt, and literal `-` stdin acquisition; whitespace handling; non-empty explicit prompts not consuming stdin; and every prompt and mode flag conflict. |
| `UI-CLI-002` | Provider, model, effort, and legacy thinking resolution across settings, environment, and CLI precedence; protocol-specific validation; first-run onboarding; and the current error precedence between configuration resolution and prompt validation. |
| `UI-CLI-003` | Exact successful stdout bytes and ordering for the optional final message, separating blank line, Session, Transcript, Run, Metrics, and Trace labels and values; separate deterministic stderr lifecycle logging; no model-delta output; and result output occurring before extraction drain completion. |
| `UI-CLI-004` | Exact failure behavior for initialization errors, nil outcomes, partial outcomes, runtime errors, and extraction failures, including which result and artifact lines remain visible, stderr error ordering, and process exit status `1` without false success output. |

## Confirmed FeishuRemote Profile Characterization Catalog

The scenarios below use scripted providers and approval responses, fake messengers, deterministic clocks and identifiers, controlled concurrency barriers, test-owned temporary session storage, immutable fixture copies, and local test HTTP servers. They MUST NOT contact a real model or Feishu service, use external credentials, bind a fixed port, rely on wall-clock sleeps or uncontrolled scheduling, or retain process-global state after a test.

| ID | Required scenario |
|---|---|
| `PF-FEI-001` | Resolving `FeishuRemote` produces an immutable, flat snapshot containing the exact Feishu session source, process-fixed workspace and provider, 20-turn budget, five-minute task timeout, global concurrency limit of four, capability ceiling, interaction ports, permission, memory, compaction, extraction, and observation policies. A `RunSpec` cannot expand any ceiling. |
| `PF-FEI-002` | Typed task identity freezes the task, chat, sender, and message identifiers plus trimmed task text. The model receives the exact existing Feishu source envelope containing sender and message identity, while `/new` remains a remote session directive rather than a TUI slash command. |
| `PF-FEI-003` | Ordinary tasks reuse the latest Feishu-source session for the exact chat and sender key. `/new`, `新会话`, each directive with a following prompt, directive-only text, surrounding whitespace, and similar non-directive text preserve their frozen post-verification creation, prompt, lookup, and error behavior. |
| `PF-FEI-004` | Consecutive tasks in one selected remote session retain continuous conversation and compatible persisted records while receiving distinct task and run identities. Different chat, sender, and forced-new sessions cannot leak messages, permission state, run state, or artifacts. |
| `PF-FEI-005` | Same-session exclusion, distinct-session concurrency, the global limit of four, fifth-task waiting, timeout, parent cancellation, task-channel closure, and shutdown are verified under deterministic barriers. FIFO, fairness, lock-wait cancellation, and drain expectations use the baseline frozen after `DV-FEI-004` through `DV-FEI-006`. |
| `PF-FEI-006` | The model-visible and runtime-executable tool surfaces agree exactly for file read, write, and edit, Bash, TODO read and update, and root-level delegation. Names, aliases, arguments, permission assessment, and parallel-safety lookup derive from the same immutable snapshot, and remote permission decoration preserves the frozen non-parallel execution behavior. |
| `PF-FEI-007` | The profile exposes no skill, user-question, Formal Plan, `submit_plan`, checkpoint, rewind, manual compaction, thinking, or model-delta capability and never waits for an unavailable interaction port. |
| `PF-FEI-008` | `ModeAsk` preserves safe fast-path, hard policy rejection, reviewable escalation, allow-once, deny, deny-with-feedback, approval-send failure, timeout, and cancellation behavior without creating TUI permission modes, Full Access state, or persistent session grants. |
| `PF-FEI-009` | Each approval is correlated to its task, session, run, and tool call. Its evidence contains compatible prior session records and the current task at most once, and its tool, action, risk, arguments, reviewer failure, and rationale cannot cross session boundaries. |
| `PF-FEI-010` | Root delegation enters the shared single-level child runtime, inherits the parent workspace, provider, permission coordinator, and evidence security ceiling, returns one child report, and cannot create a depth-two child. |
| `PF-FEI-011` | Fresh, continued, compacted, and reopened sessions receive the correct project instructions, session memory, automemory context, and model-visible conversation without skill, collaboration, or TUI fragments and without duplicate base instructions or compact summaries. No checkpoint or rewind state is created. |
| `PF-FEI-012` | Automatic compaction preserves its current trigger, summary, boundary, observer, failure fallback, circuit-breaker recovery, persisted compact state, and reopen projection. Model consistency uses the baseline frozen after `DV-FEI-008`. |
| `PF-FEI-013` | Every non-nil run result triggers run-ID-bounded fire-and-forget automemory extraction with the run's tracker. Extraction does not delay the terminal task result, and extraction failure or panic cannot alter it; process-shutdown behavior uses the `DV-FEI-006` baseline. |
| `PF-FEI-014` | Runtime produces each run, compaction, tool, final, error, and completion fact once in canonical order with task, session, run, and tool-call correlation. A late, duplicate, or stale fact from another task, session, or completed run cannot be mapped to the current task. Feishu text remains adapter-owned. |
| `PF-FEI-015` | Feishu performs no thinking request, streaming provider invocation, or model-delta observation. A final message is derived only from the current run and cannot reuse resumed history or another task's completion. |
| `PF-FEI-016` | Successful, empty-final, turn-limit, provider, tool, persistence, compaction, and partial-result paths preserve the frozen session, run, artifact, extraction, and terminal-outcome behavior without exposing CLI artifact labels to the user. |
| `PF-FEI-017` | Parent cancellation, timeout, permission cancellation, tool cancellation, runner shutdown, and task panic reach one terminal state and leave no task, tool, approval wait, session lock, or run-scoped work behind. Shutdown and panic outcomes use the baselines frozen after `DV-FEI-006`, `DV-FEI-007`, and `DV-FEI-009`. |
| `PF-FEI-018` | Runtime and application capabilities depend on no Lark SDK type, listen on no HTTP port, and generate no Chinese Feishu message. They exchange only UI-neutral typed commands, notifications, outcomes, and interaction-port values. |

## Confirmed Feishu Transport Adapter Characterization Catalog

`UI-FEI` denotes a user-facing presentation and transport adapter, not a graphical interface.

| ID | Required scenario group |
|---|---|
| `UI-FEI-001` | The four required environment variables, provider resolution from user settings, current working directory, task-channel capacity 32, `:7777` launch, initialization error ordering, runner startup, process cancellation, and final exit behavior, with shutdown expectations frozen after `DV-FEI-006`. |
| `UI-FEI-002` | Gateway-owned mux, exposed paths and methods, verification-token and encryption-key validation, Feishu challenge handling, fixed read-header, read, write, and idle timeouts, listen errors, and controlled repeated shutdown before and after server start. |
| `UI-FEI-003` | Valid, nil, malformed, encrypted, unknown-event, unsupported-message, missing event/message/chat/message-ID, missing-sender, JSON-text, raw-text, and whitespace-only payloads; deterministic task-ID encoding; source-field correlation; exact acknowledgement or ignore behavior; and no task creation on rejected input. Missing-sender expectations use the `DV-FEI-003` baseline. |
| `UI-FEI-004` | Task-channel delivery, full-channel backpressure, request cancellation, and sequential, concurrent, post-completion, and post-restart duplicate `message_id` delivery. Idempotency, acknowledgement, and persisted deduplication expectations use the baseline frozen after `DV-FEI-002`. |
| `UI-FEI-005` | Exact target chat, text, and ordering for receipt, new or continued session, run start, compaction, tool success and failure, final assistant content, empty-final completion, success status, failure status, timeout, cancellation, and panic terminal outcomes without cross-task message attribution. |
| `UI-FEI-006` | Unicode truncation boundaries and suffixes; tool argument, tool result, and final-message limits; empty-message suppression; Lark `chat_id` text request construction; SDK and API errors; logs; and delivery-failure effects on task execution and terminal outcome, using the baseline frozen after `DV-FEI-010`. |
| `UI-FEI-007` | Approval request text and identity, externally reachable authenticated callback, allow, deny, feedback, send failure, timeout, cancellation, turn termination, disconnect, unknown ID, late response, duplicate response, and concurrent response. Every request reaches one terminal state and its pending identity is removed exactly once according to `DV-FEI-001` and `DV-FEI-007`. |

## Confirmed AgentOpsTask Profile Characterization Catalog

The scenarios below use scripted providers and approval responses, fake messengers, deterministic clocks and identifiers, controlled concurrency barriers, test-owned temporary session and log directories, and immutable fixture copies. They MUST NOT contact a real model or Feishu service, use external credentials, bind a fixed port, rely on wall-clock sleeps or uncontrolled scheduling, or retain process-global state after a test.

| ID | Required scenario |
|---|---|
| `PF-AOP-001` | Resolving `AgentOpsTask` produces an immutable, flat snapshot containing the Feishu persisted source, process-fixed workspace, provider, and log directory, 24-turn budget, five-minute task timeout, global concurrency limit of four, capability ceiling, interaction ports, permission, memory, compaction, extraction, and observation policies. A `RunSpec` cannot expand any ceiling. |
| `PF-AOP-002` | Typed task identity freezes the task, chat, sender, and message identifiers plus task text. Missing and empty message-ID validity uses the baseline frozen after `DV-AOP-001`. The model receives the exact frozen incident-analysis prompt, including its existing Chinese role text, six investigation rules, requested final structure, and task interpolation, without a Planner prepass or transport-specific envelope drift. |
| `PF-AOP-003` | Every accepted task creates a fresh session with the Feishu persisted source, process-fixed workspace, sender and chat metadata, and distinct session and run identities. It never resumes a prior task session and never interprets `/new`, `new session`, or similar task text as a session-control directive. |
| `PF-AOP-004` | The global concurrency limit of four, fifth-task waiting, distinct fresh-session isolation, five-minute timeout, parent cancellation, task-channel closure, and shutdown are verified under deterministic barriers. Intake, accepted-queue, in-flight drain or cancellation, and terminal expectations use the baseline frozen after `DV-AOP-002`. |
| `PF-AOP-005` | Session creation attempts the exact existing session-created notification before session memory files are ensured. Session creation, initial-notice delivery, and memory initialization failures preserve their frozen ordering, side effects, and terminal behavior, with delivery expectations frozen after `DV-AOP-005`. |
| `PF-AOP-006` | The model-visible and runtime-executable tool surfaces agree exactly for `log_search`, file read, write, and edit, Bash, TODO read and update, and root-level delegation. Names, aliases, definitions, arguments, permission assessment, and parallel-safety lookup derive from the same immutable snapshot, and permission decoration preserves the frozen non-parallel execution behavior. |
| `PF-AOP-007` | `log_search` requires `service` and `query`; preserves the optional `limit` schema, default 50, maximum 200, and current out-of-range fallback; searches case-insensitively in line order; stops after the limit; preserves the exact no-match result; enforces the one-MiB-per-line scanner ceiling; and returns the frozen open, read, cancellation, and malformed-input errors. |
| `PF-AOP-008` | `log_search` remains a read-only observation capability scoped to the configured log root. Valid and invalid service names, permission assessments, direct traversal, separators, symlinks and the final resolved target actually opened, cancellation, and deterministic resource bounds use the security baseline frozen after `DV-AOP-006`. |
| `PF-AOP-009` | The profile exposes no skill, user-question, Formal Plan, `submit_plan`, checkpoint, rewind, manual compaction, thinking, model-delta, or other TUI-only capability and never waits for an unavailable interaction port. |
| `PF-AOP-010` | `ModeAsk` preserves safe fast-path, hard policy rejection, reviewable escalation, allow-once, deny, deny-with-feedback, approval-send failure, timeout, and cancellation behavior without creating TUI permission modes, Full Access state, or persistent session grants. |
| `PF-AOP-011` | Each approval is correlated to its task, fresh session, run, and tool call. Its evidence contains compatible records and the current incident prompt at most once, and its tool, action, risk, arguments, reviewer failure, and rationale cannot cross task boundaries. Callback reachability and exactly-once pending-state cleanup use the shared `DV-FEI-001` and `DV-FEI-007` baselines rather than a second AgentOps approval protocol. |
| `PF-AOP-012` | Root delegation enters the shared single-level child runtime, inherits the parent workspace, provider, permission coordinator, and evidence security ceiling, returns one child report, and cannot create a depth-two child. |
| `PF-AOP-013` | Each fresh session receives the correct project instructions, session memory, automemory context, and model-visible conversation without skill, collaboration, TUI, prior-task, checkpoint, or rewind fragments and without duplicate base instructions. Only automemory is shared across task sessions. |
| `PF-AOP-014` | Automatic compaction preserves its current trigger, summary, boundary, observer, failure fallback, circuit-breaker recovery, and persisted compact state within the fresh task session. Model consistency uses the baseline frozen after `DV-AOP-004`. |
| `PF-AOP-015` | Every non-nil run result triggers run-ID-bounded fire-and-forget automemory extraction with the run's tracker, including a non-nil partial result returned with an error. Extraction does not delay or alter the terminal task result, and extraction failure or panic cannot change it; process-shutdown behavior uses the `DV-AOP-002` baseline. |
| `PF-AOP-016` | Runtime produces each run, compaction, tool, final, error, and completion fact once in canonical order with task, session, run, and tool-call correlation. The AgentOps adapter observes only the current session notice and terminal outcome, not thinking, tool-progress, or model-delta presentation, and a late, duplicate, or stale fact from another task or completed run cannot be attributed to the current task. |
| `PF-AOP-017` | Success with a final message, success with nil or empty results, turn-limit, provider, tool, persistence, compaction, partial-result, and delivery-error paths preserve the frozen final content and session, run, trace, and metrics artifacts. AgentOps exposes no transcript label, and each artifact value comes from the result or its existing session fallback exactly once. |
| `PF-AOP-018` | Parent cancellation, timeout, permission cancellation, tool cancellation, runner shutdown, task panic, and terminal-delivery failure reach at most one terminal outcome correlated to the originating task and leave no task, tool, approval wait, extraction launch, or run-scoped work behind. Missing, duplicate, or late terminal behavior and cleanup use the baselines frozen after `DV-AOP-002`, `DV-AOP-003`, and `DV-AOP-005`. |
| `PF-AOP-019` | `internal/agentops` owns only the incident-analysis task policy, exact incident prompt, `log_search` capability, and runtime-control adaptation. Runtime and application capabilities depend on no Lark SDK type, listen on no HTTP port, own no process task channel or deduper, and send no direct user output. |

## Confirmed AgentOps Transport Adapter Characterization Catalog

`UI-AOP` denotes the AgentOps process and Feishu transport adapter, not a graphical interface.

| ID | Required scenario group |
|---|---|
| `UI-AOP-001` | The four required Feishu environment variables plus `AGENTOPS_WORKDIR` and `AGENTOPS_LOGDIR`; provider resolution from user settings; two task channels of capacity 64; `:7777` launch; messenger, approval-store, gateway, deduper, bridge, and runner initialization order; configuration failures; startup failures; and final exit behavior. |
| `UI-AOP-002` | The shared authenticated Feishu gateway protocol and event validation, including missing and empty message IDs, map each accepted event's task, chat, sender, message, and text fields unchanged into exactly one typed AgentOps task. Rejected or unsupported input creates no task, and empty-ID expectations use the baseline frozen after `DV-AOP-001`. |
| `UI-AOP-003` | Sequential, concurrent, exact-TTL-boundary, expired, post-completion, and post-restart duplicate message delivery; duplicate acknowledgement and terminal lifecycle; bridge backpressure and delivery failure; and task failure and completion. Acceptance timing, durable idempotency, and restart behavior use the baseline frozen after `DV-AOP-001`. |
| `UI-AOP-004` | Exact target chat, text, and ordering for the session-created notice, final assistant content, empty-final default, Session, Run, Trace, and Metrics lines, and failure notification. The initial notice precedes the one correlated terminal outcome; normal AgentOps execution sends no Feishu receipt, thinking, tool-progress, or model-delta message; and stale output cannot cross task boundaries. |
| `UI-AOP-005` | Initial-notice, final-message, and fallback-failure delivery errors; long and transport-rejected final content; Lark `chat_id` text request construction; SDK and API errors; retry or second-failure behavior; logs; and whether completed work or side effects can lack a delivered terminal outcome, using the baseline frozen after `DV-AOP-005`. |
| `UI-AOP-006` | The shared approval callback preserves request identity and exactly-once terminal cleanup according to `DV-FEI-001` and `DV-FEI-007`. Process shutdown stops new gateway intake and coordinates the bridge, both task queues, queued work, and in-flight tasks according to `DV-AOP-002`, with every accepted task either drained or cancelled into the frozen correlated terminal outcome and no late task starting after close. |

## Confirmed BenchmarkEval Profile Characterization Catalog

The scenarios below use scripted providers, deterministic clocks and identifiers, controlled execution barriers, test-owned temporary homes and workspaces, and immutable fixture copies. Local validation commands MAY run repository-declared toolchain programs against test-owned files, but the tests MUST NOT contact a real model or external service, use ambient credentials or user settings, depend on wall-clock sleeps or uncontrolled scheduling, or leave background processes or process-global state after completion.

| ID | Required scenario |
|---|---|
| `PF-BEN-001` | Resolving `BenchmarkEval` produces an immutable, flat snapshot containing the CLI persisted source, fresh fixture-workspace and session policies, provider and model scope, case-controlled turn budget with default 12, serial repeat scheduling, capability ceiling, interaction ports, permission, memory, compaction, observation, evaluation, and artifact policies. A `RunSpec` cannot expand any ceiling. |
| `PF-BEN-002` | Typed evaluation input freezes the case ID, optional name, prompt, fixture identity, turn budget, ordered validations, and repeat identity for one execution. The exact case prompt reaches the model without a Planner prepass, display-prompt rewrite, thinking phase, or presentation command interpretation; questionable input-domain expectations use `DV-BEN-005`. |
| `PF-BEN-003` | Every repeat materializes a fresh fixture-copy temporary workspace and creates a fresh CLI-source session and run. It never resumes a prior benchmark session, and source-fixture files, prior workspace mutations, session messages, TODO state, compaction state, artifacts, and identifiers cannot leak between repeats. |
| `PF-BEN-004` | Repeats execute strictly serially in requested order. Provider request state, engine turn state, compactor state, reminder and recovery state, TODO state, telemetry, runtime failures, and validation failures from one repeat cannot alter another repeat except through the ordered aggregate result list. |
| `PF-BEN-005` | Provider protocol, model, and credentials resolve through the existing benchmark user-settings and scoped-environment precedence without a TUI or per-run interactive override. One immutable protocol and model snapshot reaches model invocation, context-window decisions, telemetry, runtime fidelity, and the compactor. |
| `PF-BEN-006` | The model-visible and runtime-executable tool surfaces agree exactly for file read, write, and edit, Bash, TODO read, and TODO update. Names, aliases, definitions, arguments, structured failures, large-result handling, and parallel-safety lookup derive from the same immutable snapshot; read-only calls can exercise the applicable parallel batches while write and shell calls preserve exclusive boundaries. |
| `PF-BEN-007` | The profile exposes no delegation, model-invocable skill tool or skill list, user-question, Formal Plan, `submit_plan`, permission or approval coordinator, checkpoint, rewind, manual compaction, thinking, automemory, automemory extraction, model-delta observer, or another presentation interaction and never waits for an unavailable port. |
| `PF-BEN-008` | Each model-visible prompt contains the correct base prompt, fixture-local project instructions, session plan and TODO guidance, and session working memory without automemory, collaboration, interactive-question, or presentation fragments. A direct `$skill` reference preserves the current fixture-local prompt-fragment loading behavior without advertising or enabling a skill tool. |
| `PF-BEN-009` | Each repeat initializes and uses only its fresh session working-memory files. Reads, writes, missing files, initialization errors, TODO state, and persisted session paths remain repeat-local; no cross-session automemory store is read, written, or extracted. |
| `PF-BEN-010` | Automatic compaction uses the benchmark model and preserves its trigger, summary, boundary, observer facts, prompt-too-long retry, failure fallback, circuit-breaker recovery, tool-definition overhead, persisted compact state, and next model request within the fresh repeat session. |
| `PF-BEN-011` | Repeated tool-failure recovery, ordinary reminders, cooldown and re-anchoring, and the TODO completion gate preserve their current thresholds and ordering. The profile injects no general completion gate or next-turn-reminder source, and a failed TODO update cannot satisfy the TODO gate. |
| `PF-BEN-012` | Runtime produces UI-neutral run, model, compaction, tool, final, error, completion, artifact, and telemetry outcomes in canonical order and writes no benchmark summary, PASS/FAIL label, JSON report, or other direct user output. Evaluation-specific formatting and verdict aggregation remain outside runtime. |
| `PF-BEN-013` | Runtime execution reaches its terminal outcome before ordered validations begin. A runtime error or non-nil partial result does not suppress validations, every configured validation receives the resulting workspace, and overall evaluation success requires both a successful runtime terminal state and every validation passing. Runtime completion and validation verdict remain separately inspectable. |
| `PF-BEN-014` | Each result correlates case, repeat, workspace, session, run, runtime terminal state, runtime error or partial outcome, ordered validation verdicts, duration, artifacts, resolved profile and model provenance, case-definition identity, fixture identity, shared invariants, and intentional differences without conflating completed, failed, or aborted runtime state with validation success. `DurationMS` preserves the current Agent-run-only scope, while golden comparisons normalize rather than freeze volatile duration, workspace, session, and run values. Exact corrected provenance and schema expectations use `DV-BEN-003` and `DV-BEN-007`. |
| `PF-BEN-015` | Parent cancellation, turn limit, whole-case timeout, provider, tool, persistence, compaction, validation timeout, and validator-output overflow terminate the applicable runtime and evaluation work, reach one correlated terminal state, and leave no Agent, tool, validator shell, child process, or other run-scoped work behind. Lifetime and resource expectations use the baselines frozen after `DV-BEN-001` and `DV-BEN-006`. |
| `PF-BEN-016` | `internal/benchmark` is a runtime control client that owns case, fixture, validation, aggregation, provenance, and report-domain behavior and may directly use privileged runtime-harness APIs. It MUST NOT depend on `app`, TUI, CLI presentation adapters, concrete engine construction, or an independently assembled runtime path, and runtime MUST NOT import benchmark. |

## Confirmed Benchmark Evaluation Control-Plane Characterization Catalog

`EV-BEN` denotes benchmark evaluation orchestration and reporting, not a presentation or user-interaction adapter.

| ID | Required scenario group |
|---|---|
| `EV-BEN-001` | `-case`, `-out`, and `-repeat` parsing; required case path; default output `benchmark-result.json`; default repeat one; unexpected positional input; repeated flags; help and parse failures; and zero, negative, and overflow values, with the accepted input domain frozen after `DV-BEN-005`. |
| `EV-BEN-002` | Case-file open and YAML parse errors; required non-empty ID, fixture, prompt, and validation list; optional name; zero `max_turns` defaulting to 12; negative and positive turn budgets; unknown and duplicate YAML fields; validation type and field combinations; multiline prompts; relative fixture resolution; and exact error precedence, with invalid-field expectations frozen after `DV-BEN-005`. |
| `EV-BEN-003` | Fixture directories, nested files, empty files, content, normalized file and directory modes, source open and destination write failures, partial copies, file and directory symlinks, traversal, and final resolved targets. Every repeat receives a distinct `foxharness-benchmark-*` workspace, the immutable source fixture is never modified, and successful retention plus failure cleanup use the baseline frozen after `DV-BEN-004`; stable fixture identity uses `DV-BEN-007`. |
| `EV-BEN-004` | Workspace creation, fixture materialization, runtime-harness factory, session and memory initialization, provider resolution, tool snapshot, and compactor construction occur in the frozen order. Factory error, nil harness, missing runtime or session, partial setup, parent cancellation, and panic preserve their result, cleanup, and reporting behavior. |
| `EV-BEN-005` | A command validation runs via `bash -c` in the repeat workspace, captures stdout and stderr under a deterministic bound, preserves exit status and error text, receives a two-minute child timeout and parent cancellation, and terminates and reaps its complete process tree. Empty commands, output overflow, truncation, timeout, cancellation, spawn failure, non-zero exit, and following-validation behavior use `DV-BEN-005` and `DV-BEN-006`. |
| `EV-BEN-006` | A `file_contains` validation preserves success, missing-file, read-error, empty-content, empty-needle, match, mismatch, Unicode, relative, absolute, traversal, and symlink behavior. Unknown validation types produce one ordered failed result. Path-containment and vacuous-input expectations use `DV-BEN-004` and `DV-BEN-005`. |
| `EV-BEN-007` | Validations execute in YAML order and produce exactly one result per entry without short-circuiting after an earlier validation failure or runtime error. Overall success is true only when runtime execution succeeds and every validation passes; runtime error text and validation messages remain separately available. |
| `EV-BEN-008` | Requested repeats execute and append results strictly in order with fresh correlated repeat, workspace, session, and run identities. Mixed pass and fail results remain distinct, a prior repeat cannot overwrite a later result, and non-positive repeat plus mid-sequence infrastructure-failure behavior uses `DV-BEN-002`, `DV-BEN-005`, and `DV-BEN-007`. |
| `EV-BEN-009` | Human summary output preserves its title, passed and total counts, ordered PASS or FAIL line, case ID, Agent-run duration, session, and workspace formatting. Runtime terminal state, validation verdict, fidelity differences, provenance visibility, mixed outcomes, zero results, and volatile-field normalization use the baselines frozen after `DV-BEN-002`, `DV-BEN-003`, `DV-BEN-005`, and `DV-BEN-007`. |
| `EV-BEN-010` | JSON reporting preserves the corrected schema, deterministic field and result ordering, indentation, trailing newline, error and warning omission rules, actual volatile values, bounded validation messages, overwrite mode, target permissions, serialization failure, open and write errors, and complete runtime-fidelity and provenance data. Golden expectations normalize volatile values without changing the real report, and affected behavior uses `DV-BEN-003`, `DV-BEN-006`, and `DV-BEN-007`. |
| `EV-BEN-011` | Case load, fixture setup, harness setup, runtime terminal state, validation verdict, repeat aggregation, human summary, JSON write, log output, and process exit status preserve one explicit failure precedence. A failed, cancelled, timed-out, or partially completed evaluation cannot be reported as process success after the baseline is frozen under `DV-BEN-001`, `DV-BEN-002`, and `DV-BEN-006`. |

## Confirmed ChildRun Profile Characterization Catalog

The scenarios below exercise a synchronous child through scripted providers, deterministic identifiers, test-owned workspaces and homes, controlled execution barriers, fake permission coordinators, and locally spawned test process trees. They MUST NOT contact a real model or external service, use ambient credentials or user state, depend on wall-clock sleeps or uncontrolled scheduling, or leave child work after completion.

| ID | Required scenario |
|---|---|
| `PF-CHD-001` | Resolving `ChildRun` produces an immutable, flat snapshot containing depth one, a default and maximum 200-turn budget, non-streaming model invocation, disabled thinking, a child capability ceiling, permission policy, fresh-session policy, memory, compaction, observation, and outcome policy. No `RunSpec` or invocation adapter can expand a ceiling. |
| `PF-CHD-002` | A typed child `RunSpec` freezes the task, parent session and run identity, delegation or tool-call identity, depth, read-only mode, allowed-tool intersection, provider and model, workspace, and cancellation lineage before execution. Later caller mutation cannot change an active child. |
| `PF-CHD-003` | Every invocation creates a fresh persisted subagent-source session and run, preserves the current derived user identity `subagent-of-<parent>`, and never resumes or mutates the parent's authoritative conversation. Child messages, compact state, TODO state, working memory, identifiers, and terminal outcome cannot leak across invocations. |
| `PF-CHD-004` | One accepted invocation starts exactly one synchronous child run. The parent waits for its terminal outcome and receives no background handle, queue, resume operation, send-input operation, or later message protocol. Cancellation while waiting reaches the same child. |
| `PF-CHD-005` | Workspace, provider protocol, model, credentials, and applicable project configuration derive from one frozen parent runtime snapshot. A parent model or configuration change after child start cannot alter model invocation, compaction, telemetry, or tool execution for that child. |
| `PF-CHD-006` | The child defaults to and cannot exceed 200 turns. Exact-boundary completion, exhaustion, configured narrowing, zero and invalid values, cancellation precedence, persisted terminal state, and any correlated partial result are deterministic; an invocation cannot request a larger budget. |
| `PF-CHD-007` | Child model invocation performs no thinking request and emits no streaming model deltas. Applicable repeated-tool-failure recovery, reminders, cooldown, and re-anchoring preserve the shared runtime behavior and remain isolated from the parent and sibling children. |
| `PF-CHD-008` | A read-only child starts from `read_file` and Bash while a writable child additionally receives file write and edit. Any caller allowlist intersects with, rather than expands, that profile ceiling; names, aliases, schemas, ordering, and structured unavailable-tool failures remain stable. Exact read-only Bash semantics are frozen after `DV-CHD-001`. |
| `PF-CHD-009` | Model-visible definitions, runtime-executable calls, permission assessment, alias resolution, and parallel-safety lookup derive from the same immutable child tool snapshot. A tool omitted by profile, read-only mode, or caller allowlist cannot be reached through another registry or stale prompt fragment. |
| `PF-CHD-010` | The child exposes no TODO tools, model-invocable skill tool or skill list, user-question tool, Formal Plan or `submit_plan`, checkpoint, rewind, presentation interaction, or delegation capability, including `delegate_task`, fork-child helpers, and internal equivalents. |
| `PF-CHD-011` | A runtime depth gate rejects every attempt to create a depth-two child, independent of prompt compliance and model-visible tool filtering. Delegation-tool calls, fork-skill paths, aliases, and direct internal child-run requests cannot bypass the one-level ceiling or create child capacity before rejection. |
| `PF-CHD-012` | Read-only and writable modes preserve their exact file, shell, risk, approval, allowlist, denial, and side-effect semantics after the affected baseline is frozen under `DV-CHD-001`. Permission decisions cannot grant a tool or operation outside the immutable capability ceiling. |
| `PF-CHD-013` | Child permission assessment uses the parent coordinator with a child-specific request source and frozen child metadata. Compatible parent grants MAY be reused only under the existing policy; incompatible root-session grants, unrelated remembered evidence, or a weaker child policy cannot authorize a child operation. Missing required coordination fails closed. |
| `PF-CHD-014` | Permission evidence distinguishes the trusted parent invocation from untrusted model-produced child arguments and correlates parent session, parent run, child session, child run, delegation identity, tool call, risk, and terminal decision. Evidence cannot satisfy another child, run, tool call, or capability mode. |
| `PF-CHD-015` | The child system prompt preserves applicable base and project instructions, explicit parent-session lineage, the exact delegated task, concise child-execution guidance, and a high-density response requirement. It describes only the effective tool and permission surface and contains no parent UI, collaboration, TODO, unsupported write, or nested-delegation promise; affected text is frozen after `DV-CHD-003`. |
| `PF-CHD-016` | Child working memory is initialized and isolated per fresh session under the child policy. Automemory, when supplied from the parent runtime, is shared read-only: the child can consume the frozen projection but cannot write, extract, schedule extraction, or mutate parent or sibling memory state. |
| `PF-CHD-017` | A direct `$skill` task reference preserves the current project-local prompt-fragment loading behavior. The child receives no model-invocable skill tool, skill inventory, conditional activation protocol, or capability expansion through skill content. |
| `PF-CHD-018` | Automatic compaction uses the frozen child model, child conversation, and independent compact state and preserves trigger, summary, boundary, observer facts, prompt-too-long retry, failure fallback, circuit-breaker recovery, persistence ordering, and next request. It cannot compact or overwrite the parent conversation; affected model expectations are frozen after `DV-CHD-002`. |
| `PF-CHD-019` | Successful execution returns only the current child session identity and report through the typed runtime outcome. The child writes no stdout, TUI state, Feishu message, benchmark verdict, or other presentation or transport output and cannot reuse a stale report from another invocation. |
| `PF-CHD-020` | Session creation, provider, tool, permission, persistence, compaction, turn-limit, and cancellation failures produce one correlated terminal child outcome without a stale report. Any permitted partial result, including child session identity and report content, is frozen after `DV-CHD-006` and remains distinguishable from success. |
| `PF-CHD-021` | Parent cancellation, child cancellation, turn exhaustion, provider failure, tool failure, panic, and return terminate and reap all child-scoped model work, shell descendants, pending permissions, compactor work, observers, and run resources. No late event, side effect, report, or permission response can alter the terminal parent-visible outcome; process-tree behavior is frozen after `DV-CHD-004`. |
| `PF-CHD-022` | `runtime.ChildRunner` owns child profile resolution, session and run lifecycle, engine construction and driving, cancellation, compaction, permission and tool snapshots, and typed outcomes. `subagent` owns only model-facing protocol and report adaptation and MUST NOT assemble an engine or depend on application, TUI, CLI, Feishu, AgentOps, or benchmark presentation code. |

## Confirmed Child Invocation Adapter Characterization Catalog

`IA-CHD` denotes model-facing or command-facing adaptation into `runtime.ChildRunner`; it is neither a presentation adapter nor an independently assembled runtime.

| ID | Required scenario group |
|---|---|
| `IA-CHD-001` | The `delegate_task` tool preserves its name, description, and schema: required string `task`, optional boolean `read_only`, rejection of missing or malformed fields, and no undocumented argument that can expand model, depth, turn, tool, permission, or workspace ceilings. |
| `IA-CHD-002` | Delegate argument handling preserves JSON decoding, whitespace trimming, empty-task rejection, `read_only` defaulting to true, and consistency between permission assessment and execution. The exact normalized request assessed is the request passed to the runtime. |
| `IA-CHD-003` | Delegate assessment enforces the one-level nesting gate, read-only or writable risk classification, immutable tool ceiling, and fail-closed behavior when required permission coordination is unavailable. Assessment cannot approve a request that execution would broaden or reinterpret. |
| `IA-CHD-004` | One valid delegate call invokes `ChildRunner` exactly once and converts the correlated runtime outcome into the compatible `Subagent Session` and `Report` tool-result text. Errors, cancellation, and any post-`DV-CHD-006` partial outcome remain structured and cannot reuse another call's session or report. |
| `IA-CHD-005` | The fork-skill adapter passes the processed task, live parent runtime snapshot, provider and model, selected agent, allowed-tool ceiling, depth and cancellation lineage to `ChildRunner`, then returns the current child report. Whether and how the selected agent affects behavior is frozen only after `DV-CHD-005`. |
| `IA-CHD-006` | `delegate_task` and fork-skill entry paths share the same runtime depth gate, cancellation lineage, permission ceiling, tool intersection, fresh-session policy, terminal correlation, and cleanup. Neither adapter renders UI, prints output, swallows terminal errors, stores authoritative child state, or constructs an alternate engine path. |

## Confirmed AutodevPipeline Profile Characterization Catalog

The scenarios below use scripted core and Engineer providers, deterministic clocks and identifiers, test-owned homes and local repositories, controlled execution barriers, immutable CodexSpec fixtures, fake Git and GitHub boundaries, and bounded local process trees. They MUST NOT contact a real model, GitHub, or another external service, use ambient credentials or user state, depend on wall-clock sleeps or uncontrolled scheduling, or leave sessions, processes, worktrees, or global state after completion.

| ID | Required scenario |
|---|---|
| `PF-AUT-001` | Resolving `AutodevPipeline` produces an immutable, flat core-runtime snapshot containing a fresh CLI-source session per item attempt, CLI-configured turn budget with unlimited default, serial run policy, disabled thinking and effort, exact capability ceiling, Engineer question port, memory, checkpoint, compaction, observation, and outcome policies. A stage or invocation cannot expand a ceiling. |
| `PF-AUT-002` | A typed item-run specification freezes item and stage identity, worktree, feature directory, session and run identity, model, turn limit, tool surface, parent cancellation, and applicable project configuration before each core run. Later ledger, backlog, configuration, or caller mutation cannot alter that active run. |
| `PF-AUT-003` | Each item attempt creates one fresh CLI-source core session. Every SDD, correction, and publication run in that attempt shares its continuous conversation, while each run has distinct identity and state. A restarted process may reuse ledger and worktree progress but creates a fresh core session and cannot accidentally resume another item's conversation. |
| `PF-AUT-004` | `autodev.yml` model selection overrides the CLI-resolved model; otherwise the resolved CLI model applies. The Engineer and every core runner use the same frozen provider protocol, model, and credentials, while distinct item runners remain isolated. |
| `PF-AUT-005` | Core and Engineer model calls perform no thinking request or model-delta streaming. Non-positive core turn limits remain unlimited, positive limits preserve exact boundaries, and recovery, reminder, re-anchoring, and partial-result behavior remain shared runtime behavior. |
| `PF-AUT-006` | The model-visible and runtime-executable core tool surfaces agree exactly for file read, write, and edit, Bash, TODO read and update, root-level `delegate_task`, model-invocable `skill`, and Engineer-backed `ask_user_question`. Definitions, aliases, invocation, structured failures, large results, and parallel-safety lookup derive from one snapshot. |
| `PF-AUT-007` | Autodev installs no human permission coordinator. `ask_user_question` is an Engineer decision channel only: every question receives one answer or the existing visible conservative fallback, while permission assessment cannot be inferred from that answer or wait for an unavailable human approval surface. |
| `PF-AUT-008` | CodexSpec and project slash commands are materialized through the runner's current slash registry and executor with exact command name, artifact arguments, worktree, session, variable substitution, embedded-shell cancellation, hooks, and conditional skill activation. Skill activation cannot expand the profile ceiling. |
| `PF-AUT-009` | Root-level delegation enters the shared `ChildRunner` with the current item session, worktree, provider, model, cancellation, permission, and capability ceilings. Runtime depth enforcement and child tool filtering preserve the confirmed one-level child topology. |
| `PF-AUT-010` | Every core model request receives the correct base and project instructions, stage prompt or correction, continuous item conversation, session working memory, automemory projection, skill list, and applicable reminders without TUI, Formal Plan, presentation, or human-permission fragments. |
| `PF-AUT-011` | Automatic compaction uses the frozen item model and current core session and preserves trigger, summary, boundary, observer facts, prompt-too-long retry, failure fallback, circuit-breaker recovery, persistence ordering, and next request across stage runs. It cannot compact another item's conversation. |
| `PF-AUT-012` | Working memory, TODO state, checkpoints, per-message state history, compact state, transcript, metrics, tracing, and artifacts remain scoped to the current item session and worktree. Autodev exposes no session resume, manual compaction, rewind, Formal Plan, or plan-review interaction. |
| `PF-AUT-013` | Post-run automemory extraction preserves its current run-ID-bounded, asynchronous, non-terminal behavior: launch failure or panic cannot alter the returned core outcome. Ordering relative to the next item and worktree cleanup is frozen only after `DV-AUT-008`. |
| `PF-AUT-014` | Runtime emits UI-neutral run, model, tool, compaction, final, error, completion, artifact, and telemetry outcomes in canonical order. It does not decide stage completion, mutate the ledger, create worktrees, run quality gates, publish GitHub objects, print terminal lines, or render TUI entries. |
| `PF-AUT-015` | Provider, tool, permission-port, persistence, compaction, turn-limit, and cancellation failures produce one correlated core terminal outcome and cannot leak run state into a correction or later item. Partial-result delivery and control-plane handling are frozen after `DV-AUT-010`. |
| `PF-AUT-016` | `internal/autodev` is a privileged runtime control client that obtains item-scoped sessions and runs through `RuntimeHarness`. It MUST NOT construct a concrete engine or provider, and runtime MUST NOT import Autodev backlog, ledger, stage, worktree, gate, Engineer, GitHub, reporter, CLI, or TUI types. |

## Confirmed Autodev Control-Plane Characterization Catalog

`CP-AUT` denotes the deterministic Autodev workflow outside the core runtime and outside presentation adapters.

| ID | Required scenario group |
|---|---|
| `CP-AUT-001` | Missing, empty, valid, malformed, duplicate-key, and unknown-field `.foxharness/autodev.yml` inputs preserve exact defaults, strict YAML behavior, relative-path basis, string and boolean merge semantics, model and Engineer-persona precedence, and diagnostic errors. |
| `CP-AUT-002` | The test gate is always forced on, disabled build and gofmt gates emit current warnings, auto-merge is always forced off, and every issue, PR, and link toggle combination resolves deterministically. Accepted `concurrency` values and unsupported values are frozen after `DV-AUT-009`. |
| `CP-AUT-003` | Backlog parsing preserves heading type, title, multiline description, priority normalization, advisory status, document order, duplicate titles, ignored preamble and unknown fields, empty backlog, missing file, read errors, scanner limits, Unicode, and exact error precedence. |
| `CP-AUT-004` | Stable item identity, slug generation and collision suffixes, duplicate titles, backlog insertion, deletion, rename, reorder, requirement edits, and restart reconcile deterministically without reprocessing done work or silently executing stale work. Affected identity and description authority are frozen after `DV-AUT-003`. |
| `CP-AUT-005` | Ledger load, missing-file initialization, JSON parsing, seed, pending, in-progress, done, branch, stage, issue, PR, feature-directory, timestamp, priority ordering, advisory backlog status, unknown status, duplicate identity, and restart behavior preserve one authoritative progress model. |
| `CP-AUT-006` | Every authoritative transition is persisted atomically through temporary-file create, write, close, rename, and cleanup with deterministic permissions and no torn file. Directory, encode, create, write, close, and rename failures cannot be reduced to warnings before irreversible work; affected behavior is frozen after `DV-AUT-001`. |
| `CP-AUT-007` | In-progress items resume before pending items; each group is ordered high, medium, low and then by stable seed order; done items are skipped; an empty backlog is a successful no-op; and at most one item and one stage execute at a time regardless of configuration. |
| `CP-AUT-008` | Startup validates repository identity and conditionally validates `gh` availability and authentication before processing any item. Repository, base branch, remote, backlog, configuration, GitHub-disabled, cancellation, and error precedence produce the documented precondition or ordinary failure classification. |
| `CP-AUT-009` | A pending item creates an isolated managed worktree and lockstep `auto/<slug>` branch from the configured base, using bounded numeric collision suffixes. Missing base, existing branches, existing paths, registration conflicts, path creation, cancellation, and Git errors cannot select the primary or invoking checkout. |
| `CP-AUT-010` | An in-progress item resumes only the registered checkout of its recorded branch inside the canonical managed root, prunes stale registrations before reattachment, and rejects primary, invoking, foreign, symlink-escaped, malformed, or externally managed worktrees. |
| `CP-AUT-011` | A crash-leftover worktree is reused only when branch registration and unpublished state prove it belongs to the item. Clean or dirty published leftovers, missing or divergent remote tips, unreachable remotes, retained branches, and reset ledgers preserve conservative suffix or resume behavior without pushing post-publication debris. |
| `CP-AUT-012` | `materialize-requirements` creates exactly one bound CodexSpec feature directory and non-empty confirmed `requirements.md` from the complete authoritative backlog item, preserving timestamps, IDs, title, evidence, status, and restart idempotency. Content loss uses `DV-AUT-004`; path containment uses `DV-AUT-005`. |
| `CP-AUT-013` | The fixed pipeline executes `materialize-requirements`, `generate-spec`, `spec-to-plan`, `plan-to-tasks`, and `implement-tasks` in order. Configuration, a core response, Engineer response, or ledger mutation cannot reorder, omit, add, or prematurely terminate a required stage. |
| `CP-AUT-014` | Every command stage resolves the current slash command through the core runner and passes the explicit requirements, spec, plan, or tasks artifact path. Missing registry, missing command, missing executor, argument processing, embedded-shell failure, cancellation, and appended instructions preserve exact errors and do not invoke the model with an invalid prompt. |
| `CP-AUT-015` | For each model-driven stage, the control plane emits start, runs the core, evaluates read-only ground truth, reports the result, and advances only on success. A failed verification obtains one Engineer correction and retries the same stage; Engineer approval cannot override a gap. Core error and partial outcomes use `DV-AUT-010`. |
| `CP-AUT-016` | Spec, plan, and tasks stages require the expected non-empty artifact and review file plus a parseable exact `PASS` or `PASS_WITH_WARNINGS` overall status. Missing, empty, malformed, failing, stale, misbound, or unreadable artifacts produce a precise gap and cannot advance. |
| `CP-AUT-017` | Implementation verification requires at least one task checkbox, every checkbox complete, the configured quality gate green, and either a dirty worktree or a non-empty base-to-HEAD diff. Unchecked tasks, empty changes, Git query errors, gate infrastructure errors, and ordinary gate failures remain distinguishable. |
| `CP-AUT-018` | Completion gates run build, mandatory test, and gofmt in fixed order; disabled optional steps are represented as skipped; failures preserve command output and do not short-circuit later configured gates; aggregate success requires every enabled step to pass. Resource limits and cancellation are frozen after `DV-AUT-006`. |
| `CP-AUT-019` | Publication first stages all changes and then creates a commit through the materialized `codexspec:commit-staged` flow. Verification requires staged changes or a clean branch with commits beyond base, then a clean committed branch; resume skips only monotonically verified completed work. |
| `CP-AUT-020` | Push follows commit and succeeds only when the configured remote branch tip exactly equals local HEAD. Missing local HEAD, missing remote branch, divergent tip, authentication failure, cancellation, stale output, and retry preserve a single correlated stage outcome. |
| `CP-AUT-021` | When enabled, issue creation follows push and records a verified issue before PR creation. Title, item identity, search parsing, closed issues, duplicates, restart, record failure, and exactly-once reporting use the baseline frozen after `DV-AUT-001` and `DV-AUT-007`. |
| `CP-AUT-022` | When enabled, PR creation follows the applicable issue step, targets the configured base and item branch, verifies a non-zero PR identity, and requires the exact `Closes #N` link when configured. Missing, malformed, wrong-branch, unlinked, duplicate, cancelled, and resumed outcomes cannot mark publication complete. |
| `CP-AUT-023` | Disabling issue creation, PR creation, or issue linking preserves the current ordered subset without requiring unused GitHub preconditions. No configuration or model instruction can invoke merge, mark an unverified remote object complete, or move publication concerns into runtime. |
| `CP-AUT-024` | Restart resumes the recorded SDD stage or re-enters publication and uses current filesystem, Git, GitHub, and ledger ground truth to skip already completed work without duplicating stages, commits, pushes, issues, or PRs. Empty, known, publish, done, renamed, malformed, and unknown stage behavior is frozen after `DV-AUT-002`. |
| `CP-AUT-025` | Success persists done with branch, feature directory, issue and PR identities before emitting one item-done event and attempting worktree removal; removal failure remains a visible non-fatal inspection aid. Failure, panic, cancellation, save failure, reporter failure, and process exit preserve one correlated terminal state, resumable resources, and canonical item, stage, run, gate, remote, ledger, and cleanup event order. |

## Confirmed Autodev Entry-Adapter Characterization Catalog

| ID | Required scenario group |
|---|---|
| `UI-AUT-001` | `fox autodev` routing preserves optional backlog positional input, `-prompt`, `-C` and `-workdir`, model and provider options, max turns, duplicate input rejection, conflicts with print or TUI modes, help, parse errors, and configuration-resolution precedence without interpreting backlog content in the entry point. |
| `UI-AUT-002` | The terminal adapter maps successful drain, `PreconditionError`, and ordinary failure to exit statuses zero, two, and one; writes the complete line-oriented core and control stream to stdout; writes terminal command failure text to stderr; and never reports a failed or cancelled pipeline as success. Signal and descendant cleanup use `DV-AUT-006`. |
| `UI-AUT-003` | The TUI registers `/autodev [backlog-path]`, reports an unavailable launcher, refuses to start while another run owns the session, creates a cancellable run context, marks the model running, preserves start status and command entry, and restores the normal run-finished state exactly once. |
| `UI-AUT-004` | The TUI reporter maps core events and item, worktree, stage, Engineer, verification, gate, issue, PR, done, warning, and failure facts into the existing event channel and session entries with current titles, bodies, status text, ordering, and cancelled-context non-blocking behavior. |
| `UI-AUT-005` | Terminal and TUI reporters serialize concurrent event writes, preserve current one-line truncation and multiline formatting, expose errors and fallback decisions, and never mutate ledger, runtime, gate, or publication state. Writer, channel, cancellation, and shutdown behavior remain adapter-owned. |
| `UI-AUT-006` | CLI and TUI composition map the same typed Autodev launch request and control-plane event stream to their adapters. Neither entry constructs an engine, duplicates orchestration, interprets verification, performs Git or GitHub mutation, stores authoritative progress, or makes terminal or Bubble Tea types part of runtime or control-plane contracts. |

## Confirmed Runtime Profile Matrix

The profile names below describe required behavior bundles. Shared implementation fragments do not remove the requirement to resolve and verify each row independently.

### Lifecycle and execution policy

| Profile | Session lifecycle and persisted source | Workspace and model scope | Budget and scheduling |
|---|---|---|---|
| `TUIInteractive` | Creates a CLI-source session by default; supports explicit session, continue-latest CLI session, and forced-new selection; keeps one session across multiple runs and permits in-TUI session changes. | Workspace is fixed for the launch; model and effort may change through existing TUI controls. | CLI-configured turn limit, defaulting to unlimited; runs within one session are serialized. |
| `CLIExec` | Creates a CLI-source session by default; supports explicit session, continue-latest CLI session, and forced-new selection; executes one run and exits. | Workspace is fixed for the invocation; model, effort, and legacy thinking use resolved CLI settings. | CLI-configured turn limit, defaulting to unlimited; one synchronous run. |
| `FeishuRemote` | Reuses the latest Feishu-source session keyed by chat and sender; `/new` and `新会话` create a new session; same-session tasks are serialized. | Workspace and provider are process-configured. | 20 turns and a five-minute task timeout; global task concurrency of four. |
| `AgentOpsTask` | Creates a fresh session for every task and preserves the current Feishu persisted source value. | Workspace, provider, and log directory are process-configured. | 24 turns and a five-minute task timeout; global task concurrency of four. |
| `BenchmarkEval` | Creates a fresh CLI-source session and fresh fixture-copy temporary workspace for every repeat. | Provider and model resolve from user settings for the benchmark run. | Case-defined turn limit, defaulting to 12; repeats are serial; there is currently no whole-case timeout, while each command validation has a two-minute timeout. |
| `ChildRun` | Creates a fresh subagent-source child session for every delegation while retaining the parent-session reference. | Inherits the parent workspace and provider but has an independent session scope. | Default 200-turn child budget; propagates parent cancellation; the parent waits synchronously for the child report. |
| `AutodevPipeline` | Creates a fresh CLI-source core session per item attempt; all SDD stages in that attempt share it. Restarted work may reuse the worktree and ledger position while creating a fresh core session. | Each item uses its own worktree; `autodev.yml` model selection overrides the CLI-resolved model. | CLI-configured turn limit, defaulting to unlimited; backlog items are processed strictly serially. |

### Capability and interaction policy

| Profile | Tool and control surface | Interaction and permission behavior |
|---|---|---|
| `TUIInteractive` | File read/write/edit, Bash, TODO, skill, and root-level delegation; supports run restrictions and formal-plan tool phases. | Installs interactive user-question and plan-review ports plus the full TUI permission coordinator with Ask, Approve, Full Access, provider review, and session grants. |
| `CLIExec` | File read/write/edit, Bash, TODO, skill, and root-level delegation; no user-question or formal-plan interaction surface. | Has no permission coordinator; the current undecorated registry execution semantics MUST be preserved. |
| `FeishuRemote` | File read/write/edit, Bash, TODO, and root-level delegation; no skill, user-question, or formal-plan surface. | Uses `ModeAsk` and the existing remote approval interaction. |
| `AgentOpsTask` | File read/write/edit, Bash, TODO, root-level delegation, and `log_search`; no skill, user-question, or formal-plan surface. | Uses `ModeAsk` and the existing remote approval interaction. |
| `BenchmarkEval` | Fixed file read/write/edit, Bash, and TODO surface; no delegation, skill, user-question, formal-plan, or interactive approval surface. | Uses explicit benchmark capabilities without a user permission coordinator; validations remain benchmark control-plane operations. |
| `ChildRun` | Always provides read and Bash; write and edit are conditional on child read-only policy; an allowed-tools list further intersects that surface; excludes TODO, skill, user-question, formal-plan, and every delegation capability. | Inherits the parent's permission coordinator and evidence when present and MUST never weaken the parent's security ceiling; it has no direct user interaction port. Runtime depth enforcement and capability filtering both enforce maximum delegation depth one. |
| `AutodevPipeline` | Inherits the product root file, Bash, TODO, skill, and root-level delegation surface; SDD stages and gates remain Autodev control-plane operations. | Has no human permission coordinator; `ask_user_question` is answered by the Engineer Agent, which MUST remain distinct from permission approval. |

### State, context, and observation policy

| Profile | Recoverable state and memory | Compaction | Observation and completion behavior |
|---|---|---|---|
| `TUIInteractive` | Session memory, automemory, checkpoints, per-message state history, and user-visible rewind; post-run automemory extraction remains asynchronous. | Preserves automatic and user-triggered manual compaction. | Synchronously ordered lifecycle and tool notifications plus visible model-text delta streaming through the TUI. |
| `CLIExec` | Session memory, automemory, checkpoints, and per-message state history; no user rewind entry; waits for tracked automemory extraction before process exit. | Preserves automatic compaction. | Prints the final response and existing session, transcript, run, metrics, and trace locations after execution; no model-delta presentation. |
| `FeishuRemote` | Session memory and automemory with fire-and-forget post-run extraction; currently no checkpoint or rewind capability. | Preserves automatic compaction, subject to separate model-configuration defect verification under CON-005. | Sends existing task receipt, lifecycle/tool progress, and final messages; no model-delta streaming. |
| `AgentOpsTask` | Session memory and automemory with fire-and-forget post-run extraction; currently no checkpoint or rewind capability. | Preserves automatic compaction, subject to separate model-configuration defect verification under CON-005. | Sends the existing task result and artifact information; no model-delta streaming. |
| `BenchmarkEval` | Session working memory only; no automemory, checkpoint, or rewind capability. | Preserves automatic compaction with the benchmark model. | Produces structured benchmark and validation results. Runtime-fidelity metadata MUST derive from the resolved specification and declared differences rather than an independently maintained behavioral claim. |
| `ChildRun` | Uses isolated read-only session memory and read-only shared automemory context; performs no automemory write or extraction and has no checkpoint or rewind capability. | Preserves automatic compaction, subject to separate model-configuration defect verification under CON-005. | Produces no direct user output and returns only the final high-density report to the parent. |
| `AutodevPipeline` | Session memory, automemory, checkpoints, and per-message state history; no user rewind entry; extraction remains asynchronous. The Autodev ledger remains separate control-plane state. | Preserves automatic compaction. | Emits the existing line-oriented core lifecycle, tool, stage, gate, and publishing output without model-delta streaming. |

## Out of Scope

### OUT-001: Physical repository or module split

- **Status**: confirmed
- **Statement**: Splitting the core runtime and UI into separate repositories or Go modules, independently publishing them, or maintaining a cross-module version compatibility matrix is outside this refactor.
- **Reason**: The selected architecture uses strong package boundaries inside one repository and Go module.
- **User Evidence**: The user selected the single-repository, single-module option.

### OUT-002: Functional product changes

- **Status**: confirmed
- **Statement**: New user-facing features, changed interaction behavior, changed permission behavior, changed tool behavior, and persisted-session format changes are outside this refactor unless separately specified and approved.
- **Reason**: The work is a non-functional code-organization refactor.
- **User Evidence**: "The refactor must not affect any functionality."

## Open Questions

### OPEN-001: Final package boundaries

- **Status**: resolved by DEC-027
- **Why It Matters**: The specification must define the responsibilities and allowed dependency directions among the core runtime, application services, runtime assembly, entry-point adapters, and TUI.
- **Owner**: User / Team

### OPEN-002: Scope of TUI extraction

- **Status**: resolved by DEC-017
- **Why It Matters**: The TUI may be only an agent-specific presentation adapter or may also extract a reusable generic terminal UI library similar to `pi-tui`; the two scopes have materially different cost and value.
- **Owner**: User / Team

### OPEN-003: Refactor sequence and acceptance gates

- **Status**: resolved by DEC-041
- **Why It Matters**: The dependency order, module-focused commit boundaries, and exact behavioral coverage gates must be defined before implementation starts.
- **Owner**: User / Team

### OPEN-004: Enforcement mechanism for single-level child delegation

- **Status**: resolved by DEC-016
- **Why It Matters**: DEC-007 fixes the externally observable delegation topology at one level, but the implementation may enforce that invariant through an explicit non-relaxable runtime capability ceiling or only through child tool-surface construction. The choice affects defense in depth, future tool additions, and complexity.
- **Owner**: User / Team

## Superseded Entries

<!-- Keep replaced entries with Status: superseded. -->

## Confirmation Log

### Session 2026-08-08 17:52 +0800

- **Summary Presented**: Strong package boundaries in one repository and Go module; a headless core runtime; one integration branch and final PR; no functional, interaction, entry-point, or persisted-session behavior changes; comprehensive characterization before implementation; package-level and final verification gates.
- **User Confirmation**: The user explicitly confirmed the summary and expanded behavior preservation to all user interactions and module functions, requiring complete pre-refactor behavioral coverage.
- **Entries Confirmed**: NEED-001, NEED-002, NEED-003, NEED-004, CON-001, CON-002, CON-003, DEC-001, DEC-002, OUT-001, OUT-002

### Session 2026-08-08 17:55 +0800

- **Summary Presented**: Internal Go source APIs may change when necessary to remove coupling, while all in-repository consumers migrate atomically and functional, behavioral, and persisted-data compatibility remain mandatory.
- **User Confirmation**: The user explicitly allowed internal Go API changes.
- **Entries Confirmed**: DEC-003

### Session 2026-08-08 17:58 +0800

- **Summary Presented**: CLI, TUI, Feishu, and AgentOps use small application capabilities and UI-neutral DTOs; entry adapters do not directly control concrete runtime subsystems; `cmd/*` remains the composition root; no generic event bus is introduced.
- **User Confirmation**: The user explicitly confirmed the proposed boundary.
- **Entries Confirmed**: DEC-004

### Session 2026-08-08 18:01 +0800

- **Summary Presented**: Benchmark is a privileged runtime-harness evaluation adapter that may control core modules directly while sharing real runtime contracts and invariants and declaring intentional fidelity differences.
- **User Confirmation**: The user adopted the recommendation and explicitly classified benchmark as evaluation and feedback for core agent capabilities rather than user interaction.
- **Entries Confirmed**: DEC-005

### Session 2026-08-08 18:04 +0800

- **Summary Presented**: Subagent delegation is a nested runtime capability invoked through the model-facing `delegate_task` tool and implemented through shared child-run construction and inherited runtime invariants.
- **User Confirmation**: The user explicitly confirmed the runtime classification and asked to verify the current tool-triggered nested execution model.
- **Entries Confirmed**: DEC-006

### Session 2026-08-08 18:06 +0800

- **Summary Presented**: Preserve the existing parent-to-child delegation depth and do not add child-to-descendant recursive delegation.
- **User Confirmation**: The user explicitly chose to keep delegation single-level.
- **Entries Confirmed**: DEC-007

### Session 2026-08-08 18:10 +0800

- **Summary Presented**: Reduce `AgentEngine` to the run/turn state machine and injected collaborator coordination, while validating every abstraction for distinct ownership and rejecting redundant or file-size-driven decomposition.
- **User Confirmation**: The user explicitly agreed and required the proposed boundaries to be checked for reasonableness, maintainability, extensibility, readability, and redundancy.
- **Entries Confirmed**: CON-004, DEC-008

### Session 2026-08-08 19:38 +0800

- **Summary Presented**: Independent subagent review found the proposed direction viable but required explicit recoverable-state ownership, bidirectional interaction ports, observer/artifact/telemetry separation, acyclic runtime/application types, non-overlapping harness/session/run lifetimes, precise collaborator ownership, and inclusion of Autodev as a runtime control client.
- **User Confirmation**: The user adopted the revised architecture and requested further discussion of whether single-level child delegation requires a non-relaxable runtime safety constraint, with comparison to Codex and Claude Code.
- **Entries Confirmed**: DEC-009, DEC-010, DEC-011, DEC-012, DEC-013, DEC-014, DEC-015
- **Entries Superseded**: DEC-008

### Session 2026-08-08 19:44 +0800

- **Summary Presented**: Codex stable defaults to a runtime-enforced depth of one, while Claude Code normally removes the Agent tool from child tool surfaces and separately rejects recursive fork workers; both retain other experimental or internal multi-level modes. For Fox, the minimal behavior-preserving design is a single runtime child-creation gate plus child capability filtering, without a general configurable agent-tree subsystem.
- **User Confirmation**: The user adopted the recommended dual enforcement for Fox.
- **Entries Confirmed**: DEC-016
- **Open Questions Resolved**: OPEN-004

### Session 2026-08-08 19:51 +0800

- **Summary Presented**: The TUI can be decoupled as a Fox-specific application presentation adapter or expanded into a generic reusable terminal UI library; the latter has no current second consumer and would add scope and abstractions unrelated to the refactor objective.
- **User Confirmation**: The user adopted the Fox-specific presentation-adapter scope and rejected extracting a `pi-tui`-style generic library in this refactor.
- **Entries Confirmed**: DEC-017
- **Open Questions Resolved**: OPEN-002

### Session 2026-08-08 19:53 +0800

- **Summary Presented**: Architecture review surfaced potential pre-existing defects that must be fully verified; confirmed defects should be handled through separate requirements, failing regression tests, and defect-focused commits before establishing the refactor behavior baseline, while unconfirmed risks must not alter refactor behavior.
- **User Confirmation**: The user adopted the recommended pre-refactor defect-validation and separation principle.
- **Entries Confirmed**: CON-005

### Session 2026-08-08 20:25 +0800

- **Summary Presented**: Seven explicit runtime profiles cover TUI, CLI, Feishu, AgentOps, benchmark, child execution, and Autodev. Each profile resolves to an immutable, flat, independently testable behavior specification. Profiles define defaults, permitted variation, and capability ceilings, while per-run prompts, selections, restrictions, budgets, parentage, and observers belong to `RunSpec`. The complete lifecycle, capability, permission, state, context, compaction, and observation matrix preserves current entry-specific behavior and identifies pre-existing risks that require separate CON-005 validation.
- **User Confirmation**: The user explicitly confirmed and adopted the seven-profile matrix and the Profile/RunSpec boundary.
- **Entries Confirmed**: NEED-005, CON-006, DEC-018, DEC-019

### Session 2026-08-08 21:52 +0800

- **Summary Presented**: Local Codex and Claude Code source inspection showed that prompt or context fragment renderers do not own the complete context lifecycle. Codex gives its session/runtime layer explicit ownership of world-state capture, diffing, model-visible injection, history updates, and persistence order; Claude Code performs final assembly and per-turn injection in its runtime query pipeline but distributes more state across surrounding modules. The proposed Fox boundary keeps prompt rendering pure and assigns context collection, injection decisions, projection, and recoverable commits to runtime-owned collaborators.
- **User Confirmation**: The user explicitly confirmed renaming `internal/context` to `internal/prompt`, limiting it to pure prompt-fragment rendering, and assigning complete context lifecycle and injection decisions to runtime.
- **Entries Confirmed**: DEC-020

### Session 2026-08-08 23:07 +0800

- **Summary Presented**: Retain `internal/session`; distinguish the live `runtime.AgentSession` from persisted `session.StoredSession` and `session.StoredRun`; replace the ambiguous persistence manager with `session.FileStore` behind a consumer-owned `runtime.SessionStore`; use strong persisted IDs; keep message and compact-state names; clarify transcript artifact names; remove duplicate session working-memory ownership; and prevent persisted records from performing lifecycle or storage operations.
- **User Confirmation**: The user explicitly confirmed the complete proposed naming and responsibility set and requested that it be fixed during requirements discovery rather than deferred to planning.
- **Entries Confirmed**: DEC-021

### Session 2026-08-08 23:09 +0800

- **Summary Presented**: Keep provider, tools, session, memory, telemetry, and other concrete implementation capabilities in responsibility-focused packages rather than creating a unified `internal/infrastructure` package; use infrastructure only as an architectural classification, and rename package paths only when a clearer ownership boundary justifies the change.
- **User Confirmation**: The user explicitly confirmed the rule and emphasized that a broad infrastructure package would repeat the oversized-responsibility problem and violate high cohesion and low coupling.
- **Entries Confirmed**: DEC-022

### Session 2026-08-08 23:13 +0800

- **Summary Presented**: Retain `internal/app` as the concise application-layer package name while narrowing it to application use cases, UI-neutral commands and DTOs, notification adaptation, and interaction ports; move runtime assembly, presentation behavior, and composition to their actual owners.
- **User Confirmation**: The user corrected the proposed rename and reaffirmed the earlier decision to retain `internal/app` while improving its responsibility boundaries.
- **Entries Confirmed**: DEC-023

### Session 2026-08-08 23:19 +0800

- **Summary Presented**: Move the complete interactive presentation flow from `app.RunTUI` to the single `tui.Run` entry in `internal/tui`; move non-interactive `fox exec` and `fox -p` presentation and output formatting from `app.RunCLI` to `cli.Run` in a dedicated `internal/cli`; keep runtime completion semantics below those adapters; and restrict `cmd/fox` to process-input parsing, dependency composition, mode selection, and dispatch.
- **User Confirmation**: The user explicitly adopted the proposed exact locations and rationale.
- **Entries Confirmed**: DEC-024

### Session 2026-08-08 23:21 +0800

- **Summary Presented**: Place `RuntimeHarness`, `AgentSession`, `RunSpec`, `RunScope`, `Profile`, `ContextController`, `ChildRunner`, and the runtime-owned session persistence port in one cohesive `internal/runtime` package organized by files; do not create premature runtime subpackages; and keep engine, persistence, prompt rendering, compaction mechanics, benchmark, subagent protocol adaptation, and Autodev in independent packages.
- **User Confirmation**: The user explicitly confirmed the complete proposed runtime package scope.
- **Entries Confirmed**: DEC-025

### Session 2026-08-08 23:32 +0800

- **Summary Presented**: Codex comparison validated the thin engine direction but exposed missing Fox constraints: advertised and executable tools must share one immutable request snapshot; engine needs a consumer-owned `Conversation` port so runtime remains the context and commit owner; ordered engine facts need one typed `Observer` adapted into the runtime observer pipeline; and `AgentEngine` must not retain cross-run mutable state. The corrected package also narrows exported outcomes and expands the engine forbidden-dependency list.
- **User Confirmation**: The user explicitly adopted the corrected engine boundary.
- **Entries Confirmed**: DEC-026

### Session 2026-08-09 12:20 +0800

- **Summary Presented**: Codex and Claude Code comparison found that the proposed direction was sound but required a real package DAG rather than a simple layer chain, composition-only restrictions for `cmd/*`, protocol isolation between the model-facing subagent tool and runtime child execution, explicit acyclic interaction and observation mappings, dependency inversion for concrete implementations, and automated import-boundary enforcement.
- **User Confirmation**: The user explicitly confirmed the corrected dependency graph and added a separate requirement that module dependencies be documented.
- **Entries Confirmed**: DEC-027
- **Open Questions Resolved**: OPEN-001

### Session 2026-08-09 12:25 +0800

- **Summary Presented**: Module dependencies will be maintained as an architecture contract in `docs/package-dependencies.md`, using Mermaid as the authoritative dependency representation and documenting responsibilities, allowed and forbidden edges, composition exceptions, interaction and observation flows, and concrete injection points. Boundary changes must update this document and architecture import tests in the same commit, while draw.io remains optional non-authoritative presentation.
- **User Confirmation**: The user explicitly confirmed the proposed package-dependency documentation approach.
- **Entries Confirmed**: NEED-006, DEC-028

### Session 2026-08-09 12:37 +0800

- **Summary Presented**: Architecture import tests will use an exact decreasing allowlist that cannot grow and must be empty before final merge. The vulnerabilities confirmed by the architecture report were already repaired in merged work `#61` and `#63`; pre-refactor preparation therefore characterizes current behavior and verifies only residual unclassified risks, creating a separate defect requirement and fix only if a test proves an additional defect.
- **User Confirmation**: The user explicitly adopted the correction and requested detailed discussion of the characterization tests required before production code moves.
- **Entries Confirmed**: DEC-029, DEC-030

### Session 2026-08-09 13:38 +0800

- **Summary Presented**: Production architecture migration cannot begin until every behavior-preservation obligation and runtime-profile matrix cell has executable characterization coverage. All mandatory gates must be hermetic and offline, using controlled local resources rather than real external services or ambient user state. Persisted compatibility must be verified with immutable versioned fixtures generated once from the frozen pre-refactor baseline, copied to temporary directories for tests, and never regenerated by the implementation under test.
- **User Confirmation**: The user explicitly confirmed the hermetic definition and the complete pre-refactor test and persisted-fixture requirements.
- **Entries Confirmed**: NEED-007, CON-007, DEC-031

### Session 2026-08-09 13:43 +0800

- **Summary Presented**: Characterization coverage will use shared black-box runtime and profile scenarios plus entry-owned presentation and process tests. During migration, test-only adapters for the old and target implementations will run the same scenario authority until parity is proven, without preserving replaceable internal production APIs.
- **User Confirmation**: The user explicitly adopted the proposed characterization test organization and separately required strict Red-Green-Refactor TDD for new implementation code.
- **Entries Confirmed**: DEC-032

### Session 2026-08-09 13:59 +0800

- **Summary Presented**: New implementation and behavior corrections must follow an evidenced Red-Green-Refactor cycle. Red must fail for the expected missing or incorrect behavior, Green must be minimal, and Refactor must preserve all behavior and fixtures. Shared contracts drive the target implementation into Red before implementation; mechanical moves do not manufacture artificial failures; and each final module commit remains green and independently revertible.
- **User Confirmation**: The user explicitly confirmed the strict TDD definition.
- **Entries Confirmed**: CON-008

### Session 2026-08-09 14:17 +0800

- **Summary Presented**: Source comparison and Fox gap analysis produced a minimum shared runtime characterization catalog covering runtime turns, streaming, tool lifecycle, context and compaction, runtime policy, and run/session behavior. Existing tests may support but not replace these black-box scenarios; reference-only features remain excluded; and newly exposed defects follow the separate CON-005 process.
- **User Confirmation**: The user explicitly confirmed the complete catalog and requested clarification of its category abbreviations.
- **Entries Confirmed**: DEC-033

### Session 2026-08-09 16:04 +0800

- **Summary Presented**: The `TUIInteractive` profile requires eighteen profile-level scenarios covering resolved profile ceilings, session selection and resume, long-lived and queued runs, in-TUI session replacement, model and effort changes, exact tool surfaces and restrictions, interactive question and plan ports, permission modes and grants, memory and extraction, rewind, compaction, ordered streaming observation, cancellation cleanup, and presentation isolation. Six TUI-owned scenario groups separately retain complete terminal interaction and rendering coverage.
- **User Confirmation**: The user reviewed the scenario set, found no issue, and requested Codex and Claude Code source cross-validation before recording it. The comparison found no conflicting design defect and strengthened explicit assertions for model-dependent context refresh, duplicate-free resume, overlay-gated queue dispatch, guarded cancel restoration, visible-history versus model-context projection, and stale prior-run completion.
- **Entries Confirmed**: DEC-034

### Session 2026-08-09 16:40 +0800

- **Summary Presented**: The `CLIExec` profile requires fourteen profile-level scenarios covering immutable resolution, CLI session selection, one synchronous run, exact model-visible and executable tool surfaces, absence of interactive ports and permission coordination, skills and child delegation, context and resume, checkpoint persistence without rewind, automatic compaction, output-first extraction drain, final-message deduplication, complete outcomes and artifacts, failure cleanup, and presentation isolation. Four CLI-owned groups separately preserve process routing, prompt acquisition, configuration precedence, exact stdout and stderr, errors, and exit status.
- **User Confirmation**: The user explicitly confirmed the scenario set and clarified that the Chinese term for the declared tool set should be "model-visible tool surface" rather than a literal translation of "advertised." The user also confirmed that completed output remains visible before tracked extraction drains and that process exit waits for the drain.
- **Entries Confirmed**: DEC-035

### Session 2026-08-09 17:12 +0800

- **Summary Presented**: Comprehensive FeishuRemote source inspection and Codex and Claude Code cross-validation identified eight residual risks requiring proof before the remote profile baseline is frozen: approval callback reachability, webhook idempotency, missing-sender session isolation, timeout-aware session locking, same-session ordering and global scheduling, coordinated shutdown and in-flight draining, approval terminal-state concurrency, and compactor model consistency. Each remains a potential defect until a hermetic deterministic test proves it.
- **User Confirmation**: The user explicitly adopted all eight items as the `CON-005` FeishuRemote pre-refactor defect-verification checklist and retained the separate requirement, TDD correction, and defect-focused commit process for any item that is proven.
- **Entries Amended**: CON-005

### Session 2026-08-09 17:18 +0800

- **Summary Presented**: The Feishu task dispatcher recovers a task panic and releases its global concurrency permit, but it does not visibly send the originating user a terminal failure reply. The associated cleanup of session locking, permission waits, and other run-scoped work therefore requires explicit verification rather than being silently preserved or repaired during refactoring.
- **User Confirmation**: The user explicitly confirmed adding this panic terminal-outcome and cleanup risk as the ninth FeishuRemote pre-refactor defect-verification item.
- **Entries Amended**: CON-005

### Session 2026-08-09 17:31 +0800

- **Summary Presented**: The `FeishuRemote` profile requires eighteen profile scenarios covering immutable resolution, typed task identity, durable session selection, concurrency and timeout, exact tool and permission surfaces, approval evidence and correlation, single-level child execution, context and memory, compaction, asynchronous extraction, ordered observation, non-streaming execution, outcomes, cleanup, and transport isolation. Seven Feishu-owned adapter groups cover process configuration, authenticated webhook handling, event translation, durable duplicate delivery, exact outbound messages, delivery failures, and approval callbacks. Ten separately gated `DV-FEI` risks determine affected expectations before the behavior baseline is frozen.
- **User Confirmation**: The user accepted the complete scenario set and requested Codex and Claude Code source cross-validation. The comparison found an outbound-delivery failure gap and strengthened durable duplicate handling, stale correlation, unknown events, and exactly-once approval cleanup; the user explicitly confirmed those corrections.
- **Entries Confirmed**: DEC-036
- **Entries Amended**: CON-005

### Session 2026-08-09 17:41 +0800

- **Summary Presented**: The architecture-report AgentOps fixes for concurrency, task timeout, deduper retention, streaming log scanning, and unified parent and child permissions are already present and must not be repeated. Six additional unclassified risks require proof before the AgentOps baseline is frozen: deduplication durability and acceptance timing, coordinated two-queue shutdown, panic terminal cleanup, compactor model consistency, outbound terminal delivery, and log-directory containment and resource bounds. AgentOps reuses the Feishu approval reachability and exactly-once terminal-state verification rather than defining another approval protocol.
- **User Confirmation**: The user explicitly confirmed adding `DV-AOP-001` through `DV-AOP-006` to the CON-005 pre-refactor verification gate.
- **Entries Amended**: CON-005

### Session 2026-08-09 18:00 +0800

- **Summary Presented**: The `AgentOpsTask` profile requires nineteen profile scenarios covering immutable resolution, exact incident task and prompt identity, fresh sessions, bounded scheduling, session-notice ordering, exact tools, functional and secure log search, absent interaction capabilities, remote permission and approval correlation, single-level children, context and memory, compaction, asynchronous extraction, correlated observation, outcomes and artifacts, terminal cleanup, and runtime isolation. Six AgentOps-owned adapter groups preserve process configuration, Feishu-to-task mapping, deduplication, exact messages, delivery failures, approval callbacks, and coordinated shutdown. Codex and Claude Code cross-validation found no conflicting profile design but strengthened empty message-ID, task/session/run/terminal correlation, stale-completion isolation, unique terminal outcome, accepted-work shutdown, and final-resolved-path containment assertions.
- **User Confirmation**: The user explicitly confirmed the cross-validated `PF-AOP-001` through `PF-AOP-019` and `UI-AOP-001` through `UI-AOP-006` scenario set and its refinements.
- **Entries Confirmed**: DEC-037
- **Entries Amended**: CON-005

### Session 2026-08-09 18:23 +0800

- **Summary Presented**: The `BenchmarkEval` profile requires sixteen profile scenarios covering immutable resolution, case-controlled run input, fresh fixture workspaces and CLI-source sessions, serial repeat isolation, provider and model snapshots, exact tools, absent interaction capabilities, prompt and session memory, compaction, runtime policies, UI-neutral outcomes, evaluation handoff, terminal and validation separation, reproducible provenance, bounded termination, and runtime-control ownership. Eleven `EV-BEN` control-plane groups preserve process and case inputs, fixture materialization, harness construction, command and file validations, ordered aggregation, human summaries, JSON reports, and process failure precedence. Codex and Claude Code contain no equivalent benchmark runner, but their rollout identity and terminal states, process-tree cleanup, path security, normalized immutable fixtures, bounded output, and exit-status handling strengthened seven pre-baseline benchmark defect-verification gates.
- **User Confirmation**: The user explicitly confirmed the cross-validated `PF-BEN-001` through `PF-BEN-016`, `EV-BEN-001` through `EV-BEN-011`, and `DV-BEN-001` through `DV-BEN-007` set and its terminal-state, provenance, fixture, output-bound, and process-cleanup refinements.
- **Entries Confirmed**: DEC-038
- **Entries Amended**: CON-005

### Session 2026-08-09 19:12 +0800

- **Summary Presented**: The synchronous `ChildRun` profile requires twenty-two profile scenarios covering immutable resolution, parent lineage, fresh child sessions, one-shot execution, frozen model and workspace scope, bounded turns, exact capability and permission surfaces, non-bypassable one-level depth, prompt and memory behavior, child-local compaction, typed outcomes, cancellation, process-tree cleanup, and runtime ownership. Six `IA-CHD` invocation-adapter groups preserve the `delegate_task` schema and assessment protocol, exact runtime invocation and result adaptation, fork-skill inputs, and shared ceilings without treating either path as presentation UI. Codex and Claude Code cross-validation strengthened parent-snapshot, lineage, capability-intersection, terminal-correlation, partial-result, and cleanup coverage while excluding their background, resume, worktree, agent-tree, MCP, and team features. Six separately gated risks cover read-only Bash mutation, compactor model drift, prompt/tool mismatch, shell descendants, ignored fork-agent selection, and discarded partial outcomes.
- **User Confirmation**: The user explicitly confirmed the complete cross-validated `PF-CHD-001` through `PF-CHD-022`, `IA-CHD-001` through `IA-CHD-006`, and `DV-CHD-001` through `DV-CHD-006` set.
- **Entries Confirmed**: DEC-039
- **Entries Amended**: CON-005

### Session 2026-08-09 19:25 +0800

- **Summary Presented**: `AutodevPipeline` requires sixteen core runtime-profile scenarios, twenty-five deterministic control-plane scenario groups, and six CLI and TUI entry-adapter groups. The catalogs separate per-item core sessions and shared runtime behavior from configuration, backlog and ledger authority, worktrees, fixed CodexSpec stages, deterministic verification, quality gates, Engineer supervision, GitHub publication, restart recovery, reporting, and presentation. Codex cloud-task identity, explicit terminal states, preflight and stale-result handling and Claude Code cancellation, cleanup, exactly-once notification, and storage finalization strengthened ten pre-baseline Autodev risk gates without importing cloud, background, best-of-N, resumable-message, or multi-worker features.
- **User Confirmation**: The user explicitly confirmed the complete cross-validated `PF-AUT-001` through `PF-AUT-016`, `CP-AUT-001` through `CP-AUT-025`, `UI-AUT-001` through `UI-AUT-006`, and `DV-AUT-001` through `DV-AUT-010` scenario set.
- **Entries Confirmed**: DEC-040
- **Entries Amended**: CON-005

### Session 2026-08-09 20:00 +0800

- **Summary Presented**: Complete characterization is an unskippable Phase 0 gate before any production architecture refactor commit. Every shared, profile, presentation, transport, evaluation, child-invocation, and Autodev-control scenario must map to executable hermetic tests and authoritative fixtures or outcomes; every mandatory test must pass against the current implementation and under `go test ./...`; every `DV-*` item must be resolved; proven defects must be separately approved, corrected through TDD, and incorporated into the re-run baseline; and final evidence must trace each scenario to its test, fixture, command, result, and frozen source commit. Phase 0 may add tests, immutable fixtures, test-only adapters, deterministic harnesses, the initial architecture allowlist, and separately approved defect fixes, but may not begin production package, dependency, runtime, application, or entry migration.
- **User Confirmation**: The user explicitly confirmed this Phase 0 definition and stated that refactoring without all characterization tests would provide no observable standard for proving that behavior remained intact.
- **Entries Amended**: NEED-007

### Session 2026-08-09 20:11 +0800

- **Summary Presented**: Production migration starts only after the complete `B00` baseline freeze, then proceeds bottom-up through pure prompt and persisted-state boundaries (`M01`-`M03`), the target turn engine (`M04`-`M08`), runtime lifecycle ownership (`M09`-`M13`), profile-atomic cutovers from benchmark and child execution through CLI, Autodev, remote clients, and TUI (`M14`-`M23`), and mandatory old-path and compatibility-layer removal plus final architecture verification (`M24`-`M27`). Every production boundary is independently testable, reviewable, bisectable, and revertible; old and target implementations share black-box tests during migration, but each profile always has exactly one production path and the architecture allowlist can only decrease.
- **User Confirmation**: The user explicitly confirmed the proposed robust, testable, and traceable production-module migration sequence and commit boundaries.
- **Entries Confirmed**: DEC-041
- **Open Questions Resolved**: OPEN-003

### Session 2026-08-09 21:31 +0800

- **Summary Presented**: T040 executable proofs established every `DV-FEI-001` through `DV-FEI-010` risk as a pre-existing defect. The proposed corrections provide an authenticated approval callback, durable at-most-once message acceptance, missing-sender rejection, acceptance-scoped cancellation and timeout, per-session FIFO scheduling without permit starvation, coordinated shutdown, non-blocking exactly-once approval resolution, frozen compactor model propagation, correlated panic failure, and typed delivery-failure observation.
- **User Confirmation**: The user explicitly confirmed the proposed Feishu correction principles and authorized the separate TDD defect-correction workflow before continuing automatically.
- **Entries Confirmed**: DEC-042
- **Entries Amended**: CON-005

### Session 2026-08-10 11:09 +0800

- **Summary Presented**: T041 proved six AgentOps defects. Local Codex and Claude Code source cross-validation supported a single durable task-acceptance authority, coordinated bounded shutdown, typed exactly-once terminal transitions, one immutable task model snapshot shared with compaction, separately observable delivery failures, and resolved-path filesystem confinement. The comparison clarified that transport-specific echo or replay suppression is not a second task-acceptance authority and strengthened `log_search` from check-then-open canonicalization to rooted or equivalently race-resistant open semantics.
- **User Confirmation**: The user requested the additional source-level cross-validation and authorized adoption of the correction semantics if no conflicting defect was found; no conflict was found after the stated clarifications.
- **Entries Confirmed**: DEC-043
- **Entries Amended**: CON-005

## Appendix A: Scenario Prefix Glossary

These prefixes are stable traceability labels for the Confirmed Shared Runtime Characterization Catalog. They do not prescribe Go package, directory, type, or production API names.

| Prefix | Expansion | Scope |
|---|---|---|
| `RT` | Runtime Turn | Turn state transitions, model invocation, tool-follow-up flow, completion, and turn limits. |
| `ST` | Streaming | Model text deltas, streaming fallback, partial output, streaming failure, and stream-state cleanup. |
| `TL` | Tool Lifecycle | Tool advertisement, invocation, permission, scheduling, execution, result correlation, ordering, and persistence. |
| `CX` | Context | Model-visible conversation context, initial projection, compaction, continuation, resume, and rewind. |
| `PL` | Policy | Completion and TODO gates, reminders, recovery injection, cooldowns, and other runtime turn policies. |
| `RS` | Run and Session | Run and session lifecycle, cancellation, serialization, isolation, persistence failure policy, outcomes, and ordered observation. |
| `PF-<profile>` | Runtime Profile | Profile resolution, capability ceilings, runtime/application wiring, and profile-specific observable or persisted behavior. Examples: `PF-TUI` for `TUIInteractive`, `PF-CLI` for `CLIExec`, `PF-FEI` for `FeishuRemote`, `PF-AOP` for `AgentOpsTask`, `PF-BEN` for `BenchmarkEval`, `PF-CHD` for `ChildRun`, and `PF-AUT` for `AutodevPipeline`. |
| `UI-<adapter>` | Presentation or Transport Adapter | Entry-owned input, rendering, transport, settings, and process-interaction behavior that must not become a core runtime contract. Examples: `UI-TUI` for the TUI adapter, `UI-CLI` for the print-mode CLI adapter, `UI-FEI` for the Feishu transport adapter, `UI-AOP` for the AgentOps process and transport adapter, and `UI-AUT` for Autodev's CLI and TUI adapters. |
| `EV-BEN` | Benchmark Evaluation Control Plane | Benchmark case loading, fixture materialization, validation, aggregation, provenance, reporting, and process behavior owned outside the runtime and application presentation layers. |
| `IA-CHD` | Child Invocation Adapter | Model-facing `delegate_task` and fork-skill request assessment, normalization, runtime invocation, and result adaptation into the shared `ChildRunner` capability. |
| `CP-AUT` | Autodev Control Plane | Autodev configuration, backlog, durable ledger, worktree, stage verification, Engineer supervision, quality gate, publication, recovery, and cleanup behavior outside runtime and presentation adapters. |
