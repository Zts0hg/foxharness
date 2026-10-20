package modelinvoke

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type imageProvider struct {
	mode    string
	calls   [][]schema.Message
	failure error
}

func (p *imageProvider) ImageInput() string { return p.mode }
func (p *imageProvider) Generate(_ context.Context, m []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls = append(p.calls, cloneMessages(m))
	if len(p.calls) == 1 && p.failure != nil {
		return nil, p.failure
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "answer"}}, nil
}

func TestImageHydrationFallbackAndMissingArtifacts(t *testing.T) {
	for _, tc := range []struct {
		name, mode                       string
		failure                          error
		missing                          bool
		wantCalls                        int
		wantImage, wantNotice, wantError bool
	}{
		{"enabled", "enabled", nil, false, 1, true, false, false},
		{"disabled", "disabled", nil, false, 1, false, true, false},
		{"auto fallback", "auto", provider.ErrImageInputUnsupported, false, 2, true, true, false},
		{"generic error", "auto", errors.New("network error"), false, 1, true, false, true},
		{"enabled rejection", "enabled", provider.ErrImageInputUnsupported, false, 1, true, false, true},
		{"missing", "auto", nil, true, 1, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &imageProvider{mode: tc.mode, failure: tc.failure}
			request := engine.RunContext{Messages: []schema.Message{{Role: schema.RoleUser, ToolCallID: "shot", Content: "text snapshot", ContentParts: []schema.ContentPart{{Type: "image", MIMEType: "image/png", ArtifactKey: "media/hash.png"}}}}}
			run, err := New(p, Config{ResolveImage: func(schema.ContentPart) ([]byte, error) {
				if tc.missing {
					return nil, errors.New("image unavailable: missing")
				}
				return []byte("image bytes"), nil
			}}).StartRun(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			result, err := run.Invoke(context.Background(), request, nil)
			if (err != nil) != tc.wantError || len(p.calls) != tc.wantCalls {
				t.Fatalf("calls=%d err=%v", len(p.calls), err)
			}
			if len(request.Messages[0].ContentParts[0].Data) != 0 || request.Messages[0].ContentParts[0].ArtifactKey != "media/hash.png" {
				t.Fatal("hydration mutated history")
			}
			hasImage := false
			for _, part := range p.calls[0][0].ContentParts {
				hasImage = hasImage || len(part.Data) > 0
			}
			if hasImage != tc.wantImage {
				t.Fatalf("image=%v", hasImage)
			}
			if tc.wantNotice && (!strings.Contains(result.Message.Content, "image") || !strings.Contains(p.calls[len(p.calls)-1][0].Content, "text snapshot")) {
				t.Fatalf("missing limitation/text: %#v", result)
			}
			if tc.wantCalls == 2 {
				for _, part := range p.calls[1][0].ContentParts {
					if len(part.Data) > 0 {
						t.Fatal("fallback still includes image")
					}
				}
			}
		})
	}
}
