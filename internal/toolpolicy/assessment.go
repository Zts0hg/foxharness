// Package toolpolicy defines permission-relevant capabilities without coupling
// tools to the approval coordinator.
package toolpolicy

// Context is the runtime scope available while assessing one invocation.
type Context struct {
	Workspace string
	CWD       string
	Source    string
}

// Behavior controls which approval path may handle an invocation.
type Behavior string

const (
	BehaviorFastAllow  Behavior = "fast_allow"
	BehaviorReviewable Behavior = "reviewable"
	BehaviorHumanOnly  Behavior = "human_only"
)

// Effect describes the externally relevant behavior of a tool call.
type Effect string

const (
	EffectObserve  Effect = "observe"
	EffectMutate   Effect = "mutate"
	EffectExecute  Effect = "execute"
	EffectNetwork  Effect = "network"
	EffectDelegate Effect = "delegate"
	EffectWorkflow Effect = "workflow"
	EffectInteract Effect = "interact"
	EffectUnknown  Effect = "unknown"
)

// Scope describes where an invocation may have effects.
type Scope string

const (
	ScopeWorkspace Scope = "workspace"
	ScopeExternal  Scope = "external"
	ScopeMixed     Scope = "mixed"
	ScopeUnknown   Scope = "unknown"
)

// Risk is a coarse deterministic hint for display and fail-closed fallback.
type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

// Assessment is a tool-owned description of one exact invocation.
type Assessment struct {
	Behavior          Behavior
	Action            string
	Effects           []Effect
	Scope             Scope
	ReadOnly          bool
	NestedEnforcement bool
	RiskHint          Risk
	Reason            string
	Target            string
	Commands          []string
}
