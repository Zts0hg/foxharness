# Feature Specification: Progressive Plan Mode

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Feature ID**: `2026-0711-1441tc`
**Feature Branch**: `2026-0711-1441tc-progressive-plan-mode`
**Created**: 2026-07-11
**Status**: Draft
**Input**: `.codexspec/specs/2026-0711-1441tc-progressive-plan-mode/requirements.md`

## Context

FoxHarness currently treats Plan Mode as an automatic pre-execution model pass. When enabled, the legacy Planner generates complete `PLAN.md` and `TODO.md` files before the main Agent runs, then the Agent immediately implements the task. That behavior applies implicitly to normal TUI and non-interactive runs and is also duplicated in AgentOps and benchmark entry points.

This feature replaces that overloaded behavior with two distinct workflows:

- Default mode remains the normal execution mode. The Agent may use the existing `update_todo` tool when an execution checklist improves a multi-step task, but doing so never changes the collaboration mode.
- Formal Plan mode is a user-selected, read-only planning workflow. The Agent explores, clarifies, submits a complete proposal through `submit_plan`, and waits for user approval before implementation.

Formal Plan mode intentionally uses high-priority instructions and a reduced model-visible tool surface rather than a new sandbox or command approval subsystem. Its read-only property is therefore a behavioral contract, not a security isolation guarantee.

## Goals

- Make Formal Plan mode an explicit user choice rather than an automatic task pre-pass.
- Let users review, request changes to, and approve a complete implementation proposal before work begins.
- Continue the same task and conversation when an approved plan transitions back to Default mode.
- Derive an actionable `TODO.md` checklist from the approved plan before implementation starts.
- Remove the legacy automatic Planner consistently across interactive, non-interactive, AgentOps, and benchmark entry points.
- Preserve FoxHarness session plan/TODO storage, state snapshots, `/rewind`, Feishu behavior, and the independent Autodev SDD workflow.

## User Scenarios & Testing

### User Story 1 - Explicitly Plan Before Implementation (Priority: P1)

As a TUI user, I want to enter Formal Plan mode explicitly and receive a complete proposal before any implementation begins, so that I can validate the approach for complex or high-risk work.

**Why this priority**: This is the primary user value and defines the new meaning of Formal Plan mode.

**Independent Test**: Start the TUI in Default mode, enter Formal Plan mode with either supported control, submit a task, and verify the Agent receives planning instructions and only the Formal Plan tool surface before presenting a proposal.

**Acceptance Scenarios**:

1. **Given** a newly started or resumed TUI session, **When** no mode control has been used, **Then** the active collaboration mode is Default.
2. **Given** the TUI input is active, **When** the user presses `Shift+Tab` or invokes `/plan`, **Then** the next submitted task runs in Formal Plan mode.
3. **Given** Formal Plan mode is active, **When** the Agent begins the task, **Then** it receives high-priority read-only planning instructions and cannot see `write_file`, `edit_file`, or `update_todo`.
4. **Given** Formal Plan mode is active, **When** the Agent has completed exploration and clarification, **Then** it submits one complete Markdown proposal through `submit_plan` rather than implementing the task.

### User Story 2 - Review, Revise, and Approve a Plan (Priority: P1)

As a TUI user, I want to review the submitted proposal, provide revision feedback, and approve the final version, so that implementation follows an approach I understand and accept.

**Why this priority**: Formal planning has no value unless the user controls the transition from proposal to implementation.

**Independent Test**: Submit a proposal, request a revision with feedback, verify the replacement proposal, then approve it and confirm the same task continues in Default mode.

**Acceptance Scenarios**:

1. **Given** the Agent calls `submit_plan`, **When** the runtime accepts the submission, **Then** the exact proposal is persisted as the latest session-local `PLAN.md` and displayed for confirmation.
2. **Given** a proposal is awaiting confirmation, **When** the user requests changes and supplies feedback, **Then** the session remains in Formal Plan mode and the feedback is returned to the Agent.
3. **Given** revision feedback was returned, **When** the Agent submits a complete revised proposal, **Then** it replaces the prior `PLAN.md` proposal and the confirmation interface reopens with the revised content.
4. **Given** a proposal is awaiting confirmation, **When** the user approves it, **Then** the session returns to Default mode and continues the same task in the same conversation context.
5. **Given** a proposal is awaiting confirmation, **When** the user chooses to continue planning, **Then** implementation does not begin and Formal Plan mode remains active.

### User Story 3 - Execute the Approved Plan With a Checklist (Priority: P1)

