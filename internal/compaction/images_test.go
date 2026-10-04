package compaction

import (
	"github.com/Zts0hg/foxharness/internal/schema"
	"strings"
	"testing"
)

func TestImageArtifactsHaveBudgetAndExplicitSummaryBoundary(t *testing.T) {
	message := schema.Message{Role: schema.RoleUser, ToolCallID: "shot", ContentParts: []schema.ContentPart{{Type: "image", ArtifactKey: "media/hash.png"}}}
	estimator := ImprovedRoughEstimator{}
	plain := message.Clone()
	plain.ContentParts = nil
	if estimator.Estimate([]schema.Message{message}) <= estimator.Estimate([]schema.Message{plain}) {
		t.Fatal("image has no token budget")
	}
	text := renderMessagesForSummary([]schema.Message{message})
	if !strings.Contains(text, "media/hash.png") || !strings.Contains(text, "not included") {
		t.Fatalf("summary implies visual access: %q", text)
	}
}
