package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/session"
)

const childCleanupTimeout = 5 * time.Second

/* ChildOutcomeStatus identifies one terminal nested-runtime state. */
type ChildOutcomeStatus string

const (
	/* ChildSucceeded indicates that the child produced a final committed report. */
	ChildSucceeded ChildOutcomeStatus = "succeeded"
	/* ChildFailed indicates that an admitted child failed after startup. */
	ChildFailed ChildOutcomeStatus = "failed"
	/* ChildCancelled indicates that parent or child cancellation terminated execution. */
	ChildCancelled ChildOutcomeStatus = "cancelled"
	/* ChildTurnExhausted indicates that the child consumed its effective turn budget. */
	ChildTurnExhausted ChildOutcomeStatus = "turn_exhausted"
	/* ChildRejected indicates that runtime admission rejected the nested request. */
	ChildRejected ChildOutcomeStatus = "rejected"
	/* ChildStartFailed indicates failure before a child run identity was established. */
	ChildStartFailed ChildOutcomeStatus = "start_failed"
)

/* ChildCleanup owns run-scoped resource termination after one admitted child invocation. */
type ChildCleanup interface {
	Cleanup(context.Context) error
}

/* PermissionScope derives a least-authority permission scope for one child run. */
type PermissionScope interface {
	ChildScope(context.Context, ChildPermissionRequest) (PermissionScope, error)
}

/* ChildPermissionRequest correlates a child permission scope with immutable lineage. */
type ChildPermissionRequest struct {
	ParentSessionID session.ID
	ParentRunID     session.RunID
	ChildSessionID  session.ID
	ChildRunID      session.RunID
	DelegationID    string
	Agent           string
	ReadOnly        bool
	AllowedTools    []string
}

/* ChildRunRequest contains normalized trusted adapter input for one synchronous child. */
type ChildRunRequest struct {
	InvocationID      string
	DelegationID      string
	Agent             string
	AgentInstructions string
	Task              string
	ReadOnly          bool
	AllowedTools      []string
	AgentAllowedTools []string
	Depth             int
	MaxTurns          *int
	Observer          RunObserver
	Cleanup           ChildCleanup
}

/* ChildRunResult is the single typed parent-visible outcome of one child invocation. */
type ChildRunResult struct {
	InvocationID    string
	SessionID       session.ID
	RunID           session.RunID
	ParentSessionID session.ID
	ParentRunID     session.RunID
	DelegationID    string
	Agent           string
	Depth           int
	Status          ChildOutcomeStatus
	Report          string
	Runtime         RunResult
}

/* FrozenParentRun carries one already-admitted parent run into child composition. */
type FrozenParentRun struct {
	Profile          ProfileName
	SessionID        session.ID
	RunID            session.RunID
	WorkDir          string
	ProviderProtocol string
	Model            string
	Effort           string
	AllowedTools     []string
	Permission       PermissionScope
	Context          context.Context
	DelegationDepth  int
}

/* ChildRunner freezes one live parent run as the sole authority for nested execution. */
type ChildRunner struct {
	harness            *RuntimeHarness
	parentSession      AgentSessionSnapshot
	parentRun          RunScopeSnapshot
	parentSpec         RunSnapshot
	parentTools        []string
	permission         PermissionScope
	permissionRequired bool
	parentContext      context.Context
	parentMaxDepth     int
}

/* NewChildRunner freezes a live parent scope for synchronous depth-one execution. */
func (s *AgentSession) NewChildRunner(scope *RunScope) (*ChildRunner, error) {
	if scope == nil || scope.owner != s {
		return nil, ErrRunScopeOwner
	}
	if s.harness == nil {
		return nil, errors.New("runtime child runner requires its owning harness")
	}
	return &ChildRunner{
		harness: s.harness, parentSession: s.Snapshot(), parentRun: scope.Snapshot(),
		parentSpec: scope.resolved.Snapshot(), parentTools: scope.AllowedTools(),
		permission: scope.Permission(), permissionRequired: requiresChildPermission(scope.Snapshot().PermissionPolicy),
		parentContext: scope.Context(), parentMaxDepth: scope.profile.MaxDelegationDepth,
	}, nil
}

