# Implementation Plan: Progressive Plan Mode

<!--
Language: English, per .codexspec/config.yml language.document.
-->

**Feature ID**: `2026-0711-1441tc`
**Feature Branch**: `2026-0711-1441tc-progressive-plan-mode`
**Related Spec**: `.codexspec/specs/2026-0711-1441tc-progressive-plan-mode/spec.md`
**Confirmed Requirements**: `.codexspec/specs/2026-0711-1441tc-progressive-plan-mode/requirements.md`
**Created**: 2026-07-11
**Status**: Draft

## Fidelity Check

The confirmed requirements and specification agree on the implementation boundary:

- Formal Plan is selected explicitly in the TUI and is never inferred from task difficulty or entered by `update_todo`. Covers: REQ-001, REQ-002, REQ-010
- Formal Plan is behaviorally read-only through high-priority instructions and a reduced tool surface; Bash is retained without a sandbox guarantee. Covers: REQ-004, REQ-005, NFR-001
- A complete proposal is persisted and reviewed through `submit_plan`; revision is feedback plus full replacement, and approval continues the same task in Default mode. Covers: REQ-006, REQ-007, REQ-008, NFR-002
- The approved proposal is converted semantically by the Agent into `TODO.md` through the existing `update_todo` tool before implementation. Covers: REQ-009, REQ-010
- The legacy Planner, `EnablePlanMode`, and `-plan` flag are removed consistently while session artifacts, snapshots, `/rewind`, Feishu, and Autodev behavior remain. Covers: REQ-011, REQ-012, REQ-013, REQ-014, NFR-003
- The project constitution requires deterministic Red-Green-Refactor development for every behavior change. Covers: NFR-004, NFR-005

No confirmed open question blocks technical planning.

## Context

The current `AgentRunner` captures a legacy `enablePlanMode` boolean and, when enabled, runs `memory.Planner.BuildPlan` as a separate model request before constructing the main engine. The TUI toggles that boolean, while CLI parsing defaults `-plan` to true. AgentOps and benchmark duplicate the same automatic pre-pass.

The engine currently composes one system prompt and uses one registry object for a run, but it asks the registry for available tools at the start of every turn. That turn boundary permits a small lifecycle registry to change the model-visible tool set after a blocking `submit_plan` decision while retaining the same engine run, message history, run ID, reporter, compactor, checkpoint, and automemory hooks.

## Goals / Non-Goals

**Goals:**

- Represent Default and Formal Plan with explicit collaboration-mode state.
- Keep one user submission, one engine run, and one conversation context across planning, approval, TODO initialization, and implementation.
- Persist and display the exact successful plan submission from one payload.
- Prevent explicit implementation tools from becoming available until approval and successful TODO initialization.
- Remove all legacy automatic planning calls and startup configuration.
- Preserve existing session artifact and rewind infrastructure.

**Non-Goals:**

- OS sandboxing, Bash command classification, and generic tool approval.
- `update_plan`, `edit_plan`, incremental `PLAN.md` mutation, or `$EDITOR` integration.
- Formal Plan startup flags for one-shot CLI, AgentOps, benchmark, Feishu, or Autodev.
- A task-difficulty classifier or automatic collaboration-mode transition.

## Existing Repository Constraints

- `internal/app/runner.go` serializes runs with `runMu`, snapshots PLAN/TODO before the user message, composes prompts, builds registries, and owns mutable session/runtime state. Covers: REQ-003, REQ-008, REQ-012, REQ-014
- `internal/engine/loop.go` persists the user message once, reloads session history, asks the registry for definitions once per turn, and appends tool calls/results to the same model-visible context. Covers: REQ-008, REQ-009, NFR-005
- `internal/tools` already contains registry decorators, TODO tools, and the synchronous `UserAsker` interface pattern. Covers: REQ-005, REQ-006, REQ-010, NFR-005
- `internal/tui/asker.go` and `askform.go` demonstrate a blocking engine-to-Bubble-Tea request/reply bridge that can be mirrored for plan confirmation. Covers: REQ-006, REQ-007, REQ-008
- `internal/memory/store.go` owns session-local `PLAN.md` and `TODO.md`; `state_history.go` snapshots and restores both artifacts for `/rewind`. Covers: REQ-006, REQ-010, REQ-014
- `RunRestricted` can wrap a registry with an allow-list. Any lifecycle registry must preserve that decorator's restrictions and turn-boundary behavior. Covers: REQ-003, REQ-005
- The current TODO completion gate only acts when `update_todo` is visible and real incomplete items exist; it cannot by itself require initial TODO creation after approval. Covers: REQ-009
- Project constitution 2.0.0 mandates test-first implementation, deterministic critical/error-path tests, focused interfaces, and Go block-level documentation. Covers: NFR-004, NFR-005

