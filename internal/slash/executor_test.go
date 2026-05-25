package slash

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeForkRunner struct {
	called    bool
	agentType string
	task      string
	result    string
	err       error
}

func (f *fakeForkRunner) Run(ctx context.Context, task string, agentType string) (string, error) {
	f.called = true
	f.task = task
	f.agentType = agentType
	if f.err != nil {
		return "", f.err
	}
	return f.result, nil
}

func TestExecutor_InlineMode_FullPipeline(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{
		Type:    CommandPrompt,
		Name:    "review",
		Content: "Review: $ARGUMENTS",
		Frontmatter: Frontmatter{
			Context: "inline",
		},
	}
	got, err := exec.Execute(context.Background(), cmd, "pr-123", "sess-1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Content != "Review: pr-123" {
		t.Errorf("got %q", got.Content)
	}
	if got.Fork {
		t.Error("inline result should have Fork=false")
	}
}

func TestExecutor_AutoAppendArguments(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{Type: CommandPrompt, Content: "no placeholders here"}
	got, err := exec.Execute(context.Background(), cmd, "appended", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got.Content, "ARGUMENTS: appended") {
		t.Errorf("expected appended ARGUMENTS, got %q", got.Content)
	}
}

func TestExecutor_NamedArguments(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{
		Type:    CommandPrompt,
		Content: "[$file]-[$message]",
		Frontmatter: Frontmatter{
			Arguments: "file message",
		},
	}
	got, err := exec.Execute(context.Background(), cmd, "main.go fix", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Content != "[main.go]-[fix]" {
		t.Errorf("got %q", got.Content)
	}
}

func TestExecutor_VariableReplacement(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{
		Type:     CommandPrompt,
		Content:  "skill=${FOXHARNESS_SKILL_DIR},sess=${FOXHARNESS_SESSION_ID}",
		SkillDir: "/abs/skill",
	}
	got, err := exec.Execute(context.Background(), cmd, "", "sess-1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got.Content, "skill=/abs/skill") {
		t.Errorf("skill dir not replaced: %q", got.Content)
	}
	if !strings.Contains(got.Content, "sess=sess-1") {
		t.Errorf("session id not replaced: %q", got.Content)
	}
}

func TestExecutor_ForkMode_CallsRunner(t *testing.T) {
	fork := &fakeForkRunner{result: "fork-output"}
	exec := NewExecutor(WithForkRunner(fork))
	cmd := &Command{
		Type:    CommandPrompt,
		Content: "Do thing with $ARGUMENTS",
		Frontmatter: Frontmatter{
			Context: "fork",
			Agent:   "general-purpose",
		},
	}
	got, err := exec.Execute(context.Background(), cmd, "pr-9", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !fork.called {
		t.Fatal("ForkRunner.Run was not called")
	}
	if fork.agentType != "general-purpose" {
		t.Errorf("agentType = %q", fork.agentType)
	}
	if !strings.Contains(fork.task, "Do thing with pr-9") {
		t.Errorf("task = %q", fork.task)
	}
	if got.Content != "fork-output" {
		t.Errorf("got = %q", got.Content)
	}
	if !got.Fork {
		t.Error("Fork=true expected for fork-mode result")
	}
}

func TestExecutor_ForkMode_NoRunner_Errors(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{
		Type:    CommandPrompt,
		Content: "x",
		Frontmatter: Frontmatter{
			Context: "fork",
		},
	}
	_, err := exec.Execute(context.Background(), cmd, "", "")
	if err == nil {
		t.Fatal("expected error when fork runner unavailable")
	}
	if !strings.Contains(err.Error(), "fork mode unavailable") {
		t.Errorf("err = %v", err)
	}
}

func TestExecutor_InlineMode_NeverCallsFork(t *testing.T) {
	fork := &fakeForkRunner{result: "should not be used"}
	exec := NewExecutor(WithForkRunner(fork))
	cmd := &Command{Type: CommandPrompt, Content: "inline content"}
	_, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if fork.called {
		t.Error("inline must not call fork runner")
	}
}

func TestExecutor_EmptyContent_Valid(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{Type: CommandPrompt, Content: ""}
	got, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Content != "" {
		t.Errorf("got %q", got.Content)
	}
}

func TestExecutor_ForkRunnerError_Propagates(t *testing.T) {
	fork := &fakeForkRunner{err: errors.New("boom")}
	exec := NewExecutor(WithForkRunner(fork))
	cmd := &Command{Type: CommandPrompt, Frontmatter: Frontmatter{Context: "fork"}}
	_, err := exec.Execute(context.Background(), cmd, "", "")
	if err == nil {
		t.Fatal("expected propagated error")
	}
}

func TestExecutor_InlineMode_SurfacesAllowedTools(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{
		Type:    CommandPrompt,
		Content: "body",
		Frontmatter: Frontmatter{
			AllowedTools: []string{"read_file", "bash"},
		},
	}
	got, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got.AllowedTools) != 2 || got.AllowedTools[0] != "read_file" || got.AllowedTools[1] != "bash" {
		t.Errorf("AllowedTools = %v", got.AllowedTools)
	}
}

func TestExecutor_ForkMode_OmitsAllowedTools(t *testing.T) {
	fork := &fakeForkRunner{result: "out"}
	exec := NewExecutor(WithForkRunner(fork))
	cmd := &Command{
		Type: CommandPrompt,
		Frontmatter: Frontmatter{
			Context:      "fork",
			AllowedTools: []string{"read_file"},
		},
	}
	got, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got.AllowedTools) != 0 {
		t.Errorf("fork mode should not surface AllowedTools to caller, got %v", got.AllowedTools)
	}
}
