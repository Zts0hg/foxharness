package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRichMessageCloneAndCompatibleJSON(t *testing.T) {
	plain, err := json.Marshal(Message{Role: RoleUser, Content: "old"})
	if err != nil || string(plain) != `{"role":"user","content":"old"}` {
		t.Fatalf("old encoding changed: %s, %v", plain, err)
	}
	msg := Message{Role: RoleUser, ToolCallID: "call", IsError: true,
		ContentParts:      []ContentPart{{Type: "text", Text: "first"}, {Type: "image", MIMEType: "image/png", ArtifactKey: "media/hash.png", Data: []byte("secret-bytes")}},
		StructuredContent: json.RawMessage(`{"nested":{"value":1}}`)}
	copy := msg.Clone()
	msg.ContentParts[0].Text = "changed"
	msg.ContentParts[1].Data[0] = 'X'
	msg.StructuredContent[0] = 'X'
	if copy.ContentParts[0].Text != "first" || string(copy.ContentParts[1].Data) != "secret-bytes" || !json.Valid(copy.StructuredContent) {
		t.Fatal("clone shares mutable rich content")
	}
	encoded, err := json.Marshal(copy)
	if err != nil || strings.Contains(string(encoded), "secret-bytes") || strings.Contains(string(encoded), "c2VjcmV0") {
		t.Fatalf("transient data persisted: %s, %v", encoded, err)
	}
	var loaded Message
	if err := json.Unmarshal(encoded, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.ToolCallID != "call" || !loaded.IsError || len(loaded.ContentParts) != 2 || loaded.ContentParts[1].ArtifactKey != "media/hash.png" || string(loaded.StructuredContent) != string(copy.StructuredContent) {
		t.Fatalf("lost rich content: %#v", loaded)
	}
}
