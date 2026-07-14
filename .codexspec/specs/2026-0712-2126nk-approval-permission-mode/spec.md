# Feature Specification: Approval Permission Mode

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Feature ID**: `2026-0712-2126nk`
**Feature Branch**: `2026-0712-2126nk-approval-permission-mode`
**Created**: 2026-07-13
**Status**: Draft
**Input**: `.codexspec/specs/2026-0712-2126nk-approval-permission-mode/requirements.md`

## Context

FoxHarness currently executes interactive TUI tool calls without a unified user-selectable approval policy. This feature adds a tool-level permission boundary that can allow known-safe work, request user approval, or use an isolated LLM reviewer before escalating to the user. The feature applies only to the interactive TUI and its nested execution paths. It does not provide OS-level process, filesystem, or network isolation.

The first version must keep routine work low-noise while making consequential decisions visible and revocable. All behavior in this specification is defined independently for FoxHarness and does not depend on external source code for interpretation.

## Goals

- Provide `/permissions` as the canonical TUI command for selecting `Ask for approval`, `Approve for me`, or `Full Access`.
- Apply one consistent tool-level permission boundary across main-agent, delegated, skill, and nested execution paths.
- Automatically pass only deterministically safe calls and narrowly authorized calls approved by an isolated reviewer.
- Preserve explicit user control over escalation, denial, session grants, cancellation, and Full Access activation.
- Persist permission preferences without persisting session-scoped grants.
- Present review activity and dangerous modes without duplicating routine tool output.

## User Scenarios & Testing

### User Story 1 - Select A Predictable Permission Mode (Priority: P1)

As an interactive TUI user, I want to choose how FoxHarness handles tool calls, so that I can balance interruption frequency and control without changing the underlying tool boundary accidentally.

**Why this priority**: Mode selection defines the behavior of every later approval decision.

**Independent Test**: Open `/permissions`, select each mode, issue equivalent tool calls, and verify that `Ask for approval` and `Approve for me` enforce the same boundary while routing review differently and `Full Access` bypasses only the approval gate.

**Acceptance Scenarios**:

1. **Given** no valid permission setting exists, **When** the TUI starts, **Then** the effective mode is `Ask for approval`.
2. **Given** the user selects `Ask for approval`, **When** a call requires review, **Then** FoxHarness presents the user approval prompt directly.
3. **Given** the user selects `Approve for me`, **When** a call requires review, **Then** FoxHarness invokes the isolated LLM reviewer before deciding whether to escalate.
4. **Given** the user selects `Full Access`, **When** a tool call is dispatched, **Then** the approval gate is bypassed but argument validation, Plan mode restrictions, and tool-specific hard constraints still apply.

---

### User Story 2 - Approve Or Redirect A Tool Call (Priority: P1)

As a user facing a reviewed tool call, I want clear one-time, session, denial, and feedback choices, so that I can authorize the exact effect I understand or redirect the agent safely.

**Why this priority**: Explicit user authority is the fallback for every request that is not deterministically or automatically approved.

**Independent Test**: Trigger four equivalent reviewed calls and select each prompt decision once, then verify the current call, later calls, queue, and agent turn are affected exactly as specified.

**Acceptance Scenarios**:

1. **Given** a call is awaiting user approval, **When** the user selects `Yes, proceed`, **Then** only that exact call is authorized and the queue advances.
2. **Given** a call is awaiting user approval, **When** the user selects `Yes, allow for this session`, **Then** the current call runs and later equivalent calls may match the new typed session grant.
3. **Given** a call is awaiting user approval, **When** the user selects `No, continue without running it`, **Then** only the current call is denied, a structured denied result is returned to the agent, and queued calls continue.
4. **Given** a call is awaiting user approval, **When** the user selects `No, and tell Fox what to do differently`, **Then** the current turn ends, unstarted queued calls are cancelled, and the feedback becomes the next user input.

---

### User Story 3 - Use Automatic Review Without Losing Control (Priority: P1)

As an `Approve for me` user, I want safe and sufficiently authorized work to proceed automatically while uncertain or consequential work escalates, so that routine execution remains fluid without allowing the agent to approve its own actions.

**Why this priority**: Automatic review is the defining behavior of `Approve for me`.