/* NewChildRunnerFromFrozenParent adapts an already-admitted legacy parent without creating shadow state. */
func (h *RuntimeHarness) NewChildRunnerFromFrozenParent(parent FrozenParentRun) (*ChildRunner, error) {
	if h == nil {
		return nil, errors.New("runtime child runner requires its owning harness")
	}
	profile, err := ResolveProfile(parent.Profile)
	if err != nil {
		return nil, err
	}
	parentContext := parent.Context
	if parentContext == nil {
		parentContext = context.Background()
	}
	snapshot := profile.Snapshot()
	return &ChildRunner{
		harness: h,
		parentSession: AgentSessionSnapshot{
			ID: parent.SessionID, Profile: parent.Profile, Source: snapshot.SessionSource,
			WorkDir: parent.WorkDir,
		},
		parentRun: RunScopeSnapshot{
			Profile: parent.Profile, SessionID: parent.SessionID, RunID: parent.RunID,
			Model: parent.Model, Effort: parent.Effort, PermissionPolicy: snapshot.PermissionPolicy,
			DelegationDepth: parent.DelegationDepth,
		},
		parentSpec: RunSnapshot{
			Profile: parent.Profile, WorkDir: parent.WorkDir,
			ProviderProtocol: parent.ProviderProtocol, Model: parent.Model, Effort: parent.Effort,
			DelegationDepth: parent.DelegationDepth,
		},
		parentTools: cloneToolNames(parent.AllowedTools),
		permission:  parent.Permission, permissionRequired: requiresChildPermission(snapshot.PermissionPolicy),
		parentContext: parentContext, parentMaxDepth: snapshot.MaxDelegationDepth,
	}, nil
}

