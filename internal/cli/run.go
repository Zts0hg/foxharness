package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/Zts0hg/foxharness/internal/app"
)

/* Application is the one-shot application capability consumed by the CLI adapter. */
type Application interface {
	app.RunUseCase
	Session() app.SessionInfo
	Drain(context.Context) error
}

/* Logger is the narrow logging capability used by the CLI adapter. */
type Logger interface {
	Printf(string, ...any)
}

/* Config supplies already-composed application and presentation dependencies. */
type Config struct {
	Prompt      string
	Application Application
	Initialize  func(context.Context) (Application, error)
	Stdout      io.Writer
	Logger      Logger
}

/* Run executes and presents exactly one application command. */
func Run(ctx context.Context, config Config) error {
	application := config.Application
	if isNilApplication(application) {
		if config.Initialize == nil {
			return errors.New("CLI application is required")
		}
		var err error
		application, err = config.Initialize(ctx)
		if err != nil {
			return err
		}
		if isNilApplication(application) {
			return errors.New("CLI application initializer returned nil")
		}
	}

	session := application.Session()
	config.Logger.Printf("[CLI] Session: %s", session.ID)
	config.Logger.Printf("[CLI] Session dir: %s", session.Directory)

	result, runErr := application.Run(ctx, app.RunCommand{Prompt: config.Prompt}, nil)
	if runErr != nil {
		config.Logger.Printf("[CLI] 任务失败: %v", runErr)
	}
	if result != nil && result.FinalMessage != "" {
		fmt.Fprintln(config.Stdout, result.FinalMessage)
	}

	fmt.Fprintln(config.Stdout)
	fmt.Fprintln(config.Stdout, "Session: ", session.ID)
	fmt.Fprintln(config.Stdout, "Transcript: ", session.TranscriptPath)
	if result != nil {
		fmt.Fprintln(config.Stdout, "Run: ", result.RunID)
		fmt.Fprintln(config.Stdout, "Metrics: ", result.MetricsPath)
		fmt.Fprintln(config.Stdout, "Trace: ", result.TracePath)
	}

	_ = application.Drain(context.Background())
	return runErr
}

func isNilApplication(value Application) bool {
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
