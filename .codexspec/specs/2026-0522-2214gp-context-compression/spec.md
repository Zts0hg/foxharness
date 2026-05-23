# Feature: Multi-Level Context Compression

## Overview

Replace the current single-level, rough-estimation-based compaction system with a
multi-layer context compression architecture inspired by Claude Code's production
proven design. The new system uses actual API token usage data for precise context
tracking, implements three progressive compression layers (tool result persistence,
micro-compact, full LLM summary), and preserves foxharness's unique protocol
boundary splitting for clean compaction.

The implementation is divided into two phases:
- **Phase 1** (this spec): Provider usage data, model capability registry, tool
  result persistence, structured full compaction with 9-section summary
- **Phase 2** (future): Micro-compact, post-compact recovery, circuit breaker

## Goals

- Improve token counting accuracy by using actual API usage data instead of rough
  rune-based estimation
- Prevent context overflow from large tool results through automatic persistence
  to disk with inline previews
- Improve compaction summary quality with a structured 9-section format that
  preserves critical coding context (files, errors, decisions)
- Maintain backward compatibility with existing sessions, provider implementations,
  and the current session directory layout
- Preserve foxharness's protocol boundary splitting, which Claude Code lacks

## User Stories

### Story 1: Precise Token Tracking
**As an** agent developer
**I want** the system to use actual API token usage data for context management
**So that** compaction triggers at the right time and avoids premature or late
compression

**Acceptance Criteria:**
- [ ] The `LLMProvider` interface returns token usage (input, output, cache tokens)
  alongside the response message
- [ ] The engine uses exact usage for the known portion of context and rough
  estimation only for messages after the last API response
- [ ] Existing code that calls `Generate` continues to work without modification
- [ ] Both `ClaudeProvider` and `OpenAIProvider` extract and return usage data

### Story 2: Tool Result Persistence
**As an** AI coding agent
**I want** large tool results automatically persisted to disk with a preview in
context
**So that** my context window is not consumed by large file reads or command
outputs while I can still reference the full content when needed

**Acceptance Criteria:**
- [ ] Tool results exceeding 50,000 characters are automatically written to
  `{session_dir}/tool-results/{toolCallID}.{json|txt}`
- [ ] The context retains a 2KB preview with a reference to the persisted file
- [ ] A per-turn budget of 200,000 characters is enforced; when exceeded, the
  largest new results are persisted first
- [ ] Results that have already been seen by the model are never retroactively
  persisted (cache consistency)

### Story 3: Model Capability Registry
**As a** system operator
**I want** to configure the context window size per model
**So that** compaction thresholds are accurate for different LLM backends

**Acceptance Criteria:**
- [ ] A model capability registry maps model names to context window sizes
- [ ] Config file can override or extend the registry
- [ ] The registry is consulted at session initialization, before any API calls
- [ ] A sensible default (128K) is used when the model is unknown

### Story 4: Structured 9-Section Summary
**As an** AI coding agent
**I want** compaction to produce a structured 9-section summary
**So that** critical context (files modified, errors encountered, current task
state) is preserved across compaction boundaries

**Acceptance Criteria:**
- [ ] The summary contains 9 sections: Primary Request, Key Concepts,
  Files/Code, Errors/Fixes, Problem Solving, All User Messages, Pending Tasks,
  Current Work, Next Step
- [ ] Summary language auto-detects from the user's first message (CJK detection)
- [ ] The summary replaces the current generic Chinese summary prompt
- [ ] Protocol boundary splitting is preserved (tool call/result pairs are not
  broken across the compaction split point)

### Story 5: Backward Compatibility
**As a** project maintainer
**I want** existing sessions and provider implementations to continue working
**So that** the upgrade does not require migration or break existing integrations

**Acceptance Criteria:**
- [ ] Existing sessions can be loaded and resumed without migration
- [ ] The `LLMProvider` interface change is backward-compatible (callers that
  ignore usage data still compile and work)
