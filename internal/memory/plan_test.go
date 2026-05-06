package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type fakePlanProvider struct {
	content string
	err     error
}

func (p fakePlanProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: p.content}, nil
}

func TestBuildPlanWritesPlanAndTodoFromJSON(t *testing.T) {
	content := marshalPlanDraft(t, planDraft{
		Plan: "# PLAN\n\n## Goal\n\nRead go.mod.\n",
		Todo: "# TODO\n\n- [ ] Read go.mod\n",
	})
	store := NewStore(t.TempDir())
	planner := NewPlanner(fakePlanProvider{content: content}, store)

	if err := planner.BuildPlan(context.Background(), "读取 go.mod"); err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	plan := readTestFile(t, store.PlanPath())
	if !strings.Contains(plan, "Read go.mod") {
		t.Fatalf("PLAN.md content = %q", plan)
	}

	todo := readTestFile(t, store.TodoPath())
	if !strings.Contains(todo, "- [ ] Read go.mod") {
		t.Fatalf("TODO.md content = %q", todo)
	}
}

func TestBuildPlanRejectsInvalidJSON(t *testing.T) {
	store := NewStore(t.TempDir())
	planner := NewPlanner(fakePlanProvider{content: "```markdown\n# PLAN.md\n```"}, store)

	if err := planner.BuildPlan(context.Background(), "任务"); err == nil || !strings.Contains(err.Error(), "解析 Plan JSON 失败") {
		t.Fatalf("BuildPlan error = %v", err)
	}
}

func TestBuildPlanRejectsMissingFields(t *testing.T) {
	content := marshalPlanDraft(t, planDraft{Plan: "# PLAN\n"})
	store := NewStore(t.TempDir())
	planner := NewPlanner(fakePlanProvider{content: content}, store)

	if err := planner.BuildPlan(context.Background(), "任务"); err == nil || !strings.Contains(err.Error(), "plan 或 todo") {
		t.Fatalf("BuildPlan error = %v", err)
	}
}

func TestBuildPlanReturnsProviderError(t *testing.T) {
	store := NewStore(t.TempDir())
	planner := NewPlanner(fakePlanProvider{err: errors.New("provider failed")}, store)

	if err := planner.BuildPlan(context.Background(), "任务"); err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("BuildPlan error = %v", err)
	}
}

func marshalPlanDraft(t *testing.T, draft planDraft) string {
	t.Helper()

	data, err := json.Marshal(draft)
	if err != nil {
		t.Fatalf("marshal plan draft: %v", err)
	}
	return string(data)
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