## Architecture Overview

```text
TUI selected mode                         one AgentEngine.RunWithReporter
┌──────────────────┐                    ┌──────────────────────────────────┐
│ Default          │── submit task ───►│ full Default registry           │
│ Formal Plan      │── submit task ───►│                                  │
└──────────────────┘                    │ Formal registry                  │
       ▲                                │ read_file | bash | ask | submit  │
       │ /plan, /plan off, Shift+Tab    │              │                   │
       │                                │ submit_plan blocks on TUI review │
       │                                │       revise ───┘                │
       │                                │       approve                    │
       │                                │          ▼ next turn             │
       └──── approval selects Default ──│ Checklist gate registry          │
                                        │ read-only exploration + TODO     │
                                        │          │ update_todo succeeds   │
                                        │          ▼ next turn             │
                                        │ full Default registry            │
                                        └──────────────────────────────────┘
```

The active run captures the selected collaboration mode at submission. Ordinary mode changes mutate only the selection used by the next submission. Plan approval is represented inside the captured Formal lifecycle and is the sole transition that advances the same active run to Default behavior. Covers: REQ-001, REQ-002, REQ-003, REQ-008, REQ-009

## Plan-Level Decisions

### PLD-001: Use A Shared Collaboration Mode Type And Separate Selected From Active State

**Decision**: Add `internal/collaboration` with `ModeDefault` and `ModeFormalPlan`. `AgentRunner` stores the selected mode for the next submission; each run captures that value into immutable normal-run behavior or a private Formal lifecycle. The TUI stores the same selected mode for rendering and commands.

**Evidence**: The current boolean is duplicated in CLI config, runner, and TUI, while `runMu` already gives each submission a stable execution boundary.

**Rationale**: Separate selected and active state directly implements active-run deferral without allowing key presses to mutate an existing registry.

**Trade-off**: The shared package adds a small domain type, but avoids string/boolean drift between app and TUI packages.

Covers: REQ-001, REQ-002, REQ-003, REQ-012

### PLD-002: Continue Approval In The Same Engine Run With A Turn-Aware Registry

**Decision**: A Formal submission uses a per-run lifecycle registry with three phases: `formal`, `checklist`, and `default`. A successful approval records a pending `formal -> checklist` transition; a successful `update_todo` records `checklist -> default`. The engine invokes an optional registry `BeginTurn` hook immediately before reading tool definitions, and filtered registries delegate that hook. Pending transitions are committed only there.

**Evidence**: `AgentEngine` asks `GetAvailableTools` at each turn, while one `RunWithReporter` already preserves the complete tool-call/result context. Delaying transition until the next turn also prevents a hallucinated write tool in the same tool-call batch as `submit_plan` or `update_todo` from executing against the newly opened surface.

**Alternatives Considered**:

1. End Formal planning and launch a second hidden run. Rejected because it adds a synthetic user message/run boundary, duplicates snapshot/extraction lifecycle, and weakens the same-task presentation.
2. Mutate the registry immediately inside `submit_plan`. Rejected because later calls from the same model response could execute with permissions the model was not offered for that turn.

**Trade-off**: The engine gains one small optional lifecycle hook, but no provider protocol or message schema changes.

Covers: REQ-003, REQ-005, REQ-008, REQ-009, NFR-005

### PLD-003: Gate Explicit Implementation Tools Until TODO Initialization

