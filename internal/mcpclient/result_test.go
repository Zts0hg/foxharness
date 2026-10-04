package mcpclient

import (
	"bytes"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestEmptyToolErrorRetainsRichErrorContent(t *testing.T) {
	r := convertResult(&mcp.CallToolResult{IsError: true})
	if !r.Failed || len(r.ContentParts) != 1 || r.ContentParts[0].Type != "text" || r.Output == "" || r.ContentParts[0].Text != r.Output {
		t.Fatalf("empty MCP failure would lose persisted error flag: %+v", r)
	}
}

func TestRichConversionPreservesOrderStructuredDataAndErrors(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	source := &mcp.CallToolResult{IsError: true, StructuredContent: map[string]any{"nested": []any{true}}, Content: []mcp.Content{
		&mcp.TextContent{Text: "first"}, &mcp.ImageContent{MIMEType: "image/png", Data: imageData.Bytes()},
		&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "test://text", MIMEType: "text/plain", Text: "embedded"}},
		&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "test://image", MIMEType: "image/png", Blob: imageData.Bytes()}},
		&mcp.ResourceLink{URI: "test://linked", Name: "linked", MIMEType: "image/png"},
		&mcp.AudioContent{MIMEType: "audio/wav", Data: []byte{1}},
		&mcp.ImageContent{MIMEType: "image/png", Data: []byte("broken")},
	}}
	got := convertResult(source)
	if !got.Failed || len(got.ContentParts) != 7 || got.ContentParts[0].Text != "first" || got.ContentParts[1].Type != "image" || got.ContentParts[2].URI != "test://text" || got.ContentParts[3].URI != "test://image" || got.ContentParts[4].URI != "test://linked" {
		t.Fatalf("lost content order/correlation: %#v", got)
	}
	if !strings.Contains(got.Output, "unsupported") || !strings.Contains(got.Output, "invalid image") || !strings.Contains(string(got.StructuredContent), "nested") {
		t.Fatalf("silent loss: %#v", got)
	}
	source.Content[1].(*mcp.ImageContent).Data[0] = 'X'
	if got.ContentParts[1].Data[0] == 'X' {
		t.Fatal("result aliases server bytes")
	}
}
func TestUnsupportedImageFormatsAndOversizedPayloadsAreExplicit(t *testing.T) {
	for _, tc := range []struct {
		mime string
		data []byte
	}{{"image/gif", []byte("GIF89a")}, {"image/png", make([]byte, 20*1024*1024+1)}, {"image/jpeg", nil}} {
		got := imagePart(tc.mime, tc.data)
		if got.Type != "text" || !strings.Contains(got.Text, "image") || len(got.Data) != 0 {
			t.Fatalf("unsupported image passed: %#v", got)
		}
	}
}
