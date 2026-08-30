package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/session"
)

/* ProfileName identifies one confirmed runtime behavior bundle. */
type ProfileName string

const (
	/* TUIInteractive selects the interactive terminal runtime policy. */
	TUIInteractive ProfileName = "TUIInteractive"
	/* CLIExec selects the synchronous command-line runtime policy. */
	CLIExec ProfileName = "CLIExec"
	/* FeishuRemote selects the reusable Feishu task runtime policy. */
	FeishuRemote ProfileName = "FeishuRemote"
	/* AgentOpsTask selects the fresh-session incident task runtime policy. */
	AgentOpsTask ProfileName = "AgentOpsTask"
	/* BenchmarkEval selects the isolated benchmark repeat runtime policy. */
	BenchmarkEval ProfileName = "BenchmarkEval"
	/* ChildRun selects the isolated depth-one child runtime policy. */
	ChildRun ProfileName = "ChildRun"
	/* AutodevPipeline selects the item-scoped Autodev runtime policy. */
	AutodevPipeline ProfileName = "AutodevPipeline"
)

var profileOrder = [...]ProfileName{
	TUIInteractive,
	CLIExec,
	FeishuRemote,
	AgentOpsTask,
	BenchmarkEval,
	ChildRun,
	AutodevPipeline,
}

/* ProfileSnapshot is the flat immutable representation of one resolved profile. */
type ProfileSnapshot struct {
	Name                   ProfileName
	SessionLifecycle       string
	SessionSource          session.Source
	ExplicitSession        bool
	ContinueLatest         bool
	ForceNewSession        bool
	WorkspaceScope         string
	ModelScope             string
	ContextBudgetPolicy    string
	ModelStreaming         bool
	ModelDeltaObservation  bool
	DefaultMaxTurns        int
	MaxTurnsCeiling        int
	TurnLimitMutable       bool
	DefaultTaskTimeout     time.Duration
	TaskTimeoutMutable     bool
	MaxConcurrency         int
	Scheduling             string
	ToolCeiling            string
	QuestionPort           string
	PlanReview             bool
	PermissionPolicy       string
	DefaultThinking        bool
	ThinkingMutable        bool
	DefaultReadOnly        bool
	ReadOnlyMutable        bool
	MemoryPolicy           string
	Checkpoint             bool
	Rewind                 bool
	CompactionPolicy       string
	ExtractionPolicy       string
	ObservationPolicy      string
	DefaultDelegationDepth int
	MaxDelegationDepth     int
}

/* Profile is an immutable resolved runtime policy and capability ceiling. */
type Profile struct {
	snapshot      ProfileSnapshot
	toolCeiling   []string
	readOnlyTools []string
}

/* RunSpec contains caller-owned values that vary for one runtime execution. */
type RunSpec struct {
	Prompt            string
	DisplayPrompt     string
	SessionID         string
	ForceNewSession   bool
	ContinueLatest    bool
	CollaborationMode string
	ProviderProtocol  string
	Model             string
	Effort            string
	WorkDir           string
	LogDir            string
	Task              string
	BenchmarkCase     string
	ParentSessionID   session.ID
	MaxTurns          *int
	TaskTimeout       *time.Duration
	Thinking          *bool
	ReadOnly          *bool
	AllowedTools      []string
	DelegationDepth   *int
	Observer          RunObserver
	Permission        PermissionScope
	childPermission   *ChildPermissionRequest
}

/* RunSnapshot is the flat immutable diagnostic form of a resolved RunSpec. */
type RunSnapshot struct {
	Profile           ProfileName
	Prompt            string
	DisplayPrompt     string
	SessionID         string
	ForceNewSession   bool
	ContinueLatest    bool
	CollaborationMode string
	ProviderProtocol  string
	Model             string
	Effort            string
	WorkDir           string
	LogDir            string
	Task              string
	BenchmarkCase     string
	ParentSessionID   session.ID
	MaxTurns          int
	TaskTimeout       time.Duration
	Thinking          bool
	ReadOnly          bool
	AllowedTools      string
	DelegationDepth   int
}

/* ResolvedRunSpec is a validated run input frozen against one Profile. */
type ResolvedRunSpec struct {
	snapshot        RunSnapshot
	tools           []string
	restrictedTools bool
	observer        RunObserver
	permission      PermissionScope
	childPermission *ChildPermissionRequest
}

