package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/childruntime"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/runtimejournal"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tui"
)

func TestTUIInteractiveTargetCompositionPreservesMultiRunSessionAndCapabilitySurface(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("TARGET_TUI_PROJECT_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	model := &targetTUIProvider{}
	config := foxConfig{
		WorkDir: workDir, Model: "tui-model", EffortOverride: "high", MaxTurns: 4,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "tui-model"},
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
	}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := startup.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	initial := startup.Application.State()
	for _, prompt := range []string{"first", "second"} {
		outcome, err := startup.Application.Run(context.Background(), app.RunCommand{Prompt: prompt}, nil)
		if err != nil || outcome == nil || outcome.FinalMessage != "done:"+prompt || outcome.SessionID != initial.Session.ID {
			t.Fatalf("run %q = %#v/%v", prompt, outcome, err)
		}
	}
	var observations []targetTUIObservation
	for _, observation := range model.snapshot() {
		prompt := lastDirectUserMessage(observation.messages)
		if prompt == "first" || prompt == "second" {
			observations = append(observations, observation)
		}
	}
	if len(observations) != 2 {
		t.Fatalf("provider observations = %d, want 2", len(observations))
	}
	for _, observation := range observations {
		if !strings.Contains(observation.messages[0].Content, "TARGET_TUI_PROJECT_INSTRUCTION") || !strings.Contains(observation.messages[0].Content, "Asking the User") {
			t.Fatalf("TUI system prompt omitted fragments:\n%s", observation.messages[0].Content)
		}
		if observation.options.Effort != "high" {
			t.Fatalf("effort = %q", observation.options.Effort)
		}
		var names []string
		for _, definition := range observation.definitions {
			names = append(names, definition.Name)
		}
		sort.Strings(names)
		if got, want := strings.Join(names, ","), "ask_user_question,bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file"; got != want {
			t.Fatalf("tool surface = %q, want %q", got, want)
		}
	}
	restricted, err := startup.Application.Run(context.Background(), app.RunCommand{
		Prompt: "restricted", AllowedTools: []string{"read_file"},
	}, nil)
	if err != nil || restricted == nil || restricted.FinalMessage != "done:restricted" {
		t.Fatalf("restricted run = %#v/%v", restricted, err)
	}
	restrictedObservation := lastTUIObservationForPrompt(t, model.snapshot(), "restricted")
	var restrictedNames []string
	for _, definition := range restrictedObservation.definitions {
		restrictedNames = append(restrictedNames, definition.Name)
	}
	if got, want := strings.Join(restrictedNames, ","), "read_file"; got != want {
		t.Fatalf("restricted tool surface = %q, want %q", got, want)
	}
	/* A main-surface allowed-tools restriction filters the tool registry only;
	 * the model still receives the full baseline base prompt. */
	restrictedPrompt := restrictedObservation.messages[0].Content
	for _, want := range []string{
		"TARGET_TUI_PROJECT_INSTRUCTION",
		"Prefer reading files before editing them.",
		"After changing code, verify with the smallest relevant test command.",
		"Treat @path tokens in user messages as project-relative file references; read referenced files before making claims or edits about them.",
		"Asking the User",
	} {
		if !strings.Contains(restrictedPrompt, want) {
			t.Fatalf("restricted system prompt omitted baseline guidance %q:\n%s", want, restrictedPrompt)
		}
	}
	denyAll, err := startup.Application.Run(context.Background(), app.RunCommand{
		Prompt: "denyall", AllowedTools: []string{},
	}, nil)
	if err != nil || denyAll == nil || denyAll.FinalMessage != "done:denyall" {
		t.Fatalf("deny-all run = %#v/%v", denyAll, err)
	}
	denyAllObservation := lastTUIObservationForPrompt(t, model.snapshot(), "denyall")
	if len(denyAllObservation.definitions) != 0 {
		t.Fatalf("deny-all tool surface = %#v, want no advertised tools", denyAllObservation.definitions)
	}
	if startup.Registry == nil || startup.Executor == nil || startup.SessionLogDir != initial.Session.Directory {
		t.Fatalf("startup capabilities = registry:%v executor:%v log:%q state:%#v", startup.Registry != nil, startup.Executor != nil, startup.SessionLogDir, initial)
	}

	next, err := startup.Application.NewSession(context.Background(), app.NewSessionCommand{})
	if err != nil || next.Session.ID == "" || next.Session.ID == initial.Session.ID || next.CollaborationMode != "default" {
		t.Fatalf("new session = %#v/%v", next, err)
	}
	if records, err := startup.Application.Conversation(context.Background()); err != nil || len(records) != 0 {
		t.Fatalf("new session conversation = %#v/%v", records, err)
	}
}