**Decision**: After approval, the lifecycle enters Default collaboration mode but initially exposes a checklist gate containing `read_file`, `bash`, `ask_user_question`, `read_todo`, and `update_todo`. `write_file`, `edit_file`, subagent delegation, and skill invocation become visible only on the turn after `update_todo` succeeds. A run-specific completion gate reminds the Agent instead of accepting a final response while `submit_plan` or initial `update_todo` is still required.

**Evidence**: Instructions alone cannot guarantee ordering, and the existing TODO finalization gate ignores the initial `Not recorded` placeholder. A turn-delayed registry gate enforces ordering for explicit implementation tools without parsing Markdown.

**Rationale**: This provides deterministic coverage of the required ordering while still allowing the confirmed read-only revalidation before TODO initialization.

**Trade-off**: Bash remains available and can mutate state if the model violates instructions; this is the confirmed soft read-only boundary, not a sandbox.

Covers: REQ-004, REQ-008, REQ-009, REQ-010, NFR-001, NFR-005

### PLD-004: Treat `submit_plan` As An Atomic Full-Replacement Transaction

**Decision**: Extend `memory.Store` with an atomic exact-content PLAN replacement operation. `submit_plan(plan_markdown)` rejects whitespace-only input, atomically persists the original string without newline/content rewriting, then sends that same string to a `PlanReviewer`. The reviewer result is either approval or revision feedback; only approval schedules the lifecycle transition.

**Evidence**: Direct `os.WriteFile` can truncate an existing artifact before returning an error, which would violate preservation of the last successful plan. The settings package already establishes atomic replacement as a repository pattern.

**Rationale**: One payload instance feeds persistence and confirmation, making divergence structurally difficult and preserving the prior artifact on write failure.

**Trade-off**: `PLAN.md` may intentionally preserve a payload without a trailing newline; byte fidelity is more important than normalization for this contract.

Covers: REQ-006, REQ-007, REQ-010, REQ-014, NFR-002, NFR-005

### PLD-005: Use A Dedicated Plan Review Bridge And Inline TUI Form

**Decision**: Add a small synchronous `PlanReviewer` interface beside the tool contract and implement it with a TUI channel bridge modeled on `Asker`. The inline form renders the submitted Markdown, supports scrolling, offers `Approve` and `Continue planning`, and lets the user attach revision feedback to the latter decision. Empty feedback still means continue planning without approval; when feedback is supplied, it is returned unchanged to the Agent. Cancelling the form never approves or exits Formal Plan.

**Evidence**: The existing ask bridge already safely blocks a tool goroutine while Bubble Tea remains responsive, but its multiple-choice data model does not provide a stable full-plan review surface.

**Rationale**: A dedicated bridge keeps `internal/tools` independent of Bubble Tea and makes exact displayed content and decisions deterministic in tests.

**Trade-off**: This adds one focused form rather than generalizing all interactive overlays prematurely.

Covers: REQ-006, REQ-007, REQ-008, NFR-002, NFR-005

### PLD-006: Reinject The Complete Approved Plan After Compaction

**Decision**: On approval, the Formal lifecycle queues a one-shot runtime reminder containing the complete approved plan and the `update_todo`-before-implementation instruction. The runner combines this source with existing skill-activation reminders. The engine injects it after turn compaction, before the first checklist-gate model call.

**Evidence**: The original `submit_plan` arguments remain in model-visible history, but turn compaction may summarize them before the post-approval call. `NextTurnReminders` already injects runtime context after compaction.

**Rationale**: This guarantees the complete approved proposal is available for semantic TODO derivation without runtime Markdown parsing or a second run.

**Trade-off**: The approved plan appears both in the prior tool arguments and one runtime reminder for that boundary turn.

Covers: REQ-008, REQ-009, NFR-005

### PLD-007: Keep Formal Instructions Conditional Within One System Prompt

**Decision**: Extend the prompt composer with collaboration guidance. A Formal run receives an explicit high-priority lifecycle section that overrides general editing guidance before approval; prohibits project-file creation/modification, Git state changes, and implementation-purpose side effects; defines permitted read-only Bash use; requires complete `submit_plan`; explains revision/approval tool results; and requires `update_todo` before implementation after approval. Mode-specific session-memory guidance avoids telling the pre-approval Agent to mutate TODO.

