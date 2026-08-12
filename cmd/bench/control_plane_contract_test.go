package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEVBEN001UnexpectedPositionalInputIsRejectedBeforeExecution(t *testing.T) {
	root := t.TempDir()
	fixture := filepath.Join(root, "fixture")
	if err := os.WriteFile(fixture, []byte("not a fixture directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	casePath := filepath.Join(root, "case.yaml")
	contents := "id: positional\nfixture: fixture\nprompt: run\nvalidations:\n  - type: command\n    command: true\n"
	if err := os.WriteFile(casePath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		positionals []string
	}{
		{name: "single", positionals: []string{"unexpected"}},
		{name: "multiple", positionals: []string{"unexpected", "second"}},
		{name: "after separator", positionals: []string{"--", "unexpected"}},
		{name: "flag after positional", positionals: []string{"unexpected", "-repeat", "2"}},
		{name: "empty", positionals: []string{""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := filepath.Join(root, test.name+".json")
			args := append([]string{"-case", casePath, "-out", out}, test.positionals...)
			var exitCode int
			stdout := captureStdout(t, func() {
				exitCode = run(args)
			})
			if exitCode != 2 {
				t.Fatalf("run() = %d, want input-error exit 2", exitCode)
			}
			if strings.Contains(stdout, "Benchmark Summary:") {
				t.Fatalf("unexpected positional input printed summary: %q", stdout)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("unexpected positional input created report: %v", err)
			}
		})
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer

	type readResult struct {
		data []byte
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		data, readErr := io.ReadAll(reader)
		readDone <- readResult{data: data, err: readErr}
	}()

	func() {
		defer func() {
			os.Stdout = oldStdout
			_ = writer.Close()
		}()
		fn()
	}()

	result := <-readDone
	_ = reader.Close()
	if result.err != nil {
		t.Fatal(result.err)
	}
	return string(result.data)
}