func TestTUIInteractiveTargetFormalPlanTransitionsThroughReviewedChecklist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	model := &formalPlanTUIProvider{}
	config := foxConfig{
		WorkDir: workDir, Model: "tui-model", MaxTurns: 6,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "tui-model"},
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: approvePlanPort{},
	}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer startup.Close(context.Background())
	startup.Application.ActivateFullAccess(context.Background(), app.FullAccessCommand{})

	outcome, err := startup.Application.Run(context.Background(), app.RunCommand{
		Prompt: "plan then implement", CollaborationMode: "formal_plan",
	}, nil)
	if err != nil || outcome == nil || outcome.FinalMessage != "implemented" {
		t.Fatalf("formal run = %#v/%v", outcome, err)
	}
	surfaces := model.snapshot()
	if len(surfaces) != 3 || !containsName(surfaces[0], "submit_plan") || containsName(surfaces[0], "write_file") ||
		!containsName(surfaces[1], "update_todo") || containsName(surfaces[1], "submit_plan") || containsName(surfaces[1], "write_file") ||
		!containsName(surfaces[2], "write_file") || containsName(surfaces[2], "submit_plan") {
		t.Fatalf("formal lifecycle surfaces = %#v", surfaces)
	}
	state := startup.Application.State()
	plan, planErr := os.ReadFile(filepath.Join(state.Session.Directory, "PLAN.md"))
	todo, todoErr := os.ReadFile(filepath.Join(state.Session.Directory, "TODO.md"))
	if planErr != nil || string(plan) != "# Approved plan" || todoErr != nil || !strings.Contains(string(todo), "Implement approved plan") {
		t.Fatalf("formal artifacts plan=%q/%v todo=%q/%v", plan, planErr, todo, todoErr)
	}
	if state.CollaborationMode != "default" {
		t.Fatalf("collaboration after approval = %q, want default", state.CollaborationMode)
	}
}

func TestTUIInteractiveTargetModelAndEffortChangesAffectOnlyFutureRuns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	var factoryModels []string
	providers := make(map[string]*modelSnapshotTUIProvider)
	var savedModel string
	config := foxConfig{
		WorkDir: workDir, Model: "model-a", EffortOverride: "high", MaxTurns: 3,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "model-a"},
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
	}, func(config llmconfig.ResolvedConfig) (provider.LLMProvider, error) {
		factoryModels = append(factoryModels, config.Model)
		created := &modelSnapshotTUIProvider{model: config.Model}
		providers[config.Model] = created
		return created, nil
	}, func(model string) error { savedModel = model; return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer startup.Close(context.Background())

	first, err := startup.Application.Run(context.Background(), app.RunCommand{Prompt: "first"}, nil)
	if err != nil || first.FinalMessage != "model-a:first" {
		t.Fatalf("first run = %#v/%v", first, err)
	}
	startup.Application.UpdateEffort(context.Background(), app.EffortCommand{Effort: "low"})
	state, err := startup.Application.UpdateModel(context.Background(), app.ModelCommand{Model: "model-b"})
	if err != nil || state.Model != "model-b" || state.Effort != "low" || savedModel != "model-b" {
		t.Fatalf("updated state = %#v saved=%q err=%v", state, savedModel, err)
	}
	second, err := startup.Application.Run(context.Background(), app.RunCommand{Prompt: "second"}, nil)
	if err != nil || second.FinalMessage != "model-b:second" {
		t.Fatalf("second run = %#v/%v", second, err)
	}
	if got, want := strings.Join(factoryModels, ","), "model-a,model-b"; got != want {
		t.Fatalf("provider factory models = %q, want %q", got, want)
	}
	if providers["model-a"].efforts[0] != "high" || providers["model-b"].efforts[0] != "low" {
		t.Fatalf("run efforts model-a/model-b = %#v/%#v", providers["model-a"].efforts, providers["model-b"].efforts)
	}
}

func TestTUIInteractiveTargetModelPersistenceFailureDoesNotUndoValidatedSwitch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	providers := make(map[string]*modelSnapshotTUIProvider)
	config := foxConfig{
		WorkDir: workDir, Model: "model-a", MaxTurns: 3,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "model-a"},
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
	}, func(config llmconfig.ResolvedConfig) (provider.LLMProvider, error) {
		created := &modelSnapshotTUIProvider{model: config.Model}
		providers[config.Model] = created
		return created, nil
	}, func(string) error { return errors.New("settings write failed") })
	if err != nil {
		t.Fatal(err)
	}
	defer startup.Close(context.Background())

	state, err := startup.Application.UpdateModel(context.Background(), app.ModelCommand{Model: "model-b"})
	if err != nil || state.Model != "model-b" {
		t.Fatalf("model switch state/error = %#v/%v", state, err)
	}
	outcome, err := startup.Application.Run(context.Background(), app.RunCommand{Prompt: "after switch"}, nil)
	if err != nil || outcome.FinalMessage != "model-b:after switch" || providers["model-b"] == nil {
		t.Fatalf("run after persistence failure = %#v/%v providers=%#v", outcome, err, providers)
	}
}

