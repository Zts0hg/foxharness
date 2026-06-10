# Task Breakdown: Ask User Question Tool (`ask_user_question`)

> Spec: `./spec.md` · Plan: `./plan.md` · Reviews: `./review-spec.md` (100), `./review-plan.md` (99).
> Constitution: TDD mandatory — every code component's test task precedes its implementation task.

## Overview
- Total tasks: 23
- Parallelizable tasks: 3 (marked `[P]`: 6.2, 6.3, and any others noted inline)
- Estimated phases: 6
- Module: `github.com/Zts0hg/foxharness`

> **Go TDD note**: In Go, a component's test file and the file under test share the same package and compile together, so a `_test.go` written first will **fail to compile** (Red) until the referenced identifiers exist. Each "write test" task therefore also declares the minimal exported skeleton (signatures returning zero values / `not implemented`) in the implementation file so the test compiles and fails for the right reason. The paired implementation task then fills in the body (Green) and refactors.

---

## Phase 1: Foundation — interface, types, tool skeleton

### Task 1.1: Define `UserAsker` interface and data types
- **Type**: Setup / Implementation (types only)
- **Files**: `internal/tools/ask_user_question.go`
- **Description**: Create the file with block-level package-consistent docs and define: `UserAsker` interface (`Ask(ctx, []Question) ([]Answer, error)`), `Question`, `Option`, `Answer` structs (per plan §6/§7), and the sentinel `ErrUserCancelled`. Add an `AskUserQuestionTool` struct with `NewAskUserQuestionTool(asker UserAsker) *AskUserQuestionTool`, and skeleton methods `Name()`, `Definition()`, `Execute()` returning zero values / `"not implemented"` so dependents compile. No `ParallelSafe` method (REQ-018).
- **Dependencies**: None
- **Est. Complexity**: Low

### Task 1.2: Add `fakeAsker` test helper
- **Type**: Testing (support)
- **Files**: `internal/tools/ask_user_question_test.go`
- **Description**: Add a deterministic `fakeAsker` implementing `UserAsker` — configurable to return canned `[]Answer`, return `ErrUserCancelled`, return a context error, or record that it was (not) invoked. Supports NFR-002 determinism and the gating/answers tests.
- **Dependencies**: Task 1.1
- **Est. Complexity**: Low

---

## Phase 2: Core tool logic (TDD: Red → Green → Refactor)

### Task 2.1: Write tests for `Name()` and `Definition()`
- **Type**: Testing
- **Files**: `internal/tools/ask_user_question_test.go`
- **Description**: Assert `Name() == "ask_user_question"` (REQ-019) and that `Definition()` yields a `schema.ToolDefinition` whose `InputSchema` declares `questions` (1–4), per-question `header`/`question`/`options` (2–4 of `{label,description,preview?}`)/`multiSelect`, plus optional `answers`/`annotations`/`metadata` (REQ-001..005, REQ-020). Run: must fail (Red).
- **Dependencies**: Task 1.2
- **Est. Complexity**: Low

### Task 2.2: Implement `Name()` and `Definition()`
- **Type**: Implementation
- **Files**: `internal/tools/ask_user_question.go`
- **Description**: Implement `Name()` and the JSON `InputSchema` + description (adapted from reference prompt: when to ask, recommended-first "(Recommended)", "Other" automatic, preview usage). Make Task 2.1 pass (Green).
- **Dependencies**: Task 2.1
- **Est. Complexity**: Medium

### Task 2.3: Write tests for input validation
- **Type**: Testing
- **Files**: `internal/tools/ask_user_question_test.go`
- **Description**: Table-driven tests for: duplicate question texts → error (TC-005); duplicate option labels within a question → error (TC-006); `questions` empty/>4 → error (TC-007); options <2/>4 → error (TC-008); over-length `header`/`label` → **passes** validation (TC-019, REQ-007a); malformed JSON args → error not panic (TC-015). Assert the asker is NOT invoked on validation failure. Run: must fail (Red).
- **Dependencies**: Task 2.2
- **Est. Complexity**: Medium

### Task 2.4: Implement input decode + validation
- **Type**: Implementation
- **Files**: `internal/tools/ask_user_question.go`
- **Description**: In `Execute`, `json.Unmarshal` args defensively, then validate array bounds (REQ-007) and uniqueness (REQ-006); string lengths advisory only (REQ-007a). Return descriptive error strings; never panic. Make Task 2.3 pass (Green).
- **Dependencies**: Task 2.3
- **Est. Complexity**: Medium

