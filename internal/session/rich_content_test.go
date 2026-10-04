package session

import (
	"bytes"
	"encoding/json"
	"github.com/Zts0hg/foxharness/internal/media"
	"github.com/Zts0hg/foxharness/internal/schema"
	"image"
	"image/png"
	"path/filepath"
	"testing"
)

func TestImageReplayAfterMessageRewindKeepsArtifact(t *testing.T) {
	dir := t.TempDir()
	store := media.NewStore(dir)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	part, err := store.Put("image/png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	log := &MessageLog{path: filepath.Join(dir, "messages.jsonl")}
	message := schema.Message{Role: schema.RoleUser, ToolCallID: "shot", ContentParts: []schema.ContentPart{part}}
	for i := 0; i < 2; i++ {
		if _, err := log.Append("run", message); err != nil {
			t.Fatal(err)
		}
	}
	if err := log.TruncateBeforeSeq(1); err != nil {
		t.Fatal(err)
	}
	loaded, err := log.LoadMessages()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("rewind messages: %d", len(loaded))
	}
	got, err := store.Resolve(loaded[0].ContentParts[0])
	if err != nil || !bytes.Equal(got, data.Bytes()) {
		t.Fatalf("artifact lost: %v", err)
	}
	seq, err := log.Append("restored", message)
	if err != nil || seq != 1 {
		t.Fatalf("restore sequence: %d %v", seq, err)
	}
}

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
