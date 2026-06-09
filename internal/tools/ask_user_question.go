package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// ErrUserCancelled is returned by a UserAsker when the user dismisses the
// question prompt without providing answers. The tool maps it to a clear
// cancellation result rather than an error.
var ErrUserCancelled = errors.New("user cancelled the question prompt")

// askUserQuestionToolName is the registered name of the tool. It uses
// snake_case to match the other built-in tools (read_file, write_file, ...).
const askUserQuestionToolName = "ask_user_question"

// maxResultSizeChars caps the formatted result returned to the LLM, mirroring
// the reference tool's maxResultSizeChars of 100,000.
const maxResultSizeChars = 100_000

// Question is a single multiple-choice question presented to the user.
type Question struct {
	// Header is a short chip-style label. The ~12-character limit is advisory
	// guidance surfaced to the LLM, not enforced by validation.
	Header string
	// Prompt is the full question text. It also serves as the key that maps a
	// question to its answer and annotations.
	Prompt string
	// Options are the 2–4 choices offered for this question.
	Options []Option
	// MultiSelect allows the user to pick more than one option when true.
	MultiSelect bool
}

// Option is a single selectable choice within a Question.
type Option struct {
	// Label is the concise display text and the value reported when selected.
	Label string
	// Description explains the option's meaning or trade-offs.
	Description string
	// Preview is optional content (code, mockup) shown when the option is
	// focused. Intended for single-select questions.
	Preview string
}

// Answer is the user's response to one Question.
type Answer struct {
	// QuestionText matches the originating Question.Prompt exactly.
	QuestionText string
	// Value is the selected option label, or comma-joined labels for a
	// multi-select question, or free text when the user chose "Other".
	Value string
	// Preview is the selected option's preview content, if any.
	Preview string
	// Notes is free-text the user attached to the selection, if any.
	Notes string
}

// UserAsker collects answers to a set of questions from a human. The interactive
// TUI provides a real implementation; non-interactive runners leave it absent so
// the tool is never registered (the reference's isEnabled() analog). Defining the
// interface here keeps the tool free of any TUI dependency.
type UserAsker interface {
	// Ask presents the questions to the user and blocks until they are answered
	// or the context is cancelled. It returns one Answer per question (in the
	// same order), ErrUserCancelled if the user dismisses the prompt, or the
	// context error if cancelled.
	Ask(ctx context.Context, questions []Question) ([]Answer, error)
}

// AskUserQuestionTool lets the core LLM ask the user multiple-choice questions to
// gather preferences, clarify ambiguity, or confirm a direction. It is
// semantically read-only (it performs no workspace or system mutations) and is
// not parallel-safe: its Execute blocks on a single interactive surface, so it
// intentionally does not implement ParallelSafeTool.
type AskUserQuestionTool struct {
	asker UserAsker
}

// NewAskUserQuestionTool creates the tool backed by the given asker. The asker
// may be nil; in that case Execute returns a non-blocking message indicating no
// interactive user is available (callers normally register the tool only when an
// asker is present).
func NewAskUserQuestionTool(asker UserAsker) *AskUserQuestionTool {
	return &AskUserQuestionTool{asker: asker}
}

// Name returns the registered tool name.
func (t *AskUserQuestionTool) Name() string {
	return askUserQuestionToolName
}

