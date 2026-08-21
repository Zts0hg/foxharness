package benchmark

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/provider"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestPFBEN001CurrentProfileSnapshotIsImmutableAndFlat(t *testing.T) {
	toolSurface := []string{"read_file", "write_file", "bash", "edit_file", "read_todo", "update_todo"}
	spec := NewRuntimeSpec("openai", "fixture-model", 12, toolSurface)
	toolSurface[0] = "expanded"
	fidelity := spec.Fidelity()
	fidelity.Spec.ToolSurface[0] = "mutated"

	want := BenchmarkRuntimeSpec{
		ProviderProtocol: "openai", Model: "fixture-model", MaxTurns: 12,
		ToolSurface:  []string{"bash", "edit_file", "read_file", "read_todo", "update_todo", "write_file"},
		PromptPolicy: "base_project_session_memory", MemoryPolicy: "session_only",
		CompactionPolicy: "automatic_selected_model", PermissionPolicy: "none",
		ObservationPolicy: "structured_result", InteractionPolicy: "headless",
	}
	if !reflect.DeepEqual(spec, want) {
		t.Fatalf("benchmark profile = %#v, want %#v", spec, want)
	}
	if spec.ToolSurface[0] != "bash" {
		t.Fatalf("fidelity consumer mutated runtime spec: %#v", spec.ToolSurface)
	}
	filtered := tools.NewFilteredRegistry(benchmarkProfileRegistry(t.TempDir(), &session.StoredSession{RootDir: t.TempDir()}), []string{"read_file", "delegate_task"})
	if got := benchmarkProfileToolNames(filtered.GetAvailableTools()); got != "read_file" {
		t.Fatalf("restriction expanded benchmark ceiling: %q", got)
	}
}

func TestPFBEN002CaseSnapshotAndExactPromptIgnoreConcurrentCallerMutation(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "input.txt"), []byte("stable"), 0o600); err != nil {
		t.Fatal(err)
	}
	model := &benchmarkProfileProvider{final: "done"}
	factoryEntered := make(chan struct{})
	releaseFactory := make(chan struct{})
	runner := NewRunner(func(_ context.Context, workDir string, c *Case) (*Harness, error) {
		close(factoryEntered)
		<-releaseFactory
		return benchmarkProfileHarness(t, workDir, c, model)
	})
	c := &Case{
		ID: "case", Name: "Case", Fixture: fixture, Prompt: "  exact fixture prompt  ", MaxTurns: 3, TimeoutSeconds: 60,
		Validations: []Validation{{Type: "file_contains", Path: "input.txt", Contains: "stable"}},
	}
	resultCh := make(chan *Result, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := runner.RunRepeat(context.Background(), c, 7)
		resultCh <- result
		errCh <- err
	}()
	<-factoryEntered
	c.ID = "mutated"
	c.Prompt = "mutated prompt"
	c.Validations[0].Contains = "missing"
	close(releaseFactory)
	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(result.Workspace)
	if result.CaseID != "case" || result.RepeatIndex != 7 || !result.Success || len(result.Validations) != 1 || !result.Validations[0].Passed {
		t.Fatalf("frozen evaluation result = %#v", result)
	}
	request := model.firstRequest()
	if len(request) == 0 || request[len(request)-1].Role != schema.RoleUser || request[len(request)-1].Content != "  exact fixture prompt  " {
		t.Fatalf("model request = %#v", request)
	}
}

