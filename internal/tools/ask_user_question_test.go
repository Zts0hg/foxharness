package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// fakeAsker is a deterministic UserAsker for tests. It records whether it was
// invoked and returns canned answers or a canned error.
type fakeAsker struct {
	invoked      bool
	gotQuestions []Question
	answers      []Answer
	err          error
}

func (f *fakeAsker) Ask(ctx context.Context, questions []Question) ([]Answer, error) {
	f.invoked = true
	f.gotQuestions = questions
	if f.err != nil {
		return nil, f.err
	}
	return f.answers, nil
}

// mustArgs marshals v into json.RawMessage for use as tool arguments.
func mustArgs(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return raw
}

// oneQuestionArgs builds a minimal valid single-question payload.
func oneQuestionArgs() map[string]interface{} {
	return map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"question": "Which database should we use?",
				"header":   "Database",
				"options": []map[string]interface{}{
					{"label": "PostgreSQL", "description": "Relational"},
					{"label": "MongoDB", "description": "Document"},
				},
			},
		},
	}
}

func TestName(t *testing.T) {
	if got := NewAskUserQuestionTool(nil).Name(); got != "ask_user_question" {
		t.Fatalf("Name() = %q, want ask_user_question", got)
	}
}

func TestDefinitionShape(t *testing.T) {
	def := NewAskUserQuestionTool(nil).Definition()
	if def.Name != "ask_user_question" {
		t.Fatalf("Definition().Name = %q", def.Name)
	}
	schemaMap, ok := def.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("InputSchema is not a map: %T", def.InputSchema)
	}
	props, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing properties")
	}
	for _, key := range []string{"questions", "answers", "annotations", "metadata"} {
		if _, ok := props[key]; !ok {
			t.Errorf("InputSchema.properties missing %q", key)
		}
	}
	questions, _ := props["questions"].(map[string]interface{})
	if questions["minItems"] != 1 || questions["maxItems"] != 4 {
		t.Errorf("questions bounds = %v/%v, want 1/4", questions["minItems"], questions["maxItems"])
	}

	// Lever A: the description must direct the model on WHEN to use the tool,
	// not just what it does, so it is chosen over free-form prose clarification.
	desc := strings.ToLower(def.Description)
	for _, trigger := range []string{"clarify", "decision", "preferences"} {
		if !strings.Contains(desc, trigger) {
			t.Errorf("Definition().Description missing trigger guidance %q: %q", trigger, def.Description)
		}
	}
}

func TestValidation(t *testing.T) {
	dupQuestions := map[string]interface{}{
		"questions": []map[string]interface{}{
			{"question": "Same?", "header": "h", "options": []map[string]interface{}{{"label": "a", "description": "d"}, {"label": "b", "description": "d"}}},
			{"question": "Same?", "header": "h", "options": []map[string]interface{}{{"label": "a", "description": "d"}, {"label": "b", "description": "d"}}},
		},
	}
	dupLabels := map[string]interface{}{
		"questions": []map[string]interface{}{
			{"question": "Q?", "header": "h", "options": []map[string]interface{}{{"label": "a", "description": "d"}, {"label": "a", "description": "d"}}},
		},
	}
	tooFewOptions := map[string]interface{}{
		"questions": []map[string]interface{}{
			{"question": "Q?", "header": "h", "options": []map[string]interface{}{{"label": "a", "description": "d"}}},
		},
	}
	tooManyOptions := map[string]interface{}{
		"questions": []map[string]interface{}{
			{"question": "Q?", "header": "h", "options": []map[string]interface{}{
				{"label": "a", "description": "d"}, {"label": "b", "description": "d"},
				{"label": "c", "description": "d"}, {"label": "e", "description": "d"}, {"label": "f", "description": "d"},
			}},
		},
	}
	overLong := map[string]interface{}{
		"questions": []map[string]interface{}{
			{
				"question": "Q?",
				"header":   "this header is way longer than twelve characters",
				"options": []map[string]interface{}{
					{"label": "a very long option label far beyond five words indeed", "description": "d"},
					{"label": "b", "description": "d"},
				},
			},
		},
	}

	cases := []struct {
		name     string
		args     interface{}
		wantErr  bool
		askerHit bool
	}{
		{"TC-005 duplicate questions", dupQuestions, true, false},
		{"TC-006 duplicate labels", dupLabels, true, false},
		{"TC-007 empty questions", map[string]interface{}{"questions": []map[string]interface{}{}}, true, false},
		{"TC-008 too few options", tooFewOptions, true, false},
		{"TC-008 too many options", tooManyOptions, true, false},
		{"TC-019 over-length passes", overLong, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asker := &fakeAsker{answers: []Answer{{QuestionText: "Q?", Value: "a"}}}
			tool := NewAskUserQuestionTool(asker)
			_, err := tool.Execute(context.Background(), mustArgs(t, tc.args))
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if asker.invoked != tc.askerHit {
				t.Fatalf("asker.invoked = %v, want %v", asker.invoked, tc.askerHit)
			}
		})
	}
}

