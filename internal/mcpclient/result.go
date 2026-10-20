package mcpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Zts0hg/foxharness/internal/media"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

func convertResult(result *mcp.CallToolResult) toolprotocol.ExecutionResult {
	out := toolprotocol.ExecutionResult{Failed: result.IsError}
	var text []string
	for _, content := range result.Content {
		part := schema.ContentPart{}
		switch c := content.(type) {
		case *mcp.TextContent:
			part.Type = "text"
			part.Text = c.Text
		case *mcp.ImageContent:
			part = imagePart(c.MIMEType, c.Data)
		case *mcp.ResourceLink:
			part = schema.ContentPart{Type: "resource_link", URI: c.URI, Name: c.Name, MIMEType: c.MIMEType}
		case *mcp.EmbeddedResource:
			if c.Resource == nil {
				part = schema.ContentPart{Type: "text", Text: "unsupported empty embedded resource"}
				break
			}
			r := c.Resource
			if r.Blob == nil {
				part = schema.ContentPart{Type: "text", Text: r.Text, MIMEType: r.MIMEType}
			} else if strings.HasPrefix(r.MIMEType, "image/") {
				part = imagePart(r.MIMEType, r.Blob)
			} else {
				part = schema.ContentPart{Type: "text", Text: "unsupported embedded resource format"}
			}
			part.URI = r.URI
		default:
			part = schema.ContentPart{Type: "text", Text: "unsupported MCP result content format"}
		}
		out.ContentParts = append(out.ContentParts, part)
		if part.Text != "" {
			text = append(text, part.Text)
		}
		if part.Type == "resource_link" {
			text = append(text, fmt.Sprintf("Resource link: %s (%s)", part.URI, part.Name))
		}
	}
	if result.StructuredContent != nil {
		data, err := json.Marshal(result.StructuredContent)
		if err != nil {
			text = append(text, "unsupported structured result content")
		} else {
			out.StructuredContent = data
			text = append(text, string(data))
		}
	}
	if result.IsError && len(out.ContentParts) == 0 && len(out.StructuredContent) == 0 {
		notice := "MCP tool reported an error without details"
		out.ContentParts = []schema.ContentPart{{Type: "text", Text: notice}}
		text = append(text, notice)
	}
	out.Output = strings.Join(text, "\n")
	return out
}

func imagePart(mime string, data []byte) schema.ContentPart {
	if len(data) == 0 || len(data) > media.MaxImageBytes {
		return schema.ContentPart{Type: "text", Text: "invalid image: empty or oversized payload"}
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || "image/"+format != mime || (mime != "image/png" && mime != "image/jpeg") || int64(config.Width)*int64(config.Height) > 100_000_000 {
		return schema.ContentPart{Type: "text", Text: "invalid image or unsupported image format"}
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return schema.ContentPart{Type: "text", Text: "invalid image payload"}
	}
	return schema.ContentPart{Type: "image", MIMEType: mime, Data: append([]byte(nil), data...)}
}