func TestPFBEN003And004RepeatsUseFreshSerialRuntimeState(t *testing.T) {
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "input.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	var factoryCalls int
	var workspaces []string
	runner := NewRunner(func(_ context.Context, workDir string, c *Case) (*Harness, error) {
		factoryCalls++
		workspaces = append(workspaces, workDir)
		if factoryCalls == 1 {
			if err := os.WriteFile(filepath.Join(workDir, "first-only.txt"), []byte("first"), 0o600); err != nil {
				return nil, err
			}
		} else if _, err := os.Stat(filepath.Join(workDir, "first-only.txt")); !os.IsNotExist(err) {
			t.Fatalf("repeat inherited prior workspace mutation: %v", err)
		}
		return benchmarkProfileHarnessWithHome(workDir, home, c, &benchmarkProfileProvider{final: "done"})
	})
	c := &Case{ID: "repeat", Fixture: fixture, Prompt: "run", MaxTurns: 1, TimeoutSeconds: 60, Validations: []Validation{{Type: "file_contains", Path: "input.txt", Contains: "source"}}}
	first, err := runner.RunRepeat(context.Background(), c, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.RunRepeat(context.Background(), c, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(first.Workspace)
	defer os.RemoveAll(second.Workspace)
	if !first.Success || !second.Success || first.RepeatIndex != 1 || second.RepeatIndex != 2 {
		t.Fatalf("ordered repeat results = %#v/%#v", first, second)
	}
	if len(workspaces) != 2 || workspaces[0] == workspaces[1] || first.SessionID == second.SessionID || first.RunID == second.RunID || first.FixtureID != second.FixtureID {
		t.Fatalf("repeat isolation = workspaces %#v, results %#v/%#v", workspaces, first, second)
	}
	if _, err := os.Stat(filepath.Join(fixture, "first-only.txt")); !os.IsNotExist(err) {
		t.Fatalf("source fixture mutated: %v", err)
	}
}

func TestPFBEN008And009ContextIsFixtureAndSessionLocal(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("BENCHMARK_PROJECT_INSTRUCTION\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(workDir, ".foxharness", "skills", "fixture-skill")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: Fixture Skill\ndescription: fixture only\n---\n\nBENCHMARK_SKILL_FRAGMENT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sessionStore := session.NewFileStoreWithHome(workDir, t.TempDir())
	sess, err := sessionStore.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatal(err)
	}
	memoryPath := store.WorkingMemoryPath()
	if err := os.WriteFile(memoryPath, []byte("BENCHMARK_SESSION_MEMORY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fragments, err := foxruntime.NewPromptCollector(workDir).WithMemory(memoryPath).Collect(context.Background(), foxruntime.ContextCollectionRequest{
		Profile: foxruntime.BenchmarkEval, Prompt: "use $fixture-skill", WorkDir: workDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	composed := prompt.Render(fragments)
	for _, fragment := range []string{"BENCHMARK_PROJECT_INSTRUCTION", "BENCHMARK_SESSION_MEMORY", "BENCHMARK_SKILL_FRAGMENT", "Session Plan and Todo Files"} {
		if strings.Count(composed, fragment) != 1 {
			t.Fatalf("prompt contains %q %d times:\n%s", fragment, strings.Count(composed, fragment), composed)
		}
	}
	for _, forbidden := range []string{"## Persistent Memory", "## Available Skills", "Collaboration Mode", "## Asking the User", "## Formal Plan"} {
		if strings.Contains(composed, forbidden) {
			t.Fatalf("benchmark prompt contains forbidden fragment %q:\n%s", forbidden, composed)
		}
	}
	if sess.Source != session.SOURCECLI || sess.WorkDir != workDir {
		t.Fatalf("benchmark session = %#v", sess)
	}
}

func TestPFBEN012And016HeadlessControlPlaneOwnership(t *testing.T) {
	for _, name := range []string{"runner.go", "runtime_spec.go", "case.go", "validate.go", "report.go"} {
		source := readBenchmarkSource(t, name)
		for _, forbidden := range []string{"internal/app", "internal/tui", "cmd/fox", "engine.NewLegacyEngine"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("benchmark control file %s contains adapter/construction token %q", name, forbidden)
			}
		}
	}
	for _, name := range []string{"runner.go", "runtime_spec.go"} {
		source := readBenchmarkSource(t, name)
		for _, forbidden := range []string{"fmt.Print", "os.Stdout", "PASS", "FAIL"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("benchmark runtime path %s contains evaluation presentation token %q", name, forbidden)
			}
		}
	}
	for _, dir := range []string{filepath.Join("..", "engine"), filepath.Join("..", "app")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "internal/benchmark") {
				t.Fatalf("runtime file %s imports benchmark", filepath.Join(dir, entry.Name()))
			}
		}
	}
}

type benchmarkProfileProvider struct {
	mu       sync.Mutex
	final    string
	requests [][]schema.Message
}

func (p *benchmarkProfileProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, append([]schema.Message(nil), messages...))
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: p.final}}, nil
}

func (p *benchmarkProfileProvider) firstRequest() []schema.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 {
		return nil
	}
	return append([]schema.Message(nil), p.requests[0]...)
}

func benchmarkProfileHarness(t *testing.T, workDir string, c *Case, model provider.LLMProvider) (*Harness, error) {
	t.Helper()
	return benchmarkProfileHarnessWithHome(workDir, t.TempDir(), c, model)
}

func benchmarkProfileHarnessWithHome(workDir, home string, c *Case, model provider.LLMProvider) (*Harness, error) {
	spec := NewRuntimeSpec("scripted", "fixture-model", c.MaxTurns, []string{"read_file", "write_file", "bash", "edit_file", "read_todo", "update_todo"})
	return newTargetBenchmarkHarness(context.Background(), workDir, home, spec, model,
		func(assembly foxruntime.RunAssembly) benchmarkPromptComposer {
			store := memory.NewSessionStore(workDir, assembly.Session.RootDir)
			if err := store.EnsureFiles(); err != nil {
				return benchmarkCauseComposer{cause: err}
			}
			return benchmarkRuntimePromptComposer{
				workDir: workDir,
				collector: foxruntime.NewPromptCollector(workDir).
					WithMemory(store.WorkingMemoryPath()),
			}
		},
		func(assembly foxruntime.RunAssembly) tools.Registry {
			return benchmarkProfileRegistry(workDir, &session.StoredSession{RootDir: assembly.Session.RootDir})
		},
	)
}

type benchmarkRuntimePromptComposer struct {
	workDir   string
	collector *foxruntime.PromptCollector
}

func (c benchmarkRuntimePromptComposer) Compose(userPrompt string) (string, error) {
	fragments, err := c.collector.Collect(context.Background(), foxruntime.ContextCollectionRequest{
		Profile: foxruntime.BenchmarkEval, Prompt: userPrompt, WorkDir: c.workDir,
	})
	if err != nil {
		return "", err
	}
	return prompt.Render(fragments), nil
}

func benchmarkProfileRegistry(workDir string, sess *session.StoredSession) tools.Registry {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))
	registry.Register(tools.NewReadTodoTool(sess.RootDir))
	registry.Register(tools.NewUpdateTodoTool(sess.RootDir))
	return registry
}

func benchmarkProfileToolNames(definitions []schema.ToolDefinition) string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	return strings.Join(names, ",")
}
