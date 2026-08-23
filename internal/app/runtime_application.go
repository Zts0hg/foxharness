package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

/* RuntimeSession is the application-owned execution capability required from one live runtime session. */
type RuntimeSession interface {
	Run(context.Context, foxruntime.RunSpec) (foxruntime.RunResult, error)
}

/* RuntimeApplicationConfig binds one selected session to UI-neutral run lifecycle hooks. */
type RuntimeApplicationConfig struct {
	Session   RuntimeSession
	Info      SessionInfo
	RunSpec   foxruntime.RunSpec
	BeforeRun func(context.Context, RunCommand) error
	AfterRun  func(context.Context, foxruntime.RunResult, error)
	Drain     func(context.Context) error
}

/* RuntimeApplication drives one selected runtime session through application contracts. */
type RuntimeApplication struct {
	config RuntimeApplicationConfig
}

/* NewRuntimeApplication validates and freezes a runtime-backed application service. */
func NewRuntimeApplication(config RuntimeApplicationConfig) (*RuntimeApplication, error) {
	if isNilRuntimeSession(config.Session) {
		return nil, errors.New("runtime application session is required")
	}
	config.RunSpec = cloneRunSpec(config.RunSpec)
	return &RuntimeApplication{config: config}, nil
}

/* Session returns the presentation-safe selected session snapshot. */
func (a *RuntimeApplication) Session() SessionInfo {
	return a.config.Info
}

/* Run maps one command to an immutable runtime specification and terminal DTO. */
func (a *RuntimeApplication) Run(ctx context.Context, command RunCommand, sink NotificationSink) (*RunOutcome, error) {
	if a.config.BeforeRun != nil {
		if err := a.config.BeforeRun(ctx, command); err != nil {
			return nil, err
		}
	}
	spec := cloneRunSpec(a.config.RunSpec)
	spec.Prompt = command.Prompt
	spec.DisplayPrompt = command.DisplayPrompt
	if command.AllowedTools != nil {
		spec.AllowedTools = cloneAllowedTools(command.AllowedTools)
	}
	if command.CollaborationMode != "" {
		spec.CollaborationMode = command.CollaborationMode
	}
	if command.Model != "" {
		spec.Model = command.Model
	}
	if command.Effort != "" {
		spec.Effort = command.Effort
	}
	spec.Observer = NewRuntimeNotificationObserver(sink)

	result, runErr := a.config.Session.Run(ctx, spec)
	if a.config.AfterRun != nil {
		a.config.AfterRun(ctx, result, runErr)
	}
	if result.RunID == "" {
		return nil, runErr
	}
	outcome := MapRuntimeRunResult(result)
	runRoot := filepath.Join(a.config.Info.Directory, "runs", outcome.RunID)
	outcome.MetricsPath = filepath.Join(runRoot, "metrics.jsonl")
	outcome.TracePath = filepath.Join(runRoot, "trace.jsonl")
	return &outcome, runErr
}

/* Drain waits for application-owned asynchronous work launched by prior runs. */
func (a *RuntimeApplication) Drain(ctx context.Context) error {
	if a.config.Drain == nil {
		return nil
	}
	return a.config.Drain(ctx)
}

func cloneRunSpec(spec foxruntime.RunSpec) foxruntime.RunSpec {
	spec.AllowedTools = cloneAllowedTools(spec.AllowedTools)
	if spec.MaxTurns != nil {
		value := *spec.MaxTurns
		spec.MaxTurns = &value
	}
	if spec.TaskTimeout != nil {
		value := *spec.TaskTimeout
		spec.TaskTimeout = &value
	}
	if spec.Thinking != nil {
		value := *spec.Thinking
		spec.Thinking = &value
	}
	if spec.ReadOnly != nil {
		value := *spec.ReadOnly
		spec.ReadOnly = &value
	}
	if spec.DelegationDepth != nil {
		value := *spec.DelegationDepth
		spec.DelegationDepth = &value
	}
	return spec
}

func cloneAllowedTools(tools []string) []string {
	if tools == nil {
		return nil
	}
	return append([]string{}, tools...)
}

func isNilRuntimeSession(value RuntimeSession) bool {
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
