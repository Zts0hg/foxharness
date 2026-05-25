package app

import (
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/slash"
)

// buildHookWithRegistry wires the same hook AgentRunner would use, with
// the supplied registry. It bypasses constructing a full AgentRunner so
// the test stays scoped to the activation-gating behavior.
func buildHookWithRegistry(reg *slash.Registry) func(schema.ToolCall, schema.ToolResult) {
	r := &AgentRunner{slashRegistry: reg}
	return r.conditionalActivationHook()
}

func TestConditionalActivationHook_SuccessActivates(t *testing.T) {
	reg := slash.NewRegistry("").WithoutDiscovery()
	reg.Register(&slash.Command{
		Type: slash.CommandPrompt,
		Name: "go-test",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Paths:         []string{"*_test.go"},
		},
	})
	hook := buildHookWithRegistry(reg)
	args, _ := json.Marshal(map[string]string{"path": "loop_test.go"})
	hook(
		schema.ToolCall{Name: "read_file", Arguments: args},
		schema.ToolResult{IsError: false, Output: "..."},
	)
	if _, ok := reg.Lookup("go-test"); !ok {
		t.Fatal("successful read_file must activate matching conditional skill")
	}
}

func TestConditionalActivationHook_FailureSuppresses(t *testing.T) {
	reg := slash.NewRegistry("").WithoutDiscovery()
	reg.Register(&slash.Command{
		Type: slash.CommandPrompt,
		Name: "go-test",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Paths:         []string{"*_test.go"},
		},
	})
	hook := buildHookWithRegistry(reg)
	args, _ := json.Marshal(map[string]string{"path": "loop_test.go"})
	hook(
		schema.ToolCall{Name: "read_file", Arguments: args},
		schema.ToolResult{IsError: true, Output: "denied by middleware"},
	)
	if _, ok := reg.Lookup("go-test"); ok {
		t.Fatal("failed read_file must NOT activate matching conditional skill")
	}
}

func TestRecordSkillActivation_QueuesReminder(t *testing.T) {
	r := &AgentRunner{}
	cmd := &slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "go-test",
		Description: "Run Go tests",
		Frontmatter: slash.Frontmatter{
			WhenToUse:    "When test files change",
			ArgumentHint: "[package]",
		},
	}
	r.recordSkillActivation(cmd)
	got := r.drainPendingActivations()
	if len(got) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(got))
	}
	if !containsAll(got[0], "go-test", "Run Go tests", "When test files change", "[package]", "skill") {
		t.Errorf("reminder missing key fields: %q", got[0])
	}
	// Second drain returns nothing — drained queue is cleared.
	if again := r.drainPendingActivations(); len(again) != 0 {
		t.Errorf("drain should clear queue, got %v", again)
	}
}

func TestRecordSkillActivation_NilCommandIgnored(t *testing.T) {
	r := &AgentRunner{}
	r.recordSkillActivation(nil)
	if got := r.drainPendingActivations(); len(got) != 0 {
		t.Errorf("nil cmd should not queue: %v", got)
	}
}

func TestConditionalActivationHook_QueuesReminderOnSuccess(t *testing.T) {
	reg := slash.NewRegistry("").WithoutDiscovery()
	r := &AgentRunner{slashRegistry: reg}
	// Wire OnActivate as NewAgentRunner does.
	reg.OnActivate(r.recordSkillActivation)
	reg.Register(&slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "go-test",
		Description: "tests",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Paths:         []string{"*_test.go"},
		},
	})
	hook := r.conditionalActivationHook()
	args, _ := json.Marshal(map[string]string{"path": "loop_test.go"})
	hook(
		schema.ToolCall{Name: "read_file", Arguments: args},
		schema.ToolResult{IsError: false},
	)
	got := r.drainPendingActivations()
	if len(got) != 1 {
		t.Fatalf("expected 1 reminder queued after successful activation, got %d", len(got))
	}
	if !containsAll(got[0], "go-test") {
		t.Errorf("reminder doesn't mention go-test: %q", got[0])
	}
}

func TestConditionalActivationHook_SkipsReminderForNonInvocableSkill(t *testing.T) {
	reg := slash.NewRegistry("").WithoutDiscovery()
	r := &AgentRunner{slashRegistry: reg}
	reg.OnActivate(r.recordSkillActivation)
	reg.Register(&slash.Command{
		Type:        slash.CommandPrompt,
		Name:        "go-test",
		Description: "tests",
		Frontmatter: slash.Frontmatter{
			UserInvocable:          true,
			DisableModelInvocation: true,
			Paths:                  []string{"*_test.go"},
		},
	})
	hook := r.conditionalActivationHook()
	args, _ := json.Marshal(map[string]string{"path": "loop_test.go"})
	hook(
		schema.ToolCall{Name: "read_file", Arguments: args},
		schema.ToolResult{IsError: false},
	)
	if _, ok := reg.Lookup("go-test"); !ok {
		t.Fatal("non-invocable conditional skill should still activate")
	}
	if got := r.drainPendingActivations(); len(got) != 0 {
		t.Fatalf("non-invocable skill should not queue invocation reminder, got %v", got)
	}
}

func containsAll(s string, needles ...string) bool {
	for _, n := range needles {
		if !contains(s, n) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestConditionalActivationHook_IgnoresUnrelatedTools(t *testing.T) {
	reg := slash.NewRegistry("").WithoutDiscovery()
	reg.Register(&slash.Command{
		Type: slash.CommandPrompt,
		Name: "go-test",
		Frontmatter: slash.Frontmatter{
			UserInvocable: true,
			Paths:         []string{"*_test.go"},
		},
	})
	hook := buildHookWithRegistry(reg)
	args, _ := json.Marshal(map[string]string{"path": "loop_test.go"})
	hook(
		schema.ToolCall{Name: "bash", Arguments: args},
		schema.ToolResult{IsError: false},
	)
	if _, ok := reg.Lookup("go-test"); ok {
		t.Fatal("non-file tools must not trigger conditional activation")
	}
}