**Independent Test**: Submit low-, medium-, high-, critical-, malformed-, and insufficient-context requests through an isolated fake reviewer and verify approval, escalation, retries, timeout, and user fallback behavior.

**Acceptance Scenarios**:

1. **Given** a call satisfies a deterministic fast path, **When** it is dispatched, **Then** it runs without LLM review or reviewer-specific transcript annotation.
2. **Given** a non-fast-path call is low or medium risk, task-relevant, and narrowly scoped, **When** the reviewer finds sufficient context, **Then** it may approve only that exact invocation.
3. **Given** a high-risk call, **When** material user authorization or narrow scope is missing, **Then** the reviewer escalates it to the user.
4. **Given** a critical, suspicious, unrelated, unclear, or insufficiently contextualized call, **When** review completes, **Then** the reviewer escalates and never automatically denies it.
5. **Given** review fails technically or returns invalid structured output, **When** retry capacity remains, **Then** FoxHarness retries within the shared budget and falls back to user approval only after exhaustion.

---

### User Story 4 - Preserve Approval Across Nested And Queued Work (Priority: P1)

As a user, I want delegated and concurrent-looking tool activity to honor one ordered approval policy, so that nested execution cannot bypass my mode or act on stale approval state.

**Why this priority**: A permission UI is ineffective if delegated or queued paths can evade it.

**Independent Test**: Produce a model response containing main-agent, delegated, skill-originated, fast-path, granted, and reviewed calls, then verify inherited policy, FIFO review order, grant re-evaluation, cancellation, and non-rollback behavior.

**Acceptance Scenarios**:

1. **Given** a Subagent, Skill, or nested call originates from the interactive TUI, **When** it reaches tool dispatch, **Then** it uses the same effective mode, session grants, and approval coordinator.
2. **Given** multiple calls require review, **When** they are received in model order, **Then** only one review or user prompt is active and reviewed calls proceed in FIFO order.
3. **Given** a queued call has not started, **When** the mode or session grants change, **Then** the call is re-evaluated before review or execution.
4. **Given** earlier calls completed before a denial or cancellation, **When** the current turn is stopped, **Then** completed effects are not rolled back by the approval coordinator.

---

### User Story 5 - Inspect And Revoke Temporary Authority (Priority: P2)

As a user, I want to see the current mode and temporary grant count and clear all grants, so that session-scoped authority remains understandable and revocable.

**Why this priority**: Session grants survive several conversation operations and therefore need a visible revocation path.

**Independent Test**: Create session grants, inspect `/status` and the optional statusline item, run `/rewind`, `/compact`, mode changes, and `Clear session approvals`, then verify visibility and lifecycle.

**Acceptance Scenarios**:

1. **Given** session grants exist, **When** `/permissions` opens, **Then** `Clear session approvals` is enabled and shows the number of grants that will be removed.
2. **Given** the user clears session approvals, **When** queued calls are reconsidered, **Then** all in-memory grants are absent and unstarted calls are re-evaluated.
3. **Given** the user runs `/rewind`, `/compact`, or changes permission mode, **When** the operation completes, **Then** session grants remain active.
4. **Given** the user runs `/new`, `/clear`, or exits the TUI, **When** the session or process ends, **Then** all session grants are cleared.

---

### User Story 6 - Enter Full Access Deliberately (Priority: P2)

As a user selecting `Full Access`, I want a clear warning and predictable restart behavior, so that the TUI never silently disables approval interception without remembered consent.

**Why this priority**: Full Access removes the approval gate and therefore needs an independent activation guardrail.

**Independent Test**: Select Full Access with ordinary confirmation and remembered confirmation in separate runs, restart the TUI, switch away and back, and verify selected mode, effective mode, acknowledgment, and warning behavior.

**Acceptance Scenarios**:

1. **Given** the warning is not remembered, **When** the user ordinarily confirms Full Access, **Then** Full Access becomes effective for the current run, the selected mode remains persisted, and the warning acknowledgment remains unset.
2. **Given** Full Access is persisted without acknowledgment, **When** a later TUI run starts, **Then** the effective mode begins as `Ask for approval` and the Full Access warning is shown immediately.
3. **Given** the user selects `Enable and remember`, **When** a later TUI run starts, **Then** Full Access may become effective directly from the persisted mode and acknowledgment.
4. **Given** the warning acknowledgment is remembered, **When** the user switches to another mode, **Then** the selected mode changes but the separate acknowledgment remains unchanged.

