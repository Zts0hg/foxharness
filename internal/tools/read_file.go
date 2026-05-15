package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// ReadFileTool reads the contents of files from the filesystem.
// Files are read relative to the configured working directory.
// Large files are truncated to avoid excessive output.
type ReadFileTool struct {
	// workDir is the base directory for resolving relative file paths.
	workDir string
}

// NewReadFileTool creates a new ReadFileTool that reads files relative to the specified directory.
// The workDir parameter sets the base directory for file path resolution.
// Returns a configured ReadFileTool.
func NewReadFileTool(workDir string) *ReadFileTool {
	return &ReadFileTool{workDir: workDir}
}

// Name returns the tool identifier "read_file".
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Definition returns the tool schema for the read_file tool.
// It describes the tool's capabilities and expected input format.
func (t *ReadFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "Read the contents of a file at the specified path. Paths are relative to the working directory.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to read, e.g., cmd/fox/main.go",
				},
			},
			"required": []string{"path"},
		},
	}
}

// readFileArgs represents the input arguments for the read_file tool.
type readFileArgs struct {
	// Path is the relative path to the file to read.
	Path string `json:"path"`
}

// Execute reads the file at the specified path and returns its contents.
// The path is resolved relative to the tool's working directory.
// Files larger than 8000 bytes are truncated with a notification.
// Returns the file contents, or an error if the file cannot be read.
func (t *ReadFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input readFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	fullPath := filepath.Join(t.workDir, input.Path)
	file, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}

	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("读取文件内容失败: %w", err)
	}

	const maxLen = 8000
	if len(content) > maxLen {
		truncatedMessage := fmt.Sprintf("%s\n\n...[由于内容过长，已经被系统截断至前 %d 字节]...", string(content[:maxLen]), maxLen)
		return truncatedMessage, nil
	}

	return string(content), nil
}

// ParallelSafe indicates that this tool can be executed in parallel with other tools.
// Reading files is a read-only operation and doesn't mutate shared state.
func (t *ReadFileTool) ParallelSafe() bool {
	return true
}