- [ ] The compaction system works identically with both OpenAI-compatible and
  Claude-compatible providers
- [ ] All existing tests pass without modification

## Functional Requirements

### REQ-001: Enhanced LLMProvider Return Type
The `Generate` method must return both the response message and token usage
metadata. The return type shall be a new struct containing the message and
usage fields. Existing callers that only need the message can access it via
a field on the return struct.

**Usage fields:**
- `InputTokens` (int): Tokens in the request prompt
- `OutputTokens` (int): Tokens in the response
- `CacheCreationTokens` (int): Tokens written to cache (0 if unsupported)
- `CacheReadTokens` (int): Tokens read from cache (0 if unsupported)

### REQ-002: Hybrid Token Counting
The token counting system must combine exact API usage with rough estimation:

1. Scan messages from the end to find the last assistant message with known usage
2. Use exact usage count for all messages up to that point
3. Use `ImprovedRoughEstimator` for messages after the last known-usage point
4. If no usage is available, fall back to full rough estimation

Note on parallel tool call handling: The current `schema.Message` struct has no
`ResponseID` field, so Phase 1 uses the simpler approach of tracking the last
assistant message index for the hybrid split point. ResponseID-based parallel
tool call deduplication (where multiple assistant messages share one API
response) is deferred to Phase 2.

### REQ-002a: Improved Rough Estimator (G1)
Replace the current `runeCount + 1` estimator with a more accurate heuristic
following Claude Code's approach:

- Default: `len(content) / 4` (4 bytes per token for natural language)
- JSON content: `len(content) / 2` (tokens are denser in JSON). JSON content is
  detected by trimming leading whitespace and checking if the first non-space
  character is `{` or `[`.
- Apply a `4/3` safety margin multiplier to account for estimation variance:
  `estimate = int(float64(rawEstimate) * 4.0 / 3.0)`

This improves accuracy for the "new messages" window between API calls, directly
affecting compaction trigger precision.

### REQ-003: Model Capability Registry
A registry struct maps model name patterns to context window sizes. The lookup
order is:

1. Config file override (exact model name match)
2. Registry lookup (prefix match, longest prefix wins)
3. Default fallback (128,000 tokens)

The registry must include entries for known models:
- `glm-4`: 128,000
- `glm-4-plus`: 128,000
- `glm-4-air`: 128,000
- `claude-3.5-sonnet`: 200,000
- `claude-3-opus`: 200,000
- `claude-4-sonnet`: 200,000
- `claude-4-opus`: 200,000

### REQ-004: Compaction Threshold Configuration
Replace the current single `SoftRatio` with a multi-threshold system:

| Threshold | Formula | Purpose |
|-----------|---------|---------|
| Effective Window | context_window - 20,000 | Reserve for summary output |
| Auto-Compact | effective_window - 13,000 | Trigger automatic compaction |
| Warning | auto_compact - 20,000 | Display warning to user |
| Blocking | effective_window - 3,000 | Hard limit, refuse to continue |

The effective window accounts for the maximum output tokens needed for the
compaction summary (20,000 tokens based on p99.99 measurements from Claude Code).

### REQ-004a: Compaction Recursive Guard (G2)
The compaction system must never trigger compaction during an active compaction
operation. This prevents infinite recursion where the compaction LLM call itself
causes the context to appear over-threshold, triggering another compaction.

Implementation: maintain a `compacting` boolean flag on the Compactor. Set it to
`true` before starting compaction and `false` after completion. The `MaybeCompact`
method must check this flag first and return immediately if already compacting.

### REQ-004b: Compaction Enable/Disable Toggle (G7)
The compaction system must support enable/disable control via both config and
environment variables:

- `FOXHARNESS_DISABLE_COMPACT`: When set (any value), disable ALL compaction
  (both automatic and manual)
- `FOXHARNESS_DISABLE_AUTO_COMPACT`: When set, disable automatic compaction only
  (manual compaction trigger is reserved for Phase 2)