**Evidence**: The engine composes one system message per run; the same run must remain coherent after its tool surface transitions.

**Rationale**: Conditional instructions align the fixed system message with all lifecycle phases while the registry provides concrete tool visibility.

**Trade-off**: The contract remains behavioral for Bash, exactly as confirmed.

Covers: REQ-004, REQ-005, REQ-008, REQ-009, REQ-010, NFR-001

### PLD-008: Remove Legacy Planning Without Replacing It In Non-TUI Entry Points

**Decision**: Remove `Planner.BuildPlan` calls, `internal/memory/plan.go` and its tests, `EnablePlanMode` fields/accessors, and `-plan` registration. CLI retains independent `-thinking`; AgentOps and benchmark run their normal engine directly with thinking disabled as today after successful legacy planning. Benchmark gains TODO tools; AgentOps keeps its existing TODO tools; Feishu and Autodev retain their current workflows.

**Evidence**: Formal review requires an interactive TUI reviewer and the confirmed scope explicitly removes startup and automatic pre-planning paths.

**Rationale**: One meaning of Formal Plan remains across the product, while Default avoids the extra model call.

**Trade-off**: Existing scripts that pass `-plan` fail immediately, as confirmed.

Covers: REQ-011, REQ-012, REQ-013, NFR-003

## Components And Interfaces

### C1: Collaboration Mode Domain

Create `internal/collaboration/mode.go` with the two supported mode constants and stable display/string helpers. Replace the runner and TUI Plan boolean APIs with `CollaborationMode()` and `SetCollaborationMode(mode)`. New and resumed runners, TUI construction, and `NewSession` select Default.

Covers: REQ-001, REQ-002, REQ-003, REQ-012

### C2: Plan Persistence And Submission Tool

Add atomic `memory.Store.ReplacePlan(content string)` behavior and `internal/tools/submit_plan.go` with:

```go
type PlanReviewDecision string

const (
    PlanApproved        PlanReviewDecision = "approved"
    PlanContinuePlanning PlanReviewDecision = "continue_planning"
)

type PlanReview struct {
    Decision PlanReviewDecision
    Feedback string
}

type PlanReviewer interface {
    ReviewPlan(ctx context.Context, planMarkdown string) (PlanReview, error)
}
```

The tool schema exposes only required `plan_markdown`. Its store and reviewer dependencies are interfaces/fakes in unit tests. Persistence happens before review; write failure skips review and returns an error. Revision returns feedback to the model, while approval notifies the per-run lifecycle.

Covers: REQ-005, REQ-006, REQ-007, REQ-010, REQ-014, NFR-002, NFR-005

### C3: Formal Run Lifecycle Registry

Add a private app-layer registry/controller that delegates `tools.Registry` methods to one immutable phase registry. Add an optional `BeginTurn()` interface in `internal/tools`; `AgentEngine` invokes it once per turn and `filteredRegistry` delegates it. Phase behavior is:

| Phase | Visible tools | Exit condition |
|-------|---------------|----------------|
| Formal | `read_file`, `bash`, `ask_user_question`, `submit_plan` | User approves a successfully persisted plan |
| Checklist gate | `read_file`, `bash`, `ask_user_question`, `read_todo`, `update_todo` | `update_todo` succeeds |
| Default | Existing full runner registry, without `submit_plan` | Normal run completion |

Unknown or phase-ineligible calls return ordinary registry errors. Before starting a restricted Formal run, the runner validates that the slash-command allow-list contains every required canonical Formal tool; otherwise it returns a clear error without calling the model. Accepted allow-lists remain outer decorators and never broaden a lifecycle surface. Compatibility aliases such as `AskUserQuestion` may remain advertised in addition to the four required canonical names.

Covers: REQ-003, REQ-005, REQ-008, REQ-009, REQ-010, NFR-001, NFR-005

### C4: Engine Completion Gate And Reminder Composition

