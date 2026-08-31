package permission

import (
	"encoding/json"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

const (
	trustedEvidenceBudget   = 16 * 1024
	untrustedEvidenceBudget = 8 * 1024
	untrustedEvidenceLabel  = "[untrusted evidence]\n"
)

// Evidence is the bounded, trust-labeled context supplied to the reviewer.
type Evidence struct {
	Text        string
	Trusted     string
	Untrusted   string
	Correlation EvidenceCorrelation
}

// EvidenceCorrelation identifies the runtime lineage authorized by one
// permission review. Empty fields are valid for root requests whose runtime
// adapter does not expose child lineage.
type EvidenceCorrelation struct {
	ParentSessionID string
	ParentRunID     string
	ChildSessionID  string
	ChildRunID      string
	DelegationID    string
	ToolCallID      string
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
	header := "[request facts; not authorization]\n" + requestFactsJSON(request) + "\n"
	if len(header) > trustedEvidenceBudget {
		const marker = "\n[truncated request facts]\n"
		header = truncateUTF8(header, trustedEvidenceBudget-len(marker)) + marker
	}
	remaining := trustedEvidenceBudget - len(header)
	if remaining < 0 {
		remaining = 0
	}
	trusted := header + recentBounded(trustedChunks, remaining, "[trusted truncated content]\n")
	untrustedBody := recentBounded(untrustedChunks, untrustedEvidenceBudget-len(untrustedEvidenceLabel), "[untrusted truncated content]\n")
	untrusted := ""
	if untrustedBody != "" {
		untrusted = untrustedEvidenceLabel + untrustedBody
	}
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

func truncateUTF8Tail(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	start := len(text) - limit
	for start < len(text) && text[start]&0xc0 == 0x80 {
		start++
	}
	return text[start:]
}

func classifyMessages(messages []schema.Message, trustDirectUsers bool, untrustedOverride string) (trusted, untrusted []string) {
	pendingCalls := make(map[string]string)
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.ID != "" {
				pendingCalls[call.ID] = call.Name
			}
		}
		callName, matchedCall := "", false
		if message.ToolCallID != "" {
			callName, matchedCall = pendingCalls[message.ToolCallID]
		}
		if matchedCall {
			delete(pendingCalls, message.ToolCallID)
		}
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		switch {
		case message.Role == schema.RoleUser && message.ToolCallID != "" && matchedCall && isAskUserQuestion(callName):
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

func isAskUserQuestion(name string) bool {
	return name == "ask_user_question" || name == "AskUserQuestion"
}

func requestFactsJSON(request Request) string {
	facts := struct {
		Tool              string   `json:"tool"`
		Action            string   `json:"action"`
		Effects           []string `json:"effects"`
		Scope             string   `json:"scope"`
		ReadOnly          bool     `json:"read_only"`
		NestedEnforcement bool     `json:"nested_enforcement"`
		PlannedCommands   []string `json:"planned_commands"`
		CWD               string   `json:"cwd"`
		Workspace         string   `json:"workspace"`
		Source            string   `json:"source"`
	}{
		Tool:              request.ToolName,
		Action:            request.Action,
		Scope:             string(request.Capabilities.Scope),
		ReadOnly:          request.Capabilities.ReadOnly,
		NestedEnforcement: request.Capabilities.NestedEnforcement,
		PlannedCommands:   append([]string(nil), request.Capabilities.Commands...),
		CWD:               request.CWD,
		Workspace:         request.Workspace,
		Source:            string(request.Source),
	}
	for _, effect := range request.Capabilities.Effects {
		facts.Effects = append(facts.Effects, string(effect))
	}
	encoded, err := json.Marshal(facts)
	if err != nil {
		return `{}`
	}
	return string(encoded)
}

func isGeneratedContext(content string) bool {
	content = strings.TrimSpace(content)
	return strings.HasPrefix(content, "## Compacted Context Summary") ||
		strings.HasPrefix(content, "[Runtime System Notice]") ||
		strings.HasPrefix(content, "[Runtime System Reminder]")
}

func recentBounded(chunks []string, budget int, truncatedLabel string) string {
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
			prefix := marker + truncatedLabel
			if remaining <= len(prefix) {
				selected = append([]string{truncateUTF8(prefix, remaining)}, selected...)
			} else {
				selected = append([]string{prefix + truncateUTF8Tail(chunk, remaining-len(prefix))}, selected...)
			}
			remaining = 0
			break
		}
		selected = append([]string{chunk}, selected...)
		remaining -= len(chunk)
	}
	return strings.Join(selected, "")
}