- Config field `compaction.enabled`: Boolean, default `true`. Set to `false` to
  disable automatic compaction

Note: Manual compaction (user-triggered compaction outside the automatic flow)
is not in Phase 1 scope. The `FOXHARNESS_DISABLE_AUTO_COMPACT` env var and
`compaction.enabled` config field both control the same automatic compaction
behavior. The distinction between "all compaction" and "auto-only" is reserved
for Phase 2 when manual compaction is implemented.

The check order is: env var `DISABLE_COMPACT` → env var `DISABLE_AUTO_COMPACT`
→ config `compaction.enabled`. The most restrictive setting wins.

### REQ-005: Tool Result Persistence
When a tool result is produced:

1. If the result content exceeds 50,000 characters, persist it:
   - Write full content to `{session_dir}/tool-results/{toolCallID}.json` or `.txt`
   - Replace context content with a preview message:
     ```
     <persisted-output>
     Output too large (X KB). Full output saved to: {filepath}
     Preview (first 2KB):
     {first 2048 characters}
     </persisted-output>
     ```
2. If the file already exists (idempotent tool call), skip writing

3. Per-turn budget enforcement:
   - After all tool results for a turn are collected, calculate total characters
   - If total > 200,000, sort new results by size (largest first)
   - Persist the largest new results until total is within budget
   - Never persist results that the model has already seen in a previous turn

### REQ-005a: Absolute Tool Result Size Cap (G8)
Regardless of persistence, tool results must be capped at an absolute maximum
size to prevent excessive disk usage and memory consumption:

- `MAX_TOOL_RESULT_BYTES = 400,000` (400KB, equivalent to ~100K tokens at 4 bytes/token)
- Any tool result exceeding this limit must be truncated to the cap before
  persistence, with a truncation notice appended
- Truncation format: `\n...[truncated at 400KB, original size: X KB]`

This follows Claude Code's `MAX_TOOL_RESULT_BYTES = MAX_TOOL_RESULT_TOKENS * 4`
design to prevent single tool results from consuming excessive resources.

### REQ-006: Structured 9-Section Summary
Replace the current `summarize` prompt with a structured prompt requesting 9
sections in the following format:

```
CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.

<analysis>
[Draft thinking area - will be stripped from final output]
</analysis>

<summary>
1. Primary Request and Intent: [User's complete request and goals]
2. Key Technical Concepts: [Frameworks, APIs, patterns relevant to the task]
3. Files and Code Sections: [File paths, code snippets, reasons for changes]
4. Errors and Fixes: [Error details, root causes, and fixes applied]
5. Problem Solving: [Solved problems and ongoing debugging]
6. All User Messages: [Non-tool-result user messages, summarized]
7. Pending Tasks: [Incomplete items from the user's request]
8. Current Work: [Precise description of what was being done last]
9. Optional Next Step: [Recommended next action with rationale]
</summary>

REMINDER: Do NOT call any tools.
```

The prompt includes three sections following Claude Code's design (G3):
1. **NO_TOOLS_PREAMBLE**: "CRITICAL: Respond with TEXT ONLY. Do NOT call any
   tools." — prevents the LLM from invoking tools during summarization
2. **BASE_COMPACT_PROMPT**: The 9-section summary instruction
3. **NO_TOOLS_TRAILER**: "REMINDER: Do NOT call any tools." — reinforces the
   constraint

The output format uses `<analysis>` as a draft area (stripped from final context)
and `<summary>` as the preserved content.

The compaction LLM call must be made with **no tools available** (empty tool
list) to enforce the no-tool-call constraint at the API level.

### REQ-007: Summary Language Auto-Detection
The summary language is determined by the user's first message:

- If the first user message contains CJK characters (Unicode ranges for Chinese,
  Japanese, Korean), generate the summary in Chinese
- Otherwise, generate the summary in English
- Technical terms (API names, file paths, error messages) remain in their
  original language regardless of summary language

