# Implementation Plan: Ask User Question Tool (`ask_user_question`)

> Spec: `./spec.md` (REQ-001..022, TC-001..020, NFR-001..006). Review: `./review-spec.md` (100/100).
> Reference: `/Users/xiaoming/code/claude-code-main/src/tools/AskUserQuestionTool/`.

## 1. Tech Stack

| Category | Technology | Version | Notes |
|----------|------------|---------|-------|
| Language | Go | as per `go.mod` (Go 1.2x toolchain in repo) | Match existing module `github.com/Zts0hg/foxharness`. |
| TUI framework | `github.com/charmbracelet/bubbletea` | version already vendored in repo | Existing TUI runs on it (`tea.NewProgram`, `tea.Msg`, `tea.Cmd`). |
| TUI styling | `github.com/charmbracelet/lipgloss` (already used) | repo version | For rendering the question overlay, consistent with existing views. |
| JSON | stdlib `encoding/json` | stdlib | Tool args decode, same as other tools. |
| Testing | stdlib `testing` + table-driven tests | stdlib | Constitution §4; no new test deps. |
| Concurrency primitives | stdlib `context`, channels | stdlib | For the blocking ask bridge + cancellation. |

No new third-party dependency is introduced. The feature is built entirely from the existing stack.

## 2. Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| §1 TDD | ✅ | Phases are ordered Red→Green→Refactor. The tool's logic (validation, formatting, gating, answer consumption) is unit-tested against a **fake `UserAsker`** before any TUI wiring. See Phase 2/3. |
| §2 Code Quality (Readability/Testability/Extensibility) | ✅ | The interactive surface is hidden behind the small `UserAsker` interface (dependency injection per NFR-001); the tool has no terminal knowledge. Open-closed: new surfaces implement `UserAsker` without touching the tool. |
| §3 Go Documentation Standards | ✅ | New package `ask_user_question.go` gets a block-level package/type/func doc; exported `UserAsker`, `Question`, `Answer` fully documented; no teaching line comments. |
| §4 Testing Standards | ✅ | Tests mirror package structure (`ask_user_question_test.go`, `asker_test.go`); error/edge paths (TC-005..008, TC-011..013, TC-015..020) covered; deterministic via fakes (NFR-002). |
| §5 Architecture (Separation of Concerns) | ✅ | `tools` owns tool logic + interface; `tui` owns rendering + the bridge impl; `app` owns wiring/gating. No import cycle: `tui`→`tools` only. |
| §6 Performance | ✅ | Pure-CPU path O(total options + bytes), benchmark asserts < 1ms for max input (NFR-004); 100k result cap (REQ-022). End-to-end latency is human-bound (out of scope). |
| §7 Security | ✅ | "Other" free text is treated as untrusted display text returned to the LLM only; never executed/used as path (NFR-006). Args decoded defensively, no panics (REQ/TC-015). |

No principle conflicts with the requirements.

## 3. Architecture Overview

The tool never blocks the engine on a TTY it can't reach. Instead it depends on an injected `UserAsker`. Only the interactive TUI provides a real asker; all other runners leave it nil, so the tool is **not registered** there (the `isEnabled()` analog, REQ-012..014).

In the TUI, `Execute` runs on the engine goroutine (inside `runner.Run`, a `tea.Cmd`). It must reach the Bubble Tea update loop, render a question overlay, and block for the answer. This mirrors the existing one-way `channelReporter` (`engine → UI` over `chan tea.Msg`) but is **bidirectional**: each request carries a reply channel.

```
                         engine goroutine (tea.Cmd: runner.Run)
                         ┌───────────────────────────────────────┐
   LLM tool_call ──────▶ │ ask_user_question.Execute(ctx, args)  │
                         │   1. decode + validate args           │
                         │   2. if answers pre-filled → format   │
                         │   3. else asker.Ask(ctx, questions) ──┼──┐ blocks
                         └───────────────────────────────────────┘  │
                                                                     │ askRequest{questions, reply}
                                          chan askRequest (long-lived)│
                                                                     ▼
   Bubble Tea UI loop      ┌──────────────────────────────────────────────────┐
   (main goroutine)        │ listenForAsks cmd → Update(askUserMsg)            │
                           │   opens question overlay (per-question):          │
                           │     single/multi select list + auto "Other" entry │
                           │     "Other" → text input                          │
                           │   on finish: reply <- answerResult{answers,...} ──┼──┐
                           └──────────────────────────────────────────────────┘  │
                                                                                  │
   Execute unblocks ◀──────────────────  <-reply (or <-ctx.Done())  ◀────────────┘
                         4. format result string (verbatim entries, cap 100k)
                         5. return to engine → tool_result → LLM
```

