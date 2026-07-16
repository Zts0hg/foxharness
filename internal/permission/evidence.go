package permission

import (
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

const (
	trustedEvidenceBudget   = 16 * 1024
	untrustedEvidenceBudget = 8 * 1024
)

// Evidence is the bounded, trust-labeled context supplied to the reviewer.
type Evidence struct {
	Text      string
	Trusted   string
	Untrusted string
}

// BuildEvidence creates main-session reviewer context with explicit trust
// labels. Direct user messages and ask_user_question answers are trusted.
func BuildEvidence(messages []schema.Message, projectInstructions []string, request Request) Evidence {
	trusted := make([]string, 0, len(projectInstructions)+len(messages))
	for _, instruction := range projectInstructions {
		if strings.TrimSpace(instruction) != "" {
			trusted = append(trusted, "[trusted project instruction]\n"+instruction+"\n")
		}
	}
	messageTrusted, untrusted := classifyMessages(messages, true, "")
	trusted = append(trusted, messageTrusted...)
	return composeEvidence(request, trusted, untrusted)
}

// BuildChildEvidence preserves the parent's original labels while treating
// child-generated task and execution context as untrusted. Explicit answers
// returned through ask_user_question remain trusted.
func BuildChildEvidence(parent Evidence, childMessages []schema.Message, request Request) Evidence {
	childTrusted, childUntrusted := classifyMessages(childMessages, false, "[untrusted child context]")
	trusted := []string{parent.Trusted}
	trusted = append(trusted, childTrusted...)
	untrusted := []string{parent.Untrusted}
	untrusted = append(untrusted, childUntrusted...)
	return composeEvidence(request, trusted, untrusted)
}

func composeEvidence(request Request, trustedChunks, untrustedChunks []string) Evidence {
	header := fmt.Sprintf("[trusted request facts]\ntool=%s action=%s cwd=%s workspace=%s source=%s\n", request.ToolName, request.Action, request.CWD, request.Workspace, request.Source)
	if len(header) > trustedEvidenceBudget {
		const marker = "\n[truncated request facts]\n"
		header = truncateUTF8(header, trustedEvidenceBudget-len(marker)) + marker
	}
	remaining := trustedEvidenceBudget - len(header)
	if remaining < 0 {
		remaining = 0
	}
	trusted := header + recentBounded(trustedChunks, remaining)
	untrusted := recentBounded(untrustedChunks, untrustedEvidenceBudget)
	text := trusted
	if untrusted != "" {
		text += "\n" + untrusted
	}
	return Evidence{Text: text, Trusted: trusted, Untrusted: untrusted}
}

func truncateUTF8(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && text[cut]&0xc0 == 0x80 {
		cut--
	}
	return text[:cut]
}

func classifyMessages(messages []schema.Message, trustDirectUsers bool, untrustedOverride string) (trusted, untrusted []string) {
	trustedAnswers := make(map[string]bool)
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.Name == "ask_user_question" || call.Name == "AskUserQuestion" {
				trustedAnswers[call.ID] = true
			}
		}
	}
	for _, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		switch {
		case message.Role == schema.RoleUser && message.ToolCallID != "" && trustedAnswers[message.ToolCallID]:
			trusted = append(trusted, "[trusted user answer]\n"+message.Content+"\n")
		case message.Role == schema.RoleUser && message.ToolCallID == "" && isGeneratedContext(message.Content):
			untrusted = append(untrusted, "[untrusted generated context]\n"+message.Content+"\n")
		case trustDirectUsers && message.Role == schema.RoleUser && message.ToolCallID == "":
			trusted = append(trusted, "[trusted user]\n"+message.Content+"\n")
		default:
			label := untrustedOverride
			if label == "" {
				switch {
				case message.ToolCallID != "":
					label = "[untrusted tool result]"
				case message.Role == schema.RoleAssistant:
					label = "[untrusted assistant]"
				default:
					label = "[untrusted conversation]"
				}
			}
			untrusted = append(untrusted, label+"\n"+message.Content+"\n")
		}
	}
	return trusted, untrusted
}

func isGeneratedContext(content string) bool {
	content = strings.TrimSpace(content)
	return strings.HasPrefix(content, "## Compacted Context Summary") ||
		strings.HasPrefix(content, "[Runtime System Notice]") ||
		strings.HasPrefix(content, "[Runtime System Reminder]")
}

func recentBounded(chunks []string, budget int) string {
	if budget <= 0 {
		return ""
	}
	selected := make([]string, 0, len(chunks))
	remaining := budget
	for i := len(chunks) - 1; i >= 0 && remaining > 0; i-- {
		chunk := chunks[i]
		if chunk == "" {
			continue
		}
		if len(chunk) > remaining {
			const marker = "[truncated older content]\n"
			if remaining <= len(marker) {
				selected = append([]string{marker[:remaining]}, selected...)
			} else {
				selected = append([]string{marker + chunk[len(chunk)-(remaining-len(marker)):]}, selected...)
			}
			remaining = 0
			break
		}
		selected = append([]string{chunk}, selected...)
		remaining -= len(chunk)
	}
	return strings.Join(selected, "")
}
