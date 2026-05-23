package compaction

// ThresholdConfig defines the multi-level compaction thresholds derived from
// the model's context window. The buffers carve out reserved capacity for the
// summary response, automatic compaction headroom, warning headroom, and a
// hard blocking ceiling. Refer to the spec section REQ-004 for the formulas.
type ThresholdConfig struct {
	ContextWindow      int
	ReservedForSummary int
	AutoCompactBuffer  int
	WarningBuffer      int
	BlockingBuffer     int
}

// DefaultThresholdConfig returns a ThresholdConfig populated with the
// Claude Code-derived defaults: 20K reserved for the summary, 13K of headroom
// before automatic compaction, 20K of headroom before the warning, and 3K
// before the hard block.
func DefaultThresholdConfig(contextWindow int) ThresholdConfig {
	return ThresholdConfig{
		ContextWindow:      contextWindow,
		ReservedForSummary: 20000,
		AutoCompactBuffer:  13000,
		WarningBuffer:      20000,
		BlockingBuffer:     3000,
	}
}

// EffectiveWindow returns the usable context window after subtracting the
// reservation for the summary response.
func (c ThresholdConfig) EffectiveWindow() int {
	return c.ContextWindow - c.ReservedForSummary
}

// AutoCompact returns the token count above which automatic compaction is
// triggered.
func (c ThresholdConfig) AutoCompact() int {
	return c.EffectiveWindow() - c.AutoCompactBuffer
}

// Warning returns the token count above which a warning should be surfaced
// to the user.
func (c ThresholdConfig) Warning() int {
	return c.AutoCompact() - c.WarningBuffer
}

// Blocking returns the token count above which the engine must refuse to
// continue without immediate compaction.
func (c ThresholdConfig) Blocking() int {
	return c.EffectiveWindow() - c.BlockingBuffer
}

// IsShortWindow reports whether the effective window is below the small
// threshold (40K tokens). Callers may log a warning when this is true so
// that operators notice degraded headroom.
func (c ThresholdConfig) IsShortWindow() bool {
	return c.EffectiveWindow() < 40000
}
