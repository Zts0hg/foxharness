package app

import (
	"testing"

	"github.com/Zts0hg/foxharness/internal/subagent"
)

func TestSubagentForkRunner_UsesLiveGetters(t *testing.T) {
	mgrCalls := 0
	sessCalls := 0
	r := &subagentForkRunner{
		getManager: func() *subagent.Manager {
			mgrCalls++
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
	if mgrCalls == 0 {
		t.Error("getManager must be called at Run time, not snapshot")
	}
	// Run twice — getters must be re-invoked, proving the runner does not
	// cache stale state across calls.
	_, _ = r.Run(t.Context(), "task2", "agent", nil)
	if mgrCalls != 2 {
		t.Errorf("expected getManager to be called per Run, got %d", mgrCalls)
	}
}