/* ProfileNames returns the seven profiles in their stable public order. */
func ProfileNames() []ProfileName {
	result := make([]ProfileName, len(profileOrder))
	copy(result, profileOrder[:])
	return result
}

/* ResolveProfile returns the immutable policy identified by name. */
func ResolveProfile(name ProfileName) (Profile, error) {
	profile, ok := profiles()[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown runtime profile %q", name)
	}
	return profile, nil
}

/* Snapshot returns the profile's flat immutable value representation. */
func (p Profile) Snapshot() ProfileSnapshot {
	return p.snapshot
}

/* Resolve validates and freezes caller-owned values under the profile ceilings. */
func (p Profile) Resolve(spec RunSpec) (ResolvedRunSpec, error) {
	if p.snapshot.Name == "" {
		return ResolvedRunSpec{}, fmt.Errorf("runtime profile is not resolved")
	}
	if err := validateSessionSelection(p.snapshot, spec); err != nil {
		return ResolvedRunSpec{}, err
	}

	maxTurns, err := resolveMaxTurns(p.snapshot, spec.MaxTurns)
	if err != nil {
		return ResolvedRunSpec{}, err
	}
	taskTimeout, err := resolveTaskTimeout(p.snapshot, spec.TaskTimeout)
	if err != nil {
		return ResolvedRunSpec{}, err
	}
	thinking, err := resolveBool("thinking", p.snapshot.DefaultThinking, p.snapshot.ThinkingMutable, spec.Thinking)
	if err != nil {
		return ResolvedRunSpec{}, err
	}
	readOnly, err := resolveBool("read-only", p.snapshot.DefaultReadOnly, p.snapshot.ReadOnlyMutable, spec.ReadOnly)
	if err != nil {
		return ResolvedRunSpec{}, err
	}
	depth, err := resolveDelegationDepth(p.snapshot, spec.DelegationDepth)
	if err != nil {
		return ResolvedRunSpec{}, err
	}

	toolCeiling := p.toolCeiling
	if readOnly {
		toolCeiling = p.readOnlyTools
	}
	tools, err := narrowTools(toolCeiling, spec.AllowedTools)
	if err != nil {
		return ResolvedRunSpec{}, err
	}

	return ResolvedRunSpec{
		snapshot: RunSnapshot{
			Profile: p.snapshot.Name, Prompt: spec.Prompt, DisplayPrompt: spec.DisplayPrompt,
			SessionID: spec.SessionID, ForceNewSession: spec.ForceNewSession, ContinueLatest: spec.ContinueLatest,
			CollaborationMode: spec.CollaborationMode, ProviderProtocol: spec.ProviderProtocol,
			Model: spec.Model, Effort: spec.Effort, WorkDir: spec.WorkDir, LogDir: spec.LogDir,
			Task: spec.Task, BenchmarkCase: spec.BenchmarkCase, ParentSessionID: spec.ParentSessionID,
			MaxTurns: maxTurns, TaskTimeout: taskTimeout, Thinking: thinking, ReadOnly: readOnly,
			AllowedTools: strings.Join(tools, ","), DelegationDepth: depth,
		},
		tools: cloneToolNames(tools),
		/* ChildRun prompt guidance is always capability-scoped, so its runs are
		 * marked restricted even when the caller supplied no explicit restriction. */
		restrictedTools: spec.AllowedTools != nil || p.snapshot.Name == ChildRun,
		observer:        spec.Observer,
		permission:      spec.Permission,
		childPermission: cloneChildPermissionRequest(spec.childPermission),
	}, nil
}

/* Snapshot returns a copy-only diagnostic representation of the resolved run. */
func (s ResolvedRunSpec) Snapshot() RunSnapshot {
	return s.snapshot
}

/* AllowedTools returns a defensive copy of the effective capability names. */
func (s ResolvedRunSpec) AllowedTools() []string {
	return cloneToolNames(s.tools)
}

/* Observer returns the run-scoped observer supplied by the caller. */
func (s ResolvedRunSpec) Observer() RunObserver {
	return s.observer
}

