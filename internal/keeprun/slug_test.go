package keeprun

import (
	"strings"
	"testing"
)

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"standard_feature_title", "[feature] Add dark mode support", "add-dark-mode-support"},
		{"special_characters_stripped", "[fix] Fix timeout on slow connections!!!", "fix-timeout-on-slow-connections"},
		{"type_prefix_stripped", "[refactor] Clean Up Utils", "clean-up-utils"},
		{"no_type_prefix", "Add Logging", "add-logging"},
		{"uppercase_lowercased", "ADD DARK MODE", "add-dark-mode"},
		{"consecutive_hyphens_collapsed", "feature:   multiple   spaces", "feature-multiple-spaces"},
		{"leading_trailing_separators_stripped", "  !!Hello World!!  ", "hello-world"},
		{"digits_preserved", "[feature] Support HTTP2 and IPv6", "support-http2-and-ipv6"},
		{"unicode_becomes_hyphens", "[feature] Café Münü", "caf-m-n"},
		{"empty_after_stripping", "[feature] ", "task"},
		{"only_special_characters", "!!! @#$ %%%", "task"},
		{"only_type_prefix", "[chore]", "task"},
		{
			"truncate_at_hyphen_boundary",
			"aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm",
			"aaaa-bbbb-cccc-dddd-eeee-ffff-gggg-hhhh-iiii-jjjj-kkkk-llll",
		},
		{"truncate_hard_when_no_hyphen", strings.Repeat("a", 70), strings.Repeat("a", 60)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateSlug(tt.title)
			if got != tt.want {
				t.Errorf("GenerateSlug(%q) = %q, want %q", tt.title, got, tt.want)
			}
			if len(got) > 60 {
				t.Errorf("GenerateSlug(%q) length = %d, want <= 60", tt.title, len(got))
			}
		})
	}
}

func TestDeduplicateSlug(t *testing.T) {
	tests := []struct {
		name     string
		slug     string
		existing []string
		want     string
	}{
		{"no_collision_unchanged", "add-dark-mode", []string{"other-branch"}, "add-dark-mode"},
		{"empty_existing_unchanged", "add-dark-mode", nil, "add-dark-mode"},
		{"single_collision_appends_2", "add-dark-mode", []string{"add-dark-mode"}, "add-dark-mode-2"},
		{
			"chain_collision_increments",
			"add-dark-mode",
			[]string{"add-dark-mode", "add-dark-mode-2"},
			"add-dark-mode-3",
		},
		{
			"continues_incrementing_past_existing_suffixes",
			"add-dark-mode",
			[]string{"add-dark-mode", "add-dark-mode-2", "add-dark-mode-3"},
			"add-dark-mode-4",
		},
		{
			"slug_with_trailing_number_appends_suffix",
			"fix-bug-2",
			[]string{"fix-bug-2"},
			"fix-bug-2-2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeduplicateSlug(tt.slug, tt.existing)
			if got != tt.want {
				t.Errorf("DeduplicateSlug(%q, %v) = %q, want %q", tt.slug, tt.existing, got, tt.want)
			}
		})
	}
}
