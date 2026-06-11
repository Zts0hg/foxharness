package autodev

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// DefaultEngineerPersona is the engineer Agent's system prompt when the
// config does not supply one (REQ-016). It mirrors the constitutional
// values so the simulated engineer makes constitution-aligned decisions.
const DefaultEngineerPersona = `You are a senior software engineer supervising an autonomous coding agent.
You make pragmatic engineering decisions on the user's behalf.
Your priorities, in order: system stability, consistent code style, testability, and readability.
You favor test-driven development, small focused changes, and existing project conventions.
You never cancel or stall the work: when unsure, pick the recommended or most conservative option.
You are read-only with respect to the workspace: you decide and instruct, the coding agent acts.`

// ProviderEngineerAgent is the production EngineerAgent backed by an LLM
// provider. It shares the core Agent's model and persona across Decide,
// Reply, and Review (REQ-016).
type ProviderEngineerAgent struct {
	provider provider.LLMProvider
	model    string
	persona  string
}

// NewEngineerAgent creates a ProviderEngineerAgent on p. model is recorded
// for introspection (the provider is already bound to it); persona overrides
// DefaultEngineerPersona when non-empty.
func NewEngineerAgent(p provider.LLMProvider, model, persona string) *ProviderEngineerAgent {
	if strings.TrimSpace(persona) == "" {
		persona = DefaultEngineerPersona
	}
	return &ProviderEngineerAgent{provider: p, model: model, persona: persona}
}

var _ EngineerAgent = (*ProviderEngineerAgent)(nil)

// Model returns the model name the agent was constructed with.
func (a *ProviderEngineerAgent) Model() string { return a.model }

// decideResponse is the JSON reply format requested from the engineer LLM
// for ask_user_question decisions.
type decideResponse struct {
	Answers []struct {
		Question  string   `json:"question"`
		Selection []string `json:"selection"`
		Other     string   `json:"other"`
	} `json:"answers"`
}

// Decide implements EngineerAgent. It asks the LLM to pick an option (or
// supply "Other" free text) per question and maps the reply onto
// []tools.Answer. Unparseable replies and unanswered questions fall back to
// each question's first option so the loop never stalls (PLAN-005).
func (a *ProviderEngineerAgent) Decide(ctx context.Context, qs []tools.Question, c StageContext) ([]tools.Answer, error) {
	resp, err := a.generate(ctx, a.decidePrompt(qs, c))
	if err != nil {
		return nil, err
	}

	var parsed decideResponse
	if err := json.Unmarshal([]byte(extractJSON(resp)), &parsed); err != nil {
		return fallbackAnswers(qs), nil
	}

	byQuestion := make(map[string]string, len(parsed.Answers))
	for _, ans := range parsed.Answers {
		value := strings.TrimSpace(ans.Other)
		if value == "" {
			value = strings.TrimSpace(strings.Join(ans.Selection, ", "))
		}
		if value != "" {
			byQuestion[ans.Question] = value
		}
	}

	answers := make([]tools.Answer, 0, len(qs))
	for _, q := range qs {
		value, ok := byQuestion[q.Prompt]
		if !ok {
			value = firstOptionLabel(q)
		}
		answers = append(answers, tools.Answer{QuestionText: q.Prompt, Value: value})
	}
	return answers, nil
}

// Reply implements EngineerAgent: it answers a free-form prose question
// from the core Agent in the engineer persona.
func (a *ProviderEngineerAgent) Reply(ctx context.Context, prompt string, c StageContext) (string, error) {
	return a.generate(ctx, contextPreamble(c)+
		"The coding agent asked you the following question. Answer it directly and concisely, "+
		"making the engineering decision yourself.\n\n"+prompt)
}

// reviewApproval is the exact reply that signals approval in Review.
const reviewApproval = "APPROVE"

// Review implements EngineerAgent. It shows the engineer the core run's
// outcome together with the Go-computed verification gap and returns ""
// on approval or a corrective instruction to feed back to the core Agent
// (REQ-014).
func (a *ProviderEngineerAgent) Review(ctx context.Context, res *engine.RunResult, gap string, c StageContext) (string, error) {
	var b strings.Builder
	b.WriteString(contextPreamble(c))
	b.WriteString("You are reviewing the result of the coding agent's last run, like a user supervising it.\n\n")
	final := ""
	if res != nil {
		final = strings.TrimSpace(res.FinalMessage)
	}
	if final == "" {
		final = "(the agent produced no final message)"
	}
	b.WriteString("Agent's final message:\n" + final + "\n\n")
	if strings.TrimSpace(gap) != "" {
		b.WriteString("Deterministic ground-truth verification FAILED with this gap:\n" + gap + "\n\n")
		b.WriteString("The step is NOT complete regardless of what the agent claims. ")
	}
	b.WriteString("If everything is truly complete reply with exactly " + reviewApproval + " and nothing else. " +
		"Otherwise reply with a single corrective instruction for the coding agent " +
		"(imperative, concrete, e.g. \"Run git add -A to stage the changes, then create the commit again.\").")

	resp, err := a.generate(ctx, b.String())
	if err != nil {
		return "", err
	}
	if strings.EqualFold(strings.TrimSpace(resp), reviewApproval) {
		return "", nil
	}
	return strings.TrimSpace(resp), nil
}