/* Permission returns the run-scoped permission authority supplied by the caller. */
func (s ResolvedRunSpec) Permission() PermissionScope {
	return s.permission
}

func validateSessionSelection(snapshot ProfileSnapshot, spec RunSpec) error {
	selected := 0
	if spec.SessionID != "" {
		selected++
	}
	if spec.ForceNewSession {
		selected++
	}
	if spec.ContinueLatest {
		selected++
	}
	if selected > 1 {
		return fmt.Errorf("runtime profile %s received conflicting session selection", snapshot.Name)
	}
	if spec.SessionID != "" && !snapshot.ExplicitSession {
		return fmt.Errorf("runtime profile %s does not permit explicit session selection", snapshot.Name)
	}
	if spec.ContinueLatest && !snapshot.ContinueLatest {
		return fmt.Errorf("runtime profile %s does not permit latest-session selection", snapshot.Name)
	}
	if spec.ForceNewSession && !snapshot.ForceNewSession {
		return fmt.Errorf("runtime profile %s does not permit forced-new session selection", snapshot.Name)
	}
	return nil
}

func resolveMaxTurns(snapshot ProfileSnapshot, requested *int) (int, error) {
	if requested == nil {
		return snapshot.DefaultMaxTurns, nil
	}
	value := *requested
	if snapshot.DefaultMaxTurns > 0 && value <= 0 {
		return 0, fmt.Errorf("runtime profile %s cannot expand a bounded turn budget to unlimited", snapshot.Name)
	}
	if snapshot.MaxTurnsCeiling > 0 && value > snapshot.MaxTurnsCeiling {
		return 0, fmt.Errorf("runtime profile %s turn budget %d exceeds ceiling %d", snapshot.Name, value, snapshot.MaxTurnsCeiling)
	}
	if !snapshot.TurnLimitMutable && value != snapshot.DefaultMaxTurns && !(value > 0 && value < snapshot.DefaultMaxTurns) {
		return 0, fmt.Errorf("runtime profile %s does not permit turn-budget expansion", snapshot.Name)
	}
	return value, nil
}

func resolveTaskTimeout(snapshot ProfileSnapshot, requested *time.Duration) (time.Duration, error) {
	fallback := snapshot.DefaultTaskTimeout
	if requested == nil {
		return fallback, nil
	}
	if *requested <= 0 {
		return 0, fmt.Errorf("runtime profile %s task timeout must be positive", snapshot.Name)
	}
	if !snapshot.TaskTimeoutMutable {
		return 0, fmt.Errorf("runtime profile %s does not permit task-timeout overrides", snapshot.Name)
	}
	return *requested, nil
}

func resolveBool(name string, fallback, mutable bool, requested *bool) (bool, error) {
	if requested == nil {
		return fallback, nil
	}
	if !mutable && *requested != fallback {
		return false, fmt.Errorf("runtime profile does not permit %s override", name)
	}
	return *requested, nil
}

func resolveDelegationDepth(snapshot ProfileSnapshot, requested *int) (int, error) {
	if requested == nil {
		return snapshot.DefaultDelegationDepth, nil
	}
	if *requested < 0 || *requested > snapshot.MaxDelegationDepth {
		return 0, fmt.Errorf("runtime profile %s delegation depth %d exceeds ceiling %d", snapshot.Name, *requested, snapshot.MaxDelegationDepth)
	}
	if *requested != snapshot.DefaultDelegationDepth {
		return 0, fmt.Errorf("runtime profile %s cannot change its delegation depth", snapshot.Name)
	}
	return *requested, nil
}

func narrowTools(ceiling, requested []string) ([]string, error) {
	if requested == nil {
		return cloneToolNames(ceiling), nil
	}
	allowed := make(map[string]struct{}, len(ceiling))
	for _, name := range ceiling {
		allowed[name] = struct{}{}
	}
	selected := make(map[string]struct{}, len(requested))
	for _, name := range requested {
		if _, ok := allowed[name]; ok {
			selected[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(selected))
	for _, name := range ceiling {
		if _, ok := selected[name]; ok {
			result = append(result, name)
		}
	}
	return result, nil
}

func cloneToolNames(tools []string) []string {
	if tools == nil {
		return nil
	}
	return append([]string{}, tools...)
}
