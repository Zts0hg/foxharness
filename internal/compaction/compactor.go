// Package compaction provides automatic context summarization for
// long-running agent sessions. When the estimated token count approaches
// the configured threshold, the Compactor replaces older messages with an
// LLM-generated summary while preserving the system prompt, the original
// user message anchor, and a configurable window of recent messages.
package compaction

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

// EnvDisableCompact is the environment variable name that disables all
// compaction operations (both automatic and any future manual triggers).
const EnvDisableCompact = "FOXHARNESS_DISABLE_COMPACT"

// EnvDisableAutoCompact is the environment variable name that disables only
// automatic compaction while leaving manual triggers enabled.
const EnvDisableAutoCompact = "FOXHARNESS_DISABLE_AUTO_COMPACT"

// DefaultRecentKeep is the number of trailing messages preserved verbatim
// when compaction collapses earlier turns.
const DefaultRecentKeep = 12

// DefaultSummaryMaxTokens is the historical summary response budget; the
// engine relies on the LLM's natural output length and treats this as a soft
// hint for compatibility with persisted configurations.
const DefaultSummaryMaxTokens = 2048

// TokenEstimator estimates the token cost of a message slice.
type TokenEstimator interface {
	Estimate(messages []schema.Message) int
}

// RoughEstimator provides a fast, rune-count-based token approximation. It is
// retained for callers and tests that still depend on the historical
// estimator. New code should prefer ImprovedRoughEstimator with
// HybridEstimator for more accurate counting.
type RoughEstimator struct{}

// Estimate returns a rough token count for the given messages by summing
// Unicode rune counts across content and tool call fields.
func (RoughEstimator) Estimate(messages []schema.Message) int {
	chars := 0
	for _, msg := range messages {
		chars += utf8.RuneCountInString(msg.Content)
		for _, call := range msg.ToolCalls {
			chars += utf8.RuneCountInString(call.Name)
			chars += utf8.RuneCount(call.Arguments)
		}
	}

	if chars == 0 {
		return 0
	}

	return chars + 1
}

// CompactionConfig controls the behavior of the Compactor. The Model field is
// looked up in the embedded ModelRegistry to derive the ContextWindow when
// ContextWindow is left zero. Enabled defaults to true and may be flipped to
// false by config or environment variables.
//
// Estimator, AutoCompactThreshold, and Clock are optional injection points
// primarily used by tests and benchmarks that need deterministic token counts
// or timestamps. Leaving them at their zero values selects the production
// defaults (HybridEstimator + ThresholdConfig.AutoCompact() + time.Now).
type CompactionConfig struct {
	Enabled              bool
	Model                string
	ContextWindow        int
	RecentKeep           int
	SummaryMaxTokens     int
	SessionDir           string
	TranscriptPath       string
	Overrides            map[string]int
	Estimator            TokenEstimator
	AutoCompactThreshold int
	Clock                func() time.Time
}

// DefaultCompactionConfig returns a CompactionConfig with Enabled=true and
// the standard recent-message and summary budgets pre-populated.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Enabled:          true,
		RecentKeep:       DefaultRecentKeep,
		SummaryMaxTokens: DefaultSummaryMaxTokens,
	}
}

// Compactor decides when and how to summarize conversation history to stay
// within token limits. It tracks an active-compaction flag to prevent
// recursive entry and reads disable toggles from configuration plus
// environment variables.
type Compactor struct {
	provider provider.LLMProvider

	estimator  TokenEstimator
	registry   *ModelRegistry
	thresholds ThresholdConfig

	config               CompactionConfig
	autoCompactThreshold int
	clock                func() time.Time

	disabled     bool
	autoDisabled bool

	mu         sync.Mutex
	compacting bool
}

