package app

import (
	"context"
	"errors"
	"reflect"
	"sync"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

/* InteractiveRuntimeBinding groups the mechanism capabilities owned by one live runtime session. */
type InteractiveRuntimeBinding struct {
	Session             RuntimeSession
	State               func() InteractiveSessionState
	Conversation        func(context.Context) ([]ConversationRecord, error)
	ProjectInputHistory func(context.Context, int) ([]string, error)
	RewindTargets       func(context.Context) ([]RewindTarget, error)
	Compact             func(context.Context, CompactCommand) (CompactOutcome, error)
	Rewind              func(context.Context, RewindCommand) RewindOutcome
	RestoreLatestInput  func(context.Context) (RestoreInputOutcome, error)
	BeforeRun           func(context.Context, RunCommand) error
	AfterRun            func(context.Context, foxruntime.RunResult, error)
	Close               func(context.Context) error
}

/* InteractiveRuntimeApplicationConfig defines process-scoped selections and session lifecycle capabilities. */
type InteractiveRuntimeApplicationConfig struct {
	Initial                InteractiveRuntimeBinding
	NewSession             func(context.Context) (InteractiveRuntimeBinding, error)
	RunSpec                foxruntime.RunSpec
	Model                  string
	Effort                 string
	CollaborationMode      string
	NormalizeCollaboration func(string) string
	OnModelChange          func(string) error
	OnEffortChange         func(string)
	Permissions            InteractivePermissionController
}

/* InteractiveRuntimeApplication owns the UI-neutral workflow for one interactive runtime session. */
type InteractiveRuntimeApplication struct {
	runMu sync.Mutex
	mu    sync.RWMutex

	binding                InteractiveRuntimeBinding
	newSession             func(context.Context) (InteractiveRuntimeBinding, error)
	runSpec                foxruntime.RunSpec
	model                  string
	effort                 string
	collaborationMode      string
	defaultCollaboration   string
	normalizeCollaboration func(string) string
	onModelChange          func(string) error
	onEffortChange         func(string)
	permissions            InteractivePermissionController
}

/* NewInteractiveRuntimeApplication validates and freezes the target interactive application boundary. */
func NewInteractiveRuntimeApplication(config InteractiveRuntimeApplicationConfig) (*InteractiveRuntimeApplication, error) {
	if isNilRuntimeSession(config.Initial.Session) {
		return nil, errors.New("interactive runtime session is required")
	}
	if config.Initial.State == nil {
		return nil, errors.New("interactive runtime state capability is required")
	}
	if isNilInteractivePermissionController(config.Permissions) {
		return nil, errors.New("interactive permission controller is required")
	}
	normalize := config.NormalizeCollaboration
	if normalize == nil {
		normalize = func(value string) string { return value }
	}
	defaultMode := normalize(config.CollaborationMode)
	return &InteractiveRuntimeApplication{
		binding: config.Initial, newSession: config.NewSession, runSpec: cloneRunSpec(config.RunSpec),
		model: config.Model, effort: config.Effort, collaborationMode: defaultMode,
		defaultCollaboration: defaultMode, normalizeCollaboration: normalize,
		onModelChange: config.OnModelChange, onEffortChange: config.OnEffortChange, permissions: config.Permissions,
	}, nil
}

/* Run snapshots process selections and executes one command against the current runtime session. */
func (a *InteractiveRuntimeApplication) Run(ctx context.Context, command RunCommand, sink NotificationSink) (*RunOutcome, error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()

	a.mu.RLock()
	binding := a.binding
	base := cloneRunSpec(a.runSpec)
	model := a.model
	effort := a.effort
	mode := a.collaborationMode
	normalize := a.normalizeCollaboration
	a.mu.RUnlock()

	state := binding.State()
	base.SessionID = state.Session.ID
	base.Model = model
	base.Effort = effort
	base.CollaborationMode = mode
	if command.CollaborationMode != "" {
		command.CollaborationMode = normalize(command.CollaborationMode)
	}
	application, err := NewRuntimeApplication(RuntimeApplicationConfig{
		Session: binding.Session, Info: state.Session, RunSpec: base,
		BeforeRun: binding.BeforeRun, AfterRun: binding.AfterRun,
	})
	if err != nil {
		return nil, err
	}
	return application.Run(ctx, command, sink)
}

/* State returns the current presentation-safe session and process selection snapshot. */
func (a *InteractiveRuntimeApplication) State() InteractiveSessionState {
	a.mu.RLock()
	binding := a.binding
	model := a.model
	effort := a.effort
	mode := a.collaborationMode
	a.mu.RUnlock()
	state := binding.State()
	state.Model = model
	state.Effort = effort
	state.CollaborationMode = mode
	return state
}

/* Conversation loads the visible persisted conversation for the current session. */
func (a *InteractiveRuntimeApplication) Conversation(ctx context.Context) ([]ConversationRecord, error) {
	binding := a.currentBinding()
	if binding.Conversation == nil {
		return nil, errors.New("interactive conversation capability is unavailable")
	}
	return binding.Conversation(ctx)
}

/* ProjectInputHistory loads recent project prompts for the current session. */
func (a *InteractiveRuntimeApplication) ProjectInputHistory(ctx context.Context, limit int) ([]string, error) {
	binding := a.currentBinding()
	if binding.ProjectInputHistory == nil {
		return nil, errors.New("interactive input history capability is unavailable")
	}
	return binding.ProjectInputHistory(ctx, limit)
}

/* RewindTargets returns the current session's selectable restore points. */
func (a *InteractiveRuntimeApplication) RewindTargets(ctx context.Context) ([]RewindTarget, error) {
	binding := a.currentBinding()
	if binding.RewindTargets == nil {
		return nil, errors.New("interactive rewind capability is unavailable")
	}
	return binding.RewindTargets(ctx)
}

/* NewSession creates a replacement before closing the previous live session. */
func (a *InteractiveRuntimeApplication) NewSession(ctx context.Context, _ NewSessionCommand) (InteractiveSessionState, error) {
	a.runMu.Lock()
	defer a.runMu.Unlock()
	if a.newSession == nil {
		return InteractiveSessionState{}, errors.New("interactive new-session capability is unavailable")
	}
	next, err := a.newSession(ctx)
	if err != nil {
		return InteractiveSessionState{}, err
	}
	if isNilRuntimeSession(next.Session) || next.State == nil {
		if next.Close != nil {
			_ = next.Close(ctx)
		}
		return InteractiveSessionState{}, errors.New("interactive new-session factory returned an invalid binding")
	}

	a.mu.Lock()
	previous := a.binding
	a.binding = next
	a.collaborationMode = a.defaultCollaboration
	a.mu.Unlock()
	if previous.Close != nil {
		if err := previous.Close(ctx); err != nil {
			return a.State(), err
		}
	}
	a.permissions.ClearPermissionGrants(ctx)
	return a.State(), nil
}

/* UpdateModel changes the model used by future runs after composition validation succeeds. */
func (a *InteractiveRuntimeApplication) UpdateModel(_ context.Context, command ModelCommand) (InteractiveSessionState, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.onModelChange != nil {
		if err := a.onModelChange(command.Model); err != nil {
			return InteractiveSessionState{}, err
		}
	}
	a.model = command.Model
	return a.stateLocked(), nil
}

/* UpdateEffort changes the effort snapshot used by future runs. */
func (a *InteractiveRuntimeApplication) UpdateEffort(_ context.Context, command EffortCommand) InteractiveSessionState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.onEffortChange != nil {
		a.onEffortChange(command.Effort)
	}
	a.effort = command.Effort
	return a.stateLocked()
}