func (a *ProviderEngineerAgent) generate(ctx context.Context, userPrompt string) (string, error) {
	resp, err := a.provider.Generate(ctx, []schema.Message{
		{Role: schema.RoleSystem, Content: a.persona},
		{Role: schema.RoleUser, Content: userPrompt},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("engineer generate: %w", err)
	}
	if resp == nil || resp.Message == nil {
		return "", fmt.Errorf("engineer generate: empty response")
	}
	return resp.Message.Content, nil
}

func (a *ProviderEngineerAgent) decidePrompt(qs []tools.Question, c StageContext) string {
	var b strings.Builder
	b.WriteString(contextPreamble(c))
	b.WriteString("The coding agent asked you to choose between options. Decide for each question.\n\n")
	for i, q := range qs {
		fmt.Fprintf(&b, "Question %d: %s\n", i+1, q.Prompt)
		for _, o := range q.Options {
			fmt.Fprintf(&b, "  - %q: %s\n", o.Label, o.Description)
		}
		if q.MultiSelect {
			b.WriteString("  (multiple selections allowed)\n")
		}
	}
	b.WriteString("\nReply with ONLY this JSON, no prose:\n" +
		`{"answers":[{"question":"<exact question text>","selection":["<chosen option label>", ...]}]}` + "\n" +
		`If no offered option fits, use {"question":"...","other":"<your free-text answer>"} instead.`)
	return b.String()
}

func contextPreamble(c StageContext) string {
	var parts []string
	if c.Item.Title != "" {
		parts = append(parts, "Current requirement: "+c.Item.Title)
	}
	if c.Stage != "" {
		parts = append(parts, "Current step: "+c.Stage)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n") + "\n\n"
}

// extractJSON returns the first top-level {...} block in s so fenced or
// prose-wrapped JSON replies still parse.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}

func firstOptionLabel(q tools.Question) string {
	if len(q.Options) == 0 {
		return "Proceed with your best judgement."
	}
	return q.Options[0].Label
}

func fallbackAnswers(qs []tools.Question) []tools.Answer {
	answers := make([]tools.Answer, 0, len(qs))
	for _, q := range qs {
		answers = append(answers, tools.Answer{QuestionText: q.Prompt, Value: firstOptionLabel(q)})
	}
	return answers
}

// EngineerAsker adapts an EngineerAgent to tools.UserAsker so the merged
// ask_user_question tool is answered by the simulated engineer instead of a
// human (REQ-013). It never cancels: on any agent failure it falls back to
// each question's first (recommended) option (PLAN-005).
type EngineerAsker struct {
	agent    EngineerAgent
	reporter Reporter
	sc       *StageContext
}

// NewEngineerAsker creates an EngineerAsker. sc points at the live per-item
// StageContext so decisions see the current stage; it may be nil.
func NewEngineerAsker(agent EngineerAgent, reporter Reporter, sc *StageContext) *EngineerAsker {
	return &EngineerAsker{agent: agent, reporter: reporter, sc: sc}
}

var _ tools.UserAsker = (*EngineerAsker)(nil)

// Ask implements tools.UserAsker by delegating to EngineerAgent.Decide and
// streaming the exchange to the reporter. Every question always receives an
// answer; ErrUserCancelled is never returned.
func (a *EngineerAsker) Ask(ctx context.Context, questions []tools.Question) ([]tools.Answer, error) {
	sc := StageContext{}
	if a.sc != nil {
		sc = *a.sc
	}

	answers, err := a.agent.Decide(ctx, questions, sc)
	if err != nil || len(answers) == 0 {
		answers = fallbackAnswers(questions)
	} else {
		answers = fillMissingAnswers(questions, answers)
	}

	if a.reporter != nil {
		a.reporter.OnEngineerDecision(ctx, questions, answers)
	}
	return answers, nil
}

// fillMissingAnswers guarantees one answer per question, falling back to
// the first option for any question the agent skipped.
func fillMissingAnswers(questions []tools.Question, answers []tools.Answer) []tools.Answer {
	byQuestion := make(map[string]tools.Answer, len(answers))
	for _, ans := range answers {
		byQuestion[ans.QuestionText] = ans
	}
	out := make([]tools.Answer, 0, len(questions))
	for _, q := range questions {
		if ans, ok := byQuestion[q.Prompt]; ok && strings.TrimSpace(ans.Value) != "" {
			out = append(out, ans)
			continue
		}
		out = append(out, tools.Answer{QuestionText: q.Prompt, Value: firstOptionLabel(q)})
	}
	return out
}
