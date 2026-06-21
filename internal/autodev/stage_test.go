package autodev

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// fakeCore is a scripted CoreRunner. Each Run invocation executes the next
// effect from the script (simulating the core Agent's work) and records the
// prompt it received.
type fakeCore struct {
	workDir string
	prompts []string
	effects []func()
	asker   tools.UserAsker
}

func (f *fakeCore) Run(ctx context.Context, prompt string, r engine.Reporter) (*engine.RunResult, error) {
	f.prompts = append(f.prompts, prompt)
	if len(f.effects) > 0 {
		effect := f.effects[0]
		f.effects = f.effects[1:]
		if effect != nil {
			effect()
		}
	}
	return &engine.RunResult{FinalMessage: "done, I believe"}, nil
}

func (f *fakeCore) SetUserAsker(a tools.UserAsker) { f.asker = a }
func (f *fakeCore) SetModel(model string) error    { return nil }
func (f *fakeCore) WorkDir() string                { return f.workDir }

func (f *fakeCore) StagePrompt(ctx context.Context, command, args string) (string, error) {
	return fmt.Sprintf("PROMPT[%s|%s]", command, args), nil
}

// reviewingEngineer scripts EngineerAgent.Review responses.
type reviewingEngineer struct {
	fakeEngineerAgent
	reviews     []string
	reviewCalls int
	gaps        []string
}

func (r *reviewingEngineer) Review(ctx context.Context, res *engine.RunResult, gap string, c StageContext) (string, error) {
	r.reviewCalls++
	r.gaps = append(r.gaps, gap)
	if len(r.reviews) == 0 {
		return "", nil
	}
	out := r.reviews[0]
	r.reviews = r.reviews[1:]
	return out, nil
}

func newTestMachine(eng EngineerAgent) *StageMachine {
	return NewStageMachine(eng, NewTerminalReporter(io.Discard))
}

func artifactStage(name string, path *string) Stage {
	return Stage{
		Name:    name,
		Command: "codexspec:" + name,
		Args:    func(sc *StageContext) string { return "args" },
		Verify: func(ctx context.Context, sc *StageContext) (bool, string) {
			if *path == "" {
				return false, name + " artifact absent"
			}
			return true, ""
		},
	}
}

func TestRunStepAdvancesOnlyWhenVerifyPasses(t *testing.T) {
	artifact := ""
	core := &fakeCore{effects: []func(){
		nil, // first run produces nothing (TC-005)
		func() { artifact = "present" },
	}}
	eng := &reviewingEngineer{reviews: []string{"the artifact is missing; create it"}}
	machine := newTestMachine(eng)

	sc := &StageContext{Slug: "x"}
	if err := machine.RunStep(context.Background(), core, sc, artifactStage("generate-spec", &artifact)); err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if len(core.prompts) != 2 {
		t.Fatalf("core runs = %d, want 2 (no advance until Verify passes, TC-005)", len(core.prompts))
	}
	if eng.reviewCalls != 1 {
		t.Errorf("Review calls = %d, want 1", eng.reviewCalls)
	}
	if !strings.Contains(eng.gaps[0], "artifact absent") {
		t.Errorf("Review gap = %q, want the VerifyGap", eng.gaps[0])
	}
}

func TestRunStepSeedsWithStagePrompt(t *testing.T) {
	artifact := "present"
	core := &fakeCore{}
	machine := newTestMachine(&reviewingEngineer{})

	sc := &StageContext{}
	if err := machine.RunStep(context.Background(), core, sc, artifactStage("generate-spec", &artifact)); err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}
	if len(core.prompts) != 1 {
		t.Fatalf("core runs = %d, want 1", len(core.prompts))
	}
	if core.prompts[0] != "PROMPT[codexspec:generate-spec|args]" {
		t.Errorf("seed prompt = %q, want materialized codexspec body (REQ-015)", core.prompts[0])
	}
}

func TestRunStepEngineerApprovalCannotAdvance(t *testing.T) {
	artifact := ""
	core := &fakeCore{effects: []func(){
		nil,
		nil,
		func() { artifact = "present" },
	}}
	// The engineer wrongly approves every run (returns "").
	eng := &reviewingEngineer{}
	machine := newTestMachine(eng)

	sc := &StageContext{}
	if err := machine.RunStep(context.Background(), core, sc, artifactStage("spec-to-plan", &artifact)); err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}

	if len(core.prompts) != 3 {
		t.Fatalf("core runs = %d, want 3 — engineer approval must not advance a failing step (TC-006/TC-025)", len(core.prompts))
	}
	// The synthesized continuation must surface the gap to the core Agent.
	if !strings.Contains(core.prompts[1], "spec-to-plan artifact absent") {
		t.Errorf("retry prompt = %q, want the verification gap embedded", core.prompts[1])
	}
}