### Edge Cases

- Missing, unknown, or invalid persisted modes resolve to `Ask for approval`.
- A persisted Full Access selection without remembered acknowledgment is not effective until the startup warning is confirmed.
- Unparseable shell syntax, unsupported constructs, unknown commands, ambiguous flags, unresolved paths, and paths that cannot be proven workspace-contained miss the fast path and proceed to review rather than being denied automatically.
- Every command in a shell chain or pipeline must independently satisfy the fast path; one unsafe or uncertain atomic command sends the whole call to review.
- Symlink-aware path normalization must prevent a workspace-looking operand from escaping the active workspace.
- A valid reviewer `escalate` result is not a retryable failure.
- Reviewer cancellation ends the current turn and all unstarted queued calls without opening a fallback approval prompt.
- Clearing session grants does not interrupt a call that has already started.
- Fast-path and already-granted calls bypass the review queue, while calls awaiting review remain serial.
- Tool calls completed before denial, feedback, or cancellation are not rolled back.

## Requirements

### Functional Requirements

- **REQ-001**: The interactive TUI MUST expose `/permissions` with exactly three permission modes: `Ask for approval`, `Approve for me`, and `Full Access`. A missing, unknown, or invalid stored mode MUST resolve to `Ask for approval`.
  - Sources: NEED-001, CON-001, DEC-003, DEC-019, DEC-024

- **REQ-002**: A mode selected through `/permissions` MUST be persisted in `~/.foxharness/settings.json` and restored for later interactive TUI runs. The persisted selection MUST NOT affect non-interactive entry points.
  - Sources: CON-003, DEC-006, DEC-019

- **REQ-003**: Full Access activation MUST use a warning acknowledgment separate from the selected mode. The acknowledgment MUST be an independently resettable settings field. Ordinary confirmation MUST activate Full Access only for the current run without persisting acknowledgment. `Enable and remember` MUST persist acknowledgment. A later startup with selected Full Access but no acknowledgment MUST begin effectively in Ask mode and show the warning before activation. Switching modes MUST NOT clear remembered acknowledgment.
  - Sources: DEC-023

- **REQ-004**: Ask and Approve modes MUST enforce the same tool boundary. Trusted interaction and session-state tools, including `ask_user_question`, `read_todo`, `update_todo`, and `submit_plan`, MUST bypass the approval gate while retaining their own rules. `read_file`, `write_file`, and `edit_file` MUST bypass the gate when their normalized, symlink-aware targets remain inside the active workspace and MUST require review when their targets are outside it. Non-fast-path Bash, skills, delegated tasks, and unknown tools MUST require review. Full Access MUST bypass only this approval gate.
  - Sources: CON-002, DEC-009, DEC-010

- **REQ-005**: Main-agent, delegated, Skill-originated, forked, embedded-shell, and nested tool calls created by the interactive TUI MUST inherit one effective permission mode, approval coordinator, and session-grant set. No nested execution path may bypass the active policy.
  - Sources: CON-004, DEC-010, DEC-015, DEC-019

- **REQ-006**: Bash fast-path classification MUST use structured shell parsing and fail closed to review. Unsupported or unparseable syntax, unknown commands, ambiguous flags, unsafe constructs, unresolved paths, and paths not proven workspace-contained MUST miss the fast path without being automatically denied.
  - Sources: CON-005, DEC-010, DEC-016

