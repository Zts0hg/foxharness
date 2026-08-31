package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
)

func TestRunPrintsSuccessfulOutcomeBeforeDrain(t *testing.T) {
	order := []string{}
	application := &recordingApplication{
		session: app.SessionInfo{
			ID: "session-123", Directory: "/sessions/session-123",
			TranscriptPath: "/sessions/session-123/transcript.jsonl",
		},
		outcome: &app.RunOutcome{
			SessionID: "session-123", RunID: "run-456", FinalMessage: "completed answer",
			MetricsPath: "/sessions/session-123/runs/run-456/metrics.jsonl",
			TracePath:   "/sessions/session-123/runs/run-456/trace.jsonl",
		},
		order: &order,
	}
	stdout := &orderWriter{order: &order}
	logs := new(bytes.Buffer)

	err := Run(context.Background(), Config{
		Prompt: "do the work", Application: application, Stdout: stdout,
		Logger: log.New(logs, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantOutput := "completed answer\n\nSession:  session-123\nTranscript:  /sessions/session-123/transcript.jsonl\nRun:  run-456\nMetrics:  /sessions/session-123/runs/run-456/metrics.jsonl\nTrace:  /sessions/session-123/runs/run-456/trace.jsonl\n"
	if got := stdout.String(); got != wantOutput {
		t.Fatalf("stdout = %q, want %q", got, wantOutput)
	}
	if got, want := logs.String(), "[CLI] Session: session-123\n[CLI] Session dir: /sessions/session-123\n"; got != want {
		t.Fatalf("logs = %q, want %q", got, want)
	}
	if want := []string{"run", "write", "drain"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
	if application.command.Prompt != "do the work" || application.notifications != nil {
		t.Fatalf("application invocation = %#v/%T", application.command, application.notifications)
	}
}

func TestRunPreservesBlankAndPartialOutcomePresentation(t *testing.T) {
	checks := []struct {
		name    string
		outcome *app.RunOutcome
		runErr  error
		want    string
	}{
		{
			name: "blank final", outcome: &app.RunOutcome{RunID: "r", MetricsPath: "/metrics", TracePath: "/trace"},
			want: "\nSession:  s\nTranscript:  /transcript\nRun:  r\nMetrics:  /metrics\nTrace:  /trace\n",
		},
		{
			name: "nil result failure", runErr: errors.New("failed before result"),
			want: "\nSession:  s\nTranscript:  /transcript\n",
		},
		{
			name:    "partial result failure",
			outcome: &app.RunOutcome{RunID: "partial-run", FinalMessage: "partial answer", MetricsPath: "/partial-metrics", TracePath: "/partial-trace"},
			runErr:  errors.New("failed after result"),
			want:    "partial answer\n\nSession:  s\nTranscript:  /transcript\nRun:  partial-run\nMetrics:  /partial-metrics\nTrace:  /partial-trace\n",
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			application := &recordingApplication{
				session: app.SessionInfo{ID: "s", Directory: "/session", TranscriptPath: "/transcript"},
				outcome: check.outcome, runErr: check.runErr,
			}
			stdout := new(bytes.Buffer)
			logs := new(bytes.Buffer)
			err := Run(context.Background(), Config{Prompt: "task", Application: application, Stdout: stdout, Logger: log.New(logs, "", 0)})
			if !errors.Is(err, check.runErr) {
				t.Fatalf("Run() error = %v, want %v", err, check.runErr)
			}
			if got := stdout.String(); got != check.want {
				t.Fatalf("stdout = %q, want %q", got, check.want)
			}
			if check.runErr != nil && logs.String() != "[CLI] Session: s\n[CLI] Session dir: /session\n[CLI] 任务失败: "+check.runErr.Error()+"\n" {
				t.Fatalf("logs = %q", logs.String())
			}
			if application.drainCalls != 1 {
				t.Fatalf("drain calls = %d, want 1", application.drainCalls)
			}
		})
	}
}

func TestRunReturnsInitializationFailureWithoutPresentationOrDrain(t *testing.T) {
	wantErr := errors.New("initialize failed")
	stdout := new(bytes.Buffer)
	logs := new(bytes.Buffer)
	err := Run(context.Background(), Config{
		Prompt: "task", Initialize: func(context.Context) (Application, error) { return nil, wantErr },
		Stdout: stdout, Logger: log.New(logs, "", 0),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
	if stdout.Len() != 0 || logs.Len() != 0 {
		t.Fatalf("unexpected presentation stdout=%q logs=%q", stdout.String(), logs.String())
	}
}

func TestRunRejectsMissingOrNilApplicationComposition(t *testing.T) {
	checks := []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing initializer", config: Config{}, want: "CLI application is required"},
		{
			name:   "nil initialized application",
			config: Config{Initialize: func(context.Context) (Application, error) { return nil, nil }},
			want:   "CLI application initializer returned nil",
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := Run(context.Background(), check.config)
			if err == nil || err.Error() != check.want {
				t.Fatalf("Run() error = %v, want %q", err, check.want)
			}
		})
	}
}

func TestRunIgnoresDrainFailure(t *testing.T) {
	runErr := errors.New("run failed")
	application := &recordingApplication{
		session: app.SessionInfo{ID: "s", Directory: "/session", TranscriptPath: "/transcript"},
		runErr:  runErr, drainErr: errors.New("drain failed"),
	}
	err := Run(context.Background(), Config{Prompt: "task", Application: application, Stdout: new(bytes.Buffer), Logger: log.New(new(bytes.Buffer), "", 0)})
	if !errors.Is(err, runErr) {
		t.Fatalf("Run() error = %v, want run error %v", err, runErr)
	}
}

type recordingApplication struct {
	session       app.SessionInfo
	outcome       *app.RunOutcome
	runErr        error
	drainErr      error
	command       app.RunCommand
	notifications app.NotificationSink
	drainCalls    int
	order         *[]string
}

func (a *recordingApplication) Session() app.SessionInfo { return a.session }

func (a *recordingApplication) Run(_ context.Context, command app.RunCommand, notifications app.NotificationSink) (*app.RunOutcome, error) {
	a.command = command
	a.notifications = notifications
	if a.order != nil {
		*a.order = append(*a.order, "run")
	}
	return a.outcome, a.runErr
}

func (a *recordingApplication) Drain(context.Context) error {
	a.drainCalls++
	if a.order != nil {
		*a.order = append(*a.order, "drain")
	}
	return a.drainErr
}

type orderWriter struct {
	bytes.Buffer
	order *[]string
}

func (w *orderWriter) Write(p []byte) (int, error) {
	if len(*w.order) == 0 || (*w.order)[len(*w.order)-1] != "write" {
		*w.order = append(*w.order, "write")
	}
	return w.Buffer.Write(p)
}

func (w *orderWriter) String() string { return fmt.Sprint(w.Buffer.String()) }
