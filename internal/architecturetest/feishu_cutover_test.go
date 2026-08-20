package architecturetest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFeishuAdapterDependsOnApplicationNotRuntimeSubsystems(t *testing.T) {
	root := moduleRoot(t)
	var imports []string
	for _, edge := range productionImportEdges(t, root) {
		if edge.From == "internal/feishu" {
			imports = append(imports, edge.To)
		}
	}
	want := []string{"internal/app", "internal/approval"}
	if fmt.Sprint(imports) != fmt.Sprint(want) {
		t.Fatalf("internal/feishu production imports = %v, want focused adapter imports %v", imports, want)
	}
}

func TestFeishuEntryUsesRuntimeBackedTaskFactory(t *testing.T) {
	root := moduleRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "cmd", "feishu", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	assertSourceContains(t, string(source), "newFeishuTaskExecutionFactory(", "feishu.NewRunner(taskFactory, messenger)")
	assertSourceExcludes(t, string(source), "engine.NewLegacyEngine(", "childruntime.New(", "feishu.NewRunner(llmProvider")
}

func assertSourceContains(t *testing.T, source string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !contains(source, fragment) {
			t.Errorf("source missing %q", fragment)
		}
	}
}

func assertSourceExcludes(t *testing.T, source string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if contains(source, fragment) {
			t.Errorf("source retains obsolete production fragment %q", fragment)
		}
	}
}

func contains(source, fragment string) bool {
	for index := 0; index+len(fragment) <= len(source); index++ {
		if source[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
