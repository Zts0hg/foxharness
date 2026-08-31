package benchmark

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestM14RuntimeFidelityDerivesFromResolvedBenchmarkProfile(t *testing.T) {
	maxTurns := 17
	runSpec := foxruntime.RunSpec{
		Prompt: "evaluate", ProviderProtocol: "openai", Model: "model-a", MaxTurns: &maxTurns,
		AllowedTools: []string{"read_file", "read_todo"},
	}
	spec, err := ResolveRuntimeSpec(runSpec)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ProviderProtocol != "openai" || spec.Model != "model-a" || spec.MaxTurns != 17 {
		t.Fatalf("resolved benchmark spec = %#v", spec)
	}
	if !reflect.DeepEqual(spec.ToolSurface, []string{"read_file", "read_todo"}) {
		t.Fatalf("resolved tool surface = %#v", spec.ToolSurface)
	}
	if spec.MemoryPolicy != "session_only" || spec.PermissionPolicy != "none" || spec.ObservationPolicy != "structured_result" || spec.InteractionPolicy != "headless" {
		t.Fatalf("profile-derived policies = %#v", spec)
	}
}

func TestM14BenchmarkProductionUsesOnlyRuntimeHarnessPath(t *testing.T) {
	for _, file := range []string{"runner.go", "runtime_spec.go"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		for _, forbidden := range []string{"internal/engine", "internal/session", "LegacyEngine", "RunWithReporter", "runIdentityReporter"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s retains obsolete benchmark runtime token %q", file, forbidden)
			}
		}
	}
}

func TestM14RuntimeCleanupFailureRetainsStartedRuntimeEvidence(t *testing.T) {
	fixture := t.TempDir()
	runner := NewRunner(func(ctx context.Context, workDir string, _ *Case) (*Harness, error) {
		store := &failingFinishStore{FileStore: session.NewFileStoreWithHome(workDir, t.TempDir())}
		spec := NewRuntimeSpec("scripted", "fixture-model", 1, nil)
		return newTargetBenchmarkHarnessWithStore(
			ctx, workDir, store, spec, benchmarkFinalProvider{},
			func(foxruntime.RunAssembly) benchmarkPromptComposer { return benchmarkComposer{} }, nil,
		)
	})
	result, err := runner.RunCase(context.Background(), &Case{ID: "cleanup", Fixture: fixture, Prompt: "run", MaxTurns: 1})
	if err != nil {
		t.Fatalf("cleanup error = %v, want the close failure recorded without failing the repeat", err)
	}
	defer os.RemoveAll(result.Workspace)
	if result.CleanupError == "" || result.InfrastructureError == "" {
		t.Fatalf("cleanup evidence = %q/%q, want both recorded", result.CleanupError, result.InfrastructureError)
	}
	/* A failed terminal run write never fails the run; the recovery failure
	 * surfaces through the cleanup evidence while the runtime run stays
	 * completed with its started run identifier and verdict. */
	if result.RuntimeStatus != RuntimeStatusCompleted || result.RunID == "" || result.RuntimeCause != "" {
		t.Fatalf("runtime evidence lost to cleanup failure: %#v", result)
	}
	if !result.Success || result.Status != ResultStatusCompleted {
		t.Fatalf("completed repeat flipped by close failure: %#v", result)
	}
}

type failingFinishStore struct {
	*session.FileStore
}

func (s *failingFinishStore) FinishRun(*session.StoredRun) error {
	return errors.New("durable finish unavailable")
}
