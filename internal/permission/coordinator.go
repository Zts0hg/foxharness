package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// UserApprover prompts the user for explicit approval.
type UserApprover interface {
	Approve(ctx context.Context, request ApprovalRequest) (UserDecision, error)
}

// EventSink receives low-noise review status events.
type EventSink interface {
	OnReviewStart(request Request)
	OnReviewRetry(request Request, attempt int)
	OnAutoApproved(request Request, result ReviewResult)
	OnEscalated(request Request, result ReviewResult)
}

// StateChangeSink receives permission-state changes caused by approval flow.
type StateChangeSink interface {
	OnPermissionStateChanged()
}

// EvidenceProvider builds bounded reviewer context for a request.
type EvidenceProvider func(request Request) Evidence

// ApprovalRequest contains the visible approval prompt state.
type ApprovalRequest struct {
	Request         Request
	Review          *ReviewResult
	ReviewerFailure string
}

// UserDecision is the user's approval form outcome.
type UserDecision struct {
	Kind     UserDecisionKind
	Feedback string
}

// UserDecisionKind enumerates explicit user actions.
type UserDecisionKind string

const (
	UserAllowOnce    UserDecisionKind = "allow_once"
	UserAllowSession UserDecisionKind = "allow_session"
	UserDeny         UserDecisionKind = "deny"
	UserDenyFeedback UserDecisionKind = "deny_feedback"
)

// Coordinator serializes approval and owns session grants.
type Coordinator struct {
	mu        sync.Mutex
	state     *State
	workspace string
	cwd       string
	source    Source
	approver  UserApprover
	reviewer  Reviewer
	sink      EventSink
	evidence  EvidenceProvider
}

// Config creates a coordinator for one interactive runtime.
type Config struct {
	State     *State
	Workspace string
	CWD       string
	Source    Source
	Approver  UserApprover
	Reviewer  Reviewer
	Sink      EventSink
	Evidence  EvidenceProvider
}

// NewCoordinator creates a permission coordinator.
func NewCoordinator(cfg Config) *Coordinator {
	if cfg.State == nil {
		cfg.State = NewState(ModeAsk, false)
	}
	if cfg.Source == "" {
		cfg.Source = SourceMain
	}
	return &Coordinator{state: cfg.State, workspace: cfg.Workspace, cwd: cfg.CWD, source: cfg.Source, approver: cfg.Approver, reviewer: cfg.Reviewer, sink: cfg.Sink, evidence: cfg.Evidence}
}

// State returns the shared permission state.
func (c *Coordinator) State() *State { return c.state }

// WithSource creates a coordinator view with the same state and reviewers but a distinct source.
func (c *Coordinator) WithSource(source Source) *Coordinator {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if source == "" {
		source = c.source
	}
	return &Coordinator{
		state:     c.state,
		workspace: c.workspace,
		cwd:       c.cwd,
		source:    source,
		approver:  c.approver,
		reviewer:  c.reviewer,
		sink:      c.sink,
		evidence:  c.evidence,
	}
}

// Authorize returns nil when a tool call may execute.
func (c *Coordinator) Authorize(ctx context.Context, call schema.ToolCall, assessments ...toolpolicy.Assessment) error {
	return c.authorize(ctx, call, c.evidence, assessments...)
}

func (c *Coordinator) authorize(ctx context.Context, call schema.ToolCall, evidenceProvider EvidenceProvider, assessments ...toolpolicy.Assessment) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := c.state.Snapshot()
	decision := Classify(c.workspace, c.cwd, c.source, call, assessments...)
	request := decision.Request
	if snap.EffectiveMode == ModeFullAccess || decision.AllowFastPath {
		return nil
	}
	if _, ok := c.state.MatchingGrant(request); ok {
		return nil
	}
	if snap.EffectiveMode == ModeApprove && c.reviewer != nil && !decision.HumanOnly {
		if c.sink != nil {
			c.sink.OnReviewStart(request)
		}
		evidence := BuildEvidence(nil, nil, request)
		if evidenceProvider != nil {
			evidence = evidenceProvider(request)
		}
		result, err := c.reviewer.Review(ctx, request, evidence)
		if err == nil && result.Decision == ReviewApprove {
			if c.sink != nil {
				c.sink.OnAutoApproved(request, result)
			}
			return nil
		}
		if c.sink != nil {
			c.sink.OnEscalated(request, result)
		}
		return c.askUser(ctx, request, result, err)
	}
	return c.askUser(ctx, request, ReviewResult{}, nil)
}

