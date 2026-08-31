package main

import (
	"context"
	"testing"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* TestTUIRuntimeBuildsOneCompactorPerRun pins the baseline compactor lifetime:
 * every run starts with a fresh compactor (zero tool overhead, cleared circuit
 * breaker) while all collaborators inside one run share the same instance. */
func TestTUIRuntimeBuildsOneCompactorPerRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	store := session.NewFileStore(workDir)
	composition := &tuiRuntimeComposition{
		workDir: workDir, store: store, resources: make(map[session.ID]*tuiSessionResources),
	}
	stored, err := store.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := composition.initializeSession(context.Background(), foxruntime.AgentSessionSnapshot{ID: stored.ID}); err != nil {
		t.Fatal(err)
	}
	resource, err := composition.resource(stored.ID)
	if err != nil {
		t.Fatal(err)
	}
	modelProvider := &targetTUIProvider{}
	firstRun := foxruntime.RunAssembly{Run: foxruntime.RunScopeSnapshot{RunID: session.RunID("run-first")}}
	secondRun := foxruntime.RunAssembly{Run: foxruntime.RunScopeSnapshot{RunID: session.RunID("run-second")}}

	first, err := composition.compactor(resource, firstRun, modelProvider)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := composition.compactor(resource, firstRun, modelProvider)
	if err != nil {
		t.Fatal(err)
	}
	if first != shared {
		t.Fatal("collaborators of one run received different compactors")
	}
	second, err := composition.compactor(resource, secondRun, modelProvider)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("a later run reused the previous run's compactor")
	}
	first.SetToolOverhead(50_000)
	if second.Threshold() <= first.Threshold() {
		t.Fatalf("fresh compactor threshold = %d, want the un-degraded value above %d", second.Threshold(), first.Threshold())
	}
}
