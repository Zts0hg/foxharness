package compaction

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestDetectSummaryLanguage_English(t *testing.T) {
	got := DetectSummaryLanguage([]schema.Message{
		{Role: schema.RoleUser, Content: "Please refactor the engine package"},
	})
	if got != "en" {
		t.Fatalf("DetectSummaryLanguage = %q, want en", got)
	}
}

func TestDetectSummaryLanguage_Chinese(t *testing.T) {
	got := DetectSummaryLanguage([]schema.Message{
		{Role: schema.RoleUser, Content: "请帮我重构 engine 包"},
	})
	if got != "zh" {
		t.Fatalf("DetectSummaryLanguage = %q, want zh", got)
	}
}

func TestDetectSummaryLanguage_FirstMessageWins(t *testing.T) {
	got := DetectSummaryLanguage([]schema.Message{
		{Role: schema.RoleSystem, Content: "system"},
		{Role: schema.RoleUser, Content: "English first message"},
		{Role: schema.RoleAssistant, Content: "回复"},
		{Role: schema.RoleUser, Content: "再来一条中文"},
	})
	if got != "en" {
		t.Fatalf("DetectSummaryLanguage = %q, want en (first user message wins)", got)
	}
}

func TestDetectSummaryLanguage_EmptyDefaultsToEnglish(t *testing.T) {
	got := DetectSummaryLanguage(nil)
	if got != "en" {
		t.Fatalf("DetectSummaryLanguage(nil) = %q, want en", got)
	}
}

func TestBuildCompactPrompt_English(t *testing.T) {
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: "Implement feature X"},
		{Role: schema.RoleAssistant, Content: "Working on it"},
	}
	prompt := BuildCompactPrompt(messages, "en", "")

	if !strings.Contains(prompt, "CRITICAL: Respond with TEXT ONLY") {
		t.Fatalf("English prompt missing NO_TOOLS_PREAMBLE: %s", prompt)
	}
	if !strings.Contains(prompt, "REMINDER: Do NOT call any tools") {
		t.Fatalf("English prompt missing NO_TOOLS_TRAILER: %s", prompt)
	}
	if !strings.Contains(prompt, "Primary Request") {
		t.Fatalf("English prompt missing section 1 marker: %s", prompt)
	}
	if !strings.Contains(prompt, "Next Step") {
		t.Fatalf("English prompt missing section 9 marker")
	}
	if !strings.Contains(prompt, "<analysis>") || !strings.Contains(prompt, "<summary>") {
		t.Fatalf("English prompt missing draft/summary tags")
	}
}

func TestBuildCompactPrompt_Chinese(t *testing.T) {
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: "重构 engine 包"},
	}
	prompt := BuildCompactPrompt(messages, "zh", "")
	if !strings.Contains(prompt, "CRITICAL: Respond with TEXT ONLY") {
		t.Fatalf("Chinese prompt should still include preamble (universal): %s", prompt)
	}
	if !strings.Contains(prompt, "中文") && !strings.Contains(prompt, "Chinese") {
		t.Fatalf("Chinese prompt should request Chinese-language output: %s", prompt)
	}
	if !strings.Contains(prompt, "<summary>") {
		t.Fatalf("Chinese prompt missing summary tag")
	}
}

func TestBuildCompactPrompt_WithCustomInstructions(t *testing.T) {
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: "Implement auth flow"},
		{Role: schema.RoleAssistant, Content: "Done"},
	}
	prompt := BuildCompactPrompt(messages, "en", "focus on the database migration work")

	if !strings.Contains(prompt, "Additional Instructions") {
		t.Fatalf("prompt should contain Additional Instructions header")
	}
	if !strings.Contains(prompt, "focus on the database migration work") {
		t.Fatalf("prompt should contain user's custom instructions")
	}
	if !strings.Contains(prompt, "Primary Request") {
		t.Fatalf("prompt should still contain the standard 9-section template")
	}
}

func TestBuildCompactPrompt_EmptyCustomInstructions(t *testing.T) {
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: "Implement auth flow"},
	}
	prompt := BuildCompactPrompt(messages, "en", "")

	if strings.Contains(prompt, "Additional Instructions") {
		t.Fatalf("prompt should NOT contain Additional Instructions when empty: %s", prompt)
	}
}

func TestFormatSummary_StripsAnalysisExtractsSummary(t *testing.T) {
	raw := "<analysis>thinking here</analysis>\n<summary>9 sections here</summary>"
	got := FormatSummary(raw)
	if strings.Contains(got, "thinking here") {
		t.Fatalf("FormatSummary did not strip <analysis>: %q", got)
	}
	if !strings.Contains(got, "9 sections here") {
		t.Fatalf("FormatSummary did not include summary body: %q", got)
	}
	if strings.Contains(got, "<summary>") || strings.Contains(got, "</summary>") {
		t.Fatalf("FormatSummary should strip summary tags: %q", got)
	}
}

func TestFormatSummary_MissingAnalysisIsFine(t *testing.T) {
	raw := "<summary>just the summary</summary>"
	got := FormatSummary(raw)
	if got != "just the summary" {
		t.Fatalf("FormatSummary = %q, want 'just the summary'", got)
	}
}

func TestFormatSummary_MissingSummaryReturnsRaw(t *testing.T) {
	raw := "no tags at all here"
	got := FormatSummary(raw)
	if got != raw {
		t.Fatalf("FormatSummary returned %q, want raw unchanged", got)
	}
}