- **REQ-007**: The deterministic Bash fast path MUST enforce the confirmed command, option, grammar, and path boundary:
  - Path-free query commands: `pwd`, `whoami`, `id`, `uname`, `which`, `true`, and `false`.
  - Content and text commands: `cat`, `cut`, `echo`, `expr`, `grep`, `head`, `ls`, `nl`, `paste`, `rev`, `seq`, `stat`, `tail`, `tr`, `uniq`, and `wc`.
  - `rg` MUST reject options that execute external commands or archive helpers, including `--pre`, `--hostname-bin`, `--search-zip`, and equivalent short forms.
  - `find` MUST reject execution, deletion, confirmation, and file-output options, including `-exec`, `-execdir`, `-ok`, `-okdir`, `-delete`, `-fls`, `-fprint`, `-fprint0`, and `-fprintf`.
  - `sed` MUST allow only `-n` print expressions containing one line number or one numeric line range.
  - Git MUST be limited to `status`, `log`, `diff`, `show`, and read-only `branch` forms and MUST reject options that redirect execution, configuration, repository location, helpers, or output, including `-C`, `-c`, `--git-dir`, `--work-tree`, `--output`, `--ext-diff`, `--textconv`, and `--exec`.
  - `&&`, `||`, `;`, and pipelines MAY pass only when every atomic command passes independently.
  - Redirection, command or process substitution, subshells, background execution, heredocs, environment assignments, glob expansion, explicit executable paths, and unsupported syntax MUST miss the fast path.
  - Every explicit filesystem operand MUST resolve, including symlink handling, inside the active workspace. Commands without filesystem operands MUST NOT be required to invent a workspace operand.
  - Tests, builds, interpreters, package managers, network commands, and executable workloads such as `go test` and `go run` MUST NOT use the read-only fast path.
  - Sources: DEC-016

- **REQ-008**: Calls requiring review MUST route directly to the user in Ask mode. The prompt MUST support `Yes, proceed`, `Yes, allow for this session`, `No, continue without running it`, and `No, and tell Fox what to do differently`, with the decision effects defined by REQ-016.
  - Sources: DEC-005, DEC-009, DEC-010

- **REQ-009**: Approve mode MUST pass deterministic fast-path calls directly and send other reviewed calls to a separate LLM reviewer. The main agent MUST NOT review its own proposed call. Technical reviewer exhaustion MUST fall back to the user instead of failing the agent task.
  - Sources: DEC-008, DEC-009, DEC-024

- **REQ-010**: The LLM reviewer MUST reuse the provider and model active in the TUI at review time, including changes made through `/model`, but MUST run as an isolated, tool-free invocation without main-conversation state. The first version MUST NOT add separate reviewer provider, model, or credential settings.
  - Sources: DEC-013

- **REQ-011**: The reviewer MUST return structured `risk_level`, `user_authorization`, `decision`, and concise `rationale` fields. Risk values MUST be `low`, `medium`, `high`, or `critical`; authorization values MUST be `high`, `medium`, `low`, or `unknown`; decision MUST be `approve` or `escalate`. Approval authorizes only the exact current invocation. The reviewer MUST NOT deny calls, create grants, change modes, or modify policy.
  - Sources: DEC-011, DEC-017

- **REQ-012**: The reviewer MAY approve task-relevant, narrowly scoped low- and medium-risk actions. It MAY approve high-risk actions only with material user authorization, narrow target and blast radius, and no absolute escalation condition. It MUST escalate critical, suspicious, unrelated, insufficiently contextualized, unclear, or insufficiently authorized high-risk actions. User urgency MUST NOT increase authorization.
  - Sources: DEC-017

- **REQ-013**: Each review MUST receive a bounded, trust-aware transcript with separate message and tool-evidence budgets. It MUST include the exact canonical action and arguments, effective cwd and workspace, invocation source, and active permission boundary. Retention MUST prioritize the initial objective, latest user messages, explicit authorization, and recent execution context and MUST mark truncation explicitly.
  - Sources: CON-006, DEC-018

- **REQ-014**: Only direct user messages, applicable developer or project instructions, and direct answers from `ask_user_question` MAY establish authorization. Assistant text, proposed tool arguments, tool results, Skill content, and file content MUST be untrusted and MUST NOT independently broaden authorization. Explicit user instructions to follow an external source authorize only the stated scope. Insufficient trusted evidence MUST escalate.
  - Sources: CON-006, DEC-018

- **REQ-015**: A review MAY make at most three logical attempts, including the initial attempt, within one shared 90-second wall-clock budget. Provider transport retries MUST remain independent. A valid `approve` or `escalate` MUST stop retries. Technical errors, timeouts, and invalid output MAY retry; exhaustion MUST fall back to user approval. User cancellation MUST terminate review and the current turn, cancel every not-yet-executed queued call, and avoid fallback prompting.
  - Sources: DEC-011, DEC-012

