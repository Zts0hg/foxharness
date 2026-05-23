package compaction

import (
	"strings"
)

// DefaultContextWindow is the fallback context window size in tokens used by
// the ModelRegistry when a model name is not recognized.
const DefaultContextWindow = 128000

// ModelRegistry maps model name patterns to their context window sizes in
// tokens. Lookups are case-insensitive and resolved in the following order:
//  1. Exact match against the configuration overrides
//  2. Longest matching prefix from the registry's built-in entries
//  3. DefaultContextWindow fallback
type ModelRegistry struct {
	entries map[string]int
	config  map[string]int
}

// NewModelRegistry returns a ModelRegistry seeded with the known default
// entries for Zhipu and Anthropic models. All keys are stored in lower case
// so lookups can normalize input consistently.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		entries: map[string]int{
			"glm-4":             128000,
			"glm-4-plus":        128000,
			"glm-4-air":         128000,
			"claude-3.5-sonnet": 200000,
			"claude-3-opus":     200000,
			"claude-4-sonnet":   200000,
			"claude-4-opus":     200000,
		},
		config: map[string]int{},
	}
}

// SetConfigOverride replaces the configuration-driven overrides with the
// supplied map. Map keys are lower-cased to match lookup normalization.
func (r *ModelRegistry) SetConfigOverride(overrides map[string]int) {
	next := make(map[string]int, len(overrides))
	for name, window := range overrides {
		next[strings.ToLower(strings.TrimSpace(name))] = window
	}
	r.config = next
}

// Lookup returns the context window size for the supplied model name. When
// the model is not in the registry or configuration the DefaultContextWindow
// is returned. Comparison is case-insensitive.
func (r *ModelRegistry) Lookup(modelName string) int {
	key := strings.ToLower(strings.TrimSpace(modelName))
	if key == "" {
		return DefaultContextWindow
	}
	if window, ok := r.config[key]; ok {
		return window
	}
	bestPrefix := ""
	bestWindow := 0
	for prefix, window := range r.entries {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if len(prefix) > len(bestPrefix) {
			bestPrefix = prefix
			bestWindow = window
		}
	}
	if bestPrefix != "" {
		return bestWindow
	}
	return DefaultContextWindow
}
