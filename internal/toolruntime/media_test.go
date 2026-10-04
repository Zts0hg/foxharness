package toolruntime

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/media"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolexec"
	"github.com/Zts0hg/foxharness/internal/toolresult"
)

func TestExecutorPersistsImageBeforeReturningReference(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, fail := range []bool{false, true} {
		dir := t.TempDir()
		store := media.NewStore(dir)
		if fail {
			store = media.NewStore(filepath.Join(dir, "missing-session"))
		}
		exec := New([]toolexec.Capability{{Definition: schema.ToolDefinition{Name: "screen"}, Execute: func(context.Context, schema.ToolCall) engine.ToolExecutionResult {
			return engine.ToolExecutionResult{ModelContent: "page snapshot", FullContent: "page snapshot", ContentParts: []schema.ContentPart{{Type: "image", MIMEType: "image/png", Data: data.Bytes()}}}
		}}}, toolresult.OSFileSystem{}, dir).WithImageStore(store)
		snapshot, err := exec.Snapshot(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		batch, err := exec.Execute(context.Background(), snapshot, []schema.ToolCall{{ID: "shot", Name: "screen"}})
		if err != nil {
			t.Fatal(err)
		}
		got := batch.Results[0]
		if fail {
			if !got.IsError || !strings.Contains(got.ModelContent, "image unavailable") || !strings.Contains(got.ModelContent, "page snapshot") {
				t.Fatalf("failure hidden: %#v", got)
			}
			for _, part := range got.ContentParts {
				if part.ArtifactKey != "" || len(part.Data) > 0 {
					t.Fatal("failed store leaked dangling reference/bytes")
				}
			}
		} else {
			if got.IsError || len(got.ContentParts) != 1 || len(got.ContentParts[0].Data) > 0 {
				t.Fatalf("bad stored result: %#v", got)
			}
			loaded, err := store.Resolve(got.ContentParts[0])
			if err != nil || !bytes.Equal(loaded, data.Bytes()) {
				t.Fatalf("image not readable before commit: %v", err)
			}
		}
	}
}
