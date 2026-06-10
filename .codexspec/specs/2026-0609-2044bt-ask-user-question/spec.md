# Feature: Ask User Question Tool (`ask_user_question`)

## Overview

A built-in tool that lets the foxharness core LLM proactively ask the user
structured, multiple-choice questions during execution — to gather preferences,
clarify ambiguous instructions, confirm implementation choices, or offer
directional options — instead of guessing and proceeding on assumptions.

The design **faithfully replicates** Claude Code's `AskUserQuestion` tool
(`src/tools/AskUserQuestionTool/`), adapted to foxharness's synchronous
`Execute(ctx, args) (string, error)` tool model and its two runtime surfaces:
the interactive TUI (`cmd/fox`) and the non-interactive runners (`fox exec`
one-shot, `cmd/agentops` server).

## Goals

- Give the LLM a first-class mechanism to ask the user rather than assume.
- Replicate the reference tool's input schema and question/option semantics
  (1–4 questions, 2–4 options each, `multiSelect`, auto "Other", `preview`,
  annotations).
- Mirror Claude Code's runtime gating: the tool is only available where a human
  can answer it; in non-interactive contexts it is not exposed to the LLM
  (the `isEnabled()` equivalent), so it can never hang.
- Preserve Claude Code's programmatic answer-injection path (the `answers`
  field, analogous to `updatedInput`) for headless/scripted callers.
- Comply with the project constitution: TDD-first, block-level Go docs,
  injectable dependencies, table-driven tests.

## User Stories

### Story 1: LLM clarifies an ambiguous request (interactive TUI)
**As** the foxharness core LLM
**I want** to present the user with 1–4 multiple-choice questions and block until they answer
**So that** I can resolve ambiguity and choose the right approach before doing work

**Acceptance Criteria:**
- [ ] The tool accepts 1–4 questions, each with a `header`, `question`, 2–4 `options`, and a `multiSelect` flag.
- [ ] In the TUI, each question renders as a selectable list; the user navigates and confirms a choice.
- [ ] An "Other" option is always appended automatically, allowing free-text input.
- [ ] When `multiSelect` is true, the user may select multiple options for that question.
- [ ] After all questions are answered, the tool returns the answers to the LLM as a single formatted string.

### Story 2: LLM offers a recommended direction with previews
**As** the foxharness core LLM
**I want** to mark a recommended option and attach `preview` content (e.g. code or a mockup) to options
**So that** the user can compare concrete artifacts before deciding

**Acceptance Criteria:**
- [ ] A recommended option is placed first and its `label` ends with "(Recommended)".
- [ ] When an option carries a `preview`, the TUI displays the preview for the focused/selected option.
- [ ] The selected option's `preview` (and any user `notes`) are included in the result returned to the LLM.

### Story 3: Tool is unavailable in non-interactive runs
**As** an operator running `fox exec` one-shot or the `agentops` server
**I want** the tool to be absent from the LLM's toolset when no human is at the keyboard
**So that** the agent never blocks waiting for an answer nobody can give

**Acceptance Criteria:**
- [ ] In interactive TUI mode the tool is registered and advertised to the LLM.
- [ ] In `fox exec` and `agentops` modes the tool is NOT registered, so it does not appear in `GetAvailableTools()`.
- [ ] If the tool is somehow executed without an interactive surface and without pre-supplied answers, it returns a clear, non-blocking message rather than hanging.

