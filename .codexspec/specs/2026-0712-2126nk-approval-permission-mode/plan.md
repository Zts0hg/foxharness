# Implementation Plan: Approval Permission Mode

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Related Spec**: `.codexspec/specs/2026-0712-2126nk-approval-permission-mode/spec.md`
**Confirmed Requirements**: `.codexspec/specs/2026-0712-2126nk-approval-permission-mode/requirements.md`
**Created**: 2026-07-13
**Status**: Draft

## Context

FoxHarness already centralizes built-in tool execution through `tools.Registry`, whose synchronous `Execute` path invokes middleware and then the tool. The interactive `AgentRunner` rebuilds a registry for every run, while its session, selected model, slash executor, and TUI live across runs. Subagents and forked skills create separate registries, so a TUI-only approval feature must inject one shared coordinator into both the main and nested registry builders rather than attach state to a single registry instance.

The engine currently executes tools marked parallel-safe in goroutines. `read_file` and `read_todo` are parallel-safe, which means concurrent registry entry cannot guarantee model-order FIFO review. The TUI also receives tool-call entries before execution through `engine.Reporter`, but those events omit tool-call IDs. Existing synchronous TUI interactions use channel bridges for `ask_user_question` and plan review; the permission prompt can follow this established pattern.

Settings already load from and atomically save to `~/.foxharness/settings.json`, preserving unknown JSON fields. Provider implementations expose one `Generate` interface and already perform transport-level retries. These existing boundaries allow the feature to remain TUI-only without modifying non-interactive behavior or adding a second provider configuration path.

## Goals / Non-Goals

**Goals:**

- Add a long-lived, TUI-scoped permission coordinator shared by main, delegated, Skill-forked, and nested tool execution.
- Implement deterministic workspace and Bash classification, typed session grants, serial review, isolated LLM review, and explicit user fallback.
- Persist selected permission mode and Full Access warning acknowledgment while keeping grants and review state in memory.
- Add `/permissions`, Full Access confirmation, approval prompts, review progress, audit annotations, `/status`, statusline, and bottom-warning integration.
- Preserve hard Plan mode and tool validation boundaries by placing permission review inside existing registry and lifecycle restrictions.
- Develop every behavioral slice with Red-Green-Refactor TDD and complete security-focused test coverage.

**Non-Goals:**

- OS-level filesystem, process, or network sandboxing.
- Permission integration for one-shot CLI, autodev, Feishu, AgentOps, bench, or other non-interactive entry points.
- A read-only user mode, generalized Auto classifier, persistent permission-rule engine, per-rule editor, or persistent grants.
- A separate reviewer provider, reviewer model setting, reviewer credentials, or reviewer tools.
- Persisting reviewer research, prompt transcripts, automatic-review annotations, or session grants beyond the current TUI process.

## Tech Stack

- **Language**: Go 1.25
- **TUI**: Bubble Tea and Lip Gloss already used by `internal/tui`
- **Model Provider**: Existing `provider.LLMProvider`
- **Persistence**: Existing atomic JSON settings writer in `internal/settings`
- **Tool Interception**: A TUI-only `tools.Registry` decorator around existing registries and middleware
- **Structured Shell Parsing**: Add `mvdan.cc/sh/v3/syntax`; do not implement shell parsing with regular expressions or token splitting
- **Testing**: Standard Go tests, table-driven policy tests, fake provider tests, TUI model/form tests, and `go test ./...`

## Verified Repository Constraints

- `AgentRunner.runInternal` constructs a fresh tool registry for each run, so coordinator state cannot live in a registry instance.
- `tools.Registry.Execute` owns the complete validation, middleware, and tool execution path. A registry decorator can authorize before delegating, while `middleware.BeforeExecute` alone cannot observe completion.
- Checkpoint middleware runs inside the base registry before file writes; a permission decorator outside that base prevents denied writes from creating checkpoint side effects.
- `AgentEngine.executeToolCalls` parallelizes only tools whose registry reports `IsParallelSafe`, so a TUI-only registry view can enforce model-order dispatch without changing the engine globally.
- `subagent.Manager` constructs independent registries and engines, so it needs explicit coordinator injection.
- Model-invoked `skill` calls execute shell embeddings, hooks, or fork dispatch inside `SkillTool.Execute`; approving the top-level skill call must happen before that pipeline, and forked nested tools must inherit the coordinator.
- The current TUI already bridges synchronous engine requests through channels and renders dedicated inline forms.
- Settings save is atomic and preserves unknown fields, but boolean false values need explicit merge handling so a warning acknowledgment can be reset.
- The current reporter surface omits tool-call IDs, so audit annotation requires an optional detailed reporter extension rather than name-and-order guessing.
- Provider transport retries are internal to each `Generate` call; reviewer logical retries must wrap those calls under one outer deadline.