As a user who approved a plan, I want the Agent to turn that plan into executable TODO items before changing the project, so that implementation progress remains ordered, visible, and verifiable.

**Why this priority**: This preserves execution accuracy after the formal approval checkpoint and reuses FoxHarness's existing checklist workflow.

**Independent Test**: Approve a submitted plan and verify the Default-mode continuation includes the approved plan, calls `update_todo` before its first implementation action, and writes checklist content derived from the approved proposal.

**Acceptance Scenarios**:

1. **Given** the user approved a plan, **When** the runtime starts the Default-mode continuation, **Then** the complete approved plan is present in the continuation context for the same task.
2. **Given** the approved plan is in context, **When** the Agent prepares to implement it, **Then** the Agent semantically decomposes the plan into ordered, executable, and verifiable items and calls `update_todo` before its first implementation action.
3. **Given** `update_todo` succeeds, **When** implementation proceeds, **Then** session-local `TODO.md` contains the execution checklist while `PLAN.md` remains the submitted plan artifact.
4. **Given** the Agent only performs read-only revalidation after approval, **When** `TODO.md` has not yet been initialized, **Then** that revalidation is permitted but implementation actions remain deferred until `update_todo` succeeds.

### User Story 4 - Use Default Mode Without Hidden Pre-Planning (Priority: P2)

As a FoxHarness user across supported entry points, I want normal tasks to start directly in Default mode without an automatic extra Planner call, so that simple tasks remain fluid and every use of Formal Plan has one consistent meaning.

**Why this priority**: Removing the legacy behavior is necessary for consistency, but the primary user journey remains the interactive planning and approval flow.

**Independent Test**: Exercise TUI, `fox exec`, `fox -p`, AgentOps, benchmark, Feishu, and Autodev entry points and verify each follows its specified migration behavior without invoking the legacy Planner.

**Acceptance Scenarios**:

1. **Given** a TUI, `fox exec`, or `fox -p` task, **When** the task begins, **Then** no legacy Planner pre-pass runs.
2. **Given** an AgentOps or benchmark task, **When** the task begins, **Then** no legacy Planner pre-pass runs and `update_todo` is available to maintain an execution checklist.
3. **Given** a Feishu task, **When** the task begins, **Then** its existing no-preplan behavior is unchanged.
4. **Given** an Autodev task, **When** the task begins, **Then** its independent SDD workflow is unchanged.
5. **Given** any FoxHarness invocation, **When** the user supplies the removed `-plan` flag, **Then** argument parsing reports it as unsupported rather than silently changing behavior.

### Edge Cases

- Repeating `/plan` while already in Formal Plan mode leaves the mode unchanged.
- Invoking `/plan off` while already in Default mode leaves the mode unchanged.
- A `Shift+Tab`, `/plan`, or `/plan off` mode change requested during an active run affects the next user submission and does not alter the active Agent run.
- `Esc` retains existing FoxHarness behavior and never serves as a Formal Plan exit control.
- Empty or whitespace-only `submit_plan` input is rejected and does not open confirmation or replace the existing proposal.
- A failed `PLAN.md` write leaves Formal Plan mode active, returns a clear tool error, and does not present an unpersisted proposal for approval.
- Revision feedback does not mutate `PLAN.md` by itself; only a successful complete `submit_plan` replacement changes the proposal artifact.
- If `update_todo` fails after approval, the error is returned to the Agent and implementation actions remain deferred until checklist initialization succeeds.
- Read-only Bash exploration is permitted by instruction, but no sandbox or command classifier guarantees that a shell command is non-mutating.
- A file-based slash command that would execute embedded shell, `hooks.before`, or `hooks.after` outside the agent tool registry is rejected before preparation in Formal Plan mode; side-effect-free inline prompt commands remain available.

## Requirements

### Functional Requirements

- **REQ-001**: Every interactive TUI session, including a resumed session, MUST begin in Default collaboration mode. Formal Plan mode MUST only be entered by an explicit user action, and `update_todo` MUST NOT change collaboration mode.
  - Sources: CON-001, DEC-007, DEC-009

- **REQ-002**: With the primary TUI input active, `Shift+Tab` MUST switch between Default and Formal Plan modes. `/plan` MUST enter Formal Plan mode idempotently, `/plan off` MUST return to Default mode idempotently, and `Esc` MUST retain its existing FoxHarness semantics without exiting Formal Plan mode.
  - Sources: DEC-007

