package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/testsupport/entryfixture"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestContextInitialProjectionFreshUncompactedAndResumed(t *testing.T) {
	t.Run("fresh", func(t *testing.T) {
		sess := newContextContractSession(t)
		engine := &AgentEngine{compactor: newContextContractCompactor(t, &summaryProvider{}, 100_000, 2)}
		got, compacted, err := engine.buildInitialContext(context.Background(), sess, "system", nil, schema.Message{Role: schema.RoleUser, Content: "current"})
		if err != nil || compacted {
			t.Fatalf("buildInitialContext() compacted/error = %v, %v", compacted, err)
		}
		assertContextContents(t, got, []string{"system", "current"})
	})

	t.Run("existing uncompacted", func(t *testing.T) {
		sess := newContextContractSession(t)
		records := []session.MessageRecord{
			{Seq: 0, Message: schema.Message{Role: schema.RoleUser, Content: "earlier user"}},
			{Seq: 1, Message: schema.Message{Role: schema.RoleAssistant, Content: "earlier assistant"}},
		}
		engine := &AgentEngine{compactor: newContextContractCompactor(t, &summaryProvider{}, 100_000, 2)}
		got, compacted, err := engine.buildInitialContext(context.Background(), sess, "system", records, schema.Message{Role: schema.RoleUser, Content: "current"})
		if err != nil || compacted {
			t.Fatalf("buildInitialContext() compacted/error = %v, %v", compacted, err)
		}
		assertContextContents(t, got, []string{"system", "earlier user", "earlier assistant", "current"})
	})

	t.Run("resumed compacted fixture", func(t *testing.T) {
		sess := copyEngineContextLifecycleFixture(t)
		records, err := session.NewMessageLog(sess).LoadRecords()
		if err != nil {
			t.Fatalf("LoadRecords() error = %v", err)
		}
		engine := &AgentEngine{compactor: newContextContractCompactor(t, &summaryProvider{}, 100_000, 2)}
		got, compacted, err := engine.buildInitialContext(context.Background(), sess, "system", records, schema.Message{Role: schema.RoleUser, Content: "resume now"})
		if err != nil || compacted {
			t.Fatalf("buildInitialContext() compacted/error = %v, %v", compacted, err)
		}
		if len(got) != 5 {
			t.Fatalf("resumed projection = %#v, want system + summary + two active + current", got)
		}
		if got[0].Content != "system" || !strings.Contains(got[1].Content, "The user asked to inspect the runtime boundary") || !strings.Contains(got[1].Content, sess.TranscriptPath()) {
			t.Fatalf("resumed prefix = %#v", got[:2])
		}
		assertContextContents(t, got[2:], []string{"Continue with the migration plan.", "The next step is characterization coverage.", "resume now"})
	})
}

func TestInitialHistoryCompactionPersistsWithoutDuplicationAfterReopen(t *testing.T) {
	sess := newContextContractSession(t)
	records := make([]session.MessageRecord, 0, 8)
	for seq := int64(0); seq < 8; seq++ {
		records = append(records, session.MessageRecord{Seq: seq, Message: schema.Message{Role: schema.RoleUser, Content: strings.Repeat(string(rune('a'+seq)), 40)}})
	}
	provider := &summaryProvider{}
	firstEngine := &AgentEngine{compactor: newContextContractCompactor(t, provider, 250, 2)}
	current := schema.Message{Role: schema.RoleUser, Content: "current"}
	first, compacted, err := firstEngine.buildInitialContext(context.Background(), sess, "system", records, current)
	if err != nil || !compacted {
		t.Fatalf("first build compacted/error = %v, %v", compacted, err)
	}
	state, err := session.LoadCompactState(sess)
	if err != nil {
		t.Fatalf("LoadCompactState() error = %v", err)
	}
	if state.Summary != "short persisted summary" || state.CoveredUntilSeq != 5 {
		t.Fatalf("compact state = %#v", state)
	}

	secondEngine := &AgentEngine{compactor: newContextContractCompactor(t, provider, 250, 2)}
	second, compactedAgain, err := secondEngine.buildInitialContext(context.Background(), sess, "system", records, current)
	if err != nil || compactedAgain {
		t.Fatalf("reopened build compacted/error = %v, %v", compactedAgain, err)
	}
	if provider.calls != 1 {
		t.Fatalf("summary provider calls = %d, want exactly one", provider.calls)
	}
	if len(first) != len(second) {
		t.Fatalf("projection lengths = %d and %d", len(first), len(second))
	}
	for index := range first {
		if first[index].Role != second[index].Role || first[index].Content != second[index].Content {
			t.Fatalf("reopened projection[%d] = %#v, want %#v", index, second[index], first[index])
		}
	}
	var summaryCount int
	for _, message := range second {
		if strings.Contains(message.Content, "short persisted summary") {
			summaryCount++
		}
	}
	if summaryCount != 1 {
		t.Fatalf("summary count after reopen = %d, want 1", summaryCount)
	}
}

