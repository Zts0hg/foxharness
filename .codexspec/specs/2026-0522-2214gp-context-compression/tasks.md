# Task Breakdown: Multi-Level Context Compression

## Overview
- **Total tasks**: 58
- **Parallelizable tasks**: 14
- **Estimated phases**: 7
- **Spec**: 2026-0522-2214gp-context-compression/spec.md
- **Plan**: 2026-0522-2214gp-context-compression/plan.md

## Implementation Status

All 7 phases implemented (2026-05-23). Full test suite passes including `-race`.

| Phase | Status | Notes |
|-------|--------|-------|
| 1. Schema & Provider Foundation | ✅ Complete | `schema.Usage`, `provider.GenerateResponse`, OpenAI/Claude/engine wiring |
| 2. Token Estimation | ✅ Complete | `ImprovedRoughEstimator`, `HybridEstimator` |
| 3. Model Registry | ✅ Complete | 7 default entries, config overrides, case-insensitive |
| 4. Thresholds & Guards | ✅ Complete | 4-level thresholds, recursive guard, env+config disable |
| 5. Tool Result Persistence | ✅ Complete | `TruncateToCap`, `PersistIfNeeded`, `EnforceBudget`, engine integration |
| 6. Structured Summary & Format | ✅ Complete | 9-section prompt, CJK detection, boundary marker, continuation wrapper, no-tools |
| 7. Integration & Benchmarks | ✅ Complete | E2E test, benchmarks well under NFR-002 targets |

Benchmarks (Apple M3): `ImprovedRoughEstimator`/`HybridEstimator` ~1-2µs per 500-message context, `PersistIfNeeded` ~2µs in-memory.

## User Story Coverage

| User Story | Tasks | Phase |
|-----------|-------|-------|
| US-001: Precise Token Tracking | 1.1–1.6, 2.1–2.3 | Phase 1, 2 |
| US-002: Tool Result Persistence | 5.0–5.4 | Phase 5 |
| US-003: Model Capability Registry | 3.1–3.2 | Phase 3 |
| US-004: Structured 9-Section Summary | 6.1–6.7 | Phase 6 |
| US-005: Backward Compatibility | 7.3 | Phase 7 |

## Phase 1: Schema & Provider Foundation (REQ-001)

Foundation layer: usage data types and provider interface update.

### Task 1.1: Write Usage struct tests
- **Type**: Testing (RED)
- **Files**: `internal/schema/message_test.go`
- **Description**: Write `TestUsageJSONRoundTrip` verifying Usage serializes/deserializes
  correctly with all fields including omitempty CacheCreationTokens and CacheReadTokens.
  Test zero-value Usage and Usage with all fields populated.
- **Dependencies**: None
- **Test Cases**: (supports TC-001)
- **Est. Complexity**: Low

### Task 1.2: Implement Usage struct
- **Type**: Implementation (GREEN)
- **Files**: `internal/schema/message.go`
- **Description**: Add `Usage` struct with `InputTokens`, `OutputTokens`,
  `CacheCreationTokens`, `CacheReadTokens` fields. Use `json:"input_tokens"`,
  `json:"cache_creation_tokens,omitempty"` tags. Make tests from 1.1 pass.
- **Dependencies**: Task 1.1
- **Est. Complexity**: Low

### Task 1.3: Write Message with Usage tests
- **Type**: Testing (RED)
- **Files**: `internal/schema/message_test.go`
- **Description**: Write `TestMessageWithUsage` and `TestMessageWithoutUsage`.
  Verify messages with Usage round-trip through JSON. Verify existing messages
  without Usage load correctly (nil pointer, omitted in JSON output).
- **Dependencies**: Task 1.2
- **Est. Complexity**: Low

### Task 1.4: Add Usage field to Message
- **Type**: Implementation (GREEN)
- **Files**: `internal/schema/message.go`
- **Description**: Add `Usage *Usage` field with `json:"usage,omitempty"` tag to
  Message struct. Verify all existing schema tests pass:
  `go test ./internal/schema/...`
- **Dependencies**: Task 1.3
- **Est. Complexity**: Low

### Task 1.5: Write GenerateResponse tests
- **Type**: Testing (RED)
- **Files**: `internal/provider/interface_test.go`
- **Description**: Write `TestGenerateResponseAccess` verifying GenerateResponse
  has Message and Usage fields, and that Message is accessible via field access.
- **Dependencies**: Task 1.4
- **Test Cases**: (supports TC-014)
- **Est. Complexity**: Low

