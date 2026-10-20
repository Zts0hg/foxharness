package mcpclient

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"strings"
	"testing"
)

func TestTextResourcesPreservePayloadAndMIME(t *testing.T) {
	for _, tc := range []struct{ mime, text string }{
		{"application/json", `{"answer":42}`},
		{"application/xml", `<answer>42</answer>`},
		{"application/javascript", `const answer = 42;`},
		{"image/svg+xml", `<svg xmlns="http://www.w3.org/2000/svg"/>`},
		{"application/json", ""},
		{"text/plain", "ordinary text"},
	} {
		t.Run(tc.mime+"/"+tc.text, func(t *testing.T) {
			r := convertResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "test://value", MIMEType: tc.mime, Text: tc.text}}}})
			if r.Failed || r.Output != tc.text || len(r.ContentParts) != 1 {
				t.Fatalf("text resource lost: %+v", r)
			}
			part := r.ContentParts[0]
			if part.Type != "text" || part.Text != tc.text || part.URI != "test://value" || part.MIMEType != tc.mime || len(part.Data) != 0 {
				t.Fatalf("text resource metadata lost: %+v", part)
			}
		})
	}
	r := convertResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{URI: "test://binary", MIMEType: "application/octet-stream", Blob: []byte{1, 2, 3}}}}})
	if !strings.Contains(r.Output, "unsupported") || r.ContentParts[0].URI != "test://binary" {
		t.Fatal("unsupported binary was not explicit")
	}
}
