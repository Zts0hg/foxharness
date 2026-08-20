package engine

import "fmt"

// TurnLimitError reports that an engine run consumed its configured turn
// budget before reaching a terminal assistant response.
type TurnLimitError struct {
	MaxTurns int
}

// Error preserves the established user-visible turn-limit message.
func (e *TurnLimitError) Error() string {
	return fmt.Sprintf("超过最大 Turn 数限制: %d", e.MaxTurns)
}

// RuntimeErrorKind exposes the stable cross-boundary classification without requiring consumers to import engine.
func (*TurnLimitError) RuntimeErrorKind() string { return "turn_limit" }
