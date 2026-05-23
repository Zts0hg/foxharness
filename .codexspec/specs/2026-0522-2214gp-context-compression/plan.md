# Implementation Plan: Multi-Level Context Compression

## 1. Tech Stack

| Category | Technology | Version | Notes |
|----------|------------|---------|-------|
| Language | Go | 1.25.0 | Per go.mod |
| LLM SDK (OpenAI) | openai-go | v3.33.0 | OpenAI-compatible providers (Zhipu) |
| LLM SDK (Anthropic) | anthropic-sdk-go | v1.43.0 | Anthropic-compatible providers (Zhipu) |
| Config | gopkg.in/yaml.v3 | v3.0.1 | For model registry config overrides |
| Testing | Go testing + testable mocks | stdlib | Injectable dependencies per NFR-005 |

No new external dependencies are required. The implementation uses only existing
libraries and the Go standard library.

## 2. Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| 1. TDD | ✅ | Every phase lists test files first; Red-Green-Refactor cycle enforced per phase |
| 2. Code Quality | ✅ | Interfaces before implementations; injectable dependencies throughout; small focused interfaces |
| 3. Go Documentation | ✅ | Block comments on all exported identifiers; no teaching comments; godoc-compatible |
| 4. Testing Standards | ✅ | Table-driven tests for multi-scenario logic (estimator, registry, thresholds); edge case coverage from spec |
| 5. Architecture | ✅ | New packages have single responsibilities; public interfaces are small and focused |
| 6. Performance | ✅ | NFR-002 addressed: hybrid counting <1ms; persistence <10ms; benchmarks included |
| 7. Security | ✅ | Tool results stored in session-scoped paths; no secrets in scope |

### Constitution-Driven Design Decisions

1. **TDD cycle per component**: Each module's test file is listed before its
   implementation file in every phase. Tests are written first and must fail
   before implementation begins.

2. **Interface-first design**: New abstractions (`TokenEstimator`, `FileSystem`,
   `ModelRegistry`) are defined as interfaces before any concrete implementation,
   following Principle 5 (Architecture) and enabling testability (Principle 2).

3. **Injectable dependencies**: Tool result persistence uses a `FileSystem`
   interface. The model registry accepts config overrides through a
   `ConfigProvider` interface. These enable unit testing without file I/O or
   config files.

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         AgentEngine (engine)                        │
│                                                                     │
│  ┌──────────┐   ┌──────────────┐   ┌────────────────────────────┐  │
│  │ callModel │──▶│ Store Usage  │──▶│ MaybeCompact (per turn)    │  │
│  │          │   │ on Response   │   │                            │  │
│  └──────────┘   └──────────────┘   └───────────┬────────────────┘  │
│         │                                       │                   │
│         │              ┌────────────────────────┘                   │
│         ▼              ▼                                            │
│  ┌──────────────┐  ┌──────────────────────┐                        │
│  │ Tool Result  │  │ Hybrid Token Counter │                        │
│  │ Persistence  │  │ (exact + estimation) │                        │
│  └──────┬───────┘  └──────────┬───────────┘                        │
│         │                     │                                     │
└─────────┼─────────────────────┼─────────────────────────────────────┘
          │                     │
          ▼                     ▼
┌─────────────────┐  ┌─────────────────────────────────────────────┐
│ toolresult      │  │ compaction                                   │
│                 │  │                                               │
│ ┌─────────────┐│  │ ┌────────────────┐  ┌──────────────────────┐ │
│ │ Persist()   ││  │ │ ImprovedRough  │  │ ModelRegistry        │ │
│ │ EnforceBgt()││  │ │ Estimator      │  │ (lookup + config)    │ │
│ │ Truncate()  ││  │ └────────────────┘  └──────────────────────┘ │
│ └─────────────┘│  │                                               │
│                 │  │ ┌────────────────┐  ┌──────────────────────┐ │
│ FileSystem iface│  │ │ Thresholds     │  │ Compactor            │ │
│ (injectable)   │  │ │ (4-level)      │  │ (MaybeCompact)       │ │
│                 │  │ └────────────────┘  │ - recursive guard    │ │
└─────────────────┘  │                     │ - enable/disable     │ │
                     │ ┌────────────────┐  │ - structured summary │ │
                     │ │ BoundaryMarker │  │ - boundary marker    │ │
                     │ │ (system msg)   │  │ - cleanup            │ │
                     │ └────────────────┘  └──────────────────────┘ │
                     │                                               │
                     └───────────────────────────────────────────────┘

Data Flow:
  Provider API ──▶ GenerateResponse (Message + Usage)
                      │
                      ▼
              Engine stores Usage on Message
                      │
            ┌─────────┴─────────┐
            ▼                   ▼
   HybridTokenCount      Tool Result
   (scan for last         Persistence
    usage, estimate       (>50K → disk)
    remainder)
            │                   │
            └─────────┬─────────┘
                      ▼
              Compactor.MaybeCompact()
              - check thresholds
              - recursive guard
              - enable/disable
              - summarize with 9-section prompt
              - build boundary marker
              - post-compact cleanup
