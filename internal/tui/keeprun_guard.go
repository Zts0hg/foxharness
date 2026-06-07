package tui

import (
	"context"
	"encoding/json"

	"github.com/Zts0hg/foxharness/internal/keeprun"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
)

// mergeGuard is a tool middleware that denies any bash command performing a
// branch or PR merge, enforcing keep-run's merge prohibition by construction
// (FR-010). It is installed on the tool registry used for keep-run phase runs.
// A guard is required because a merge is a bash command (e.g. "git merge"), not
// a distinct tool that could be withheld from the allow-list. It mirrors the
// existing DangerMiddle pattern.
type mergeGuard struct{}

// BeforeExecute denies bash calls whose command merges (git/gh/glab merge); all
// other calls are allowed.
func (mergeGuard) BeforeExecute(_ context.Context, call schema.ToolCall) (middleware.Decision, error) {
	if call.Name != "bash" {
		return middleware.Allow(), nil
	}
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return middleware.Allow(), nil
	}
	if keeprun.MergeProhibited(args.Command) {
		return middleware.Deny("keep-run prohibits merging into an integration branch: " + args.Command), nil
	}
	return middleware.Allow(), nil
}