// NewCompactor constructs a Compactor with the supplied provider and
// configuration. The model name is resolved against the built-in
// ModelRegistry (and any user overrides) to determine the context window
// when CompactionConfig.ContextWindow is zero. Disable flags from
// FOXHARNESS_DISABLE_COMPACT and FOXHARNESS_DISABLE_AUTO_COMPACT are read
// once at construction time so the Compactor is hermetic per session.
func NewCompactor(p provider.LLMProvider, cfg CompactionConfig) (*Compactor, error) {
	if p == nil {
		return nil, fmt.Errorf("compaction: provider is required")
	}

	registry := NewModelRegistry()
	if len(cfg.Overrides) > 0 {
		registry.SetConfigOverride(cfg.Overrides)
	}

	contextWindow := cfg.ContextWindow
	if contextWindow <= 0 {
		contextWindow = registry.Lookup(cfg.Model)
	}
	thresholds := DefaultThresholdConfig(contextWindow)
	if thresholds.IsShortWindow() {
		log.Printf("[Compactor] effective window %d is below 40K — compaction headroom is degraded", thresholds.EffectiveWindow())
	}

	if cfg.RecentKeep <= 0 {
		cfg.RecentKeep = DefaultRecentKeep
	}
	if cfg.SummaryMaxTokens <= 0 {
		cfg.SummaryMaxTokens = DefaultSummaryMaxTokens
	}

	estimator := cfg.Estimator
	if estimator == nil {
		estimator = NewHybridEstimator(ImprovedRoughEstimator{})
	}

	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}

	disabled := envHasValue(EnvDisableCompact)
	autoDisabled := disabled || envHasValue(EnvDisableAutoCompact) || !cfg.Enabled

	return &Compactor{
		provider:             p,
		estimator:            estimator,
		registry:             registry,
		thresholds:           thresholds,
		config:               cfg,
		autoCompactThreshold: cfg.AutoCompactThreshold,
		clock:                clock,

		disabled:     disabled,
		autoDisabled: autoDisabled,
	}, nil
}

// Estimate returns the estimated token count for messages using the
// configured estimator.
func (c *Compactor) Estimate(messages []schema.Message) int {
	return c.estimator.Estimate(messages)
}

// Threshold returns the soft token threshold that triggers compaction.
// When CompactionConfig.AutoCompactThreshold is set (non-zero), that value
// takes precedence; otherwise the ThresholdConfig-derived value is used.
func (c *Compactor) Threshold() int {
	if c.autoCompactThreshold > 0 {
		return c.autoCompactThreshold
	}
	return c.thresholds.AutoCompact()
}

// Thresholds exposes the underlying multi-level threshold configuration.
func (c *Compactor) Thresholds() ThresholdConfig {
	return c.thresholds
}

// Registry exposes the embedded ModelRegistry.
func (c *Compactor) Registry() *ModelRegistry {
	return c.registry
}

// Config returns a defensive copy of the active CompactionConfig so callers
// cannot mutate the Compactor's internal state through the returned map
// reference.
func (c *Compactor) Config() CompactionConfig {
	cfg := c.config
	if len(c.config.Overrides) > 0 {
		cfg.Overrides = make(map[string]int, len(c.config.Overrides))
		for k, v := range c.config.Overrides {
			cfg.Overrides[k] = v
		}
	}
	return cfg
}

// TranscriptPath returns the configured transcript path used when wrapping
// summary messages with continuation instructions.
func (c *Compactor) TranscriptPath() string {
	return c.config.TranscriptPath
}

// RecentKeep returns the number of recent messages preserved during
// compaction.
func (c *Compactor) RecentKeep() int {
	return c.config.RecentKeep
}

// Summarize produces a high-density summary for messages. The full message
// history is used only to detect the summary language; the actual content
// summarized is the supplied messages slice.
func (c *Compactor) Summarize(ctx context.Context, messages []schema.Message) (string, error) {
	return c.summarize(ctx, messages, messages)
}

// BuildSummaryMessage builds the model-visible summary message including the
// continuation wrapper (REQ-009a). The wrapper instructs the model to resume
// without acknowledging the summary and links to the full transcript for
// detail recovery when the transcript path is non-empty.
func BuildSummaryMessage(summary, transcriptPath string) schema.Message {
	body := strings.TrimSpace(summary)
	var b strings.Builder
	b.WriteString("## Compacted Context Summary\n\n")
	b.WriteString("This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion.\n\n")
	b.WriteString(body)
	b.WriteString("\n\n")
	if transcriptPath != "" {
		b.WriteString(fmt.Sprintf("If you need specific details from before compaction, read the full transcript at: %s\n\n", transcriptPath))
	}
	b.WriteString("Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary.")
	return schema.Message{
		Role:    schema.RoleUser,
		Content: b.String(),
	}
}

