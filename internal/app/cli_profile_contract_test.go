package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestPFCLI001CurrentProfileSnapshotIsFlatAndRestrictionsOnlyNarrow(t *testing.T) {
	runner, sess := newCLIProfileRunner(t, &cliProfileProvider{})
	base := runner.buildRegistry(sess, runner.llmProvider, runner.checkpointer, func() string { return "" })

	got := currentCLIProfileSnapshot{
		name:                  "CLIExec",
		sessionSource:         string(sess.Source),
		workspace:             runner.WorkDir(),
		oneSynchronousRun:     true,
		maxTurns:              runner.maxTurns,
		model:                 runner.model,
		effort:                runner.effortOverride,
		thinking:              runner.enableThinking,
		interactiveQuestion:   runner.userAsker != nil,
		interactivePlan:       runner.planReviewer != nil,
		permissionCoordinator: runner.permissionCoordinator != nil,
		memory:                runner.store != nil && runner.autoMemory != nil,
		checkpoint:            runner.checkpointer != nil,
		automaticCompaction:   true,
		manualCompaction:      false,
		extraction:            "tracked-and-drained",
		observation:           "final-only",
		canonicalTools:        cliProfileToolNames(base.GetAvailableTools()),
	}
	want := currentCLIProfileSnapshot{
		name:                "CLIExec",
		sessionSource:       string(session.SOURCECLI),
		workspace:           runner.WorkDir(),
		oneSynchronousRun:   true,
		maxTurns:            5,
		model:               "cli-profile-model",
		effort:              "high",
		thinking:            true,
		memory:              true,
		checkpoint:          true,
		automaticCompaction: true,
		manualCompaction:    false,
		extraction:          "tracked-and-drained",
		observation:         "final-only",
		canonicalTools:      "bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file",
	}
	if got != want {
		t.Fatalf("current CLI profile snapshot = %#v, want %#v", got, want)
	}

	restricted := slash.NewFilteredRegistry(base, []string{"read_file", "not-in-profile"})
	if names := cliProfileToolNames(restricted.GetAvailableTools()); names != "read_file" {
		t.Fatalf("restricted tools = %q, want read_file", names)
	}
	if result := restricted.Execute(context.Background(), schema.ToolCall{ID: "expand", Name: "not-in-profile"}); !result.IsError {
		t.Fatalf("run restriction expanded the CLI profile: %#v", result)
	}

	runner.model = "later-model"
	runner.maxTurns = 99
	if got != want {
		t.Fatalf("captured CLI profile changed after runner mutation: %#v", got)
	}
}

func TestPFCLI002SessionSelectionPreservesCurrentContracts(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	existingCLI, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(session.CreateOptions{Source: session.SOURCEFeishu, WorkDir: workDir}); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name string
		cfg  CLIConfig
		want string
	}{
		{name: "explicit", cfg: CLIConfig{SessionID: string(existingCLI.ID)}, want: string(existingCLI.ID)},
		{name: "continue latest CLI", cfg: CLIConfig{ContinueSession: true}, want: string(existingCLI.ID)},
		{name: "default fresh", cfg: CLIConfig{}},
		{name: "forced fresh", cfg: CLIConfig{NewSession: true}},
	}
	created := map[string]bool{string(existingCLI.ID): true}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			sess, err := resolveCLISession(manager, workDir, tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && string(sess.ID) != tc.want {
				t.Fatalf("session = %q, want %q", sess.ID, tc.want)
			}
			if tc.want == "" && created[string(sess.ID)] {
				t.Fatalf("fresh selection reused session %q", sess.ID)
			}
			if sess.Source != session.SOURCECLI {
				t.Fatalf("session source = %q, want CLI", sess.Source)
			}
			created[string(sess.ID)] = true
		})
	}

	for _, tc := range []struct {
		cfg  CLIConfig
		want string
	}{
		{cfg: CLIConfig{NewSession: true, SessionID: string(existingCLI.ID)}, want: "-new 不能和 -session 或 -continue 同时使用"},
		{cfg: CLIConfig{NewSession: true, ContinueSession: true}, want: "-new 不能和 -session 或 -continue 同时使用"},
		{cfg: CLIConfig{SessionID: string(existingCLI.ID), ContinueSession: true}, want: "-session 不能和 -continue 同时使用"},
		{cfg: CLIConfig{SessionID: "missing"}, want: "Session missing 不存在"},
	} {
		if _, err := resolveCLISession(manager, workDir, tc.cfg); err == nil || err.Error() != tc.want {
			t.Fatalf("resolveCLISession(%#v) error = %v, want %q", tc.cfg, err, tc.want)
		}
	}
	empty := session.NewManagerWithHome(workDir, t.TempDir())
	if _, err := resolveCLISession(empty, workDir, CLIConfig{ContinueSession: true}); err == nil || err.Error() != "没有可继续的 CLI Session" {
		t.Fatalf("empty continue error = %v", err)
	}
}

