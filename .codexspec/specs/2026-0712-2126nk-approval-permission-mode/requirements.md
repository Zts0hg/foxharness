# Confirmed Requirements: approval-permission-mode

<!--
Language: Maintain this document in the language specified in .codexspec/config.yml.
This file is the authoritative, persistent record of user-confirmed intent.
Do not copy the full conversation. Keep only confirmed decisions and short evidence
quotes needed to resolve later interpretation disputes.
-->

**Feature ID**: `2026-0712-2126nk`
**Status**: Discovery Complete - Requirements Confirmed
**Last Confirmed**: 2026-07-13 10:41:55 CST

## Authority Rules

- Only entries with `Status: confirmed` are binding downstream inputs.
- `open` entries MUST NOT be converted into confirmed product requirements.
- Replaced entries remain in this file with `Status: superseded` and a link to the replacement.
- AI inferences must be labeled as assumptions and require user confirmation before becoming binding.

## Needs

### NEED-001: TUI permission and approval mode

- **Status**: confirmed
- **Statement**: FoxHarness TUI needs a first-version permission and approval feature modeled primarily after Codex CLI's permission presets and tool-call approval flow.
- **Rationale**: The project needs a smoother approval experience that can gate potentially unsafe tool calls without replacing the existing agent engine or requiring a Claude Code-style classifier in the first iteration.
- **User Evidence**: "确认。并且权限模式的命令与 codex 一样使用“/permissions”这个命令名"
- **Confirmed At**: 2026-07-12 21:31:47 CST

### NEED-002: Low-noise approval visibility and auditability

- **Status**: confirmed
- **Statement**: Users need visible review progress, understandable escalations, and an inspectable record of automatic approval without duplicating normal tool-call output or adding review noise to deterministic fast-path calls.
- **User Evidence**: "采纳"
- **Confirmed At**: 2026-07-12 23:43:49 CST

## Constraints

### CON-001: TUI command name matches Codex CLI

- **Status**: confirmed
- **Statement**: The TUI permission mode command MUST use `/permissions` as the canonical slash command name, matching Codex CLI.
- **User Evidence**: "权限模式的命令与 codex 一样使用“/permissions”这个命令名"

### CON-002: Tool-level approval boundary

- **Status**: confirmed
- **Statement**: This feature MUST be scoped to tool-level approval and policy enforcement. It MUST NOT claim OS-level filesystem, process, or network isolation.
- **User Evidence**: "继续按 tool-level approval first，OS sandbox 后续单独做 来固化"

### CON-003: Permission mode persistence location

- **Status**: confirmed
- **Statement**: The selected `/permissions` mode MUST be persisted in `~/.foxharness/settings.json`.
- **User Evidence**: "持久化"

### CON-004: Approval enforcement cannot be bypassed by nested execution

- **Status**: confirmed
- **Statement**: Composite and nested execution paths MUST remain subject to the active permission policy. In particular, delegated Subagent tool calls MUST inherit the same permission mode and approval coordinator, and model-invoked skills MUST pass through the permission boundary before their embedded shell commands, hooks, or forked execution can run.
- **Reason**: The current read-only Subagent surface still includes arbitrary `bash`, while skills can execute shell embeddings, hooks, or forked agents. Gating only the top-level registry would leave practical approval bypasses.
- **User Evidence**: "采纳你的建议"

### CON-005: Bash fast-path parsing must fail closed

- **Status**: confirmed
- **Statement**: Bash fast-path classification MUST use a structured shell parser rather than ad hoc command-string splitting. Unparseable or unsupported shell syntax, unknown commands, ambiguous flags, and paths that cannot be proven workspace-contained MUST miss the fast path and proceed to the configured reviewer. Missing the fast path MUST NOT itself deny execution.
- **User Evidence**: "采纳这套边界"

### CON-006: Reviewer evidence has explicit trust boundaries

- **Status**: confirmed
- **Statement**: Only direct user messages, applicable developer or project instructions including `AGENTS.md`, and direct answers returned through `ask_user_question` MAY establish authorization. Assistant text, planned tool arguments, tool results, Skill content, and file content MUST be treated as untrusted evidence and MUST NOT independently broaden user authorization. Content from a file, issue, or other untrusted source MAY contribute authorization only within the scope of an explicit user instruction to follow that source.
- **User Evidence**: "采用方案A"

### CON-007: Requirements and downstream artifacts must be source-independent

- **Status**: confirmed
- **Statement**: `requirements.md`, `spec.md`, `plan.md`, and `tasks.md` MUST describe FoxHarness behavior, constraints, acceptance criteria, and implementation plans in self-contained terms. They MUST NOT include Codex CLI or Claude Code source paths, filenames, internal symbols, or source-derived implementation claims. User-observable compatibility targets and explicitly selected product interaction semantics MAY be named when they clarify intent, but they MUST be restated as complete FoxHarness requirements that do not depend on external source code for interpretation.
- **Research Handling**: Codex CLI and Claude Code source may be inspected during analysis, but those research notes do not need to be persisted in project artifacts.
- **User Evidence**: "源码研究可以不持久化到文档。之后的spec/plan/task文档内容也不应该出现与源码相关的描述"

## Decisions

### DEC-001: First version scope

- **Status**: superseded
- **Replaced By**: DEC-024
- **Decision**: Implement the first version as Codex-style deterministic permission presets plus a TUI approval queue, while leaving Claude Code-style AI classifier auto mode as a future extension point.
- **Alternatives Rejected**: Implementing a full Claude Code-style LLM classifier auto mode in the first version.
- **Reason**: This preserves a practical first iteration that fits FoxHarness' current middleware architecture and avoids adding classifier complexity before the base permission/approval UX exists.
- **User Evidence**: "确认"

### DEC-002: Sandbox sequencing

- **Status**: confirmed
- **Decision**: Continue with tool-level approval first; implement OS sandboxing as a later, separate feature.
- **Alternatives Rejected**: Implementing full OS sandboxing inside this approval feature.
- **Reason**: Approval and OS sandboxing are separate layers. The approval MVP can use existing middleware architecture, while OS sandboxing requires a broader execution-isolation design.
- **User Evidence**: "继续按 tool-level approval first，OS sandbox 后续单独做 来固化"

### DEC-003: User-visible permission modes