func TestTUIInteractiveTargetCompactionAndRewindUseActiveSessionState(t *testing.T) {
	newStartup := func(t *testing.T) tui.Startup {
		t.Helper()
		t.Setenv("HOME", t.TempDir())
		workDir := t.TempDir()
		config := foxConfig{
			WorkDir: workDir, Model: "tui-model", MaxTurns: 3,
			ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "tui-model"},
		}
		startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
			Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
		}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return &targetTUIProvider{}, nil }, nil)
		if err != nil {
			t.Fatal(err)
		}
		return startup
	}
	runConversation := func(t *testing.T, startup tui.Startup) {
		t.Helper()
		for _, prompt := range []string{"first", "second"} {
			if _, err := startup.Application.Run(context.Background(), app.RunCommand{Prompt: prompt}, nil); err != nil {
				t.Fatalf("run %q: %v", prompt, err)
			}
		}
	}

	t.Run("manual compaction", func(t *testing.T) {
		startup := newStartup(t)
		defer startup.Close(context.Background())
		runConversation(t, startup)
		outcome, err := startup.Application.Compact(context.Background(), app.CompactCommand{Instructions: "retain decisions"})
		if err != nil || outcome.MessagesSummarized < 2 || outcome.PreTokens <= 0 || outcome.PostTokens <= 0 {
			t.Fatalf("compact outcome/error = %#v/%v", outcome, err)
		}
	})

	t.Run("conversation rewind", func(t *testing.T) {
		startup := newStartup(t)
		defer startup.Close(context.Background())
		runConversation(t, startup)
		targets, err := startup.Application.RewindTargets(context.Background())
		if err != nil || len(targets) < 2 {
			t.Fatalf("rewind targets/error = %#v/%v", targets, err)
		}
		var first app.RewindTarget
		found := false
		for _, target := range targets {
			if target.Content == "first" {
				first = target
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("first prompt target absent: %#v", targets)
		}
		outcome := startup.Application.Rewind(context.Background(), app.RewindCommand{Sequence: first.Sequence, Action: app.RewindConversation})
		if !outcome.ConversationAttempted || outcome.ConversationError != "" || outcome.RestoredInput != "first" {
			t.Fatalf("rewind outcome = %#v", outcome)
		}
		for _, record := range outcome.Conversation {
			if record.Content == "second" || record.DisplayContent == "second" {
				t.Fatalf("rewind retained later prompt: %#v", outcome.Conversation)
			}
		}
	})
}

func TestTUIInteractiveTargetForkCarriesParentPermissionEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("TRUSTED_FORK_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var childConfig childruntime.Config
	config := foxConfig{
		WorkDir: workDir, Model: "tui-model", MaxTurns: 3,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "tui-model"},
		NewChildRunner: func(config childruntime.Config) subagent.Runner {
			childConfig = config
			return targetTUIChildRunner{}
		},
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
	}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return &targetTUIProvider{}, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer startup.Close(context.Background())
	t.Setenv("HOME", t.TempDir())

	state := startup.Application.State()
	result, err := startup.Executor.Execute(context.Background(), &slash.Command{
		Type: slash.CommandPrompt, Content: "inspect parent",
		Frontmatter: slash.Frontmatter{Context: "fork", Agent: "general-purpose", AllowedTools: []string{"read_file"}},
	}, "", state.Session.ID)
	if err != nil || !result.Fork || result.Content != "child report" {
		t.Fatalf("fork result/error = %#v/%v", result, err)
	}
	if childConfig.ParentEvidence == nil {
		t.Fatal("fork child ParentEvidence = nil")
	}
	if childConfig.HomeDir != home {
		t.Fatalf("TUI fork child HomeDir = %q, want frozen parent home %q", childConfig.HomeDir, home)
	}
	evidence := childConfig.ParentEvidence(permission.Request{ToolName: "read_file", Action: "read AGENTS.md"})
	if !strings.Contains(evidence.Trusted, "TRUSTED_FORK_INSTRUCTION") {
		t.Fatalf("fork parent evidence omitted project instructions: %q", evidence.Trusted)
	}
}

func TestTUIRuntimeAfterRunReleasesAssemblyResourcesOnEarlyFailure(t *testing.T) {
	runID := session.RunID("run-failed")
	composition := &tuiRuntimeComposition{
		runs: map[session.RunID]tuiRunResources{runID: {}},
		journals: tuiJournalSet{journals: map[session.RunID]*runtimejournal.Journal{
			runID: nil,
		}},
	}

	composition.afterRun(nil, foxruntime.RunResult{RunID: runID}, errors.New("assembly failed"))

	if len(composition.runs) != 0 {
		t.Fatalf("run resources retained after failure: %#v", composition.runs)
	}
	if len(composition.journals.journals) != 0 {
		t.Fatalf("journal retained after failure: %#v", composition.journals.journals)
	}
}