## Architecture Overview

The implementation introduces `internal/permission` as a self-contained policy domain. A single coordinator is created only by `RunTUI`, attached to the long-lived runner, and reused by every registry built for that TUI process. Non-interactive runners leave the field nil and therefore retain current behavior.

For TUI-originated runs, a permission-aware registry view reports all tools as non-parallel-safe. The engine therefore reaches the coordinator in model call order. Calls that pass deterministic policy or match a grant return immediately without entering the coordinator's review queue; all others receive FIFO tickets and are processed one at a time. This trades parallel read throughput for deterministic approval order in the first version.

```
TUI model
  |  /permissions, warning forms, approval form, review events
  v
TUI permission bridge <------> permission.Coordinator
                                  |       |       |
                                  |       |       +--> in-memory typed grants
                                  |       +----------> isolated LLM reviewer
                                  +------------------> deterministic policy
                                                           |
AgentRunner -> Plan/allow-list -> permission registry -> checkpoint middleware -> tool
                     |
                     +-> Skill tool -> embedded pipeline / fork runner
                     |                                  |
                     +-> delegate_task -----------------+-> Subagent registry
                                                            uses same coordinator
```

**Covers**: REQ-001 through REQ-021, NFR-001 through NFR-004

## Component Structure

### 1. Permission Domain And Live State

Create `internal/permission` with mode normalization, selected/effective state, approval request and decision types, review results, event types, and concurrency-safe grant/state access. Keep this package independent of `internal/tui` and `internal/app`.

Planned files:

- `internal/permission/mode.go`
- `internal/permission/types.go`
- `internal/permission/state.go`
- `internal/permission/state_test.go`

The state object owns selected mode, effective mode, remembered-warning state, and grants. It exposes snapshots and mutations under a mutex. Mode changes do not clear grants. `ClearSessionGrants` is explicit and `ResetSession` clears only grants.

**Covers**: REQ-001, REQ-003, REQ-017, REQ-018, REQ-020

### 2. Canonical Paths, Grant Keys, And Deterministic Policy

Implement canonical workspace path resolution, typed grant-key generation, file-tool policy, trusted-tool bypass, and Bash classification in `internal/permission`. Path resolution must follow actual file-tool resolution, evaluate existing symlinks, resolve missing tails through the nearest existing ancestor, and verify containment with `filepath.Rel` rather than prefix comparison.

Planned files:

- `internal/permission/path.go`
- `internal/permission/grant.go`
- `internal/permission/policy.go`
- `internal/permission/shell.go`
- `internal/permission/path_test.go`
- `internal/permission/grant_test.go`
- `internal/permission/policy_test.go`
- `internal/permission/shell_test.go`

The shell classifier parses a complete command into an AST, rejects every unsupported node, validates each command and its flags with a command-specific validator, classifies filesystem operands, and requires every atomic command in a list or pipeline to pass. The AST printer provides canonical Bash text for grant keys when parsing succeeds; unparsable input uses an exact trimmed raw-command fallback so it cannot create broader equivalence.

Normalized JSON for `skill`, `delegate_task`, and policy-unknown advertised tools is produced by decoding and deterministically re-encoding values. File grant keys store capability plus canonical path; write and edit share one mutation capability. An advertised tool with no explicit policy rule requires review. A call name that is not registered or advertised retains the registry's existing not-found validation because it has no executable action to authorize.

**Covers**: REQ-004, REQ-006, REQ-007, REQ-017, NFR-001, NFR-002

### 3. Trust-Aware Evidence Builder And Isolated Reviewer

Build reviewer evidence from the active session message log and explicitly loaded project instructions. Map assistant tool-call IDs to later results so only direct results from `ask_user_question` are marked trusted; all other assistant content, tool results, Skill content, and file-derived content remain untrusted.

Planned files:

- `internal/permission/evidence.go`
- `internal/permission/reviewer.go`
- `internal/permission/evidence_test.go`
- `internal/permission/reviewer_test.go`
- `internal/context/prompt.go` and `internal/context/prompt_test.go` for a reusable project-instruction loader already used by the composer

Use separate configurable character budgets for trusted message evidence and untrusted execution evidence. Default plan-level limits are 16 KiB for trusted messages and 8 KiB for assistant/tool evidence. Preserve the first user objective, newest direct user messages, direct ask-user answers, and recent execution context; exact action terms and targets increase retention priority. Drop untrusted evidence before trusted evidence when both budgets are pressured. Every omission emits a typed truncation marker, and missing authorization forces escalation rather than optimistic inference.