Add an optional run-local completion-gate callback to `engine.Config`. Before accepting a no-tool final response, the engine asks the gate for a blocking reminder. It injects one reminder and retries; a repeated unsatisfied final response returns a deterministic error, matching the bounded behavior of the existing TODO completion gate. The Formal lifecycle uses it to require `submit_plan` before approval and `update_todo` after approval.

Compose lifecycle one-shot reminders with `AgentRunner.drainPendingActivations` rather than replacing skill reminders. No provider, wire schema, transcript schema, or public session format changes.

Covers: REQ-006, REQ-008, REQ-009, NFR-005

### C5: Prompt Composer Collaboration Guidance

Extend `internal/context/prompt.go` with a typed collaboration option and focused Formal lifecycle text. Default composition retains existing behavior with no Formal section. Formal composition omits conflicting pre-approval TODO mutation guidance and clearly states the post-approval checklist contract.

Covers: REQ-004, REQ-005, REQ-008, REQ-009, REQ-010, NFR-001

### C6: TUI Mode Controls And Plan Review Form

Update `internal/tui` as follows:

- Add `/plan` and `/plan off` command handling and help rows.
- Keep primary-input `Shift+Tab` as the mode toggle; sidebar-local `Shift+Tab` and all Esc behavior remain unchanged.
- While a run is active, mode controls update only the next-submission selection and corresponding footer text.
- Add a `PlanReviewer` request/reply bridge and inline `planReviewForm` that displays the exact submitted source, renders Markdown for reading, supports scrolling, and returns approval or revision feedback.
- On approval, update the selected mode/footer to Default while the same run continues.
- Refresh sidebar PLAN content through the existing document reload path; do not add direct editing.
- Update `/status` and the bottom mode row to read collaboration-mode state.

Covers: REQ-001, REQ-002, REQ-003, REQ-006, REQ-007, REQ-008, REQ-014, NFR-002, NFR-005

### C7: Runner And Entry-Point Migration

Update `internal/app/runner.go` to capture selected mode, construct the lifecycle only for Formal TUI submissions, and otherwise use the existing Default registry directly. Wire both `UserAsker` and `PlanReviewer` from `internal/app/tui.go`. Remove legacy fields and pre-pass code from app/CLI, AgentOps, benchmark, and Autodev construction. Add benchmark TODO tools and leave Feishu untouched except for regression verification.

Covers: REQ-001, REQ-003, REQ-011, REQ-012, REQ-013, NFR-003

### C8: Preserved Session Artifacts And Rewind

Keep `memory.Store.EnsureFiles`, `StateHistory.SnapshotBeforeMessage`, `RestoreBeforeMessage`, sidebar artifact loading, and `/rewind` paths. The single-run lifecycle retains one pre-user-message PLAN/TODO snapshot, so rewinding that message restores the state before both proposal submission and implementation TODO updates.

Covers: REQ-006, REQ-010, REQ-014

## Implementation Phases

### Phase 1: Characterize Legacy Boundaries And Introduce Collaboration State

Write failing/characterization tests for Default startup/resume/new session, active-run selection deferral, `/plan` flag rejection, and absence of extra Planner provider calls. Implement `internal/collaboration`, replace boolean runner/TUI contracts, and remove CLI config plumbing without yet enabling Formal execution.

Covers: REQ-001, REQ-002, REQ-003, REQ-011, REQ-012, NFR-003, NFR-004, NFR-005

### Phase 2: Plan Store And `submit_plan` TDD

Write failing memory/tool tests for exact successful bytes, whitespace rejection, atomic failure preservation, review-after-write ordering, revision feedback, approval notification, reviewer cancellation, and reviewer errors. Implement atomic PLAN replacement and the injected `submit_plan` dependencies.

Covers: REQ-005, REQ-006, REQ-007, REQ-010, REQ-014, NFR-002, NFR-004, NFR-005

### Phase 3: Formal Lifecycle And Prompt TDD

Write failing registry, engine, prompt, and runner tests proving:

