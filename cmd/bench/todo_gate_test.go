package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
)

/* TestBenchmarkTurnPolicyBlocksFinalAnswerOnUncheckedTODO verifies that a
 * benchmark run cannot finish while its session TODO.md still has unchecked
 * checklist items and update_todo is registered. */
func TestBenchmarkTurnPolicyBlocksFinalAnswerOnUncheckedTODO(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "TODO.md"), []byte("# TODO\n\n- [ ] unfinished step\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy := newBenchmarkTurnPolicy(root)
	run, err := policy.StartRun(context.Background(), engine.RunInput{Prompt: "task"})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := run.AfterModel(context.Background(), engine.TurnState{
		Turn: 1, Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Complete {
		t.Fatalf("AfterModel() completed while TODO.md still has unchecked items")
	}
	if len(decision.Changes) != 1 || decision.Changes[0].Source != engine.ConversationSourceTODOGate {
		t.Fatalf("AfterModel() changes = %#v, want one TODO completion continuation", decision.Changes)
	}
}

/* TestBenchmarkTurnPolicyAllowsFinalAnswerAfterTODOUpdate verifies that a
 * settled checklist no longer blocks the final answer. */
func TestBenchmarkTurnPolicyAllowsFinalAnswerAfterTODOUpdate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "TODO.md"), []byte("# TODO\n\n- [x] finished step\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	policy := newBenchmarkTurnPolicy(root)
	run, err := policy.StartRun(context.Background(), engine.RunInput{Prompt: "task"})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := run.AfterModel(context.Background(), engine.TurnState{
		Turn: 1, Model: engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Complete {
		t.Fatalf("AfterModel() blocked completion despite a settled checklist: %#v", decision)
	}
}