The reviewer uses the provider returned by a live callback at review time. Each logical attempt sends a newly constructed system policy and evidence request with no main conversation state and no tools. It accepts only one strict JSON object matching the confirmed fields and enums. Provider errors, timeouts, nil responses, and invalid JSON/schema are technical failures. Valid `approve` and `escalate` stop retries.

**Covers**: REQ-009 through REQ-015

### 4. Coordinator, Queue, And Registry Adapter

Implement the coordinator as the only component that combines live mode, deterministic policy, grants, LLM review, and user prompting. A permission-aware `tools.Registry` decorator calls the coordinator before delegating to the base registry. The coordinator owns an explicit FIFO ticket queue and re-reads mode and grants immediately before review and execution authorization.

Planned files:

- `internal/permission/coordinator.go`
- `internal/permission/queue.go`
- `internal/permission/registry.go`
- `internal/permission/context.go`
- `internal/permission/coordinator_test.go`
- `internal/permission/queue_test.go`
- `internal/permission/registry_test.go`

The permission-aware registry delegates registration, middleware, and definitions to the wrapped registry, returns false from `IsParallelSafe`, and controls `Execute`. Each default, Formal Plan, checklist, and Subagent base registry is wrapped separately. Plan-lifecycle and allowed-tool wrappers remain outside permission registries, so unavailable tools fail their harder outer constraint before approval and Full Access cannot restore them.

Permission-registry `Execute` behavior:

1. Validate and canonicalize the exact call context.
2. Return allow immediately for effective Full Access, trusted tools, workspace-contained file tools, Bash fast-path calls, or matching grants.
3. Enqueue every remaining call with a cancellation-aware FIFO ticket.
4. Re-evaluate effective mode and grants when the ticket becomes active.
5. In Ask mode, invoke the user approver.
6. In Approve mode, emit review progress, run isolated review, allow exact approval, or invoke the user approver after escalation/exhaustion.
7. Convert deny-and-continue to a structured denied `schema.ToolResult`; convert feedback or cancellation to a typed turn-cancel result after the TUI has cancelled the run.
8. For known leaf tools, keep the active ticket until delegated base execution returns, then finish the ticket and wake the next request. For composite `skill` and `delegate_task` calls and policy-unknown advertised tools, finish the parent ticket after authorization but before delegated base execution so possible nested tool calls can enter the same coordinator without deadlock. The outer engine remains blocked on the call, so later sibling calls from the same model response cannot overtake it.

**Covers**: REQ-004, REQ-005, REQ-008, REQ-009, REQ-015, REQ-016, REQ-018, NFR-001, NFR-002

### 5. Runner, Subagent, Skill, And Plan-Lifecycle Integration

Add an optional coordinator field and setter to `AgentRunner`. `RunTUI` constructs and attaches it; all other constructors and entry points leave it nil. Refactor registry construction through a helper that wraps each completed base registry with the same coordinator. Use that helper for the default registry and for the separately constructed Formal Plan and checklist registries; those two registries do not currently inherit behavior from the default registry. Plan-lifecycle selection and any allowed-tool filter are composed outside the wrapped registries.

Planned changes:

- `internal/app/runner.go` and focused runner tests
- `internal/app/tui.go` and focused TUI bootstrap tests
- `internal/subagent/manager.go` and manager/filter tests
- `internal/app/plan_lifecycle.go` integration tests where required
- `internal/slash/skilltool/tool_test.go` and fork-runner tests for pre-pipeline and inherited enforcement

Execution context values carry invocation source, effective workspace/cwd, active session evidence source, and trusted project instructions without adding fields to `schema.ToolCall`. Main runs use a main-agent source; Subagent runs use a delegated source. The top-level `skill` call is reviewed before its executor pipeline, and a forked skill passes the same coordinator into its Subagent manager.

`AgentRunner.NewSession` calls `ResetSession` only after the new session succeeds. `RunTUI` clears grants on exit. Rewind, compaction, model changes, and collaboration-mode changes do not clear permission state. All three Plan-lifecycle registries use the coordinator, while Plan lifecycle and filtered wrappers remain outside the approval bypass, so Full Access cannot restore tools removed by Plan mode or an allow-list.

**Covers**: REQ-005, REQ-010, REQ-018, REQ-021, NFR-001, NFR-002

### 6. Settings Schema And Transactional Mode Changes

Extend `settings.TUISettings` and its raw merge logic:

```json
{
  "tui": {
    "permission_mode": "ask",
    "full_access_warning_acknowledged": false
  }
}
```

Planned files:

- `internal/settings/settings.go`
- `internal/settings/settings_test.go`
- `internal/tui/model.go` settings integration tests

Accepted values are `ask`, `approve`, and `full_access`; unknown values normalize to Ask in the permission domain. The merge path explicitly writes `false` when resetting an existing acknowledgment and continues preserving unknown fields.