type targetTUIObservation struct {
	messages    []schema.Message
	definitions []schema.ToolDefinition
	options     provider.GenerateOptions
}

type targetTUIProvider struct {
	mu           sync.Mutex
	observations []targetTUIObservation
}

type formalPlanTUIProvider struct {
	mu       sync.Mutex
	surfaces [][]string
}

type modelSnapshotTUIProvider struct {
	model   string
	efforts []string
}

type targetTUIChildRunner struct{}

func (targetTUIChildRunner) Run(context.Context, subagent.Request) (*subagent.Result, error) {
	return &subagent.Result{Report: "child report"}, nil
}

func (targetTUIChildRunner) PermissionEnforced() bool { return true }
func (targetTUIChildRunner) DelegationAllowed() bool  { return true }

func (p *modelSnapshotTUIProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *modelSnapshotTUIProvider) GenerateWithOptions(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	if len(definitions) == 9 {
		p.efforts = append(p.efforts, options.Effort)
	}
	return &provider.GenerateResponse{Message: &schema.Message{
		Role: schema.RoleAssistant, Content: p.model + ":" + lastDirectUserMessage(messages),
	}}, nil
}

func (p *formalPlanTUIProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *formalPlanTUIProvider) GenerateWithOptions(_ context.Context, _ []schema.Message, definitions []schema.ToolDefinition, _ provider.GenerateOptions) (*provider.GenerateResponse, error) {
	var names []string
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	if containsName(names, "submit_plan") {
		p.record(names)
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{
			ID: "plan", Name: "submit_plan", Arguments: []byte(`{"plan_markdown":"# Approved plan"}`),
		}}}}, nil
	}
	if containsName(names, "update_todo") && !containsName(names, "write_file") {
		p.record(names)
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{
			ID: "todo", Name: "update_todo", Arguments: []byte(`{"content":"# TODO\n\n- [ ] Implement approved plan\n"}`),
		}}}}, nil
	}
	if containsName(names, "write_file") {
		p.record(names)
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "implemented"}}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
}

func (p *formalPlanTUIProvider) record(names []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.surfaces = append(p.surfaces, append([]string(nil), names...))
}

func (p *formalPlanTUIProvider) snapshot() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([][]string, len(p.surfaces))
	for index, surface := range p.surfaces {
		result[index] = append([]string(nil), surface...)
	}
	return result
}

func containsName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func lastTUIObservationForPrompt(t *testing.T, observations []targetTUIObservation, prompt string) targetTUIObservation {
	t.Helper()
	for i := len(observations) - 1; i >= 0; i-- {
		if lastDirectUserMessage(observations[i].messages) == prompt {
			return observations[i]
		}
	}
	t.Fatalf("provider observation for prompt %q absent in %#v", prompt, observations)
	return targetTUIObservation{}
}

func (p *targetTUIProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *targetTUIProvider) GenerateWithOptions(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.observations = append(p.observations, targetTUIObservation{
		messages: append([]schema.Message(nil), messages...), definitions: append([]schema.ToolDefinition(nil), definitions...), options: options,
	})
	p.mu.Unlock()
	prompt := lastDirectUserMessage(messages)
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done:" + prompt}}, nil
}

func (p *targetTUIProvider) snapshot() []targetTUIObservation {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]targetTUIObservation(nil), p.observations...)
}

type denyPermissionPort struct{}

func (denyPermissionPort) RequestPermission(_ context.Context, request app.PermissionRequest) (app.PermissionResponse, error) {
	return app.PermissionResponse{CorrelationID: request.Correlation.ID, Decision: app.PermissionDeny}, nil
}

type cancelQuestionPort struct{}

func (cancelQuestionPort) AskQuestions(_ context.Context, request app.QuestionRequest) (app.QuestionResponse, error) {
	return app.QuestionResponse{CorrelationID: request.Correlation.ID}, app.ErrQuestionCancelled
}

type cancelPlanPort struct{}

func (cancelPlanPort) ReviewPlan(_ context.Context, request app.PlanReviewRequest) (app.PlanReviewResponse, error) {
	return app.PlanReviewResponse{CorrelationID: request.Correlation.ID}, app.ErrPlanReviewCancelled
}

type approvePlanPort struct{}

func (approvePlanPort) ReviewPlan(_ context.Context, request app.PlanReviewRequest) (app.PlanReviewResponse, error) {
	return app.PlanReviewResponse{CorrelationID: request.Correlation.ID, Decision: app.PlanApproved}, nil
}
