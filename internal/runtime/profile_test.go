package runtime

import (
	"reflect"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/session"
)

func TestProfilesResolveToConfirmedFlatSnapshots(t *testing.T) {
	tests := []struct {
		name ProfileName
		want ProfileSnapshot
	}{
		{TUIInteractive, ProfileSnapshot{Name: TUIInteractive, SessionLifecycle: "selectable_multi_run", SessionSource: session.SOURCECLI, ExplicitSession: true, ContinueLatest: true, ForceNewSession: true, WorkspaceScope: "launch_fixed", ModelScope: "mutable_future_runs", ContextBudgetPolicy: "selected_model", ModelStreaming: true, ModelDeltaObservation: true, DefaultMaxTurns: 0, TurnLimitMutable: true, Scheduling: "session_serial", ToolCeiling: "ask_user_question,bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file", QuestionPort: "interactive", PlanReview: true, PermissionPolicy: "interactive", DefaultThinking: false, ThinkingMutable: true, MemoryPolicy: "session_automemory_async", Checkpoint: true, Rewind: true, CompactionPolicy: "automatic_and_manual", ExtractionPolicy: "asynchronous", ObservationPolicy: "ordered_lifecycle_and_deltas", MaxDelegationDepth: 1}},
		{CLIExec, ProfileSnapshot{Name: CLIExec, SessionLifecycle: "selectable_single_run", SessionSource: session.SOURCECLI, ExplicitSession: true, ContinueLatest: true, ForceNewSession: true, WorkspaceScope: "invocation_fixed", ModelScope: "run_fixed", ContextBudgetPolicy: "selected_model", ModelStreaming: true, DefaultMaxTurns: 0, TurnLimitMutable: true, Scheduling: "synchronous_once", ToolCeiling: "bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file", PermissionPolicy: "none", DefaultThinking: false, ThinkingMutable: true, MemoryPolicy: "session_automemory_drained", Checkpoint: true, CompactionPolicy: "automatic", ExtractionPolicy: "tracked_and_drained", ObservationPolicy: "final_only", MaxDelegationDepth: 1}},
		{FeishuRemote, ProfileSnapshot{Name: FeishuRemote, SessionLifecycle: "remote_reusable", SessionSource: session.SOURCEFeishu, ForceNewSession: true, WorkspaceScope: "process_fixed", ModelScope: "process_fixed", ContextBudgetPolicy: "process_model", DefaultMaxTurns: 20, MaxTurnsCeiling: 20, DefaultTaskTimeout: 5 * time.Minute, MaxConcurrency: 4, Scheduling: "global_bounded_session_serial", ToolCeiling: "bash,delegate_task,edit_file,read_file,read_todo,update_todo,write_file", PermissionPolicy: "ask_remote_approval", MemoryPolicy: "session_automemory_async", CompactionPolicy: "automatic", ExtractionPolicy: "fire_and_forget", ObservationPolicy: "facts_no_deltas", MaxDelegationDepth: 1}},
		{AgentOpsTask, ProfileSnapshot{Name: AgentOpsTask, SessionLifecycle: "fresh_per_task", SessionSource: session.SOURCEFeishu, WorkspaceScope: "process_fixed", ModelScope: "task_fixed", ContextBudgetPolicy: "task_model", DefaultMaxTurns: 24, MaxTurnsCeiling: 24, DefaultTaskTimeout: 5 * time.Minute, MaxConcurrency: 4, Scheduling: "global_bounded", ToolCeiling: "bash,delegate_task,edit_file,log_search,read_file,read_todo,update_todo,write_file", PermissionPolicy: "ask_remote_approval", MemoryPolicy: "session_automemory_async", CompactionPolicy: "automatic", ExtractionPolicy: "fire_and_forget", ObservationPolicy: "terminal_only", MaxDelegationDepth: 1}},
		{BenchmarkEval, ProfileSnapshot{Name: BenchmarkEval, SessionLifecycle: "fresh_per_repeat", SessionSource: session.SOURCECLI, WorkspaceScope: "fresh_fixture_copy", ModelScope: "settings_resolved", ContextBudgetPolicy: "case_model", DefaultMaxTurns: 12, TurnLimitMutable: true, DefaultTaskTimeout: 10 * time.Minute, TaskTimeoutMutable: true, Scheduling: "serial_repeats", ToolCeiling: "bash,edit_file,read_file,read_todo,update_todo,write_file", PermissionPolicy: "none", MemoryPolicy: "session_only", CompactionPolicy: "automatic", ExtractionPolicy: "none", ObservationPolicy: "structured_evaluation", MaxDelegationDepth: 0}},
		{ChildRun, ProfileSnapshot{Name: ChildRun, SessionLifecycle: "fresh_child", SessionSource: session.SOURCESubagent, WorkspaceScope: "parent_inherited", ModelScope: "parent_inherited", ContextBudgetPolicy: "parent_model_snapshot", DefaultMaxTurns: 200, MaxTurnsCeiling: 200, TurnLimitMutable: true, Scheduling: "parent_waits", ToolCeiling: "bash,edit_file,read_file,write_file", PermissionPolicy: "parent_ceiling", DefaultReadOnly: true, ReadOnlyMutable: true, MemoryPolicy: "isolated_session_shared_readonly_automemory", CompactionPolicy: "automatic", ExtractionPolicy: "none", ObservationPolicy: "typed_parent_report", DefaultDelegationDepth: 1, MaxDelegationDepth: 1}},
		{AutodevPipeline, ProfileSnapshot{Name: AutodevPipeline, SessionLifecycle: "fresh_per_item_attempt", SessionSource: session.SOURCECLI, WorkspaceScope: "item_worktree", ModelScope: "config_override", ContextBudgetPolicy: "item_model", DefaultMaxTurns: 0, TurnLimitMutable: true, Scheduling: "serial_items", ToolCeiling: "AskUserQuestion,ask_user_question,bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file", QuestionPort: "engineer", PermissionPolicy: "full_access_no_human", MemoryPolicy: "session_automemory_async", Checkpoint: true, CompactionPolicy: "automatic", ExtractionPolicy: "item_owned_drain_close", ObservationPolicy: "typed_core_outcome", MaxDelegationDepth: 1}},
	}

	for _, test := range tests {
		t.Run(string(test.name), func(t *testing.T) {
			profile, err := ResolveProfile(test.name)
			if err != nil {
				t.Fatal(err)
			}
			if got := profile.Snapshot(); got != test.want {
				t.Fatalf("profile snapshot = %#v, want %#v", got, test.want)
			}
		})
	}

	if names := ProfileNames(); !reflect.DeepEqual(names, []ProfileName{TUIInteractive, CLIExec, FeishuRemote, AgentOpsTask, BenchmarkEval, ChildRun, AutodevPipeline}) {
		t.Fatalf("profile names = %v", names)
	}
	if _, err := ResolveProfile(ProfileName("unknown")); err == nil {
		t.Fatal("unknown profile resolved")
	}
	for _, name := range ProfileNames() {
		snapshot := mustProfile(t, name).Snapshot()
		if snapshot.ContextBudgetPolicy == "" {
			t.Errorf("%s has no context-budget policy", name)
		}
		if snapshot.ModelDeltaObservation && !snapshot.ModelStreaming {
			t.Errorf("%s observes model deltas without streaming", name)
		}
	}
}

