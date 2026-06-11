package autodev

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func (f *fakeCore) StagePrompt(command, args string) (string, error) {
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

func TestLeanPipelineOrder(t *testing.T) {
	stages := LeanPipeline(PipelineDeps{})
	want := []string{"generate-spec", "spec-to-plan", "plan-to-tasks", "implement-tasks"}
	if len(stages) != len(want) {
		t.Fatalf("len(stages) = %d, want %d", len(stages), len(want))
	}
	for i, name := range want {
		if stages[i].Name != name {
			t.Errorf("stages[%d].Name = %q, want %q (REQ-009)", i, stages[i].Name, name)
		}
		if stages[i].Command != "codexspec:"+name {
			t.Errorf("stages[%d].Command = %q, want codexspec command", i, stages[i].Command)
		}
	}
}

func TestGenerateSpecSeedEmbedsDescription(t *testing.T) {
	workDir := t.TempDir()
	specDir := filepath.Join(workDir, ".codexspec", "specs", "feat")
	core := &fakeCore{workDir: workDir, effects: []func(){func() {
		if err := os.MkdirAll(specDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("# Spec"), 0o644); err != nil {
			t.Fatal(err)
		}
	}}}
	machine := newTestMachine(&reviewingEngineer{})

	sc := &StageContext{
		WorkDir: workDir,
		Item:    Item{Description: "Persist durable discoveries to MEMORY.md."},
	}
	gen := LeanPipeline(PipelineDeps{})[0]
	if err := machine.RunStep(context.Background(), core, sc, gen); err != nil {
		t.Fatalf("RunStep returned error: %v", err)
	}
	if len(core.prompts) != 1 {
		t.Fatalf("core runs = %d, want 1", len(core.prompts))
	}
	if !strings.Contains(core.prompts[0], "PROMPT[codexspec:generate-spec|") {
		t.Errorf("seed prompt = %q, want materialized generate-spec body", core.prompts[0])
	}
	if !strings.Contains(core.prompts[0], "Persist durable discoveries") {
		t.Errorf("seed prompt = %q, want the item Description embedded (REQ-010)", core.prompts[0])
	}
}

func TestGenerateSpecVerifyBindsNewSpecDir(t *testing.T) {
	workDir := t.TempDir()
	specsRoot := filepath.Join(workDir, ".codexspec", "specs")
	if err := os.MkdirAll(filepath.Join(specsRoot, "old-feature"), 0o755); err != nil {
		t.Fatal(err)
	}

	stages := LeanPipeline(PipelineDeps{})
	gen := stages[0]
	sc := &StageContext{WorkDir: workDir}

	if gen.Prepare != nil {
		if err := gen.Prepare(context.Background(), sc); err != nil {
			t.Fatalf("Prepare returned error: %v", err)
		}
	}

	if ok, _ := gen.Verify(context.Background(), sc); ok {
		t.Fatal("Verify passed before any spec dir was created")
	}

	newDir := filepath.Join(specsRoot, "2026-0610-new-feature")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "spec.md"), []byte("# Spec\ncontent"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, gap := gen.Verify(context.Background(), sc)
	if !ok {
		t.Fatalf("Verify failed after spec.md created: %s", gap)
	}
	if sc.SpecDir == "" || !strings.Contains(sc.SpecDir, "2026-0610-new-feature") {
		t.Errorf("SpecDir = %q, want bound to the new directory (TC-019)", sc.SpecDir)
	}

	// Subsequent stages must thread the bound dir through their args.
	planArgs := stages[1].Args(sc)
	if !strings.Contains(planArgs, sc.SpecDir) {
		t.Errorf("spec-to-plan args = %q, want bound spec dir threaded (TC-019)", planArgs)
	}
}

func TestGenerateSpecVerifyRejectsEmptySpec(t *testing.T) {
	workDir := t.TempDir()
	newDir := filepath.Join(workDir, ".codexspec", "specs", "feat")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "spec.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	gen := LeanPipeline(PipelineDeps{})[0]
	sc := &StageContext{WorkDir: workDir, PreexistingSpecDirs: map[string]bool{}}

	if ok, _ := gen.Verify(context.Background(), sc); ok {
		t.Error("Verify passed with an empty spec.md, want fail (REQ-012)")
	}
}

func TestPlanAndTasksVerifyRequireArtifacts(t *testing.T) {
	workDir := t.TempDir()
	specDir := filepath.Join(".codexspec", "specs", "feat")
	if err := os.MkdirAll(filepath.Join(workDir, specDir), 0o755); err != nil {
		t.Fatal(err)
	}

	stages := LeanPipeline(PipelineDeps{})
	sc := &StageContext{WorkDir: workDir, SpecDir: specDir}

	if ok, _ := stages[1].Verify(context.Background(), sc); ok {
		t.Error("spec-to-plan Verify passed without plan.md")
	}
	if err := os.WriteFile(filepath.Join(workDir, specDir, "plan.md"), []byte("# Plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, gap := stages[1].Verify(context.Background(), sc); !ok {
		t.Errorf("spec-to-plan Verify failed with plan.md present: %s", gap)
	}

	if ok, _ := stages[2].Verify(context.Background(), sc); ok {
		t.Error("plan-to-tasks Verify passed without tasks.md")
	}
	if err := os.WriteFile(filepath.Join(workDir, specDir, "tasks.md"), []byte("# Tasks"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, gap := stages[2].Verify(context.Background(), sc); !ok {
		t.Errorf("plan-to-tasks Verify failed with tasks.md present: %s", gap)
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
			stages := LeanPipeline(PipelineDeps{Gate: tt.gate, Git: tt.git})
			impl := stages[3]
			sc := &StageContext{WorkDir: t.TempDir(), BaseBranch: "main"}

			ok, gap := impl.Verify(context.Background(), sc)
			if ok != tt.wantOK {
				t.Errorf("Verify = %v (gap %q), want %v (TC-010/TC-026)", ok, gap, tt.wantOK)
			}
		})
	}
}
