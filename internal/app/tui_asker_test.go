package app

import (
	"context"
	"testing"

	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestLegacyTUIBindingCanInstallAsker(t *testing.T) {
	runner := &AgentRunner{}
	asker := testInteractiveAsker{}
	runner.SetUserAsker(asker)
	if runner.userAsker == nil {
		t.Fatal("legacy TUI binding did not install the asker on the runner")
	}
}

type testInteractiveAsker struct{}

func (testInteractiveAsker) Ask(context.Context, []tools.Question) ([]tools.Answer, error) {
	return nil, nil
}
