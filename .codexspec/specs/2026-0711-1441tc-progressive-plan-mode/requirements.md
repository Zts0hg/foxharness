# Confirmed Requirements: progressive-plan-mode

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0711-1441tc`
**Status**: Discovery Complete - Requirements Confirmed
**Last Confirmed**: 2026-07-11

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: Reviewable formal planning workflow

- **Status**: confirmed
- **Statement**: Formal Plan mode MUST produce a reviewable implementation plan and MUST wait for explicit user approval before implementation begins.
- **Rationale**: Complex or high-risk work needs a deliberate planning checkpoint that reduces task misunderstanding and execution errors without forcing that workflow onto ordinary Default mode work.
- **User Evidence**: The user confirmed that `submit_plan` presents the generated plan for review and that implementation starts only after approval.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

## Constraints

### CON-001: Explicit Formal Plan activation

- **Status**: confirmed
- **Statement**: Formal Plan mode MUST only be entered through an explicit user action. Agent use of `update_todo` in Default mode MUST NOT change the collaboration mode.
- **User Evidence**: The user clarified that autonomous progress planning is not an automatic transition into Formal Plan mode.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

### CON-002: Behavioral read-only boundary

- **Status**: confirmed
- **Statement**: Formal Plan mode read-only behavior MUST be enforced primarily through high-priority injected instructions and a reduced tool surface. It is a behavioral contract, not a security isolation guarantee.
- **User Evidence**: The user chose not to introduce sandboxing or command approval and accepted instruction-driven read-only behavior.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

## Decisions

### DEC-001: Inject read-only planning instructions

- **Status**: confirmed
- **Decision**: While Formal Plan mode is active, the Agent Engine MUST receive high-priority instructions prohibiting project file creation or modification, Git state mutations, and side-effectful actions whose purpose is to implement the proposed solution. Read-only exploration through Bash remains permitted.
- **Alternatives Rejected**: OS-level sandbox enforcement and command-by-command approval in the initial scope.
- **Reason**: Preserve useful repository, Git, system, and environment inspection without adding a new security subsystem to this feature.
- **User Evidence**: The user requested instruction injection as the primary mechanism for keeping the Agent Engine read-only.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

### DEC-002: Filter the Formal Plan tool surface

- **Status**: confirmed
- **Decision**: Formal Plan mode MUST NOT expose `write_file`, `edit_file`, or `update_todo`. It MUST retain `read_file`, `bash`, `ask_user_question`, and the new `submit_plan` tool.
- **Alternatives Rejected**: Exposing explicit write tools and relying solely on the model instructions not to call them.
- **Reason**: Removing obvious mutation tools reduces accidental writes without requiring sandbox or approval infrastructure.
- **User Evidence**: The user adopted the recommendation to remove explicit file write tools while retaining Bash for read-only inspection.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

### DEC-003: Submit complete plans through a dedicated tool

- **Status**: confirmed
- **Decision**: Add `submit_plan(plan_markdown)` as the only Formal Plan submission tool. The runtime MUST store the complete submitted Markdown as the latest session-local `PLAN.md` proposal and present it in a plan confirmation interface.
- **Alternatives Rejected**: Letting the Agent incrementally edit `PLAN.md` through generic or plan-specific file editing tools.
- **Reason**: A dedicated submission boundary keeps the displayed proposal, persisted artifact, and approval request synchronized.
- **User Evidence**: The user confirmed that only a complete submission tool should be added and that no incremental plan editing tool is needed.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

### DEC-004: Revise plans through feedback and full resubmission

- **Status**: confirmed
- **Decision**: When the user requests changes, the session MUST remain in Formal Plan mode, return the user's feedback to the Agent, and require the Agent to call `submit_plan` again with the complete revised plan. The new submission MUST replace the previous `PLAN.md` proposal and reopen confirmation.
- **Alternatives Rejected**: Incremental Agent edits to `PLAN.md` and direct user editing through `$EDITOR` in the initial scope.
- **Reason**: Full resubmission avoids divergence between conversation context, the persisted plan, and the proposal shown for approval.
- **User Evidence**: The user confirmed the feedback-to-full-resubmission workflow and deferred direct editor support.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

### DEC-005: Continue the same task after approval

- **Status**: confirmed
- **Decision**: Approving the proposed plan MUST switch the session to Default mode and continue the same task in the same conversation context. Choosing to continue planning MUST keep the session in Formal Plan mode.
- **Alternatives Rejected**: Ending the task after approval or requiring the user to submit the implementation request again.
- **Reason**: Preserve context and make the plan-to-implementation transition uninterrupted.
- **User Evidence**: The user selected automatic return to Default mode with continuation of the same task.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

### DEC-006: Retain `update_todo` as the execution checklist tool

- **Status**: confirmed
- **Decision**: Retain `update_todo` as the only execution checklist tool. It MUST manage session-local `TODO.md`; no `update_plan` tool will be introduced. `submit_plan` remains responsible only for the session-local `PLAN.md` proposal.
- **Alternatives Rejected**: Adding a Codex-named `update_plan` tool with behavior overlapping the existing `update_todo` tool.
- **Reason**: Avoid duplicate tool responsibilities while preserving FoxHarness's existing persistent checklist workflow.
- **User Evidence**: The user questioned the duplicate tool and explicitly confirmed retaining `update_todo` instead.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

### DEC-007: Provide explicit Formal Plan mode controls

- **Status**: confirmed
- **Decision**: `Shift+Tab` MUST switch between Default and Formal Plan modes, preserving the existing FoxHarness interaction. `/plan` MUST enter Formal Plan mode idempotently, and `/plan off` MUST explicitly return to Default mode. A mode switch requested during an active run MUST apply to the next user submission. `Esc` MUST retain its existing FoxHarness semantics and MUST NOT exit Formal Plan mode. Plan approval MUST continue to return the session to Default mode automatically.
- **Alternatives Rejected**: Command-only mode switching, key-only mode switching, and overloading `Esc` to leave Formal Plan mode.
- **Reason**: Preserve current FoxHarness muscle memory while adding discoverable, explicit commands compatible with Codex and Claude Code interaction patterns.
- **User Evidence**: The user adopted the proposed `Shift+Tab`, `/plan`, and `/plan off` control scheme without changing `Esc` behavior.
- **Confirmed At**: 2026-07-11 21:47:44 +0800

### DEC-008: Derive the execution checklist from the approved plan

- **Status**: confirmed
- **Decision**: After plan approval, the runtime MUST switch to Default mode and inject the complete approved plan into the continuation context for the same task. Before performing an implementation action, the Agent MUST semantically decompose that approved plan into ordered, executable, and verifiable checklist items and call `update_todo` to replace the session-local `TODO.md`. Read-only revalidation MAY occur before this checklist initialization.
- **Alternatives Rejected**: Having the runtime infer TODO items by parsing free-form `PLAN.md` Markdown with deterministic program rules, or making `submit_plan` generate both artifacts.
- **Reason**: Task decomposition requires semantic understanding of dependencies, verification steps, and possible parallel work. The runtime should orchestrate and transport the approved plan while the Agent performs that reasoning.
- **User Evidence**: The user confirmed that TODO items must be derived from the approved plan and accepted the distinction between runtime plan transport and Agent semantic decomposition.
- **Confirmed At**: 2026-07-11 21:55:33 +0800

### DEC-009: Remove the legacy `-plan` startup flag

- **Status**: confirmed
- **Decision**: Remove the existing `-plan` CLI flag without a deprecation transition. Interactive TUI sessions MUST start in Default mode, and users MUST enter Formal Plan mode through `Shift+Tab` or `/plan`. Non-interactive `fox exec` and `fox -p` runs MUST remain in Default mode and MUST NOT expose a Formal Plan startup option. A generic permission-mode startup flag MUST NOT be introduced in this feature.
- **Alternatives Rejected**: Repurposing `-plan` to select the startup mode, retaining it as a deprecated or no-op compatibility flag, and introducing `--permission-mode plan` before FoxHarness has a broader permission-mode system.
- **Reason**: The existing flag means "run the legacy automatic Planner before every task," which conflicts with the newly defined user-controlled Formal Plan lifecycle. Removing it avoids silently preserving misleading behavior.
- **User Evidence**: After comparing Codex and Claude Code startup interfaces, the user explicitly confirmed direct removal of `-plan` with no deprecation period.
- **Confirmed At**: 2026-07-11 22:18:06 +0800

### DEC-010: Remove the legacy automatic Planner across entry points

- **Status**: confirmed
- **Decision**: Remove automatic `Planner.BuildPlan` pre-passes from the shared `AgentRunner`, AgentOps, and benchmark entry points. Default mode Agents MUST decide whether to maintain an execution checklist through `update_todo`. Benchmark runs MUST expose `read_todo` and `update_todo` so their Default tool surface remains aligned with normal runs. Feishu MUST retain its existing no-preplan behavior, and Autodev MUST retain its independent SDD workflow. Remove the obsolete `internal/memory/plan.go` implementation, its tests, the legacy `EnablePlanMode` configuration fields, and related getters and setters. Preserve `memory.Store`, session-local `PLAN.md` and `TODO.md`, state snapshots, and `/rewind`; the new Formal Plan state MUST be represented by collaboration-mode state rather than the legacy boolean.
- **Alternatives Rejected**: Migrating only the TUI while retaining automatic planning in AgentOps or benchmark, and preserving the old Planner under a second legacy mode.
- **Reason**: Every entry point should use one consistent meaning of Formal Plan mode: an explicitly selected, user-controlled planning lifecycle. Hidden pre-planning would conflict with that contract and retain an unnecessary extra model call.
- **User Evidence**: The user explicitly confirmed the unified migration proposal covering the shared runner, AgentOps, benchmark, Feishu, Autodev, and retained session-state infrastructure.
- **Confirmed At**: 2026-07-11 22:19:52 +0800

## Out of Scope

### OUT-001: Sandbox and command approval infrastructure

- **Status**: confirmed
- **Statement**: The initial feature MUST NOT add OS-level sandboxing or command/tool invocation approval infrastructure. This exclusion does not remove the required user confirmation of a submitted Formal Plan proposal.
- **Reason**: The initial implementation prioritizes interaction design and Agent instruction behavior over a new security subsystem.
- **User Evidence**: The user explicitly deferred sandbox and approval mechanisms.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

### OUT-002: Incremental or direct plan-file editing

- **Status**: confirmed
- **Statement**: The initial feature MUST NOT add `edit_plan`, incremental Agent editing of `PLAN.md`, or direct user editing of the proposal through `$EDITOR`.
- **Reason**: Plan adjustments use user feedback followed by complete Agent resubmission.
- **User Evidence**: The user explicitly confirmed that the initial version should only support feedback and full resubmission.
- **Confirmed At**: 2026-07-11 21:37:34 +0800

## Open Questions

### OPEN-001: Formal Plan mode TUI controls

- **Status**: resolved
- **Question**: Which explicit TUI controls and commands enter, leave, or bypass Formal Plan mode?
- **Why It Matters**: The interaction contract and command/keybinding compatibility must be settled before the mode lifecycle can be specified.
- **Owner**: User
- **Resolved By**: DEC-007

### OPEN-002: Execution checklist initialization

- **Status**: resolved
- **Question**: After plan approval, how and when should session-local `TODO.md` be initialized from the approved proposal?
- **Why It Matters**: This determines the boundary between `submit_plan`, the Default mode continuation prompt, and the first `update_todo` call.
- **Owner**: User
- **Resolved By**: DEC-008

### OPEN-003: Existing Planner migration

- **Status**: resolved
- **Question**: How should the current pre-execution Planner that writes both `PLAN.md` and `TODO.md` be removed, migrated, or retained for non-TUI workflows?
- **Why It Matters**: The new Formal Plan lifecycle must not conflict with the existing automatic pre-planning path or regress other entry points.
- **Owner**: User / Team
- **Resolved By**: DEC-009, DEC-010

## Superseded Entries

None.

## Confirmation Log

### Session 2026-07-11 21:37:34 +0800

- **Summary Presented**: Formal Plan mode uses instruction-driven read-only behavior and a reduced tool surface; complete plans are submitted through `submit_plan`, revised through feedback and full resubmission, and executed in the same task after approval. Existing `update_todo` remains the sole execution checklist tool.
- **User Confirmation**: The user explicitly replied "确认" after correcting and reconfirming the `update_todo` decision.
- **Entries Confirmed**: NEED-001, CON-001, CON-002, DEC-001, DEC-002, DEC-003, DEC-004, DEC-005, DEC-006, OUT-001, OUT-002

### Session 2026-07-11 21:47:44 +0800

- **Summary Presented**: Preserve `Shift+Tab` mode switching, add idempotent `/plan` and explicit `/plan off`, apply active-run changes to the next submission, preserve FoxHarness `Esc` semantics, and return to Default automatically after approval.
- **User Confirmation**: The user explicitly replied "采纳".
- **Entries Confirmed**: DEC-007

### Session 2026-07-11 21:55:33 +0800

- **Summary Presented**: After approval, the runtime returns to Default mode and injects the complete approved plan into the same-task continuation. The Agent, not a deterministic runtime parser, derives actionable TODO items and calls `update_todo` before implementation.
- **User Confirmation**: The user explicitly replied "确认".
- **Entries Confirmed**: DEC-008

### Session 2026-07-11 22:18:06 +0800

- **Summary Presented**: Codex has no public Plan startup flag and Claude Code uses a generic permission-mode flag rather than a standalone `--plan`. Remove FoxHarness's legacy `-plan` directly, start TUI sessions in Default, keep non-interactive runs in Default, and do not add a generic mode flag in this feature.
- **User Confirmation**: The user explicitly replied "确认".
- **Entries Confirmed**: DEC-009

### Session 2026-07-11 22:19:52 +0800

- **Summary Presented**: Remove every legacy automatic Planner pre-pass, let Default mode use `update_todo` adaptively, align benchmark tools, preserve Feishu and Autodev behavior, remove obsolete Planner and `EnablePlanMode` code, and retain session plan/TODO storage, snapshots, and `/rewind`.
- **User Confirmation**: The user explicitly replied "确认".
- **Entries Confirmed**: DEC-010