### REQ-008: Protocol Boundary Splitting (Preserved)
The existing `moveSplitToProtocolBoundary` function must be preserved in the
new compaction system. When determining the split point between "old messages to
summarize" and "recent messages to keep", the system must adjust the split
point so that no tool call is separated from its corresponding tool result.

This ensures the compacted context never contains orphaned tool calls or results.

### REQ-009: Compaction Message Format
After compaction, the context history is restructured as:

```
[CompactBoundaryMarker (system message with metadata)]
[SummaryMessage (user message with 9-section summary + continuation instructions)]
[PreservedMessages? (optional, e.g., first user message anchor)]
[RecentMessages (last N messages, protocol-boundary aligned)]
```

The summary message uses the role `user` with a header indicating it is a
compacted context summary, matching the current `SummaryMessage` pattern.

### REQ-009a: Summary Continuation Instructions (G4)
The summary message must be wrapped with continuation instructions following
Claude Code's design, telling the LLM to seamlessly continue without pausing:

```
This session is being continued from a previous conversation that ran out of
context. The summary below covers the earlier portion.

{formatted_summary}

If you need specific details from before compaction, read the full transcript at:
{transcript_path}

Continue the conversation from where it left off without asking the user any
further questions. Resume directly — do not acknowledge the summary.
```

This prevents the LLM from pausing after a compaction event to ask the user
for confirmation or acknowledgment.

### REQ-009b: Compact Boundary Marker (G5)
A system message marker must be inserted at the compaction boundary to record
metadata about the compaction event. The marker contains:

- `trigger`: Compaction trigger type (`"auto"` or `"manual"`)
- `pre_tokens`: Token count before compaction
- `messages_summarized`: Number of messages that were summarized
- `timestamp`: When the compaction occurred

This serves as a delimiter for identifying compaction boundaries in the message
history, enabling future features (partial compaction, tool discovery pass-through).

### REQ-009c: Post-Compaction Cleanup (G6)
After a compaction completes, the system must perform cleanup to invalidate
stale cached state:

- Reset any internal estimation caches that reference the pre-compaction message
  indices
- Clear any cached token counts derived from the old message order
- Log the compaction event with pre/post token counts for observability

This follows Claude Code's `runPostCompactCleanup` pattern to prevent stale
state from causing incorrect behavior after compaction replaces the message
history.

## Non-Functional Requirements

### NFR-001: Token Counting Accuracy
For messages where API usage data is available, the reported count must match
exactly (zero variance). For messages estimated via the improved rough estimator,
no specific accuracy target is required, but the 4/3 margin multiplier ensures
conservative over-estimation rather than under-estimation.

### NFR-002: Performance
- Tool result persistence must not add more than 10ms latency per tool call
  (disk I/O is the bottleneck)
- Token counting (hybrid) must complete in under 1ms for contexts up to 500
  messages
- The full compaction LLM call is bounded by the provider's response time;
  no additional latency constraints beyond the LLM call itself

### NFR-003: Disk Usage
- Persisted tool results are stored in the session directory and are cleaned up
  when the session is deleted
- The persistence system must not write more than 100MB per session for tool
  results (enforced by the per-turn budget)

### NFR-004: Backward Compatibility
- The `LLMProvider` interface change must use a new return struct, not modify
  the existing `*schema.Message` return. Code that ignores usage data must
  compile without changes.
- Existing session files (messages.jsonl, session.json) must be loadable without
  migration
- The `Compactor.MaybeCompact` method signature remains unchanged; internal
  behavior is enhanced

### NFR-005: Testability
- All new components must be designed with injectable dependencies
- Token usage extraction must be testable with mock provider responses
- Tool result persistence must be testable with a mock filesystem
- The model registry must be testable without config files

## Acceptance Criteria (Test Cases)

### TC-001: Provider Returns Token Usage
Given a provider that returns a response with `usage.input_tokens=1000` and
`usage.output_tokens=500`, when `Generate` is called, then the return struct
contains `Usage.InputTokens=1000` and `Usage.OutputTokens=500`.