// MaybeCompact checks whether the estimated token usage exceeds the
// automatic-compaction threshold and, if so, summarizes older messages into a
// new user message while preserving the system prompt, the first user message
// anchor, and the most recent messages. If compaction is not needed or has
// been disabled the original slice is returned unchanged.
func (c *Compactor) MaybeCompact(ctx context.Context, messages []schema.Message) ([]schema.Message, error) {
	c.mu.Lock()
	if c.compacting {
		c.mu.Unlock()
		return messages, nil
	}
	if c.disabled || c.autoDisabled {
		c.mu.Unlock()
		return messages, nil
	}
	c.compacting = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.compacting = false
		c.mu.Unlock()
	}()

	used := c.Estimate(messages)
	threshold := c.Threshold()

	if used < threshold {
		return messages, nil
	}

	if len(messages) <= c.config.RecentKeep+2 {
		return messages, nil
	}

	system := messages[0]
	keepStart := 1
	var anchors []schema.Message
	if len(messages) > 1 && messages[1].Role == schema.RoleUser && messages[1].ToolCallID == "" {
		anchors = append(anchors, messages[1])
		keepStart = 2
	}

	split := len(messages) - c.config.RecentKeep
	if split < keepStart {
		return messages, nil
	}
	split = moveSplitToProtocolBoundary(messages, split, keepStart)
	if split <= keepStart {
		return messages, nil
	}

	old := messages[keepStart:split]
	recent := messages[split:]
	summary, err := c.summarize(ctx, messages, old)
	if err != nil {
		return messages, fmt.Errorf("context compaction 失败: %w", err)
	}

	boundary := CompactBoundary{
		Trigger:            "auto",
		PreTokens:          used,
		MessagesSummarized: len(old),
		Timestamp:          c.clock().UTC().Format(time.RFC3339),
	}
	summaryMessage := BuildSummaryMessage(summary, c.config.TranscriptPath)

	compacted := make([]schema.Message, 0, 3+len(anchors)+len(recent))
	compacted = append(compacted, system)
	compacted = append(compacted, anchors...)
	compacted = append(compacted, BoundaryMessage(boundary))
	compacted = append(compacted, summaryMessage)
	compacted = append(compacted, recent...)
	c.runPostCompactCleanup(used, len(old), compacted)
	return compacted, nil
}

// runPostCompactCleanup emits an observability log line summarizing the
// compaction event with pre/post token counts as required by REQ-009c.
func (c *Compactor) runPostCompactCleanup(preTokens, summarized int, compacted []schema.Message) {
	postTokens := c.Estimate(compacted)
	log.Printf("[Compactor] summarized %d messages (pre-tokens=%d post-tokens=%d delta=%d)",
		summarized, preTokens, postTokens, preTokens-postTokens)
}

func moveSplitToProtocolBoundary(messages []schema.Message, split int, min int) int {
	if split >= len(messages) {
		return split
	}

	for split > min && messages[split].ToolCallID != "" {
		split--
	}

	return split
}

func (c *Compactor) summarize(ctx context.Context, fullHistory, toCompact []schema.Message) (string, error) {
	language := DetectSummaryLanguage(fullHistory)
	prompt := BuildCompactPrompt(toCompact, language)

	resp, err := c.provider.Generate(ctx, []schema.Message{
		{Role: schema.RoleUser, Content: prompt},
	}, nil)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Message == nil {
		return "", fmt.Errorf("compaction summary provider returned empty response")
	}

	return FormatSummary(resp.Message.Content), nil
}

func renderMessagesForSummary(messages []schema.Message) string {
	var b strings.Builder
	for i, msg := range messages {
		b.WriteString(fmt.Sprintf("\n--- message %d role=%s ---\n", i+1, msg.Role))

		if msg.Content != "" {
			b.WriteString(truncate(msg.Content, 4000))
			b.WriteByte('\n')
		}

		for _, call := range msg.ToolCalls {
			b.WriteString(fmt.Sprintf(
				"[tool_call] id=%s name=%s args=%s\n",
				call.ID,
				call.Name,
				truncate(string(call.Arguments), 1000),
			))
		}

		if msg.ToolCallID != "" {
			b.WriteString(fmt.Sprintf("[tool_result_for] %s\n", msg.ToolCallID))
		}
	}

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated for compaction]..."
}

func envHasValue(name string) bool {
	v, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	return strings.TrimSpace(v) != ""
}