TUI mode changes follow the existing theme/statusline transaction pattern: stage the next persisted values, save atomically, and update live selected/effective state only after save succeeds. Ordinary Full Access confirmation persists the selected mode but leaves acknowledgment false; remembered confirmation persists both before activation. On startup, selected Full Access with false acknowledgment creates effective Ask and schedules the warning immediately.

**Covers**: REQ-002, REQ-003

### 7. TUI Commands, Forms, Status, And Audit Rendering

Add dedicated forms rather than overloading the generic ask-user form:

- permission mode picker with the three modes and conditional `Clear session approvals`
- Full Access warning with current-run enable, enable-and-remember, and cancel
- tool approval form with the four confirmed decisions and optional feedback input

Planned files:

- `internal/tui/permission_bridge.go`
- `internal/tui/permission_form.go`
- `internal/tui/approval_form.go`
- `internal/tui/full_access_form.go`
- corresponding `*_test.go` files
- `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/statusline.go`, and focused model/view tests

The bridge follows existing synchronous channel patterns: the coordinator blocks on a per-request reply channel while Bubble Tea owns form state and user input. Feedback submission enqueues the feedback through the existing queued-prompt path before cancelling the active run. Plain Esc cancellation cancels the active run and pending permission requests without creating feedback or fallback prompts.

The TUI model adds permission state snapshots, pending review metadata keyed by tool-call ID, and form priority ahead of normal input. `/status` receives effective mode and grant count. `permissions` is added to the available statusline items but not the default collection. `renderKeybinds` or an adjacent bottom band adds an unconditional `[full access]` line whenever effective mode is Full Access.

Review events update transient footer status. Automatic approval stores risk and collapsed rationale on the existing tool-call entry. Fast-path events are not emitted. User prompts show canonical action, cwd, session-grant scope, and reviewer risk/rationale only when review produced them.

**Covers**: REQ-001 through REQ-003, REQ-008, REQ-018 through REQ-020, NFR-004

### 8. Tool-Call Correlation Without Breaking Existing Reporters

Add optional detailed reporter interfaces in `internal/engine/reporter.go` that receive complete `schema.ToolCall` and paired result data. `AgentEngine` detects the optional interface and uses it for TUI reporting while retaining the existing `Reporter` methods for Feishu, autodev, and other callers.

Planned files:

- `internal/engine/reporter.go`
- `internal/engine/loop.go`
- `internal/engine/reporter_test.go`
- `internal/tui/reporter.go`
- `internal/tui/reporter_test.go` and model/view audit tests

TUI entries gain an internal tool-call ID and optional review metadata. Because permission events and reporter events use separate channels, the model keeps unmatched review events in a pending map and applies them when the corresponding tool-call entry arrives. This avoids dependence on cross-channel scheduling or tool-name uniqueness.

**Covers**: REQ-019, NFR-004

## Interface Contracts

The following contracts are design targets; exact unexported naming may change during implementation without changing responsibilities.

### User Approval And Review

```go
type UserApprover interface {
    Approve(ctx context.Context, request ApprovalRequest) (UserDecision, error)
}

type Reviewer interface {
    Review(ctx context.Context, request ReviewRequest) (ReviewResult, error)
}

type EventSink interface {
    Publish(ctx context.Context, event ReviewEvent)
}
```

`UserApprover` is implemented by the TUI channel bridge. `Reviewer` is implemented by the isolated provider adapter. `EventSink` reports review start, retries, automatic approval, escalation, and exhaustion but emits nothing for deterministic fast paths.

**Covers**: REQ-008 through REQ-015, REQ-019

### Coordinator State And Session Lifecycle

```go
type Controller interface {
    Snapshot() StateSnapshot
    SetSelectedMode(mode Mode)
    Activate(mode Mode)
    SetWarningAcknowledged(value bool)
    GrantCount() int
    ClearSessionGrants() int
    ResetSession()
}
```

TUI persistence remains outside these mutation methods so the model can save first and commit live state second. Coordinator policy reads snapshots and grants under synchronization.

**Covers**: REQ-001 through REQ-003, REQ-017, REQ-018, REQ-020

### Execution Evidence Context

```go
type ExecutionContext struct {
    Source              InvocationSource
    Workspace           string
    CWD                 string
    Messages            EvidenceSource
    ProjectInstructions TrustedInstructionSource
}
```

Context helpers attach this value to main and Subagent engine contexts. Missing context is non-authorizing and causes any call needing contextual review to escalate.

**Covers**: REQ-005, REQ-013, REQ-014

### Optional Detailed Reporter

```go
type DetailedToolReporter interface {
    OnToolCallDetail(ctx context.Context, call schema.ToolCall)
    OnToolResultDetail(ctx context.Context, call schema.ToolCall, result schema.ToolResult)
}
```

