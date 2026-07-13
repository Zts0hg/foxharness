package permission

import (
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

const evidenceBudget = 12000

// Evidence is the bounded, trust-labeled context supplied to the reviewer.
type Evidence struct {
	Text string
}

// BuildEvidence creates a compact reviewer context with explicit trust labels.
func BuildEvidence(messages []schema.Message, projectInstructions []string, request Request) Evidence {
	var b strings.Builder
	b.WriteString("[trusted request]\n")
	b.WriteString(fmt.Sprintf("tool=%s action=%s cwd=%s workspace=%s source=%s\n", request.ToolName, request.Action, request.CWD, request.Workspace, request.Source))
	for _, instruction := range projectInstructions {
		writeBounded(&b, "[trusted project instruction]\n"+instruction+"\n", evidenceBudget)
	}
	if len(messages) > 0 {
		first := messages[0]
		if first.Role == schema.RoleUser {
			writeBounded(&b, "[trusted initial user]\n"+first.Content+"\n", evidenceBudget)
		}
	}
	start := len(messages) - 6
	if start < 0 {
		start = 0
	}
	for _, msg := range messages[start:] {
		label := "[untrusted conversation]"
		if msg.Role == schema.RoleUser && msg.ToolCallID == "" {
			label = "[trusted user]"
		}
		writeBounded(&b, label+"\n"+msg.Content+"\n", evidenceBudget)
	}
	return Evidence{Text: b.String()}
}

func writeBounded(b *strings.Builder, text string, limit int) {
	if b.Len() >= limit {
		return
	}
	remaining := limit - b.Len()
	if len(text) <= remaining {
		b.WriteString(text)
		return
	}
	b.WriteString(text[:remaining])
	b.WriteString("\n[truncated]\n")
}
