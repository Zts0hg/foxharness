package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/benchmark"
)

func TestDVBEN002ResultStatusControlsProcessExit(t *testing.T) {
	tests := []struct {
		name       string
		results    []*benchmark.Result
		infraError bool
		want       int
	}{
		{name: "all success", results: []*benchmark.Result{{Success: true}}, want: 0},
		{name: "evaluation failure", results: []*benchmark.Result{{Status: benchmark.ResultStatusFailed}}, want: 1},
		{name: "cancelled", results: []*benchmark.Result{{Status: benchmark.ResultStatusCancelled}}, want: 1},
		{name: "timed out", results: []*benchmark.Result{{Status: benchmark.ResultStatusTimedOut}}, want: 1},
		{name: "infrastructure result", results: []*benchmark.Result{{Status: benchmark.ResultStatusInfrastructureFailed}}, want: 2},
		{name: "report failure", results: []*benchmark.Result{{Success: true}}, infraError: true, want: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resultExitCode(test.results, test.infraError); got != test.want {
				t.Fatalf("resultExitCode() = %d, want %d", got, test.want)
			}
		})
	}
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "os.Exit(run(") {
		t.Fatal("production main does not return the benchmark exit decision")
	}
}

func TestDVBEN003CompositionUsesResolvedRuntimeSpec(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"benchmark.NewRuntimeSpec", "RuntimeFidelity: runtimeSpec.Fidelity()"} {
		if !strings.Contains(string(source), required) {
			t.Fatalf("resolved runtime composition %q missing", required)
		}
	}
	for _, forbidden := range []string{"todo tool surface", "context compaction", "no interactive approval surface"} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("composition retains manual fidelity claim %q", forbidden)
		}
	}
}

func TestDVBEN005NonPositiveRepeatCanProduceSuccessfulZeroRunReport(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "for i := 0; i < *repeat; i++") {
		t.Fatal("repeat orchestration changed")
	}
	if strings.Contains(text, "*repeat <= 0") {
		t.Fatal("non-positive repeat is now rejected; update DV-BEN-005 classification")
	}
}