- **Status**: confirmed
- **Decision**: The first version of `/permissions` MUST expose three user-visible modes: `Ask for approval`, `Approve for me`, and `Full Access`.
- **Alternatives Rejected**: Exposing `read-only` as a first-version user-visible mode.
- **Reason**: The feature should align with the main Codex CLI `/permissions` user experience instead of surfacing an extra lower-level preset as a primary mode.
- **User Evidence**: "按这个三模式方案固化"

### DEC-004: Codex-aligned mode semantics

- **Status**: superseded
- **Replaced By**: DEC-008
- **Decision**: `Ask for approval` and `Approve for me` MUST share the same tool-level permission boundary. `Ask for approval` uses the user as reviewer; `Approve for me` uses deterministic automatic review first and falls back to user approval for dangerous or uncertain tool calls. `Full Access` disables tool-level approval interception.
- **Alternatives Rejected**: Treating `Approve for me` as a broader permission mode than `Ask for approval`, or treating `Full Access` as OS-level sandbox disablement.
- **Reason**: Keeping one permission boundary for `Ask for approval` and `Approve for me` ensures that automatic review changes only who evaluates a request, not what authority the mode grants. `Full Access` remains an explicit approval bypass at the FoxHarness tool-policy layer only.
- **User Evidence**: "采纳建议"

### DEC-005: Approval prompt decisions

- **Status**: confirmed
- **Decision**: The TUI approval prompt MUST expose four decisions for blocked tool calls: allow once (`Yes, proceed`), allow equivalent requests for the current session (`Yes, allow for this session`), deny this call while allowing the agent to continue (`No, continue without running it`), and cancel with user feedback (`No, and tell Fox what to do differently`).
- **Alternatives Rejected**: A minimal yes/no-only prompt.
- **Reason**: This mirrors Codex's approval overlay semantics and gives the agent enough feedback to either continue without the tool call or revise its approach.
- **User Evidence**: "采纳"

### DEC-006: Persist selected permission mode

- **Status**: confirmed
- **Decision**: `/permissions` changes MUST persist the selected mode across TUI sessions.
- **Alternatives Rejected**: Session-only permission mode changes.
- **Reason**: FoxHarness already persists TUI preferences such as `/theme` and `/statusline` in `~/.foxharness/settings.json`; permission mode should follow the same user preference model.
- **User Evidence**: "持久化"

### DEC-007: Full Access warning confirmation

- **Status**: superseded
- **Replaced By**: DEC-023
- **Decision**: Switching to `Full Access` MUST show a dangerous-mode confirmation unless the warning has been remembered. The confirmation MUST offer a one-time enable action and an enable-and-remember-warning action. Remembering the warning MUST persist a separate acknowledgment flag in settings.
- **Alternatives Rejected**: Enabling `Full Access` with no warning, or showing the warning forever with no remember option.
- **Reason**: `Full Access` disables tool-level approval interception, so accidental activation needs an explicit guardrail while still allowing experienced users to avoid repeated warnings.
- **User Evidence**: "采纳"

### DEC-008: Approve for me review strategy

- **Status**: confirmed
- **Decision**: `Approve for me` MUST use deterministic fast paths plus an LLM auto reviewer. Deterministic fast paths MAY auto-allow clearly safe operations, including read-only file tools and a limited set of read-only bash commands. Tool calls that are not fast-path allowed MUST be reviewed by a separate LLM reviewer rather than by the main agent. If LLM review fails, times out, or returns invalid output, FoxHarness MUST retry; only after retries are exhausted should it fall back to user approval.
- **Alternatives Rejected**: Pure deterministic auto review, main-agent self-approval, or immediate user fallback on the first LLM reviewer error.
- **Reason**: Deterministic fast paths avoid unnecessary latency for provably safe operations, while an isolated reviewer can evaluate contextual requests without allowing the main agent to approve its own actions. Retrying technical failures and then falling back to the user preserves task continuity.
- **User Evidence**: "改成 deterministic fast path + LLM auto reviewer... 异常情况增加重试，重试无果才会回退到用户审批"

### DEC-009: Shared permission boundary for Ask and Approve modes

- **Status**: confirmed
- **Decision**: `Ask for approval` and `Approve for me` MUST share the same tool-level permission boundary. `Full Access` disables tool-level approval interception but MUST NOT be described as OS-level sandboxing.
- **Alternatives Rejected**: Treating `Approve for me` as a broader permission mode than `Ask for approval`.
- **Reason**: The two modes differ only in whether the user or the isolated LLM reviewer evaluates non-fast-path requests; automatic review MUST NOT silently broaden the underlying permission boundary.
- **User Evidence**: "改成 deterministic fast path + LLM auto reviewer，按照你建议的行为设计"

### DEC-010: Codex-style workspace-write tool boundary

- **Status**: confirmed
- **Decision**: `Ask for approval` and `Approve for me` MUST apply the following shared boundary:
  - Trusted interaction and session-state tools such as `ask_user_question`, `read_todo`, `update_todo`, and `submit_plan` bypass this permission gate while retaining their own workflow rules.
  - `read_file`, `write_file`, and `edit_file` are automatically allowed only when the normalized, symlink-aware target remains inside the active workspace. Access outside the workspace requires review.
  - `bash` is automatically allowed only when it matches a strict read-only command fast path. Shell redirection, command substitution, network access, and commands with possible side effects require review.
  - `skill`, `delegate_task`, and unknown or newly registered tools require review by default. Delegated tool calls inherit the same permission mode and coordinator.
  - Requests requiring review go directly to the user in `Ask for approval`; in `Approve for me`, they go to the LLM reviewer first.
  - `Full Access` bypasses this approval gate but does not bypass argument validation, Plan mode restrictions, tool-specific safety checks, or other hard policy constraints.
- **Alternatives Rejected**: Automatically allowing workspace-external file access, treating arbitrary `bash` as workspace-confined, allowing nested agents to bypass the parent policy, or making all workspace writes require approval.
- **Reason**: This mirrors Codex's workspace-write experience at the tool-policy layer while compensating for FoxHarness not yet having an OS sandbox.
- **User Evidence**: "采纳你的建议"

### DEC-011: LLM reviewer has single-call approval authority only

