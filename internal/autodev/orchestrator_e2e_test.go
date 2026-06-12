package autodev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// e2eItemState is the simulated ground truth for one item's worktree,
// mutated by the fake core Agent and observed by the fake git/gh runners.
type e2eItemState struct {
	specDir   string
	dirty     bool
	staged    bool
	committed bool
	pushed    bool
}

// e2eWorld holds the shared simulation state across the fakes.
type e2eWorld struct {
	mu        sync.Mutex
	t         *testing.T
	repoRoot  string
	byWorkDir map[string]*e2eItemState
	issues    map[string]int
	prs       map[string]int
	prBodies  map[string]string
	issueSeq  int
	prSeq     int

	ledgerStatusAtSpec []Status
}

func (w *e2eWorld) state(dir string) *e2eItemState {
	if w.byWorkDir[dir] == nil {
		w.byWorkDir[dir] = &e2eItemState{}
	}
	return w.byWorkDir[dir]
}

// e2eGit simulates git: worktree add/remove manage real directories so the
// SDD artifact verifies can use the filesystem, and the read-only queries
// answer from the per-worktree state.
type e2eGit struct{ world *e2eWorld }

func (g *e2eGit) Run(ctx context.Context, dir string, args ...string) (string, error) {
	w := g.world
	w.mu.Lock()
	defer w.mu.Unlock()
	switch args[0] {
	case "rev-parse":
		if len(args) > 1 && args[1] == "--is-inside-work-tree" {
			return "true\n", nil
		}
		if w.state(dir).pushed || w.state(dir).committed {
			return "tip-" + filepath.Base(dir) + "\n", nil
		}
		return "base\n", nil
	case "worktree":
		switch args[1] {
		case "add":
			path := args[3]
			if args[2] != "-b" {
				path = args[2]
			}
			if err := os.MkdirAll(path, 0o755); err != nil {
				return "", err
			}
			return "", nil
		case "remove":
			return "", os.RemoveAll(args[len(args)-1])
		}
	case "status":
		st := w.state(dir)
		if st.dirty || st.staged {
			return " M code.go\n", nil
		}
		return "", nil
	case "diff":
		st := w.state(dir)
		if len(args) > 1 && args[1] == "--cached" {
			if st.staged {
				return "code.go\n", nil
			}
			return "", nil
		}
		if st.committed {
			return "code.go\n", nil
		}
		return "", nil
	case "rev-list":
		if w.state(dir).committed {
			return "1\n", nil
		}
		return "0\n", nil
	case "ls-remote":
		if w.state(dir).pushed {
			return "tip-" + filepath.Base(dir) + "\trefs/heads/x\n", nil
		}
		return "", nil
	}
	return "", fmt.Errorf("unexpected git args %v", args)
}

// e2eExec simulates the gate commands and read-only gh queries.
type e2eExec struct{ world *e2eWorld }

func (e *e2eExec) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	w := e.world
	w.mu.Lock()
	defer w.mu.Unlock()
	if name == "go" || name == "gofmt" {
		return "", nil
	}
	if name != "gh" {
		return "", errors.New("unexpected command " + name)
	}
	switch args[0] {
	case "auth":
		return "Logged in", nil
	case "issue":
		title := ""
		for i, a := range args {
			if a == "--search" && i+1 < len(args) {
				title = args[i+1]
			}
		}
		if n, ok := w.issues[title]; ok {
			return fmt.Sprintf(`[{"number":%d,"title":%q}]`, n, title), nil
		}
		return "[]", nil
	case "pr":
		branch := args[2]
		if n, ok := w.prs[branch]; ok {
			return fmt.Sprintf(`{"number":%d,"body":%q}`, n, w.prBodies[branch]), nil
		}
		return "no pull requests found", errors.New("exit status 1")
	}
	return "", errors.New("unexpected gh args")
}

// e2eCore simulates the core Agent: it reacts to each seeded prompt by
// producing the step's real ground truth (files on disk or state flags).
type e2eCore struct {
	world   *e2eWorld
	workDir string
	title   string
	branch  string
	issueN  *int
	asker   tools.UserAsker
	runs    int
}

