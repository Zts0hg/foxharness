package app

import "testing"

func TestAttachInteractiveAskerInstallsAsker(t *testing.T) {
	runner := &AgentRunner{}
	asker := attachInteractiveAsker(runner)
	if asker == nil {
		t.Fatal("attachInteractiveAsker returned nil")
	}
	if runner.userAsker == nil {
		t.Fatal("attachInteractiveAsker did not install the asker on the runner")
	}
}