- **Status**: confirmed
- **Decision**: The LLM reviewer MUST return one of two valid decisions for a reviewed tool call: `approve` or `escalate`. `approve` authorizes only that exact invocation. `escalate` immediately routes the request to user approval and is not retried. The reviewer MUST NOT automatically deny a call, create a session grant, change the active permission mode, or modify policy.
- **Error Behavior**: Retries apply only to technical reviewer failures such as provider errors, timeouts, or invalid structured output. A valid `escalate` decision is a successful review result rather than an error.
- **Alternatives Rejected**: Allowing the reviewer to silently deny calls or to establish session-scoped permissions.
- **Reason**: Durable authority and explicit denial remain user decisions, while the reviewer is limited to removing interruptions for tool calls it judges safe for one-time execution.
- **User Evidence**: "采用方案A"

### DEC-012: Reliability-first reviewer retry budget

- **Status**: confirmed
- **Decision**: Each LLM review MAY make at most three logical review attempts, including the initial attempt, and all attempts MUST share a 90-second wall-clock budget. Provider-level transport retries remain independent and do not count as logical reviewer attempts. A valid `approve` or `escalate` result ends the retry loop immediately. If the logical-attempt limit or total time budget is exhausted without a valid result, the request MUST fall back to user approval rather than fail the task.
- **Interaction Behavior**: While review is in progress, the TUI MUST show a visible review-in-progress state and remain cancellable. User cancellation terminates the review immediately rather than retrying or opening a fallback approval prompt.
- **Alternatives Rejected**: A lower-latency two-attempt or 60-second budget, and unbounded retries.
- **Reason**: The preferred experience prioritizes automatic recovery from transient provider failures and malformed reviewer output, while the shared deadline prevents an indefinite wait and user approval preserves task continuity after recovery is exhausted.
- **User Evidence**: "采纳这个可靠性优先方案"

### DEC-013: Reviewer reuses the active model in an isolated invocation

- **Status**: confirmed
- **Decision**: The first-version LLM reviewer MUST reuse the provider and model currently selected for the active TUI session, including subsequent `/model` changes, but MUST run as a separate invocation with an isolated review prompt and context. It MUST NOT inherit the main Agent conversation state or receive any executable tools. The first version MUST NOT add a separate reviewer provider, model, or credential setting.
- **Alternatives Rejected**: A separately configurable reviewer model in the first version, or a hard-coded reviewer model.
- **Reason**: This preserves compatibility with both existing OpenAI-compatible and Claude-compatible providers without additional setup, while isolation prevents the main Agent from directly approving its own proposed action.
- **User Evidence**: "采用方案A"

### DEC-014: Tool-specific session approval keys

- **Status**: confirmed
- **Decision**: `Yes, allow for this session` MUST store in-memory, tool-specific approval keys rather than one generic raw-JSON fingerprint:
  - `bash` uses the canonicalized command plus the effective working directory.
  - Workspace-external file reads use a read capability plus the canonical target path.
  - Workspace-external `write_file` and `edit_file` share a file-mutation capability plus the canonical target path, so approval applies to subsequent mutations of that same file but not to other files.
  - `skill` and `delegate_task` use their canonical tool type and normalized arguments.
  - Unknown or newly registered tools fall back to canonical tool name plus fully normalized arguments.
- **Lifetime**: These approvals MUST remain in memory only and MUST NOT be written to settings, session history, or project files. The first version MUST NOT create persistent Bash-prefix grants, directory-wide grants, or tool-wide grants from this action.
- **Alternatives Rejected**: One generic exact JSON fingerprint for every tool, a session-wide all-edits grant, directory-wide grants, or persistent prefix rules in the first version.
- **Reason**: Tool-specific keys preserve useful equivalence between repeated requests without granting broader authority than the user explicitly reviewed, especially before FoxHarness has an OS sandbox.
- **User Evidence**: "采纳你的建议"

### DEC-015: Session approval grant lifecycle

- **Status**: confirmed
- **Decision**: In-memory session approval grants created by `Yes, allow for this session` MUST be cleared when `/new` or its `/clear` alias starts a new session, and when the TUI process exits. They MUST remain active across `/rewind`, `/compact`, and `/permissions` mode changes within the same session. Main-Agent and delegated Subagent calls MUST share the active session's grants through the common approval coordinator.
- **Clarification**: This lifecycle applies only to `session_approval_grants`. It does not apply to the persisted `/permissions` mode, the persisted Full Access warning acknowledgment, deterministic fast-path policy, or one-time user/reviewer approvals.
- **Alternatives Rejected**: Clearing session grants during `/rewind` or every permission-mode change, or restoring grants from persisted session history after process restart.
- **Reason**: Conversation-history operations and session-scoped user authority are separate concerns. Rewinding or compacting conversation state should not unexpectedly revoke an approval, while starting a new session or process must not inherit temporary authority.
- **User Evidence**: "采用这个修正版的方案"

### DEC-016: Conservative read-only Bash fast path

- **Status**: confirmed
- **Decision**: The deterministic Bash fast path MUST use the following first-version boundary:
  - Simple query commands: `pwd`, `whoami`, `id`, `uname`, `which`, `true`, and `false`.
  - Content and text commands: `cat`, `cut`, `echo`, `expr`, `grep`, `head`, `ls`, `nl`, `paste`, `rev`, `seq`, `stat`, `tail`, `tr`, `uniq`, and `wc`.
  - `rg` is allowed only without external-command or archive-execution options such as `--pre`, `--hostname-bin`, `--search-zip`, and equivalent short forms.
  - `find` is allowed only without execution, deletion, confirmation, or file-output options such as `-exec`, `-execdir`, `-ok`, `-okdir`, `-delete`, `-fls`, `-fprint`, `-fprint0`, and `-fprintf`.
  - `sed` is allowed only for `-n` print expressions containing one line number or one numeric line range.
  - Git is limited to `status`, `log`, `diff`, `show`, and read-only `branch` forms. Global or subcommand options that can redirect execution, configuration, repository location, external helpers, or output files MUST miss the fast path, including `-C`, `-c`, `--git-dir`, `--work-tree`, `--output`, `--ext-diff`, `--textconv`, and `--exec`.