### Task 1.6: Implement GenerateResponse + update interface
- **Type**: Implementation (GREEN)
- **Files**: `internal/provider/interface.go`
- **Description**: Add `GenerateResponse` struct with `Message *schema.Message`
  and `Usage schema.Usage` fields. Update `LLMProvider.Generate` signature to
  return `(*GenerateResponse, error)`. Add block comment on GenerateResponse.
- **Dependencies**: Task 1.5
- **Est. Complexity**: Low

### Task 1.7: Write OpenAI usage extraction tests [P]
- **Type**: Testing (RED)
- **Files**: `internal/provider/openai_test.go`
- **Description**: Write `TestOpenAIProviderReturnsUsage` (TC-001). Mock OpenAI
  response with `PromptTokens=1000`, `CompletionTokens=500`. Verify
  `GenerateResponse.Usage.InputTokens=1000`, `OutputTokens=500`.
- **Dependencies**: Task 1.6
- **Test Cases**: TC-001
- **Est. Complexity**: Medium

### Task 1.8: Implement OpenAI usage extraction [P]
- **Type**: Implementation (GREEN)
- **Files**: `internal/provider/openai.go`
- **Description**: Update `OpenAIProvider.Generate` to return `*GenerateResponse`.
  Extract `resp.Usage.PromptTokens` and `resp.Usage.CompletionTokens` from
  `openai.ChatCompletion` response. Map to `schema.Usage{InputTokens, OutputTokens}`.
- **Dependencies**: Task 1.7
- **Est. Complexity**: Medium

### Task 1.9: Write Claude usage extraction tests [P]
- **Type**: Testing (RED)
- **Files**: `internal/provider/claude_test.go`
- **Description**: Write `TestClaudeProviderReturnsUsage` (TC-001). Mock Anthropic
  response with `InputTokens=1000`, `OutputTokens=500`. Verify
  `GenerateResponse.Usage.InputTokens=1000`, `OutputTokens=500`.
- **Dependencies**: Task 1.6
- **Test Cases**: TC-001
- **Est. Complexity**: Medium

### Task 1.10: Implement Claude usage extraction [P]
- **Type**: Implementation (GREEN)
- **Files**: `internal/provider/claude.go`
- **Description**: Update `ClaudeProvider.Generate` to return `*GenerateResponse`.
  Extract `resp.Usage.InputTokens` and `resp.Usage.OutputTokens` from
  `anthropic.Message` response. Map to `schema.Usage{InputTokens, OutputTokens}`.
- **Dependencies**: Task 1.9
- **Est. Complexity**: Medium

### Task 1.11: Write engine callModel tests
- **Type**: Testing (RED)
- **Files**: `internal/engine/loop_test.go`
- **Description**: Write `TestCallModelUsesGenerateResponse`. Verify engine
  accesses `resp.Message` and sets `resp.Message.Usage = &resp.Usage` before
  appending to history.
- **Dependencies**: Task 1.8, Task 1.10
- **Est. Complexity**: Medium

### Task 1.12: Update engine callModel
- **Type**: Implementation (GREEN)
- **Files**: `internal/engine/loop.go`
- **Description**: Update `callModel` to handle `*GenerateResponse`. Extract
  `response.Message` and set `response.Message.Usage = &response.Usage` before
  returning. Verify all existing engine tests pass: `go test ./internal/engine/...`
- **Dependencies**: Task 1.11
- **Est. Complexity**: Medium

**Phase 1 Checkpoint**:
```bash
go test ./internal/schema/... ./internal/provider/... ./internal/engine/...
```
All existing + new tests must pass.

---

## Phase 2: Token Estimation (REQ-002, REQ-002a)

Improved estimation and hybrid counting. **Phase 2 and Phase 3 can run in parallel.**

### Task 2.1: Write ImprovedRoughEstimator tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/estimator_test.go` (NEW)
- **Description**: Write table-driven `TestImprovedRoughEstimator` (TC-017).
  Cases: plain text 1000 bytes → 333, JSON 1000 bytes → 666, empty string → 0,
  mixed content (text with embedded JSON). JSON detected by first non-space
  char `{` or `[`.
- **Dependencies**: Task 1.4 (schema.Usage)
- **Test Cases**: TC-017
- **Est. Complexity**: Low

