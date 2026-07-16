package slash

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeForkRunner struct {
	called       bool
	agentType    string
	task         string
	allowedTools []string
	result       string
	err          error
}

func (f *fakeForkRunner) Run(ctx context.Context, task string, agentType string, allowedTools []string) (string, error) {
	f.called = true
	f.task = task
	f.agentType = agentType
	f.allowedTools = append([]string(nil), allowedTools...)
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

func TestExecutor_InlineModeCarriesFrontmatterEffort(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{
		Type:    CommandPrompt,
		Name:    "deep-review",
		Content: "Review deeply",
		Frontmatter: Frontmatter{
			Context: "inline",
			Effort:  "xhigh",
		},
	}
	got, err := exec.Execute(context.Background(), cmd, "", "sess-1")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Effort != "xhigh" {
		t.Fatalf("Effort = %q, want xhigh", got.Effort)
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

func TestExecutor_ClaudeVariableAliases(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{
		Type:     CommandPrompt,
		Content:  "skill=${CLAUDE_SKILL_DIR},sess=${CLAUDE_SESSION_ID}",
		SkillDir: "/abs/skill",
	}
	got, err := exec.Execute(context.Background(), cmd, "", "sess-1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Content != "skill=/abs/skill,sess=sess-1" {
		t.Errorf("content = %q", got.Content)
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

func TestExecutor_InlineMode_AfterHookDeferred(t *testing.T) {
	wd := t.TempDir()
	marker := wd + "/after.touched"
	exec := NewExecutor(WithWorkDir(wd))
	cmd := &Command{
		Type:    CommandPrompt,
		Content: "body",
		Frontmatter: Frontmatter{
			Hooks: &FrontmatterHooks{After: "touch " + marker},
		},
	}
	res, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.AfterHook == nil {
		t.Fatal("AfterHook must be set for inline + hooks.after")
	}
	// CRITICAL: marker must NOT exist yet — the hook must not have run
	// just because Execute returned.
	if fileExists(marker) {
		t.Fatal("after-hook fired prematurely inside Execute (defer bug)")
	}
	// Caller fires it explicitly.
	res.AfterHook(context.Background())
	if !fileExists(marker) {
		t.Errorf("after-hook closure did not run when caller invoked it")
	}
}

func TestExecutor_ForkMode_AfterHookSynchronous(t *testing.T) {
	wd := t.TempDir()
	marker := wd + "/fork.after.touched"
	fork := &fakeForkRunner{result: "report"}
	exec := NewExecutor(WithWorkDir(wd), WithForkRunner(fork))
	cmd := &Command{
		Type: CommandPrompt,
		Frontmatter: Frontmatter{
			Context: "fork",
			Hooks:   &FrontmatterHooks{After: "touch " + marker},
		},
	}
	res, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.AfterHook != nil {
		t.Error("fork-mode result must NOT carry AfterHook — Execute already fired it")
	}
	if !fileExists(marker) {
		t.Error("fork-mode after-hook must run synchronously inside Execute")
	}
}

func TestExecutor_InlineMode_NoAfterHookWhenNotDeclared(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{Type: CommandPrompt, Content: "body"}
	res, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.AfterHook != nil {
		t.Error("AfterHook must be nil when no after-hook declared")
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
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

func TestExecutor_ForkMode_PassesAllowedToolsToRunner(t *testing.T) {
	// Critical: when a fork-mode command declares allowed-tools, the
	// executor must thread that allow-list down into the ForkRunner so
	// the sub-agent's registry is filtered. Without this, fork mode (the
	// recommended escape hatch for model-invoked restricted skills) is a
	// silent policy bypass.
	fork := &fakeForkRunner{result: "out"}
	exec := NewExecutor(WithForkRunner(fork))
	cmd := &Command{
		Type: CommandPrompt,
		Frontmatter: Frontmatter{
			Context:      "fork",
			Agent:        "general-purpose",
			AllowedTools: []string{"read_file", "bash"},
		},
	}
	_, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(fork.allowedTools) != 2 {
		t.Fatalf("ForkRunner received %v allowedTools, want [read_file bash]", fork.allowedTools)
	}
	if fork.allowedTools[0] != "read_file" || fork.allowedTools[1] != "bash" {
		t.Errorf("ForkRunner allowedTools = %v, want [read_file bash]", fork.allowedTools)
	}
}

func TestExecutor_ForkMode_NoAllowedToolsPassesEmpty(t *testing.T) {
	fork := &fakeForkRunner{result: "out"}
	exec := NewExecutor(WithForkRunner(fork))
	cmd := &Command{
		Type: CommandPrompt,
		Frontmatter: Frontmatter{
			Context: "fork",
		},
	}
	_, err := exec.Execute(context.Background(), cmd, "", "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(fork.allowedTools) != 0 {
		t.Errorf("ForkRunner allowedTools should be empty when not declared, got %v", fork.allowedTools)
	}
}