- **REQ-003**: A mode change requested during an active run MUST be stored for the next user submission and MUST NOT alter the collaboration mode or tool surface of the active run. Approval of a submitted plan is the exception that starts the confirmed same-task Default continuation.
  - Sources: DEC-005, DEC-007

- **REQ-004**: While Formal Plan mode is active, the Agent Engine MUST receive high-priority instructions that prohibit creating or modifying project files, changing Git state, and running side-effectful actions whose purpose is to implement the solution. The instructions MUST permit read-only repository, Git, system, environment, and feasibility exploration through Bash.
  - Sources: CON-002, DEC-001

- **REQ-005**: The Formal Plan model-visible tool surface MUST contain `read_file`, `bash`, `ask_user_question`, and `submit_plan`, and MUST exclude `write_file`, `edit_file`, and `update_todo`. No generic or plan-specific incremental plan editing tool may be exposed.
  - Sources: CON-002, DEC-002, DEC-003, OUT-002

- **REQ-006**: `submit_plan` MUST accept one complete Markdown proposal as `plan_markdown`, reject empty content, persist a successful submission as the latest session-local `PLAN.md`, and present that same content in the plan confirmation interface. The runtime-owned write performed by `submit_plan` is the only Formal Plan file-write exception.
  - Sources: NEED-001, DEC-003, OUT-002

- **REQ-007**: Requesting plan changes MUST keep the session in Formal Plan mode and return the user's feedback to the Agent. A subsequent successful `submit_plan` MUST replace the entire prior `PLAN.md` proposal and reopen confirmation; feedback alone MUST NOT mutate the artifact.
  - Sources: NEED-001, DEC-004, OUT-002

- **REQ-008**: Approving a proposal MUST switch the session to Default mode and continue the same task in the same conversation context without requiring a new user prompt. Choosing to continue planning MUST leave Formal Plan mode active and MUST NOT begin implementation.
  - Sources: NEED-001, DEC-005, DEC-007

- **REQ-009**: The Default-mode continuation after approval MUST include the complete approved plan, and the runtime MUST keep the complete plan available on every checklist-gate model call until `update_todo` succeeds, including after compaction. Before its first implementation action, the Agent MUST derive ordered, executable, and verifiable checklist items from that plan and successfully call `update_todo` to replace session-local `TODO.md`. Read-only revalidation MAY precede the checklist update.
  - Sources: DEC-005, DEC-006, DEC-008

- **REQ-010**: `update_todo` MUST remain the sole execution checklist tool and MUST manage only session-local `TODO.md`. The feature MUST NOT introduce `update_plan`; `submit_plan` MUST manage only the session-local `PLAN.md` proposal.
  - Sources: DEC-003, DEC-006, DEC-008

- **REQ-011**: The legacy `-plan` CLI flag MUST be removed without a deprecation or no-op compatibility path. `fox exec` and `fox -p` MUST run in Default mode and MUST NOT expose a Formal Plan startup option. This feature MUST NOT introduce a generic permission-mode startup flag.
  - Sources: CON-001, DEC-009

- **REQ-012**: Automatic `Planner.BuildPlan` pre-passes MUST be removed from the shared `AgentRunner`, AgentOps, and benchmark entry points. The obsolete legacy Planner implementation and tests, `EnablePlanMode` configuration fields, and related getters and setters MUST be removed. The new Formal Plan state MUST use collaboration-mode state rather than the legacy boolean.
  - Sources: CON-001, DEC-009, DEC-010

- **REQ-013**: Benchmark runs MUST expose `read_todo` and `update_todo`; Feishu MUST preserve its current no-preplan behavior; and Autodev MUST preserve its independent SDD workflow.
  - Sources: DEC-010

- **REQ-014**: Existing `memory.Store` support for session-local `PLAN.md` and `TODO.md`, session state snapshots, restoration behavior, and `/rewind` compatibility MUST remain available after the legacy Planner is removed.
  - Sources: DEC-003, DEC-006, DEC-010

### Non-Functional Requirements

- **NFR-001**: Formal Plan read-only behavior MUST be documented and tested as an instruction-and-tool-surface contract, not represented as an OS-level security guarantee.
  - Sources: CON-002, DEC-001, OUT-001

- **NFR-002**: The persisted `PLAN.md` proposal and the proposal displayed for confirmation MUST originate from the same successful `submit_plan` payload so that approval never targets content different from the stored artifact.
  - Sources: NEED-001, DEC-003, DEC-004

- **NFR-003**: Default-mode execution MUST NOT incur the legacy Planner's additional model request before the primary Agent run.
  - Sources: DEC-009, DEC-010