### Task 2.2: Implement ImprovedRoughEstimator
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/estimator.go` (NEW)
- **Description**: Implement `ImprovedRoughEstimator` struct with
  `EstimateText(text string) int` and `EstimateMessages(messages []schema.Message) int`.
  Logic: plain text `len/4 * 4/3`, JSON `len/2 * 4/3`, JSON detection by
  first non-space char. Add block comments on exported types.
- **Dependencies**: Task 2.1
- **Est. Complexity**: Low

### Task 2.3: Write HybridEstimator tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/estimator_test.go`
- **Description**: Write `TestHybridEstimator_WithUsage` (TC-002) and
  `TestHybridEstimator_WithoutUsage` (TC-003). With usage: last assistant has
  Usage, messages after estimated. Without: full rough fallback. Edge (EC-001):
  zero-value Usage treated as no usage.
- **Dependencies**: Task 2.2
- **Test Cases**: TC-002, TC-003, EC-001
- **Est. Complexity**: Medium

### Task 2.4: Implement HybridEstimator
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/estimator.go`
- **Description**: Implement `HybridEstimator` struct with rough `TokenEstimator`
  field and `Estimate(messages []schema.Message) int` method. Scan from end,
  find last assistant with non-nil Usage, sum exact tokens for known portion,
  estimate remainder. Fallback to full rough if no usage found.
- **Dependencies**: Task 2.3
- **Est. Complexity**: Medium

### Task 2.5: Replace RoughEstimator references
- **Type**: Refactoring
- **Files**: `internal/compaction/compactor.go`, `internal/compaction/compactor_test.go`
- **Description**: Update existing compaction tests to use `ImprovedRoughEstimator`.
  Update `Compactor` to default to `ImprovedRoughEstimator`. Remove old
  `RoughEstimator` from compaction package if no other references exist.
  Verify: `go test ./internal/compaction/...`
- **Dependencies**: Task 2.4
- **Est. Complexity**: Low

**Phase 2 Checkpoint**:
```bash
go test ./internal/compaction/... -run "TestImprovedRough|TestHybrid"
```

---

## Phase 3: Model Registry (REQ-003)

Model name → context window mapping. **Runs in parallel with Phase 2.**

### Task 3.1: Write registry lookup tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/registry_test.go` (NEW)
- **Description**: Write table-driven `TestModelRegistry_Lookup` (TC-004, TC-005).
  Cases: `glm-4` → 128000, unknown model → 128000 default, prefix match
  `glm-4-air-x` → 128000, case insensitive `GLM-4` → 128000 (EC-007).
  Verify all 7 default entries produce correct values.
- **Dependencies**: Task 1.4 (schema types)
- **Test Cases**: TC-004, TC-005, EC-007
- **Est. Complexity**: Low

### Task 3.2: Implement ModelRegistry
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/registry.go` (NEW)
- **Description**: Implement `ModelRegistry` with `NewModelRegistry()`,
  `Lookup(modelName string) int`, `SetConfigOverride(overrides map[string]int)`.
  Default entries: glm-4 (128K), glm-4-plus (128K), glm-4-air (128K),
  claude-3.5-sonnet (200K), claude-3-opus (200K), claude-4-sonnet (200K),
  claude-4-opus (200K). Case-insensitive lookup. Longest prefix match.
- **Dependencies**: Task 3.1
- **Est. Complexity**: Medium

### Task 3.3: Write config override tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/registry_test.go`
- **Description**: Write `TestModelRegistry_ConfigOverride` (TC-006). Cases:
  config overrides exact model match, config adds new model not in defaults,
  config override takes precedence over prefix match.
- **Dependencies**: Task 3.2
- **Test Cases**: TC-006
- **Est. Complexity**: Low

### Task 3.4: Implement config override
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/registry.go`
- **Description**: Implement `SetConfigOverride` and update `Lookup` priority:
  config override (exact match) → registry (longest prefix match) → default 128000.
- **Dependencies**: Task 3.3
- **Est. Complexity**: Low

**Phase 3 Checkpoint**:
```bash
go test ./internal/compaction/... -run "TestModelRegistry"
```

---

## Phase 4: Compaction Thresholds & Guards (REQ-004, REQ-004a, REQ-004b)

Multi-threshold system, recursive guard, enable/disable. Depends on Phase 2 + Phase 3.

### Task 4.1: Write threshold tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/thresholds_test.go` (NEW)
- **Description**: Write `TestThresholdConfig_128K` (TC-016) and
  `TestThresholdConfig_ShortWindow` (EC-009). 128K: effective=108K, auto=95K,
  warning=75K, blocking=105K. Short window (<50K): all values positive,
  warn if effective < 40K. Test default field values (ReservedForSummary=20000, etc.).
