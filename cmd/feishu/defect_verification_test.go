package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDVFEI006ProductionEntryHasNoCoordinatedShutdown(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(current), "main.go"))
	if err != nil {
		t.Fatalf("ReadFile(main.go) error = %v", err)
	}
	source := string(data)
	for _, currentBehavior := range []string{
		"ctx := context.Background()",
		"go runner.Start(ctx, tasks)",
		"gateway.Listen(\":7777\")",
	} {
		if !strings.Contains(source, currentBehavior) {
			t.Fatalf("production entry no longer contains %q; update DV-FEI-006 classification", currentBehavior)
		}
	}
	for _, absentCoordination := range []string{
		"signal.NotifyContext",
		"gateway.Shutdown(",
		"close(tasks)",
	} {
		if strings.Contains(source, absentCoordination) {
			t.Fatalf("production entry now contains %q; update DV-FEI-006 classification", absentCoordination)
		}
	}
}
