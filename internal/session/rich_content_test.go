package session

import (
	"encoding/json"
	"github.com/Zts0hg/foxharness/internal/schema"
	"path/filepath"
	"testing"
)

func TestMessageLogPersistsRichContentAndReturnsOwnedRecord(t *testing.T) {
	log := &MessageLog{path: filepath.Join(t.TempDir(), "messages.jsonl")}
	msg := schema.Message{Role: schema.RoleUser, ToolCallID: "screenshot", IsError: true,
		ContentParts: []schema.ContentPart{{Type: "image", ArtifactKey: "media/hash.png", MIMEType: "image/png"}}, StructuredContent: json.RawMessage(`{"nested":[1]}`)}
	record, err := log.appendMessageRecord("run", MessageKindNormal, msg, "")
	if err != nil {
		t.Fatal(err)
	}
	msg.ContentParts[0].ArtifactKey = "changed"
	msg.StructuredContent[0] = 'X'
	loaded, err := log.LoadMessages()
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []schema.Message{record.Message, loaded[0]} {
		if got.ToolCallID != "screenshot" || !got.IsError || got.ContentParts[0].ArtifactKey != "media/hash.png" || !json.Valid(got.StructuredContent) {
			t.Fatalf("lost or aliased content: %#v", got)
		}
	}
}
