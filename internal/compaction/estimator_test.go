package compaction

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestImprovedRoughEstimator_EstimateText(t *testing.T) {
	est := ImprovedRoughEstimator{}

	tests := []struct {
		name string
		in   string
		want int
	}{
		{
			name: "empty string is zero",
			in:   "",
			want: 0,
		},
		{
			name: "plain text 1000 bytes",
			in:   strings.Repeat("a", 1000),
			want: 333,
		},
		{
			name: "JSON object 1000 bytes",
			in:   "{" + strings.Repeat("a", 998) + "}",
			want: 666,
		},
		{
			name: "JSON array 1000 bytes",
			in:   "[" + strings.Repeat("a", 998) + "]",
			want: 666,
		},
		{
			name: "leading whitespace before JSON treated as JSON",
			in:   "   " + "{" + strings.Repeat("a", 996) + "}",
			want: (1000 / 2) * 4 / 3,
		},
		{
			name: "text starting with letter treated as text",
			in:   "abc {not json}",
			want: (14 / 4) * 4 / 3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := est.EstimateText(tc.in)
			if got != tc.want {
				t.Fatalf("EstimateText(%q) = %d, want %d", trim(tc.in), got, tc.want)
			}
		})
	}
}

func TestImprovedRoughEstimator_EstimateMessages(t *testing.T) {
	est := ImprovedRoughEstimator{}
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: strings.Repeat("x", 400)},
		{Role: schema.RoleAssistant, Content: strings.Repeat("y", 400)},
	}
	got := est.EstimateMessages(messages)
	wantPerMessage := 400 / 4 * 4 / 3
	want := 2 * wantPerMessage
	if got != want {
		t.Fatalf("EstimateMessages = %d, want %d", got, want)
	}
}

func TestHybridEstimator_WithUsage(t *testing.T) {
	est := NewHybridEstimator(ImprovedRoughEstimator{})

	messages := []schema.Message{
		{Role: schema.RoleSystem, Content: strings.Repeat("s", 4000)},
		{Role: schema.RoleUser, Content: strings.Repeat("u", 4000)},
		{
			Role:    schema.RoleAssistant,
			Content: "ok",
			Usage:   &schema.Usage{InputTokens: 5000, OutputTokens: 100},
		},
		{Role: schema.RoleUser, Content: strings.Repeat("x", 400)},
		{Role: schema.RoleUser, Content: strings.Repeat("y", 400)},
	}

	got := est.Estimate(messages)
	wantExact := 5000 + 100
	wantTail := ImprovedRoughEstimator{}.EstimateMessages(messages[3:])
	want := wantExact + wantTail
	if got != want {
		t.Fatalf("Estimate = %d, want exact %d + tail %d = %d", got, wantExact, wantTail, want)
	}
}

func TestHybridEstimator_WithoutUsage(t *testing.T) {
	est := NewHybridEstimator(ImprovedRoughEstimator{})
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: strings.Repeat("u", 800)},
		{Role: schema.RoleAssistant, Content: strings.Repeat("a", 800)},
	}
	got := est.Estimate(messages)
	want := ImprovedRoughEstimator{}.EstimateMessages(messages)
	if got != want {
		t.Fatalf("Estimate without usage = %d, want fallback %d", got, want)
	}
}

func TestHybridEstimator_ZeroUsageTreatedAsAbsent(t *testing.T) {
	est := NewHybridEstimator(ImprovedRoughEstimator{})
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: strings.Repeat("u", 400)},
		{
			Role:    schema.RoleAssistant,
			Content: "ok",
			Usage:   &schema.Usage{},
		},
		{Role: schema.RoleUser, Content: strings.Repeat("x", 400)},
	}
	got := est.Estimate(messages)
	want := ImprovedRoughEstimator{}.EstimateMessages(messages)
	if got != want {
		t.Fatalf("Estimate with zero-value usage = %d, want fallback %d", got, want)
	}
}

func TestHybridEstimator_ImplementsTokenEstimator(t *testing.T) {
	var _ TokenEstimator = NewHybridEstimator(ImprovedRoughEstimator{})
	var _ TokenEstimator = ImprovedRoughEstimator{}
}

func trim(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:40] + "..."
}

func BenchmarkImprovedRoughEstimator(b *testing.B) {
	messages := make([]schema.Message, 500)
	body := strings.Repeat("token ", 100)
	for i := range messages {
		role := schema.RoleUser
		if i%2 == 1 {
			role = schema.RoleAssistant
		}
		messages[i] = schema.Message{Role: role, Content: body}
	}
	est := ImprovedRoughEstimator{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		est.EstimateMessages(messages)
	}
}

func BenchmarkHybridEstimator(b *testing.B) {
	messages := make([]schema.Message, 500)
	body := strings.Repeat("token ", 100)
	for i := range messages {
		role := schema.RoleUser
		if i%2 == 1 {
			role = schema.RoleAssistant
		}
		messages[i] = schema.Message{Role: role, Content: body}
	}
	messages[300] = schema.Message{
		Role:    schema.RoleAssistant,
		Content: body,
		Usage:   &schema.Usage{InputTokens: 5000, OutputTokens: 200},
	}
	est := NewHybridEstimator(ImprovedRoughEstimator{})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		est.Estimate(messages)
	}
}
