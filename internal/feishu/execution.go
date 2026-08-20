package feishu

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/app"
)

/* TextMessenger is the Feishu-owned outbound text capability. */
type TextMessenger interface {
	SendText(context.Context, string, string) error
}

/* TaskExecutionRequest freezes one accepted task's application input and session directive. */
type TaskExecutionRequest struct {
	Task            Task
	Prompt          string
	ForceNewSession bool
}

/* PreparedTaskExecution binds one selected session to its application use case and cleanup. */
type PreparedTaskExecution struct {
	Application app.RunUseCase
	Session     app.SessionInfo
	Created     bool
	SetupError  error
	Drain       func(context.Context) error
}

/* TaskExecutionFactory prepares one runtime-backed application execution for an accepted task. */
type TaskExecutionFactory interface {
	PrepareTask(context.Context, TaskExecutionRequest) (PreparedTaskExecution, error)
}