## 4. Component Structure

```
internal/
├── tools/
│   ├── ask_user_question.go        # NEW: UserAsker interface, Question/Answer types,
│   │                               #      AskUserQuestionTool (BaseTool), validation,
│   │                               #      answer formatting, result cap. No TUI imports.
│   ├── ask_user_question_test.go   # NEW: unit tests against a fake UserAsker (TC-001..020)
│   └── registry.go                 # unchanged
├── tui/
│   ├── asker.go                    # NEW: tuiAsker implementing tools.UserAsker; owns
│   │                               #      chan askRequest; Ask() sends + blocks on reply/ctx.
│   ├── asker_test.go               # NEW: bridge tests (reply path, ctx cancel, no-listener)
│   ├── askform.go                  # NEW: question-overlay sub-model (per-question list,
│   │                               #      multiSelect, auto "Other" → text input)
│   ├── askform_test.go             # NEW: overlay navigation/selection tests
│   └── model.go                    # MODIFY: hold asker + overlay state; listenForAsks cmd;
│                                   #         Update/View handle askUserMsg + overlay
└── app/
    ├── tui.go                      # MODIFY: RunTUI creates tuiAsker, sets it on runner,
    │                               #         passes it via tui.Config
    └── runner.go                   # MODIFY: AgentRunner gains userAsker field + setter;
                                    #         buildRegistry registers tool iff userAsker != nil
```

Runners that intentionally do **not** register the tool (no asker): `internal/app` CLI path (`RunCLI`), `internal/agentops/runner.go`, `internal/feishu/runner.go`, `internal/subagent/manager.go`, `cmd/bench/main.go`.

## 5. Module Dependency Graph

```
        ┌────────────────────┐
        │  internal/app      │  (wiring + mode gating)
        │  runner.go, tui.go │
        └───────┬─────┬──────┘
                │     │ provides asker (TUI only)
        depends │     │
                ▼     ▼
   ┌────────────────┐   ┌────────────────────────┐
   │ internal/tools │◀──│ internal/tui           │
   │  UserAsker     │   │  tuiAsker, askform,    │
   │  (interface)   │   │  model.go              │
   │  AskUserQ tool │   │  (implements UserAsker)│
   └────────────────┘   └────────────────────────┘
            │                      │
            ▼                      ▼
   ┌────────────────┐      ┌──────────────────┐
   │ internal/schema│      │ bubbletea/lipgloss│
   └────────────────┘      └──────────────────┘
```

Key: `tools` defines and consumes the `UserAsker` interface (Go "accept interfaces" idiom) and depends only on `schema`. `tui` imports `tools` to implement the interface — **no reverse import**, so no cycle.

## 6. Module Specifications

### Module: `internal/tools/ask_user_question.go`
- **Responsibility**: Define the tool and its contract. Decode/validate input (REQ-001..007a), drive answer collection via the injected asker or consume pre-supplied `answers` (REQ-021), format the result string with annotations and the 100k cap (REQ-015, REQ-016, REQ-022), and expose tool properties (name `ask_user_question`; **not** parallel-safe). The tool is semantically read-only (performs no mutations) but adds **no** read-only property — foxharness has no such hook to report through (REQ-017, Decision 6).
- **Dependencies**: `internal/schema`, stdlib `context`/`encoding/json`/`strings`. The `UserAsker` interface (defined here).
- **Interface (exposed)**:
  - `type UserAsker interface { Ask(ctx context.Context, questions []Question) ([]Answer, error) }`
  - `type Question struct { Header, Prompt string; Options []Option; MultiSelect bool }`
  - `type Option struct { Label, Description, Preview string }`
  - `type Answer struct { QuestionText, Value, Preview, Notes string }` (`Value` is comma-joined for multi-select)
  - `func NewAskUserQuestionTool(asker UserAsker) *AskUserQuestionTool`
  - Implements `BaseTool` (`Name`, `Definition`, `Execute`). Does **not** implement `ParallelSafeTool` (REQ-018, TC-016).
  - Sentinel error `ErrUserCancelled` for REQ-011/TC-011.