func (c *Coordinator) askUser(ctx context.Context, request Request, review ReviewResult, reviewErr error) error {
	if c.approver == nil {
		return fmt.Errorf("permission approval required for %s but no approver is attached", request.Action)
	}
	approval := ApprovalRequest{Request: request}
	if review.Decision != "" {
		approval.Review = &review
	}
	if reviewErr != nil {
		approval.ReviewerFailure = reviewErr.Error()
	}
	decision, err := c.approver.Approve(ctx, approval)
	if err != nil {
		return err
	}
	switch decision.Kind {
	case UserAllowOnce:
		return nil
	case UserAllowSession:
		c.state.AddGrant(GrantForRequest(request))
		if sink, ok := c.sink.(StateChangeSink); ok {
			sink.OnPermissionStateChanged()
		}
		return nil
	case UserDenyFeedback:
		if decision.Feedback != "" {
			return fmt.Errorf("tool denied with feedback: %s", decision.Feedback)
		}
		return fmt.Errorf("tool denied with feedback")
	default:
		return fmt.Errorf("tool denied by user")
	}
}

// Registry decorates an existing tool registry with permission checks.
type Registry struct {
	base        tools.Registry
	coordinator *Coordinator
	evidence    EvidenceProvider
}

// DecorateRegistry wraps base with permission checks.
func DecorateRegistry(base tools.Registry, coordinator *Coordinator, providers ...EvidenceProvider) tools.Registry {
	if base == nil || coordinator == nil {
		return base
	}
	evidence := coordinator.evidence
	if len(providers) > 0 {
		evidence = providers[0]
	}
	return &Registry{base: base, coordinator: coordinator, evidence: evidence}
}

func (r *Registry) Register(tool tools.BaseTool) { r.base.Register(tool) }
func (r *Registry) Use(m middleware.Middleware)  { r.base.Use(m) }

// GetAvailableTools delegates tool discovery.
func (r *Registry) GetAvailableTools() []schema.ToolDefinition { return r.base.GetAvailableTools() }

// Execute authorizes the call before delegating to the base registry.
func (r *Registry) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	if !toolAdvertised(r.base, call.Name) {
		return r.base.Execute(ctx, call)
	}
	assessment := missingAssessment(call.Name)
	if registry, ok := r.base.(tools.PermissionRegistry); ok {
		candidate, found, err := registry.AssessPermission(call.Name, toolpolicy.Context{
			Workspace: r.coordinator.workspace,
			CWD:       r.coordinator.cwd,
			Source:    string(r.coordinator.source),
		}, call.Arguments)
		switch {
		case err != nil:
			assessment.Reason = "tool capability assessment failed: " + err.Error()
		case found:
			assessment = candidate
		}
	}
	evidenceProvider := func(request Request) Evidence {
		evidence := BuildEvidence(nil, nil, request)
		if r.evidence != nil {
			evidence = r.evidence(request)
		}
		evidence.Correlation.ToolCallID = call.ID
		return evidence
	}
	if err := r.coordinator.authorize(ctx, call, evidenceProvider, assessment); err != nil {
		return schema.ToolResult{ToolCallID: call.ID, Output: "Tool execution denied by permission policy: " + err.Error(), IsError: true}
	}
	return r.base.Execute(ctx, call)
}

func toolAdvertised(registry tools.Registry, name string) bool {
	for _, definition := range registry.GetAvailableTools() {
		if definition.Name == name {
			return true
		}
	}
	return false
}

// IsParallelSafe disables parallel execution while approvals may be interactive.
func (r *Registry) IsParallelSafe(toolName string) bool { return false }

// AssessPermission preserves capability metadata through nested registry
// decorators.
func (r *Registry) AssessPermission(name string, ctx toolpolicy.Context, args json.RawMessage) (toolpolicy.Assessment, bool, error) {
	registry, ok := r.base.(tools.PermissionRegistry)
	if !ok {
		return toolpolicy.Assessment{}, false, nil
	}
	return registry.AssessPermission(name, ctx, args)
}

// MarshalJSON supports small debug snapshots in tests.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	type alias Snapshot
	return json.Marshal(alias(s))
}

var _ tools.Registry = (*Registry)(nil)
var _ tools.PermissionRegistry = (*Registry)(nil)
