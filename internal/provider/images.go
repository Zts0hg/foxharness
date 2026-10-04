package provider

import (
	"errors"
	"net/http"
	"strings"
)

/* ErrImageInputUnsupported identifies a verified image capability rejection. */
var ErrImageInputUnsupported = errors.New("model does not support image input")

/* ImageInput returns the configured model's image-input policy. */
func (p *OpenAIProvider) ImageInput() string { return p.imageInput }

/* ImageInput returns the configured model's image-input policy. */
func (p *ClaudeProvider) ImageInput() string { return p.imageInput }

/* IsImageInputUnsupported accepts explicit capability rejections, not network or authentication errors. */
func IsImageInputUnsupported(err error) bool {
	if errors.Is(err, ErrImageInputUnsupported) {
		return true
	}
	code, ok := errorStatusCode(err)
	if !ok || code != http.StatusBadRequest {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, pattern := range []string{"image input is not supported", "does not support image input", "image_url is only supported by certain models", "does not support vision", "does not support images"} {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}
