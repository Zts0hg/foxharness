package autodev

import (
	"net/url"
	"strings"
)

func stdoutResult(output string) CommandResult { return CommandResult{Stdout: output} }

func markerFromSearchArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	decoded, _ := url.QueryUnescape(args[len(args)-1])
	start := strings.Index(decoded, "<!-- fox-autodev-item-id:")
	if start < 0 {
		return ""
	}
	end := strings.Index(decoded[start:], "-->")
	if end < 0 {
		return ""
	}
	return decoded[start : start+end+3]
}
