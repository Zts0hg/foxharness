package runtimecompaction

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/media"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"image"
	"image/png"
	"testing"
)

func TestCompactingImageHistoryKeepsReplayArtifact(t *testing.T) {
	store := media.NewStore(t.TempDir())
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	part, err := store.Put("image/png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	msg := schema.Message{Role: schema.RoleUser, ContentParts: []schema.ContentPart{part}}
	mechanism := &stubMechanism{summary: "textual summary"}
	proposal, err := New(mechanism).Compact(context.Background(), foxruntime.ContextCompactionRequest{Trigger: foxruntime.ContextCompactionManual, Messages: []engine.Message{msg}, Records: []session.MessageRecord{{Seq: 4, Message: msg}}})
	if err != nil || !proposal.Changed || proposal.CompactState.CoveredUntilSeq != 4 {
		t.Fatalf("compaction: %#v %v", proposal, err)
	}
	if got, err := store.Resolve(msg.ContentParts[0]); err != nil || !bytes.Equal(got, data.Bytes()) {
		t.Fatalf("history artifact lost: %v", err)
	}
}

func TestCompactionClonePreservesOwnedImageReferences(t *testing.T) {
	var msg engine.Message
	if err := json.Unmarshal([]byte(`{"role":"user","content_parts":[{"type":"image","artifact_key":"media/hash.png"}],"structured_content":{"nested":[1]}}`), &msg); err != nil {
		t.Fatal(err)
	}
	copy := cloneMessages([]engine.Message{msg})
	msg.ContentParts[0].ArtifactKey = "changed"
	msg.StructuredContent[0] = 'X'
	if copy[0].ContentParts[0].ArtifactKey != "media/hash.png" || !json.Valid(copy[0].StructuredContent) {
		t.Fatal("compaction projection aliases history")
	}
}
