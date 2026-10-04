package modelinvoke

import (
	"fmt"
	"github.com/Zts0hg/foxharness/internal/schema"
	"strings"
)

func (i *runInvoker) projectImages(messages []schema.Message, disabled bool) ([]schema.Message, string, bool) {
	var notices []string
	hasImages := false
	for index := range messages {
		msg := &messages[index]
		parts := make([]schema.ContentPart, 0, len(msg.ContentParts))
		for _, part := range msg.ContentParts {
			if part.Type != "image" {
				parts = append(parts, part)
				continue
			}
			notice := ""
			if disabled {
				notice = "Visual analysis unavailable: model image input is disabled. Screenshot retained: " + part.ArtifactKey
			} else if len(part.Data) == 0 {
				var err error
				if i.config.ResolveImage == nil {
					err = fmt.Errorf("image resolver is unavailable")
				} else {
					part.Data, err = i.config.ResolveImage(part)
				}
				if err != nil || len(part.Data) == 0 {
					notice = "image unavailable: " + part.ArtifactKey
				}
			}
			if notice != "" {
				msg.Content += "\n" + notice
				notices = append(notices, notice)
				continue
			}
			hasImages = true
			parts = append(parts, part)
		}
		msg.ContentParts = parts
	}
	return messages, strings.Join(notices, "\n"), hasImages
}