- **Dependencies**: Task 2.4 (estimator), Task 3.2 (registry)
- **Test Cases**: TC-016, EC-009
- **Est. Complexity**: Low

### Task 4.2: Implement ThresholdConfig
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/thresholds.go` (NEW)
- **Description**: Implement `ThresholdConfig` struct with ContextWindow,
  ReservedForSummary (20000), AutoCompactBuffer (13000), WarningBuffer (20000),
  BlockingBuffer (3000). Implement `EffectiveWindow()`, `AutoCompact()`,
  `Warning()`, `Blocking()` methods. Log warning if effective < 40K.
- **Dependencies**: Task 4.1
- **Est. Complexity**: Low

### Task 4.3: Write recursive guard tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/compactor_test.go`
- **Description**: Write `TestCompactor_RecursiveGuard` (TC-018, EC-011).
  When `compacting == true`, `MaybeCompact` returns original messages immediately.
  During normal compaction, `compacting` is set and prevents re-entry.
- **Dependencies**: Task 2.5 (compactor updated)
- **Test Cases**: TC-018, EC-011
- **Est. Complexity**: Low

### Task 4.4: Add recursive guard
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/compactor.go`
- **Description**: Add `compacting bool` field to Compactor struct. Add guard
  check at start of `MaybeCompact`: return original messages if compacting.
  Set `compacting = true` before compaction starts with `defer` reset.
- **Dependencies**: Task 4.3
- **Est. Complexity**: Low

### Task 4.5: Write disable toggle tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/compactor_test.go`
- **Description**: Write `TestCompactor_Disabled` (TC-023, TC-024, EC-014).
  Cases: `FOXHARNESS_DISABLE_COMPACT` set → all disabled,
  `FOXHARNESS_DISABLE_AUTO_COMPACT` set → auto disabled,
  `compaction.enabled=false` → auto disabled, env var takes precedence over config.
  Use `t.Setenv` for env var tests.
- **Dependencies**: Task 4.4
- **Test Cases**: TC-023, TC-024, EC-014
- **Est. Complexity**: Medium

### Task 4.6: Add disable checks
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/compactor.go`
- **Description**: Add `disabled` and `autoDisabled` bool fields. In
  `NewCompactor`, check `FOXHARNESS_DISABLE_COMPACT` and
  `FOXHARNESS_DISABLE_AUTO_COMPACT` env vars via `os.LookupEnv`. Add guard
  checks at start of `MaybeCompact`: return original if disabled or autoDisabled
  for automatic compaction.
- **Dependencies**: Task 4.5
- **Est. Complexity**: Medium

### Task 4.7: Write restructured Compactor tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/compactor_test.go`
- **Description**: Write `TestNewCompactor_WithRegistry` verifying new constructor
  uses ModelRegistry for context window lookup and creates ThresholdConfig.
  Verify CompactionConfig fields propagate correctly.
- **Dependencies**: Task 4.6, Task 3.4
- **Est. Complexity**: Medium