Legacy reporter methods remain unchanged. The engine chooses detailed callbacks when implemented and legacy callbacks otherwise, preventing duplicate events.

**Covers**: REQ-019, NFR-004

## Plan-Level Decisions

### PLD-001: Use A New TUI-Scoped Permission Package

**Evidence**: Existing `internal/approval` manages synchronous external approval requests and is used by non-interactive integration, while this feature has modes, grants, LLM review, queueing, and TUI lifecycle.

**Decision**: Add `internal/permission`; leave `internal/approval` behavior unchanged.

**Rationale**: Separate ownership prevents the new TUI policy from changing non-interactive approval behavior and keeps the package responsibilities focused.

**Alternatives Considered**: Expand `internal/approval`; embed all policy in `internal/tui`; add policy directly to `tools.Registry`.

**Trade-off**: One additional package and adapter layer are introduced.

**Covers**: REQ-001, REQ-021, NFR-001

### PLD-002: Serialize TUI Sibling Tool Dispatch At The Registry Boundary

**Evidence**: The engine starts parallel-safe tool goroutines without a deterministic middleware-entry order, while reviewed calls must be FIFO in model order.

**Decision**: Wrap only TUI-originated main and nested registries with a permission registry whose `IsParallelSafe` always returns false. Hold review tickets through known leaf execution; release an authorized composite or policy-unknown parent before execution so possible child calls can use the same coordinator.

**Rationale**: Existing engine sequencing then establishes model order before coordinator queueing without a global engine protocol change.

**Alternatives Considered**: Add batch-order APIs to every registry wrapper; infer order from goroutine arrival; run concurrent review and reorder prompts later.

**Trade-off**: Parallel read throughput is reduced for interactive TUI runs. Non-interactive runtimes keep current parallelism, and later optimization can add batch-aware ordering without changing policy behavior.

**Covers**: REQ-016, REQ-021, NFR-002

### PLD-003: Authorize With A Registry Decorator Outside Existing Middleware

**Evidence**: Registry decorators are already used for per-run tool filtering. The registry owns the complete execution lifecycle, while middleware observes only the pre-execution phase and checkpoint middleware performs pre-write work.

**Decision**: Implement permission enforcement as a `tools.Registry` decorator that authorizes before delegating to the base registry. Keep Plan-lifecycle and allow-list wrappers outside it and checkpoint middleware inside it.

**Rationale**: Denied calls never reach existing middleware or tool side effects, leaf tickets can cover execution completion, composite calls can allow nested re-entry, and Plan mode plus allowed-tool registries remain stronger outer constraints.

Unregistered calls remain hard validation failures before approval. The default-review rule applies to registered tools that are unknown to the permission policy, so a filtered or nonexistent capability cannot be restored by approval or Full Access.

**Alternatives Considered**: Gate only in the TUI reporter; modify every tool; replace the registry implementation.

**Trade-off**: The decorator must construct structured denied and cancellation results consistently with the base registry and distinguish composite from leaf execution.

**Covers**: REQ-004, REQ-005, REQ-016, NFR-001, NFR-002

### PLD-004: Use A Full Shell AST With Command-Specific Validators

**Evidence**: The confirmed grammar includes chains and pipelines while rejecting substitutions, redirection, executable paths, and command-specific unsafe options.

**Decision**: Add a maintained shell syntax parser and validate a strict AST subset plus per-command flags and operands.

**Rationale**: Regular expressions and token splitting cannot safely distinguish all confirmed constructs or produce stable canonical commands.

**Alternatives Considered**: Regex allow-list; invoking the shell in a dry-run mode; approving every Bash call.

**Trade-off**: One new Go module dependency and a sizable table-driven validator suite are required.

**Covers**: REQ-006, REQ-007

### PLD-005: Reuse The Live Provider Through A Tool-Free Adapter

**Evidence**: `AgentRunner` swaps its provider on `/model`, and all provider implementations share `Generate` with internal transport retries.

**Decision**: Resolve the current provider at each logical review attempt and call it with a fresh isolated message list and no tools.

**Rationale**: This follows active model changes, avoids additional credentials, and keeps reviewer logical retry separate from transport retry.

**Alternatives Considered**: Cache the provider at startup; add reviewer-specific settings; reuse the main conversation.

**Trade-off**: Three logical attempts may each include provider transport retries, but the shared 90-second context deadline bounds the total review.

**Covers**: REQ-009, REQ-010, REQ-015

### PLD-006: Build Reviewer Evidence From Typed Session Records

**Evidence**: Session message records preserve roles, tool-call IDs, and tool-result IDs; project instructions are already loaded by the prompt composer.