```

## 4. Component Structure

```
internal/
├── schema/
│   ├── message.go          # MODIFY: add Usage struct + field on Message
│   └── message_test.go     # MODIFY: add Usage serialization tests
│
├── provider/
│   ├── interface.go        # MODIFY: new GenerateResponse return type
│   ├── openai.go           # MODIFY: extract usage from OpenAI response
│   ├── claude.go           # MODIFY: extract usage from Anthropic response
│   ├── openai_test.go      # MODIFY: usage extraction tests
│   └── claude_test.go      # MODIFY: usage extraction tests
│
├── compaction/
│   ├── compactor.go        # MAJOR REWRITE: new Compactor with all features
│   ├── compactor_test.go   # REWRITE: comprehensive test coverage
│   ├── estimator.go        # NEW: ImprovedRoughEstimator + HybridEstimator
│   ├── estimator_test.go   # NEW: estimator tests (TC-002, TC-003, TC-017)
│   ├── registry.go         # NEW: ModelRegistry with prefix matching
│   ├── registry_test.go    # NEW: registry tests (TC-004, TC-005, TC-006)
│   ├── thresholds.go       # NEW: multi-threshold system
│   ├── thresholds_test.go  # NEW: threshold tests (TC-016)
│   ├── prompt.go           # NEW: structured summary prompt builder
│   ├── prompt_test.go      # NEW: prompt tests (TC-011, TC-012, TC-013)
│   ├── boundary.go         # NEW: CompactBoundaryMarker
│   └── boundary_test.go    # NEW: boundary marker tests (TC-021)
│
├── toolresult/
│   ├── persist.go          # NEW: persistence + budget enforcement
│   ├── persist_test.go     # NEW: persistence tests (TC-007-TC-009, TC-025)
│   ├── truncate.go         # NEW: absolute size cap
│   └── truncate_test.go    # NEW: truncation tests
│
├── engine/
│   ├── loop.go             # MODIFY: integrate usage tracking + persistence
│   ├── loop_test.go        # MODIFY: integration tests
│   └── context.go          # MODIFY: updated context building
│
└── session/
    └── session.go          # MODIFY: add ToolResultsDir() method
```

## 5. Module Dependency Graph

```
┌───────────┐
│  schema   │  (shared types: Message, Usage, ToolCall, etc.)
└─────┬─────┘
      │
      ├──────────────────────────────────────┐
      ▼                                      ▼
┌───────────┐                         ┌────────────┐
│ provider  │─────────────────────────│ toolresult  │
│ (Generate │                         │ (Persist,   │
│  Response)│                         │  Enforce,   │
└─────┬─────┘                         │  Truncate)  │
      │                               └──────┬─────┘
      │                                      │
      ▼                                      ▼
┌───────────┐                         ┌────────────┐
│ compaction│◀───uses schema.Usage────│   engine    │
│ (Estimate,│                         │ (orchestr.) │
│  Registry,│                         │             │
│  Compact) │                         └──────┬─────┘
└───────────┘                                │
                                             ▼
                                      ┌────────────┐
                                      │  session    │
                                      │ (persist.)  │
                                      └────────────┘
