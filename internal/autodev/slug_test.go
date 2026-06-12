package autodev

import "testing"

func TestSlugDerivesKebabCase(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Engine writes durable discoveries to MEMORY.md during runs", "engine-writes-durable-discoveries-to-memory-md-during-runs"},
		{"Add /compact command", "add-compact-command"},
		{"  Trim   spaces  ", "trim-spaces"},
		{"Ünïcode & symbols!!", "n-code-symbols"},
		{"", "item"},
	}

	for _, tt := range tests {
		got := Slug(tt.title, nil)
		if got != tt.want {
			t.Errorf("Slug(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestSlugTruncatesLongTitles(t *testing.T) {
	long := "this is a very long title that keeps going and going and going and going and going far past the limit"
	got := Slug(long, nil)
	if len(got) > 64 {
		t.Errorf("Slug length = %d, want <= 64 (%q)", len(got), got)
	}
	if got[len(got)-1] == '-' {
		t.Errorf("Slug = %q, want no trailing hyphen", got)
	}
}

func TestSlugDisambiguatesCollisions(t *testing.T) {
	taken := map[string]bool{}

	first := Slug("Fix the bug", taken)
	if first != "fix-the-bug" {
		t.Fatalf("first Slug = %q, want fix-the-bug", first)
	}
	taken[first] = true

	second := Slug("Fix the bug", taken)
	if second == first {
		t.Fatalf("second Slug = %q, want disambiguated from %q", second, first)
	}
	if second != "fix-the-bug-2" {
		t.Errorf("second Slug = %q, want fix-the-bug-2", second)
	}
	taken[second] = true

	third := Slug("Fix the bug", taken)
	if third != "fix-the-bug-3" {
		t.Errorf("third Slug = %q, want fix-the-bug-3", third)
	}
}