func TestPreTurnCompactionUsesTheResolvedRequestToolSnapshot(t *testing.T) {
	sess := newContextContractSession(t)
	log := session.NewMessageLog(sess)
	for index := 0; index < 6; index++ {
		if _, err := log.Append("seed", schema.Message{Role: schema.RoleUser, Content: strings.Repeat(string(rune('a'+index)), 100)}); err != nil {
			t.Fatalf("Append(seed %d) error = %v", index, err)
		}
	}
	definition := schema.ToolDefinition{
		Name: "snapshot_tool", Description: strings.Repeat("D", 800), InputSchema: map[string]any{"type": "object"},
	}
	baseRegistry := tools.NewRegistry()
	baseRegistry.Register(&contextDefinitionTool{definition: definition})
	registry := &countingContextRegistry{Registry: baseRegistry}
	modelProvider := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "<summary>pre-turn summary</summary>"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	reporter := &currentContractReporter{}
	engine := NewAgentEngine(modelProvider, registry, sess.WorkDir, staticComposer{}, Config{MaxTurns: 1})
	engine.WithCompactor(newContextContractCompactor(t, modelProvider, 1_000, 1))

	if _, err := engine.RunWithReporter(context.Background(), sess, "current", reporter); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if registry.definitionCalls != 2 {
		t.Fatalf("GetAvailableTools() calls = %d, want request snapshot plus post-response TODO gate lookup", registry.definitionCalls)
	}
	if len(modelProvider.seen) != 2 || len(modelProvider.seenTools[0]) != 0 || strings.Join(modelProvider.seenTools[1], ",") != definition.Name {
		t.Fatalf("provider tool surfaces = %#v, want summary none then action snapshot", modelProvider.seenTools)
	}
	if len(modelProvider.seenDefinitions) != 2 || len(modelProvider.seenDefinitions[0]) != 0 || !reflect.DeepEqual(modelProvider.seenDefinitions[1], []schema.ToolDefinition{definition}) {
		t.Fatalf("provider tool definitions = %#v, want exact action snapshot %#v", modelProvider.seenDefinitions, definition)
	}
	actionMessages := modelProvider.seen[1]
	if len(actionMessages) < 5 || !strings.HasPrefix(actionMessages[2].Content, compaction.BoundaryMarkerPrefix) || !strings.Contains(actionMessages[3].Content, "pre-turn summary") {
		t.Fatalf("compacted action request = %#v", actionMessages)
	}
	if len(reporter.facts) != 4 || reporter.facts[1].Kind != "compaction" || reporter.facts[1].Name != "turn_context" {
		t.Fatalf("observer facts = %#v, want ordered turn_context compaction", reporter.facts)
	}
}

func TestReactiveCompactionRetriesOnceWithConsistentProjection(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		retryError    bool
		wantRunError  bool
		wantCallCount int
	}{
		{name: "retry succeeds", wantCallCount: 3},
		{name: "second prompt-too-long is terminal", retryError: true, wantRunError: true, wantCallCount: 3},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			sess := newContextContractSession(t)
			log := session.NewMessageLog(sess)
			for index := 0; index < 8; index++ {
				if _, err := log.Append("seed", schema.Message{Role: schema.RoleUser, Content: "history " + string(rune('a'+index))}); err != nil {
					t.Fatalf("Append(seed) error = %v", err)
				}
			}
			modelProvider := &reactiveContextProvider{retryError: testCase.retryError}
			reporter := &currentContractReporter{}
			engine := NewAgentEngine(modelProvider, tools.NewRegistry(), sess.WorkDir, staticComposer{}, Config{MaxTurns: 1})
			engine.WithCompactor(newContextContractCompactor(t, modelProvider, 100_000, 8))

			result, err := engine.RunWithReporter(context.Background(), sess, "current", reporter)
			if testCase.wantRunError {
				if err == nil || !strings.Contains(err.Error(), "context_length_exceeded") || result != nil {
					t.Fatalf("terminal retry result/error = %#v, %v", result, err)
				}
			} else if err != nil || result == nil || result.FinalMessage != "done" {
				t.Fatalf("successful retry result/error = %#v, %v", result, err)
			}
			if modelProvider.calls != testCase.wantCallCount {
				t.Fatalf("provider calls = %d, want %d", modelProvider.calls, testCase.wantCallCount)
			}
			if len(modelProvider.retryMessages) < 5 || !strings.HasPrefix(modelProvider.retryMessages[2].Content, compaction.BoundaryMarkerPrefix) || !strings.Contains(modelProvider.retryMessages[3].Content, "reactive summary") {
				t.Fatalf("reactive retry projection = %#v", modelProvider.retryMessages)
			}
			var compactions int
			for _, fact := range reporter.facts {
				if fact.Kind == "compaction" && fact.Name == "reactive" {
					compactions++
				}
			}
			if compactions != 1 {
				t.Fatalf("reactive compaction facts = %d, want one: %#v", compactions, reporter.facts)
			}
		})
	}
}