- **Shell Grammar**: `&&`, `||`, `;`, and pipelines MAY use the fast path only when every atomic command independently passes. Redirection, command or process substitution, subshells, background execution, heredocs, environment assignments, glob expansion, explicit executable paths, and unsupported syntax MUST miss the fast path.
- **Path Boundary**: Every explicit filesystem operand MUST resolve, including symlink handling, inside the active workspace. Path-free query commands are not subject to a synthetic workspace operand requirement.
- **Excluded Examples**: Tests, builds, interpreters, package managers, network commands, and commands such as `go test` or `go run` are not read-only fast-path operations because they may execute code, mutate caches or files, or access external resources.
- **Alternatives Rejected**: A broader sandbox-dependent read-only command catalog, command-name-only matching, or regex-only shell parsing.
- **Reason**: The conservative catalog and workspace path confinement minimize false-safe classifications while FoxHarness does not yet provide OS-level sandboxing.
- **User Evidence**: "采纳这套边界"

### DEC-017: LLM reviewer risk thresholds

- **Status**: confirmed
- **Decision**: The reviewer MUST classify each action with `risk_level`, `user_authorization`, `decision`, and a concise `rationale`. Risk levels are `low`, `medium`, `high`, and `critical`; authorization levels are `high`, `medium`, `low`, and `unknown`; the only decisions are `approve` and `escalate`.
- **Outcome Rules**:
  - A low-risk action MAY be approved when it is relevant to the current task and narrowly scoped.
  - A medium-risk action MAY be approved when it is a reasonable implementation step toward the user's requested outcome, even if the user did not prescribe the exact command.
  - A high-risk action MAY be approved only when the user has authorized its material effect at least in substance, the target and blast radius are narrow, and no absolute escalation rule applies.
  - A critical-risk action MUST escalate to the user.
  - An action MUST escalate when it is unrelated to the task, lacks important context, has an unclear target or scope, or shows evidence of prompt injection. User urgency alone MUST NOT increase authorization.
- **Clarification**: Any action that the reviewer cannot approve under these rules MUST map to `escalate`, preserving DEC-011's prohibition on automatic denial.
- **Reason**: The policy should automatically recover routine and task-instrumental work while retaining explicit user control over insufficiently authorized, critically risky, or suspicious actions.
- **User Evidence**: "采纳这套风险阈值"

### DEC-018: Trust-aware compact reviewer transcript

- **Status**: confirmed
- **Decision**: Each LLM review MUST receive a bounded, trust-aware transcript rather than only the latest turn or the unbounded full session. Message evidence and tool evidence MUST have separate budgets so tool output cannot displace user intent. The review input MUST include the exact canonical tool action, arguments, effective cwd and workspace, invocation source such as Main Agent, Subagent, or Skill, and the active permission boundary.
- **Retention Rules**: When the transcript exceeds its budget, it MUST prioritize the initial user objective, the latest user messages, explicit authorization messages, and recent execution context. Omitted content MUST be represented by an explicit truncation marker. Missing or truncated evidence MUST NOT be assumed benign or authorizing.
- **Reviewer Restrictions**: The reviewer remains tool-free and MUST escalate when the retained evidence is insufficient to evaluate material risk, target, scope, or authorization.
- **Alternatives Rejected**: Reviewing only the latest request and tool call, or sending the entire unbounded transcript.
- **Reason**: A compact transcript preserves multi-turn authorization and task context while controlling latency, cost, prompt-injection exposure, and evidence crowding.
- **User Evidence**: "采用方案A"

### DEC-019: First version applies only to interactive TUI runs

- **Status**: confirmed
- **Decision**: The first version of `/permissions`, its approval coordinator, approval queue, and persisted mode MUST apply only to the default interactive FoxHarness TUI. Subagents, Skill forks, and nested tool calls created by that TUI MUST inherit the same coordinator. Non-interactive entry points, including `fox exec`, `fox -p`, autodev, Feishu, AgentOps, and bench, MUST retain their existing behavior and middleware and MUST ignore the TUI permission-mode setting.
- **Default Behavior**: A missing, unknown, or invalid persisted mode MUST resolve to `Ask for approval`. A persisted `Full Access` selection MUST NOT affect non-interactive entry points.
- **Alternatives Rejected**: Extending the first version to every runtime, or adding non-interactive approval flags and fallback protocols in this feature.
- **Reason**: Human approval requires an interactive surface. Limiting the first version avoids silently denying, auto-approving, or blocking non-interactive workflows whose approval transport has not been designed.
- **User Evidence**: "采用方案A"

### DEC-020: Serial FIFO approval queue

- **Status**: confirmed
- **Decision**: Tool calls requiring review MUST enter one serial FIFO queue ordered by the model's tool-call order. At most one LLM review or user approval prompt may be active at a time. Fast-path calls and calls matching an existing session grant bypass the approval queue. Before a queued call starts review or execution, the coordinator MUST re-evaluate the current permission mode and session grants.
- **Decision Effects**:
  - `Yes, proceed` authorizes and executes only the current call, then advances the queue.
  - `Yes, allow for this session` authorizes the current call, records its typed approval key, and causes later queued calls to be re-evaluated against the new grant.
  - `No, continue without running it` denies only the current call, returns a structured denied tool result to the Agent, and continues the queue.
  - `No, and tell Fox what to do differently` collects feedback, terminates the current Agent turn, cancels all queued calls that have not started execution, and submits the feedback as the next user input.
  - Cancellation while an LLM review is running terminates the current turn and all not-yet-executed queued calls without opening a fallback approval prompt.
- **Non-Transactional Boundary**: Tool calls that completed before denial, feedback cancellation, or user cancellation MUST NOT be rolled back by the approval coordinator.
- **Alternatives Rejected**: Concurrent LLM reviews with serialized prompts, or fully parallel approval and execution.
- **Reason**: Serial review preserves model tool-call ordering, prevents decisions based on stale side-effect state, and provides one coherent approval surface.
- **User Evidence**: "采用方案A"

### DEC-021: Codex-style permissions menu with bulk session-grant clearing