func TestPFCLI003ConfigAndOneRunInvocationAreFrozen(t *testing.T) {
	resolved := llmconfig.ResolvedConfig{ProviderID: "profile", Protocol: "openai", BaseURL: "https://example.test", Model: "cli-model"}
	cfg := CLIConfig{
		WorkDir: "/tmp/cli-profile", Model: "cli-model", ResolvedLLM: resolved,
		EffortOverride: "high", EnableThinking: true, MaxTurns: 4,
		SessionID: "session-one", ContinueSession: false, NewSession: false,
	}
	wantConfig := AgentRunnerConfig{
		WorkDir: "/tmp/cli-profile", Model: "cli-model", LLM: resolved,
		EffortOverride: "high", EnableThinking: true, MaxTurns: 4, SessionID: "session-one",
	}
	if got := agentRunnerConfigFromCLI(cfg); !reflect.DeepEqual(got, wantConfig) {
		t.Fatalf("agentRunnerConfigFromCLI() = %#v, want %#v", got, wantConfig)
	}

	modelProvider := &cliProfileProvider{}
	runner, _ := newCLIProfileRunner(t, modelProvider)
	runner.enableThinking = false
	runner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}
	result, err := runner.Run(context.Background(), "one synchronous request", nil)
	if err != nil || result == nil || result.FinalMessage != "done:one synchronous request" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	if modelProvider.callCount() != 1 {
		t.Fatalf("provider calls = %d, want one tool-free call", modelProvider.callCount())
	}
	observed := modelProvider.snapshot()[0]
	if observed.options.Effort != "high" || lastDirectUser(observed.messages) != "one synchronous request" {
		t.Fatalf("invocation snapshot = %#v", observed)
	}
}