- **REQ-016**: Calls requiring review MUST use one serial FIFO queue in model call order, with at most one active LLM review or user prompt. Fast-path and matching-grant calls bypass the review queue. Before a queued call is reviewed or executed, the coordinator MUST re-evaluate mode and grants. Allow-once affects only the current call; allow-for-session records a typed grant; deny-and-continue returns a structured denied result and advances the queue; feedback cancellation ends the turn and cancels unstarted calls. Completed calls MUST NOT be rolled back.
  - Sources: DEC-005, DEC-020

- **REQ-017**: Session approvals MUST use tool-specific keys: canonical Bash command plus effective cwd; read capability plus canonical external path; shared write/edit mutation capability plus canonical external path; canonical tool type plus normalized arguments for skills and delegation; and canonical tool name plus fully normalized arguments for unknown tools. Grants MUST remain in memory only and MUST NOT become prefix-wide, directory-wide, tool-wide, history-restored, or persistent permissions.
  - Sources: DEC-014

- **REQ-018**: Session grants MUST clear on `/new`, `/clear`, and TUI exit and MUST survive `/rewind`, `/compact`, and permission-mode changes. Main and delegated calls MUST share them. `/permissions` MUST include a separate `Clear session approvals` action that is enabled only when grants exist, shows the removal count, clears all grants, does not interrupt started calls, and causes queued calls to be re-evaluated.
  - Sources: DEC-015, DEC-021

- **REQ-019**: LLM review MUST show transient `Reviewing <tool>...` status and show the logical attempt number only after retry begins. Auto-approved calls MUST annotate the existing tool summary with `Auto-approved` and risk level, with rationale collapsed in details. Deterministic fast-path calls MUST have no reviewer-specific annotation. Reviewer escalation prompts MUST show the exact action, effective cwd, risk, rationale, and session-grant scope. Retry exhaustion MUST be disclosed before normal user choices.
  - Sources: NEED-002, DEC-022

- **REQ-020**: `/status` MUST show the effective permission mode and current session-grant count. `/statusline` MUST support an optional declarative `permissions` item without enabling it by default. Effective Full Access MUST always show a separate persistent `[full access]` warning near the bottom of the TUI regardless of statusline configuration.
  - Sources: NEED-002, DEC-022

- **REQ-021**: The permission mode, coordinator, queue, grants, and approval UI MUST apply only to the default interactive TUI and nested execution originating from it. `fox exec`, `fox -p`, autodev, Feishu, AgentOps, bench, and other non-interactive entry points MUST retain existing behavior and ignore the persisted TUI permission mode.
  - Sources: DEC-019

### Non-Functional Requirements

- **NFR-001**: The feature MUST describe and enforce a tool-policy boundary only and MUST NOT claim OS-level filesystem, process, or network isolation.
  - Sources: CON-002, DEC-002, DEC-009

- **NFR-002**: Approval enforcement MUST be consistent across nested execution so that adding delegation, skills, or unknown tools cannot create an unreviewed path by default.
  - Sources: CON-004, DEC-010

- **NFR-003**: Requirements, specification, plan, and task artifacts MUST describe FoxHarness behavior and implementation in self-contained terms and MUST NOT depend on external source paths, filenames, internal symbols, or source-derived implementation claims.
  - Sources: CON-007

- **NFR-004**: Review visibility MUST remain low-noise: normal fast-path calls keep normal rendering, automatic decisions reuse existing tool entries, and persistent warning treatment is reserved for effective Full Access.
  - Sources: NEED-002, DEC-022

### Key Entities

- **Selected Permission Mode**: The persisted user preference with one of `Ask for approval`, `Approve for me`, or `Full Access`.
- **Effective Permission Mode**: The mode currently enforced by the TUI. It may temporarily be Ask while an unacknowledged persisted Full Access selection awaits confirmation.
- **Full Access Warning Acknowledgment**: A separate, resettable persisted setting controlling whether later runs may activate selected Full Access without showing the warning.
- **Approval Request**: One canonical tool invocation with exact arguments, effective cwd, workspace, invocation source, and permission context.
- **Reviewer Result**: One structured risk, authorization, decision, and rationale result that applies only to the current approval request.
- **Session Approval Grant**: One in-memory, tool-specific authorization key created only by explicit user selection of `Yes, allow for this session`.
- **Approval Queue**: The serial FIFO sequence of calls that require review and have not yet started execution.

