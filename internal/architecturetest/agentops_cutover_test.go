package architecturetest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentOpsAdapterDependsOnApplicationAndOwnedPolicyOnly(t *testing.T) {
	root := moduleRoot(t)
	var imports []string
	for _, edge := range productionImportEdges(t, root) {
		if edge.From == "internal/agentops" {
			imports = append(imports, edge.To)
		}
	}
	want := []string{"internal/agentops/logsearch", "internal/app"}
	if fmt.Sprint(imports) != fmt.Sprint(want) {
		t.Fatalf("internal/agentops production imports = %v, want focused adapter imports %v", imports, want)
	}
}

func TestAgentOpsEntryUsesRuntimeBackedTaskFactory(t *testing.T) {
	root := moduleRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "cmd", "agentops", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	assertSourceContains(t, string(source), "newAgentOpsTaskExecutionFactory(", "agentops.NewRunner(taskFactory, messenger)")
	assertSourceExcludes(t, string(source), "engine.NewLegacyEngine(", "childruntime.New(", "agentops.NewRunner(llmProvider")
}
