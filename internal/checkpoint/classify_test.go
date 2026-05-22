package checkpoint

import (
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestIsSynthetic(t *testing.T) {
	cases := []session.MessageRecord{
		{Kind: "progress"},
		{Kind: "system"},
		{Message: schema.Message{Role: schema.RoleSystem, Content: "system"}},
		{Message: schema.Message{Role: schema.RoleUser, ToolCallID: "call-1", Content: ""}},
		{IsMeta: true, Message: schema.Message{Role: schema.RoleUser, Content: "meta"}},
		{IsCompactSummary: true, Message: schema.Message{Role: schema.RoleUser, Content: "summary"}},
		{IsVisibleInTranscriptOnly: true, Message: schema.Message{Role: schema.RoleAssistant, Content: "hidden"}},
		{Message: schema.Message{Role: schema.RoleUser, Content: "## Compacted Context Summary\nold"}},
	}
	for i, rec := range cases {
		if !IsSynthetic(rec) {
			t.Fatalf("case %d synthetic = false, want true: %#v", i, rec)
		}
	}
}

func TestIsMeaningful(t *testing.T) {
	cases := []session.MessageRecord{
		{Message: schema.Message{Role: schema.RoleAssistant, Content: "answer"}},
		{Message: schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Name: "read_file"}}}},
		{Message: schema.Message{Role: schema.RoleUser, ToolCallID: "call-1", Content: "tool output"}},
	}
	for i, rec := range cases {
		if !IsMeaningful(rec) {
			t.Fatalf("case %d meaningful = false, want true: %#v", i, rec)
		}
	}
}

func TestSelectableMessages(t *testing.T) {
	now := time.Now()
	records := []session.MessageRecord{
		{Seq: 1, Time: now, Message: schema.Message{Role: schema.RoleUser, Content: "first"}},
		{Seq: 2, Time: now, IsMeta: true, Message: schema.Message{Role: schema.RoleUser, Content: "meta"}},
		{Seq: 3, Time: now, Message: schema.Message{Role: schema.RoleAssistant, Content: "answer"}},
		{Seq: 4, Time: now, Message: schema.Message{Role: schema.RoleUser, ToolCallID: "call", Content: "tool result"}},
	}
	got := SelectableMessages(records)
	if len(got) != 1 || got[0].Seq != 1 || got[0].Content != "first" {
		t.Fatalf("SelectableMessages() = %#v, want first only", got)
	}
}

func TestMessagesAfterAreOnlySynthetic(t *testing.T) {
	records := []session.MessageRecord{
		{Message: schema.Message{Role: schema.RoleUser, Content: "prompt"}},
		{Kind: "progress"},
		{Message: schema.Message{Role: schema.RoleUser, ToolCallID: "call", Content: ""}},
	}
	if !MessagesAfterAreOnlySynthetic(records, 0) {
		t.Fatalf("MessagesAfterAreOnlySynthetic() = false, want true")
	}
	records = append(records, session.MessageRecord{Message: schema.Message{Role: schema.RoleAssistant, Content: "real answer"}})
	if MessagesAfterAreOnlySynthetic(records, 0) {
		t.Fatalf("MessagesAfterAreOnlySynthetic() = true with meaningful assistant content")
	}
}