func TestPFCLI004ToolSurfaceAndUndecoratedExecutionAgree(t *testing.T) {
	runner, sess := newCLIProfileRunner(t, &cliProfileProvider{})
	registry := runner.buildRegistry(sess, runner.llmProvider, runner.checkpointer, func() string { return "" })
	if got, want := cliProfileToolNames(registry.GetAvailableTools()), "bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file"; got != want {
		t.Fatalf("CLI tool definitions = %q, want %q", got, want)
	}
	if registryHasTool(registry, "ask_user_question") || registryHasTool(registry, "submit_plan") {
		t.Fatalf("CLI registry exposed an interactive tool: %#v", registry.GetAvailableTools())
	}

	assessor, ok := registry.(tools.PermissionRegistry)
	if !ok {
		t.Fatalf("registry %T does not expose permission lookup", registry)
	}
	assessment, found, err := assessor.AssessPermission("read_file", toolpolicy.Context{Workspace: runner.WorkDir(), CWD: runner.WorkDir()}, []byte(`{"path":"fixture.txt"}`))
	if err != nil || !found || assessment.Behavior != toolpolicy.BehaviorFastAllow {
		t.Fatalf("read_file assessment = %#v/%v/%v", assessment, found, err)
	}
	if !registry.IsParallelSafe("read_file") {
		t.Fatal("read_file parallel-safety lookup disagrees with its current capability")
	}
	if err := os.WriteFile(filepath.Join(runner.WorkDir(), "fixture.txt"), []byte("cli fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := registry.Execute(context.Background(), schema.ToolCall{ID: "read", Name: "read_file", Arguments: []byte(`{"path":"fixture.txt"}`)})
	if result.IsError || !strings.Contains(result.Output, "cli fixture") {
		t.Fatalf("advertised read_file execution = %#v", result)
	}
}

func TestPFCLI005And006NoInteractivePortsAndSlashPromptIsModelInput(t *testing.T) {
	modelProvider := &cliProfileProvider{}
	runner, _ := newCLIProfileRunner(t, modelProvider)
	runner.enableThinking = false
	runner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}
	if runner.userAsker != nil || runner.planReviewer != nil || runner.permissionCoordinator != nil || runner.collaborationMode != collaboration.ModeDefault {
		t.Fatalf("CLI interaction wiring = asker %T reviewer %T permission %T mode %q", runner.userAsker, runner.planReviewer, runner.permissionCoordinator, runner.collaborationMode)
	}

	result, err := runner.Run(context.Background(), "/help is ordinary CLI model input", nil)
	if err != nil || result == nil || result.FinalMessage != "done:/help is ordinary CLI model input" {
		t.Fatalf("slash-prefixed Run() = %#v, %v", result, err)
	}
	observed := modelProvider.snapshot()[0]
	if lastDirectUser(observed.messages) != "/help is ordinary CLI model input" {
		t.Fatalf("provider did not receive slash prompt unchanged: %#v", observed.messages)
	}
	if strings.Contains(observed.messages[0].Content, "ask_user_question") || strings.Contains(observed.messages[0].Content, "Formal Plan Collaboration Mode") {
		t.Fatalf("CLI system prompt advertised unavailable interaction:\n%s", observed.messages[0].Content)
	}
	if got := cliProfileToolNames(observed.definitions); got != "bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file" {
		t.Fatalf("model-visible skills/delegation surface = %q", got)
	}
}

func TestPFCLI007FreshAndResumedRunsComposeContextOnce(t *testing.T) {
	modelProvider := &cliProfileProvider{}
	runner, sess := newCLIProfileRunner(t, modelProvider)
	runner.enableThinking = false
	runner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}
	if _, err := session.NewMessageLog(sess).Append("prior-run", schema.Message{Role: schema.RoleUser, Content: "prior CLI request"}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewMessageLog(sess).Append("prior-run", schema.Message{Role: schema.RoleAssistant, Content: "prior CLI answer"}); err != nil {
		t.Fatal(err)
	}

	if _, err := runner.Run(context.Background(), "resumed CLI request", nil); err != nil {
		t.Fatal(err)
	}
	observed := modelProvider.snapshot()[0]
	if len(observed.messages) < 4 {
		t.Fatalf("resumed messages = %#v", observed.messages)
	}
	systemPrompt := observed.messages[0].Content
	for _, fragment := range []string{"CLI_PROFILE_PROJECT_INSTRUCTION", "CLI_PROFILE_SESSION_MEMORY", "cli-profile-memory.md", "cli-profile-skill"} {
		if strings.Count(systemPrompt, fragment) != 1 {
			t.Fatalf("system prompt contains %q %d times, want once:\n%s", fragment, strings.Count(systemPrompt, fragment), systemPrompt)
		}
	}
	joined := messageContents(observed.messages)
	for _, content := range []string{"prior CLI request", "prior CLI answer", "resumed CLI request"} {
		if strings.Count(joined, content) != 1 {
			t.Fatalf("resumed projection contains %q %d times:\n%s", content, strings.Count(joined, content), joined)
		}
	}
}

func TestPFCLI008RunCreatesCheckpointAndStateWithoutCLIRewindEntry(t *testing.T) {
	modelProvider := &cliProfileProvider{}
	runner, _ := newCLIProfileRunner(t, modelProvider)
	runner.enableThinking = false
	runner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}
	cp := &cliRecordingCheckpointer{}
	runner.checkpointer = cp
	if err := os.WriteFile(runner.store.PlanPath(), []byte("baseline plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runner.store.TodoPath(), []byte("baseline todo"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runner.Run(context.Background(), "checkpoint CLI request", nil); err != nil {
		t.Fatal(err)
	}
	if len(cp.snapshots) != 1 || cp.snapshots[0] == "" {
		t.Fatalf("checkpoint snapshots = %#v, want one user-message snapshot", cp.snapshots)
	}
	if err := os.WriteFile(runner.store.PlanPath(), []byte("mutated plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := memory.NewStateHistory(runner.store).RestoreBeforeMessage(0); err != nil {
		t.Fatalf("state snapshot is not consumable: %v", err)
	}
	if data, err := os.ReadFile(runner.store.PlanPath()); err != nil || string(data) != "baseline plan" {
		t.Fatalf("restored PLAN = %q, %v", data, err)
	}

	cliSource, err := os.ReadFile("cli.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"CompactNow(", "TruncateMessageHistory(", "RestoreSessionStateBefore(", "rewind"} {
		if strings.Contains(string(cliSource), forbidden) {
			t.Fatalf("CLI adapter exposes manual restore token %q", forbidden)
		}
	}
}

func TestPFCLI010ResultPrecedesTrackedExtractionAndDrainJoins(t *testing.T) {
	modelProvider := &cliExtractionBarrierProvider{
		extractionStarted: make(chan struct{}),
		releaseExtraction: make(chan struct{}),
	}
	runner, _ := newCLIProfileRunner(t, modelProvider)
	runner.enableThinking = false

	result, err := runner.Run(context.Background(), "extract after result", nil)
	if err != nil || result == nil || result.FinalMessage != "main result" || result.RunID == "" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	select {
	case <-modelProvider.extractionStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("tracked extraction did not start")
	}
	drained := make(chan error, 1)
	go func() { drained <- runner.DrainExtraction(context.Background()) }()
	select {
	case err := <-drained:
		t.Fatalf("DrainExtraction returned before extraction completion: %v", err)
	default:
	}
	close(modelProvider.releaseExtraction)
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
}

func TestPFCLI012SuccessfulResultAndArtifactsAreCorrelatedOnce(t *testing.T) {
	modelProvider := &cliProfileProvider{usage: schema.Usage{InputTokens: 7, OutputTokens: 3}}
	runner, sess := newCLIProfileRunner(t, modelProvider)
	runner.enableThinking = false
	runner.extractionFire = func(*session.Session, string, *automemory.Tracker) {}
	result, err := runner.Run(context.Background(), "artifact request", nil)
	if err != nil || result == nil {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
	if result.SessionID != string(sess.ID) || result.RunID == "" || result.FinalMessage != "done:artifact request" {
		t.Fatalf("result identity = %#v", result)
	}
	for _, path := range []string{runner.TranscriptPath(), result.MetricsPath, result.TracePath} {
		if path == "" {
			t.Fatal("successful result contains an empty artifact path")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("artifact %q: %v", path, err)
		}
	}
	records, err := session.NewMessageLog(sess).LoadRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].RunID != session.RunID(result.RunID) || records[1].RunID != session.RunID(result.RunID) || records[1].Message.Usage == nil || records[1].Message.Usage.InputTokens != 7 || records[1].Message.Usage.OutputTokens != 3 {
		t.Fatalf("correlated records = %#v", records)
	}
}

func TestPFCLI014CoreCapabilitiesDoNotOwnPrintPresentation(t *testing.T) {
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
		for _, forbidden := range []string{"internal/tui", "github.com/charmbracelet", "fmt.Print", "os.Stdout", "os.Stderr", "\\x1b["} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("core capability %s owns CLI presentation token %q", path, forbidden)
			}
		}
	}
	adapter, err := os.ReadFile("cli.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"fmt.Fprintln", "Session: ", "Transcript: ", "Metrics: ", "Trace: "} {
		if !strings.Contains(string(adapter), label) {
			t.Fatalf("current CLI adapter no longer owns expected presentation token %q", label)
		}
	}
}

type currentCLIProfileSnapshot struct {
	name                  string
	sessionSource         string
	workspace             string
	oneSynchronousRun     bool
	maxTurns              int
	model                 string
	effort                string
	thinking              bool
	interactiveQuestion   bool
	interactivePlan       bool
	permissionCoordinator bool
	memory                bool
	checkpoint            bool
	automaticCompaction   bool
	manualCompaction      bool
	extraction            string
	observation           string
	canonicalTools        string
}

type cliProfileObservation struct {
	messages    []schema.Message
	definitions []schema.ToolDefinition
	options     provider.GenerateOptions
}

type cliProfileProvider struct {
	mu           sync.Mutex
	observations []cliProfileObservation
	usage        schema.Usage
}

func (p *cliProfileProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *cliProfileProvider) GenerateWithOptions(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.observations = append(p.observations, cliProfileObservation{
		messages:    append([]schema.Message(nil), messages...),
		definitions: append([]schema.ToolDefinition(nil), definitions...),
		options:     options,
	})
	p.mu.Unlock()
	prompt := lastDirectUser(messages)
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done:" + prompt}, Usage: p.usage}, nil
}

func (p *cliProfileProvider) snapshot() []cliProfileObservation {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]cliProfileObservation(nil), p.observations...)
}

