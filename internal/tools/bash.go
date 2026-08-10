package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/toolresult"
)

const (
	defaultBashTimeout = 30 * time.Second
	MaxBashOutputBytes = toolresult.MaxToolResultBytes
)

// BashCommandResult captures the local shell process result before applying
// model-tool-specific formatting.
type BashCommandResult struct {
	Output    string
	ExitCode  int
	TimedOut  bool
	Truncated bool
	Err       error
}

// RunBashCommand executes command with bash in workDir and returns combined
// stdout/stderr plus process status.
func RunBashCommand(ctx context.Context, workDir string, command string, timeout time.Duration) BashCommandResult {
	if timeout <= 0 {
		timeout = defaultBashTimeout
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", command)
	cmd.Dir = workDir
	configureShellCommand(cmd)

	output := newBoundedOutput(MaxBashOutputBytes)
	cmd.Stdout = output
	cmd.Stderr = output

	err := cmd.Run()
	result := BashCommandResult{
		Output:    output.String(),
		Truncated: output.Truncated(),
		Err:       err,
	}
	if timeoutCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Err = timeoutCtx.Err()
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	return result
}

type boundedOutput struct {
	mu        sync.Mutex
	limit     int
	buf       []byte
	truncated bool
}

func newBoundedOutput(limit int) *boundedOutput {
	return &boundedOutput{limit: limit}
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.limit <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(b.buf) < b.limit {
		remaining := b.limit - len(b.buf)
		if len(p) <= remaining {
			b.buf = append(b.buf, p...)
		} else {
			b.buf = append(b.buf, p[:remaining]...)
			b.truncated = true
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

func (b *boundedOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (b *boundedOutput) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// BashTool executes shell commands in a bash environment.
// Commands run with a 30-second timeout and are executed in the
// configured working directory.
type BashTool struct {
	// workDir is the directory where commands will be executed.
	workDir        string
	readOnly       bool
	readOnlyRunner readOnlyBashRunner
}

// NewReadOnlyBashTool creates a Bash-compatible tool whose commands must pass
// the conservative read-only shell policy before execution.
func NewReadOnlyBashTool(workDir string) *BashTool {
	return newReadOnlyBashToolWithRunner(workDir, newPlatformReadOnlyBashRunner())
}

// NewBashTool creates a new BashTool that executes commands in the specified directory.
// The workDir parameter sets the working directory for command execution.
// Returns a configured BashTool.
func NewBashTool(workDir string) *BashTool {
	return &BashTool{
		workDir: workDir,
	}
}

// Name returns the tool identifier "bash".
func (t *BashTool) Name() string {
	return "bash"
}

// Definition returns the tool schema for the bash tool.
// It describes the tool's capabilities and expected input format.
func (t *BashTool) Definition() schema.ToolDefinition {
	description := "Execute arbitrary bash commands in the current working directory. Supports chained commands (e.g., &&). Returns both stdout and stderr."
	if t.readOnly {
		description = "Execute conservatively validated read-only bash commands inside the current workspace. Mutation, background execution, dynamic shell forms, and network access are unavailable."
	}
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: description,
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The bash command to execute. Examples: ls -la or go test ./...",
				},
			},
			"required": []string{"command"},
		},
	}
}

// bashArgs represents the input arguments for the bash tool.
type bashArgs struct {
	// Command is the bash command string to execute.
	Command string `json:"command"`
}

// ExecuteResult runs a bash command and marks non-zero exits or timeouts as
// tool-level failures while preserving useful stdout/stderr for the model.
func (t *BashTool) ExecuteResult(ctx context.Context, args json.RawMessage) (ExecutionResult, error) {
	var input bashArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return ExecutionResult{}, fmt.Errorf("参数解析失败: %w", err)
	}
	if t.readOnly {
		readOnly, _, parsed := toolpolicy.AssessShell(input.Command, t.workDir, t.workDir)
		if !parsed || !readOnly {
			return ExecutionResult{
				Output: "Read-only Bash rejected a command that could not be proven non-mutating.",
				Failed: true,
			}, nil
		}
	}

	var result BashCommandResult
	if t.readOnly {
		if t.readOnlyRunner == nil {
			result = BashCommandResult{Err: ErrReadOnlyBashSandboxUnavailable}
		} else {
			result = t.readOnlyRunner.Run(ctx, readOnlyBashRequest{
				WorkDir:       t.workDir,
				ReadableRoots: []string{t.workDir},
				Command:       input.Command,
				Timeout:       defaultBashTimeout,
			})
		}
	} else {
		result = RunBashCommand(ctx, t.workDir, input.Command, defaultBashTimeout)
	}
	outputStr := result.Output
	if result.Truncated {
		outputStr = appendBashTruncationNotice(outputStr)
	}

	if result.TimedOut {
		warning := "\n[警告: 命令执行超时(30s)，已被系统强制终止。] "
		if !t.readOnly {
			warning = "\n[警告: 命令执行超时(30s)，已被系统强制终止。如果是常驻服务，请尝试将其转入后台。] "
		}
		return ExecutionResult{
			Output: outputStr + warning,
			Failed: true,
		}, nil
	}

	if result.Err != nil {
		return ExecutionResult{
			Output: fmt.Sprintf("执行报错: %v\n输出:\n%s", result.Err, outputStr),
			Failed: true,
		}, nil
	}

	if outputStr == "" {
		return ExecutionResult{Output: "命令执行成功，无终端输出。"}, nil
	}

	return ExecutionResult{Output: outputStr}, nil

}

// Execute runs a bash command with the provided arguments.
// The command executes with a 30-second timeout in the tool's working directory.
// Returns the command output (combined stdout and stderr), or an error if argument parsing fails.
// Timeouts are represented in the output for backward-compatible direct callers.
func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	result, err := t.ExecuteResult(ctx, args)
	if err != nil {
		return "", err
	}
	return result.Output, nil

}

func appendBashTruncationNotice(output string) string {
	return fmt.Sprintf("%s\n\n...[终端输出过长，已截断至前 %d 字节]...", output, MaxBashOutputBytes)
}
