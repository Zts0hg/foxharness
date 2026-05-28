package engine

import (
	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/schema"
)

// Config controls the behavior of the AgentEngine.
// It provides options for enabling the Thinking phase and setting
// turn limits for agent execution.
type Config struct {
	// EnableThinking enables the two-phase execution per turn:
	// Phase 1 (Thinking): LLM responds without tool access for planning
	// Phase 2 (Action): LLM has full tool access for execution
	EnableThinking bool

	// MaxTurns is the maximum number of turns the engine will execute.
	// If <= 0, the engine has no turn limit. Each turn consists of optional
	// thinking followed by action execution.
	MaxTurns int

	// ProviderProtocol identifies the provider wire protocol used for model
	// calls, for trace/debug metadata.
	ProviderProtocol string

	// Model identifies the model used for model calls, for trace/debug metadata.
	Model string

	// Checkpointer receives user-message snapshot hooks when configured.
	Checkpointer checkpoint.Checkpointer

	// DisplayPrompt is optional user-facing text for the initial user message.
	// When set, the model still receives the RunWithReporter userPrompt, while
	// transcripts and UI restore paths can render this human-authored form.
	DisplayPrompt string

	// OnUserMessageID is called with the persisted user message sequence. It is
	// used by middleware wiring to associate later file edits with the same
	// snapshot.
	OnUserMessageID func(messageID string)

	// OnToolCalled is invoked after every tool execution with the call and
	// its result. The hook is intentionally synchronous so callers can
	// inspect the call and reactively update external state — for example,
	// the slash command system uses this to activate conditional skills
	// whose path globs match a touched file. The hook must not block.
	OnToolCalled func(call schema.ToolCall, result schema.ToolResult)

	// NextTurnReminders, when set, is called at the start of every turn
	// and may return one or more strings to inject into the conversation
	// as system reminders. Returning nil/empty skips the injection for
	// that turn. The slash command system uses this to surface
	// conditionally-activated skills mid-run — by the time CheckConditional
	// fires inside OnToolCalled, the system prompt has already been
	// composed, so a per-turn drain is the only way to give the model
	// access to the new skill within the run that activated it.
	NextTurnReminders func() []string

	// OnContextEstimate is called at the start of each turn with the
	// estimated token usage and context window size. The TUI uses this
	// to display accurate context utilization during a run, since the
	// in-memory context (which may have been compacted) diverges from
	// the persisted message log.
	OnContextEstimate func(usedTokens, contextWindow int)
}

// DefaultConfig returns a Config with sensible defaults.
// EnableThinking is disabled, and MaxTurns is unlimited.
func DefaultConfig() Config {
	return Config{
		EnableThinking: false,
		MaxTurns:       0,
	}
}