// maxE2ERuns is a fail-fast fuse: the happy-path script needs nine runs per
// item, so far more signals a prompt-matching bug that would otherwise hang
// the unbounded RunStep loop.
const maxE2ERuns = 60

func (c *e2eCore) Run(ctx context.Context, prompt string, r engine.Reporter) (*engine.RunResult, error) {
	w := c.world
	w.mu.Lock()
	defer w.mu.Unlock()
	c.runs++
	if c.runs > maxE2ERuns {
		return nil, fmt.Errorf("e2e core exceeded %d runs; last prompt: %.120s", maxE2ERuns, prompt)
	}
	st := w.state(c.workDir)
	lower := strings.ToLower(prompt)
	// Match the most specific step markers first: the PR step's appended
	// instructions mention the linked issue, so "codexspec:pr" must win
	// over the bare "issue" fallback.
	switch {
	case strings.Contains(lower, "generate-spec"):
		w.ledgerStatusAtSpec = append(w.ledgerStatusAtSpec, ledgerStatusOnDisk(w.t, w.repoRoot, slugFromWorkDir(c.workDir)))
		st.specDir = filepath.Join(c.workDir, ".codexspec", "specs", "feat-"+filepath.Base(c.workDir))
		mustWrite(w.t, filepath.Join(st.specDir, "spec.md"), "# Spec")
	case strings.Contains(lower, "spec-to-plan"):
		mustWrite(w.t, filepath.Join(st.specDir, "plan.md"), "# Plan")
	case strings.Contains(lower, "plan-to-tasks"):
		mustWrite(w.t, filepath.Join(st.specDir, "tasks.md"), "# Tasks")
	case strings.Contains(lower, "implement-tasks"):
		mustWrite(w.t, filepath.Join(c.workDir, "code.go"), "package main")
		st.dirty = true
	case strings.Contains(lower, "git add"):
		st.staged = true
	case strings.Contains(lower, "commit-staged"):
		if st.staged {
			st.staged = false
			st.dirty = false
			st.committed = true
		}
	case strings.Contains(lower, "git push"):
		st.pushed = true
	case strings.Contains(lower, "codexspec:pr"):
		w.prSeq++
		w.prs[c.branch] = 1000 + w.prSeq
		w.prBodies[c.branch] = fmt.Sprintf("Implements %s.\n\nCloses #%d", c.title, *c.issueN)
	case strings.Contains(lower, "gh issue create"):
		w.issueSeq++
		w.issues[c.title] = w.issueSeq
		*c.issueN = w.issueSeq
	}
	return &engine.RunResult{FinalMessage: "done"}, nil
}

func (c *e2eCore) SetUserAsker(a tools.UserAsker) { c.asker = a }
func (c *e2eCore) SetModel(model string) error    { return nil }
func (c *e2eCore) WorkDir() string                { return c.workDir }
func (c *e2eCore) StagePrompt(ctx context.Context, command, args string) (string, error) {
	return fmt.Sprintf("PROMPT[%s|%s]", command, args), nil
}

// e2eCoreFactory builds one e2eCore per worktree, deriving the item title
// from the worktree slug the way the orchestrator names them.
type e2eCoreFactory struct {
	world  *e2eWorld
	titles map[string]string
	cores  []*e2eCore
}

func (f *e2eCoreFactory) New(ctx context.Context, workDir, model string) (CoreRunner, error) {
	slug := slugFromWorkDir(workDir)
	issueN := 0
	core := &e2eCore{
		world:   f.world,
		workDir: workDir,
		title:   f.titles[slug],
		branch:  "auto/" + slug,
		issueN:  &issueN,
	}
	f.cores = append(f.cores, core)
	return core, nil
}

