package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/benchmark"
)

func TestEVBEN001FlagContract(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		want           benchmarkOptions
		wantParseError bool
		wantHelp       bool
		wantArgs       []string
	}{
		{
			name: "defaults",
			args: []string{"-case", "case.yaml"},
			want: benchmarkOptions{casePath: "case.yaml", outPath: "benchmark-result.json", repeat: 1},
		},
		{
			name: "explicit values",
			args: []string{"-case", "case.yaml", "-out", "report.json", "-repeat", "3"},
			want: benchmarkOptions{casePath: "case.yaml", outPath: "report.json", repeat: 3},
		},
		{
			name: "repeated scalar flags use last value",
			args: []string{"-case", "first.yaml", "-case", "last.yaml", "-out", "first.json", "-out", "last.json", "-repeat", "2", "-repeat", "4"},
			want: benchmarkOptions{casePath: "last.yaml", outPath: "last.json", repeat: 4},
		},
		{
			name:           "unknown flag",
			args:           []string{"-unknown"},
			want:           benchmarkOptions{outPath: "benchmark-result.json", repeat: 1},
			wantParseError: true,
		},
		{
			name:           "invalid repeat",
			args:           []string{"-repeat", "many"},
			want:           benchmarkOptions{outPath: "benchmark-result.json"},
			wantParseError: true,
		},
		{
			name:           "help",
			args:           []string{"-h"},
			want:           benchmarkOptions{outPath: "benchmark-result.json", repeat: 1},
			wantParseError: true,
			wantHelp:       true,
		},
		{
			name:     "positional remains unconsumed",
			args:     []string{"-case", "case.yaml", "unexpected", "-repeat", "2"},
			want:     benchmarkOptions{casePath: "case.yaml", outPath: "benchmark-result.json", repeat: 1},
			wantArgs: []string{"unexpected", "-repeat", "2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flags, options := newBenchmarkFlagSet()
			err := flags.Parse(test.args)
			if !test.wantParseError && err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if test.wantParseError && err == nil {
				t.Fatalf("Parse() error = nil, want parse failure")
			}
			if test.wantHelp && !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("Parse() error = %v, want flag.ErrHelp", err)
			}
			if *options != test.want {
				t.Fatalf("options = %#v, want %#v", *options, test.want)
			}
			if strings.Join(flags.Args(), "\x00") != strings.Join(test.wantArgs, "\x00") {
				t.Fatalf("Args() = %#v, want %#v", flags.Args(), test.wantArgs)
			}
		})
	}
}

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

func TestEVBEN001RequiredCasePathFailsBeforeReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.json")
	if got := run([]string{"-out", out}); got != 2 {
		t.Fatalf("run() = %d, want input-error exit 2", got)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("missing case path created report: %v", err)
	}
}

func TestEVBEN008RepeatAggregationPreservesMixedOrderedResults(t *testing.T) {
	want := []*benchmark.Result{
		{RepeatIndex: 1, Success: true, Status: benchmark.ResultStatusCompleted},
		{RepeatIndex: 2, Status: benchmark.ResultStatusFailed},
		{RepeatIndex: 3, Success: true, Status: benchmark.ResultStatusCompleted},
	}
	results, infrastructureFailed := executeRepeats(context.Background(), &benchmark.Case{ID: "mixed"}, len(want), func(_ context.Context, _ *benchmark.Case, index int) (*benchmark.Result, error) {
		return want[index-1], nil
	})
	if infrastructureFailed || len(results) != len(want) {
		t.Fatalf("mixed aggregation = results:%d infrastructure:%t", len(results), infrastructureFailed)
	}
	for index := range want {
		if results[index] != want[index] || results[index].RepeatIndex != index+1 {
			t.Fatalf("result[%d] = %#v, want original %#v", index, results[index], want[index])
		}
	}
}

func TestEVBEN011CommandFailurePrecedenceAndSideEffectOrder(t *testing.T) {
	tests := []struct {
		name       string
		loadErr    error
		results    []*benchmark.Result
		infra      bool
		writeErr   error
		wantExit   int
		wantEvents string
	}{
		{name: "case load", loadErr: errors.New("load"), wantExit: 2, wantEvents: "load"},
		{name: "success", results: []*benchmark.Result{{Success: true, Status: benchmark.ResultStatusCompleted}}, wantExit: 0, wantEvents: "load,execute:2,summary,write:report.json"},
		{name: "evaluation failure", results: []*benchmark.Result{{Status: benchmark.ResultStatusFailed}}, wantExit: 1, wantEvents: "load,execute:2,summary,write:report.json"},
		{name: "runtime cancellation", results: []*benchmark.Result{{Status: benchmark.ResultStatusCancelled}}, wantExit: 1, wantEvents: "load,execute:2,summary,write:report.json"},
		{name: "setup infrastructure", results: []*benchmark.Result{{Status: benchmark.ResultStatusInfrastructureFailed}}, infra: true, wantExit: 2, wantEvents: "load,execute:2,summary,write:report.json"},
		{name: "report write", results: []*benchmark.Result{{Success: true, Status: benchmark.ResultStatusCompleted}}, writeErr: errors.New("write"), wantExit: 2, wantEvents: "load,execute:2,summary,write:report.json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var events []string
			dependencies := benchmarkCommandDependencies{
				loadCase: func(path string) (*benchmark.Case, error) {
					events = append(events, "load")
					if path != "case.yaml" {
						t.Fatalf("load path = %q", path)
					}
					return &benchmark.Case{ID: "case"}, test.loadErr
				},
				execute: func(_ context.Context, _ *benchmark.Case, repeat int) ([]*benchmark.Result, bool) {
					events = append(events, "execute:"+strconv.Itoa(repeat))
					return test.results, test.infra
				},
				printSummary: func(results []*benchmark.Result) {
					events = append(events, "summary")
					if len(results) != len(test.results) {
						t.Fatalf("summary results = %d, want %d", len(results), len(test.results))
					}
				},
				writeJSON: func(path string, results []*benchmark.Result) error {
					events = append(events, "write:"+path)
					return test.writeErr
				},
			}
			if got := runWithDependencies([]string{"-case", "case.yaml", "-repeat", "2", "-out", "report.json"}, dependencies); got != test.wantExit {
				t.Fatalf("runWithDependencies() = %d, want %d", got, test.wantExit)
			}
			if got := strings.Join(events, ","); got != test.wantEvents {
				t.Fatalf("events = %q, want %q", got, test.wantEvents)
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
