package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileToolReturnsCompleteLargeFile(t *testing.T) {
	workDir := t.TempDir()
	content := strings.Repeat("abcdefghij", 1000)
	path := filepath.Join(workDir, "large.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	args, err := json.Marshal(map[string]string{"path": "large.txt"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got, err := NewReadFileTool(workDir).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got != content {
		t.Fatalf("Execute() returned %d bytes, want complete %d-byte file", len(got), len(content))
	}
}
