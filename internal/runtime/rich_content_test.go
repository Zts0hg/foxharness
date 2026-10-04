package runtime

import (
	"encoding/json"
	"github.com/Zts0hg/foxharness/internal/engine"
	"testing"
)

func TestContextCloneOwnsRichMessage(t *testing.T) {
	var original engine.Message
	if err := json.Unmarshal([]byte(`{"role":"user","content_parts":[{"type":"text","text":"before"}],"structured_content":{"nested":[1]},"usage":{"input_tokens":2}}`), &original); err != nil {
		t.Fatal(err)
	}
	cloned := cloneContextMessage(original)
	original.ContentParts[0].Text = "after"
	original.StructuredContent[0] = 'X'
	original.Usage.InputTokens = 99
	if cloned.ContentParts[0].Text != "before" || !json.Valid(cloned.StructuredContent) || cloned.Usage.InputTokens != 2 {
		t.Fatal("context clone aliases rich message")
	}
}
