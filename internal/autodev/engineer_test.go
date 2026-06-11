package autodev

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// fakeEngineerAgent is a scripted EngineerAgent for asker tests.
type fakeEngineerAgent struct {
	decideAnswers []tools.Answer
	decideErr     error
	decideCalls   int
	lastQuestions []tools.Question
}

func (f *fakeEngineerAgent) Decide(ctx context.Context, qs []tools.Question, c StageContext) ([]tools.Answer, error) {
	f.decideCalls++
	f.lastQuestions = qs
	return f.decideAnswers, f.decideErr
}

func (f *fakeEngineerAgent) Reply(ctx context.Context, prompt string, c StageContext) (string, error) {
	return "", nil
}

func (f *fakeEngineerAgent) Review(ctx context.Context, res *engine.RunResult, gap string, c StageContext) (string, error) {
	return "", nil
}

// scriptedProvider replays canned LLM responses and records the requests.
type scriptedProvider struct {
	responses []string
	err       error
	requests  [][]schema.Message
}

func (p *scriptedProvider) Generate(ctx context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.requests = append(p.requests, messages)
	if p.err != nil {
		return nil, p.err
	}
	resp := p.responses[0]
	if len(p.responses) > 1 {
		p.responses = p.responses[1:]
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: resp}}, nil
}

func sampleQuestions() []tools.Question {
	return []tools.Question{{
		Header: "Location",
		Prompt: "Where should discoveries be appended?",
		Options: []tools.Option{
			{Label: "MEMORY.md (Recommended)", Description: "project memory"},
			{Label: "working_memory.md", Description: "session memory"},
		},
	}}
}

func TestEngineerAskerRoutesToAgent(t *testing.T) {
	agent := &fakeEngineerAgent{
		decideAnswers: []tools.Answer{{
			QuestionText: "Where should discoveries be appended?",
			Value:        "MEMORY.md (Recommended)",
		}},
	}
	asker := NewEngineerAsker(agent, NewTerminalReporter(io.Discard), &StageContext{Slug: "x"})

	answers, err := asker.Ask(context.Background(), sampleQuestions())
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if agent.decideCalls != 1 {
		t.Fatalf("Decide calls = %d, want 1", agent.decideCalls)
	}
	if len(answers) != 1 {
		t.Fatalf("len(answers) = %d, want 1", len(answers))
	}
	if answers[0].QuestionText != "Where should discoveries be appended?" {
		t.Errorf("QuestionText = %q, want the question prompt (TC-007)", answers[0].QuestionText)
	}
	if answers[0].Value != "MEMORY.md (Recommended)" {
		t.Errorf("Value = %q, want the selected option label (TC-007)", answers[0].Value)
	}
}

func TestEngineerAskerPassesThroughOtherFreeText(t *testing.T) {
	agent := &fakeEngineerAgent{
		decideAnswers: []tools.Answer{{
			QuestionText: "Where should discoveries be appended?",
			Value:        "Append under a '## Discoveries' section in MEMORY.md",
		}},
	}
	asker := NewEngineerAsker(agent, NewTerminalReporter(io.Discard), &StageContext{})

	answers, err := asker.Ask(context.Background(), sampleQuestions())
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if !strings.Contains(answers[0].Value, "Discoveries") {
		t.Errorf("Value = %q, want free-text Other answer preserved", answers[0].Value)
	}
}

func TestEngineerAskerNeverCancels(t *testing.T) {
	agent := &fakeEngineerAgent{decideErr: errors.New("model unavailable")}
	asker := NewEngineerAsker(agent, NewTerminalReporter(io.Discard), &StageContext{})

	answers, err := asker.Ask(context.Background(), sampleQuestions())
	if err != nil {
		t.Fatalf("Ask returned error %v, want nil — the engineer never cancels (PLAN-005)", err)
	}
	if len(answers) != 1 {
		t.Fatalf("len(answers) = %d, want 1 fallback answer", len(answers))
	}
	if answers[0].Value != "MEMORY.md (Recommended)" {
		t.Errorf("Value = %q, want the first/recommended option as fallback", answers[0].Value)
	}
}

func TestEngineerAskerFillsMissingAnswers(t *testing.T) {
	agent := &fakeEngineerAgent{decideAnswers: nil}
	asker := NewEngineerAsker(agent, NewTerminalReporter(io.Discard), &StageContext{})

	answers, err := asker.Ask(context.Background(), sampleQuestions())
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("len(answers) = %d, want 1 (every question answered)", len(answers))
	}
	if answers[0].Value == "" {
		t.Error("Value empty, want fallback option label")
	}
}