## Acceptance Criteria And Expected Error Behavior

- Selecting a mode must not be reported as persisted when the settings write fails; the TUI must surface the persistence error instead of silently claiming restart durability.
- Invalid or missing mode settings use Ask mode. Invalid settings must not activate Full Access.
- An unacknowledged Full Access selection starts effectively in Ask mode; dismissing or cancelling the warning leaves approval interception active.
- Fast-path classification uncertainty always routes to the active reviewer and never becomes an automatic allow or automatic deny.
- Reviewer provider errors, timeout, and invalid structured output retry only within the confirmed attempt and time limits. Exhaustion opens user approval and does not fail the task.
- A valid reviewer escalation opens user approval immediately without consuming another logical attempt.
- Ask-mode prompts show the exact action, effective cwd, and session-grant scope. When escalation follows LLM review, the prompt also shows the reviewer risk and rationale.
- Auto-approval applies only when the canonical action reviewed is identical to the action executed. Any argument, cwd, workspace, or invocation change requires re-evaluation.
- Denial without feedback affects only the current call. Feedback cancellation and reviewer cancellation stop the turn and cancel every unstarted reviewed call.
- Clearing grants or changing modes never interrupts a tool call that has started; all unstarted reviewed calls are re-evaluated before proceeding.
- Session grants cannot be restored from transcript, session history, settings, or project files.
- Non-interactive runtimes neither prompt for TUI approval nor receive broader authority from the persisted TUI mode.

## Success Criteria

- **SC-001**: Automated acceptance coverage passes for all three modes, missing and invalid settings, both Full Access confirmation paths, restart behavior, and mode changes.
- **SC-002**: Every tested unsupported shell construct, unsafe option, unresolved path, symlink escape, and non-allowlisted command misses the fast path and reaches review; no such case is automatically allowed.
- **SC-003**: Reviewer tests demonstrate exact-invocation approval, mandatory escalation conditions, no automatic denial, at most three logical attempts, one shared 90-second budget, and user fallback after exhaustion.
- **SC-004**: Queue tests demonstrate one active review surface, FIFO ordering for reviewed calls, grant and mode re-evaluation before execution, deterministic cancellation effects, and no rollback of completed calls.
- **SC-005**: Nested execution tests demonstrate that main-agent, delegated, Skill-originated, and unknown tool calls all use the same effective policy and cannot bypass review by changing invocation source.
- **SC-006**: TUI tests demonstrate low-noise review status, collapsed rationale, accurate mode and grant count, optional non-default statusline item, and an unconditional bottom warning whenever Full Access is effective.

## Confirmed Constraints And Decisions

- Ask and Approve modes share one tool boundary and differ only by reviewer.
- Full Access bypasses approval interception but not hard workflow or tool constraints.
- Deterministic classification failure routes to review rather than denial.
- The isolated reviewer can approve one exact call or escalate; durable authority and denial remain user decisions.
- Session grants are typed, in-memory, shared within the active interactive session, and bulk-revocable.
- Review is serial and non-transactional.
- Mode selection and Full Access warning acknowledgment are persisted separately.
- The feature is interactive-TUI-only and source-independent in all SDD artifacts.

## Out of Scope

- OS-level filesystem, process, and network sandboxing.
  - Sources: OUT-002, CON-002, DEC-002
- A user-visible `read-only` permission mode in the first version.
  - Sources: OUT-003, DEC-003
- Approval integration for non-interactive entry points.
  - Sources: OUT-004, DEC-019
- A general-purpose classifier-driven Auto mode, persistent allow/deny rule engine, or per-rule management interface.
  - Sources: OUT-005, DEC-021, DEC-024
- Persistent command-prefix, directory-wide, tool-wide, or history-restored grants.
  - Sources: DEC-014, DEC-015
- A separate reviewer provider, model, credential, or executable tool surface.
  - Sources: DEC-013

## Assumptions

No additional product assumptions are required. Planning may choose implementation structure only where it preserves every behavior and boundary above.

## Dependencies