### Story 4: Headless caller supplies answers programmatically
**As** a scripted/headless caller
**I want** to pre-fill the `answers` field on the tool input
**So that** the tool resolves immediately without any interactive prompt (mirroring Claude Code's `updatedInput`)

**Acceptance Criteria:**
- [ ] When `answers` are present in the input, the tool uses them directly and skips interactive prompting.
- [ ] The returned string is formatted identically to the interactive path.

## Functional Requirements

### Input Schema (parity with reference)
- **[REQ-001]** Input MUST contain a `questions` array with 1–4 items.
- **[REQ-002]** Each question MUST have: `question` (string, the full prompt), `header` (string, short chip label; the ~12-character limit is **advisory guidance** surfaced to the LLM in the schema description, NOT enforced by validation — parity with reference), `options` (array, 2–4 items), and `multiSelect` (boolean, default `false`).
- **[REQ-003]** Each option MUST have `label` (concise display text; "1–5 words" is advisory guidance, not validated) and `description` (explanation/trade-offs), and MAY have `preview` (string content for visual comparison).
- **[REQ-004]** Input MAY contain an optional `answers` field (map keyed by the **exact question text** → answer string; multi-select answers comma-separated) used as the programmatic answer-injection channel (the `updatedInput` analog).
- **[REQ-005]** Input MAY contain optional `annotations` (map keyed by question text, each with optional `preview` and `notes`) and optional `metadata.source`.
- **[REQ-006]** Validation MUST reject input where question texts are not unique, or where option `label`s are not unique within a single question (parity with reference `UNIQUENESS_REFINE`).
- **[REQ-007]** Validation MUST reject `questions` arrays outside 1–4 items, and any `options` array outside 2–4 items.
- **[REQ-007a]** Validation scope MUST match the reference exactly: only array-size bounds (REQ-007) and uniqueness (REQ-006) are enforced. String-length limits on `header`/`label` are advisory only and MUST NOT cause validation failure; over-length values are handled at render time (see Edge Cases), not rejected.

### Interaction Semantics
- **[REQ-008]** The system MUST automatically append an "Other" choice to every question, enabling free-text user input; the LLM MUST NOT supply its own "Other"/"Custom" option.
- **[REQ-009]** Multi-select answers MUST be joined into a single comma-separated string in the result.
- **[REQ-010]** In the TUI, questions are presented sequentially; the tool blocks (respecting `ctx` cancellation) until all questions are answered or the user cancels.
- **[REQ-011]** If the user cancels/aborts the prompt, the tool MUST return a clear "user cancelled" result (not an empty/ambiguous answer).
- **[REQ-021]** Answer-consumption semantics MUST mirror the reference (`answers` defaults to `{}`; result is `Object.entries(answers)`): when an `answers` map is supplied it is treated as the **authoritative, already-collected** answer set and is formatted **verbatim, entry by entry, with no interactive re-prompting and no merging** of pre-supplied and interactive answers. Conversely, the interactive collector is responsible for producing a complete answer set (one entry per question) before formatting. A `answers` map that omits some questions is therefore NOT an error and MUST NOT trigger re-prompting — only the entries present are formatted (exactly as the reference does).

### Runtime Gating (the `isEnabled()` equivalent)
- **[REQ-012]** The tool MUST be registered into the tool registry ONLY for the interactive TUI surface (`cmd/fox` TUI).
- **[REQ-013]** The tool MUST NOT be registered for `fox exec` one-shot or `agentops` server surfaces.
- **[REQ-014]** If `Execute` is invoked with no interactive surface available AND no `answers` pre-supplied, it MUST return a descriptive message (e.g. "No interactive user available; proceed using your best judgement.") and MUST NOT block.

### Output
- **[REQ-015]** The result returned to the LLM MUST be a single string of the form: `User has answered your questions: "<question>"="<answer>", ...` followed by guidance to continue with the answers in mind (parity with reference `mapToolResultToToolResultBlockParam`).
- **[REQ-016]** When a selected option carries a `preview` or the user added `notes`, those MUST be appended to that question's segment of the output.
- **[REQ-022]** The formatted result string MUST be capped at **100,000 characters** (parity with the reference's `maxResultSizeChars: 100_000`); if the assembled result (e.g. due to large `preview` content) would exceed the cap, it MUST be truncated with a clear truncation marker rather than returned unbounded.

### Tool Properties
- **[REQ-017]** The tool MUST be **semantically read-only**: `Execute` MUST NOT mutate workspace files or system state (it only reads the user's selection). foxharness's `BaseTool`/`Registry` exposes **no** read-only property or method to report (unlike `ParallelSafeTool` for concurrency), so **no `IsReadOnly`-style property/method is added** — this requirement is satisfied purely by the implementation performing no mutations, stated in the tool's doc comment. *(Divergence from the reference, which has a consumed `isReadOnly()`; foxharness has no consumer for it — see plan Decision 6.)*
- **[REQ-018]** The tool MUST NOT be marked parallel-safe (it must not implement `ParallelSafeTool` returning true), because its interactive `Execute` blocks on a single TUI surface and concurrent prompts would conflict. *(Intentional divergence from the reference, which marks it concurrency-safe because answers are collected in a separate permission layer that foxharness lacks.)*
- **[REQ-019]** The tool's name MUST be `ask_user_question` (snake_case, consistent with existing tools `read_file`, `write_file`, `edit_file`, `read_todo`).
- **[REQ-020]** The tool's `Definition()` description and input schema MUST clearly instruct the LLM on usage (when to ask, recommended-first convention, "Other" is automatic, `preview` usage), adapted from the reference prompt text.

## Non-Functional Requirements

- **[NFR-001] Testability**: The interactive prompt mechanism MUST be abstracted behind an injectable interface (a "question asker") so `Execute` can be unit-tested without a real terminal. The TUI implementation and a test fake both satisfy it.
- **[NFR-002] Determinism**: All unit tests MUST be deterministic — no real TTY, no timing flakiness; the asker is faked in tests.
- **[NFR-003] Documentation**: All exported identifiers MUST carry block-level Go doc comments; no line-level teaching comments (per constitution §3).
- **[NFR-004] Performance**: Pure-CPU work (schema validation + result formatting) MUST complete in **< 1 ms** for the maximum input (4 questions × 4 options) on commodity hardware, and MUST be O(total options + total result bytes). End-to-end latency is **human-input-bound and therefore out of scope for performance measurement** — the tool spends effectively all wall-clock time awaiting the user. The only hard output bound is the 100,000-character result cap (REQ-022).
- **[NFR-005] Robustness**: Malformed JSON arguments and schema violations MUST yield a clear error string/`error`, never a panic.
- **[NFR-006] Security**: User free-text ("Other") input is untrusted text returned to the LLM only; it MUST NOT be interpreted as a command or path by this tool.

## Acceptance Criteria (Test Cases)

- **[TC-001]** Valid single-question, single-select input with a faked asker → returns `User has answered your questions: "<q>"="<chosen label>". ...`.
- **[TC-002]** Valid input with 4 questions → all four answers appear in the output, in order.
- **[TC-003]** `multiSelect: true` with two selected options → answers joined as `"opt A, opt B"` for that question.
- **[TC-004]** User selects the auto-appended "Other" and types free text → output reflects the free-text answer.
- **[TC-005]** Duplicate question texts → validation error (no prompt shown).
- **[TC-006]** Duplicate option labels within one question → validation error.
- **[TC-007]** `questions` empty or >4 → validation error.
- **[TC-008]** A question with <2 or >4 options → validation error.
- **[TC-009]** Pre-supplied `answers` present → asker is NOT invoked; output uses the injected answers.
- **[TC-010]** Selected option has `preview`, or annotation `notes` present → output includes the preview/notes segment.
- **[TC-011]** User cancels the prompt → returns a clear "user cancelled" result.
- **[TC-012]** Asker returns a context-cancelled error → `Execute` returns promptly with a clear message (no hang).
- **[TC-013]** No-interactive-surface fake + no `answers` → returns the descriptive "no interactive user" message; asker not invoked.
- **[TC-014]** Registry wiring: TUI registration path includes `ask_user_question` in `GetAvailableTools()`; `exec`/`agentops` paths do not.
- **[TC-015]** Malformed JSON arguments → returns an error, not a panic.
- **[TC-016]** `IsParallelSafe("ask_user_question")` returns `false`.
- **[TC-017]** Partial pre-supplied `answers` (covers 1 of 2 questions) → asker NOT invoked; output formats only the supplied entry, verbatim, with no error (REQ-021).
- **[TC-018]** Assembled result longer than 100,000 chars (oversized `preview`) → result is truncated to the cap with a truncation marker (REQ-022).
- **[TC-019]** Over-length `header` (>12 chars) and over-length `label` → validation PASSES (lengths are advisory, REQ-007a); rendering does not error.
- **[TC-020]** `answers`/`annotations` keyed by exact question text → entries match their questions; a key that matches no question text is formatted as-is without panicking (REQ-004).

## Edge Cases

- **No interactive surface but tool somehow invoked**: Return a descriptive non-blocking message (REQ-014); never wait on a TTY that has no reader.
- **User cancels mid-sequence (e.g. after answering 2 of 3 questions)**: Treat the whole call as cancelled and return a clear cancellation result (REQ-011); do not return partial answers as if complete.
- **Context cancellation / timeout while waiting**: `Execute` returns promptly honoring `ctx.Done()`.
- **`header` longer than 12 chars / `label` longer than guidance**: Documented as soft guidance to the LLM; the tool renders gracefully (truncate or wrap) rather than erroring on length alone.
- **`preview` provided on a `multiSelect` question**: Following the reference, previews are intended for single-select; the tool still renders the option label/description and does not crash if preview is present.
- **Duplicate or empty answers from the asker**: Defensive formatting — empty answer string is rendered as an explicit empty value, not dropped silently.
- **Partial `answers` injection** (map covers some questions, omits others): Not an error and not re-prompted (REQ-021). The result formats exactly the entries present, verbatim — matching the reference's `Object.entries(answers)`. The collector, when it runs, is the component responsible for completeness.
- **Result exceeds the 100,000-char cap** (e.g. very large `preview`): Output is truncated with a clear marker rather than returned unbounded (REQ-022).

## Output Examples

Single-select result:
```
User has answered your questions: "Which database should we use?"="PostgreSQL". You can now continue with the user's answers in mind.
```

Multi-question, with multi-select and a preview annotation:
```
User has answered your questions: "Which platforms to support?"="Web Browser, iOS App", "Which layout?"="Compact" selected preview:
+----------------+
| compact mockup |
+----------------+. You can now continue with the user's answers in mind.
```

Non-interactive fallback (REQ-014):
```
No interactive user is available to answer questions in this run mode. Proceed using your best judgement based on the information you already have.
```

User-cancelled result (REQ-011):
```
The user dismissed the questions without answering. Proceed using your best judgement or ask again later if needed.
```

## Out of Scope

- Porting Claude Code's full permission layer / `canUseTool` / hook architecture — foxharness has no such layer; runtime gating is done by conditional registration instead.
- Two-way Q&A relay over external channels (Feishu/Telegram/Discord); non-interactive surfaces simply do not register the tool.
- Rich/graphical preview rendering beyond what the terminal TUI can display (HTML previews, side-by-side panes are not required for v1; readable text rendering suffices).
- Persisting questions/answers to session memory or transcript beyond what the existing engine already records for tool calls.
- Changing the engine loop's turn/stop semantics around asking questions (the tool returns a normal tool result; no special turn-termination behavior is introduced).