func (p *cliProfileProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.observations)
}

type cliExtractionBarrierProvider struct {
	mu                sync.Mutex
	calls             int
	extractionStarted chan struct{}
	releaseExtraction chan struct{}
	extractionOnce    sync.Once
}

func (p *cliExtractionBarrierProvider) Generate(ctx context.Context, _ []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "main result"}}, nil
	}
	p.extractionOnce.Do(func() { close(p.extractionStarted) })
	select {
	case <-p.releaseExtraction:
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *cliExtractionBarrierProvider) GenerateWithOptions(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition, _ provider.GenerateOptions) (*provider.GenerateResponse, error) {
	return p.Generate(ctx, messages, definitions)
}

type cliRecordingCheckpointer struct {
	snapshots []string
}

func (*cliRecordingCheckpointer) TrackEdit(string, string) error { return nil }
func (c *cliRecordingCheckpointer) MakeSnapshot(messageID string) error {
	c.snapshots = append(c.snapshots, messageID)
	return nil
}
func (*cliRecordingCheckpointer) Rewind(string) ([]string, error) { return nil, errors.New("unused") }
func (*cliRecordingCheckpointer) GetDiffStats(string) (*checkpoint.DiffStats, error) {
	return &checkpoint.DiffStats{}, nil
}
func (*cliRecordingCheckpointer) HasAnyChanges(string) (bool, error) { return false, nil }
func (*cliRecordingCheckpointer) SetDisabled(bool)                   {}
func (*cliRecordingCheckpointer) IsDisabled() bool                   { return false }
func (*cliRecordingCheckpointer) RestoreStateFromLog() error         { return nil }