// Definition returns the tool schema advertised to the LLM, including usage
// guidance adapted from the reference tool.
func (t *AskUserQuestionTool) Definition() schema.ToolDefinition {
	optionSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Concise display text for the choice (1-5 words).",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "Explanation of what this option means or its trade-offs.",
			},
			"preview": map[string]interface{}{
				"type":        "string",
				"description": "Optional preview content (code, mockup) shown when this option is focused. Single-select questions only.",
			},
		},
		"required": []string{"label", "description"},
	}

	questionSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"question": map[string]interface{}{
				"type":        "string",
				"description": "The complete question to ask, ending with a question mark.",
			},
			"header": map[string]interface{}{
				"type":        "string",
				"description": "Very short chip label (about 12 characters), e.g. \"Auth method\".",
			},
			"options": map[string]interface{}{
				"type":        "array",
				"minItems":    2,
				"maxItems":    4,
				"items":       optionSchema,
				"description": "The 2-4 distinct choices. Do NOT add an \"Other\" option; it is appended automatically.",
			},
			"multiSelect": map[string]interface{}{
				"type":        "boolean",
				"description": "Set true to allow selecting multiple options for this question.",
			},
		},
		"required": []string{"question", "header", "options"},
	}

	return schema.ToolDefinition{
		Name: t.Name(),
		Description: "Ask the user multiple-choice questions to gather preferences, clarify ambiguity, " +
			"or choose between approaches. Notes: an \"Other\" free-text choice is always added automatically; " +
			"if you recommend an option, put it first and append \"(Recommended)\" to its label. " +
			"Ask 1-4 questions per call, each with 2-4 options.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"questions": map[string]interface{}{
					"type":        "array",
					"minItems":    1,
					"maxItems":    4,
					"items":       questionSchema,
					"description": "The 1-4 questions to ask.",
				},
				"answers": map[string]interface{}{
					"type":        "object",
					"description": "Optional pre-collected answers keyed by exact question text; when present the tool uses them directly without prompting.",
				},
				"annotations": map[string]interface{}{
					"type":        "object",
					"description": "Optional per-question annotations keyed by exact question text, each with optional preview/notes.",
				},
				"metadata": map[string]interface{}{
					"type":        "object",
					"description": "Optional metadata (e.g. a source identifier); not shown to the user.",
				},
			},
			"required": []string{"questions"},
		},
	}
}

// askInput is the decoded wire form of the tool arguments.
type askInput struct {
	Questions []struct {
		Question string `json:"question"`
		Header   string `json:"header"`
		Options  []struct {
			Label       string `json:"label"`
			Description string `json:"description"`
			Preview     string `json:"preview"`
		} `json:"options"`
		MultiSelect bool `json:"multiSelect"`
	} `json:"questions"`
	Answers     map[string]string          `json:"answers"`
	Annotations map[string]annotationInput `json:"annotations"`
	Metadata    *struct {
		Source string `json:"source"`
	} `json:"metadata"`
}

// annotationInput is one entry of the optional annotations map.
type annotationInput struct {
	Preview string `json:"preview"`
	Notes   string `json:"notes"`
}

// Execute validates the input, collects answers (from the pre-supplied answers
// map or via the asker), and returns a single formatted result string. It never
// panics on malformed input and honors context cancellation promptly.
func (t *AskUserQuestionTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input askInput
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	questions, err := validateAndConvert(input)
	if err != nil {
		return "", err
	}

	if len(input.Answers) > 0 {
		return formatResult(answersFromInput(questions, input)), nil
	}

	if t.asker == nil {
		return noInteractiveUserMessage, nil
	}

	answers, err := t.asker.Ask(ctx, questions)
	if err != nil {
		switch {
		case errors.Is(err, ErrUserCancelled):
			return userCancelledMessage, nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return promptInterruptedMessage, nil
		default:
			return "", fmt.Errorf("收集用户回答失败: %w", err)
		}
	}
	return formatResult(answers), nil
}

const (
	noInteractiveUserMessage = "No interactive user is available to answer questions in this run mode. " +
		"Proceed using your best judgement based on the information you already have."
	userCancelledMessage = "The user dismissed the questions without answering. " +
		"Proceed using your best judgement or ask again later if needed."
	promptInterruptedMessage = "The question prompt was interrupted before the user answered. " +
		"Proceed using your best judgement based on the information you already have."
)