func TestMalformedJSON(t *testing.T) {
	tool := NewAskUserQuestionTool(&fakeAsker{})
	_, err := tool.Execute(context.Background(), json.RawMessage(`{not json`))
	if err == nil {
		t.Fatalf("expected error on malformed JSON")
	}
}

func TestSingleSelectFormatting(t *testing.T) {
	asker := &fakeAsker{answers: []Answer{{QuestionText: "Which database should we use?", Value: "PostgreSQL"}}}
	tool := NewAskUserQuestionTool(asker)
	out, err := tool.Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `User has answered your questions: "Which database should we use?"="PostgreSQL". You can now continue with the user's answers in mind.`
	if out != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}
	if !asker.invoked {
		t.Fatalf("asker should have been invoked")
	}
}

func TestFourQuestionsInOrder(t *testing.T) {
	args := map[string]interface{}{"questions": []map[string]interface{}{}}
	qs := []map[string]interface{}{}
	answers := []Answer{}
	for _, name := range []string{"Q1?", "Q2?", "Q3?", "Q4?"} {
		qs = append(qs, map[string]interface{}{
			"question": name, "header": "h",
			"options": []map[string]interface{}{{"label": "x", "description": "d"}, {"label": "y", "description": "d"}},
		})
		answers = append(answers, Answer{QuestionText: name, Value: "x"})
	}
	args["questions"] = qs
	asker := &fakeAsker{answers: answers}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i1, i4 := strings.Index(out, "Q1?"), strings.Index(out, "Q4?"); i1 < 0 || i4 < 0 || i1 > i4 {
		t.Fatalf("answers not in order: %q", out)
	}
}

func TestMultiSelectAndOtherFreeText(t *testing.T) {
	// TC-003: a multi-select answer arrives pre-joined from the asker.
	// TC-004: an "Other" free-text answer is surfaced verbatim.
	asker := &fakeAsker{answers: []Answer{{QuestionText: "Which database should we use?", Value: "PostgreSQL, MongoDB"}}}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `="PostgreSQL, MongoDB"`) {
		t.Fatalf("multi-select join not surfaced: %q", out)
	}

	asker2 := &fakeAsker{answers: []Answer{{QuestionText: "Which database should we use?", Value: "DuckDB (typed by user)"}}}
	out2, _ := NewAskUserQuestionTool(asker2).Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if !strings.Contains(out2, "DuckDB (typed by user)") {
		t.Fatalf("free-text answer not surfaced: %q", out2)
	}
}

func TestPreviewAndNotesAnnotations(t *testing.T) {
	asker := &fakeAsker{answers: []Answer{{
		QuestionText: "Which database should we use?",
		Value:        "PostgreSQL",
		Preview:      "CREATE TABLE t (id int);",
		Notes:        "prefer SQL",
	}}}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "selected preview:\nCREATE TABLE t (id int);") {
		t.Fatalf("preview missing: %q", out)
	}
	if !strings.Contains(out, "user notes: prefer SQL") {
		t.Fatalf("notes missing: %q", out)
	}
}

func TestAnswersInjectionFull(t *testing.T) {
	args := oneQuestionArgs()
	args["answers"] = map[string]string{"Which database should we use?": "PostgreSQL"}
	asker := &fakeAsker{}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asker.invoked {
		t.Fatalf("asker must NOT be invoked when answers are pre-supplied")
	}
	if !strings.Contains(out, `"Which database should we use?"="PostgreSQL"`) {
		t.Fatalf("injected answer not used: %q", out)
	}
}

func TestAnswersInjectionPartial(t *testing.T) {
	// TC-017: answers covers 1 of 2 questions -> only present entry formatted, no error, no prompt.
	args := map[string]interface{}{
		"questions": []map[string]interface{}{
			{"question": "Q1?", "header": "h", "options": []map[string]interface{}{{"label": "a", "description": "d"}, {"label": "b", "description": "d"}}},
			{"question": "Q2?", "header": "h", "options": []map[string]interface{}{{"label": "a", "description": "d"}, {"label": "b", "description": "d"}}},
		},
		"answers": map[string]string{"Q1?": "a"},
	}
	asker := &fakeAsker{}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asker.invoked {
		t.Fatalf("asker must NOT be invoked for partial injection")
	}
	if !strings.Contains(out, `"Q1?"="a"`) || strings.Contains(out, "Q2?") {
		t.Fatalf("partial injection should include only Q1?: %q", out)
	}
}

