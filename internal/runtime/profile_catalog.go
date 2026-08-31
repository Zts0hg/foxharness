package runtime

import (
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/session"
)

func profiles() map[ProfileName]Profile {
	return map[ProfileName]Profile{
		TUIInteractive: newProfile(ProfileSnapshot{
			Name: TUIInteractive, SessionLifecycle: "selectable_multi_run", SessionSource: session.SOURCECLI,
			ExplicitSession: true, ContinueLatest: true, ForceNewSession: true,
			WorkspaceScope: "launch_fixed", ModelScope: "mutable_future_runs", ContextBudgetPolicy: "selected_model",
			ModelStreaming: true, ModelDeltaObservation: true, TurnLimitMutable: true,
			Scheduling: "session_serial", QuestionPort: "interactive", PlanReview: true,
			PermissionPolicy: "interactive", ThinkingMutable: true,
			MemoryPolicy: "session_automemory_async", Checkpoint: true, Rewind: true,
			CompactionPolicy: "automatic_and_manual", ExtractionPolicy: "asynchronous",
			ObservationPolicy: "ordered_lifecycle_and_deltas", MaxDelegationDepth: 1,
		}, []string{"ask_user_question", "bash", "delegate_task", "edit_file", "read_file", "read_todo", "skill", "update_todo", "write_file"}, nil),
		CLIExec: newProfile(ProfileSnapshot{
			Name: CLIExec, SessionLifecycle: "selectable_single_run", SessionSource: session.SOURCECLI,
			ExplicitSession: true, ContinueLatest: true, ForceNewSession: true,
			WorkspaceScope: "invocation_fixed", ModelScope: "run_fixed", ContextBudgetPolicy: "selected_model",
			ModelStreaming: true, TurnLimitMutable: true,
			Scheduling: "synchronous_once", PermissionPolicy: "none", ThinkingMutable: true,
			MemoryPolicy: "session_automemory_drained", Checkpoint: true, CompactionPolicy: "automatic",
			ExtractionPolicy: "tracked_and_drained", ObservationPolicy: "final_only", MaxDelegationDepth: 1,
		}, []string{"bash", "delegate_task", "edit_file", "read_file", "read_todo", "skill", "update_todo", "write_file"}, nil),
		FeishuRemote: newProfile(ProfileSnapshot{
			Name: FeishuRemote, SessionLifecycle: "remote_reusable", SessionSource: session.SOURCEFeishu,
			ForceNewSession: true,
			WorkspaceScope:  "process_fixed", ModelScope: "process_fixed", ContextBudgetPolicy: "process_model",
			DefaultMaxTurns: 20, MaxTurnsCeiling: 20,
			DefaultTaskTimeout: 5 * time.Minute, MaxConcurrency: 4, Scheduling: "global_bounded_session_serial",
			PermissionPolicy: "ask_remote_approval", MemoryPolicy: "session_automemory_async",
			CompactionPolicy: "automatic", ExtractionPolicy: "fire_and_forget", ObservationPolicy: "facts_no_deltas",
			MaxDelegationDepth: 1,
		}, []string{"bash", "delegate_task", "edit_file", "read_file", "read_todo", "update_todo", "write_file"}, nil),
		AgentOpsTask: newProfile(ProfileSnapshot{
			Name: AgentOpsTask, SessionLifecycle: "fresh_per_task", SessionSource: session.SOURCEFeishu,
			WorkspaceScope: "process_fixed", ModelScope: "task_fixed", ContextBudgetPolicy: "task_model",
			DefaultMaxTurns: 24, MaxTurnsCeiling: 24,
			DefaultTaskTimeout: 5 * time.Minute, MaxConcurrency: 4, Scheduling: "global_bounded",
			PermissionPolicy: "ask_remote_approval", MemoryPolicy: "session_automemory_async",
			CompactionPolicy: "automatic", ExtractionPolicy: "fire_and_forget", ObservationPolicy: "terminal_only",
			MaxDelegationDepth: 1,
		}, []string{"bash", "delegate_task", "edit_file", "log_search", "read_file", "read_todo", "update_todo", "write_file"}, nil),
		BenchmarkEval: newProfile(ProfileSnapshot{
			Name: BenchmarkEval, SessionLifecycle: "fresh_per_repeat", SessionSource: session.SOURCECLI,
			WorkspaceScope: "fresh_fixture_copy", ModelScope: "settings_resolved", ContextBudgetPolicy: "case_model",
			DefaultMaxTurns:  12,
			TurnLimitMutable: true, DefaultTaskTimeout: 10 * time.Minute, TaskTimeoutMutable: true,
			Scheduling: "serial_repeats", PermissionPolicy: "none",
			MemoryPolicy: "session_only", CompactionPolicy: "automatic", ExtractionPolicy: "none",
			ObservationPolicy: "structured_evaluation",
		}, []string{"bash", "edit_file", "read_file", "read_todo", "update_todo", "write_file"}, nil),
		ChildRun: newProfile(ProfileSnapshot{
			Name: ChildRun, SessionLifecycle: "fresh_child", SessionSource: session.SOURCESubagent,
			WorkspaceScope: "parent_inherited", ModelScope: "parent_inherited", ContextBudgetPolicy: "parent_model_snapshot",
			DefaultMaxTurns: 200,
			MaxTurnsCeiling: 200, TurnLimitMutable: true, Scheduling: "parent_waits",
			PermissionPolicy: "parent_ceiling",
			DefaultReadOnly:  true, ReadOnlyMutable: true,
			MemoryPolicy: "isolated_session_shared_readonly_automemory", CompactionPolicy: "automatic",
			ExtractionPolicy: "none", ObservationPolicy: "typed_parent_report",
			DefaultDelegationDepth: 1, MaxDelegationDepth: 1,
		}, []string{"bash", "edit_file", "read_file", "write_file"}, []string{"bash", "read_file"}),
		AutodevPipeline: newProfile(ProfileSnapshot{
			Name: AutodevPipeline, SessionLifecycle: "fresh_per_item_attempt", SessionSource: session.SOURCECLI,
			WorkspaceScope: "item_worktree", ModelScope: "config_override", ContextBudgetPolicy: "item_model",
			TurnLimitMutable: true,
			Scheduling:       "serial_items", QuestionPort: "engineer", PermissionPolicy: "full_access_no_human",
			MemoryPolicy: "session_automemory_async", Checkpoint: true, CompactionPolicy: "automatic",
			ExtractionPolicy: "item_owned_drain_close", ObservationPolicy: "typed_core_outcome", MaxDelegationDepth: 1,
		}, []string{"AskUserQuestion", "ask_user_question", "bash", "delegate_task", "edit_file", "read_file", "read_todo", "skill", "update_todo", "write_file"}, nil),
	}
}

func newProfile(snapshot ProfileSnapshot, tools, readOnlyTools []string) Profile {
	snapshot.ToolCeiling = strings.Join(tools, ",")
	return Profile{
		snapshot:      snapshot,
		toolCeiling:   append([]string(nil), tools...),
		readOnlyTools: append([]string(nil), readOnlyTools...),
	}
}
