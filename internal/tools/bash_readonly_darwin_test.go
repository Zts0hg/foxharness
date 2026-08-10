//go:build darwin

package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDarwinReadOnlyBashCommandUsesDenyByDefaultSandbox(t *testing.T) {
	request := readOnlyBashRequest{
		WorkDir:       t.TempDir(),
		ReadableRoots: []string{t.TempDir()},
		Command:       "pwd",
		Timeout:       time.Second,
	}
	cmd, err := newDarwinReadOnlyBashCommand(context.Background(), "/usr/bin/sandbox-exec", request)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Dir != request.WorkDir {
		t.Fatalf("sandbox command dir = %q, want %q", cmd.Dir, request.WorkDir)
	}
	if strings.Join(cmd.Env, "\x00") != strings.Join(readOnlyBashEnvironment(), "\x00") {
		t.Fatalf("sandbox environment = %v", cmd.Env)
	}
	joinedArgs := strings.Join(cmd.Args, " ")
	for _, want := range []string{"sandbox-exec", "-p", "/bin/bash", "--noprofile", "--norc", "-c", "pwd"} {
		if !strings.Contains(joinedArgs, want) {
			t.Fatalf("sandbox args missing %q: %v", want, cmd.Args)
		}
	}
	profile := cmd.Args[2]
	for _, want := range []string{"(deny default)", "(deny network*)", "(deny file-write*)", request.WorkDir, request.ReadableRoots[0]} {
		if !strings.Contains(profile, want) {
			t.Fatalf("sandbox profile missing %q:\n%s", want, profile)
		}
	}
	if strings.Contains(profile, "(allow default)") {
		t.Fatalf("sandbox profile permits default access:\n%s", profile)
	}
}

func TestDarwinReadOnlyBashStartFailureIsUnavailable(t *testing.T) {
	runner := darwinReadOnlyBashRunner{sandboxPath: "/definitely/missing/sandbox-exec"}
	result := runner.Run(context.Background(), readOnlyBashRequest{
		WorkDir:       t.TempDir(),
		ReadableRoots: []string{t.TempDir()},
		Command:       "pwd",
		Timeout:       time.Second,
	})
	if !errors.Is(result.Err, ErrReadOnlyBashSandboxUnavailable) {
		t.Fatalf("sandbox start error = %v, want unavailable", result.Err)
	}
}