- **Files**: `ask_user_question.go`, `ask_user_question_test.go`.

### Module: `internal/tui/asker.go` (`tuiAsker`)
- **Responsibility**: The interactive `UserAsker` implementation. Owns a long-lived `chan askRequest`. `Ask` builds an `askRequest{questions, reply}`, sends it, and blocks on `<-reply` or `<-ctx.Done()` (returns ctx error → REQ-014/TC-012 promptness). Exposes the request channel for the model to listen on.
- **Dependencies**: `internal/tools` (for `Question`/`Answer` types), `context`.
- **Interface**: `func NewAsker() *Asker`; `func (a *Asker) Ask(...)` (satisfies `tools.UserAsker`); `func (a *Asker) Requests() <-chan askRequest`.
- **Files**: `asker.go`, `asker_test.go`.

### Module: `internal/tui/askform.go` (overlay sub-model)
- **Responsibility**: Render and drive one `askRequest` to completion: iterate questions; per question show a selectable list (arrow keys, existing keybinding style), support multi-select toggling, always append an "Other" entry that switches to a text-input mode (REQ-008/REQ-009), render `preview` for the focused option when present (REQ-016). Produce `[]tools.Answer` or a cancellation.
- **Dependencies**: `internal/tools`, `bubbletea`, `lipgloss`.
- **Interface**: `newAskForm(req askRequest) askForm`; `Update`, `View`, and a completion signal consumed by the parent model.
- **Files**: `askform.go`, `askform_test.go`.

### Module: `internal/tui/model.go` (modifications)
- **Responsibility**: Integrate the overlay. Hold optional `asker *Asker` and `askForm` overlay state. Add a `listenForAsks` `tea.Cmd` (mirrors the events-channel reader) that converts `askRequest` → `askUserMsg`. In `Update`, opening overlay on `askUserMsg`, route key events to the overlay while active, and on completion send the result on the request's `reply` channel. `View` overlays the form when active.
- **Dependencies**: existing + `askform`, `asker`.
- **Files**: `model.go` (modify), plus assertions in existing `model_test.go` or new tests.

### Module: `internal/app/runner.go` + `tui.go` (wiring/gating)
- **Responsibility**: `AgentRunner` gains `userAsker tools.UserAsker` (+ `SetUserAsker`). `buildRegistry` registers `NewAskUserQuestionTool(r.userAsker)` **only if `r.userAsker != nil`** (REQ-012..014). `RunTUI` constructs the `tuiAsker`, calls `runner.SetUserAsker`, and passes it into `tui.Config`. `RunCLI` and the other runners never set it.
- **Dependencies**: `internal/tools`, `internal/tui`.
- **Files**: `runner.go` (modify), `tui.go` (modify), `runner_test.go`/`tui_test.go` (gating assertions, TC-014).

## 7. Data Models

No persistent storage. In-memory types only (defined in `internal/tools/ask_user_question.go`):

### `Question` (decoded tool input element)
| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| `Header` | string | Short chip label | length advisory only (REQ-002/007a) |
| `Prompt` | string | Full question text; also the answer map key | must be unique across questions (REQ-006) |
| `Options` | []Option | Choices | len 2–4 (REQ-007) |
| `MultiSelect` | bool | Allow multiple selections | default false |

### `Option`
| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| `Label` | string | Display text; also unique within question | unique per question (REQ-006); length advisory |
| `Description` | string | Meaning/trade-offs | — |
| `Preview` | string | Optional preview content | optional |

### `Answer` (collected result element)
| Field | Type | Description |
|-------|------|-------------|
| `QuestionText` | string | Matches `Question.Prompt` exactly (REQ-004) |
| `Value` | string | Selected label(s); comma-joined for multi-select (REQ-009) |
| `Preview` | string | Selected option preview, if any (REQ-016) |
| `Notes` | string | Free-text user notes, if any (REQ-016) |