### Task 2.5: Write tests for answer collection + formatting
- **Type**: Testing
- **Files**: `internal/tools/ask_user_question_test.go`
- **Description**: Using `fakeAsker`: single-question single-select output shape (TC-001); 4 questions in order (TC-002); multi-select comma-join (TC-003, REQ-009); "Other"/free-text answer surfaced (TC-004); preview/notes annotation segments appended (TC-010, REQ-016); answers keyed by exact question text, non-matching key formatted without panic (TC-020, REQ-004). Verify the exact `User has answered your questions: ...` format (REQ-015). Run: must fail (Red).
- **Dependencies**: Task 2.4
- **Est. Complexity**: Medium

### Task 2.6: Implement answer collection + result formatting
- **Type**: Implementation
- **Files**: `internal/tools/ask_user_question.go`
- **Description**: After validation: call `asker.Ask(ctx, questions)`, then format `[]Answer` into the REQ-015 string (with preview/notes per REQ-016, multi-select join per REQ-009). Make Task 2.5 pass (Green); refactor for readability.
- **Dependencies**: Task 2.5
- **Est. Complexity**: Medium

### Task 2.7: Write tests for `answers` injection, cancellation, ctx, nil-asker, cap, parallel-safety
- **Type**: Testing
- **Files**: `internal/tools/ask_user_question_test.go`
- **Description**: Pre-supplied full `answers` → asker NOT invoked, formatted verbatim (TC-009); partial `answers` → asker NOT invoked, only present entries formatted, no error (TC-017, REQ-021); `ErrUserCancelled` → cancellation message (TC-011); ctx-cancelled asker → prompt return, no hang (TC-012); nil asker + no answers → REQ-014 message, asker not invoked (TC-013); assembled result >100k → truncated with marker (TC-018, REQ-022); tool does not satisfy `ParallelSafeTool` / `IsParallelSafe` is false (TC-016). Run: must fail (Red).
- **Dependencies**: Task 2.6
- **Est. Complexity**: Medium

### Task 2.8: Implement injection/cancel/ctx/nil-asker/cap branches
- **Type**: Implementation
- **Files**: `internal/tools/ask_user_question.go`
- **Description**: Add the `Execute` branches: consume pre-supplied `answers` verbatim and skip the asker (REQ-021); map `ErrUserCancelled`→REQ-011 message and ctx errors→prompt-return (REQ-014); nil-asker fallback message (REQ-014); 100k truncation (REQ-022). Confirm no `ParallelSafe` method exists (REQ-018). Make Task 2.7 pass (Green).
- **Dependencies**: Task 2.7
- **Est. Complexity**: Medium

### Task 2.9: Add formatting/validation benchmark
- **Type**: Testing (benchmark)
- **Files**: `internal/tools/ask_user_question_test.go`
- **Description**: `BenchmarkAskUserQuestionFormat` over the max input (4 questions × 4 options, with `answers` pre-supplied to avoid the asker) asserting the pure-CPU validation+format path is well under 1ms (NFR-004). Document the human-bound-latency exclusion in a comment.
- **Dependencies**: Task 2.8
- **Est. Complexity**: Low
- **Note**: not `[P]` — it is the last Phase-2 task and shares `ask_user_question_test.go` with the earlier test tasks, so there is no sibling to run in parallel with.

---

## Phase 3: TUI bridge (`tuiAsker`)

### Task 3.1: Write tests for the TUI asker bridge (+ minimal skeleton)
- **Type**: Testing
- **Files**: `internal/tui/asker_test.go` (also adds the minimal skeleton to `internal/tui/asker.go`)
- **Description**: Mirroring the Task 1.1 pattern (per the Go-TDD note): first add the minimal skeleton to `internal/tui/asker.go` — the `Asker` type, `askRequest`/`answerResult`, and `Ask`/`Requests` returning zero values / `not implemented` — so the test file compiles. Then write tests that drive `Asker` directly (no full TUI): a goroutine reads `Requests()` and replies with answers → `Ask` returns them; reply with `cancelled:true` → `Ask` returns `tools.ErrUserCancelled` (REQ-011); cancel the ctx with no reader → `Ask` returns promptly with the ctx error (TC-012, REQ-010). Run: must fail for the right reason (Red).
- **Dependencies**: Task 2.8
- **Est. Complexity**: Medium

### Task 3.2: Implement `tuiAsker`
- **Type**: Implementation
- **Files**: `internal/tui/asker.go`
- **Description**: Fill in the bodies of the skeleton from Task 3.1 so `Asker` satisfies `tools.UserAsker`: owns a long-lived `chan askRequest`; `Ask` builds `askRequest{questions, reply}`, sends it, and `select`s on `<-reply` vs `<-ctx.Done()`; `Requests() <-chan askRequest` exposes the channel. Make Task 3.1 pass (Green).
- **Dependencies**: Task 3.1
- **Est. Complexity**: Medium