- **Status**: confirmed
- **Decision**: `/permissions` MUST remain primarily a Codex-style selector for the three confirmed modes and MUST add one separate `Clear session approvals` action. This action is not a fourth mode. It MUST be enabled only when session grants exist, display the number of grants to be removed, and clear all in-memory session grants immediately.
- **Queue Interaction**: Clearing grants MUST NOT interrupt a tool call that has already started. Calls still waiting in the approval queue are re-evaluated under DEC-020 before review or execution.
- **Scope Limit**: The first version MUST NOT provide a Claude Code-style per-rule browser, individual grant deletion, or persistent permission-rule editor.
- **Alternatives Rejected**: A strict Codex menu with no revocation path, or a full Claude Code-style permission-rule manager.
- **Reason**: This preserves the simple Codex mode picker while allowing users to revoke grants that otherwise survive rewind and mode changes without abandoning the current conversation.
- **User Evidence**: "采用方案B"

### DEC-022: Low-noise review status and audit surfaces

- **Status**: confirmed
- **Decision**: LLM review MUST use a transient `Reviewing <tool>...` status and show the logical attempt number only after a retry begins. Automatic approval MUST annotate the existing tool-call summary with `Auto-approved` and its risk level rather than add a separate history row. Reviewer rationale MUST be available in the tool call's expanded details and remain collapsed by default. Deterministic fast-path calls MUST retain normal tool-call rendering without reviewer-specific status or audit annotations.
- **Escalation Display**: A user approval prompt MUST show the exact action, effective cwd, risk level, reviewer rationale, and the precise scope of any session grant. After reviewer retry exhaustion, the prompt MUST state that auto-review was unavailable after three attempts before presenting normal user decisions.
- **Status Surfaces**: `/status` MUST show the active permission mode and current session-grant count. `/statusline` MUST support an optional declarative `permissions` item, but this item MUST NOT be added to the default statusline collection. While `Full Access` is active, the TUI MUST display a separate persistent `[full access]` warning near the bottom of the interface regardless of statusline configuration.
- **Alternatives Rejected**: Hiding automatic review outcomes, emitting a separate history entry for every review, annotating deterministic fast-path calls, or adding `permissions` to the default statusline.
- **Reason**: The interaction should remain compact during routine work while preserving enough state and rationale to understand consequential automatic decisions and dangerous modes.
- **User Evidence**: "采纳"

### DEC-023: Persisted Full Access requires effective startup confirmation

- **Status**: confirmed
- **Decision**: Selecting `Full Access` in `/permissions` MUST persist `Full Access` as the selected permission mode. If the Full Access warning has not been remembered, ordinary confirmation MUST activate `Full Access` only for the current TUI run and MUST NOT persist the warning acknowledgment. On a later startup, a persisted `Full Access` selection without a remembered warning acknowledgment MUST initially use `Ask for approval` as the effective mode and immediately show the Full Access warning before `Full Access` can become effective. Choosing `Enable and remember` MUST persist both the `Full Access` selection and a separate warning acknowledgment, allowing later TUI runs to activate `Full Access` directly.
- **Settings Lifecycle**: Switching from `Full Access` to another mode MUST update only the persisted selected mode and MUST leave the remembered warning acknowledgment unchanged. The warning acknowledgment MUST be a separate, resettable settings field.
- **Alternatives Rejected**: Treating one-time confirmation as a session-only mode selection, silently activating persisted `Full Access` on the next startup without remembered acknowledgment, or clearing the acknowledgment whenever the user changes modes.
- **Reason**: Mode persistence remains predictable while each unremembered startup requires an explicit dangerous-mode confirmation before approval interception is disabled.
- **User Evidence**: "采用方案A"

### DEC-024: Final first-version approval architecture

- **Status**: confirmed
- **Decision**: The first version MUST combine the three Codex-style permission modes with deterministic fast paths, an isolated LLM auto reviewer for `Approve for me`, and the serial FIFO approval queue. The LLM reviewer is part of this feature and MUST follow DEC-008 through DEC-018. The first version MUST NOT implement Claude Code's general-purpose classifier-driven Auto mode or its persistent permission-rule engine.
- **Alternatives Rejected**: A deterministic-only reviewer, a full Claude Code-style classifier and rule system, or treating all LLM-based review as out of scope.
- **Reason**: This preserves the confirmed low-interruption reviewer behavior while keeping the feature bounded to Codex-style tool-call approval rather than introducing a second generalized policy-classification architecture.
- **User Evidence**: "按照建议修正。"

## Out of Scope

### OUT-001: Claude Code classifier auto mode implementation

- **Status**: superseded
- **Replaced By**: OUT-005
- **Statement**: The first version will not implement Claude Code's LLM classifier-based Auto mode.
- **Reason**: The first version will focus on Codex-style presets and deterministic approval behavior, with classifier support reserved as an extension point.
- **User Evidence**: "确认"

### OUT-002: OS sandbox implementation

- **Status**: confirmed
- **Statement**: OS-level sandboxing, including process-level filesystem isolation and network isolation, is out of scope for this feature.
- **Reason**: OS sandboxing will be handled by a later dedicated feature after the tool-level approval UX and policy layer exist.
- **User Evidence**: "OS sandbox 后续单独做"

### OUT-003: First-version read-only mode

- **Status**: confirmed
- **Statement**: A user-visible `read-only` permission mode is out of scope for the first version.
- **Reason**: The first version should expose the three Codex-aligned primary modes only.
- **User Evidence**: "按这个三模式方案固化"

### OUT-004: Non-interactive approval integration

- **Status**: confirmed
- **Statement**: Approval-mode integration for `fox exec`, `fox -p`, autodev, Feishu, AgentOps, bench, and other non-interactive entry points is out of scope for the first version.
- **Reason**: Those entry points require a separately specified approval transport or non-interactive fallback contract.
- **User Evidence**: "采用方案A"

### OUT-005: Claude Code general classifier and persistent rule engine

- **Status**: confirmed
- **Statement**: Claude Code's general-purpose classifier-driven Auto mode, persistent allow/deny rule engine, and rule-management interface are out of scope for the first version. This exclusion does not apply to the isolated tool-call LLM reviewer confirmed for `Approve for me`.
- **Reason**: FoxHarness is implementing Codex-style tool-call review with narrowly defined reviewer authority, not a generalized classifier and persistent permission-policy subsystem.
- **User Evidence**: "按照建议修正。"

## Open Questions

### OPEN-001: Permission preset semantics

- **Status**: resolved
- **Question**: Which user-visible permission modes should `/permissions` expose in the first version?
- **Resolution**: Expose `Ask for approval`, `Approve for me`, and `Full Access`; do not expose `read-only` in the first version.
- **Why It Matters**: This determines the TUI command surface and prevents implementation from adding a non-confirmed fourth mode.
- **Owner**: User / Team / Research

