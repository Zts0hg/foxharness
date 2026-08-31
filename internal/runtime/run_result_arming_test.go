package runtime

import (
	"errors"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
)

/* TestRunResultReturnedResultArmsLikeTheBaseline pins the post-run hook arming
 * condition: the baseline fired whenever its Run returned a result, which
 * turn-limit, gate, and blocked-budget terminations did, while mid-run
 * persistence and model failures returned no result at all. */
func TestRunResultReturnedResultArmsLikeTheBaseline(t *testing.T) {
	tests := []struct {
		name string
		data RunResult
		want bool
	}{
		{
			name: "completed run",
			data: RunResult{RunID: "run-1"},
			want: true,
		},
		{
			name: "turn-limit partial",
			data: RunResult{RunID: "run-1", Outcome: engine.RunOutcome{Err: errors.New("超过最大 Turn 数限制: 3"), Partial: true}},
			want: true,
		},
		{
			name: "blocked budget",
			data: RunResult{RunID: "run-1", Outcome: engine.RunOutcome{Err: &ContextBlockedError{UsedTokens: 101, Limit: 100}}},
			want: true,
		},
		{
			name: "mid-run model failure",
			data: RunResult{RunID: "run-1", Outcome: engine.RunOutcome{Err: engine.ErrPromptTooLong}},
			want: false,
		},
		{
			name: "mid-run persistence failure",
			data: RunResult{RunID: "run-1", Outcome: engine.RunOutcome{Err: errors.New("写入 Session 工具结果失败: unavailable")}},
			want: false,
		},
		{
			name: "run never started",
			data: RunResult{},
			want: false,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.data.ReturnedResult(); got != testCase.want {
				t.Fatalf("ReturnedResult() = %v, want %v", got, testCase.want)
			}
		})
	}
}