- All four required canonical Formal tools are visible and explicit write/TODO tools are absent; compatibility aliases do not weaken the surface.
- General editing instructions are overridden by Formal read-only guidance.
- Revision remains Formal; approval transitions only at the next turn.
- The complete plan is reinjected after compaction.
- The checklist gate blocks explicit implementation tools and final completion until `update_todo` succeeds.
- A same-batch write after `submit_plan` or `update_todo` is denied, while the next eligible turn receives the new surface.
- The entire approve-to-implementation flow uses one run and one original user message.

Implement the turn hook, lifecycle registry, completion gate, reminder composition, and typed prompt option incrementally.

Covers: REQ-003, REQ-004, REQ-005, REQ-007, REQ-008, REQ-009, REQ-010, NFR-001, NFR-002, NFR-004, NFR-005

### Phase 4: TUI Controls And Confirmation TDD

Write failing TUI tests for Default initialization, idempotent `/plan` and `/plan off`, primary-input Shift+Tab, active-run deferral, unchanged Esc/sidebar behavior, exact plan form source, scrolling, revision feedback, approval mode reset, cancellation without approval, and re-arming the review listener. Implement the bridge/form and app wiring, then update reporter summaries and status displays only as needed by those tests.

Covers: REQ-001, REQ-002, REQ-003, REQ-006, REQ-007, REQ-008, NFR-002, NFR-004, NFR-005

### Phase 5: Remove Automatic Planner Entry Points TDD

Add or update focused tests for one-shot CLI unsupported `-plan`, direct Default execution, AgentOps registry/run behavior, benchmark TODO tools with no pre-pass, unchanged Feishu tools, and unchanged Autodev SDD adapter behavior. Remove Planner production/tests and stale comments/config fields.

Covers: REQ-011, REQ-012, REQ-013, NFR-003, NFR-004, NFR-005

### Phase 6: Rewind Regression And Full Verification

Add a lifecycle regression test that snapshots old PLAN/TODO content, submits/approves a new plan, initializes TODO, then restores the pre-message state. Format changed Go files, run focused package tests, run `go test ./...`, and statically verify no `EnablePlanMode`, registered `-plan`, production `NewPlanner`, or `BuildPlan` remains.

Covers: REQ-012, REQ-013, REQ-014, NFR-003, NFR-004, NFR-005

## Verification Strategy

- Red-Green-Refactor evidence is recorded per implementation task: each behavior test must fail for the intended missing behavior before production edits.
- Focused suites:
  - `go test ./internal/collaboration`
  - `go test ./internal/memory ./internal/tools`
  - `go test ./internal/context ./internal/engine ./internal/app`
  - `go test ./internal/tui`
  - `go test ./internal/agentops ./internal/feishu ./cmd/bench ./cmd/fox`
- Full suite: `go test ./...`
- Static migration checks:
  - `rg 'EnablePlanMode|SetPlanMode|PlanMode\(' --glob '*.go'`
  - `rg 'NewPlanner|BuildPlan' --glob '*.go'`
  - `rg 'BoolVar\([^\n]*"plan"' cmd internal`
- Fake-provider acceptance sequence observes tool definitions, prompts, messages, run IDs, and call ordering without a live model.
- Fake Plan store/reviewer collaborators deterministically cover write and confirmation failures.
- ANSI-stripped TUI assertions verify semantic content; source-field assertions verify exact plan identity independently of visual Markdown styling.

Covers: REQ-001, REQ-002, REQ-003, REQ-004, REQ-005, REQ-006, REQ-007, REQ-008, REQ-009, REQ-010, REQ-011, REQ-012, REQ-013, REQ-014, NFR-001, NFR-002, NFR-003, NFR-004, NFR-005

## Risks And Trade-Offs