### OPEN-002: Approval decision choices

- **Status**: resolved
- **Question**: Which approval choices should the TUI prompt expose for a blocked tool call, such as allow once, allow for session, deny, or cancel with feedback?
- **Resolution**: Expose allow once, allow for current session, deny and continue, and cancel with feedback.
- **Why It Matters**: This determines the shape of the approval overlay, session-scoped grants, and what result is returned to the agent after rejection.
- **Owner**: User

### OPEN-003: Full Access confirmation persistence

- **Status**: resolved
- **Question**: Should acknowledging the `Full Access` warning be remembered permanently, remembered only for the session, or shown every time?
- **Resolution**: Persist the selected `Full Access` mode in both cases, but ordinary confirmation activates it only for the current TUI run. On later startup, use `Ask for approval` as the effective mode and show the warning again unless a separate remembered-warning acknowledgment was persisted. Keep that acknowledgment when switching modes and expose it as an independently resettable setting.
- **Why It Matters**: This determines whether the settings schema needs a separate warning acknowledgment flag and how aggressively the TUI should protect users from accidental `Full Access`.
- **Owner**: User

### OPEN-004: Shared permission boundary

- **Status**: resolved
- **Question**: Which tools should be automatically allowed, reviewed, or inherited by nested execution in `Ask for approval` and `Approve for me`?
- **Resolution**: Use the Codex-style workspace-write boundary in DEC-010, including canonical workspace path checks, a strict read-only Bash fast path, default review for composite and unknown tools, and inherited enforcement for delegated calls.
- **Why It Matters**: This defines the actual security and interaction behavior of all three permission modes and closes approval bypasses through paths, skills, and Subagents.
- **Owner**: User

### OPEN-005: LLM reviewer authority

- **Status**: resolved
- **Question**: May the LLM reviewer automatically deny calls or create session grants, or may it only approve one call and escalate uncertain requests?
- **Resolution**: The reviewer may approve only the exact current invocation or escalate it to the user. It cannot deny, create session grants, change modes, or modify policy.
- **Why It Matters**: This keeps user authority explicit and prevents an LLM review result from silently broadening permissions or derailing execution.
- **Owner**: User

### OPEN-006: LLM reviewer retry budget

- **Status**: resolved
- **Question**: How many logical review attempts and how much total time should be allowed before falling back to user approval?
- **Resolution**: Allow up to three logical attempts within one shared 90-second budget, preserve provider-level transport retries, show progress, permit cancellation, and fall back to user approval after exhaustion.
- **Why It Matters**: This balances recovery reliability against bounded interaction latency and prevents reviewer failures from failing the agent task.
- **Owner**: User

### OPEN-007: Reviewer model selection

- **Status**: resolved
- **Question**: Should the reviewer reuse the active model, use a separately configurable model, or use a fixed built-in model?
- **Resolution**: Reuse the active TUI provider and model in a separate, tool-free reviewer invocation with isolated context; do not add reviewer-specific model configuration in the first version.
- **Why It Matters**: This determines configuration complexity, provider compatibility, reviewer isolation, and how `/model` changes affect approval behavior.
- **Owner**: User

### OPEN-008: Session approval matching scope

- **Status**: resolved
- **Question**: Should session approval use a generic exact invocation fingerprint, reusable command or directory rules, or tool-specific semantic keys?
- **Resolution**: Use conservative Codex-style typed approval keys with exact canonical scope per tool. Keep them in memory and defer persistent prefix, directory-wide, and tool-wide rules.
- **Why It Matters**: This determines which later calls may bypass review and prevents a narrowly presented approval from silently becoming broad authority.
- **Owner**: User

### OPEN-009: Session approval grant lifecycle

- **Status**: resolved
- **Question**: Which TUI actions should retain or clear in-memory session approval grants?
- **Resolution**: Clear grants on `/new`, `/clear`, and TUI exit; retain them across `/rewind`, `/compact`, and permission-mode changes; never restore them from persisted history.
- **Why It Matters**: This separates persistent mode preferences, conversation history operations, and temporary user authorization without surprising permission loss or cross-session carryover.
- **Owner**: User

### OPEN-010: Read-only Bash fast path

- **Status**: resolved
- **Question**: Which Bash commands and shell constructs may bypass review as deterministically read-only?
- **Resolution**: Use the conservative command and flag catalog in DEC-016, require structured parsing and workspace-contained operands, and route every unsupported or uncertain call to review rather than denying it.
- **Why It Matters**: This fast path is the primary latency optimization for both interactive and LLM-reviewed modes and must not become a side-effect or workspace-boundary bypass.
- **Owner**: User

### OPEN-011: LLM reviewer risk thresholds

- **Status**: resolved
- **Question**: Which risk and authorization combinations may be automatically approved by the LLM reviewer?
- **Resolution**: Apply the Codex-style matrix in DEC-017, with critical, suspicious, unrelated, insufficiently contextualized, or insufficiently authorized high-risk actions escalated to the user rather than automatically denied.
- **Why It Matters**: This controls how often `Approve for me` removes interruptions versus requiring explicit human authority for consequential actions.
- **Owner**: User

### OPEN-012: Reviewer context and evidence trust

- **Status**: resolved
- **Question**: Should the reviewer receive only the latest turn, the full transcript, or a bounded transcript with explicit trust separation?
- **Resolution**: Use the trust-aware compact transcript in DEC-018 and the evidence trust rules in CON-006; preserve user intent and recent execution context under separate budgets and escalate when truncation leaves material uncertainty.
- **Why It Matters**: The reviewer needs enough context to recognize valid multi-turn authorization without allowing untrusted tool or file content to manufacture permission.
- **Owner**: User

### OPEN-013: Runtime scope and default mode

- **Status**: resolved
- **Question**: Should the first version apply only to TUI runs or also cover non-interactive entry points, and what mode should be used when settings are absent or invalid?
- **Resolution**: Apply the feature only to the interactive TUI and its nested execution, default to `Ask for approval`, and leave every non-interactive entry point unchanged.
- **Why It Matters**: This prevents a TUI preference from unexpectedly blocking or broadening unattended execution paths that cannot present the approval UI.
- **Owner**: User

