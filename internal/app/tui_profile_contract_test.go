package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestPFTUI001CurrentProfileSnapshotIsFlatAndRestrictionsOnlyNarrow(t *testing.T) {
	runner, sess := newTUIProfileRunner(t, &profilePromptProvider{})
	base := runner.buildRegistry(sess, runner.llmProvider, nil, func() string { return "" })

	got := currentTUIProfileSnapshot{
		name:                 "TUIInteractive",
		sessionSource:        string(runner.currentSession.Source),
		workspace:            runner.WorkDir(),
		maxTurns:             runner.maxTurns,
		serializedRuns:       true,
		modelMutableNextRun:  true,
		effortMutableNextRun: true,
		interactiveQuestion:  runner.userAsker != nil,
		interactivePlan:      runner.planReviewer != nil,
		permissionModes:      "ask,approve,full-access",
		memory:               runner.store != nil && runner.autoMemory != nil,
		checkpointAndRewind:  true,
		autoAndManualCompact: true,
		observation:          "ordered-lifecycle-and-deltas",
		canonicalTools:       canonicalProfileToolNames(base.GetAvailableTools()),
	}
	want := currentTUIProfileSnapshot{
		name:                 "TUIInteractive",
		sessionSource:        string(session.SOURCECLI),
		workspace:            runner.WorkDir(),
		maxTurns:             7,
		serializedRuns:       true,
		modelMutableNextRun:  true,
		effortMutableNextRun: true,
		interactiveQuestion:  true,
		interactivePlan:      true,
		permissionModes:      "ask,approve,full-access",
		memory:               true,
		checkpointAndRewind:  true,
		autoAndManualCompact: true,
		observation:          "ordered-lifecycle-and-deltas",
		canonicalTools:       "ask_user_question,bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file",
	}
	if got != want {
		t.Fatalf("current TUI profile snapshot = %#v, want %#v", got, want)
	}

	restricted := slash.NewFilteredRegistry(base, []string{"read_file", "not-in-profile"})
	if names := canonicalProfileToolNames(restricted.GetAvailableTools()); names != "read_file" {
		t.Fatalf("restricted canonical tools = %q, want read_file", names)
	}
	if result := restricted.Execute(context.Background(), schema.ToolCall{ID: "expand", Name: "not-in-profile"}); !result.IsError {
		t.Fatalf("restriction expanded profile with unknown tool: %#v", result)
	}

	runner.model = "mutated-after-snapshot"
	runner.maxTurns = 99
	if got != want {
		t.Fatalf("captured profile changed after runner mutation: %#v", got)
	}
}

func TestPFTUI002SessionSelectionPreservesCurrentCLIContracts(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	existingCLI, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(session.CreateOptions{Source: session.SOURCEFeishu, WorkDir: workDir}); err != nil {
		t.Fatal(err)
	}

	explicit, err := resolveRunnerSession(manager, workDir, AgentRunnerConfig{SessionID: string(existingCLI.ID)})
	if err != nil || explicit.ID != existingCLI.ID {
		t.Fatalf("explicit session = %#v, %v; want %s", explicit, err, existingCLI.ID)
	}
	continued, err := resolveRunnerSession(manager, workDir, AgentRunnerConfig{ContinueSession: true})
	if err != nil || continued.ID != existingCLI.ID || continued.Source != session.SOURCECLI {
		t.Fatalf("continued session = %#v, %v; want latest CLI %s", continued, err, existingCLI.ID)
	}

	created, err := resolveRunnerSession(manager, workDir, AgentRunnerConfig{})
	if err != nil || created.ID == existingCLI.ID || created.Source != session.SOURCECLI {
		t.Fatalf("default session = %#v, %v; want fresh CLI session", created, err)
	}
	forced, err := resolveRunnerSession(manager, workDir, AgentRunnerConfig{NewSession: true})
	if err != nil || forced.ID == existingCLI.ID || forced.ID == created.ID || forced.Source != session.SOURCECLI {
		t.Fatalf("forced session = %#v, %v; want another fresh CLI session", forced, err)
	}

	conflicts := []struct {
		name string
		cfg  AgentRunnerConfig
		want string
	}{
		{name: "new and explicit", cfg: AgentRunnerConfig{NewSession: true, SessionID: string(existingCLI.ID)}, want: "-new 不能和 -session 或 -continue 同时使用"},
		{name: "new and continue", cfg: AgentRunnerConfig{NewSession: true, ContinueSession: true}, want: "-new 不能和 -session 或 -continue 同时使用"},
		{name: "explicit and continue", cfg: AgentRunnerConfig{SessionID: string(existingCLI.ID), ContinueSession: true}, want: "-session 不能和 -continue 同时使用"},
	}
	for _, tc := range conflicts {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveRunnerSession(manager, workDir, tc.cfg); err == nil || err.Error() != tc.want {
				t.Fatalf("resolveRunnerSession() error = %v, want %q", err, tc.want)
			}
		})
	}

	emptyManager := session.NewManagerWithHome(workDir, t.TempDir())
	if _, err := resolveRunnerSession(emptyManager, workDir, AgentRunnerConfig{ContinueSession: true}); err == nil || err.Error() != "没有可继续的 CLI Session" {
		t.Fatalf("continue without CLI error = %v", err)
	}
	if _, err := resolveRunnerSession(manager, workDir, AgentRunnerConfig{SessionID: "missing"}); err == nil || err.Error() != "Session missing 不存在" {
		t.Fatalf("missing explicit session error = %v", err)
	}
}