### TC-002: Hybrid Token Count With Known Usage
Given a message history where the last assistant message has known usage of
5,000 tokens and 3 new messages are added after it, when token count is
calculated, then the result is `5,000 + improvedRoughEstimate(3 new messages)`.

### TC-003: Hybrid Token Count Without Usage
Given a message history with no usage data, when token count is calculated,
then the result equals `roughEstimate(all messages)`.

### TC-004: Model Registry Lookup
Given the registry has `glm-4` mapped to 128,000, when looking up `glm-4`,
then the result is 128,000.

### TC-005: Model Registry Fallback
Given the registry has no entry for `unknown-model-v2`, when looking up
`unknown-model-v2`, then the result is 128,000 (default).

### TC-006: Model Registry Config Override
Given config specifies `glm-4: 200000`, when looking up `glm-4`, then the
result is 200,000 (config overrides registry).

### TC-007: Tool Result Persistence Threshold
Given a tool result of 60,000 characters, when the engine processes it, then
the full content is written to disk and the context contains a 2KB preview.

### TC-008: Tool Result Below Threshold
Given a tool result of 30,000 characters, when the engine processes it, then
no persistence occurs and the full content remains in context.

### TC-009: Per-Turn Budget Enforcement
Given a turn produces tool results totaling 250,000 characters, when budget
enforcement runs, then the largest new results are persisted until total is
at most 200,000 characters.

### TC-010: Protocol Boundary Splitting
Given a message list where the split point falls on a tool result message (with
`ToolCallID != ""`), when `moveSplitToProtocolBoundary` adjusts the split, then
the split is moved earlier to avoid breaking a tool call/result pair.

### TC-011: 9-Section Summary Structure
Given a conversation with file modifications and errors, when compaction runs,
then the summary contains all 9 sections with `<analysis>` stripped and
`<summary>` content preserved.

### TC-012: Summary Language Auto-Detection (Chinese)
Given the first user message contains Chinese characters, when the summary
prompt is generated, then the prompt requests a Chinese-language summary.

### TC-013: Summary Language Auto-Detection (English)
Given the first user message is in English, when the summary prompt is
generated, then the prompt requests an English-language summary.

### TC-014: Backward Compatible Provider Call
Given existing code that calls `Generate` and only uses the message field, when
the new return type is used, then the code compiles and runs without changes.

### TC-015: Existing Sessions Load Without Migration
Given an existing session with messages.jsonl from before the upgrade, when the
session is loaded, then all messages are accessible and no migration is needed.

### TC-016: Compaction Threshold Calculation
Given a model with context window 128,000, when thresholds are calculated, then:
- Effective window = 128,000 - 20,000 = 108,000
- Auto-compact = 108,000 - 13,000 = 95,000
- Warning = 95,000 - 20,000 = 75,000
- Blocking = 108,000 - 3,000 = 105,000

### TC-017: Improved Rough Estimator (G1)
Given a plain text string of 1,000 bytes, when `ImprovedRoughEstimator.EstimateText`
is called, then the result is `int(1000/4 * 4.0/3.0) = 333`. For JSON content
of 1,000 bytes, the result is `int(1000/2 * 4.0/3.0) = 666`.

### TC-018: Recursive Guard Blocks Nested Compaction (G2)
Given a Compactor that is currently in the middle of a compaction operation
(`compacting == true`), when `MaybeCompact` is called, then it returns the
original messages immediately without attempting another compaction.

### TC-019: No-Tools Constraint During Summary (G3)
Given the compaction summary LLM call, when the call is made, then the tool
list passed to `Generate` is empty (no tools available) and the prompt includes
both the "CRITICAL: Respond with TEXT ONLY" preamble and "REMINDER: Do NOT call
any tools" trailer.

### TC-020: Summary Continuation Instructions (G4)
Given a compaction that produces a summary, when the summary message is built,
then it contains the continuation wrapper including "Continue the conversation
from where it left off without asking the user any further questions" and a
reference to the transcript path.

