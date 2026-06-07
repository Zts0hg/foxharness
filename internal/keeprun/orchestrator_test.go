package keeprun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// --- fakes -----------------------------------------------------------------

type recordedCall struct {
	command string
	review  bool
	instr   string
	specDir string
}

type fakeRun struct {
	calls []recordedCall
	fn    func(req PhaseRequest, n int) (PhaseOutcome, error)
}

func (f *fakeRun) RunPhase(_ context.Context, req PhaseRequest) (PhaseOutcome, error) {
	n := len(f.calls)
	f.calls = append(f.calls, recordedCall{req.Phase.Command, req.Phase.Review, req.Instruction, req.SpecDir})
	if f.fn != nil {
		return f.fn(req, n)
	}
	return PhaseOutcome{Output: "ok"}, nil
}

func (f *fakeRun) commands() []string {
	out := make([]string, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.command
	}
	return out
}

type fakeVerify struct {
	counts map[string]int
	fn     func(phase Phase, n int) error
}

func (f *fakeVerify) VerifyPhase(_ context.Context, phase Phase, _ TaskContext, _ PhaseOutcome) error {
	if f.counts == nil {
		f.counts = map[string]int{}
	}
	n := f.counts[phase.Command]
	f.counts[phase.Command]++
	if f.fn != nil {
		return f.fn(phase, n)
	}
	return nil
}

type fakeWT struct {
	repoDir  string
	branches []string
	created  int
	removed  int
	specDir  string
}