func TestNonMatchingAnswerKey(t *testing.T) {
	// TC-020: an answers key matching no question text is still formatted, no panic.
	args := oneQuestionArgs()
	args["answers"] = map[string]string{"Totally unrelated key?": "value"}
	out, err := NewAskUserQuestionTool(&fakeAsker{}).Execute(context.Background(), mustArgs(t, args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"Totally unrelated key?"="value"`) {
		t.Fatalf("non-matching key not formatted: %q", out)
	}
}

func TestUserCancelled(t *testing.T) {
	asker := &fakeAsker{err: ErrUserCancelled}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if err != nil {
		t.Fatalf("cancellation should not be an error: %v", err)
	}
	if !strings.Contains(out, "dismissed the questions") {
		t.Fatalf("expected cancellation message, got: %q", out)
	}
}

func TestContextCancelled(t *testing.T) {
	asker := &fakeAsker{err: context.Canceled}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if err != nil {
		t.Fatalf("ctx cancellation should return a message, not error: %v", err)
	}
	if !strings.Contains(out, "interrupted") {
		t.Fatalf("expected interruption message, got: %q", out)
	}
}

func TestNilAskerFallback(t *testing.T) {
	out, err := NewAskUserQuestionTool(nil).Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if err != nil {
		t.Fatalf("nil asker should return a message, not error: %v", err)
	}
	if !strings.Contains(out, "No interactive user is available") {
		t.Fatalf("expected no-interactive-user message, got: %q", out)
	}
}

func TestResultSizeCap(t *testing.T) {
	big := strings.Repeat("x", 200_000)
	asker := &fakeAsker{answers: []Answer{{QuestionText: "Which database should we use?", Value: "PostgreSQL", Preview: big}}}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) > maxResultSizeChars {
		t.Fatalf("result not capped: len=%d", len(out))
	}
	if !strings.HasSuffix(out, "[truncated]") {
		t.Fatalf("expected truncation marker, got suffix: %q", out[len(out)-20:])
	}
}

func TestResultSizeCapKeepsValidUTF8(t *testing.T) {
	// CODE-003: truncation must not split a multi-byte rune.
	big := strings.Repeat("世", 100_000) // 3 bytes each
	asker := &fakeAsker{answers: []Answer{{QuestionText: "Which database should we use?", Value: "PostgreSQL", Preview: big}}}
	out, err := NewAskUserQuestionTool(asker).Execute(context.Background(), mustArgs(t, oneQuestionArgs()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) > maxResultSizeChars {
		t.Fatalf("result not capped: len=%d", len(out))
	}
	if !utf8.ValidString(out) {
		t.Fatalf("truncated result is not valid UTF-8")
	}
}

func TestNotParallelSafe(t *testing.T) {
	var tool interface{} = NewAskUserQuestionTool(nil)
	if _, ok := tool.(ParallelSafeTool); ok {
		t.Fatalf("ask_user_question must NOT implement ParallelSafeTool")
	}
}

func TestAskUserQuestionAdvertisesUpstreamAlias(t *testing.T) {
	var tool interface{} = NewAskUserQuestionTool(nil)
	aliasable, ok := tool.(AliasableTool)
	if !ok {
		t.Fatalf("AskUserQuestionTool must implement AliasableTool so imported prompts can call it by the upstream name")
	}
	want := "AskUserQuestion"
	for _, a := range aliasable.Aliases() {
		if a == want {
			return
		}
	}
	t.Fatalf("aliases missing upstream name %q: %v", want, aliasable.Aliases())
}

// TestAskUserQuestionCallableViaUpstreamAlias verifies the full registry path:
// a tool call bearing the imported "AskUserQuestion" name resolves to the tool
// and reaches the asker, proving re-imported prompts work unmodified.
func TestAskUserQuestionCallableViaUpstreamAlias(t *testing.T) {
	asker := &fakeAsker{answers: []Answer{{QuestionText: "Q?", Value: "a"}}}
	reg := NewRegistry()
	reg.Register(NewAskUserQuestionTool(asker))

	defs := reg.GetAvailableTools()
	sawAlias := false
	for _, d := range defs {
		if d.Name == "AskUserQuestion" {
			sawAlias = true
		}
	}
	if !sawAlias {
		t.Fatalf("AskUserQuestion alias not advertised: %+v", defs)
	}

	res := reg.Execute(context.Background(), schema.ToolCall{ID: "1", Name: "AskUserQuestion", Arguments: mustArgs(t, oneQuestionArgs())})
	if res.IsError {
		t.Fatalf("alias call failed: %s", res.Output)
	}
	if !asker.invoked {
		t.Fatal("alias call did not reach the asker")
	}
}

func TestRegistryReportsNotParallelSafe(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewAskUserQuestionTool(&fakeAsker{}))
	if reg.IsParallelSafe("ask_user_question") {
		t.Fatalf("registry should report ask_user_question as not parallel-safe")
	}
}

func BenchmarkAskUserQuestionFormat(b *testing.B) {
	// Max input: 4 questions x 4 options, answers pre-supplied so the asker is
	// not involved. Measures only the pure-CPU validate+format path (NFR-004);
	// end-to-end latency is human-input-bound and out of scope.
	qs := []map[string]interface{}{}
	answers := map[string]string{}
	for _, name := range []string{"Q1?", "Q2?", "Q3?", "Q4?"} {
		qs = append(qs, map[string]interface{}{
			"question": name, "header": "hdr",
			"options": []map[string]interface{}{
				{"label": "a", "description": "d"}, {"label": "b", "description": "d"},
				{"label": "c", "description": "d"}, {"label": "e", "description": "d"},
			},
		})
		answers[name] = "a"
	}
	raw, _ := json.Marshal(map[string]interface{}{"questions": qs, "answers": answers})
	tool := NewAskUserQuestionTool(nil)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tool.Execute(ctx, raw); err != nil {
			b.Fatalf("execute: %v", err)
		}
	}
}