- Existing interactive TUI slash-command, settings, status, statusline, and overlay capabilities.
- Existing tool-dispatch paths for main-agent, delegated, Skill-originated, and nested execution.
- Existing provider and active-model selection used by the interactive TUI.
- Existing session lifecycle for `/new`, `/clear`, `/rewind`, `/compact`, and process exit.
- Project constitution requirements, including test-first development and security review of security-sensitive behavior.

## Open Questions

No blocking open questions remain. `OPEN-001` through `OPEN-017` in `requirements.md` are resolved by confirmed entries.

## Requirements Traceability

| Confirmed Entry | Spec Coverage | Notes |
|-----------------|---------------|-------|
| NEED-001 | Context, Goals, User Stories 1-4, REQ-001, REQ-009 | Primary TUI approval goal and low-interruption execution covered. |
| NEED-002 | User Story 5, REQ-019, REQ-020, NFR-004, SC-006 | Review visibility and auditability covered. |
| CON-001 | REQ-001 | Canonical `/permissions` command covered. |
| CON-002 | REQ-004, NFR-001, Out of Scope | Tool-level boundary and sandbox exclusion covered. |
| CON-003 | REQ-002 | Settings persistence location covered. |
| CON-004 | User Story 4, REQ-005, NFR-002, SC-005 | Nested enforcement and inherited coordinator covered. |
| CON-005 | Edge Cases, REQ-006, REQ-007, SC-002 | Structured fail-closed Bash classification covered. |
| CON-006 | REQ-013, REQ-014 | Reviewer evidence trust boundary covered. |
| CON-007 | Context, NFR-003, Confirmed Constraints | Source-independent artifact constraint covered. |
| DEC-002 | NFR-001, Out of Scope | OS sandbox sequencing preserved. |
| DEC-003 | User Story 1, REQ-001, Out of Scope | Three modes and read-only exclusion covered. |
| DEC-005 | User Story 2, REQ-008, REQ-016 | Four approval choices and effects covered. |
| DEC-006 | REQ-002 | Selected mode persistence covered. |
| DEC-008 | User Story 3, REQ-009, REQ-015 | Deterministic and LLM review strategy covered. |
| DEC-009 | User Story 1, REQ-004, REQ-008, REQ-009, NFR-001 | Shared boundary and reviewer difference covered. |
| DEC-010 | REQ-004 through REQ-006, NFR-002 | Tool categories and hard constraints covered. |
| DEC-011 | REQ-011, REQ-015 | Reviewer authority and retryable-result distinction covered. |
| DEC-012 | User Story 3, REQ-015, SC-003 | Reliability-first retry budget covered. |
| DEC-013 | REQ-010, Out of Scope | Reviewer model reuse and isolation covered. |
| DEC-014 | REQ-017, Out of Scope | Typed in-memory grant keys and breadth limits covered. |
| DEC-015 | User Story 5, REQ-005, REQ-018, Out of Scope | Grant lifecycle and shared session state covered. |
| DEC-016 | Edge Cases, REQ-006, REQ-007, SC-002 | Conservative Bash fast path covered. |
| DEC-017 | User Story 3, REQ-011, REQ-012, SC-003 | Risk, authorization, and escalation matrix covered. |
| DEC-018 | REQ-013, REQ-014 | Bounded trust-aware reviewer context covered. |
| DEC-019 | REQ-001, REQ-002, REQ-005, REQ-021, Out of Scope | Interactive-only runtime scope and defaults covered. |
| DEC-020 | User Stories 2 and 4, REQ-016, SC-004 | Serial queue, cancellation, and non-rollback covered. |
| DEC-021 | User Story 5, REQ-018, Out of Scope | Bulk grant clearing and rule-editor exclusion covered. |
| DEC-022 | REQ-019, REQ-020, NFR-004, SC-006 | Low-noise review and status surfaces covered. |
| DEC-023 | User Story 6, REQ-003 | Full Access persistence and warning interaction covered. |
| DEC-024 | Goals, REQ-001, REQ-009, Out of Scope | Final first-version architecture and classifier exclusion covered. |
| OUT-002 | NFR-001, Out of Scope | OS sandbox excluded. |
| OUT-003 | REQ-001, Out of Scope | Read-only mode excluded. |
| OUT-004 | REQ-021, Out of Scope | Non-interactive integration excluded. |
| OUT-005 | Confirmed Constraints, Out of Scope | General classifier and persistent rule engine excluded. |