```

Dependency rules:
- `schema` has no internal dependencies (leaf package)
- `provider` depends only on `schema`
- `toolresult` depends only on `schema` (via FileSystem interface for testability)
- `compaction` depends on `schema` and `provider` (for LLM calls)
- `engine` depends on `schema`, `provider`, `compaction`, `toolresult`, `session`
- `session` depends only on `schema`

## 6. Module Specifications

### Module: schema (Modified)

- **Responsibility**: Shared data types for the entire framework
- **Dependencies**: None
- **Interface**: `Usage` struct, extended `Message` struct
- **Files**: `message.go` (modified), `message_test.go` (modified)

#### New Type: `Usage`

```go
// Usage reports token consumption from an LLM API response.
type Usage struct {
    InputTokens        int `json:"input_tokens"`
    OutputTokens       int `json:"output_tokens"`
    CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
    CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
}
```

#### Modified Type: `Message`

Add an optional `Usage` field to `Message`:

```go
type Message struct {
    Role       Role       `json:"role"`
    Content    string     `json:"content"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
    Usage      *Usage     `json:"usage,omitempty"`
}
```

This field is populated for assistant messages when the provider returns usage
data. It enables hybrid token counting without a separate tracking mechanism.
Existing messages without Usage load fine (nil pointer, omitted in JSON).

### Module: provider (Modified)

- **Responsibility**: LLM provider abstraction with usage reporting
- **Dependencies**: `schema`
- **Interface**: Updated `LLMProvider.Generate` return type
- **Files**: `interface.go` (modified), `openai.go` (modified), `claude.go` (modified)

#### New Return Type: `GenerateResponse`

```go
// GenerateResponse wraps an LLM response message with token usage metadata.
type GenerateResponse struct {
    Message *schema.Message
    Usage   schema.Usage
}
```

#### Updated Interface

```go
type LLMProvider interface {
    Generate(ctx context.Context, message []schema.Message,
        availableTools []schema.ToolDefinition) (*GenerateResponse, error)
}
```

#### Provider Changes

**OpenAI Provider** (`openai.go`):
- Extract `resp.Usage.PromptTokens`, `resp.Usage.CompletionTokens` from
  `openai.ChatCompletion` response
- Map to `schema.Usage{InputTokens, OutputTokens}`

**Claude Provider** (`claude.go`):
- Extract `resp.Usage.InputTokens`, `resp.Usage.OutputTokens` from
  `anthropic.Message` response
- Map to `schema.Usage{InputTokens, OutputTokens}`

### Module: compaction (Major Restructure)

- **Responsibility**: Context compression including estimation, thresholds,
  model registry, structured summarization, boundary markers
- **Dependencies**: `schema`, `provider`
- **Interface**: `Compactor.MaybeCompact`, `TokenEstimator`, `ModelRegistry`
- **Files**: Multiple new files (see structure above)

#### New: `estimator.go` (REQ-002, REQ-002a)

`ImprovedRoughEstimator` implements the `TokenEstimator` interface:

```go
// ImprovedRoughEstimator provides a byte-based token approximation with
// separate heuristics for natural language and JSON content.
type ImprovedRoughEstimator struct{}

func (ImprovedRoughEstimator) EstimateText(text string) int
func (ImprovedRoughEstimator) EstimateMessages(messages []schema.Message) int
```

Logic:
- Plain text: `len(text) / 4 * 4/3`
- JSON content: `len(text) / 2 * 4/3`
- JSON detection: trim leading whitespace, check first non-space char is `{` or `[`

`HybridEstimator` combines exact API usage with rough estimation:

```go
// HybridEstimator combines exact API usage data with rough estimation
// for messages after the last known usage point.
type HybridEstimator struct {
    rough TokenEstimator
}

func (h *HybridEstimator) Estimate(messages []schema.Message) int
```

Logic:
1. Scan messages from end to find last assistant message with non-nil `Usage`
2. Sum `Usage.InputTokens + Usage.CacheCreationTokens + Usage.CacheReadTokens + Usage.OutputTokens` for exact count
3. Use `ImprovedRoughEstimator` for messages after that point
4. If no usage found, fall back to full rough estimation

#### New: `registry.go` (REQ-003)

```go
// ModelRegistry maps model name patterns to context window sizes.
type ModelRegistry struct {
    entries map[string]int
    config  map[string]int
}

func NewModelRegistry() *ModelRegistry
func (r *ModelRegistry) Lookup(modelName string) int
func (r *ModelRegistry) SetConfigOverride(overrides map[string]int)
```

Lookup order: config override (exact match) → registry (longest prefix match) → default (128000).

Default entries include: `glm-4` (128K), `glm-4-plus` (128K), `glm-4-air` (128K),
`claude-3.5-sonnet` (200K), `claude-3-opus` (200K), `claude-4-sonnet` (200K),
`claude-4-opus` (200K).

Case-insensitive lookup (EC-007).

#### New: `thresholds.go` (REQ-004, REQ-004a, REQ-004b)

```go
// ThresholdConfig defines the multi-level compaction thresholds.
type ThresholdConfig struct {
    ContextWindow       int
    ReservedForSummary  int  // default 20000
    AutoCompactBuffer   int  // default 13000
    WarningBuffer       int  // default 20000
    BlockingBuffer      int  // default 3000
}

func (c ThresholdConfig) EffectiveWindow() int
func (c ThresholdConfig) AutoCompact() int
func (c ThresholdConfig) Warning() int
func (c ThresholdConfig) Blocking() int
```

**Enable/disable config loading** (REQ-004b): The compaction enabled/disabled
state is determined at `Compactor` construction time from three sources, checked
in order:

1. `FOXHARNESS_DISABLE_COMPACT` env var (any non-empty value → disable all)
2. `FOXHARNESS_DISABLE_AUTO_COMPACT` env var (any non-empty value → disable auto)
3. `compaction.enabled` field from the project's config file (YAML, same file as
   model registry overrides)

The `NewCompactor` function accepts a `CompactionConfig` struct containing the
`Enabled bool` field. The engine reads this config when constructing the Compactor
and passes it in. Env vars are checked inside `NewCompactor` via `os.LookupEnv`,
so they override the config file. This matches the spec's "most restrictive wins"
rule and keeps config I/O out of the compaction package itself (testable without
files).

#### New: `prompt.go` (REQ-006, REQ-007)

```go
// BuildCompactPrompt constructs the structured 9-section summary prompt.
func BuildCompactPrompt(messages []schema.Message, language string) string

// DetectSummaryLanguage determines the summary language from the first user message.
func DetectSummaryLanguage(messages []schema.Message) string

// FormatSummary strips the <analysis> draft area and extracts <summary> content.
func FormatSummary(raw string) string
```

#### New: `boundary.go` (REQ-009b)

```go
// CompactBoundary represents a compaction boundary marker in the message history.
type CompactBoundary struct {
    Trigger           string `json:"trigger"`
    PreTokens         int    `json:"pre_tokens"`
    MessagesSummarized int   `json:"messages_summarized"`
    Timestamp         string `json:"timestamp"`
}

// BoundaryMessage creates a system message containing the boundary marker.
func BoundaryMessage(boundary CompactBoundary) schema.Message
```

#### Restructured: `compactor.go`

The `Compactor` struct is restructured to integrate all new components:

```go
type Compactor struct {
    provider    provider.LLMProvider
    estimator   TokenEstimator
    hybrid      *HybridEstimator
    registry    *ModelRegistry
    thresholds  ThresholdConfig
    compacting  bool  // recursive guard (REQ-004a)

    // Configuration
    recentKeep       int
    summaryMaxTokens int
    sessionDir       string
    transcriptPath   string

    // Enable/disable (REQ-004b)
    disabled     bool
    autoDisabled bool
}
```

The `MaybeCompact` method flow:

