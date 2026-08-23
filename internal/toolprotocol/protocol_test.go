package toolprotocol

import (
	"context"
	"testing"
)

func TestCapabilitiesPreserveExplicitEmptySnapshot(t *testing.T) {
	ctx := WithCapabilities(context.Background(), []string{})
	if !HasCapabilities(ctx) {
		t.Fatal("expected explicit capability snapshot")
	}
	got := CapabilitiesFromContext(ctx)
	if got == nil || len(got) != 0 {
		t.Fatalf("capabilities = %#v, want explicit empty deny-all slice", got)
	}
}