**Decision**: Reuse a project-instruction loader and construct bounded typed evidence from message records, explicitly marking trust and truncation.

**Rationale**: Typed evidence can distinguish direct user text and ask-user answers from untrusted assistant, tool, Skill, and file content without sending the unbounded conversation.

**Alternatives Considered**: Latest message only; full transcript; pre-summarize with the main agent.

**Trade-off**: Bounded evidence can omit context and increase escalation frequency; it never increases automatic authority.

**Covers**: REQ-013, REQ-014

### PLD-007: Keep Grants As Typed In-Memory Keys

**Evidence**: Equivalence differs by tool, and persistent or broad grants are excluded.

**Decision**: Generate comparable typed keys per tool family and store them only in the coordinator state.

**Rationale**: Exact typed equivalence allows useful session reuse without broadening path, cwd, tool, or argument scope.

**Alternatives Considered**: Raw JSON equality only; command-prefix rules; directory and tool-wide grants; settings persistence.

**Trade-off**: Semantically similar but noncanonical calls may prompt again, which is safer than overmatching.

**Covers**: REQ-017, REQ-018

### PLD-008: Correlate Audit Events By Tool-Call ID

**Evidence**: Existing reporter events are name-based and all call entries are emitted before execution, while permission review happens during execution and can involve duplicate tool names.

**Decision**: Add an optional detailed reporter interface and key TUI audit metadata by tool-call ID, with a pending map for cross-channel ordering.

**Rationale**: Exact correlation avoids annotating the wrong entry and leaves non-TUI reporters unchanged.

**Alternatives Considered**: Match the latest tool name and arguments; add review rows; replace the reporter interface globally.

**Trade-off**: The engine gains a small optional interface branch and TUI entries gain internal metadata.

**Covers**: REQ-019, NFR-004

### PLD-009: Persist Before Committing Live Mode State

**Evidence**: Existing theme and statusline flows restore prior state when settings save fails.

**Decision**: Use the same transaction ordering for mode and remembered-warning changes.

**Rationale**: The UI never reports durable state that was not written, and restart behavior remains predictable.

**Alternatives Considered**: Change live state first and retry persistence asynchronously; ignore save failure.

**Trade-off**: A filesystem error prevents the requested state transition until the user retries or fixes settings access.

**Covers**: REQ-002, REQ-003

## Implementation Phases

Each phase follows Red-Green-Refactor. Tests listed first must fail for the expected missing behavior before implementation begins.

### Phase 1: Domain State And Settings

1. Write failing tests for mode normalization, selected/effective split, Full Access startup state, acknowledgment reset, mode-change grant retention, grant clearing, settings round-trip, false acknowledgment merge, unknown-field preservation, and invalid-mode defaulting.
2. Implement permission state types and settings schema/merge behavior.
3. Refactor synchronization and persistence helpers while keeping tests green.

**Covers**: REQ-001 through REQ-003, REQ-017, REQ-018

### Phase 2: Canonical Policy And Bash Fast Path

1. Write failing table tests for workspace containment, `..` escapes, absolute and missing paths, symlink escapes, every allowlisted command, every denied option, chain/pipeline all-safe behavior, unsupported AST nodes, and canonical grant keys.
2. Add the shell parser dependency and implement path, policy, command validators, and typed keys.
3. Refactor validators into small command-specific units and run focused race-safe tests.

**Covers**: REQ-004, REQ-006, REQ-007, REQ-017, NFR-001

### Phase 3: Coordinator, FIFO, Grants, And Cancellation

1. Write failing tests for fast-path bypass, Ask routing, Approve routing, exact allow, session grant reuse, deny-and-continue, feedback cancellation, reviewer cancellation, mode/grant re-evaluation, one active request, FIFO order, Full Access bypass limits, and no rollback behavior.
2. Implement queue, coordinator authorization, permission-aware registry, context helpers, and typed cancellation results.
3. Refactor queue ownership and cancellation cleanup under `go test -race` focused on `internal/permission`.

**Covers**: REQ-004, REQ-005, REQ-008, REQ-009, REQ-015 through REQ-018, NFR-001, NFR-002

### Phase 4: Evidence And LLM Reviewer

1. Write failing tests for trust labeling, ask-user answer recognition, first/latest message retention, separate budgets, truncation markers, insufficient-evidence escalation, strict output parsing, risk matrix, live provider lookup, no tools, logical retry count, shared deadline, transport-retry independence, and exhaustion fallback.
2. Implement evidence builder, reusable project-instruction loading, reviewer prompt, strict parser, and retry loop.
3. Refactor prompt construction and budget selection without weakening escalation rules.

**Covers**: REQ-009 through REQ-015

### Phase 5: Main And Nested Runtime Wiring

