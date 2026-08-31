package autodev

import "context"

// Reporter receives every observable autodev event. It embeds CoreReporter so
// the same reporter can stream core messages and tool calls, and it adds the
// control-plane orchestration events so nothing material happens silently
// (REQ-024, REQ-026, NFR-004).
type Reporter interface {
	CoreReporter

	// OnItemStart announces that item processing began (index/total are
	// 1-based progress within this run).
	OnItemStart(ctx context.Context, index, total int, item LedgerItem)
	// OnWorktree announces the worktree and branch bound to the item.
	OnWorktree(ctx context.Context, wt Worktree)
	// OnStageStart announces that the control plane is driving a step.
	OnStageStart(ctx context.Context, slug, stage string)
	// OnEngineerDecision streams an ask_user_question exchange answered by
	// the engineer Agent on the user's behalf.
	OnEngineerDecision(ctx context.Context, questions []Question, answers []Answer)
	// OnEngineerReview streams the corrective instruction the engineer
	// Agent fed back after reviewing a core run.
	OnEngineerReview(ctx context.Context, stage, instruction string)
	// OnVerify reports a step's ground-truth verification outcome.
	OnVerify(ctx context.Context, stage string, ok bool, gap string)
	// OnGate reports the completion-gate outcome for the item's worktree.
	OnGate(ctx context.Context, result GateResult)
	// OnIssue reports the verified GitHub issue number.
	OnIssue(ctx context.Context, number int)
	// OnRemoteEvent consumes one durable logical event idempotently. A
	// delivery error leaves the event pending for a later retry.
	OnRemoteEvent(ctx context.Context, event RemoteEvent) error
	// OnPR reports the verified pull-request number.
	OnPR(ctx context.Context, number int)
	// OnItemDone announces that the item completed and was recorded done.
	OnItemDone(ctx context.Context, item LedgerItem)
	// OnInfo carries control-plane notices such as gate warnings and
	// cleanup actions.
	OnInfo(ctx context.Context, msg string)
}
