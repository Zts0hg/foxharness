package agentops

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/app"
)

/* TaskApplication is one prepared AgentOps run plus its bounded cleanup capability. */
type TaskApplication interface {
	app.RunUseCase
	Drain(context.Context) error
}

/* TaskExecutionRequest freezes one accepted incident task and its exact model prompt. */
type TaskExecutionRequest struct {
	Task   Task
	Prompt string
}

/* PreparedTaskExecution exposes session identity before runtime initialization begins. */
type PreparedTaskExecution struct {
	Session     app.SessionInfo
	TracePath   string
	MetricsPath string
	Start       func(context.Context) (TaskApplication, error)
}

/* TaskExecutionFactory creates one fresh task session without starting its runtime. */
type TaskExecutionFactory interface {
	PrepareTask(context.Context, TaskExecutionRequest) (PreparedTaskExecution, error)
}