func TestContextBlockingDecisionIncludesTheSameToolSnapshot(t *testing.T) {
	sess := newContextContractSession(t)
	definition := schema.ToolDefinition{
		Name: "large_definition", Description: strings.Repeat("T", 2_000), InputSchema: map[string]any{"type": "object"},
	}
	baseRegistry := tools.NewRegistry()
	baseRegistry.Register(&contextDefinitionTool{definition: definition})
	registry := &countingContextRegistry{Registry: baseRegistry}
	modelProvider := &countingFinalContextProvider{}
	engine := NewAgentEngine(modelProvider, registry, sess.WorkDir, staticComposer{}, Config{MaxTurns: 1})
	config := compaction.DefaultCompactionConfig()
	config.Model = "context-contract"
	config.ContextWindow = 25_000
	config.AutoCompactThreshold = 100_000
	config.RecentKeep = 1
	config.Estimator = compaction.RoughEstimator{}
	compactor, err := compaction.NewCompactor(modelProvider, config)
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	engine.WithCompactor(compactor)

	_, runErr := engine.Run(context.Background(), sess, strings.Repeat("P", 500))
	if runErr == nil || !strings.Contains(runErr.Error(), "阻塞阈值") {
		t.Fatalf("Run() error = %v, want tool-overhead blocking decision", runErr)
	}
	if registry.definitionCalls != 1 {
		t.Fatalf("GetAvailableTools() calls = %d, want one snapshot", registry.definitionCalls)
	}
	if modelProvider.calls != 0 {
		t.Fatalf("provider calls = %d, want none after blocking", modelProvider.calls)
	}
}

func newContextContractSession(t *testing.T) *session.Session {
	t.Helper()
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return sess
}

func newContextContractCompactor(t *testing.T, modelProvider provider.LLMProvider, threshold, recentKeep int) *compaction.Compactor {
	t.Helper()
	config := compaction.DefaultCompactionConfig()
	config.Model = "context-contract"
	config.ContextWindow = 200_000
	config.AutoCompactThreshold = threshold
	config.RecentKeep = recentKeep
	config.Estimator = compaction.RoughEstimator{}
	compactor, err := compaction.NewCompactor(modelProvider, config)
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	return compactor
}

func copyEngineContextLifecycleFixture(t *testing.T) *session.Session {
	t.Helper()
	fixtureRoot := filepath.Join("..", "..", "testdata", "characterization", "v1")
	manifest, err := entryfixture.Load(fixtureRoot)
	if err != nil {
		t.Fatalf("Load fixture manifest error = %v", err)
	}
	destination := t.TempDir()
	for _, fixture := range manifest.Fixtures {
		if !strings.HasPrefix(fixture.Path, "sessions/context-lifecycle/") {
			continue
		}
		if _, err := entryfixture.CopyFixture(fixtureRoot, fixture.Path, destination); err != nil {
			t.Fatalf("CopyFixture(%q) error = %v", fixture.Path, err)
		}
	}
	sessionRoot := filepath.Join(destination, "sessions", "context-lifecycle")
	data, err := os.ReadFile(filepath.Join(sessionRoot, "session.json"))
	if err != nil {
		t.Fatalf("ReadFile(session.json) error = %v", err)
	}
	var sess session.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		t.Fatalf("Unmarshal(session.json) error = %v", err)
	}
	sess.RootDir = sessionRoot
	return &sess
}

func assertContextContents(t *testing.T, got []schema.Message, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("context len = %d, want %d: %#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index].Content != want[index] {
			t.Fatalf("context[%d] content = %q, want %q", index, got[index].Content, want[index])
		}
	}
}

type contextDefinitionTool struct {
	definition schema.ToolDefinition
}

func (t *contextDefinitionTool) Name() string { return t.definition.Name }

func (t *contextDefinitionTool) Definition() schema.ToolDefinition { return t.definition }

func (*contextDefinitionTool) Execute(context.Context, json.RawMessage) (string, error) {
	return "", nil
}

type countingContextRegistry struct {
	tools.Registry
	definitionCalls int
}

func (r *countingContextRegistry) GetAvailableTools() []schema.ToolDefinition {
	r.definitionCalls++
	return r.Registry.GetAvailableTools()
}

type reactiveContextProvider struct {
	calls         int
	retryError    bool
	retryMessages []schema.Message
}

type countingFinalContextProvider struct {
	calls int
}

func (p *countingFinalContextProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (p *reactiveContextProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	switch p.calls {
	case 1:
		return nil, errors.New("context_length_exceeded: fixture prompt")
	case 2:
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "<summary>reactive summary</summary>"}}, nil
	case 3:
		p.retryMessages = append([]schema.Message(nil), messages...)
		if p.retryError {
			return nil, errors.New("context_length_exceeded: retry prompt")
		}
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
	default:
		return nil, errors.New("unexpected extra provider call")
	}
}