- **NFR-004**: All new and changed code MUST follow the project constitution's Red-Green-Refactor TDD workflow, including characterization tests before modifying legacy behavior and deterministic coverage of error paths and mode transitions.
  - Sources: DEC-007, DEC-010, Project Constitution 2.0.0

- **NFR-005**: Mode selection, tool filtering, proposal submission, revision, approval continuation, and TODO initialization MUST be testable with deterministic fake providers and runtime collaborators; normal acceptance coverage MUST NOT require a live model or external service.
  - Sources: CON-002, DEC-002, DEC-003, DEC-005, DEC-008

### Key Entities

- **Collaboration Mode**: The session's current or next-submission mode. This feature defines `Default` and `Formal Plan` behavior while leaving broader permission modes out of scope.
- **Plan Proposal**: Complete Markdown submitted by the Agent through `submit_plan` and persisted as the latest session-local `PLAN.md`, whether awaiting approval or submitted again after feedback.
- **Plan Confirmation**: The interactive TUI decision point that either approves the current proposal and continues the same task in Default mode or returns feedback and keeps Formal Plan mode active.
- **Execution Checklist**: Ordered, executable, and verifiable work items derived by the Agent from an approved plan and persisted through `update_todo` as session-local `TODO.md`.
- **Deferred Mode Selection**: A mode change requested during an active run and applied to the next user submission without changing the active run.

## Acceptance Criteria and Expected Error Behavior

- TUI startup and resume select Default mode without consulting a removed `-plan` value.
- `/plan`, `/plan off`, and primary-input `Shift+Tab` produce the confirmed idempotent mode transitions; existing `Esc` behavior remains unchanged.
- Active-run mode changes update only the next-submission selection and leave the current engine context and tool registry unchanged.
- Formal Plan runs receive the confirmed instruction block and reduced tool registry. Attempts to request removed tools cannot execute because those tools are absent from the model-visible registry.
- `submit_plan` with empty or whitespace-only Markdown returns a clear validation error, remains in Formal Plan mode, preserves any prior proposal, and does not open confirmation.
- If persistence of a submitted proposal fails, the tool returns a clear error, remains in Formal Plan mode, preserves the last successfully stored proposal, and does not open confirmation for the failed submission.
- The confirmation interface displays exactly the latest successfully persisted proposal.
- Revision feedback is returned to the same planning conversation; a successful full resubmission replaces the prior proposal before confirmation reopens.
- Approval starts a Default-mode continuation for the same task with the complete approved plan in context. It does not require the user to repeat the task.
- Before project file edits, Git state changes, or other implementation actions in that continuation, the Agent successfully replaces `TODO.md` through `update_todo` with items semantically derived from the approved plan.
- If `update_todo` fails, the Agent receives the failure and implementation remains deferred; read-only revalidation is still permitted.
- `fox exec`, `fox -p`, AgentOps, benchmark, and Feishu do not invoke the legacy Planner. Benchmark exposes TODO tools; Autodev retains its existing SDD behavior.
- Passing `-plan` fails argument parsing as an unsupported option; it is not accepted as an alias or ignored compatibility flag.
- `/rewind` continues to restore session-local `PLAN.md` and `TODO.md` snapshots after the legacy Planner implementation is removed.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Deterministic TUI tests cover every confirmed Default/Formal Plan transition, including idempotent commands, active-run deferral, approval, continued planning, and unchanged `Esc` behavior.
- **SC-002**: Deterministic tool-registry tests verify that all four required Formal Plan tools are visible and that `write_file`, `edit_file`, and `update_todo` are absent.
- **SC-003**: Submission and revision tests verify byte-for-byte agreement between the latest successful `submit_plan` payload, persisted `PLAN.md`, and displayed confirmation content.
- **SC-004**: Approval-continuation tests verify that the complete approved plan remains available across repeated checklist-gate turns and that `update_todo` succeeds with checklist content derived from that plan before the first implementation tool call.
- **SC-005**: Repository tests and static searches find no production invocation of the legacy `Planner.BuildPlan`, no `EnablePlanMode` field or accessor, and no registered `-plan` flag.
- **SC-006**: Existing session storage, snapshot, `/rewind`, Feishu, and Autodev regression tests continue to pass.

## Confirmed Constraints and Decisions

