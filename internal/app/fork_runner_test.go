package app

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/childruntime"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/subagent"
)

func TestSubagentForkRunner_UsesLiveGetters(t *testing.T) {
	runnerCalls := 0
	sessCalls := 0
	r := &subagentForkRunner{
		getRunner: func() subagent.Runner {
			runnerCalls++
			return nil
		},
		getSession: func() string {
			sessCalls++
			return ""
		},
	}
	// Manager is nil, so Run returns an error — but we still want to know
	// that the manager getter was invoked at call time, not at construction.
	_, _ = r.Run(t.Context(), "task", "agent", nil)
	if runnerCalls == 0 {
		t.Error("getRunner must be called at Run time, not snapshot")
	}
	// Run twice — getters must be re-invoked, proving the runner does not
	// cache stale state across calls.
	_, _ = r.Run(t.Context(), "task2", "agent", nil)
	if runnerCalls != 2 {
		t.Errorf("expected getRunner to be called per Run, got %d", runnerCalls)
	}
}

func TestIACHD005ForkRunnerReadsLiveManagerAndSessionForEveryInvocation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	providers := []*forkGetterProvider{{report: "first"}, {report: "second"}}
	parentSessions := []string{"parent-one", "parent-two"}
	call := 0
	runner := &subagentForkRunner{
		getRunner: func() subagent.Runner {
			return childruntime.New(childruntime.Config{
				Provider: providers[call], WorkDir: workDir, ParentProfile: childruntime.TUIInteractive,
				Permission: permission.NewCoordinator(permission.Config{State: permission.NewState(permission.ModeFullAccess, true)}),
			})
		},
		getSession: func() string {
			return parentSessions[call]
		},
	}
	for i := range providers {
		report, err := runner.Run(context.Background(), "task", "general-purpose", []string{"read_file"})
		if err != nil {
			t.Fatal(err)
		}
		if report != providers[i].report || providers[i].calls != 1 || !strings.Contains(providers[i].prompt, parentSessions[i]) {
			t.Fatalf("fork invocation %d = report %q provider calls %d", i, report, providers[i].calls)
		}
		call++
	}
}

type forkGetterProvider struct {
	report string
	calls  int
	prompt string
}

func (p *forkGetterProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	for _, message := range messages {
		p.prompt += message.Content
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: p.report}}, nil
}