/* Run creates, executes, cleans, and closes one isolated depth-one child session. */
func (r *ChildRunner) Run(ctx context.Context, request ChildRunRequest) (result ChildRunResult, resultErr error) {
	request = cloneChildRunRequest(request)
	if request.Depth == 0 {
		request.Depth = 1
	}
	if request.DelegationID == "" {
		request.DelegationID = request.InvocationID
	}
	if request.Agent == "" {
		request.Agent = "general-purpose"
	}
	result = ChildRunResult{
		InvocationID: request.InvocationID, ParentSessionID: r.parentSession.ID,
		ParentRunID: r.parentRun.RunID, DelegationID: request.DelegationID,
		Agent: request.Agent, Depth: request.Depth, Status: ChildStartFailed,
	}
	if err := r.validate(request); err != nil {
		result.Status = ChildRejected
		return result, err
	}
	childContext, cancelChild := context.WithCancel(ctx)
	stopParentCancellation := context.AfterFunc(r.parentContext, cancelChild)
	defer func() {
		stopParentCancellation()
		cancelChild()
	}()

	var child *AgentSession
	defer func() {
		if recovered := recover(); recovered != nil {
			recordChildFailure(&result, &resultErr, "panic", fmt.Errorf("runtime child execution panic: %v", recovered))
		}
		if request.Cleanup != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), childCleanupTimeout)
			cleanupErr := runChildTerminalStep("cleanup", func() error {
				return request.Cleanup.Cleanup(cleanupCtx)
			})
			cancel()
			if cleanupErr != nil {
				recordChildFailure(&result, &resultErr, "cleanup", fmt.Errorf("runtime child cleanup: %w", cleanupErr))
			}
		}
		if child != nil {
			recoveryCtx, cancel := context.WithTimeout(context.Background(), childCleanupTimeout)
			recoveryErr := runChildTerminalStep("persistence recovery", func() error {
				return child.RecoverRunFinish(recoveryCtx)
			})
			cancel()
			if recoveryErr != nil {
				recordChildFailure(&result, &resultErr, "persistence", fmt.Errorf("recover runtime child finish: %w", recoveryErr))
			}
			closeCtx, cancel := context.WithTimeout(context.Background(), childCleanupTimeout)
			closeErr := runChildTerminalStep("session close", func() error {
				return child.Close(closeCtx)
			})
			cancel()
			if closeErr != nil {
				recordChildFailure(&result, &resultErr, "cleanup", fmt.Errorf("close runtime child session: %w", closeErr))
			}
		}
	}()

	workDir := r.parentSpec.WorkDir
	if workDir == "" {
		workDir = r.parentSession.WorkDir
	}
	var err error
	child, err = r.harness.CreateSession(childContext, ChildRun, SessionOptions{
		WorkDir: workDir, UserID: "subagent-of-" + string(r.parentSession.ID),
		ParentSessionID: r.parentSession.ID, ParentRunID: r.parentRun.RunID,
		DelegationID: request.DelegationID, Agent: request.Agent,
	})
	if err != nil {
		return result, err
	}
	result.SessionID = child.Snapshot().ID

	depth := 1
	readOnly := request.ReadOnly
	allowed, err := resolveChildAllowedTools(readOnly, intersectChildTools(r.parentTools, request.AgentAllowedTools, request.AllowedTools))
	if err != nil {
		return result, err
	}
	var permissionRequest *ChildPermissionRequest
	if r.permissionRequired || !isNilRuntimeDependency(r.permission) {
		permissionRequest = &ChildPermissionRequest{
			ParentSessionID: r.parentSession.ID, ParentRunID: r.parentRun.RunID,
			DelegationID: request.DelegationID, Agent: request.Agent,
			ReadOnly: readOnly, AllowedTools: cloneToolNames(allowed),
		}
	}
	runResult, err := child.Run(childContext, RunSpec{
		Prompt: renderChildPrompt(r.parentSession.ID, request, allowed, r.parentRun.PermissionPolicy), Task: request.Task,
		ParentSessionID: r.parentSession.ID, WorkDir: workDir,
		ProviderProtocol: r.parentSpec.ProviderProtocol, Model: r.parentRun.Model, Effort: r.parentRun.Effort,
		MaxTurns: request.MaxTurns, ReadOnly: &readOnly, AllowedTools: allowed,
		DelegationDepth: &depth, Observer: request.Observer,
		Permission: r.permission, childPermission: permissionRequest,
	})
	result.Runtime = runResult
	result.RunID = runResult.RunID
	result.Report = runResult.CommittedMessage
	result.Status = classifyChildOutcome(runResult, err)
	return result, err
}

func runChildTerminalStep(operation string, run func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("runtime child %s panic: %v", operation, recovered)
		}
	}()
	return run()
}

func recordChildFailure(result *ChildRunResult, resultErr *error, kind string, err error) {
	*resultErr = errors.Join(*resultErr, err)
	result.Status = ChildFailed
	if result.Runtime.Outcome.ErrorKind == "" {
		result.Runtime.Outcome.ErrorKind = kind
	}
	result.Runtime.Outcome.Err = errors.Join(result.Runtime.Outcome.Err, err)
	if result.Report != "" {
		result.Runtime.Outcome.Partial = true
	}
}

func requiresChildPermission(policy string) bool {
	return policy == "interactive" || policy == "ask_remote_approval"
}

func cloneChildPermissionRequest(request *ChildPermissionRequest) *ChildPermissionRequest {
	if request == nil {
		return nil
	}
	copy := *request
	copy.AllowedTools = cloneToolNames(request.AllowedTools)
	return &copy
}