func TestRunStepEngineerCorrectionDrivesRetry(t *testing.T) {
	artifact := ""
	core := &fakeCore{effects: []func(){
		nil,
		func() { artifact = "present" },
	}}
	eng := &reviewingEngineer{reviews: []string{"Run git add -A, then retry the commit."}}
	machine := newTestMachine(eng)

	sc := &StageContext{}
	if err := machine.RunStep(context.Background(), core, sc, artifactStage("commit", &artifact)); err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}
	if core.prompts[1] != "Run git add -A, then retry the commit." {
		t.Errorf("retry prompt = %q, want the engineer's correction verbatim (TC-009/TC-024)", core.prompts[1])
	}
}

func TestRunStepHonorsContextCancellation(t *testing.T) {
	artifact := ""
	ctx, cancel := context.WithCancel(context.Background())
	core := &fakeCore{effects: []func(){
		nil,
		func() { cancel() },
	}}
	machine := newTestMachine(&reviewingEngineer{})

	sc := &StageContext{}
	err := machine.RunStep(ctx, core, sc, artifactStage("never-done", &artifact))
	if err == nil {
		t.Fatal("RunStep returned nil error after cancellation, want ctx error")
	}
}

func TestRunStepSkipPredicateSkipsRun(t *testing.T) {
	artifact := ""
	core := &fakeCore{}
	machine := newTestMachine(&reviewingEngineer{})

	st := artifactStage("push", &artifact)
	st.Skip = func(ctx context.Context, sc *StageContext) bool { return true }

	if err := machine.RunStep(context.Background(), core, sc(), st); err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}
	if len(core.prompts) != 0 {
		t.Errorf("core runs = %d, want 0 when Skip reports already-done (PLAN-002)", len(core.prompts))
	}
}

func sc() *StageContext { return &StageContext{} }

func TestRequirementsFirstPipelineOrder(t *testing.T) {
	stages := RequirementsFirstPipeline(PipelineDeps{Clock: newTestClock()})
	want := []string{"materialize-requirements", "generate-spec", "spec-to-plan", "plan-to-tasks", "implement-tasks"}
	if len(stages) != len(want) {
		t.Fatalf("len(stages) = %d, want %d", len(stages), len(want))
	}
	for i, name := range want {
		if stages[i].Name != name {
			t.Errorf("stages[%d].Name = %q, want %q (REQ-009)", i, stages[i].Name, name)
		}
		if name == "materialize-requirements" {
			if stages[i].Control == nil {
				t.Errorf("materialize stage missing deterministic Control function")
			}
			continue
		}
		if stages[i].Command != "codexspec:"+name {
			t.Errorf("stages[%d].Command = %q, want codexspec command", i, stages[i].Command)
		}
	}
}