func newCLIProfileRunner(t *testing.T, modelProvider provider.LLMProvider) (*AgentRunner, *session.Session) {
	t.Helper()
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("CLI_PROFILE_PROJECT_INSTRUCTION\n"), 0o644); err != nil {
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
	if err := os.WriteFile(sess.MemoryPath(), []byte("CLI_PROFILE_SESSION_MEMORY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	autoMemory := automemory.NewStore(manager.HomeDir(), workDir)
	if err := autoMemory.Save(automemory.Memory{
		Name: "cli-profile-memory", Description: "CLI profile contract memory.", Type: automemory.TypeProject,
		Body: "CLI profile fact.\n\n**Why:** freezes injection.\n**How to apply:** include the index.",
	}); err != nil {
		t.Fatal(err)
	}
	registry := slash.NewRegistry(workDir)
	registry.Register(&slash.Command{
		Type: slash.CommandPrompt, Name: "cli-profile-skill", Description: "CLI profile contract skill.",
		Source: slash.SourceProject, Frontmatter: slash.Frontmatter{UserInvocable: true},
	})
	runner := &AgentRunner{
		workDir: workDir, model: "cli-profile-model", providerProtocol: "openai",
		effortOverride: "high", enableThinking: true, collaborationMode: collaboration.ModeDefault, maxTurns: 5,
		store: store, autoMemory: autoMemory, manager: manager, llmProvider: modelProvider,
		currentSession: sess, checkpointer: checkpoint.New(checkpoint.Config{SessionDir: sess.RootDir}),
		slashRegistry: registry,
	}
	runner.slashExecutor = slash.NewExecutor(slash.WithWorkDir(workDir), slash.WithForkRunner(&subagentForkRunner{
		getManager: runner.currentSubagentManager,
		getSession: runner.currentSessionIDLocked,
	}))
	return runner, sess
}

func cliProfileToolNames(definitions []schema.ToolDefinition) string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func lastDirectUser(messages []schema.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == schema.RoleUser && messages[index].ToolCallID == "" {
			return messages[index].Content
		}
	}
	return ""
}

func messageContents(messages []schema.Message) string {
	contents := make([]string, 0, len(messages))
	for _, message := range messages {
		contents = append(contents, message.Content)
	}
	return strings.Join(contents, "\n")
}