---

## Phase 4: TUI overlay + model integration

### Task 4.1: Write tests for the `askform` overlay (+ minimal skeleton)
- **Type**: Testing
- **Files**: `internal/tui/askform_test.go` (also adds the minimal skeleton to `internal/tui/askform.go`)
- **Description**: Mirroring the Task 1.1 pattern (per the Go-TDD note): first add the minimal skeleton to `internal/tui/askform.go` — `askForm`, `newAskForm(req askRequest)`, `Update`/`View`, and the `askDoneMsg{answers, cancelled}` type — so the test compiles. Then unit-test the overlay sub-model via `Update`/`View`: arrow navigation + select on a single-select question; multi-select toggle joins labels (REQ-009); the auto-appended "Other" entry (REQ-008) switches to text-input mode and captures free text (TC-004); preview shown for the focused option when present and harmless on a `multiSelect` question (REQ-016 + edge case); cancel produces a cancelled result (REQ-011); completing all questions yields ordered `[]tools.Answer`. Run: must fail for the right reason (Red).
- **Dependencies**: Task 3.2
- **Est. Complexity**: High

### Task 4.2: Implement the `askform` overlay sub-model
- **Type**: Implementation
- **Files**: `internal/tui/askform.go`
- **Description**: Fill in the bodies of the skeleton from Task 4.1 so `askForm` (`newAskForm(req askRequest)`, `Update`, `View`) drives one `askRequest` across its questions: per-question selectable list (borrow keybinding/list style from `internal/tui/selector`), multi-select toggling, auto "Other"→text input, preview render for the focused option. Emit completion via a `tea.Cmd` producing `askDoneMsg{answers, cancelled}` (the concrete handshake, plan PLAN-003). Handle the duplicate/empty-answer defensive case. Make Task 4.1 pass (Green).
- **Dependencies**: Task 4.1
- **Est. Complexity**: High

### Task 4.3: Write tests for model integration of the overlay
- **Type**: Testing
- **Files**: `internal/tui/model_test.go`
- **Description**: Assert: an injected `askUserMsg` opens the overlay; while active, key events route to the overlay (not the prompt input); on `askDoneMsg` the model sends the result on the request's `reply` channel and closes the overlay; `View` renders the overlay when active. Run: must fail (Red).
- **Dependencies**: Task 4.2
- **Est. Complexity**: Medium

### Task 4.4: Integrate overlay + `listenForAsks` into the model
- **Type**: Implementation
- **Files**: `internal/tui/model.go`
- **Description**: Add optional `asker *Asker` + overlay state to `Model`; add a `listenForAsks` `tea.Cmd` (mirrors the events-channel reader) converting `askRequest`→`askUserMsg`; in `Update` open/route/close the overlay and reply on completion; render in `View`. Extend `Config` with the asker. Make Task 4.3 pass (Green).
- **Dependencies**: Task 4.3
- **Est. Complexity**: High

---

## Phase 5: Wiring & mode gating

### Task 5.1: Write tests for mode-gated registration
- **Type**: Testing
- **Files**: `internal/app/runner_test.go`
- **Description**: Assert `buildRegistry` (or the registration path) includes `ask_user_question` in `GetAvailableTools()` **only when** a `userAsker` is set, and excludes it when nil (TC-014, REQ-012..014). Run: must fail (Red).
- **Dependencies**: Task 2.8
- **Est. Complexity**: Medium
- **Note (TC-014 negative cases)**: The agentops/feishu/subagent/bench registries never call `NewAskUserQuestionTool`, so they cannot expose the tool by construction — no separate test is needed for those packages; this app-level nil-asker case is the representative negative.

### Task 5.2: Add `userAsker` field + gated registration to `AgentRunner`
- **Type**: Implementation
- **Files**: `internal/app/runner.go`
- **Description**: Add `userAsker tools.UserAsker` field + `SetUserAsker(...)`; in `buildRegistry`, register `tools.NewAskUserQuestionTool(r.userAsker)` **iff `r.userAsker != nil`**. Leave CLI/agentops/feishu/subagent/bench paths untouched (they never set it). Make Task 5.1 pass (Green).
- **Dependencies**: Task 5.1
- **Est. Complexity**: Low

