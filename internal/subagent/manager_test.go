package subagent

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type finalReportProvider struct{}

func (p *finalReportProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "subagent report"},
	}, nil
}

func TestManagerRunDoesNotWriteStdout(t *testing.T) {
	manager := NewManager(&finalReportProvider{}, t.TempDir())

	var result *Result
	stdout := captureStdout(t, func() {
		var err error
		result, err = manager.Run(context.Background(), Request{
			ParentSessionID: "parent-session",
			Task:            "inspect code",
			ReadOnly:        true,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	if stdout != "" {
		t.Fatalf("Run() wrote stdout %q, want empty", stdout)
	}
	if result == nil || result.Report != "subagent report" {
		t.Fatalf("Run() result = %#v, want subagent report", result)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("reader Close() error = %v", err)
	}

	return string(out)
}
