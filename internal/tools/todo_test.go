package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTodoReturnsDefaultWhenMissing(t *testing.T) {
	tool := NewReadTodoTool(t.TempDir())
	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out, "- [ ] Not recorded.") {
		t.Fatalf("output = %q, want default TODO", out)
	}
}

func TestUpdateTodoWritesContentAndCountsCheckboxes(t *testing.T) {
	dir := t.TempDir()
	tool := NewUpdateTodoTool(dir)
	out, err := tool.Execute(context.Background(), []byte(`{"content":"# TODO\n\n- [x] Done\n- [ ] Next"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if out != "TODO.md updated: 1/2 items complete." {
		t.Fatalf("output = %q", out)
	}

	data, err := os.ReadFile(filepath.Join(dir, "TODO.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); got != "# TODO\n\n- [x] Done\n- [ ] Next\n" {
		t.Fatalf("TODO.md = %q", got)
	}
}

func TestUpdateTodoRejectsBlankContent(t *testing.T) {
	tool := NewUpdateTodoTool(t.TempDir())
	if _, err := tool.Execute(context.Background(), []byte(`{"content":"   "}`)); err == nil {
		t.Fatalf("Execute() error = nil, want error")
	}
}