func TestProfileSnapshotIsFlatAndContainsNoPerRunValues(t *testing.T) {
	typeOf := reflect.TypeOf(ProfileSnapshot{})
	forbidden := map[string]bool{
		"Prompt": true, "DisplayPrompt": true, "SessionID": true, "CollaborationMode": true,
		"ProviderProtocol": true, "Model": true, "Effort": true, "WorkDir": true,
		"LogDir": true, "Task": true, "BenchmarkCase": true, "ParentSessionID": true,
		"Observer": true, "AllowedTools": true,
	}
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if forbidden[field.Name] {
			t.Errorf("ProfileSnapshot owns per-run field %s", field.Name)
		}
		switch field.Type.Kind() {
		case reflect.Array, reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.Struct:
			t.Errorf("ProfileSnapshot.%s is not a flat immutable value: %s", field.Name, field.Type)
		}
	}

	names := ProfileNames()
	names[0] = ProfileName("mutated")
	if ProfileNames()[0] != TUIInteractive {
		t.Fatal("caller mutated the stable profile-name catalog")
	}
}

func TestRunSpecResolvesImmutableNarrowedToolSnapshot(t *testing.T) {
	profile, err := ResolveProfile(TUIInteractive)
	if err != nil {
		t.Fatal(err)
	}
	allowed := []string{"read_file", "bash", "read_file"}
	resolved, err := profile.Resolve(RunSpec{Prompt: "inspect", AllowedTools: allowed})
	if err != nil {
		t.Fatal(err)
	}
	allowed[0] = "write_file"
	if got, want := resolved.Snapshot().AllowedTools, "bash,read_file"; got != want {
		t.Fatalf("resolved tools = %q, want %q", got, want)
	}
	tools := resolved.AllowedTools()
	tools[0] = "write_file"
	if got, want := resolved.AllowedTools(), []string{"bash", "read_file"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("caller mutated resolved tool snapshot: %v", got)
	}

	intersected, err := profile.Resolve(RunSpec{AllowedTools: []string{"read_file", "not-in-profile"}})
	if err != nil || intersected.Snapshot().AllowedTools != "read_file" {
		t.Fatalf("ceiling intersection = %#v, %v", intersected.Snapshot(), err)
	}
	if _, err := profile.Resolve(RunSpec{ReadOnly: boolValue(true)}); err == nil {
		t.Fatal("TUI accepted unsupported read-only variation")
	}
}

