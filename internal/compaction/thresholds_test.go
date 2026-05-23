package compaction

import "testing"

func TestThresholdConfig_DefaultBuffers(t *testing.T) {
	c := DefaultThresholdConfig(128000)

	if c.ReservedForSummary != 20000 {
		t.Fatalf("ReservedForSummary = %d, want 20000", c.ReservedForSummary)
	}
	if c.AutoCompactBuffer != 13000 {
		t.Fatalf("AutoCompactBuffer = %d, want 13000", c.AutoCompactBuffer)
	}
	if c.WarningBuffer != 20000 {
		t.Fatalf("WarningBuffer = %d, want 20000", c.WarningBuffer)
	}
	if c.BlockingBuffer != 3000 {
		t.Fatalf("BlockingBuffer = %d, want 3000", c.BlockingBuffer)
	}
}

func TestThresholdConfig_128K(t *testing.T) {
	c := DefaultThresholdConfig(128000)

	if got, want := c.EffectiveWindow(), 108000; got != want {
		t.Fatalf("EffectiveWindow = %d, want %d", got, want)
	}
	if got, want := c.AutoCompact(), 95000; got != want {
		t.Fatalf("AutoCompact = %d, want %d", got, want)
	}
	if got, want := c.Warning(), 75000; got != want {
		t.Fatalf("Warning = %d, want %d", got, want)
	}
	if got, want := c.Blocking(), 105000; got != want {
		t.Fatalf("Blocking = %d, want %d", got, want)
	}
}

func TestThresholdConfig_ShortWindow_PositiveValues(t *testing.T) {
	c := DefaultThresholdConfig(45000)

	if got := c.EffectiveWindow(); got <= 0 {
		t.Fatalf("EffectiveWindow = %d, want > 0", got)
	}
	if got := c.AutoCompact(); got <= 0 {
		t.Fatalf("AutoCompact = %d, want > 0", got)
	}
	if got := c.Blocking(); got <= 0 {
		t.Fatalf("Blocking = %d, want > 0", got)
	}
	if !c.IsShortWindow() {
		t.Fatalf("IsShortWindow() = false for effective < 40000, want true")
	}
}

func TestThresholdConfig_NormalWindow_NotShort(t *testing.T) {
	c := DefaultThresholdConfig(128000)
	if c.IsShortWindow() {
		t.Fatalf("IsShortWindow() = true for 108K effective window, want false")
	}
}