/* UpdateCollaborationMode changes the normalized collaboration selection used by future runs. */
func (a *InteractiveRuntimeApplication) UpdateCollaborationMode(_ context.Context, command CollaborationCommand) InteractiveSessionState {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.collaborationMode = a.normalizeCollaboration(command.Mode)
	return a.stateLocked()
}

/* Compact delegates manual compaction to the current session capability. */
func (a *InteractiveRuntimeApplication) Compact(ctx context.Context, command CompactCommand) (CompactOutcome, error) {
	binding := a.currentBinding()
	if binding.Compact == nil {
		return CompactOutcome{}, errors.New("interactive compaction capability is unavailable")
	}
	return binding.Compact(ctx, command)
}

/* Rewind delegates ordered state restoration to the current session capability. */
func (a *InteractiveRuntimeApplication) Rewind(ctx context.Context, command RewindCommand) RewindOutcome {
	binding := a.currentBinding()
	if binding.Rewind == nil {
		return RewindOutcome{Error: "interactive rewind capability is unavailable"}
	}
	return binding.Rewind(ctx, command)
}

/* RestoreLatestInput delegates cancellation recovery to the current session capability. */
func (a *InteractiveRuntimeApplication) RestoreLatestInput(ctx context.Context) (RestoreInputOutcome, error) {
	binding := a.currentBinding()
	if binding.RestoreLatestInput == nil {
		return RestoreInputOutcome{}, errors.New("interactive input restore capability is unavailable")
	}
	return binding.RestoreLatestInput(ctx)
}

/* PermissionState returns the current application-owned permission snapshot. */
func (a *InteractiveRuntimeApplication) PermissionState() PermissionState {
	return a.permissions.PermissionState()
}

/* UpdatePermissionMode applies one future-run permission selection. */
func (a *InteractiveRuntimeApplication) UpdatePermissionMode(ctx context.Context, command PermissionModeCommand) PermissionState {
	return a.permissions.UpdatePermissionMode(ctx, command)
}

/* ActivateFullAccess confirms unrestricted operation through the application permission capability. */
func (a *InteractiveRuntimeApplication) ActivateFullAccess(ctx context.Context, command FullAccessCommand) PermissionState {
	return a.permissions.ActivateFullAccess(ctx, command)
}

/* ClearPermissionGrants removes current-session permission grants. */
func (a *InteractiveRuntimeApplication) ClearPermissionGrants(ctx context.Context) PermissionGrantClearOutcome {
	return a.permissions.ClearPermissionGrants(ctx)
}

func (a *InteractiveRuntimeApplication) currentBinding() InteractiveRuntimeBinding {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.binding
}

func (a *InteractiveRuntimeApplication) stateLocked() InteractiveSessionState {
	state := a.binding.State()
	state.Model = a.model
	state.Effort = a.effort
	state.CollaborationMode = a.collaborationMode
	return state
}

func isNilInteractivePermissionController(value InteractivePermissionController) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