1. Check recursive guard → return if compacting
2. Check enable/disable (env vars + config) → return if disabled
3. Run `HybridEstimator.Estimate` on messages
4. Compare against `Thresholds.AutoCompact()`
5. If over threshold:
   a. Set `compacting = true` (defer reset)
   b. Determine split point with `moveSplitToProtocolBoundary`
   c. Build structured summary prompt with `BuildCompactPrompt`
   d. Call provider with empty tools list
   e. Format summary with `FormatSummary`
   f. Build boundary marker with `BoundaryMessage`
   g. Build summary message with continuation instructions
   h. Assemble compacted context: [boundary] [summary] [anchors] [recent]
   i. Run post-compact cleanup
6. Return compacted messages

The `SummaryMessage` function is updated to use the new continuation wrapper
format (REQ-009a) instead of the current simple Chinese header. The existing
`renderMessagesForSummary` function is replaced by `BuildCompactPrompt` (in
`prompt.go`) and should be removed during Phase 6 implementation.

### Module: toolresult (New)

- **Responsibility**: Persist large tool results to disk with inline previews
- **Dependencies**: `schema` (via `FileSystem` interface for testability)
- **Interface**: `Persist`, `EnforceBudget`, `TruncateToCap`
- **Files**: `persist.go`, `truncate.go`

#### FileSystem Interface (for testability)

```go
// FileSystem abstracts file operations for testability.
type FileSystem interface {
    WriteFile(path string, data []byte, perm os.FileMode) error
    Stat(path string) (os.FileInfo, error)
    MkdirAll(path string, perm os.FileMode) error
}
```

Production implementation uses `osFS{}`. Tests use `memFS{}` (in-memory map).

#### `persist.go` (REQ-005)

```go
const (
    PersistenceThreshold = 50_000   // characters
    PerTurnBudget        = 200_000  // characters
    PreviewSize          = 2_048    // bytes
)

// PersistedResult holds the result of a persistence operation.
type PersistedResult struct {
    Original  schema.ToolResult
    Preview   string    // Content to keep in context
    FilePath  string    // Disk path of persisted content
    Persisted bool
}

// PersistIfNeeded checks a single tool result against the persistence threshold.
func PersistIfNeeded(fs FileSystem, dir string, result schema.ToolResult) PersistedResult

// EnforceBudget enforces the per-turn budget on a set of tool results.
func EnforceBudget(fs FileSystem, dir string, results []PersistedResult, seenIDs map[string]bool) []PersistedResult
```

#### `truncate.go` (REQ-005a)

```go
const MaxToolResultBytes = 400_000

// TruncateToCap truncates content to the absolute maximum size.
func TruncateToCap(content string) string
```

### Module: engine (Modified)

- **Responsibility**: Orchestrate agent loop with new compression features
- **Dependencies**: `schema`, `provider`, `compaction`, `toolresult`, `session`
- **Files**: `loop.go` (modified), `context.go` (modified)

#### `loop.go` Changes

1. **`callModel`**: Updated to use `*GenerateResponse` return type. Extracts
   `response.Message` and `response.Usage`. Sets `resp.Message.Usage = &resp.Usage`
   before returning.

2. **Tool result processing**: After `executeToolCalls`, apply tool result
   persistence:
   ```go
   // After collecting tool results
   for i, item := range toolResults {
       processed := toolresult.TruncateToCap(item.Result.Output)
       item.Result.Output = processed
   }
   // Build set of tool call IDs already present in context before this turn.
   // Scan contextHistory for messages with non-empty ToolCallID; these represent
   // tool results the model has already seen. New results from the current turn's
   // tool calls (identified by actionResponse.ToolCalls[].ID) are excluded from
   // this set, ensuring EnforceBudget only persists new results.
   seenIDs := make(map[string]bool)
   for _, msg := range contextHistory {
       if msg.ToolCallID != "" {
           seenIDs[msg.ToolCallID] = true
       }
   }
   persisted := toolresult.EnforceBudget(fs, toolResultsDir, results, seenIDs)
   ```

3. **Compactor initialization**: `NewCompactor` now requires model name for
   registry lookup. Update `WithCompactor` or `NewAgentEngine` to pass model info.

#### `context.go` Changes

Updated to use the new `Compactor` API. The `buildInitialContext` flow remains
conceptually the same but uses `HybridEstimator` instead of `RoughEstimator`.

### Module: session (Minor Modification)

- **Responsibility**: Session lifecycle management
- **Dependencies**: `schema`
- **Files**: `session.go` (modified)

Add `ToolResultsDir()` method:

```go
func (s *Session) ToolResultsDir() string {
    return filepath.Join(s.RootDir, "tool-results")
}
```

Create this directory in `Manager.Create` alongside existing directories.

## 7. Data Models

### Usage

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| InputTokens | int | Tokens in the request prompt | ≥ 0 |
| OutputTokens | int | Tokens in the response | ≥ 0 |
| CacheCreationTokens | int | Tokens written to cache | ≥ 0, 0 if unsupported |
| CacheReadTokens | int | Tokens read from cache | ≥ 0, 0 if unsupported |

### GenerateResponse

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| Message | *schema.Message | LLM response message | Non-nil on success |
| Usage | schema.Usage | Token consumption data | Zero-value if unavailable |

### CompactBoundary

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| Trigger | string | Compaction trigger type | "auto" or "manual" |
| PreTokens | int | Token count before compaction | > 0 |
| MessagesSummarized | int | Number of summarized messages | > 0 |
| Timestamp | string | ISO 8601 timestamp | Non-empty |

