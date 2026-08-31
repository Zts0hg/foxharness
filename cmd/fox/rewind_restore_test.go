package main

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* TestConversationContentRestoresTrimmedInput pins the baseline rewind
 * behavior: the input restored into the composer is trimmed, so trailing
 * newlines from persisted prompts do not reach the input field. */
func TestConversationContentRestoresTrimmedInput(t *testing.T) {
	records := []session.MessageRecord{
		{Seq: 0, DisplayContent: "  first prompt  ", Message: schema.Message{Role: schema.RoleUser, Content: "first prompt"}},
		{Seq: 1, Message: schema.Message{Role: schema.RoleUser, Content: "second prompt\n"}},
	}
	if got := conversationContent(records, 0); got != "first prompt" {
		t.Fatalf("restored display input = %q, want the trimmed prompt", got)
	}
	if got := conversationContent(records, 1); got != "second prompt" || strings.HasSuffix(got, "\n") {
		t.Fatalf("restored input = %q, want the trimmed prompt", got)
	}
}
