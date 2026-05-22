package engine

import "github.com/Zts0hg/foxharness/internal/checkpoint"

// Config controls the behavior of the AgentEngine.
// It provides options for enabling the Thinking phase and setting
// turn limits for agent execution.
type Config struct {
	// EnableThinking enables the two-phase execution per turn:
	// Phase 1 (Thinking): LLM responds without tool access for planning
	// Phase 2 (Action): LLM has full tool access for execution
	EnableThinking bool

	// MaxTurns is the maximum number of turns the engine will execute.
	// If <= 0, defaults to 20. Each turn consists of optional thinking
	// followed by action execution.
	MaxTurns int

	// ProviderProtocol identifies the provider wire protocol used for model
	// calls, for trace/debug metadata.
	ProviderProtocol string

	// Model identifies the model used for model calls, for trace/debug metadata.
	Model string

	// Checkpointer receives user-message snapshot hooks when configured.
	Checkpointer checkpoint.Checkpointer

	// OnUserMessageID is called with the persisted user message sequence. It is
	// used by middleware wiring to associate later file edits with the same
	// snapshot.
	OnUserMessageID func(messageID string)
}

// DefaultConfig returns a Config with sensible defaults.
// EnableThinking is disabled, and MaxTurns is set to 20.
func DefaultConfig() Config {
	return Config{
		EnableThinking: false,
		MaxTurns:       20,
	}
}
