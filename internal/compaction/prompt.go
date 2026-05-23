package compaction

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// NoToolsPreamble is the leading directive prepended to compaction prompts to
// prevent the LLM from invoking tools during summarization.
const NoToolsPreamble = "CRITICAL: Respond with TEXT ONLY. Do NOT call any tools."

// NoToolsTrailer is the closing directive appended to compaction prompts to
// reinforce the no-tool-use constraint at the end of the message.
const NoToolsTrailer = "REMINDER: Do NOT call any tools."

// LanguageEnglish is the language code used when summarization output should
// be produced in English.
const LanguageEnglish = "en"

// LanguageChinese is the language code used when summarization output should
// be produced in Chinese (zh-CN).
const LanguageChinese = "zh"

// DetectSummaryLanguage examines the first user message in the conversation
// and returns LanguageChinese if it contains any CJK ideograph, otherwise
// LanguageEnglish. Empty input falls back to English.
func DetectSummaryLanguage(messages []schema.Message) string {
	for _, msg := range messages {
		if msg.Role != schema.RoleUser {
			continue
		}
		if msg.ToolCallID != "" {
			continue
		}
		if containsCJK(msg.Content) {
			return LanguageChinese
		}
		return LanguageEnglish
	}
	return LanguageEnglish
}

// BuildCompactPrompt constructs the structured 9-section summary prompt for
// the supplied messages and target language. The prompt always includes
// NoToolsPreamble and NoToolsTrailer so the LLM cannot escape the no-tool
// constraint at the protocol level.
func BuildCompactPrompt(messages []schema.Message, language string) string {
	body := renderMessagesForSummary(messages)
	languageName := "English"
	if language == LanguageChinese {
		languageName = "Chinese (中文)"
	}

	template := `%s

You are summarizing the conversation below so the agent can continue working
after the older messages are dropped. Produce the summary in %s.

<analysis>
[Use this area to draft your thinking. Everything between these tags is
discarded — only the <summary> block is preserved.]
</analysis>

<summary>
1. Primary Request and Intent: [User's complete request and goals]
2. Key Technical Concepts: [Frameworks, APIs, patterns relevant to the task]
3. Files and Code Sections: [File paths, code snippets, reasons for changes]
4. Errors and Fixes: [Error details, root causes, and fixes applied]
5. Problem Solving: [Solved problems and ongoing debugging]
6. All User Messages: [Non-tool-result user messages, summarized]
7. Pending Tasks: [Incomplete items from the user's request]
8. Current Work: [Precise description of what was being done last]
9. Optional Next Step: [Recommended next action with rationale]
</summary>

Conversation history follows:

%s

%s
`
	return fmt.Sprintf(template, NoToolsPreamble, languageName, body, NoToolsTrailer)
}

// FormatSummary strips the <analysis> draft block and extracts the content of
// the <summary> tags. When neither tag is present the input is returned
// unchanged so a sloppy LLM response is still usable.
func FormatSummary(raw string) string {
	stripped := stripTag(raw, "analysis")
	if extracted, ok := extractTag(stripped, "summary"); ok {
		return strings.TrimSpace(extracted)
	}
	return strings.TrimSpace(stripped)
}

func stripTag(content, tag string) string {
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	for {
		start := strings.Index(content, openTag)
		if start < 0 {
			return content
		}
		end := strings.Index(content[start:], closeTag)
		if end < 0 {
			return content[:start]
		}
		end += start + len(closeTag)
		content = content[:start] + content[end:]
	}
}

func extractTag(content, tag string) (string, bool) {
	openTag := "<" + tag + ">"
	closeTag := "</" + tag + ">"
	start := strings.Index(content, openTag)
	if start < 0 {
		return "", false
	}
	rest := content[start+len(openTag):]
	end := strings.Index(rest, closeTag)
	if end < 0 {
		return rest, true
	}
	return rest[:end], true
}

func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}