func TestProviderEngineerAgentDecideParsesSelection(t *testing.T) {
	p := &scriptedProvider{responses: []string{
		`{"answers":[{"question":"Where should discoveries be appended?","selection":["MEMORY.md (Recommended)"]}]}`,
	}}
	agent := NewEngineerAgent(p, "glm-4.7", "")

	answers, err := agent.Decide(context.Background(), sampleQuestions(), StageContext{})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("len(answers) = %d, want 1", len(answers))
	}
	if answers[0].Value != "MEMORY.md (Recommended)" {
		t.Errorf("Value = %q, want selected label", answers[0].Value)
	}
}

func TestProviderEngineerAgentDecideOtherFreeText(t *testing.T) {
	p := &scriptedProvider{responses: []string{
		`{"answers":[{"question":"Where should discoveries be appended?","other":"Use a new DISCOVERIES.md file"}]}`,
	}}
	agent := NewEngineerAgent(p, "glm-4.7", "")

	answers, err := agent.Decide(context.Background(), sampleQuestions(), StageContext{})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if answers[0].Value != "Use a new DISCOVERIES.md file" {
		t.Errorf("Value = %q, want Other free text", answers[0].Value)
	}
}

func TestProviderEngineerAgentDecideFallsBackOnGarbage(t *testing.T) {
	p := &scriptedProvider{responses: []string{"sure, sounds good!"}}
	agent := NewEngineerAgent(p, "glm-4.7", "")

	answers, err := agent.Decide(context.Background(), sampleQuestions(), StageContext{})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if answers[0].Value != "MEMORY.md (Recommended)" {
		t.Errorf("Value = %q, want first option fallback on unparseable reply", answers[0].Value)
	}
}

func TestProviderEngineerAgentReviewReturnsCorrection(t *testing.T) {
	p := &scriptedProvider{responses: []string{
		"The commit failed because nothing was staged. Run `git add -A` and then retry the commit.",
	}}
	agent := NewEngineerAgent(p, "glm-4.7", "")

	res := &engine.RunResult{FinalMessage: "Nothing to commit; I think we're done."}
	correction, err := agent.Review(context.Background(), res, "HEAD did not advance (no commit was created)", StageContext{Stage: "commit"})
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if !strings.Contains(correction, "git add") {
		t.Errorf("correction = %q, want git add steer (TC-024)", correction)
	}

	// The review prompt must carry both the run outcome and the Go gap so
	// the engineer can supervise like a human user would (REQ-014).
	last := p.requests[len(p.requests)-1]
	joined := ""
	for _, m := range last {
		joined += m.Content + "\n"
	}
	if !strings.Contains(joined, "Nothing to commit") {
		t.Error("review prompt missing the core run's final message")
	}
	if !strings.Contains(joined, "HEAD did not advance") {
		t.Error("review prompt missing the verification gap")
	}
}

func TestProviderEngineerAgentReviewApproves(t *testing.T) {
	p := &scriptedProvider{responses: []string{"APPROVE"}}
	agent := NewEngineerAgent(p, "glm-4.7", "")

	correction, err := agent.Review(context.Background(), &engine.RunResult{FinalMessage: "done"}, "", StageContext{})
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	if correction != "" {
		t.Errorf("correction = %q, want empty on approval", correction)
	}
}

func TestProviderEngineerAgentUsesPersona(t *testing.T) {
	p := &scriptedProvider{responses: []string{"APPROVE"}}
	agent := NewEngineerAgent(p, "glm-4.7", "You are Margaret, principal engineer.")

	if _, err := agent.Review(context.Background(), &engine.RunResult{}, "", StageContext{}); err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	first := p.requests[0][0]
	if first.Role != schema.RoleSystem {
		t.Fatalf("first message role = %q, want system persona", first.Role)
	}
	if !strings.Contains(first.Content, "Margaret") {
		t.Errorf("system prompt = %q, want configured persona", first.Content)
	}
}

func TestProviderEngineerAgentDefaultPersona(t *testing.T) {
	p := &scriptedProvider{responses: []string{"APPROVE"}}
	agent := NewEngineerAgent(p, "glm-4.7", "")

	if _, err := agent.Review(context.Background(), &engine.RunResult{}, "", StageContext{}); err != nil {
		t.Fatalf("Review returned error: %v", err)
	}
	persona := strings.ToLower(p.requests[0][0].Content)
	for _, want := range []string{"stability", "testability", "readability"} {
		if !strings.Contains(persona, want) {
			t.Errorf("default persona missing %q (REQ-016)", want)
		}
	}
}

func TestProviderEngineerAgentModel(t *testing.T) {
	agent := NewEngineerAgent(&scriptedProvider{}, "glm-4.7", "")
	if agent.Model() != "glm-4.7" {
		t.Errorf("Model() = %q, want glm-4.7", agent.Model())
	}
}