func TestRunSpecCannotRelaxBudgetThinkingOrDelegationCeilings(t *testing.T) {
	feishu := mustProfile(t, FeishuRemote)
	if _, err := feishu.Resolve(RunSpec{MaxTurns: intValue(21)}); err == nil {
		t.Fatal("Feishu turn ceiling expanded")
	}
	if _, err := feishu.Resolve(RunSpec{MaxTurns: intValue(0)}); err == nil {
		t.Fatal("Feishu bounded turn ceiling became unlimited")
	}
	if got, err := feishu.Resolve(RunSpec{MaxTurns: intValue(10)}); err != nil || got.Snapshot().MaxTurns != 10 {
		t.Fatalf("Feishu narrowed turns = %#v, %v", got.Snapshot(), err)
	}
	if _, err := feishu.Resolve(RunSpec{Thinking: boolValue(true)}); err == nil {
		t.Fatal("Feishu enabled forbidden thinking")
	}

	child := mustProfile(t, ChildRun)
	resolved, err := child.Resolve(RunSpec{ReadOnly: boolValue(true), AllowedTools: []string{"read_file", "bash"}, DelegationDepth: intValue(1)})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resolved.Snapshot().AllowedTools, "bash,read_file"; got != want {
		t.Fatalf("read-only child tools = %q, want %q", got, want)
	}
	empty, err := child.Resolve(RunSpec{ReadOnly: boolValue(true), AllowedTools: []string{"write_file"}})
	if err != nil || empty.Snapshot().AllowedTools != "" {
		t.Fatalf("read-only intersection = %#v, %v", empty.Snapshot(), err)
	}
	for _, spec := range []RunSpec{
		{DelegationDepth: intValue(2)},
		{MaxTurns: intValue(201)},
		{MaxTurns: intValue(0)},
	} {
		if _, err := child.Resolve(spec); err == nil {
			t.Fatalf("child ceiling expanded by %#v", spec)
		}
	}
}

func TestRunSpecValidationRejectsContradictoryDynamicInput(t *testing.T) {
	profile := mustProfile(t, CLIExec)
	for _, spec := range []RunSpec{
		{SessionID: "existing", ForceNewSession: true},
		{SessionID: "existing", ContinueLatest: true},
		{ForceNewSession: true, ContinueLatest: true},
		{DelegationDepth: intValue(-1)},
	} {
		if _, err := profile.Resolve(spec); err == nil {
			t.Fatalf("contradictory run spec accepted: %#v", spec)
		}
	}
}

func TestSessionSelectionRespectsPerProfileCeilings(t *testing.T) {
	feishu := mustProfile(t, FeishuRemote)
	if _, err := feishu.Resolve(RunSpec{ForceNewSession: true}); err != nil {
		t.Fatalf("Feishu force-new rejected: %v", err)
	}
	for _, spec := range []RunSpec{{SessionID: "existing"}, {ContinueLatest: true}} {
		if _, err := feishu.Resolve(spec); err == nil {
			t.Fatalf("Feishu accepted unsupported session selection: %#v", spec)
		}
	}
	for _, name := range []ProfileName{AgentOpsTask, BenchmarkEval, ChildRun, AutodevPipeline} {
		if _, err := mustProfile(t, name).Resolve(RunSpec{ForceNewSession: true}); err == nil {
			t.Fatalf("%s accepted caller-controlled session selection", name)
		}
	}
}

func TestTaskTimeoutResolvesOnlyForProfilePermittedVariation(t *testing.T) {
	benchmark := mustProfile(t, BenchmarkEval)
	if got, err := benchmark.Resolve(RunSpec{}); err != nil || got.Snapshot().TaskTimeout != 10*time.Minute {
		t.Fatalf("benchmark default timeout = %v, %v", got.Snapshot().TaskTimeout, err)
	}
	if got, err := benchmark.Resolve(RunSpec{TaskTimeout: durationValue(30 * time.Second)}); err != nil || got.Snapshot().TaskTimeout != 30*time.Second {
		t.Fatalf("benchmark case timeout = %v, %v", got.Snapshot().TaskTimeout, err)
	}
	if _, err := benchmark.Resolve(RunSpec{TaskTimeout: durationValue(0)}); err == nil {
		t.Fatal("benchmark accepted non-positive case timeout")
	}
	if _, err := mustProfile(t, FeishuRemote).Resolve(RunSpec{TaskTimeout: durationValue(time.Minute)}); err == nil {
		t.Fatal("Feishu accepted a run override for its fixed task timeout")
	}
	if got, err := mustProfile(t, FeishuRemote).Resolve(RunSpec{}); err != nil || got.Snapshot().TaskTimeout != 5*time.Minute {
		t.Fatalf("Feishu fixed timeout = %v, %v", got.Snapshot().TaskTimeout, err)
	}
}

func mustProfile(t *testing.T, name ProfileName) Profile {
	t.Helper()
	profile, err := ResolveProfile(name)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func intValue(value int) *int { return &value }

func boolValue(value bool) *bool { return &value }

func durationValue(value time.Duration) *time.Duration { return &value }