1. Write failing runner and Subagent tests showing TUI-only attachment, authorization-before-checkpoint ordering, coordinator presence in default, Formal Plan, checklist, and Subagent registries, outer Plan/allow-list enforcement, model-switch provider lookup, new-session grant clearing, rewind/compact retention, Plan mode hard constraints, top-level Skill review before shell/hooks, composite nested re-entry, and inherited fork/delegate enforcement.
2. Add optional coordinator injection to runner, registry builders, Subagent manager, fork runner, and execution contexts.
3. Refactor shared construction helpers while proving non-interactive registry behavior remains unchanged.

**Covers**: REQ-005, REQ-010, REQ-018, REQ-021, NFR-001, NFR-002

### Phase 6: TUI Commands, Forms, Audit, And Status

1. Write failing model/form/view tests for `/permissions`, disabled and counted clear action, all prompt decisions, feedback entry, Full Access current-run and remembered paths, startup warning, save rollback, transient review/retry status, tool-ID audit correlation, collapsed rationale, escalation details, `/status`, optional statusline item, and bottom warning.
2. Implement bridges, forms, detailed reporter callbacks, entry metadata, command dispatch, settings transaction, status surfaces, and rendering.
3. Refactor form navigation and event handling while checking narrow terminal widths and cancellation deadlocks.

**Covers**: REQ-001 through REQ-003, REQ-008, REQ-015, REQ-018 through REQ-020, NFR-004

### Phase 7: End-To-End Security And Regression Verification

1. Add cross-package tests covering a complete Ask flow, automatic approval flow, reviewer exhaustion flow, nested delegation, model switch, new session, and Full Access restart behavior with fake providers and temporary settings.
2. Run `gofmt -w` on changed Go files, focused package tests, `go test -race` for permission/TUI concurrency paths, `go test ./...`, and `go vet ./...`.
3. Review security-sensitive code for fail-closed parsing, trust-boundary violations, registry-wrapper ordering, cancellation leaks, and accidental non-interactive activation.

**Covers**: REQ-001 through REQ-021, NFR-001 through NFR-004

## Verification Strategy

### Unit Tests

- `internal/permission`: exhaustive policy, parser, path, key, state, evidence, reviewer, queue, retry, and cancellation tests.
- `internal/settings`: schema parsing, invalid values, atomic merge, explicit false reset, and unknown-field preservation.
- `internal/tui`: form key handling, mode transactions, event correlation, rendering, statusline, status overview, and narrow-width behavior.
- `internal/context`: shared project-instruction loading behavior.

**Covers**: REQ-001 through REQ-020, NFR-001 through NFR-004

### Integration Tests

- `internal/app`: TUI-only coordinator attachment, main registry ordering, checkpoint ordering, session lifecycle, and current provider lookup.
- `internal/subagent` and `internal/slash/skilltool`: inherited permission registry and pre-pipeline review.
- `internal/engine`: optional detailed reporter dispatch without duplicate legacy events.

**Covers**: REQ-005, REQ-010, REQ-016, REQ-018, REQ-019, REQ-021, NFR-002, NFR-004

### Full Verification

- `go test ./...`
- `go vet ./...`
- Focused `go test -race` for `internal/permission`, `internal/tui`, `internal/app`, and `internal/subagent`
- Manual TUI smoke test for picker layout, approval form, review status, Full Access warning, statusline, and cancellation

**Covers**: REQ-001 through REQ-021, NFR-001 through NFR-004

## Security Considerations

- Treat every classification failure as review-required, never safe by default.
- Canonicalize path operands with symlink awareness before checking workspace containment or building grants.
- Bind reviewer approval and allow-once to the exact canonical call, cwd, workspace, and source evaluated.
- Keep reviewer context trust labels explicit and prevent untrusted content from being copied into trusted sections.
- Use a hard, tool-free reviewer prompt and strict result schema; malformed content is a technical failure, not approval.
- Keep permission registries outside checkpoint middleware and tool side effects while leaving Plan/allow-list constraints outside permission bypass.
- Preserve Plan mode and allowed-tool filtering outside permission bypass, including Full Access.
- Never restore grants from settings, messages, transcript, rewind state, or project files.
- Keep non-interactive runners coordinator-free and add regression tests that assert no persisted TUI mode reaches them.

**Covers**: REQ-004 through REQ-018, REQ-021, NFR-001 through NFR-003

## Performance Considerations

- TUI tool calls become model-order serial in the first version. This removes parallel read execution but simplifies correct review ordering and avoids stale-side-effect approvals.
- Deterministic fast paths and grants avoid LLM and user latency even though execution remains serial.
- Reviewer evidence is bounded before provider invocation; tool output cannot consume the trusted-message budget.
- The shared 90-second review deadline bounds nested provider retries and logical retries.
- Settings writes occur only on explicit mode or acknowledgment changes, not per tool call.

