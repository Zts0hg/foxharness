package compaction

import (
	"testing"
)

func TestModelRegistry_Lookup(t *testing.T) {
	r := NewModelRegistry()

	tests := []struct {
		name  string
		model string
		want  int
	}{
		{"glm-4 exact", "glm-4", 128000},
		{"glm-4-plus exact", "glm-4-plus", 128000},
		{"glm-4-air exact", "glm-4-air", 128000},
		{"glm-4-air-x prefix matches glm-4-air", "glm-4-air-x", 128000},
		{"claude-3.5-sonnet", "claude-3.5-sonnet", 200000},
		{"claude-3-opus", "claude-3-opus", 200000},
		{"claude-4-sonnet", "claude-4-sonnet", 200000},
		{"claude-4-opus", "claude-4-opus", 200000},
		{"case insensitive uppercase", "GLM-4", 128000},
		{"case insensitive mixed", "Claude-4-Sonnet", 200000},
		{"unknown model falls back to default", "totally-unknown-model", DefaultContextWindow},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := r.Lookup(tc.model)
			if got != tc.want {
				t.Fatalf("Lookup(%q) = %d, want %d", tc.model, got, tc.want)
			}
		})
	}
}

func TestModelRegistry_LongestPrefixWins(t *testing.T) {
	r := NewModelRegistry()
	r.entries["glm-4"] = 100000
	r.entries["glm-4-special-edition"] = 250000

	if got := r.Lookup("glm-4-special-edition-x"); got != 250000 {
		t.Fatalf("Lookup with longest prefix = %d, want 250000", got)
	}
}

func TestModelRegistry_ConfigOverride(t *testing.T) {
	t.Run("config overrides exact match", func(t *testing.T) {
		r := NewModelRegistry()
		r.SetConfigOverride(map[string]int{"glm-4": 200000})
		if got := r.Lookup("glm-4"); got != 200000 {
			t.Fatalf("Lookup(glm-4) with override = %d, want 200000", got)
		}
	})

	t.Run("config adds new model", func(t *testing.T) {
		r := NewModelRegistry()
		r.SetConfigOverride(map[string]int{"custom-model-v9": 500000})
		if got := r.Lookup("custom-model-v9"); got != 500000 {
			t.Fatalf("Lookup(custom-model-v9) = %d, want 500000", got)
		}
	})

	t.Run("config override beats prefix match", func(t *testing.T) {
		r := NewModelRegistry()
		r.SetConfigOverride(map[string]int{"glm-4-air-x": 60000})
		if got := r.Lookup("glm-4-air-x"); got != 60000 {
			t.Fatalf("Lookup(glm-4-air-x) with override = %d, want 60000", got)
		}
		if got := r.Lookup("glm-4-air"); got != 128000 {
			t.Fatalf("Lookup(glm-4-air) should still hit prefix = %d, want 128000", got)
		}
	})

	t.Run("override is case insensitive", func(t *testing.T) {
		r := NewModelRegistry()
		r.SetConfigOverride(map[string]int{"GLM-4": 175000})
		if got := r.Lookup("glm-4"); got != 175000 {
			t.Fatalf("Lookup(glm-4) with uppercased override = %d, want 175000", got)
		}
	})
}