### TC-021: Compact Boundary Marker (G5)
Given an automatic compaction with 15,000 pre-compaction tokens and 8 messages
summarized, when the result is built, then the boundary marker contains
`trigger="auto"`, `pre_tokens=15000`, and `messages_summarized=8`.

### TC-022: Post-Compaction Cleanup (G6)
Given a completed compaction, when the cleanup runs, then any cached token
counts from before compaction are invalidated and subsequent estimations use
the new message order.

### TC-023: Compaction Disabled via Environment (G7)
Given the environment variable `FOXHARNESS_DISABLE_COMPACT` is set, when
`MaybeCompact` is called, then it returns the original messages immediately.

### TC-024: Auto-Compact Disabled via Config (G7)
Given the config `compaction.enabled` is `false`, when `MaybeCompact` is
called, then automatic compaction is skipped.

### TC-025: Absolute Tool Result Size Cap (G8)
Given a tool result of 500,000 bytes (exceeding the 400KB cap), when the engine
processes it, then the result is truncated to 400,000 bytes with a truncation
notice appended before persistence.

## Edge Cases

### EC-001: Provider Returns No Usage Data
Some providers or API errors may return responses without usage information.
The hybrid estimator must gracefully fall back to full rough estimation.

### EC-002: Empty Tool Result
A tool result with empty output (0 characters) must not trigger persistence
or budget calculations.

### EC-003: Tool Result Exactly At Threshold
A tool result of exactly 50,000 characters is at the boundary. The persistence
threshold is `> 50,000` (strictly greater), so exactly 50,000 is NOT persisted.

### EC-004: Concurrent Tool Calls With Large Results
When multiple parallel-safe tools return large results simultaneously, the budget
enforcement must process all results together, not individually. All results are
collected first, then budget is enforced on the total.

### EC-005: Compaction With No Old Messages
If all messages are "recent" (within the keep window), compaction must not be
triggered even if the token count exceeds the threshold.

### EC-006: Summary LLM Call Failure
If the LLM call to generate the summary fails, the compaction must return the
original messages unchanged (not partially compacted).

### EC-007: Model Name Case Sensitivity
Model name lookups in the registry must be case-insensitive to handle
variations in provider responses (e.g., `GLM-4` vs `glm-4`).

### EC-008: Persisted File Already Exists
If a tool call is retried with the same ID and the persisted file already
exists, the system must skip writing (idempotent behavior).

### EC-009: Very Short Context Window
For models with very small context windows (< 50,000 tokens), the threshold
calculations must still produce valid positive values. The system should log a
warning if the effective window is below 40,000 tokens.

### EC-010: Mixed Language Conversation
If the user alternates between Chinese and English across turns, the summary
language is determined by the FIRST user message only. Subsequent language
changes do not affect the summary language.

### EC-011: Recursive Compaction Attempt (G2)
If the compaction LLM call produces a response that appears to push the context
over the threshold (e.g., a verbose summary), the recursive guard must prevent
a second compaction attempt within the same cycle.

### EC-012: Tool Calls During Summary (G3)
If the LLM model ignores the no-tools instruction and returns a tool call in
the summary response, the compaction system must extract only the text content
and discard any tool calls. The compaction must not fail due to an unexpected
tool call.

### EC-013: Tool Result Exceeding Both Persistence and Absolute Cap (G8)
Given a tool result of 600,000 bytes, when processed, the result is first
truncated to 400,000 bytes (absolute cap), then checked against the 50,000
character persistence threshold. Since 400,000 bytes > 50,000 characters,
it is also persisted with a 2KB preview.

### EC-014: Disable Env Var and Config Conflict (G7)
If `FOXHARNESS_DISABLE_COMPACT` (all compaction) is set but config
`compaction.enabled` is `true`, the env var takes precedence — all compaction
is disabled.