### PersistedResult

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| Original | ToolResult | Original tool result | Non-nil |
| Preview | string | Content to keep in context | ≤ 2KB when persisted |
| FilePath | string | Disk path of full content | Empty if not persisted |
| Persisted | bool | Whether content was written | — |

### ModelRegistryEntry (internal)

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| Prefix | string | Model name prefix for matching | Non-empty, lowercase |
| ContextWindow | int | Maximum context tokens | > 0 |

## 8. Implementation Phases

### Phase 1: Schema & Provider Foundation (REQ-001)

Foundation layer: add usage data types and update the provider interface.

**TDD Cycle 1.1: Usage struct**
- [ ] RED: Write `TestUsageJSONRoundTrip` in `internal/schema/message_test.go`
  - Verify Usage serializes/deserializes correctly with omitempty fields
- [ ] GREEN: Add `Usage` struct to `internal/schema/message.go`
- [ ] REFACTOR: Clean up if needed

**TDD Cycle 1.2: Message with optional Usage**
- [ ] RED: Write `TestMessageWithUsage` and `TestMessageWithoutUsage`
  - Verify existing messages load without Usage (nil pointer)
  - Verify messages with Usage round-trip through JSON
- [ ] GREEN: Add `Usage *Usage` field to `Message` struct
- [ ] REFACTOR: Verify existing tests still pass: `go test ./internal/schema/...`

**TDD Cycle 1.3: GenerateResponse return type**
- [ ] RED: Write `TestGenerateResponseAccess` in `internal/provider/interface_test.go`
  - Verify GenerateResponse has Message and Usage fields
- [ ] GREEN: Add `GenerateResponse` struct to `internal/provider/interface.go`
  Update `LLMProvider.Generate` signature
- [ ] REFACTOR: N/A (new type)

**TDD Cycle 1.4: OpenAI provider usage extraction**
- [ ] RED: Write `TestOpenAIProviderReturnsUsage` (TC-001)
  - Mock OpenAI response with usage fields
  - Verify Usage fields are populated
- [ ] GREEN: Update `OpenAIProvider.Generate` to return `*GenerateResponse`
  Extract `resp.Usage.PromptTokens` and `resp.Usage.CompletionTokens`
- [ ] REFACTOR: Clean up message construction

**TDD Cycle 1.5: Claude provider usage extraction**
- [ ] RED: Write `TestClaudeProviderReturnsUsage` (TC-001)
  - Mock Anthropic response with usage fields
  - Verify Usage fields are populated
- [ ] GREEN: Update `ClaudeProvider.Generate` to return `*GenerateResponse`
  Extract `resp.Usage.InputTokens` and `resp.Usage.OutputTokens`
- [ ] REFACTOR: Clean up message construction

**TDD Cycle 1.6: Engine callModel update**
- [ ] RED: Write `TestCallModelUsesGenerateResponse` in `internal/engine/loop_test.go`
  - Verify engine accesses `resp.Message` and sets `resp.Message.Usage`
- [ ] GREEN: Update `callModel` to handle `*GenerateResponse`
  Set `response.Message.Usage = &response.Usage` before appending to history
- [ ] REFACTOR: Verify all existing engine tests pass: `go test ./internal/engine/...`

**Phase 1 Deliverables**:
- `schema.Usage` struct defined and tested
- `schema.Message` extended with optional Usage field
- `provider.GenerateResponse` struct defined
- Both providers extract and return usage data
- Engine integrates usage data into message history
- All existing tests pass

### Phase 2: Token Estimation (REQ-002, REQ-002a)

Improved estimation and hybrid counting.

**TDD Cycle 2.1: ImprovedRoughEstimator**
- [ ] RED: Write `TestImprovedRoughEstimator_PlainText` and `TestImprovedRoughEstimator_JSON`
  (TC-017) in `internal/compaction/estimator_test.go`
  - Plain text: 1000 bytes → `int(1000/4 * 4.0/3.0) = 333`
  - JSON: 1000 bytes → `int(1000/2 * 4.0/3.0) = 666`
  - Edge: empty string → 0
  - Edge: mixed content (text with JSON embedded)
- [ ] GREEN: Implement `ImprovedRoughEstimator` in `internal/compaction/estimator.go`
  JSON detection: trim whitespace, check first char `{` or `[`
- [ ] REFACTOR: Extract JSON detection helper if complex

**TDD Cycle 2.2: HybridEstimator**
- [ ] RED: Write `TestHybridEstimator_WithUsage` (TC-002) and
  `TestHybridEstimator_WithoutUsage` (TC-003)
  - With usage: last assistant has Usage, messages after estimated
  - Without usage: falls back to full rough estimation
  - Edge (EC-001): provider returns zero-value Usage
- [ ] GREEN: Implement `HybridEstimator` in `internal/compaction/estimator.go`
  Scan from end, find last non-nil Usage, split exact/estimated
- [ ] REFACTOR: Verify the `TokenEstimator` interface in compaction package
  is compatible

**TDD Cycle 2.3: Replace RoughEstimator references**
- [ ] RED: Write tests that use `ImprovedRoughEstimator` instead of `RoughEstimator`
  in existing compaction tests
- [ ] GREEN: Update `Compactor` to use `ImprovedRoughEstimator` as default
- [ ] REFACTOR: Remove old `RoughEstimator` from compaction package if unused

