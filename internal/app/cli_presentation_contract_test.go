package app

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
)

func TestUICLI003SuccessfulOutputIsExactAndPrecedesDrain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &cliPresentationRunner{
		sessionID:      "session-123",
		sessionDir:     "/sessions/session-123",
		transcriptPath: "/sessions/session-123/transcript.md",
		result: &engine.RunResult{
			SessionID:    "session-123",
			RunID:        "run-456",
			FinalMessage: "completed answer",
			MetricsPath:  "/sessions/session-123/runs/run-456/metrics.jsonl",
			TracePath:    "/sessions/session-123/runs/run-456/trace.jsonl",
		},
	}
	runner.onDrain = func() {
		want := "completed answer\n\n" +
			"Session:  session-123\n" +
			"Transcript:  /sessions/session-123/transcript.md\n" +
			"Run:  run-456\n" +
			"Metrics:  /sessions/session-123/runs/run-456/metrics.jsonl\n" +
			"Trace:  /sessions/session-123/runs/run-456/trace.jsonl\n"
		if got := stdout.String(); got != want {
			t.Errorf("stdout before drain = %q, want %q", got, want)
		}
	}

	err := runCLIWithFactory(context.Background(), CLIConfig{Prompt: "task"}, &stdout, log.New(&stderr, "", 0), func(context.Context, AgentRunnerConfig) (cliPresentationRunnerAPI, error) {
		return runner, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wantStdout := "completed answer\n\n" +
		"Session:  session-123\n" +
		"Transcript:  /sessions/session-123/transcript.md\n" +
		"Run:  run-456\n" +
		"Metrics:  /sessions/session-123/runs/run-456/metrics.jsonl\n" +
		"Trace:  /sessions/session-123/runs/run-456/trace.jsonl\n"
	if got := stdout.String(); got != wantStdout {
		t.Fatalf("stdout = %q, want %q", got, wantStdout)
	}
	if got, want := stderr.String(), "[CLI] Session: session-123\n[CLI] Session dir: /sessions/session-123\n"; got != want {
		t.Fatalf("stderr lifecycle log = %q, want %q", got, want)
	}
	if runner.runCalls != 1 || runner.drainCalls != 1 {
		t.Fatalf("run/drain calls = %d/%d, want 1/1", runner.runCalls, runner.drainCalls)
	}
	if strings.Contains(stdout.String(), "delta") {
		t.Fatalf("stdout leaked model deltas: %q", stdout.String())
	}
}

func TestUICLI003BlankFinalKeepsExactMetadataLayout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &cliPresentationRunner{
		sessionID: "s", sessionDir: "/sessions/s", transcriptPath: "/sessions/s/transcript.md",
		result: &engine.RunResult{SessionID: "s", RunID: "r", MetricsPath: "/metrics", TracePath: "/trace"},
	}
	if err := runCLIWithFactory(context.Background(), CLIConfig{}, &stdout, log.New(&stderr, "", 0), fixedCLIRunnerFactory(runner)); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "\nSession:  s\nTranscript:  /sessions/s/transcript.md\nRun:  r\nMetrics:  /metrics\nTrace:  /trace\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestUICLI004InitializationNilPartialAndRuntimeFailuresAreExact(t *testing.T) {
	t.Run("initialization", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		wantErr := errors.New("initialize failed")
		err := runCLIWithFactory(context.Background(), CLIConfig{}, &stdout, log.New(&stderr, "", 0), func(context.Context, AgentRunnerConfig) (cliPresentationRunnerAPI, error) {
			return nil, wantErr
		})
		if !errors.Is(err, wantErr) || stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("initialization = err %v stdout %q stderr %q", err, stdout.String(), stderr.String())
		}
	})

	for _, tc := range []struct {
		name       string
		result     *engine.RunResult
		wantStdout string
	}{
		{name: "nil outcome", wantStdout: "\nSession:  s\nTranscript:  /transcript\n"},
		{
			name:       "partial outcome",
			result:     &engine.RunResult{RunID: "partial-run", FinalMessage: "partial answer", MetricsPath: "/partial-metrics", TracePath: "/partial-trace"},
			wantStdout: "partial answer\n\nSession:  s\nTranscript:  /transcript\nRun:  partial-run\nMetrics:  /partial-metrics\nTrace:  /partial-trace\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			wantErr := errors.New("runtime failed")
			runner := &cliPresentationRunner{sessionID: "s", sessionDir: "/session", transcriptPath: "/transcript", result: tc.result, runErr: wantErr}
			err := runCLIWithFactory(context.Background(), CLIConfig{}, &stdout, log.New(&stderr, "", 0), fixedCLIRunnerFactory(runner))
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want runtime failure", err)
			}
			if got := stdout.String(); got != tc.wantStdout {
				t.Fatalf("stdout = %q, want %q", got, tc.wantStdout)
			}
			if got, want := stderr.String(), "[CLI] Session: s\n[CLI] Session dir: /session\n[CLI] 任务失败: runtime failed\n"; got != want {
				t.Fatalf("stderr = %q, want %q", got, want)
			}
			if runner.drainCalls != 1 {
				t.Fatalf("drain calls = %d, want one even after runtime failure", runner.drainCalls)
			}
		})
	}
}

func TestUICLI004ExtractionFailureDoesNotRewriteSuccessfulOutcome(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &cliPresentationRunner{
		sessionID: "s", sessionDir: "/session", transcriptPath: "/transcript",
		result:   &engine.RunResult{RunID: "r", FinalMessage: "success", MetricsPath: "/metrics", TracePath: "/trace"},
		drainErr: errors.New("extraction failed"),
	}
	if err := runCLIWithFactory(context.Background(), CLIConfig{}, &stdout, log.New(&stderr, "", 0), fixedCLIRunnerFactory(runner)); err != nil {
		t.Fatalf("extraction failure changed successful outcome: %v", err)
	}
	if got, want := stdout.String(), "success\n\nSession:  s\nTranscript:  /transcript\nRun:  r\nMetrics:  /metrics\nTrace:  /trace\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if strings.Contains(stderr.String(), "extraction failed") {
		t.Fatalf("current adapter surfaced extraction failure: %q", stderr.String())
	}
}

type cliPresentationRunner struct {
	sessionID      string
	sessionDir     string
	transcriptPath string
	result         *engine.RunResult
	runErr         error
	drainErr       error
	runCalls       int
	drainCalls     int
	onDrain        func()
}

func (r *cliPresentationRunner) SessionID() string      { return r.sessionID }
func (r *cliPresentationRunner) SessionDir() string     { return r.sessionDir }
func (r *cliPresentationRunner) TranscriptPath() string { return r.transcriptPath }
func (r *cliPresentationRunner) Run(context.Context, string, engine.Reporter) (*engine.RunResult, error) {
	r.runCalls++
	return r.result, r.runErr
}
func (r *cliPresentationRunner) WaitForExtraction() {
	r.drainCalls++
	if r.onDrain != nil {
		r.onDrain()
	}
	_ = r.drainErr
}

func fixedCLIRunnerFactory(runner cliPresentationRunnerAPI) cliPresentationRunnerFactory {
	return func(context.Context, AgentRunnerConfig) (cliPresentationRunnerAPI, error) { return runner, nil }
}