func TestMaterializeRequirementsCreatesConfirmedRequirements(t *testing.T) {
	workDir := t.TempDir()
	core := &fakeCore{workDir: workDir}
	machine := newTestMachine(&reviewingEngineer{})

	sc := &StageContext{
		WorkDir: workDir,
		Slug:    "persist-memory",
		Item: Item{
			Title:       "Persist Memory",
			Description: "Persist durable discoveries to MEMORY.md.",
		},
	}
	mat := RequirementsFirstPipeline(PipelineDeps{Clock: newTestClock()})[0]
	if err := machine.RunStep(context.Background(), core, sc, mat); err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}
	if len(core.prompts) != 0 {
		t.Fatalf("core runs = %d, want 0 for deterministic requirements materialization", len(core.prompts))
	}
	if ok := regexp.MustCompile(`^\.codexspec/specs/2026-0610-1200[a-z0-9]{2}-persist-memory$`).MatchString(sc.FeatureDir); !ok {
		t.Fatalf("FeatureDir = %q, want CodexSpec timestamp feature directory", sc.FeatureDir)
	}
	content, err := os.ReadFile(filepath.Join(workDir, sc.FeatureDir, "requirements.md"))
	if err != nil {
		t.Fatalf("read requirements.md: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"# Confirmed Requirements: Persist Memory",
		"### NEED-001: Persist Memory",
		"**Status**: confirmed",
		"Persist durable discoveries to MEMORY.md.",
		"Entries Confirmed**: NEED-001",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("requirements.md missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "[What outcome") || strings.Contains(text, "**Status**: open") {
		t.Errorf("requirements.md kept placeholder/open template content:\n%s", text)
	}
}

func TestGenerationStagesUseExplicitFeatureArtifactArgs(t *testing.T) {
	stages := RequirementsFirstPipeline(PipelineDeps{Clock: newTestClock()})
	sc := &StageContext{FeatureDir: filepath.Join(".codexspec", "specs", "2026-0610-1200ab-feature")}

	tests := []struct {
		index int
		want  string
	}{
		{1, filepath.Join(sc.FeatureDir, "requirements.md")},
		{2, filepath.Join(sc.FeatureDir, "spec.md")},
		{3, filepath.Join(sc.FeatureDir, "plan.md")},
		{4, filepath.Join(sc.FeatureDir, "tasks.md")},
	}
	for _, tt := range tests {
		if got := stages[tt.index].Args(sc); got != tt.want {
			t.Errorf("stage %s args = %q, want %q", stages[tt.index].Name, got, tt.want)
		}
	}
}

func TestGenerationVerifyRequiresArtifactAndPassingReview(t *testing.T) {
	workDir := t.TempDir()
	featureDir := filepath.Join(".codexspec", "specs", "2026-0610-1200ab-feature")
	sc := &StageContext{WorkDir: workDir, FeatureDir: featureDir}
	stages := RequirementsFirstPipeline(PipelineDeps{Clock: newTestClock()})
	gen := stages[1]

	ok, gap := gen.Verify(context.Background(), sc)
	if ok || !strings.Contains(gap, "spec.md") {
		t.Fatalf("Verify = %v, gap = %q, want missing spec.md failure", ok, gap)
	}
	writeArtifact(t, workDir, featureDir, "spec.md", "# Spec")
	ok, gap = gen.Verify(context.Background(), sc)
	if ok || !strings.Contains(gap, "review-spec.md") {
		t.Fatalf("Verify = %v, gap = %q, want missing review-spec.md failure", ok, gap)
	}
	writeArtifact(t, workDir, featureDir, "review-spec.md", "# Report\n\n- **Overall Status**: NEEDS_REVISION\n")
	ok, gap = gen.Verify(context.Background(), sc)
	if ok || !strings.Contains(gap, "NEEDS_REVISION") {
		t.Fatalf("Verify = %v, gap = %q, want review status failure", ok, gap)
	}
	writeArtifact(t, workDir, featureDir, "review-spec.md", "# Report\n\n- **Overall Status**: PASS\n")
	ok, gap = gen.Verify(context.Background(), sc)
	if !ok {
		t.Fatalf("Verify failed with artifact and passing review: %s", gap)
	}
}

func TestPlanAndTasksVerifyRequireArtifactsAndReviews(t *testing.T) {
	workDir := t.TempDir()
	featureDir := filepath.Join(".codexspec", "specs", "2026-0610-1200ab-feature")

	stages := RequirementsFirstPipeline(PipelineDeps{Clock: newTestClock()})
	sc := &StageContext{WorkDir: workDir, FeatureDir: featureDir}

	if ok, _ := stages[2].Verify(context.Background(), sc); ok {
		t.Error("spec-to-plan Verify passed without plan.md")
	}
	writeArtifact(t, workDir, featureDir, "plan.md", "# Plan")
	writeArtifact(t, workDir, featureDir, "review-plan.md", "# Report\n\n- **Overall Status**: PASS_WITH_WARNINGS\n")
	if ok, gap := stages[2].Verify(context.Background(), sc); !ok {
		t.Errorf("spec-to-plan Verify failed with plan.md and passing review: %s", gap)
	}

	if ok, _ := stages[3].Verify(context.Background(), sc); ok {
		t.Error("plan-to-tasks Verify passed without tasks.md")
	}
	writeArtifact(t, workDir, featureDir, "tasks.md", "# Tasks")
	writeArtifact(t, workDir, featureDir, "review-tasks.md", "# Report\n\n- **Overall Status**: BLOCKED\n")
	if ok, gap := stages[3].Verify(context.Background(), sc); ok || !strings.Contains(gap, "BLOCKED") {
		t.Errorf("plan-to-tasks Verify = %v, gap %q, want BLOCKED review failure", ok, gap)
	}
	writeArtifact(t, workDir, featureDir, "review-tasks.md", "# Report\n\n- **Overall Status**: PASS\n")
	if ok, gap := stages[3].Verify(context.Background(), sc); !ok {
		t.Errorf("plan-to-tasks Verify failed with tasks.md and passing review: %s", gap)
	}
}

func writeArtifact(t *testing.T, workDir, featureDir, name, content string) {
	t.Helper()
	path := filepath.Join(workDir, featureDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeGate scripts the completion gate outcome for implement Verify tests.
type fakeGate struct {
	result GateResult
}

func (g *fakeGate) Check(ctx context.Context, workDir string, cfg GateConfig) (GateResult, error) {
	return g.result, nil
}

// fakeDiffGit fakes the GitRunner used by the implement Verify diff check.
type fakeDiffGit struct {
	status string
	diff   string
}

func (g *fakeDiffGit) Run(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "status" {
		return g.status, nil
	}
	return g.diff, nil
}

// erroringGit fails every git invocation, simulating a broken git binary.
type erroringGit struct{}

func (erroringGit) Run(ctx context.Context, dir string, args ...string) (string, error) {
	return "git: not found", fmt.Errorf("exec git: not found")
}

func TestImplementVerifySurfacesGitErrors(t *testing.T) {
	stages := RequirementsFirstPipeline(PipelineDeps{
		Gate:  &fakeGate{result: GateResult{Passed: true}},
		Git:   erroringGit{},
		Clock: newTestClock(),
	})
	impl := stages[4]
	workDir := t.TempDir()
	featureDir := filepath.Join(".codexspec", "specs", "2026-0610-1200ab-feature")
	writeArtifact(t, workDir, featureDir, "tasks.md", "- [x] Task 1\n")
	sc := &StageContext{WorkDir: workDir, FeatureDir: featureDir, BaseBranch: "main"}

	ok, gap := impl.Verify(context.Background(), sc)
	if ok {
		t.Fatal("Verify passed although git queries failed")
	}
	if !strings.Contains(gap, "cannot inspect the worktree") {
		t.Errorf("gap = %q, want the git failure surfaced, not a phantom empty diff (CODE-002)", gap)
	}
	if strings.Contains(gap, "no changes") {
		t.Errorf("gap = %q, must not claim an empty diff when git itself failed", gap)
	}
}

func TestImplementVerifyRequiresGatesAndNonEmptyDiff(t *testing.T) {
	greenGate := &fakeGate{result: GateResult{Passed: true}}
	redGate := &fakeGate{result: GateResult{Passed: false, Steps: []GateStep{{Name: "test", Passed: false}}}}

	tests := []struct {
		name   string
		gate   GateChecker
		git    GitRunner
		wantOK bool
	}{
		{"gates green, dirty worktree", greenGate, &fakeDiffGit{status: " M foo.go"}, true},
		{"gates green, committed diff", greenGate, &fakeDiffGit{diff: "foo.go"}, true},
		{"gates green, empty diff", greenGate, &fakeDiffGit{}, false},
		{"gates red, dirty worktree", redGate, &fakeDiffGit{status: " M foo.go"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stages := RequirementsFirstPipeline(PipelineDeps{Gate: tt.gate, Git: tt.git, Clock: newTestClock()})
			impl := stages[4]
			workDir := t.TempDir()
			featureDir := filepath.Join(".codexspec", "specs", "2026-0610-1200ab-feature")
			writeArtifact(t, workDir, featureDir, "tasks.md", "- [x] Task 1\n")
			sc := &StageContext{WorkDir: workDir, FeatureDir: featureDir, BaseBranch: "main"}

			ok, gap := impl.Verify(context.Background(), sc)
			if ok != tt.wantOK {
				t.Errorf("Verify = %v (gap %q), want %v (TC-010/TC-026)", ok, gap, tt.wantOK)
			}
		})
	}
}

func TestImplementVerifyRejectsUncheckedTasks(t *testing.T) {
	stages := RequirementsFirstPipeline(PipelineDeps{
		Gate:  &fakeGate{result: GateResult{Passed: true}},
		Git:   &fakeDiffGit{status: " M code.go"},
		Clock: newTestClock(),
	})
	impl := stages[4]
	workDir := t.TempDir()
	featureDir := filepath.Join(".codexspec", "specs", "2026-0610-1200ab-feature")
	writeArtifact(t, workDir, featureDir, "tasks.md", "- [x] Done\n- [ ] Still open\n")

	ok, gap := impl.Verify(context.Background(), &StageContext{
		WorkDir:    workDir,
		FeatureDir: featureDir,
		BaseBranch: "main",
	})
	if ok || !strings.Contains(gap, "unchecked") {
		t.Fatalf("Verify = %v, gap %q, want unchecked task failure", ok, gap)
	}
}