func (f *fakeWT) ResolveSpecDir(context.Context, string) (string, error) { return f.specDir, nil }
func (f *fakeWT) DefaultBranch(context.Context) (string, error)          { return "main", nil }
func (f *fakeWT) ListBranches(context.Context) ([]string, error) {
	return append([]string(nil), f.branches...), nil
}
func (f *fakeWT) HeadCommit(context.Context, string) (string, error) {
	return "headbefore", nil
}
func (f *fakeWT) Create(_ context.Context, slug, _ string) (string, error) {
	dir := filepath.Join(f.repoDir, ".claude", "worktrees", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f.branches = append(f.branches, worktreeBranchPrefix+slug)
	f.created++
	return dir, nil
}
func (f *fakeWT) Remove(_ context.Context, dir string) error {
	f.removed++
	return os.RemoveAll(dir)
}

type capSink struct{ events []ProgressEvent }

func (c *capSink) Event(ev ProgressEvent) { c.events = append(c.events, ev) }
func (c *capSink) count(k ProgressKind) int {
	n := 0
	for _, e := range c.events {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func noSleep(context.Context, time.Duration) error { return nil }

func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupRepo(t *testing.T, backlog, config string) string {
	t.Helper()
	repo := t.TempDir()
	writeFileT(t, filepath.Join(repo, "BACKLOG.md"), backlog)
	if config != "" {
		writeFileT(t, filepath.Join(repo, "keep-run.config.json"), config)
	}
	return repo
}

const localConfig = `{"remote_enabled": false}`

func oneTask() string {
	return "## [feature] Add dark mode\n**Priority**: high\n**Status**: pending\n**Description**: d\n"
}

// --- tests -----------------------------------------------------------------

func TestOrchestratorBasicPipeline(t *testing.T) {
	repo := setupRepo(t, oneTask(), localConfig)
	runner := &fakeRun{}
	wt := &fakeWT{repoDir: repo}
	sink := &capSink{}
	o := NewOrchestrator(repo, runner,
		WithWorktrees(wt), WithVerifier(&fakeVerify{}),
		WithProgressSink(sink), WithSleeper(noSleep))

	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	want := []string{
		"codexspec:specify", "codexspec:clarify", "codexspec:generate-spec",
		"codexspec:review-spec", "codexspec:spec-to-plan", "codexspec:review-plan",
		"codexspec:plan-to-tasks", "codexspec:review-tasks", "codexspec:implement-tasks",
		"codexspec:review-code", "codexspec:commit-staged",
	}
	if got := runner.commands(); !reflect.DeepEqual(got, want) {
		t.Errorf("phase order:\n got=%v\nwant=%v", got, want)
	}
	if wt.created != 1 || wt.removed != 1 {
		t.Errorf("worktree created=%d removed=%d, want 1/1", wt.created, wt.removed)
	}
	if sink.count(EventPhaseComplete) != 11 {
		t.Errorf("phase-complete events = %d, want 11", sink.count(EventPhaseComplete))
	}
	b, _ := os.ReadFile(filepath.Join(repo, "BACKLOG.md"))
	if !strings.Contains(string(b), "**Status**: done") {
		t.Errorf("BACKLOG not marked done:\n%s", b)
	}
}

func TestOrchestratorResolvesSpecDir(t *testing.T) {
	repo := setupRepo(t, oneTask(), localConfig)
	resolved := filepath.Join(repo, ".claude", "worktrees", "add-dark-mode",
		".codexspec", "specs", "2026-0604-1200zz-add-dark-mode")
	runner := &fakeRun{}
	wt := &fakeWT{repoDir: repo, specDir: resolved}
	o := NewOrchestrator(repo, runner, WithWorktrees(wt), WithVerifier(&fakeVerify{}), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	sawSpecToPlan := false
	for _, c := range runner.calls {
		if c.command == "codexspec:spec-to-plan" {
			sawSpecToPlan = true
			if c.specDir != resolved {
				t.Errorf("spec-to-plan SpecDir = %q, want detected %q", c.specDir, resolved)
			}
		}
		if strings.Contains(c.specDir, filepath.Join("specs", "add-dark-mode")) {
			t.Errorf("phase %s used a constructed bare-slug SpecDir %q; detection must be used instead", c.command, c.specDir)
		}
	}
	if !sawSpecToPlan {
		t.Fatal("spec-to-plan phase did not run")
	}
}

func TestOrchestratorRemoteEnabled(t *testing.T) {
	repo := setupRepo(t, oneTask(), `{"remote_enabled": true}`)
	runner := &fakeRun{}
	o := NewOrchestrator(repo, runner,
		WithWorktrees(&fakeWT{repoDir: repo}), WithVerifier(&fakeVerify{}), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	cmds := runner.commands()
	if len(cmds) != 12 {
		t.Fatalf("phases = %d, want 12", len(cmds))
	}
	if cmds[len(cmds)-1] != "codexspec:pr" {
		t.Errorf("last phase = %s, want codexspec:pr", cmds[len(cmds)-1])
	}
}

func TestOrchestratorResumeFromState(t *testing.T) {
	repo := setupRepo(t, oneTask(), localConfig)
	wtDir := filepath.Join(repo, ".claude", "worktrees", "add-dark-mode")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteState(wtDir, State{
		TaskSlug: "add-dark-mode", WorktreePath: wtDir,
		CompletedPhases: []int{1, 2, 3, 4, 5, 6}, RemoteEnabled: false,
	}); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRun{}
	wt := &fakeWT{repoDir: repo, branches: []string{"keep-run-add-dark-mode"}}
	o := NewOrchestrator(repo, runner, WithWorktrees(wt), WithVerifier(&fakeVerify{}), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	want := []string{
		"codexspec:plan-to-tasks", "codexspec:review-tasks", "codexspec:implement-tasks",
		"codexspec:review-code", "codexspec:commit-staged",
	}
	if got := runner.commands(); !reflect.DeepEqual(got, want) {
		t.Errorf("resume order:\n got=%v\nwant=%v", got, want)
	}
	if wt.created != 0 {
		t.Errorf("expected no worktree Create on resume, got %d", wt.created)
	}
}

func TestOrchestratorVerifyGateRetries(t *testing.T) {
	repo := setupRepo(t, oneTask(), localConfig)
	runner := &fakeRun{}
	verify := &fakeVerify{fn: func(p Phase, n int) error {
		if p.Command == "codexspec:generate-spec" && n == 0 {
			return fmt.Errorf("spec.md missing")
		}
		return nil
	}}
	o := NewOrchestrator(repo, runner, WithWorktrees(&fakeWT{repoDir: repo}), WithVerifier(verify), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	got := 0
	for _, c := range runner.calls {
		if c.command == "codexspec:generate-spec" {
			got++
		}
	}
	if got != 2 {
		t.Errorf("generate-spec runs = %d, want 2 (one retry after gate failure)", got)
	}
}

func TestOrchestratorReviewIteration(t *testing.T) {
	repo := setupRepo(t, oneTask(), localConfig)
	runner := &fakeRun{}
	verify := &fakeVerify{fn: func(p Phase, n int) error {
		if p.Command == "codexspec:review-spec" && n == 0 {
			return fmt.Errorf("not clean")
		}
		return nil
	}}
	o := NewOrchestrator(repo, runner, WithWorktrees(&fakeWT{repoDir: repo}), WithVerifier(verify), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	var rs []recordedCall
	for _, c := range runner.calls {
		if c.command == "codexspec:review-spec" {
			rs = append(rs, c)
		}
	}
	if len(rs) != 3 {
		t.Fatalf("review-spec calls = %d, want 3 (review, fix, review)", len(rs))
	}
	if !rs[0].review || rs[1].review || !rs[2].review {
		t.Errorf("review-iteration pattern = [%v,%v,%v], want [review, fix, review]", rs[0].review, rs[1].review, rs[2].review)
	}
	if !strings.Contains(rs[0].instr, "keep-run-verdict") {
		t.Errorf("review run instruction missing verdict contract: %q", rs[0].instr)
	}
	if !strings.Contains(rs[1].instr, "Fix all") {
		t.Errorf("fix run instruction missing review_fix_prompt: %q", rs[1].instr)
	}
}

func TestOrchestratorMultiTask(t *testing.T) {
	backlog := "## [feature] One\n**Status**: pending\n\n## [fix] Two\n**Status**: pending\n"
	repo := setupRepo(t, backlog, localConfig)
	runner := &fakeRun{}
	wt := &fakeWT{repoDir: repo}
	o := NewOrchestrator(repo, runner, WithWorktrees(wt), WithVerifier(&fakeVerify{}), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if wt.created != 2 || wt.removed != 2 {
		t.Errorf("worktrees created=%d removed=%d, want 2/2", wt.created, wt.removed)
	}
	b, _ := os.ReadFile(filepath.Join(repo, "BACKLOG.md"))
	if strings.Count(string(b), "**Status**: done") != 2 {
		t.Errorf("expected both tasks done:\n%s", b)
	}
}

func TestOrchestratorExitAllDone(t *testing.T) {
	repo := setupRepo(t, "## [feature] Done one\n**Status**: done\n", localConfig)
	runner := &fakeRun{}
	sink := &capSink{}
	o := NewOrchestrator(repo, runner, WithWorktrees(&fakeWT{repoDir: repo}), WithVerifier(&fakeVerify{}), WithProgressSink(sink), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("no phases should run when all done, got %d", len(runner.calls))
	}
	if sink.count(EventExit) != 1 {
		t.Errorf("expected one exit event, got %d", sink.count(EventExit))
	}
}

func TestOrchestratorEmptyBacklog(t *testing.T) {
	repo := setupRepo(t, "# Backlog\n\nnothing here\n", localConfig)
	runner := &fakeRun{}
	o := NewOrchestrator(repo, runner, WithWorktrees(&fakeWT{repoDir: repo}), WithVerifier(&fakeVerify{}), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("no phases should run for empty backlog, got %d", len(runner.calls))
	}
}

func TestOrchestratorMissingBacklog(t *testing.T) {
	repo := t.TempDir()
	o := NewOrchestrator(repo, &fakeRun{}, WithWorktrees(&fakeWT{repoDir: repo}), WithVerifier(&fakeVerify{}), WithSleeper(noSleep))
	if err := o.Run(context.Background()); err == nil {
		t.Error("missing BACKLOG.md: want error, got nil")
	}
}

func TestOrchestratorContextCancel(t *testing.T) {
	repo := setupRepo(t, oneTask(), localConfig)
	ctx, cancel := context.WithCancel(context.Background())
	runner := &fakeRun{fn: func(PhaseRequest, int) (PhaseOutcome, error) {
		cancel()
		return PhaseOutcome{Output: "ok"}, nil
	}}
	o := NewOrchestrator(repo, runner, WithWorktrees(&fakeWT{repoDir: repo}), WithVerifier(&fakeVerify{}), WithSleeper(noSleep))
	if err := o.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Run after cancel = %v, want context.Canceled", err)
	}
}

// TestOrchestratorDeduplicatesAgainstExistingBranch tests that when a branch
// with the same slug already exists, the orchestrator deduplicates by appending
// a numeric suffix. The branch list from ListBranches includes the
// "keep-run-" prefix, so deduplication must strip this prefix before comparing.
func TestOrchestratorDeduplicatesAgainstExistingBranch(t *testing.T) {
	repo := setupRepo(t, oneTask(), localConfig)
	// Simulate an existing branch with the same slug.
	var createdSlug string
	wt := &fakeWTHook{
		fakeWT: fakeWT{
			repoDir:  repo,
			branches: []string{"keep-run-add-dark-mode"},
		},
		onCreate: func(slug string) {
			createdSlug = slug
		},
	}
	runner := &fakeRun{}
	o := NewOrchestrator(repo, runner, WithWorktrees(wt), WithVerifier(&fakeVerify{}), WithSleeper(noSleep))

	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// The existing branch is "keep-run-add-dark-mode", so the new slug should
	// be deduplicated to "add-dark-mode-2".
	if createdSlug != "add-dark-mode-2" {
		t.Errorf("Create received slug %q, want %q (deduplicated against existing branch)", createdSlug, "add-dark-mode-2")
	}
	if wt.created != 1 {
		t.Errorf("worktree created count = %d, want 1", wt.created)
	}
}

// fakeWTHook wraps fakeWT to allow hooking into Create for testing.
type fakeWTHook struct {
	fakeWT
	onCreate func(slug string)
}

func (f *fakeWTHook) Create(ctx context.Context, slug, base string) (string, error) {
	if f.onCreate != nil {
		f.onCreate(slug)
	}
	return f.fakeWT.Create(ctx, slug, base)
}