func resolveRunPermission(ctx context.Context, scope *RunScope, assembly RunAssembly) (PermissionScope, error) {
	permission := scope.Permission()
	request := cloneChildPermissionRequest(scope.resolved.childPermission)
	if request == nil {
		return permission, nil
	}
	if isNilRuntimeDependency(permission) {
		return nil, errors.New("runtime child permission authority is required")
	}
	request.ChildSessionID = assembly.Session.ID
	request.ChildRunID = assembly.Run.RunID
	derived, err := permission.ChildScope(ctx, *request)
	if err != nil {
		return nil, fmt.Errorf("derive child permission scope: %w", err)
	}
	if isNilRuntimeDependency(derived) {
		return nil, errors.New("derive child permission scope returned nil")
	}
	return derived, nil
}

func (r *ChildRunner) validate(request ChildRunRequest) error {
	if r == nil || r.harness == nil {
		return errors.New("runtime child runner is required")
	}
	if r.parentMaxDepth < 1 || r.parentRun.DelegationDepth >= r.parentMaxDepth || request.Depth != 1 {
		return fmt.Errorf("runtime child depth must be exactly one beneath a root run")
	}
	if strings.TrimSpace(request.InvocationID) == "" {
		return errors.New("runtime child invocation ID is required")
	}
	if strings.TrimSpace(request.Task) == "" {
		return errors.New("runtime child task is required")
	}
	return nil
}

func cloneChildRunRequest(request ChildRunRequest) ChildRunRequest {
	request.InvocationID = strings.TrimSpace(request.InvocationID)
	request.DelegationID = strings.TrimSpace(request.DelegationID)
	request.Agent = strings.TrimSpace(request.Agent)
	request.Task = strings.TrimSpace(request.Task)
	request.AllowedTools = cloneToolNames(request.AllowedTools)
	request.AgentAllowedTools = cloneToolNames(request.AgentAllowedTools)
	if request.MaxTurns != nil {
		value := *request.MaxTurns
		request.MaxTurns = &value
	}
	return request
}

func resolveChildAllowedTools(readOnly bool, allowed []string) ([]string, error) {
	profile, err := ResolveProfile(ChildRun)
	if err != nil {
		return nil, err
	}
	resolved, err := profile.Resolve(RunSpec{ReadOnly: &readOnly, AllowedTools: allowed})
	if err != nil {
		return nil, err
	}
	snapshot := resolved.Snapshot()
	if snapshot.AllowedTools == "" {
		return []string{}, nil
	}
	return strings.Split(snapshot.AllowedTools, ","), nil
}

func intersectChildTools(parent, agent, requested []string) []string {
	effective := cloneToolNames(parent)
	if agent != nil {
		effective = intersectToolNames(effective, agent)
	}
	if requested != nil {
		effective = intersectToolNames(effective, requested)
	}
	return effective
}

func intersectToolNames(base, ceiling []string) []string {
	allowed := make(map[string]struct{}, len(ceiling))
	for _, name := range ceiling {
		allowed[name] = struct{}{}
	}
	result := make([]string, 0, len(base))
	for _, name := range base {
		if _, ok := allowed[name]; ok {
			result = append(result, name)
		}
	}
	return result
}

func renderChildPrompt(parent session.ID, request ChildRunRequest, allowedTools []string, permissionPolicy string) string {
	mode := "writable"
	if request.ReadOnly {
		mode = "read-only"
	}
	return prompt.Render([]prompt.Fragment{
		prompt.Text("Agent: " + request.Agent),
		prompt.Section("Child execution", strings.TrimSpace(request.AgentInstructions)),
		prompt.Section("Parent session", string(parent)),
		prompt.Section("Delegated task", request.Task),
		prompt.Section("Execution mode", mode),
		prompt.Section("Effective tools", strings.Join(allowedTools, ", ")),
		prompt.Section("Permission policy", permissionPolicy),
		prompt.Text("Return only a concise, high-density report for the parent runtime."),
	})
}

func classifyChildOutcome(result RunResult, err error) ChildOutcomeStatus {
	switch {
	case err == nil:
		return ChildSucceeded
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ChildCancelled
	case result.Outcome.ErrorKind == "turn_limit":
		return ChildTurnExhausted
	case result.RunID == "":
		return ChildStartFailed
	default:
		return ChildFailed
	}
}
