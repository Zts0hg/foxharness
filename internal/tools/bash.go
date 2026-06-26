package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
)

const (
	defaultBashTimeout = 30 * time.Second
	MaxBashOutputBytes = 8000
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
	workDir string
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
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "Execute arbitrary bash commands in the current working directory. Supports chained commands (e.g., &&). Returns both stdout and stderr.",
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

// Execute runs a bash command with the provided arguments.
// The command executes with a 30-second timeout in the tool's working directory.
// Returns the command output (combined stdout and stderr), or an error if argument parsing fails.
// Timeouts are reported in the output rather than returning an error.
func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input bashArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	result := RunBashCommand(ctx, t.workDir, input.Command, defaultBashTimeout)
	outputStr := result.Output
	if result.Truncated {
		outputStr = appendBashTruncationNotice(outputStr)
	}

	if result.TimedOut {
		return outputStr + "\n[警告: 命令执行超时(30s)，已被系统强制终止。如果是常驻服务，请尝试将其转入后台。] ", nil
	}

	if result.Err != nil {
		return fmt.Sprintf("执行报错: %v\n输出:\n%s", result.Err, outputStr), nil
	}

	if outputStr == "" {
		return "命令执行成功，无终端输出。", nil
	}

	return outputStr, nil

}

func appendBashTruncationNotice(output string) string {
	return fmt.Sprintf("%s\n\n...[终端输出过长，已截断至前 %d 字节]...", output, MaxBashOutputBytes)
}
