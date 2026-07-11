package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/memory"
	providerpkg "github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type approvingPlanReviewer struct {
	plans []string
}

func (r *approvingPlanReviewer) ReviewPlan(ctx context.Context, planMarkdown string) (tools.PlanReview, error) {
	r.plans = append(r.plans, planMarkdown)
	return tools.PlanReview{Decision: tools.PlanApproved}, nil
}

type formalLifecycleProvider struct {
	calls        int
	toolSurfaces [][]string
	seen         [][]schema.Message
	plan         string
}

func (p *formalLifecycleProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*providerpkg.GenerateResponse, error) {
	names := make([]string, 0, len(availableTools))
	for _, definition := range availableTools {
		names = append(names, definition.Name)
	}
	p.toolSurfaces = append(p.toolSurfaces, names)
	p.seen = append(p.seen, append([]schema.Message(nil), messages...))

	call := p.calls
	p.calls++
	switch call {
	case 0:
		return &providerpkg.GenerateResponse{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{
				{ID: "submit", Name: "submit_plan", Arguments: lifecycleArgs(map[string]string{"plan_markdown": p.plan})},
				{ID: "early-write-1", Name: "write_file", Arguments: lifecycleArgs(map[string]string{"path": "too-early-1.txt", "content": "bad"})},
			},
		}}, nil
	case 1:
		return &providerpkg.GenerateResponse{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{
				{ID: "todo", Name: "update_todo", Arguments: lifecycleArgs(map[string]string{"content": "# TODO\n\n- [ ] Implement the approved change\n"})},
				{ID: "early-write-2", Name: "write_file", Arguments: lifecycleArgs(map[string]string{"path": "too-early-2.txt", "content": "bad"})},
			},
		}}, nil
	case 2:
		return &providerpkg.GenerateResponse{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{
				{ID: "implementation", Name: "write_file", Arguments: lifecycleArgs(map[string]string{"path": "implemented.txt", "content": "done"})},
			},
		}}, nil
	default:
		return &providerpkg.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "implemented"}}, nil
	}
}

func lifecycleArgs(value interface{}) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func newFormalLifecycleRunner(t *testing.T, provider providerpkg.LLMProvider, reviewer tools.PlanReviewer) (*AgentRunner, *session.Session, *memory.Store) {
	t.Helper()
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	runner := &AgentRunner{
		workDir:           workDir,
		model:             "fake-model",
		providerProtocol:  "openai",
		collaborationMode: collaboration.ModeFormalPlan,
		maxTurns:          8,
		store:             store,
		manager:           manager,
		llmProvider:       provider,
		currentSession:    sess,
		planReviewer:      reviewer,
	}
	return runner, sess, store
}

func TestFormalPlanLifecycleContinuesInOneRunAndGatesImplementation(t *testing.T) {
	plan := "# Approved plan\n\n1. Inspect current behavior.\n2. Implement and test."
	provider := &formalLifecycleProvider{plan: plan}
	reviewer := &approvingPlanReviewer{}
	runner, sess, store := newFormalLifecycleRunner(t, provider, reviewer)

	result, err := runner.Run(context.Background(), "change the project", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil || result.FinalMessage != "implemented" || provider.calls != 4 {
		t.Fatalf("result = %#v, provider calls = %d, want implemented after 4 calls", result, provider.calls)
	}

	assertLifecycleToolSurface(t, provider.toolSurfaces[0],
		[]string{"read_file", "bash", "ask_user_question", "submit_plan"},
		[]string{"write_file", "edit_file", "update_todo"})
	assertLifecycleToolSurface(t, provider.toolSurfaces[1],
		[]string{"read_file", "bash", "ask_user_question", "read_todo", "update_todo"},
		[]string{"write_file", "edit_file", "submit_plan"})
	assertLifecycleToolSurface(t, provider.toolSurfaces[2],
		[]string{"read_file", "write_file", "edit_file", "update_todo"},
		[]string{"submit_plan"})

	if len(provider.seen) < 2 || !lifecycleMessagesContain(provider.seen[1], plan) {
		t.Fatalf("post-approval context missing complete approved plan: %#v", provider.seen)
	}
	if len(provider.seen[0]) == 0 {
		t.Fatal("first provider call received no system prompt")
	}
	systemPrompt := provider.seen[0][0].Content
	for _, want := range []string{"Formal Plan", "Do not create or modify project files", "Git state", "read-only", "submit_plan"} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("Formal system prompt missing %q:\n%s", want, systemPrompt)
		}
	}

	for _, path := range []string{"too-early-1.txt", "too-early-2.txt"} {
		if _, err := os.Stat(filepath.Join(runner.WorkDir(), path)); !os.IsNotExist(err) {
			t.Fatalf("phase-ineligible write created %s: %v", path, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(runner.WorkDir(), "implemented.txt")); err != nil || string(data) != "done" {
		t.Fatalf("implemented file = %q, err=%v", data, err)
	}
	if data, err := os.ReadFile(store.PlanPath()); err != nil || string(data) != plan {
		t.Fatalf("PLAN.md = %q, err=%v, want exact approved plan", data, err)
	}
	if data, err := os.ReadFile(store.TodoPath()); err != nil || !strings.Contains(string(data), "Implement the approved change") {
		t.Fatalf("TODO.md = %q, err=%v", data, err)
	}
	if len(reviewer.plans) != 1 || reviewer.plans[0] != plan {
		t.Fatalf("reviewer plans = %#v, want exact approved plan", reviewer.plans)
	}
	if got := runner.CollaborationMode(); got != collaboration.ModeDefault {
		t.Fatalf("runner collaboration mode = %q, want Default after approval", got)
	}

	records, err := session.NewMessageLog(sess).LoadRecords()
	if err != nil {
		t.Fatalf("LoadRecords() error = %v", err)
	}
	userPrompts := 0
	runIDs := map[string]bool{}
	for _, record := range records {
		runIDs[record.RunID] = true
		if record.Message.Role == schema.RoleUser && record.Message.ToolCallID == "" {
			userPrompts++
		}
	}
	if userPrompts != 1 || len(runIDs) != 1 || !runIDs[result.RunID] {
		t.Fatalf("message continuity: user prompts=%d run IDs=%v result run=%s", userPrompts, runIDs, result.RunID)
	}
}

func TestFormalPlanRestrictedRunRejectsMissingRequiredToolsBeforeModelCall(t *testing.T) {
	provider := &immediateCountingProvider{}
	runner, _, _ := newFormalLifecycleRunner(t, provider, &approvingPlanReviewer{})

	_, err := runner.RunRestricted(context.Background(), "plan this", []string{"read_file"}, nil)
	if err == nil || !strings.Contains(err.Error(), "Formal Plan") {
		t.Fatalf("RunRestricted() error = %v, want missing Formal Plan tools", err)
	}
	if got := provider.count(); got != 0 {
		t.Fatalf("provider calls = %d, want 0 after preflight rejection", got)
	}
}

func assertLifecycleToolSurface(t *testing.T, got []string, required []string, forbidden []string) {
	t.Helper()
	set := make(map[string]bool, len(got))
	for _, name := range got {
		set[name] = true
	}
	for _, name := range required {
		if !set[name] {
			t.Fatalf("tool surface %v missing %q", got, name)
		}
	}
	for _, name := range forbidden {
		if set[name] {
			t.Fatalf("tool surface %v unexpectedly contains %q", got, name)
		}
	}
}

func lifecycleMessagesContain(messages []schema.Message, want string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, want) {
			return true
		}
	}
	return false
}