**Covers**: REQ-009, REQ-013, REQ-015, REQ-016, NFR-004

## Risks / Trade-offs

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| A shell validator accidentally treats a side-effecting form as read-only | Medium | High | AST allow-list, command-specific option tests, fail-closed defaults, and security review. |
| Path canonicalization differs from actual tool resolution | Medium | High | Share one tested resolver contract based on current file-tool behavior and cover missing/symlink paths. |
| TUI request/reply channels deadlock during cancellation | Medium | High | Buffered per-request replies, context-aware sends/waits, typed cancellation, race tests, and idempotent queue cleanup. |
| Review events arrive before reporter tool entries | High | Medium | Correlate by tool-call ID and retain unmatched audit events until the entry arrives. |
| Provider retries make logical review feel slow | Medium | Medium | One shared 90-second context, visible attempt status, immediate Esc cancellation, and user fallback. |
| Bounded evidence omits useful authorization | Medium | Medium | Favor trusted messages, preserve first/latest and action-relevant evidence, mark truncation, and escalate on insufficiency. |
| Selected and effective mode diverge unexpectedly | Medium | High | Explicit state snapshot, transactional persistence, startup tests, and visible warning/pending state. |
| Nested registry is created without coordinator | Medium | High | One injected coordinator field, constructor helpers, delegation/fork integration tests, and default-review behavior for nested tools. |
| Serial TUI execution reduces throughput | High | Low | Accepted first-version trade-off; preserve non-interactive parallelism and keep a future batch-aware optimization path. |

## Assumptions

- The current file tools remain rooted through `filepath.Join(workDir, suppliedPath)` during this feature. If implementation changes that resolution, the permission resolver and tool behavior must be changed together.
- The existing provider implementations are safe for a review call while the main engine is blocked waiting for authorization; the same provider is not used concurrently by the blocked main request.
- Automatic-review audit metadata is required for the active TUI transcript only and is not restored after process restart.
- The warning acknowledgment is resettable through settings state, but the first-version `/permissions` menu does not add a separate reset-warning action beyond the confirmed mode picker and grant clearing.

## Requirements Coverage

| Spec Requirement | Plan Coverage | Reference |
|------------------|---------------|-----------|
| REQ-001 | Full | Components 1, 6, 7; PLD-001; Phases 1 and 6 |
| REQ-002 | Full | Component 6; PLD-009; Phases 1 and 6 |
| REQ-003 | Full | Components 1, 6, 7; PLD-009; Phases 1 and 6 |
| REQ-004 | Full | Components 2 and 4; PLD-003; Phases 2 and 3 |
| REQ-005 | Full | Components 4 and 5; PLD-003; Phases 3 and 5 |
| REQ-006 | Full | Component 2; PLD-004; Phase 2 |
| REQ-007 | Full | Component 2; PLD-004; Phase 2 |
| REQ-008 | Full | Components 4 and 7; User Approval interface; Phases 3 and 6 |
| REQ-009 | Full | Components 3 and 4; PLD-005; Phases 3 and 4 |
| REQ-010 | Full | Components 3 and 5; PLD-005; Phases 4 and 5 |
| REQ-011 | Full | Component 3; PLD-005; Phase 4 |
| REQ-012 | Full | Component 3; Phase 4 |
| REQ-013 | Full | Component 3; PLD-006; Evidence Context; Phase 4 |
| REQ-014 | Full | Component 3; PLD-006; Evidence Context; Phase 4 |
| REQ-015 | Full | Components 3, 4, and 7; PLD-005; Phases 3, 4, and 6 |
| REQ-016 | Full | Component 4; PLD-002 and PLD-003; Phase 3 |
| REQ-017 | Full | Components 1 and 2; PLD-007; Phases 1 through 3 |
| REQ-018 | Full | Components 1, 4, 5, and 7; PLD-007; Phases 3, 5, and 6 |
| REQ-019 | Full | Components 7 and 8; PLD-008; Phase 6 |
| REQ-020 | Full | Components 1 and 7; Phase 6 |
| REQ-021 | Full | Components 5; PLD-001 and PLD-002; Phase 5 |
| NFR-001 | Full | Components 2, 4, and 5; PLD-001 and PLD-003; Security Considerations |
| NFR-002 | Full | Components 2, 4, and 5; PLD-002 and PLD-003; Phases 3 and 5 |
| NFR-003 | Full | Context, documentation constraint throughout plan, Security Considerations |
| NFR-004 | Full | Components 7 and 8; PLD-008; Phase 6 |

## Unresolved Items

No unresolved product or technical decisions block task generation. The implementation must preserve the explicit assumptions above and stop for re-planning if verified repository behavior invalidates them.