### Task 4.8: Restructure Compactor
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/compactor.go`
- **Description**: Restructure `Compactor` struct to include `hybrid *HybridEstimator`,
  `registry *ModelRegistry`, `thresholds ThresholdConfig` fields. Update
  `NewCompactor` to accept model name, use registry for context window, create
  ThresholdConfig. Remove old `Config` and `DefaultConfig`. CompactionConfig
  contains `Enabled bool` read from config, overridden by env vars.
- **Dependencies**: Task 4.7
- **Est. Complexity**: High

**Phase 4 Checkpoint**:
```bash
go test ./internal/compaction/... -run "TestThresholdConfig|TestCompactor_Recursive|TestCompactor_Disabled|TestNewCompactor"
```

---

## Phase 5: Tool Result Persistence (REQ-005, REQ-005a)

Persist large tool results to disk. **Phase 5 can start in parallel with Phase 2–4
(after Phase 1 only).**

### Task 5.0: Add ToolResultsDir to session [P]
- **Type**: Implementation
- **Files**: `internal/session/session.go`
- **Description**: Add `ToolResultsDir() string` method returning
  `filepath.Join(s.RootDir, "tool-results")`. Create this directory in
  `Manager.Create` alongside existing directories. Add test for new method.
- **Dependencies**: None (session package is independent)
- **Est. Complexity**: Low

### Task 5.1: Write truncation tests
- **Type**: Testing (RED)
- **Files**: `internal/toolresult/truncate_test.go` (NEW)
- **Description**: Write `TestTruncateToCap` (TC-025, EC-013). Cases: content
  < 400KB → unchanged, content = 400KB → unchanged, content > 400KB → truncated
  with `\n...[truncated at 400KB, original size: X KB]` notice. Test 600KB
  content (exceeds both persistence and cap, EC-013).
- **Dependencies**: None (new package)
- **Test Cases**: TC-025, EC-013
- **Est. Complexity**: Low

### Task 5.2: Implement TruncateToCap
- **Type**: Implementation (GREEN)
- **Files**: `internal/toolresult/truncate.go` (NEW)
- **Description**: Create `toolresult` package with `MaxToolResultBytes = 400_000`.
  Implement `TruncateToCap(content string) string` that truncates at cap and
  appends notice. Add package block comment.
- **Dependencies**: Task 5.1
- **Est. Complexity**: Low

### Task 5.3: Write persistence threshold tests
- **Type**: Testing (RED)
- **Files**: `internal/toolresult/persist_test.go` (NEW)
- **Description**: Write `TestPersistIfNeeded` (TC-007, TC-008, EC-002, EC-003,
  EC-008). Cases: content > 50K → persisted with 2KB preview, content ≤ 50K →
  not persisted, content = exactly 50,000 → not persisted, empty content → not
  persisted, file already exists → skip write (idempotent). Use `memFS` test double.
- **Dependencies**: Task 5.2
- **Test Cases**: TC-007, TC-008, EC-002, EC-003, EC-008
- **Est. Complexity**: Medium

### Task 5.4: Implement PersistIfNeeded + FileSystem
- **Type**: Implementation (GREEN)
- **Files**: `internal/toolresult/persist.go` (NEW)
- **Description**: Define `FileSystem` interface with `WriteFile`, `Stat`,
  `MkdirAll`. Implement `osFS` production wrapper and `memFS` test double.
  Define constants: `PersistenceThreshold=50000`, `PerTurnBudget=200000`,
  `PreviewSize=2048`. Implement `PersistedResult` struct and `PersistIfNeeded`.
  Preview format: `<persisted-output>...Full output saved to: {path}\nPreview...`.
- **Dependencies**: Task 5.3
- **Est. Complexity**: Medium

### Task 5.5: Write budget enforcement tests
- **Type**: Testing (RED)
- **Files**: `internal/toolresult/persist_test.go`
- **Description**: Write `TestEnforceBudget` (TC-009, EC-004). Cases: total ≤ 200K
  → no persistence, total > 200K → largest new results persisted first, seen
  results (in seenIDs) never persisted, parallel results processed together.
- **Dependencies**: Task 5.4
- **Test Cases**: TC-009, EC-004
- **Est. Complexity**: Medium

### Task 5.6: Implement EnforceBudget
- **Type**: Implementation (GREEN)
- **Files**: `internal/toolresult/persist.go`
- **Description**: Implement `EnforceBudget(fs FileSystem, dir string, results
  []PersistedResult, seenIDs map[string]bool) []PersistedResult`. Sort new results
  by size (largest first), persist until total within budget. Skip results in
  seenIDs.
- **Dependencies**: Task 5.5
- **Est. Complexity**: Medium

### Task 5.7: Write engine persistence tests
- **Type**: Testing (RED)
- **Files**: `internal/engine/loop_test.go`
- **Description**: Write `TestEngine_ToolResultPersistence` integration test.
  Verify tool results flow through TruncateToCap then EnforceBudget. Verify
  seenIDs built from contextHistory. Verify ToolResultsDir used for storage.
- **Dependencies**: Task 5.6, Task 5.0, Task 1.12
- **Est. Complexity**: Medium

### Task 5.8: Wire tool result processing into engine
- **Type**: Implementation (GREEN)
- **Files**: `internal/engine/loop.go`
- **Description**: After `executeToolCalls`, apply tool result processing:
  (1) truncate each result with `toolresult.TruncateToCap`, (2) build seenIDs
  from contextHistory, (3) call `toolresult.EnforceBudget` on results,
  (4) use `session.ToolResultsDir()` for storage path.
- **Dependencies**: Task 5.7
- **Est. Complexity**: Medium

**Phase 5 Checkpoint**:
```bash
go test ./internal/toolresult/... ./internal/engine/... -run "TestTruncate|TestPersist|TestEnforce|TestEngine_Tool"
```

---

## Phase 6: Structured Summary & Compaction Format (REQ-006–REQ-009c)

9-section summary, language detection, boundary markers, cleanup.
Depends on Phase 4.

### Task 6.1: Write prompt builder tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/prompt_test.go` (NEW)
- **Description**: Write `TestBuildCompactPrompt_English` (TC-013) and
  `TestBuildCompactPrompt_Chinese` (TC-012, EC-010). Cases: English first
  message → English prompt, Chinese first message → Chinese prompt, mixed
  languages → first message wins. Verify prompt includes NO_TOOLS_PREAMBLE
  ("CRITICAL: Respond with TEXT ONLY") and NO_TOOLS_TRAILER ("REMINDER: Do NOT
  call any tools"). Verify 9-section structure in prompt body.
- **Dependencies**: Task 4.8
- **Test Cases**: TC-012, TC-013, EC-010
- **Est. Complexity**: Medium

### Task 6.2: Implement BuildCompactPrompt + DetectSummaryLanguage
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/prompt.go` (NEW)
- **Description**: Implement `BuildCompactPrompt(messages []schema.Message,
  language string) string` with 9-section prompt template. Implement
  `DetectSummaryLanguage(messages []schema.Message) string` using CJK Unicode
  range detection on first user message. Extract CJK detection to reusable
  helper. Add block comments.
- **Dependencies**: Task 6.1
- **Est. Complexity**: Medium

### Task 6.3: Write FormatSummary tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/prompt_test.go`
- **Description**: Write `TestFormatSummary` (TC-011). Cases: strip `<analysis>`
  block and extract `<summary>` content, handle missing `<analysis>` (summary-only),
  handle missing `<summary>` (return raw), handle nested tags.
- **Dependencies**: Task 6.2
- **Test Cases**: TC-011
- **Est. Complexity**: Low

### Task 6.4: Implement FormatSummary
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/prompt.go`
- **Description**: Implement `FormatSummary(raw string) string`. Strip `<analysis>`
  draft area, extract `<summary>` content. Return raw if no summary tags found.
- **Dependencies**: Task 6.3
- **Est. Complexity**: Low

### Task 6.5: Write boundary marker tests [P]
- **Type**: Testing (RED)
- **Files**: `internal/compaction/boundary_test.go` (NEW)
- **Description**: Write `TestBoundaryMessage` (TC-021). Verify marker has trigger,
  pre_tokens, messages_summarized, timestamp. Verify marker is a system message.
  Verify JSON round-trip of CompactBoundary data. Verify trigger values ("auto",
  "manual").
- **Dependencies**: Task 4.8
- **Test Cases**: TC-021
- **Est. Complexity**: Low

### Task 6.6: Implement CompactBoundary + BoundaryMessage [P]
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/boundary.go` (NEW)
- **Description**: Implement `CompactBoundary` struct with Trigger, PreTokens,
  MessagesSummarized, Timestamp fields. Implement `BoundaryMessage(boundary
  CompactBoundary) schema.Message` creating a system message with JSON-encoded
  boundary data.
- **Dependencies**: Task 6.5
- **Est. Complexity**: Low

### Task 6.7: Write continuation instruction tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/compactor_test.go`
- **Description**: Write `TestSummaryMessage_Continuation` (TC-020). Verify
  summary message contains continuation wrapper ("Continue the conversation from
  where it left off without asking the user any further questions"), references
  transcript path, contains formatted summary content. Verify message role is
  `user`.
- **Dependencies**: Task 6.4
- **Test Cases**: TC-020
- **Est. Complexity**: Low

### Task 6.8: Update SummaryMessage with continuation wrapper
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/compactor.go`
- **Description**: Update `SummaryMessage` to use continuation wrapper (REQ-009a).
  Wrap formatted summary with header indicating compacted context, transcript
  path reference, and continuation instruction. Message role is `user`. Remove
  old `stripExistingCompactionSummary` if no longer needed. Remove old
  `renderMessagesForSummary` (replaced by `BuildCompactPrompt`).
- **Dependencies**: Task 6.7
- **Est. Complexity**: Medium

### Task 6.9: Write no-tools constraint tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/compactor_test.go`
- **Description**: Write `TestCompact_SummaryWithNoTools` (TC-019, EC-012).
  Verify empty tools list passed to Generate during compaction. Verify prompt
  includes preamble and trailer. If LLM returns tool calls in summary response,
  extract text content only (EC-012).
- **Dependencies**: Task 6.8
- **Test Cases**: TC-019, EC-012
- **Est. Complexity**: Medium

### Task 6.10: Update Compactor.summarize for no-tools constraint
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/compactor.go`
- **Description**: Update `Compactor.summarize` to pass empty tools list to
  provider Generate call. Use `BuildCompactPrompt` for prompt construction.
  Apply `FormatSummary` on response. Extract text content only if LLM returns
  tool calls (EC-012).
- **Dependencies**: Task 6.9
- **Est. Complexity**: Medium

### Task 6.11: Write post-compact cleanup tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/compactor_test.go`
- **Description**: Write `TestPostCompactCleanup` (TC-022, EC-015). Verify
  cached token counts invalidated after compaction. Verify subsequent estimation
  uses new message order, not stale pre-compaction data.
- **Dependencies**: Task 6.10
- **Test Cases**: TC-022, EC-015
- **Est. Complexity**: Low

### Task 6.12: Add post-compact cleanup
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/compactor.go`
- **Description**: Add cleanup logic after successful compaction in `MaybeCompact`.
  Invalidate any cached token counts. Log compaction event with pre/post token
  counts for observability.
- **Dependencies**: Task 6.11
- **Est. Complexity**: Low

### Task 6.13: Write compaction message format tests
- **Type**: Testing (RED)
- **Files**: `internal/compaction/compactor_test.go`
- **Description**: Write `TestMaybeCompact_MessageFormat` integration test.
  Verify output format: [boundary] [summary] [anchor?] [recent]. Verify
  protocol boundary splitting (TC-010): split adjusted to avoid breaking
  tool call/result pairs. Verify no compaction when all messages are recent
  (EC-005). Verify summary LLM failure returns original messages (EC-006).
- **Dependencies**: Task 6.12, Task 6.6
- **Test Cases**: TC-010, EC-005, EC-006
- **Est. Complexity**: High

### Task 6.14: Assemble full MaybeCompact
- **Type**: Implementation (GREEN)
- **Files**: `internal/compaction/compactor.go`
- **Description**: Assemble all components in `MaybeCompact` flow:
  (1) check guards, (2) estimate tokens, (3) compare threshold, (4) split
  with protocol boundary, (5) build prompt, (6) summarize with no tools,
  (7) format summary, (8) build boundary marker, (9) build summary message,
  (10) assemble: [boundary] [summary] [anchor?] [recent], (11) cleanup.
  Verify all compaction tests pass.
- **Dependencies**: Task 6.13
- **Est. Complexity**: High

**Phase 6 Checkpoint**:
```bash
go test ./internal/compaction/... -run "TestBuildCompact|TestFormat|TestBoundary|TestSummary|TestCompact|TestMaybeCompact"
```

---

## Phase 7: Integration, Edge Cases & Benchmarks

Final integration, edge cases, backward compatibility, performance.

### Task 7.1: Write engine end-to-end test
- **Type**: Testing (RED)
- **Files**: `internal/engine/loop_test.go`
- **Description**: Write `TestEngine_FullCompactionFlow` integration test. Create
  session with history exceeding auto-compact threshold. Verify usage tracking
  through callModel → HybridEstimator → MaybeCompact. Verify tool result
  persistence integration. Verify compacted context has correct format
  (boundary + summary + recent).
- **Dependencies**: Task 6.14, Task 5.8
- **Est. Complexity**: High

### Task 7.2: Wire all components in engine
- **Type**: Implementation (GREEN)
- **Files**: `internal/engine/loop.go`, `internal/engine/context.go`
- **Description**: Final engine wiring: update `context.go` to use HybridEstimator
  instead of RoughEstimator, update `NewAgentEngine` or `WithCompactor` to pass
  model name for registry lookup, ensure compactor initialization uses model
  registry. Verify all engine tests pass.
- **Dependencies**: Task 7.1
- **Est. Complexity**: High

### Task 7.3: Write edge case sweep tests [P]
- **Type**: Testing (RED)
- **Files**: `internal/compaction/thresholds_test.go`, `internal/compaction/compactor_test.go`
- **Description**: Write tests for remaining edge cases: EC-009 (very short
  context window warning), EC-014 (disable env var vs config conflict). EC-007
  already covered at unit level in Phase 3.1 — only add integration test if
  needed for engine → registry → threshold flow.
- **Dependencies**: Task 7.2
- **Test Cases**: EC-009, EC-014
- **Est. Complexity**: Low

### Task 7.4: Verify backward compatibility [P]
- **Type**: Testing
- **Files**: Multiple (verification across all packages)
- **Description**: Run `go test ./...` and verify all existing tests pass.
  Create test loading an existing session (TC-015) without migration. Verify
  backward-compatible provider call: `resp, err := p.Generate(...)` followed by
  `resp.Message` access (TC-014).
- **Dependencies**: Task 7.2
- **Test Cases**: TC-014, TC-015
- **Est. Complexity**: Medium

### Task 7.5: Write performance benchmarks [P]
- **Type**: Testing
- **Files**: `internal/compaction/estimator_test.go`, `internal/toolresult/persist_test.go`
- **Description**: Write `BenchmarkImprovedRoughEstimator` for 500-message context
  (verify < 1ms per NFR-002). Write `BenchmarkHybridEstimator` for 500-message
  context with mixed usage data. Write `BenchmarkPersistIfNeeded` for disk I/O
  (verify < 10ms per NFR-002).
- **Dependencies**: Task 7.2
- **Est. Complexity**: Low

### Task 7.6: Run full test suite and benchmarks
- **Type**: Verification
- **Files**: All modified/new files
- **Description**: Run `go test ./...` — all tests must pass. Run benchmarks:
  `go test -bench=. ./internal/compaction/... ./internal/toolresult/...`.
  Verify coverage: `go test -cover ./internal/compaction/...
  ./internal/toolresult/... ./internal/provider/... ./internal/engine/...`.
- **Dependencies**: Task 7.3, Task 7.4, Task 7.5
- **Est. Complexity**: Low

**Phase 7 Checkpoint**:
```bash
go test ./...
go test -bench=. ./internal/compaction/... ./internal/toolresult/...
```

---

## Execution Order

```
Phase 1: 1.1 → 1.2 → 1.3 → 1.4 → 1.5 → 1.6 → ┬→ 1.7 → 1.8 ─┐
                                                  └→ 1.9 → 1.10 ─┤
                                                                  │
                                                          1.11 → 1.12
                                                                  │
Phase 2: ─────────────────────────────────────────────────────────┤
  2.1 → 2.2 → 2.3 → 2.4 → 2.5                                   │
                                                                  │
Phase 3: [P with Phase 2] ────────────────────────────────────────┤
  3.1 → 3.2 → 3.3 → 3.4                                         │
                                                                  │
Phase 4: ─────────────────────────────────────────────────────────┤
  4.1 → 4.2 ──────────────────────────────────────────┐           │
  4.3 → 4.4 → 4.5 → 4.6 ─────────────────────────────┤→ 4.7 → 4.8│
                                                       │           │
Phase 5: [P with Phase 2–4] ──────────────────────────┤───────────┤
  5.0 [P]                                             │           │
  5.1 → 5.2 → 5.3 → 5.4 → 5.5 → 5.6 → 5.7 → 5.8    │           │
                                                       │           │
Phase 6: ──────────────────────────────────────────────┤───────────┤
  6.1 → 6.2 → 6.3 → 6.4 ─────────────────┐           │           │
  6.5 → 6.6 [P with 6.1-6.4] ────────────┤           │           │
  6.7 → 6.8 → 6.9 → 6.10 ←──────────────┘           │           │
                    │                                  │           │
  6.11 → 6.12 → 6.13 ←──────────────────────────────┘           │
                    │                                              │
  6.14 ←───────────┘                                              │
                    │                                              │
Phase 7: ──────────┴──────────────────────────────────────────────┘
  7.1 → 7.2 → ┬→ 7.3 [P]
               ├→ 7.4 [P]
               ├→ 7.5 [P]
               └→ 7.6
```

## Checkpoints

- [x] **Checkpoint 1** (After Phase 1): `go test ./internal/schema/... ./internal/provider/... ./internal/engine/...`
  Verify usage data flows through provider → engine → message history.

- [x] **Checkpoint 2** (After Phase 2+3): `go test ./internal/compaction/... -run "TestImprovedRough|TestHybrid|TestModelRegistry"`
  Verify improved estimation and model registry work independently.

- [x] **Checkpoint 3** (After Phase 4): `go test ./internal/compaction/... -run "TestThreshold|TestCompactor_"`
  Verify thresholds, guards, and restructured Compactor.

- [x] **Checkpoint 4** (After Phase 5): `go test ./internal/toolresult/... ./internal/engine/... -run "TestTruncate|TestPersist|TestEnforce|TestEngine_Tool"`
  Verify tool result persistence and engine integration.

- [x] **Checkpoint 5** (After Phase 6): `go test ./internal/compaction/...`
  Verify full compaction pipeline with structured summary.

- [x] **Checkpoint 6** (After Phase 7): `go test ./...` + benchmarks
  Verify everything works together, backward compatible, and meets performance targets.