**Phase 2 Deliverables**:
- `ImprovedRoughEstimator` with JSON-aware estimation
- `HybridEstimator` combining exact usage + rough estimation
- Compaction package uses improved estimation

### Phase 3: Model Registry (REQ-003)

Model name → context window mapping.

**TDD Cycle 3.1: Registry lookup**
- [ ] RED: Write table-driven `TestModelRegistry_Lookup` (TC-004, TC-005)
  in `internal/compaction/registry_test.go`
  - Known model → correct window
  - Unknown model → 128000 default
  - Prefix match: `glm-4-air-x` matches `glm-4-air` prefix → 128000
  - Case insensitive (EC-007): `GLM-4` → 128000
- [ ] GREEN: Implement `ModelRegistry` in `internal/compaction/registry.go`
  with default entries and prefix matching
- [ ] REFACTOR: Extract prefix matching logic

**TDD Cycle 3.2: Config override**
- [ ] RED: Write `TestModelRegistry_ConfigOverride` (TC-006)
  - Config overrides exact model match
  - Config adds new model not in defaults
  - Config override takes precedence over prefix match
- [ ] GREEN: Implement `SetConfigOverride` and update lookup priority
- [ ] REFACTOR: Clean up

**Phase 3 Deliverables**:
- `ModelRegistry` with 7 default entries
- Prefix matching with longest-prefix-wins
- Config override support
- Case-insensitive lookups

### Phase 4: Compaction Thresholds & Guards (REQ-004, REQ-004a, REQ-004b)

Multi-threshold system, recursive guard, enable/disable.

**TDD Cycle 4.1: Multi-threshold calculation**
- [ ] RED: Write `TestThresholdConfig_128K` (TC-016) and
  `TestThresholdConfig_ShortWindow` (EC-009)
  in `internal/compaction/thresholds_test.go`
  - 128K model: effective=108K, auto=95K, warning=75K, blocking=105K
  - Short window (<50K): all values positive, warn if effective < 40K
- [ ] GREEN: Implement `ThresholdConfig` in `internal/compaction/thresholds.go`
- [ ] REFACTOR: Clean up

**TDD Cycle 4.2: Recursive guard**
- [ ] RED: Write `TestCompactor_RecursiveGuard` (TC-018, EC-011)
  in `internal/compaction/compactor_test.go`
  - `compacting == true` → MaybeCompact returns original
  - Normal compact sets compacting during operation
- [ ] GREEN: Add `compacting bool` field and guard logic to Compactor
- [ ] REFACTOR: N/A

**TDD Cycle 4.3: Enable/disable toggle**
- [ ] RED: Write `TestCompactor_Disabled` (TC-023, TC-024, EC-014)
  - `FOXHARNESS_DISABLE_COMPACT` set → all disabled
  - `FOXHARNESS_DISABLE_AUTO_COMPACT` set → auto disabled
  - Config `compaction.enabled=false` → auto disabled
  - Env var takes precedence over config
- [ ] GREEN: Add disable checks to Compactor, check env vars in constructor
- [ ] REFACTOR: Extract env var check to helper

**TDD Cycle 4.4: Restructure Compactor**
- [ ] RED: Write `TestNewCompactor_WithRegistry` verifying new constructor
  uses model registry for context window
- [ ] GREEN: Restructure `Compactor` struct and `NewCompactor` to accept
  model name and use registry for context window. Replace `Config` struct
  with new fields from `ThresholdConfig`.
- [ ] REFACTOR: Remove old `Config` and `DefaultConfig`

**Phase 4 Deliverables**:
- Multi-threshold calculation with 4 levels
- Recursive guard preventing nested compaction
- Enable/disable via env vars and config
- Restructured Compactor with registry integration

### Phase 5: Tool Result Persistence (REQ-005, REQ-005a)

Persist large tool results to disk.

**TDD Cycle 5.1: Truncation**
- [ ] RED: Write `TestTruncateToCap` (TC-025, EC-013) in
  `internal/toolresult/truncate_test.go`
  - Content < 400KB → unchanged
  - Content = 400KB → unchanged
  - Content > 400KB → truncated with notice
- [ ] GREEN: Implement `TruncateToCap` in `internal/toolresult/truncate.go`
- [ ] REFACTOR: N/A

**TDD Cycle 5.2: Persistence threshold**
- [ ] RED: Write `TestPersistIfNeeded` (TC-007, TC-008, EC-002, EC-003, EC-008)
  in `internal/toolresult/persist_test.go`
  - Content > 50K → persisted with 2KB preview
  - Content ≤ 50K → not persisted
  - Content = 50,000 (exactly) → not persisted
  - Empty content → not persisted
  - File already exists → skip write (idempotent)
- [ ] GREEN: Implement `PersistIfNeeded` and `FileSystem` interface
- [ ] REFACTOR: Extract preview generation

**TDD Cycle 5.3: Budget enforcement**
- [ ] RED: Write `TestEnforceBudget` (TC-009, EC-004)
  - Total ≤ 200K → no persistence
  - Total > 200K → largest new results persisted first
  - Seen results never persisted (cache consistency)
  - Parallel results processed together
- [ ] GREEN: Implement `EnforceBudget` with sort-by-size logic
- [ ] REFACTOR: Clean up sorting

**TDD Cycle 5.4: Engine integration**
- [ ] RED: Write `TestEngine_ToolResultPersistence` integration test
  - Verify tool results flow through truncation and persistence