### Task 5.3: Wire the `tuiAsker` into `RunTUI`
- **Type**: Implementation
- **Files**: `internal/app/tui.go`
- **Description**: In `RunTUI`, construct a `tui.NewAsker()`, call `runner.SetUserAsker(asker)`, and pass it via `tui.Config`. Only the TUI path sets the asker (the `isEnabled()` analog). To close the automated-coverage gap (TASK-002), factor the asker construction + `SetUserAsker` into a small testable helper (e.g. `attachInteractiveAsker(runner)`) and add an assertion in `internal/app/tui_test.go` that after it runs the runner's registry contains `ask_user_question` — without starting the full Bubble Tea program.
- **Dependencies**: Task 5.2, Task 4.4
- **Est. Complexity**: Low

---

## Phase 6: Validation & docs

### Task 6.1: Full test + format + vet gate
- **Type**: Testing
- **Files**: (repo-wide) — no single file
- **Description**: Run `go test ./...`, `gofmt -l .` (must be empty), `go vet ./...`. Fix any fallout. Confirms all TC-001..020 + benchmark pass and the build is clean.
- **Dependencies**: Task 5.3, Task 2.9
- **Est. Complexity**: Low

### Task 6.2: Manual TUI smoke verification [P]
- **Type**: Testing (manual)
- **Files**: (manual) — `go run ./cmd/fox`
- **Description**: Launch the TUI, prompt the agent so it calls `ask_user_question`; verify single-select, multi-select, "Other" free text, preview rendering, and cancellation all behave per spec. Confirm the tool is absent in `go run ./cmd/fox exec` (non-interactive).
- **Dependencies**: Task 6.1
- **Est. Complexity**: Low

### Task 6.3: Tool doc comment + brief usage note [P]
- **Type**: Documentation
- **Files**: `internal/tools/ask_user_question.go`
- **Description**: Ensure block-level Go docs on all exported identifiers (constitution §3), explicitly noting the tool is semantically read-only with no read-only property (REQ-017, plan Decision 6) and not parallel-safe (REQ-018). No teaching line comments.
- **Dependencies**: Task 6.1
- **Est. Complexity**: Low

---

## Execution Order

```
Phase 1:  Task 1.1 ──► Task 1.2
                          │
Phase 2:  Task 2.1 ► 2.2 ► 2.3 ► 2.4 ► 2.5 ► 2.6 ► 2.7 ► 2.8 ──► Task 2.9
                                                          │
              ┌───────────────────────────────────────────┼───────────────┐
Phase 3:      │                                            │           Phase 5 (gating):
         Task 3.1 ──► Task 3.2                              │           Task 5.1 ──► Task 5.2
                          │                                 │                              │
Phase 4:           Task 4.1 ──► 4.2 ──► 4.3 ──► 4.4 ────────┼──────────────────────────────┤
                                                            │                              ▼
                                                            └───────────────────────► Task 5.3
                                                                                          │
Phase 6:                                              Task 6.1 ◄─(also needs 2.9)─────────┘
                                                          │
                                                 ┌────────┴────────┐
                                            Task 6.2 [P]      Task 6.3 [P]
```

Notes:
- The tool logic (Phase 2) is fully testable with `fakeAsker` and has **no dependency** on the TUI (Phases 3–4) — confirming the DI design (plan Decision 1).
- Phase 5 gating (5.1→5.2) depends only on the tool existing (Task 2.8); wiring 5.3 additionally needs the TUI model (4.4).
- `[P]` tasks: 6.2 & 6.3 (independent manual/doc work after the gate). Task 2.9 is **not** parallel — it is the last Phase-2 task and shares `ask_user_question_test.go` with the earlier tests.

## Checkpoints
- [x] **Checkpoint 1** (after Phase 1): Package compiles; `UserAsker`/types/skeleton + `fakeAsker` in place.
- [x] **Checkpoint 2** (after Phase 2): All tool-logic tests (TC-001..020 applicable to the tool) + benchmark pass; zero TUI dependency. *(20 tests pass; benchmark ~31µs/op ≪ 1ms.)*
- [x] **Checkpoint 3** (after Phase 3): `tuiAsker` bridge tests pass (reply, cancel, ctx-cancel promptness). *(4 tests; race-clean.)*
- [x] **Checkpoint 4** (after Phase 4): Overlay + model integration tests pass; overlay renders and replies. *(9 askform + 2 model-integration tests.)*
- [x] **Checkpoint 5** (after Phase 5): TC-014 gating passes — tool present in TUI registry, absent elsewhere. *(buildRegistry gating + attachInteractiveAsker wiring tests.)*
- [x] **Checkpoint 6** (after Phase 6): `go test ./...` clean (exit 0), `gofmt -l` empty, `go vet` clean, `-race` clean on the bridge. Docs complete (Task 6.3). **Task 6.2 (manual TUI smoke) is left for the user — it needs an interactive terminal + a live model; the non-interactive absence is covered automatically by the gating test.**
