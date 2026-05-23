package compaction

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestBoundaryMessage_ContainsMetadata(t *testing.T) {
	boundary := CompactBoundary{
		Trigger:            "auto",
		PreTokens:          15000,
		MessagesSummarized: 8,
		Timestamp:          "2026-05-23T10:00:00Z",
	}
	msg := BoundaryMessage(boundary)

	if msg.Role != schema.RoleSystem {
		t.Fatalf("BoundaryMessage role = %q, want system", msg.Role)
	}
	if !strings.Contains(msg.Content, `"trigger":"auto"`) {
		t.Fatalf("BoundaryMessage missing trigger: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, `"pre_tokens":15000`) {
		t.Fatalf("BoundaryMessage missing pre_tokens: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, `"messages_summarized":8`) {
		t.Fatalf("BoundaryMessage missing messages_summarized: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "2026-05-23T10:00:00Z") {
		t.Fatalf("BoundaryMessage missing timestamp: %q", msg.Content)
	}
}

func TestCompactBoundary_JSONRoundTrip(t *testing.T) {
	want := CompactBoundary{
		Trigger:            "manual",
		PreTokens:          50000,
		MessagesSummarized: 20,
		Timestamp:          "2026-05-23T10:00:00Z",
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got CompactBoundary
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != want {
		t.Fatalf("round-trip = %#v, want %#v", got, want)
	}
}
