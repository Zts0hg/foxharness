package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
)

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

	timeoutCtx, cancal := context.WithTimeout(ctx, 30*time.Second)
	defer cancal()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", input.Command)
	cmd.Dir = t.workDir

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return outputStr + "\n[警告: 命令执行超时(30s)，已被系统强制终止。如果是常驻服务，请尝试将其转入后台。] ", nil
	}

	if err != nil {
		return fmt.Sprintf("执行报错: %v\n输出:\n%s", err, outputStr), nil
	}

	if outputStr == "" {
		return "命令执行成功，无终端输出。", nil
	}

	const maxLen = 8000
	if len(outputStr) > maxLen {
		return fmt.Sprintf("%s\n\n...[终端输出过长，已截断至前 %d 字节]...", outputStr[:maxLen], maxLen), nil
	}

	return outputStr, nil

}
