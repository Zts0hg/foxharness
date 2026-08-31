package tui

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
)

func TestRunOwnsInteractionInitializationProgramAndCleanup(t *testing.T) {
	logDir := t.TempDir()
	application := newFakeRunner()
	var initialized bool
	var started bool
	var closed bool
	previousWriter := log.Writer()

	err := Run(context.Background(), Config{
		Initialize: func(_ context.Context, interactions Interactions) (Startup, error) {
			initialized = true
			if interactions.Permissions == nil || interactions.Questions == nil || interactions.PlanReview == nil || interactions.InteractionNotices == nil {
				t.Fatalf("incomplete interaction capabilities = %#v", interactions)
			}
			return Startup{
				Application:   application,
				SessionLogDir: logDir,
				Close:         func(context.Context) error { closed = true; return nil },
			}, nil
		},
		runProgram: func(model Model) error {
			started = true
			if model.runner != application || model.asker == nil || model.planReviewer == nil || model.permissionBridge == nil {
				t.Fatalf("program model is not bound to initialized application and interactions")
			}
			if log.Writer() == previousWriter {
				t.Fatal("TUI log writer was not redirected before program start")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !initialized || !started || !closed {
		t.Fatalf("lifecycle initialized/started/closed = %v/%v/%v", initialized, started, closed)
	}
	if log.Writer() != previousWriter {
		t.Fatal("TUI log writer was not restored")
	}
	if _, err := os.Stat(filepath.Join(logDir, "tui.log")); err != nil {
		t.Fatalf("TUI log file: %v", err)
	}
}

func TestRunInitializationFailureDoesNotStartProgram(t *testing.T) {
	want := errors.New("initialize")
	started := false
	err := Run(context.Background(), Config{
		Initialize: func(context.Context, Interactions) (Startup, error) { return Startup{}, want },
		runProgram: func(Model) error { started = true; return nil },
	})
	if !errors.Is(err, want) || started {
		t.Fatalf("Run error/started = %v/%v", err, started)
	}
}

func TestRunRejectsTypedNilInitializedApplicationAndStillCleansUp(t *testing.T) {
	var application *fakeRunner
	closed := false
	err := Run(context.Background(), Config{
		Initialize: func(context.Context, Interactions) (Startup, error) {
			return Startup{Application: application, Close: func(context.Context) error { closed = true; return nil }}, nil
		},
		runProgram: func(Model) error { t.Fatal("program started"); return nil },
	})
	if err == nil || !closed {
		t.Fatalf("Run error/closed = %v/%v", err, closed)
	}
}

var _ app.InteractiveApplication = (*fakeRunner)(nil)