// validateAndConvert enforces the structural rules (1–4 questions, 2–4 options,
// unique question texts, unique option labels per question) and converts the
// decoded input into []Question. String-length limits are advisory and are not
// enforced here.
func validateAndConvert(input askInput) ([]Question, error) {
	if n := len(input.Questions); n < 1 || n > 4 {
		return nil, fmt.Errorf("questions 数量必须在 1 到 4 之间，收到 %d", n)
	}

	seenQuestions := make(map[string]struct{}, len(input.Questions))
	questions := make([]Question, 0, len(input.Questions))
	for i, q := range input.Questions {
		if _, dup := seenQuestions[q.Question]; dup {
			return nil, fmt.Errorf("问题文本必须唯一，重复: %q", q.Question)
		}
		seenQuestions[q.Question] = struct{}{}

		if n := len(q.Options); n < 2 || n > 4 {
			return nil, fmt.Errorf("第 %d 个问题的 options 数量必须在 2 到 4 之间，收到 %d", i+1, n)
		}

		seenLabels := make(map[string]struct{}, len(q.Options))
		options := make([]Option, 0, len(q.Options))
		for _, o := range q.Options {
			if _, dup := seenLabels[o.Label]; dup {
				return nil, fmt.Errorf("第 %d 个问题的选项 label 必须唯一，重复: %q", i+1, o.Label)
			}
			seenLabels[o.Label] = struct{}{}
			options = append(options, Option{Label: o.Label, Description: o.Description, Preview: o.Preview})
		}

		questions = append(questions, Question{
			Header:      q.Header,
			Prompt:      q.Question,
			Options:     options,
			MultiSelect: q.MultiSelect,
		})
	}
	return questions, nil
}

// answersFromInput builds answers from a pre-supplied answers map, mirroring the
// reference's verbatim Object.entries(answers) behavior: entries are emitted in
// question order for determinism, any answer keys not matching a question are
// appended (sorted), and questions absent from the map are skipped without
// re-prompting.
func answersFromInput(questions []Question, input askInput) []Answer {
	matched := make(map[string]struct{}, len(input.Answers))
	answers := make([]Answer, 0, len(input.Answers))

	for _, q := range questions {
		value, ok := input.Answers[q.Prompt]
		if !ok {
			continue
		}
		matched[q.Prompt] = struct{}{}
		ann := input.Annotations[q.Prompt]
		answers = append(answers, Answer{
			QuestionText: q.Prompt,
			Value:        value,
			Preview:      ann.Preview,
			Notes:        ann.Notes,
		})
	}

	var extraKeys []string
	for key := range input.Answers {
		if _, ok := matched[key]; !ok {
			extraKeys = append(extraKeys, key)
		}
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		ann := input.Annotations[key]
		answers = append(answers, Answer{
			QuestionText: key,
			Value:        input.Answers[key],
			Preview:      ann.Preview,
			Notes:        ann.Notes,
		})
	}
	return answers
}

// formatResult renders the collected answers into the single result string
// returned to the LLM, mirroring the reference's mapToolResultToToolResultBlockParam,
// and truncates to maxResultSizeChars.
func formatResult(answers []Answer) string {
	segments := make([]string, 0, len(answers))
	for _, a := range answers {
		parts := []string{fmt.Sprintf("%q=%q", a.QuestionText, a.Value)}
		if a.Preview != "" {
			parts = append(parts, "selected preview:\n"+a.Preview)
		}
		if a.Notes != "" {
			parts = append(parts, "user notes: "+a.Notes)
		}
		segments = append(segments, strings.Join(parts, " "))
	}

	result := "User has answered your questions: " + strings.Join(segments, ", ") +
		". You can now continue with the user's answers in mind."
	return truncate(result, maxResultSizeChars)
}

// truncate clips s to at most limit bytes, appending a marker when it cuts. The
// cut is moved back to a UTF-8 rune boundary so the result is never invalid.
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	const marker = "\n...[truncated]"
	if limit <= len(marker) {
		return s[:trimToRuneBoundary(s, limit)]
	}
	cut := trimToRuneBoundary(s, limit-len(marker))
	return s[:cut] + marker
}

// trimToRuneBoundary returns the largest index <= n that does not fall inside a
// multi-byte UTF-8 sequence.
func trimToRuneBoundary(s string, n int) int {
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return n
}
