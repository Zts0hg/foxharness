package runtimecontract

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestVerifyScenarioAcceptsExactObservation(t *testing.T) {
	scenario := testScenario()
	adapter := AdapterFunc(func(_ context.Context, input RunInput, script Script) (Observed, error) {
		if input.Profile != "CLIExec" {
			t.Fatalf("Profile = %q, want CLIExec", input.Profile)
		}
		if len(script.ModelSteps) != 1 {
			t.Fatalf("ModelSteps = %d, want 1", len(script.ModelSteps))
		}
		return scenario.Expected.Observed, nil
	})

	if err := VerifyScenario(context.Background(), adapter, scenario); err != nil {
		t.Fatalf("VerifyScenario() error = %v", err)
	}
}

func TestVerifyScenarioRejectsFactReordering(t *testing.T) {
	scenario := testScenario()
	actual := scenario.Expected.Observed
	actual.Facts = []Fact{actual.Facts[1], actual.Facts[0]}
	adapter := AdapterFunc(func(context.Context, RunInput, Script) (Observed, error) {
		return actual, nil
	})

	err := VerifyScenario(context.Background(), adapter, scenario)
	if err == nil || !strings.Contains(err.Error(), "facts") {
		t.Fatalf("VerifyScenario() error = %v, want ordered facts mismatch", err)
	}
}

func TestVerifyScenarioRejectsArtifactMismatch(t *testing.T) {
	scenario := testScenario()
	actual := scenario.Expected.Observed
	actual.Artifacts = append([]Artifact(nil), actual.Artifacts...)
	actual.Artifacts[0].Content = "changed"
	adapter := AdapterFunc(func(context.Context, RunInput, Script) (Observed, error) {
		return actual, nil
	})

	err := VerifyScenario(context.Background(), adapter, scenario)
	if err == nil || !strings.Contains(err.Error(), "artifacts") {
		t.Fatalf("VerifyScenario() error = %v, want artifact mismatch", err)
	}
}

func TestVerifyScenarioMatchesExpectedAdapterError(t *testing.T) {
	scenario := testScenario()
	scenario.Expected.AdapterErrorContains = "provider unavailable"
	adapter := AdapterFunc(func(context.Context, RunInput, Script) (Observed, error) {
		return Observed{}, errors.New("invoke model: provider unavailable")
	})

	if err := VerifyScenario(context.Background(), adapter, scenario); err != nil {
		t.Fatalf("VerifyScenario() error = %v", err)
	}
}

func TestScenarioValidationRejectsMalformedID(t *testing.T) {
	scenario := testScenario()
	scenario.ID = "runtime-one"
	adapter := AdapterFunc(func(context.Context, RunInput, Script) (Observed, error) {
		t.Fatal("adapter must not run for invalid scenario")
		return Observed{}, nil
	})

	err := VerifyScenario(context.Background(), adapter, scenario)
	if err == nil || !strings.Contains(err.Error(), "scenario ID") {
		t.Fatalf("VerifyScenario() error = %v, want scenario ID error", err)
	}
}

func testScenario() Scenario {
	return Scenario{
		ID: "RT-001",
		Input: RunInput{
			Profile:       "CLIExec",
			Prompt:        "hello",
			Model:         "scripted-model",
			Provider:      "scripted",
			SessionChoice: "new",
		},
		Script: Script{
			ModelSteps: []ModelStep{{
				Response: ModelResponse{Content: "done", FinishReason: "stop"},
			}},
		},
		Expected: Expected{
			Observed: Observed{
				Facts: []Fact{
					{Kind: "run_started", Sequence: 1},
					{Kind: "run_completed", Sequence: 2, Content: "done"},
				},
				Outcome: Outcome{
					FinalMessage: "done",
					FinishReason: "stop",
					TurnCount:    1,
				},
				Artifacts: []Artifact{{Kind: "transcript", Path: "transcript.jsonl", Content: "done"}},
			},
		},
	}
}