- Formal Plan is user-selected and never entered by `update_todo` or an Agent difficulty classifier.
- Read-only behavior relies on injected instructions plus removal of explicit write tools; Bash remains available for instructed read-only exploration.
- `submit_plan` is the only plan proposal submission mechanism and always submits a complete replacement.
- `update_todo` remains the only execution checklist mechanism; no `update_plan` tool is added.
- Approval returns to Default and continues the same task; revision feedback remains in Formal Plan.
- TUI starts in Default and exposes no startup Plan flag.
- Legacy automatic planning is removed consistently rather than retained for selected entry points.

## Out of Scope

- OS-level sandboxing for Agent or Bash execution.
  - Sources: OUT-001
- Command-by-command or tool-call approval infrastructure. Plan proposal confirmation remains required and is not excluded.
  - Sources: OUT-001
- `edit_plan`, incremental Agent editing of `PLAN.md`, or direct user editing through `$EDITOR`.
  - Sources: OUT-002
- A new `update_plan` tool.
  - Sources: DEC-006
- A generic `--permission-mode` or replacement Formal Plan startup flag.
  - Sources: DEC-009
- Formal Plan approval in `fox exec`, `fox -p`, AgentOps, benchmark, or Feishu.
  - Sources: DEC-009, DEC-010
- Changes to the independent Autodev SDD lifecycle.
  - Sources: DEC-010

## Assumptions

- The primary-input `Shift+Tab` requirement does not replace context-specific key handling while the sidebar or another focused overlay owns the key; those existing local interactions remain unchanged unless implementation evidence shows they prevent the confirmed main-input mode switch.
- The runtime may read or carry the submitted proposal to build the Default continuation, but it does not deterministically parse Markdown into TODO items; semantic decomposition belongs to the Agent.
- `PLAN.md` represents the latest successfully submitted proposal. Approval state is represented by the runtime interaction lifecycle rather than inferred from the file contents.

## Dependencies

- Existing Agent Engine prompt composition and tool registry filtering.
- Existing TUI input handling, slash command dispatch, status presentation, and ask-user interaction infrastructure.
- Existing session-local `memory.Store`, `PLAN.md`, `TODO.md`, state-history snapshots, and `/rewind` behavior.
- Existing `update_todo`, `read_todo`, session continuation, transcript, and reporter infrastructure.
- Existing fake providers, model tests, runner tests, and full `go test ./...` verification path required by the project constitution.

## Open Questions

No blocking open questions remain. `OPEN-001`, `OPEN-002`, and `OPEN-003` in `requirements.md` are resolved by confirmed decisions.

## Requirements Traceability

| Confirmed Entry | Spec Coverage | Notes |
|-----------------|---------------|-------|
| NEED-001 | User Stories 1-3, REQ-006, REQ-007, REQ-008, NFR-002 | Reviewable proposal and approval gate fully covered. |
| CON-001 | REQ-001, REQ-011, REQ-012 | Formal Plan is explicit and Default remains the normal mode. |
| CON-002 | Context, REQ-004, REQ-005, NFR-001, NFR-005 | Behavioral read-only boundary preserved without a security guarantee. |
| DEC-001 | REQ-004, NFR-001 | Injected planning restrictions and allowed exploration covered. |
| DEC-002 | REQ-005, NFR-005 | Formal Plan tool surface covered. |
| DEC-003 | REQ-005, REQ-006, REQ-010, REQ-014, NFR-002 | Dedicated complete proposal submission and persistence covered. |
| DEC-004 | REQ-007, NFR-002 | Feedback and full replacement workflow covered. |
| DEC-005 | REQ-003, REQ-008, REQ-009, NFR-005 | Same-task Default continuation covered. |
| DEC-006 | REQ-009, REQ-010, REQ-014 | Existing `update_todo` retained with distinct artifact ownership. |
| DEC-007 | REQ-001, REQ-002, REQ-003, REQ-008, NFR-004 | Explicit TUI controls, active-run behavior, Esc, and approval transition covered. |
| DEC-008 | REQ-009, REQ-010, NFR-005 | Agent-derived TODO initialization from approved plan covered. |
| DEC-009 | REQ-001, REQ-011, REQ-012, NFR-003 | Legacy flag removal and Default-only non-interactive behavior covered. |
| DEC-010 | REQ-012, REQ-013, REQ-014, NFR-003, NFR-004 | Unified Planner migration and preserved infrastructure covered. |
| OUT-001 | NFR-001, Out of Scope | Sandbox and command/tool approval excluded. |
| OUT-002 | REQ-005, REQ-006, REQ-007, Out of Scope | Incremental and direct plan editing excluded. |