- **Bash can still mutate state in Formal or checklist-gate phases.** Mitigation: retain the confirmed explicit read-only instructions and document/test the boundary as behavioral, not secure. Covers: REQ-004, NFR-001
- **A fixed system prompt spans multiple phases.** Mitigation: use explicit conditional lifecycle instructions and concrete registry gating; test the exact prompt and each phase's definitions. Covers: REQ-004, REQ-005, REQ-008, REQ-009
- **Compaction could otherwise hide the submitted proposal.** Mitigation: inject the complete approved plan through the existing post-compaction reminder point. Covers: REQ-008, REQ-009
- **A model may request multiple tools in one response.** Mitigation: commit tool-surface transitions only at `BeginTurn`, so same-batch calls remain constrained by the surface advertised for that turn. Covers: REQ-005, REQ-009, NFR-005
- **Formal planning consumes turns from the existing run limit.** Mitigation: keep existing limit semantics and deterministic repeated-gate failure instead of an unbounded loop; TUI defaults remain unlimited. Covers: REQ-008, NFR-005
- **Direct `-plan` removal breaks existing scripts.** Mitigation: test ordinary flag-parser failure and update usage text; no compatibility alias is added. Covers: REQ-011, NFR-003
- **Atomic rename semantics vary at filesystem boundaries.** Mitigation: create the temporary file beside `PLAN.md`, preserve exact bytes, and test replacement/failure behavior through injected collaborators and local filesystem tests. Covers: REQ-006, REQ-014, NFR-002

## Assumptions

- Formal Plan remains a TUI-only selectable collaboration mode in this feature; programmatic tests may set it directly, but non-interactive commands do not expose a user control.
- Primary-input mode controls do not replace sidebar-local Shift+Tab behavior or overlay-local cancellation behavior.
- Existing slash-command allow-lists remain stricter outer constraints. A restricted Formal command that omits a lifecycle-required canonical tool is rejected before the engine starts rather than silently broadening its allow-list or starting an unsatisfiable run.
- Approval/revision decisions are session-local runtime events and are not added to settings persistence.

## Requirements Coverage

| Requirement | Plan References |
|-------------|-----------------|
| REQ-001 | Fidelity Check, Architecture, PLD-001, C1, C6, C7, Phase 1, Phase 4 |
| REQ-002 | Fidelity Check, Architecture, PLD-001, C1, C6, Phase 1, Phase 4 |
| REQ-003 | Existing Constraints, Architecture, PLD-001, PLD-002, C1, C3, C6, C7, Phase 1, Phase 3, Phase 4 |
| REQ-004 | Fidelity Check, PLD-003, PLD-007, C5, Phase 3, Risks |
| REQ-005 | Fidelity Check, PLD-002, PLD-004, PLD-007, C2, C3, C5, Phase 2, Phase 3 |
| REQ-006 | Fidelity Check, PLD-004, PLD-005, C2, C4, C6, C8, Phase 2, Phase 4 |
| REQ-007 | Fidelity Check, PLD-004, PLD-005, C2, C3, C6, Phase 2, Phase 3, Phase 4 |
| REQ-008 | Architecture, PLD-002, PLD-003, PLD-005, PLD-006, PLD-007, C3, C4, C5, C6, Phase 3, Phase 4 |
| REQ-009 | Architecture, PLD-002, PLD-003, PLD-006, PLD-007, C3, C4, C5, Phase 3, Risks |
| REQ-010 | Fidelity Check, PLD-003, PLD-004, PLD-007, C2, C3, C5, C8, Phase 2, Phase 3 |
| REQ-011 | Fidelity Check, PLD-008, C7, Phase 1, Phase 5, Risks |
| REQ-012 | Fidelity Check, PLD-001, PLD-008, C1, C7, Phase 1, Phase 5, Phase 6 |
| REQ-013 | Fidelity Check, PLD-008, C7, Phase 5, Phase 6 |
| REQ-014 | Existing Constraints, PLD-004, C2, C8, Phase 2, Phase 6 |
| NFR-001 | Fidelity Check, PLD-003, PLD-007, C3, C5, Phase 3, Risks |
| NFR-002 | PLD-004, PLD-005, C2, C6, Phase 2, Phase 3, Phase 4, Verification |
| NFR-003 | Fidelity Check, PLD-008, C7, Phase 1, Phase 5, Phase 6 |
| NFR-004 | Existing Constraints, every implementation phase, Verification |
| NFR-005 | Existing Constraints, PLD-002, PLD-003, PLD-004, PLD-005, PLD-006, C2, C3, C4, C6, every implementation phase, Verification |

## Unresolved Items

None.