- [ ] GREEN: Wire tool result processing into engine's tool result loop
  - After `executeToolCalls`, truncate each result
  - Call `EnforceBudget` on the set
  - Use `session.ToolResultsDir()` for storage path
- [ ] REFACTOR: Verify all tests pass

**Phase 5 Deliverables**:
- Tool result truncation at 400KB
- Persistence at 50K character threshold
- Per-turn budget enforcement at 200K
- Engine integration with tool result processing

### Phase 6: Structured Summary & Compaction Format (REQ-006–REQ-009c)

9-section summary, language detection, boundary markers, cleanup.

**TDD Cycle 6.1: Summary prompt builder**
- [ ] RED: Write `TestBuildCompactPrompt_English` (TC-013) and
  `TestBuildCompactPrompt_Chinese` (TC-012, EC-010)
  in `internal/compaction/prompt_test.go`
  - English first message → English prompt
  - Chinese first message → Chinese prompt
  - Mixed languages → uses first message language
  - Prompt includes NO_TOOLS_PREAMBLE and NO_TOOLS_TRAILER
- [ ] GREEN: Implement `BuildCompactPrompt`, `DetectSummaryLanguage`,
  and CJK detection
- [ ] REFACTOR: Extract CJK detection to reusable function

**TDD Cycle 6.2: Summary formatting**
- [ ] RED: Write `TestFormatSummary` (TC-011)
  - Strip `<analysis>` block
  - Extract `<summary>` content
  - Handle missing `<analysis>` (summary-only output)
  - Handle missing `<summary>` (return raw)
- [ ] GREEN: Implement `FormatSummary`
- [ ] REFACTOR: Use regexp or string scanning

**TDD Cycle 6.3: Boundary marker**
- [ ] RED: Write `TestBoundaryMessage` (TC-021)
  in `internal/compaction/boundary_test.go`
  - Marker has trigger, pre_tokens, messages_summarized, timestamp
  - Marker is a system message
  - JSON round-trip of boundary data
- [ ] GREEN: Implement `CompactBoundary` and `BoundaryMessage`
- [ ] REFACTOR: Clean up

**TDD Cycle 6.4: Summary message with continuation instructions**
- [ ] RED: Write `TestSummaryMessage_Continuation` (TC-020)
  - Contains continuation wrapper text
  - References transcript path
  - Contains formatted summary content
- [ ] GREEN: Update `SummaryMessage` to use continuation wrapper (REQ-009a)
- [ ] REFACTOR: Remove old `stripExistingCompactionSummary` if no longer needed

**TDD Cycle 6.5: No-tools constraint**
- [ ] RED: Write `TestCompact_SummaryWithNoTools` (TC-019, EC-012)
  - Verify empty tools list passed to Generate during compaction
  - Verify prompt includes preamble and trailer
  - If LLM returns tool calls, extract text only (EC-012)
- [ ] GREEN: Update `Compactor.summarize` to pass empty tools and use new prompt
- [ ] REFACTOR: Clean up summarize method

**TDD Cycle 6.6: Post-compact cleanup**
- [ ] RED: Write `TestPostCompactCleanup` (TC-022, EC-015)
  - Cached token counts invalidated after compaction
  - Subsequent estimation uses new message order
- [ ] GREEN: Add cleanup logic to `MaybeCompact` after successful compaction
- [ ] REFACTOR: Clean up

**TDD Cycle 6.7: Compaction message format**
- [ ] RED: Write `TestMaybeCompact_MessageFormat` integration test
  - Verify output: [boundary] [summary] [anchor?] [recent]
  - Verify protocol boundary splitting (TC-010)
  - Verify compaction with no old messages → no compaction (EC-005)
  - Verify summary LLM failure → original messages (EC-006)
- [ ] GREEN: Assemble all components in `MaybeCompact`
- [ ] REFACTOR: Final cleanup, verify all tests pass

**Phase 6 Deliverables**:
- 9-section structured summary prompt
- Language auto-detection (CJK vs English)
- NO_TOOLS preamble/trailer enforcement
- Summary formatting (strip analysis, extract summary)
- Compact boundary marker
- Continuation instructions wrapper
- Post-compact cleanup
- Complete compaction message format

### Phase 7: Integration, Edge Cases & Benchmarks

Final integration, edge case coverage, performance validation.

**TDD Cycle 7.1: Engine end-to-end**
- [ ] RED: Write `TestEngine_FullCompactionFlow` integration test
  - Session with history exceeding threshold → compaction triggers
  - Verify usage tracking through callModel → HybridEstimator → MaybeCompact
  - Verify tool result persistence integration
- [ ] GREEN: Wire all components in engine
- [ ] REFACTOR: Clean up wiring

**TDD Cycle 7.2: Edge case sweep**
- [ ] RED → GREEN for each remaining edge case:
  - EC-007: Model name case sensitivity — already covered at unit level in
    Phase 3.1 (`TestModelRegistry_Lookup`); verify at integration level
    (engine → registry → threshold calculation) only if needed
  - EC-009: Very short context window warning
  - EC-014: Disable env var vs config conflict resolution
- [ ] REFACTOR: Consolidate edge case patterns

**TDD Cycle 7.3: Backward compatibility**
- [ ] Verify all existing tests pass: `go test ./...`
- [ ] Verify existing sessions load without migration
- [ ] Verify backward-compatible provider call (TC-014, TC-015)