### Internal bridge type (`internal/tui/asker.go`)
`askRequest struct { questions []tools.Question; reply chan answerResult }`, `answerResult struct { answers []tools.Answer; cancelled bool }`.

## 8. API Contracts (Tool Definition)

### Tool: `ask_user_question`
- **Definition** (`Definition()` → `schema.ToolDefinition`): name `ask_user_question`; description adapted from reference prompt (when to ask; recommended option first with "(Recommended)"; "Other" is automatic; preview usage). `InputSchema` JSON Schema with:
  - `questions`: array, minItems 1, maxItems 4. Each item: `question` (string), `header` (string), `options` (array minItems 2 maxItems 4 of `{label, description, preview?}`), `multiSelect` (bool, default false).
  - `answers` (optional object: question-text → string), `annotations` (optional, keyed by question text: `{preview?, notes?}`), `metadata.source` (optional).
- **Execute(ctx, args)** behavior:
  1. `json.Unmarshal` args; on failure return error string (TC-015), no panic.
  2. Validate REQ-006/007/007a; on failure return descriptive error (TC-005..008), asker NOT called.
  3. If `answers` present → build `[]Answer` from entries verbatim, skip asker (REQ-021, TC-009/TC-017).
  4. Else if `asker == nil` (defensive; should be unreachable due to gating) → return REQ-014 message, no block (TC-013).
  5. Else `asker.Ask(ctx, questions)`; on `ErrUserCancelled` → REQ-011 message (TC-011); on ctx error → prompt-return message (TC-012).
  6. Format `User has answered your questions: "<q>"="<a>" [selected preview:\n..] [user notes: ..], ...`; truncate to 100k (REQ-022, TC-018). Return.
- **Output**: single string (see spec Output Examples).
- **Errors**: invalid JSON, schema violation, cancellation, ctx cancellation — all returned as clear strings/`error`, never panic.

## 9. Implementation Phases

### Phase 1: Foundation — interface & types (TDD setup)
- [ ] Create `internal/tools/ask_user_question.go` with `UserAsker`, `Question`, `Option`, `Answer`, `ErrUserCancelled`, and a skeleton `AskUserQuestionTool` (`Name`, `Definition`, `Execute` returning not-implemented).
- [ ] Add a `fakeAsker` test helper in `ask_user_question_test.go`.
- [ ] Write failing tests for `Name()` == `ask_user_question` and `Definition()` schema shape.

### Phase 2: Core tool logic (Red→Green→Refactor)
- [ ] TC-005..008: validation (uniqueness, array bounds) — tests first, then implement.
- [ ] TC-019: advisory lengths pass validation.
- [ ] TC-001..004, TC-009: single/multi-question, multi-select join, "Other", full `answers` injection — drive via `fakeAsker`.
- [ ] TC-010, TC-020: preview/notes annotations + exact-question-text keying.
- [ ] TC-011, TC-012, TC-013: cancellation, ctx cancellation, nil-asker fallback message.
- [ ] TC-015: malformed JSON. TC-016: not parallel-safe. TC-018 + NFR-004 benchmark: 100k cap and < 1ms format.

### Phase 3: TUI bridge (`tuiAsker`)
- [ ] `internal/tui/asker.go`: `Asker` with `chan askRequest`, `Ask` blocking on reply/ctx.
- [ ] `asker_test.go`: reply delivers answers; ctx cancel returns promptly; cancelled flag → `ErrUserCancelled`.

### Phase 4: TUI overlay & integration
- [ ] `internal/tui/askform.go` + tests: per-question list, multi-select toggle, auto "Other" → text input, preview render.
- [ ] `internal/tui/model.go`: hold asker + overlay; `listenForAsks` cmd; `Update`/`View` handle `askUserMsg`; reply on completion; key routing while overlay active.

### Phase 5: Wiring & mode gating
- [ ] `internal/app/runner.go`: add `userAsker` field + `SetUserAsker`; gate registration in `buildRegistry`.
- [ ] `internal/app/tui.go`: `RunTUI` builds `tuiAsker`, sets it on runner, passes via `tui.Config`.
- [ ] TC-014: assert tool present in TUI registry, absent in CLI/agentops/feishu/bench/subagent registries.

