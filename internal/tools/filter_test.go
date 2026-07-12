package tools

import "testing"

type countingTurnRegistry struct {
	Registry
	turns int
}

func (r *countingTurnRegistry) BeginTurn() {
	r.turns++
}

func TestFilteredRegistryDelegatesBeginTurn(t *testing.T) {
	base := &countingTurnRegistry{Registry: NewRegistry()}
	filtered := NewFilteredRegistry(base, []string{"read_file"})
	turnAware, ok := filtered.(TurnAwareRegistry)
	if !ok {
		t.Fatalf("filtered registry type %T does not implement TurnAwareRegistry", filtered)
	}

	turnAware.BeginTurn()
	turnAware.BeginTurn()
	if base.turns != 2 {
		t.Fatalf("base BeginTurn calls = %d, want 2", base.turns)
	}
}