### OPEN-014: Approval queue concurrency and cancellation

- **Status**: resolved
- **Question**: Should review run serially or concurrently, and how should each approval or cancellation decision affect queued calls?
- **Resolution**: Use the serial FIFO lifecycle in DEC-020, re-evaluate grants before each call, distinguish single-call denial from turn cancellation with feedback, and do not roll back completed work.
- **Why It Matters**: This defines ordering, prevents stale-context approvals, and ensures user cancellation has deterministic effects across multi-tool model responses.
- **Owner**: User

### OPEN-015: Session-grant management surface

- **Status**: resolved
- **Question**: Should `/permissions` omit grant management like Codex, add a bulk clear action, or provide a full Claude Code-style rule manager?
- **Resolution**: Keep the Codex-style mode selector and add only a bulk `Clear session approvals` action with a visible grant count.
- **Why It Matters**: Users need a way to revoke in-memory authority that survives rewind and mode changes without introducing a persistent rule-management subsystem.
- **Owner**: User

### OPEN-016: Review visibility and audit presentation

- **Status**: resolved
- **Question**: How much review progress and decision detail should appear in the TUI, history, `/status`, and statusline?
- **Resolution**: Use the low-noise transient, collapsed, and status-oriented surfaces in DEC-022; show a mandatory warning only for Full Access and do not annotate deterministic fast-path calls.
- **Why It Matters**: This balances operational transparency with the compact Codex-style tool-call presentation already established in the TUI.
- **Owner**: User

### OPEN-017: Full Access mode and warning persistence interaction

- **Status**: resolved
- **Question**: When a user confirms `Full Access` without remembering its warning, should the selected mode itself remain persisted, and what should become effective at the next startup?
- **Resolution**: Keep `Full Access` persisted as the selected mode, but activate `Ask for approval` initially on the next startup and require the warning again. Only `Enable and remember` allows direct Full Access activation on future startups; changing modes does not clear the separate warning acknowledgment.
- **Why It Matters**: This separates the user's persisted mode preference from consent to suppress a dangerous-mode warning and prevents silent startup in Full Access.
- **Owner**: User

## Superseded Entries

Superseded entries are retained in their original sections with replacement links:

- `DEC-001` was replaced by `DEC-024` after the scope added an isolated LLM auto reviewer while continuing to exclude Claude Code's general-purpose classifier and persistent rule engine.
- `DEC-004` was replaced by `DEC-008` when `Approve for me` changed from deterministic-only review to deterministic fast paths plus a separate LLM auto reviewer with retry-before-user-fallback behavior.
- `DEC-007` was replaced by `DEC-023` to distinguish persisted mode selection, current-run activation, and remembered Full Access warning acknowledgment.
- `OUT-001` was replaced by `OUT-005` to clarify that the exclusion applies to Claude Code's generalized classifier and persistent rule subsystem, not every LLM-based approval path.

## Confirmation Log

### Session 2026-07-12 21:31:47 CST

- **Summary Presented**: First version should use Codex-style permission presets and a TUI approval queue; `/permissions` is the canonical command name; Claude Code classifier auto mode remains a future extension point.
- **User Confirmation**: "确认。并且权限模式的命令与 codex 一样使用“/permissions”这个命令名"
- **Entries Confirmed**: NEED-001, CON-001, DEC-001, OUT-001

### Session 2026-07-12 21:38:39 CST

- **Summary Presented**: Approval and OS sandboxing are separate layers; continue with tool-level approval in this feature and leave OS sandboxing to a later dedicated feature.
- **User Confirmation**: "继续按 tool-level approval first，OS sandbox 后续单独做 来固化"
- **Entries Confirmed**: CON-002, DEC-002, OUT-002

### Session 2026-07-12 21:42:36 CST

- **Summary Presented**: Align the first-version `/permissions` user-visible modes with Codex CLI's main interaction: `Ask for approval`, `Approve for me`, and `Full Access`; do not expose `read-only` as a first-version mode.
- **User Confirmation**: "按这个三模式方案固化"
- **Entries Confirmed**: DEC-003, OUT-003

### Session 2026-07-12 21:50:21 CST

- **Summary Presented**: Mirror Codex semantics at tool level: `Ask for approval` and `Approve for me` share the same permission boundary but differ by reviewer; `Full Access` disables tool-level approval and is not OS sandboxing.
- **User Confirmation**: "采纳建议"
- **Entries Confirmed**: DEC-004

### Session 2026-07-12 21:52:29 CST

- **Summary Presented**: Approval prompt should provide Codex-aligned choices: allow once, allow for current session, deny and continue, or cancel with feedback.
- **User Confirmation**: "采纳"
- **Entries Confirmed**: DEC-005

### Session 2026-07-12 21:53:55 CST

- **Summary Presented**: Persist `/permissions` mode selection across TUI sessions in `~/.foxharness/settings.json`, consistent with existing TUI preferences.
- **User Confirmation**: "持久化"
- **Entries Confirmed**: CON-003, DEC-006

### Session 2026-07-12 21:55:00 CST

- **Summary Presented**: `Full Access` should show a dangerous-mode confirmation by default, with one-time enable and enable-and-remember-warning choices; remembered warning is stored separately in settings.
- **User Confirmation**: "采纳"
- **Entries Confirmed**: DEC-007

### Session 2026-07-12 22:13:57 CST

- **Summary Presented**: Update `Approve for me` from deterministic-only review to deterministic fast paths plus a separate LLM auto reviewer; retry reviewer failures before falling back to user approval.
- **User Confirmation**: "改成 deterministic fast path + LLM auto reviewer，按照你建议的行为设计，但是\"LLM reviewer 失败、超时、输出不合法时 fail closed，回退到用户审批。\"这一条需要为异常情况增加重试，重试无果才会回退到用户审批。"
- **Entries Confirmed**: DEC-008, DEC-009

### Session 2026-07-12 22:28:51 CST

- **Summary Presented**: Apply a workspace-contained tool boundary: trusted interaction/session tools pass through; canonical workspace-contained file reads and writes pass through; only strict read-only Bash commands use a fast path; skills, delegated agents, and unknown tools require review; nested calls inherit the policy; Full Access bypasses approval but not hard constraints.
- **User Confirmation**: "采纳你的建议"
- **Entries Confirmed**: CON-004, DEC-010