### Phase 6: Validation
- [ ] `go test ./...`, `gofmt -l .` clean, `go vet ./...`.
- [ ] Manual TUI smoke: run `go run ./cmd/fox`, have the agent call the tool, verify selection, multi-select, "Other", preview, and cancellation.

## 10. Technical Decisions

### Decision 1: Inject a `UserAsker` interface instead of giving the tool TUI knowledge
- **Choice**: The tool depends on a small `UserAsker` interface defined in `internal/tools`; the TUI implements it.
- **Rationale**: Constitution §2 (testability/DI) and §5 (separation). Lets the tool be fully unit-tested with a fake; avoids any `tools → tui` import.
- **Alternatives**: Tool directly imports the TUI program / uses globals. Rejected (untestable, import cycle, couples layers).
- **Trade-offs**: One extra indirection; worth it for testability.

### Decision 2: Mode gating via conditional registration (the `isEnabled()` analog)
- **Choice**: Register the tool only when `userAsker != nil` (TUI only); other runners never set it.
- **Rationale**: Faithful to Claude Code's `isEnabled()===false` when no interactive surface — the LLM never sees a tool it can't get answered (REQ-012..014). foxharness has no permission layer to host an equivalent.
- **Alternatives**: Always register + return a fallback message (option considered in spec). Kept only as a defensive last resort (step 4), not the primary mechanism.
- **Trade-offs**: Tool availability now depends on run mode; documented in the tool's spec and wiring.

### Decision 3: Bidirectional channel bridge mirroring `channelReporter`
- **Choice**: A long-lived `chan askRequest` carrying a per-request `reply` channel; the model listens via a `listenForAsks` cmd, like the existing events reader.
- **Rationale**: Reuses the proven engine↔UI messaging pattern; keeps `Execute` synchronous (matches `BaseTool`) while not blocking the UI loop.
- **Alternatives**: `tea.Program.Send` from the tool. Rejected: the tool would need a program handle (coupling) and still needs a reply path.
- **Trade-offs**: Slightly more plumbing than one-way reporting; necessary for a request/response interaction.

### Decision 4: Build a dedicated `askform` overlay rather than reuse `internal/tui/selector`
- **Choice**: New generic question overlay; borrow keybinding/list-render style from `selector`.
- **Rationale**: The existing `selector` is hard-wired to the checkpoint-rewind domain (`RestoreAction`, `SelectableMessage`). Reuse would require invasive generalization.
- **Alternatives**: Generalize `selector`. Rejected for v1 (scope creep, risk to rewind feature).
- **Trade-offs**: Some duplicated list-navigation code; acceptable, and a future refactor could unify them.

### Decision 5: Not parallel-safe (intentional divergence from reference)
- **Choice**: The tool does not implement `ParallelSafeTool`/returns no parallel safety (REQ-018).
- **Rationale**: `Execute` blocks on a single TUI overlay; concurrent invocations would contend for one screen. The reference marks it concurrency-safe only because answers are collected in a separate permission layer that foxharness lacks.
- **Alternatives**: Mark parallel-safe like the reference. Rejected — would allow overlapping overlays.
- **Trade-offs**: Question calls serialize with other tool calls; acceptable for an interactive prompt.

### Decision 6: Read-only is semantic only — no property is implemented
- **Choice**: Do not add any `IsReadOnly`-style property/method to the tool or to `BaseTool`/`Registry`. The tool is read-only by construction (its `Execute` performs no mutations); this is documented in its doc comment only.
- **Rationale**: In the reference, `isReadOnly()` exists because it is **consumed** — plan-mode gating, auto-approval (`extractMemories`), permission-prompt wording, and UI badges all read it. foxharness has **no consumer** for such a flag: it expresses "read-only" structurally instead (subagent read-only mode omits `write_file`/`edit_file` from the registry; `RunRestricted` uses a tool allowlist), and its plan mode does not filter tools at runtime. Adding the property would create dead metadata that nothing reads.
- **Alternatives**: (a) Add `IsReadOnly` to the tool interface and a consumer (e.g. plan-mode read-only enforcement) to mirror the reference — rejected as a separate, larger feature out of this scope. (b) Add the property with no consumer — rejected as dead code.
- **Trade-offs**: One reference property is intentionally not mirrored; if foxharness later introduces plan-mode read-only enforcement, the property (and this tool returning `true`) can be added then.