func TestPFTUI008RootToolSurfaceAndAliasesShareOneCapabilityCeiling(t *testing.T) {
	runner, sess := newTUIProfileRunner(t, &profilePromptProvider{})
	interactive := runner.buildRegistry(sess, runner.llmProvider, nil, func() string { return "" })
	if got, want := profileToolNames(interactive.GetAvailableTools()), []string{
		"AskUserQuestion",
		"ask_user_question",
		"bash",
		"delegate_task",
		"edit_file",
		"read_file",
		"read_todo",
		"skill",
		"update_todo",
		"write_file",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("interactive definitions = %#v, want %#v", got, want)
	}
	if interactive.IsParallelSafe("AskUserQuestion") || interactive.IsParallelSafe("ask_user_question") {
		t.Fatal("question alias and canonical name must both remain exclusive")
	}
	assessor, ok := interactive.(tools.PermissionRegistry)
	if !ok {
		t.Fatalf("interactive registry %T does not expose permission lookup", interactive)
	}
	canonicalAssessment, canonicalFound, canonicalErr := assessor.AssessPermission("ask_user_question", toolpolicy.Context{}, nil)
	aliasAssessment, aliasFound, aliasErr := assessor.AssessPermission("AskUserQuestion", toolpolicy.Context{}, nil)
	if canonicalErr != nil || aliasErr != nil || !canonicalFound || !aliasFound || !reflect.DeepEqual(canonicalAssessment, aliasAssessment) {
		t.Fatalf("question permission alias mismatch: canonical=%#v/%v/%v alias=%#v/%v/%v", canonicalAssessment, canonicalFound, canonicalErr, aliasAssessment, aliasFound, aliasErr)
	}

	runner.SetUserAsker(nil)
	headless := runner.buildRegistry(sess, runner.llmProvider, nil, func() string { return "" })
	for _, name := range profileToolNames(headless.GetAvailableTools()) {
		if name == "ask_user_question" || name == "AskUserQuestion" {
			t.Fatalf("non-interactive registry advertised %q", name)
		}
	}
}

