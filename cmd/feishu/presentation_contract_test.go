package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/feishu"
)

func TestUIFEI001RequiredEnvironmentOrderingAndExit(t *testing.T) {
	keys := []string{"FEISHU_APP_ID", "FEISHU_APP_SECRET", "FEISHU_VERIFICATION_TOKEN", "FEISHU_ENCRYPT_KEY"}
	for missingIndex, wantKey := range keys {
		t.Run(wantKey, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestUIFEI001ProcessHelper$")
			env := append(os.Environ(), "FOX_UI_FEISHU_ENV_HELPER=1")
			for i, key := range keys {
				value := ""
				if i < missingIndex {
					value = "configured"
				}
				env = append(env, key+"="+value)
			}
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("process error = %v, want exit 1", err)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "missing environment variable: "+wantKey) {
				t.Fatalf("stdout/stderr = %q/%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestUIFEI001ProcessHelper(t *testing.T) {
	if os.Getenv("FOX_UI_FEISHU_ENV_HELPER") != "1" {
		return
	}
	main()
}

func TestUIFEI001CompositionConstantsAndListenFailure(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"make(chan feishu.Task, 32)",
		`serve(signalCtx, gateway, runner, tasks, ":7777", defaultShutdownTimeout)`,
		"os.Getwd()",
		"newConfiguredLLMProvider(homeDir, os.Getenv)",
	} {
		if !strings.Contains(string(source), required) {
			t.Fatalf("composition root missing %q", required)
		}
	}

	wantErr := errors.New("listen failed")
	gateway := &immediateGateway{listenErr: wantErr}
	runner := &drainingRunner{}
	tasks := make(chan feishu.Task)
	if err := serve(context.Background(), gateway, runner, tasks, ":0", time.Second); !errors.Is(err, wantErr) {
		t.Fatalf("serve() error = %v, want listener failure", err)
	}
	if !runner.returned {
		t.Fatal("runner did not drain closed task channel after listener failure")
	}
}

type immediateGateway struct{ listenErr error }

func (g *immediateGateway) Listen(string) error          { return g.listenErr }
func (*immediateGateway) Shutdown(context.Context) error { return nil }

type drainingRunner struct{ returned bool }

func (r *drainingRunner) Start(_ context.Context, tasks <-chan feishu.Task) {
	for range tasks {
	}
	r.returned = true
}
