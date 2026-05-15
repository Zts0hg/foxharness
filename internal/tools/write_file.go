package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// WriteFileTool creates or overwrites files with the provided content.
// Parent directories are created automatically if they don't exist.
type WriteFileTool struct {
	// workDir is the base directory for resolving relative file paths.
	workDir string
}

// NewWriteFileTool creates a new WriteFileTool that writes files relative to the specified directory.
// The workDir parameter sets the base directory for file path resolution.
// Returns a configured WriteFileTool.
func NewWriteFileTool(workDir string) *WriteFileTool {
	return &WriteFileTool{
		workDir: workDir,
	}
}

// Name returns the tool identifier "write_file".
func (t *WriteFileTool) Name() string {
	return "write_file"
}

// Definition returns the tool schema for the write_file tool.
// It describes the tool's capabilities and expected input format.
func (t *WriteFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "Create or overwrite a file with the provided content. Parent directories are created automatically if they don't exist. Provide paths relative to the working directory.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to write, e.g., src/main.go",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Complete content to write to the file",
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

// writeFileArgs represents the input arguments for the write_file tool.
type writeFileArgs struct {
	// Path is the relative path to the file to write.
	Path string `json:"path"`
	// Content is the complete file content to write.
	Content string `json:"content"`
}

// Execute writes the provided content to the file at the specified path.
// The path is resolved relative to the tool's working directory.
// Parent directories are created automatically if they don't exist.
// Returns a success message, or an error if the write operation fails.
func (t *WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input writeFileArgs

	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	fullPath := filepath.Join(t.workDir, input.Path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("创建父目录失败: %w", err)
	}

	err := os.WriteFile(fullPath, []byte(input.Content), 0644)
	if err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return fmt.Sprintf("成功将内容写入到文件: %s", input.Path), nil

}