### EC-015: Post-Compact Estimation Consistency (G6)
After compaction replaces messages, a subsequent token count estimation must
reflect only the compacted messages, not include stale data from the pre-
compaction message set.

## Output Examples

### Tool Result Preview
```
<persisted-output>
Output too large (234 KB). Full output saved to:
  /home/user/.foxharness/projects/abc/sessions/20260522/session/tool-results/call_abc123.json
Preview (first 2KB):
  {"status":"ok","files":["src/main.go","src/util.go","src/handler.go",...]
</persisted-output>
```

### Structured Summary (Chinese)
```
## Compacted Context Summary

以下是较早会话历史的压缩摘要。原始消息仍保存在 session 的 messages.jsonl 中。

1. 用户主要请求：重构 internal/engine 包，将 Loop 拆分为 Loop 和 AgentEngine 两个结构体
2. 关键技术概念：Go interface 嵌入、依赖注入、Provider 协议适配
3. 文件和代码：internal/engine/loop.go（已拆分为 loop.go + engine.go），
   internal/provider/interface.go（新增 LLMProvider 方法）
4. 错误和修复：首次尝试使用 struct embedding 导致方法冲突，改用显式委托模式
5. 问题解决：OpenAI 和 Claude provider 的消息格式差异已通过 Normalizer 解决
6. 用户消息：[1] "重构 engine 包" [2] "确保所有测试通过"
7. 待办：为新的 AgentEngine 编写集成测试
8. 当前工作：正在运行 go test ./internal/engine/... 验证最终状态
9. 建议下一步：运行完整测试套件 go test ./... 确认无回归
```

### Structured Summary (English)
```
## Compacted Context Summary

This session is continued from a previous conversation. Summary of earlier context:

1. Primary Request: Implement a multi-level context compression system replacing
   the current single-level compaction
2. Key Technical Concepts: Token counting, prompt caching, structured summaries,
   tool result persistence
3. Files and Code: internal/compaction/compactor.go (restructured),
   internal/provider/interface.go (enhanced return type)
4. Errors and Fixes: RoughEstimator off by ~30% vs actual usage — resolved by
   hybrid counting approach
5. Problem Solving: Backward compatibility achieved via new return struct without
   breaking existing callers
6. All User Messages: [1] "Implement context compression" [2] "Keep it backward compatible"
7. Pending Tasks: Implement micro-compact layer (Phase 2)
8. Current Work: Writing test cases for tool result persistence
9. Optional Next Step: Complete TC-007 through TC-009 test cases
```

## Out of Scope

The following items are explicitly excluded from Phase 1:

- **Micro-compact**: Time-based tool result decay and cache editing (Phase 2)
- **Post-compact recovery**: File attachments (5 files/50K tokens), plan
  preservation, skill preservation, delta attachments (Phase 2)
- **Circuit breaker**: Auto-stop after consecutive compaction failures (Phase 2)
- **Prompt-too-long retry**: Head truncation when compaction request exceeds
  context (Phase 2)
- **Partial compaction**: Compacting a subset of messages from/to a specific
  point (Phase 2)
- **Forked agent**: Reusing the main thread's prompt cache for compaction calls
  (Phase 2)
- **Session Memory compaction**: Using memory.md as an alternative to LLM
  summaries (Phase 2)
- **Streaming compaction UI**: Real-time display of compaction progress in the
  TUI (Phase 2)
- **Cross-session compaction state**: Tracking compaction progress across
  multiple sessions (not planned)
- **tiktoken or BPE tokenization**: Exact tokenizer-based counting (rough
  estimation is sufficient for new messages)
- **Sub-agent safety**: Resetting module-level state only for the main thread
  after compaction (not applicable — foxharness does not have a sub-agent
  architecture in Phase 1)
- **Image/document stripping**: Replacing images with `[image]` markers before
  summarization (not applicable — current message schema does not include
  image attachments)
- **Pre/post compact hooks**: Hook system for compaction events (Phase 2)
