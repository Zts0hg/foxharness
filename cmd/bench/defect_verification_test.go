package main

import (
	"os"
	"strings"
	"testing"
)

func TestDVBEN002FailedResultsDoNotControlProcessStatus(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "results = append(results, result)") {
		t.Fatal("benchmark result aggregation changed")
	}
	for _, absent := range []string{"if !result.Success", "os.Exit(", "return exit"} {
		if strings.Contains(text, absent) {
			t.Fatalf("process status now contains %q; update DV-BEN-002 classification", absent)
		}
	}
}

func TestDVBEN003CompositionUsesManuallyMaintainedFidelityClaims(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{"todo tool surface", "context compaction", "no interactive approval surface"} {
		if !strings.Contains(string(source), literal) {
			t.Fatalf("manual fidelity literal %q missing; update DV-BEN-003 classification", literal)
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