func TestPFTUI007EffortSwitchCannotMutateActiveRunSnapshot(t *testing.T) {
	modelProvider := &profileEffortProvider{
		entered: make(chan string, 2),
		release: make(chan struct{}, 2),
	}
	runner, _ := newLifecycleAgentRunner(t, t.TempDir(), modelProvider, nil)
	runner.providerProtocol = "openai"
	runner.effortOverride = "low"

	firstDone := make(chan error, 1)
	go func() {
		_, err := runner.Run(context.Background(), "first effort", nil)
		firstDone <- err
	}()
	if got := <-modelProvider.entered; got != "low" {
		t.Fatalf("first run effort = %q, want low", got)
	}
	runner.SetEffortOverride("high")
	modelProvider.release <- struct{}{}
	if err := <-firstDone; err != nil {
		t.Fatalf("first Run() error = %v", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := runner.Run(context.Background(), "second effort", nil)
		secondDone <- err
	}()
	if got := <-modelProvider.entered; got != "high" {
		t.Fatalf("second run effort = %q, want high", got)
	}
	modelProvider.release <- struct{}{}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
}

func TestPFTUI007ModelSwitchWaitsForActiveRunSnapshot(t *testing.T) {
	modelProvider := &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})}
	runner, _ := newLifecycleAgentRunner(t, t.TempDir(), modelProvider, nil)
	runner.llmConfig = testResolvedLLM("openai", "old-model")
	runner.model = "old-model"
	runner.providerProtocol = "openai"

	runDone := make(chan error, 1)
	go func() {
		_, err := runner.Run(context.Background(), "active model snapshot", nil)
		runDone <- err
	}()
	select {
	case <-modelProvider.entered:
	case <-time.After(time.Second):
		t.Fatal("run did not enter old provider")
	}

	switchDone := make(chan error, 1)
	go func() { switchDone <- runner.SetModel("new-model") }()
	select {
	case err := <-switchDone:
		t.Fatalf("SetModel completed during active run: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if runner.Model() != "old-model" {
		t.Fatalf("active model changed to %q", runner.Model())
	}

	close(modelProvider.release)
	if err := <-runDone; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := <-switchDone; err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if runner.Model() != "new-model" || runner.llmConfig.Model != "new-model" {
		t.Fatalf("future model snapshot = %q/%q, want new-model", runner.Model(), runner.llmConfig.Model)
	}
}

func TestPFTUI013EveryRunReceivesAllCurrentPromptFragments(t *testing.T) {
	provider := &profilePromptProvider{}
	runner, _ := newTUIProfileRunner(t, provider)
	runner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}

	for _, prompt := range []string{"first profile turn", "second profile turn"} {
		result, err := runner.Run(context.Background(), prompt, nil)
		if err != nil || result == nil || result.FinalMessage != "done:"+prompt {
			t.Fatalf("Run(%q) = %#v, %v", prompt, result, err)
		}
	}

	seen := provider.systemPrompts()
	if len(seen) != 2 {
		t.Fatalf("system prompt count = %d, want 2", len(seen))
	}
	for i, systemPrompt := range seen {
		for _, fragment := range []string{
			"PROFILE_PROJECT_INSTRUCTION",
			"PROFILE_SESSION_MEMORY",
			"profile-memory.md",
			"profile-skill",
			"ask_user_question",
		} {
			if !strings.Contains(systemPrompt, fragment) {
				t.Fatalf("run %d system prompt missing %q:\n%s", i+1, fragment, systemPrompt)
			}
		}
		if strings.Contains(systemPrompt, "Formal Plan Collaboration Mode") {
			t.Fatalf("default-mode run %d received Formal Plan fragment", i+1)
		}
		for _, fragment := range []string{"PROFILE_PROJECT_INSTRUCTION", "PROFILE_SESSION_MEMORY", "profile-memory.md", "profile-skill"} {
			if strings.Count(systemPrompt, fragment) != 1 {
				t.Fatalf("run %d system prompt contains %q %d times, want once", i+1, fragment, strings.Count(systemPrompt, fragment))
			}
		}
	}
}

func TestPFTUI018CoreCapabilitiesDoNotOwnTerminalPresentation(t *testing.T) {
	paths := []string{"runner.go", "plan_lifecycle.go"}
	engineEntries, err := os.ReadDir(filepath.Join("..", "engine"))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range engineEntries {
		if !item.IsDir() && strings.HasSuffix(item.Name(), ".go") && !strings.HasSuffix(item.Name(), "_test.go") {
			paths = append(paths, filepath.Join("..", "engine", item.Name()))
		}
	}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"github.com/charmbracelet",
			"internal/tui",
			"fmt.Print",
			"os.Stdout",
			"os.Stderr",
			"\\x1b[",
			"\x1b[",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("core capability %s owns terminal presentation token %q", path, forbidden)
			}
		}
	}
}

