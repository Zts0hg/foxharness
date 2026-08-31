package main

import (
	"errors"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* TestRunArmsMemoryExtractionLikeBaseline pins the baseline post-run
 * extraction guard: extraction arms whenever the run returned a result —
 * including blocked and turn-limit failures — and stays off for the mid-run
 * failures that return no result. */
func TestRunArmsMemoryExtractionLikeBaseline(t *testing.T) {
	tests := []struct {
		name    string
		result  foxruntime.RunResult
		runErr  error
		wantArm bool
	}{
		{
			name:    "successful run",
			result:  runGateResult(engine.RunOutcome{FinalMessage: "done"}),
			wantArm: true,
		},
		{
			name:    "turn limit failure with empty text",
			result:  runGateResult(engine.RunOutcome{ErrorKind: "turn_limit", Partial: true, Err: errors.New("超过最大 Turn 数限制: 3")}),
			runErr:  errors.New("超过最大 Turn 数限制: 3"),
			wantArm: true,
		},
		{
			name:    "blocked budget failure with empty text",
			result:  runGateResult(engine.RunOutcome{ErrorKind: "conversation", Err: &foxruntime.ContextBlockedError{UsedTokens: 130000, Limit: 120000}}),
			runErr:  &foxruntime.ContextBlockedError{UsedTokens: 130000, Limit: 120000},
			wantArm: true,
		},
		{
			name:    "mid-run failure with partial text",
			result:  runGateResult(engine.RunOutcome{ErrorKind: "provider", FinalMessage: "partial text", Err: errors.New("模型生成失败: unavailable")}),
			runErr:  errors.New("模型生成失败: unavailable"),
			wantArm: false,
		},
		{
			name:    "run that never started",
			result:  foxruntime.RunResult{},
			runErr:  errors.New("missing LLM configuration"),
			wantArm: false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := runArmsMemoryExtraction(testCase.result); got != testCase.wantArm {
				t.Fatalf("runArmsMemoryExtraction() = %v, want %v", got, testCase.wantArm)
			}
		})
	}
}

func runGateResult(outcome engine.RunOutcome) foxruntime.RunResult {
	return foxruntime.RunResult{SessionID: session.ID("s"), RunID: session.RunID("run-1"), Outcome: outcome}
}
