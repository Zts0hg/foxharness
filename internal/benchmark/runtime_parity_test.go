package benchmark

import (
	"context"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestM14CurrentAndTargetBenchmarkAdaptersHaveParity(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	caseInput := &Case{ID: "parity", Prompt: "  preserve exact prompt  ", MaxTurns: 1}

	currentModel := &benchmarkProfileProvider{final: "done"}
	currentStore := session.NewFileStoreWithHome(workDir, t.TempDir())
	currentSession, err := currentStore.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	currentEngine := engine.NewLegacyEngine(
		currentModel, tools.NewRegistry(), workDir,
		benchmarkComposer{},
		engine.Config{MaxTurns: 1, ProviderProtocol: "scripted", Model: "fixture-model"},
	)
	currentResult, err := currentEngine.Run(ctx, currentSession, caseInput.Prompt)
	if err != nil {
		t.Fatal(err)
	}

	targetModel := &benchmarkProfileProvider{final: "done"}
	targetSpecDefinition := NewRuntimeSpec("scripted", "fixture-model", 1, nil)
	targetHarness, err := newTargetBenchmarkHarness(
		ctx, workDir, t.TempDir(), targetSpecDefinition, targetModel,
		func(foxruntime.RunAssembly) engine.PromptComposer { return benchmarkComposer{} }, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	targetSpec := targetHarness.RunSpec
	targetSpec.Prompt = caseInput.Prompt
	targetResult, err := targetHarness.Session.Run(ctx, targetSpec)
	if err != nil {
		t.Fatal(err)
	}
	targetSession := targetHarness.Session.Snapshot()
	if err := targetHarness.Session.Close(ctx); err != nil {
		t.Fatal(err)
	}

	if currentResult.FinalMessage != targetResult.Outcome.FinalMessage {
		t.Fatalf("final messages = current:%q target:%q", currentResult.FinalMessage, targetResult.Outcome.FinalMessage)
	}
	if !reflect.DeepEqual(currentModel.firstRequest(), targetModel.firstRequest()) {
		t.Fatalf("model requests differ:\ncurrent=%#v\ntarget=%#v", currentModel.firstRequest(), targetModel.firstRequest())
	}
	currentRecords, err := currentStore.LoadMessageRecords(currentSession)
	if err != nil {
		t.Fatal(err)
	}
	targetRecords, err := session.NewMessageLog(&session.StoredSession{
		ID: targetSession.ID, Source: targetSession.Source, WorkDir: targetSession.WorkDir, RootDir: targetSession.RootDir,
	}).LoadRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(currentRecords) != 2 || len(targetRecords) != 2 || targetResult.CommittedMessage != "done" {
		t.Fatalf("persisted parity = current:%#v target:%#v", currentRecords, targetResult)
	}
	for index := range currentRecords {
		if !reflect.DeepEqual(currentRecords[index].Message, targetRecords[index].Message) || currentRecords[index].DisplayContent != targetRecords[index].DisplayContent {
			t.Fatalf("persisted message %d differs: current=%#v target=%#v", index, currentRecords[index], targetRecords[index])
		}
	}
}