type currentTUIProfileSnapshot struct {
	name                 string
	sessionSource        string
	workspace            string
	maxTurns             int
	serializedRuns       bool
	modelMutableNextRun  bool
	effortMutableNextRun bool
	interactiveQuestion  bool
	interactivePlan      bool
	permissionModes      string
	memory               bool
	checkpointAndRewind  bool
	autoAndManualCompact bool
	observation          string
	canonicalTools       string
}

type profilePromptProvider struct {
	mu      sync.Mutex
	systems []string
}

type profileEffortProvider struct {
	entered chan string
	release chan struct{}
}

func (p *profileEffortProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *profileEffortProvider) GenerateWithOptions(ctx context.Context, _ []schema.Message, _ []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.entered <- options.Effort
	select {
	case <-p.release:
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *profilePromptProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	systemPrompt := ""
	if len(messages) > 0 && messages[0].Role == schema.RoleSystem {
		systemPrompt = messages[0].Content
	}
	prompt := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.RoleUser && messages[i].ToolCallID == "" {
			prompt = messages[i].Content
			break
		}
	}
	p.mu.Lock()
	p.systems = append(p.systems, systemPrompt)
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done:" + prompt}}, nil
}

func (p *profilePromptProvider) systemPrompts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.systems...)
}

type profileAsker struct{}

func (profileAsker) Ask(context.Context, []tools.Question) ([]tools.Answer, error) {
	return nil, nil
}

type profilePlanReviewer struct{}

func (profilePlanReviewer) ReviewPlan(context.Context, string) (tools.PlanReview, error) {
	return tools.PlanReview{}, nil
}

func newTUIProfileRunner(t *testing.T, modelProvider provider.LLMProvider) (*AgentRunner, *session.Session) {
	t.Helper()
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("PROFILE_PROJECT_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sess.MemoryPath(), []byte("PROFILE_SESSION_MEMORY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	autoMemory := automemory.NewStore(manager.HomeDir(), workDir)
	if err := autoMemory.Save(automemory.Memory{
		Name:        "profile-memory",
		Description: "Profile contract memory.",
		Type:        automemory.TypeProject,
		Body:        "Profile contract fact.\n\n**Why:** freezes injection.\n**How to apply:** include the index.",
	}); err != nil {
		t.Fatal(err)
	}
	registry := slash.NewRegistry(workDir)
	registry.Register(&slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "profile-skill",
		Description: "Profile contract skill.",
		Source:      slash.SourceProject,
		Frontmatter: slash.Frontmatter{UserInvocable: true},
	})
	state := permission.NewState(permission.ModeAsk, false)
	coordinator := permission.NewCoordinator(permission.Config{State: state, Workspace: workDir, CWD: workDir, Source: permission.SourceMain})
	runner := &AgentRunner{
		workDir:               workDir,
		model:                 "profile-model",
		providerProtocol:      "scripted",
		collaborationMode:     collaboration.ModeDefault,
		maxTurns:              7,
		store:                 store,
		autoMemory:            autoMemory,
		manager:               manager,
		llmProvider:           modelProvider,
		currentSession:        sess,
		slashRegistry:         registry,
		slashExecutor:         slash.NewExecutor(slash.WithWorkDir(workDir)),
		userAsker:             profileAsker{},
		planReviewer:          profilePlanReviewer{},
		permissionCoordinator: coordinator,
	}
	return runner, sess
}

func profileToolNames(definitions []schema.ToolDefinition) []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	return names
}

func canonicalProfileToolNames(definitions []schema.ToolDefinition) string {
	names := profileToolNames(definitions)
	canonical := names[:0]
	for _, name := range names {
		if name != "AskUserQuestion" {
			canonical = append(canonical, name)
		}
	}
	return strings.Join(canonical, ",")
}