### Session 2026-07-12 22:31:13 CST

- **Summary Presented**: Limit the LLM reviewer to approving the exact current invocation or escalating it to the user. It cannot deny automatically, create session grants, or change permission policy; only technical failures are retried, while a valid escalation is not.
- **User Confirmation**: "采用方案A"
- **Entries Confirmed**: DEC-011

### Session 2026-07-12 22:47:04 CST

- **Summary Presented**: Use a reliability-first LLM review budget: at most three logical attempts within a shared 90-second deadline; valid decisions stop retries; exhaustion falls back to user approval; the TUI shows review progress and permits immediate cancellation.
- **User Confirmation**: "采纳这个可靠性优先方案"
- **Entries Confirmed**: DEC-012

### Session 2026-07-12 22:50:10 CST

- **Summary Presented**: Reuse the active TUI provider and model for review, but run it with isolated context, no main conversation state, and no tools. Do not add a separate reviewer model or credential setting in the first version.
- **User Confirmation**: "采用方案A"
- **Entries Confirmed**: DEC-013

### Session 2026-07-12 23:08:10 CST

- **Summary Presented**: Replace a generic raw-JSON session fingerprint with typed approval keys: canonical command plus cwd for Bash, capability plus canonical path for file access, normalized tool-specific arguments for skills and delegation, and an exact canonical fallback for unknown tools. Keep all grants in memory and defer prefix, directory-wide, and persistent rules.
- **User Confirmation**: "采纳你的建议"
- **Entries Confirmed**: DEC-014

### Session 2026-07-12 23:15:07 CST

- **Summary Presented**: Correct the earlier rewind assumption: clear session grants on `/new`, `/clear`, and TUI exit; retain them across `/rewind`, `/compact`, and permission-mode changes; keep persisted permission mode independent.
- **User Confirmation**: "采用这个修正版的方案"
- **Entries Confirmed**: DEC-015

### Session 2026-07-12 23:20:30 CST

- **Summary Presented**: Use a conservative read-only Bash catalog with command-specific flag checks, structured shell parsing, workspace-contained path operands, and all-commands-safe handling for simple chains and pipelines. Unsupported syntax and non-fast-path commands go to review rather than being denied.
- **User Confirmation**: "采纳这套边界"
- **Entries Confirmed**: CON-005, DEC-016

### Session 2026-07-12 23:26:25 CST

- **Summary Presented**: Use low, medium, high, and critical risk thresholds with explicit authorization scoring. Automatically approve task-relevant low and medium actions and narrowly scoped high-risk actions with sufficient user authorization; escalate critical, suspicious, unrelated, unclear, or insufficiently authorized actions. Never automatically deny.
- **User Confirmation**: "采纳这套风险阈值"
- **Entries Confirmed**: DEC-017

### Session 2026-07-12 23:28:48 CST

- **Summary Presented**: Provide the reviewer a bounded transcript with separate user-message and tool-evidence budgets; retain initial and recent user intent, explicit authorization, recent execution context, exact action metadata, and truncation markers. Treat assistant, tool, Skill, and file content as untrusted evidence and escalate when retained context is insufficient.
- **User Confirmation**: "采用方案A"
- **Entries Confirmed**: CON-006, DEC-018

### Session 2026-07-12 23:31:42 CST

- **Summary Presented**: Scope the first version to the default interactive TUI and its inherited nested calls; default missing or invalid settings to `Ask for approval`; leave all non-interactive runtimes and existing middleware unchanged and unaffected by persisted TUI mode.
- **User Confirmation**: "采用方案A"
- **Entries Confirmed**: DEC-019, OUT-004

### Session 2026-07-12 23:33:21 CST

- **Summary Presented**: Serialize reviewed tool calls in model order; bypass the queue for deterministic or session-granted calls; re-evaluate policy before execution; continue after single-call denial; cancel the current turn and pending queue after feedback cancellation or review cancellation; never roll back completed calls.
- **User Confirmation**: "采用方案A"
- **Entries Confirmed**: DEC-020

### Session 2026-07-12 23:39:14 CST

- **Summary Presented**: Preserve Codex's three-mode `/permissions` picker while adding one bulk `Clear session approvals` action; do not build Claude Code's per-rule permission manager in the first version.
- **User Confirmation**: "采用方案B"
- **Entries Confirmed**: DEC-021

### Session 2026-07-12 23:43:49 CST

- **Summary Presented**: Show transient review progress and retry count; attach compact auto-approval and risk metadata to the existing tool-call entry with rationale in collapsed details; show escalation context and retry exhaustion in the approval prompt; expose mode and grant count through `/status`, add a non-default `permissions` statusline item, and force a bottom warning while Full Access is active.
- **User Confirmation**: "采纳"
- **Entries Confirmed**: NEED-002, DEC-022

### Session 2026-07-12 23:48:55 CST

- **Summary Presented**: Keep the selected Full Access mode persisted, but treat ordinary warning confirmation as current-run activation only. On later startup, an unremembered Full Access selection starts effectively in Ask mode and asks for confirmation again; `Enable and remember` permits direct future activation, and changing modes does not clear the separate acknowledgment.
- **User Confirmation**: "采用方案A"
- **Entries Confirmed**: DEC-023
- **Entries Superseded**: DEC-007

### Session 2026-07-12 23:52:30 CST

- **Summary Presented**: Define the first version as Codex-style three-mode approval with deterministic fast paths, an isolated LLM auto reviewer, and a FIFO approval queue. Exclude only Claude Code's general-purpose classifier-driven Auto mode and persistent permission-rule engine, rather than excluding all LLM review.
- **User Confirmation**: "按照建议修正。"
- **Entries Confirmed**: DEC-024, OUT-005
- **Entries Superseded**: DEC-001, OUT-001

### Session 2026-07-13 10:41:55 CST

- **Summary Presented**: Keep user-observable compatibility targets while removing source-code implementation claims from the requirements record. Require requirements, specification, plan, and task artifacts to describe FoxHarness behavior independently of Codex CLI or Claude Code source details; source research does not need to be persisted.
- **User Confirmation**: "按照此边界清理 requirements，源码研究可以不持久化到文档。之后的spec/plan/task文档内容也不应该出现与源码相关的描述"
- **Entries Confirmed**: CON-007