func slugFromWorkDir(workDir string) string { return filepath.Base(workDir) }

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ledgerStatusOnDisk reads the persisted ledger and returns slug's status,
// proving durable state transitions mid-run.
func ledgerStatusOnDisk(t *testing.T, repoRoot, slug string) Status {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"))
	if err != nil {
		return ""
	}
	var file struct {
		Items []struct {
			Slug   string `json:"slug"`
			Status Status `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return ""
	}
	for _, it := range file.Items {
		if it.Slug == slug {
			return it.Status
		}
	}
	return ""
}

func TestOrchestratorEndToEndDrainsBacklog(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "BACKLOG.md"), []byte(twoItemBacklog), 0o644); err != nil {
		t.Fatal(err)
	}

	world := &e2eWorld{
		t:         t,
		repoRoot:  repoRoot,
		byWorkDir: map[string]*e2eItemState{},
		issues:    map[string]int{},
		prs:       map[string]int{},
		prBodies:  map[string]string{},
	}
	factory := &e2eCoreFactory{
		world: world,
		titles: map[string]string{
			"first-item":  "First item",
			"second-item": "Second item",
		},
	}
	recorder := newEventRecorder()

	deps := Deps{
		Config:      defaultConfig(repoRoot),
		RepoRoot:    repoRoot,
		CoreFactory: factory,
		Engineer:    &reviewingEngineer{},
		Git:         &e2eGit{world: world},
		Exec:        &e2eExec{world: world},
		Reporter:    recorder,
		Clock:       newTestClock(),
	}

	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Serial transitions: item 2 starts only after item 1 is done (TC-018).
	var itemEvents []string
	for _, e := range recorder.list() {
		if strings.HasPrefix(e, "start:") || strings.HasPrefix(e, "done:") {
			itemEvents = append(itemEvents, e)
		}
	}
	want := []string{"start:first-item", "done:first-item", "start:second-item", "done:second-item"}
	if strings.Join(itemEvents, ",") != strings.Join(want, ",") {
		t.Errorf("item events = %v, want %v", itemEvents, want)
	}

	// The ledger went pending → in-progress (persisted before the first
	// stage ran) → done.
	for i, status := range world.ledgerStatusAtSpec {
		if status != StatusInProgress {
			t.Errorf("ledger status during item %d generate-spec = %q, want in-progress", i+1, status)
		}
	}

	led, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"first-item", "second-item"} {
		it, ok := led.Get(slug)
		if !ok {
			t.Fatalf("ledger missing %s", slug)
		}
		if it.Status != StatusDone {
			t.Errorf("%s status = %q, want done", slug, it.Status)
		}
		if it.Issue == 0 || it.PR == 0 {
			t.Errorf("%s issue/pr = %d/%d, want recorded (Story 1)", slug, it.Issue, it.PR)
		}
		if it.SpecDir == "" {
			t.Errorf("%s SpecDir empty, want bound spec dir recorded (REQ-011)", slug)
		}
		if it.Branch != "auto/"+slug {
			t.Errorf("%s branch = %q, want auto/%s", slug, it.Branch, slug)
		}
	}

	// Worktrees were created during the run and removed after success.
	worktreeRoot := filepath.Join(filepath.Dir(repoRoot), filepath.Base(repoRoot)+"-worktrees")
	for _, slug := range []string{"first-item", "second-item"} {
		if dirExists(filepath.Join(worktreeRoot, slug)) {
			t.Errorf("worktree %s still exists, want removed after PR (TC-014)", slug)
		}
	}

	// Each item got its own core runner with the engineer asker installed.
	if len(factory.cores) != 2 {
		t.Fatalf("core runners = %d, want 2", len(factory.cores))
	}
	for _, core := range factory.cores {
		if core.asker == nil {
			t.Error("core runner missing the EngineerAsker (REQ-013)")
		}
	}

	// PR bodies link their issues (TC-012).
	for branch, body := range world.prBodies {
		issueN := 0
		for _, n := range world.issues {
			if strconv.Itoa(n) != "" && strings.Contains(body, fmt.Sprintf("Closes #%d", n)) {
				issueN = n
			}
		}
		if issueN == 0 {
			t.Errorf("PR body for %s = %q, want a Closes #N link", branch, body)
		}
	}
}