**TDD Cycle 7.4: Performance benchmarks**
- [ ] Write `BenchmarkImprovedRoughEstimator` for 500-message context
  Verify < 1ms (NFR-002)
- [ ] Write `BenchmarkHybridEstimator` for 500-message context
- [ ] Write `BenchmarkPersistIfNeeded` for disk I/O
  Verify < 10ms (NFR-002)

**Phase 7 Deliverables**:
- Full end-to-end integration
- All edge cases covered
- Backward compatibility verified
- Performance benchmarks passing

## 9. Technical Decisions

### Decision 1: Usage on Message vs Side Channel

- **Choice**: Add `Usage *Usage` as an optional field on `schema.Message`
- **Rationale**: Enables hybrid token counting to scan messages for the last
  known usage without requiring a parallel data structure. The `omitempty`
  JSON tag ensures backward compatibility with existing sessions. The provider
  returns `GenerateResponse` (a wrapper struct), and the engine attaches Usage
  to the Message before appending to history.
- **Alternatives**:
  1. Separate `map[int]Usage` in engine — more complex, requires index management
  2. Only in `GenerateResponse` — loses usage data after the response is processed
- **Trade-offs**: Adds a field to the shared Message struct, but it's optional
  (pointer) and backward-compatible. The spec says "new return struct" for
  the provider (achieved via GenerateResponse), while Message gains an optional
  field for internal tracking.

### Decision 2: Provider Return Type Change

- **Choice**: Change `Generate` to return `(*GenerateResponse, error)` instead
  of `(*schema.Message, error)`
- **Rationale**: The `GenerateResponse` wrapper is the cleanest way to return
  both message and usage without modifying the `Message` struct itself. The
  spec explicitly requires this approach (REQ-001, NFR-004).
- **Alternatives**:
  1. Keep `Generate` as-is, add `GenerateWithUsage` — dual API surface, confusing
  2. Add Usage fields to Message — violates "not modify the existing return"
- **Trade-offs**: Breaking change for the `LLMProvider` interface, but only
  two internal implementations exist (OpenAI, Claude), both updated in Phase 1.
- **NFR-004 Reconciliation**: The spec states "Code that ignores usage data must
  compile without changes." This is interpreted as applying to code that accesses
  `schema.Message` fields (which remain unchanged), not the `Generate` call site
  itself. The only internal callers are `engine.callModel` and provider tests —
  both updated in Phase 1 within the same commit. External callers (if any) would
  need a one-line change from `msg, err := p.Generate(...)` to `resp, err := p.Generate(...)`.
  The `*schema.Message` struct is not modified (no Usage fields added to it);
  instead, `GenerateResponse` wraps it.

### Decision 3: Model Registry in Compaction Package

- **Choice**: `ModelRegistry` lives in `internal/compaction/` alongside the
  Compactor
- **Rationale**: The registry is primarily consumed by the Compactor for
  threshold calculation. It's a small struct with no external dependencies.
  Keeping it co-located avoids a micro-package.
- **Alternatives**:
  1. `internal/modelregistry/` — very small package, adds import complexity
  2. `internal/provider/` — registry is not about provider abstraction
- **Trade-offs**: Slightly larger compaction package, but related functionality
  stays together.

### Decision 4: Tool Result as Separate Package

- **Choice**: `internal/toolresult/` as its own package
- **Rationale**: Tool result persistence has distinct responsibilities
  (threshold detection, budget enforcement, file I/O, truncation) that don't
  belong in the engine or compaction packages. It has its own `FileSystem`
  interface for testability.
- **Alternatives**:
  1. In `internal/engine/` — engine already manages tools, but persistence
     is a separate concern
  2. In `internal/compaction/` — persistence happens during tool execution,
     not during compaction
- **Trade-offs**: Adds a new package, but keeps responsibilities clean.

### Decision 5: Environment Variable Prefix

- **Choice**: Use `FOXHARNESS_` prefix for environment variables
- **Rationale**: Avoids collision with Claude Code's `DISABLE_COMPACT` and
  other tools. Makes the variables self-documenting.
- **Alternatives**:
  1. No prefix (`DISABLE_COMPACT`) — risk of collision with Claude Code
  2. `FOX_` prefix — shorter but less specific
- **Trade-offs**: Longer variable names, but unambiguous and searchable.

### Decision 6: Summary in User Message (Not System Prompt)

- **Choice**: Summary is placed in a user message, not the system prompt
- **Rationale**: Matches Claude Code's design. The system prompt should remain
  stable (unchanged by compaction). User messages are the appropriate role for
  context summaries. The current code puts summaries in the system prompt
  (`compactedSystem.Content`), which will be changed.
- **Alternatives**:
  1. System prompt approach (current) — mixes operational instructions with
     context history, harder to manage
- **Trade-offs**: Changes the current compaction behavior, but aligns with the
  spec's resolved SPEC-002 issue.

### Decision 7: Config Override via YAML

- **Choice**: Model registry config overrides loaded from a YAML config file
- **Rationale**: The project already uses `gopkg.in/yaml.v3` (in go.mod). YAML
  is human-readable for config files. The config file can specify model name →
  context window mappings.
- **Alternatives**:
  1. JSON config — less human-readable
  2. Environment variables only — doesn't scale for multiple models
  3. Go code only — requires recompilation for new models
- **Trade-offs**: Adds a config file dependency, but uses existing library.
  Config file location is resolved during Phase 6 integration (per SPEC-012
  suggestion in review).
