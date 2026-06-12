package autodev

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGit records GitRunner invocations and replays scripted outputs.
type fakeGit struct {
	calls   []string
	dirs    []string
	outputs map[string]string
	errs    map[string]error
}

func (f *fakeGit) Run(ctx context.Context, dir string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	f.dirs = append(f.dirs, dir)
	if f.errs != nil {
		if err, ok := f.errs[key]; ok {
			return f.outputs[key], err
		}
	}
	return f.outputs[key], nil
}

func TestWorktreeCreateAddsBranchInSiblingDir(t *testing.T) {
	repoRoot := t.TempDir()
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "engine-memory", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	wantPath := filepath.Join(filepath.Dir(repoRoot), "wt", "engine-memory")
	if wt.Path != wantPath {
		t.Errorf("Path = %q, want %q (sibling dir, REQ-005)", wt.Path, wantPath)
	}
	if wt.Branch != "auto/engine-memory" {
		t.Errorf("Branch = %q, want auto/engine-memory", wt.Branch)
	}
	if wt.Resumed {
		t.Error("Resumed = true for a fresh worktree, want false")
	}

	if len(git.calls) != 1 {
		t.Fatalf("git calls = %v, want exactly one worktree add", git.calls)
	}
	want := "worktree add -b auto/engine-memory " + wantPath + " main"
	if git.calls[0] != want {
		t.Errorf("git call = %q, want %q (TC-004)", git.calls[0], want)
	}
	if git.dirs[0] != repoRoot {
		t.Errorf("git dir = %q, want repo root %q", git.dirs[0], repoRoot)
	}
}

func TestWorktreeCreateResumesExistingInProgress(t *testing.T) {
	repoRoot := t.TempDir()
	wtPath := filepath.Join(filepath.Dir(repoRoot), "wt-resume", "engine-memory")
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		"worktree list --porcelain": porcelainList(repoRoot, "main", wtPath, "auto/engine-memory"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-resume", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "engine-memory",
		Status: StatusInProgress,
		Branch: "auto/engine-memory",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !wt.Resumed {
		t.Error("Resumed = false, want true for an existing in-progress worktree")
	}
	if wt.Path != wtPath {
		t.Errorf("Path = %q, want existing %q", wt.Path, wtPath)
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree add") {
			t.Errorf("git ran %q, want no worktree add when resuming an existing worktree", call)
		}
	}
}

func TestWorktreeCreateReattachesExistingBranchWithoutDir(t *testing.T) {
	repoRoot := t.TempDir()
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-reattach", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "engine-memory",
		Status: StatusInProgress,
		Branch: "auto/engine-memory",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	want := "worktree add " + wt.Path + " auto/engine-memory"
	if len(git.calls) == 0 || git.calls[len(git.calls)-1] != want {
		t.Errorf("git calls = %v, want %q last (reuse the recorded branch)", git.calls, want)
	}
}

func TestWorktreeCreateSuffixesLeftoverPath(t *testing.T) {
	repoRoot := t.TempDir()
	base := filepath.Join(filepath.Dir(repoRoot), "wt-leftover")
	if err := os.MkdirAll(filepath.Join(base, "engine-memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-leftover", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "engine-memory", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Path == filepath.Join(base, "engine-memory") {
		t.Error("Path reused a leftover directory, want a uniquely-suffixed path")
	}
	if !strings.HasPrefix(filepath.Base(wt.Path), "engine-memory-") {
		t.Errorf("Path = %q, want suffixed engine-memory-N", wt.Path)
	}
	if want := "auto/" + filepath.Base(wt.Path); wt.Branch != want {
		t.Errorf("Branch = %q, want %q (suffixed in lockstep with the path)", wt.Branch, want)
	}
}

func TestWorktreeCreateReusesLeftoverWorktreeOnItemBranch(t *testing.T) {
	repoRoot := t.TempDir()
	base := filepath.Join(filepath.Dir(repoRoot), "wt-reuse")
	leftover := filepath.Join(base, "engine-memory")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		"worktree list --porcelain": porcelainList(repoRoot, "main", leftover, "auto/engine-memory"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-reuse", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "engine-memory", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !wt.Resumed {
		t.Error("Resumed = false, want true when the leftover dir is a worktree on the item's branch")
	}
	if wt.Path != leftover {
		t.Errorf("Path = %q, want the leftover worktree %q reused", wt.Path, leftover)
	}
	if wt.Branch != "auto/engine-memory" {
		t.Errorf("Branch = %q, want auto/engine-memory", wt.Branch)
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree add") {
			t.Errorf("git ran %q, want no worktree add when reusing the leftover worktree", call)
		}
	}
}

func TestWorktreeCreateSuffixesBranchWhenBranchExistsWithoutDir(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-stale")
	git := &fakeGit{
		outputs: map[string]string{},
		errs: map[string]error{
			"worktree add -b auto/x " + filepath.Join(root, "x") + " main": errors.New("fatal: a branch named 'auto/x' already exists"),
		},
	}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-stale", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v, want fallback to a suffixed branch", err)
	}
	if wt.Branch != "auto/x-2" {
		t.Errorf("Branch = %q, want auto/x-2 (lockstep-suffixed past the stale branch)", wt.Branch)
	}
	wantPath := filepath.Join(root, "x-2")
	if wt.Path != wantPath {
		t.Errorf("Path = %q, want %q (suffixed in lockstep with the branch)", wt.Path, wantPath)
	}
	if wt.Resumed {
		t.Error("Resumed = true, want false for a freshly created suffixed worktree")
	}
	wantAdd := "worktree add -b auto/x-2 " + wantPath + " main"
	found := false
	for _, call := range git.calls {
		if call == wantAdd {
			found = true
		}
	}
	if !found {
		t.Errorf("git calls = %v, want %q", git.calls, wantAdd)
	}
}

// porcelainList renders pairs of (path, branch) in the `git worktree list
// --porcelain` wire format the manager parses.
func porcelainList(pairs ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		b.WriteString("worktree " + pairs[i] + "\nHEAD 0000000000000000000000000000000000000000\nbranch refs/heads/" + pairs[i+1] + "\n\n")
	}
	return b.String()
}

func TestWorktreeCreateResumesSuffixedBranchAtItsWorktree(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-suffixed")
	suffixed := filepath.Join(root, "x-2")
	if err := os.MkdirAll(suffixed, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		"worktree list --porcelain": porcelainList(repoRoot, "main", suffixed, "auto/x-2"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-suffixed", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "x",
		Status: StatusInProgress,
		Branch: "auto/x-2",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !wt.Resumed || wt.Path != suffixed || wt.Branch != "auto/x-2" {
		t.Errorf("got %+v, want the suffixed worktree %q on auto/x-2 reused", wt, suffixed)
	}
	for _, call := range git.calls {
		if strings.HasPrefix(call, "worktree add") {
			t.Errorf("git ran %q, want no worktree add when the branch is already checked out", call)
		}
	}
}

func TestWorktreeCreatePrunesStaleRegistrationBeforeReattach(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-pruned")
	gone := filepath.Join(root, "x-2")
	git := &fakeGit{outputs: map[string]string{
		// The registry still lists the worktree, but its directory is gone
		// (removed without `git worktree remove`).
		"worktree list --porcelain": porcelainList(repoRoot, "main", gone, "auto/x-2"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-pruned", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "x",
		Status: StatusInProgress,
		Branch: "auto/x-2",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Path != gone || wt.Branch != "auto/x-2" || !wt.Resumed {
		t.Errorf("got %+v, want auto/x-2 reattached at %q", wt, gone)
	}
	want := []string{
		"worktree list --porcelain",
		"worktree prune",
		"worktree add " + gone + " auto/x-2",
	}
	if strings.Join(git.calls, "; ") != strings.Join(want, "; ") {
		t.Errorf("git calls = %v, want %v (prune the stale record, then reattach)", git.calls, want)
	}
}

func TestWorktreeCreateIgnoresForeignWorktreeOnSameBranchName(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-foreign")
	foreign := filepath.Join(root, "x")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		// A worktree of some other repository sits at the candidate path and
		// happens to have an identically named branch checked out, so a local
		// rev-parse would report auto/x — but our registry does not list it.
		"rev-parse --abbrev-ref HEAD": "auto/x\n",
		"worktree list --porcelain":   porcelainList(repoRoot, "main"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-foreign", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Resumed {
		t.Error("Resumed = true, want false: a foreign repository's worktree must never be reused")
	}
	if wt.Path == foreign {
		t.Errorf("Path = %q reused the foreign worktree, want a fresh suffixed path", wt.Path)
	}
	if wt.Branch != "auto/x-2" || wt.Path != filepath.Join(root, "x-2") {
		t.Errorf("got %+v, want a fresh auto/x-2 worktree at %q", wt, filepath.Join(root, "x-2"))
	}
}

func TestWorktreeCreateResumeRejectsPrimaryCheckout(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-primary")
	git := &fakeGit{outputs: map[string]string{
		// The user manually checked the item's branch out in the primary
		// repository; the registry therefore lists repoRoot on auto/x-2.
		"worktree list --porcelain": porcelainList(repoRoot, "auto/x-2"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-primary", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "x",
		Status: StatusInProgress,
		Branch: "auto/x-2",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if samePath(wt.Path, repoRoot) {
		t.Fatalf("Path = %q resumed into the primary checkout, want it rejected (worktree isolation)", wt.Path)
	}
	want := "worktree add " + filepath.Join(root, "x-2") + " auto/x-2"
	if len(git.calls) == 0 || git.calls[len(git.calls)-1] != want {
		t.Errorf("git calls = %v, want %q last (reattach inside the managed root)", git.calls, want)
	}
}

func TestWorktreeCreateSuffixesPastPublishedLeftover(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-published")
	leftover := filepath.Join(root, "x")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		// A completed run whose Remove failed: registered on auto/x, clean,
		// and its HEAD already published on the remote.
		"worktree list --porcelain":       porcelainList(repoRoot, "main", leftover, "auto/x"),
		"status --porcelain":              "",
		"rev-parse HEAD":                  "abc123\n",
		"ls-remote --heads origin auto/x": "abc123\trefs/heads/auto/x\n",
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-published", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Resumed || wt.Path == leftover {
		t.Errorf("got %+v, want the published leftover left alone (never rerun onto a pushed PR branch)", wt)
	}
	if wt.Branch != "auto/x-2" || wt.Path != filepath.Join(root, "x-2") {
		t.Errorf("got %+v, want a fresh auto/x-2 worktree at %q", wt, filepath.Join(root, "x-2"))
	}
}

func TestWorktreeCreateSkipsLeftoverWhenRemoteUnknown(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-unknown")
	leftover := filepath.Join(root, "x")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{
		outputs: map[string]string{
			// A clean leftover registered on auto/x while the remote cannot
			// be queried: published or crashed is undecidable, and resuming a
			// published worktree would push new commits onto its PR branch.
			"worktree list --porcelain": porcelainList(repoRoot, "main", leftover, "auto/x"),
			"status --porcelain":        "",
			"rev-parse HEAD":            "abc123\n",
		},
		errs: map[string]error{
			"ls-remote --heads origin auto/x": errors.New("fatal: could not read from remote repository"),
		},
	}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-unknown", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Resumed || wt.Path == leftover {
		t.Errorf("got %+v, want the undecidable leftover skipped, not resumed", wt)
	}
	if wt.Branch != "auto/x-2" || wt.Path != filepath.Join(root, "x-2") {
		t.Errorf("got %+v, want a fresh auto/x-2 worktree at %q", wt, filepath.Join(root, "x-2"))
	}
}

func TestWorktreeCreateReusesDirtyLeftoverWhenRemoteUnknown(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-dirty")
	leftover := filepath.Join(root, "x")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{
		outputs: map[string]string{
			// A dirty tree is unpublished work by definition: the crashed run
			// must be resumed even when the remote is unreachable.
			"worktree list --porcelain": porcelainList(repoRoot, "main", leftover, "auto/x"),
			"status --porcelain":        " M main.go\n",
		},
		errs: map[string]error{
			"ls-remote --heads origin auto/x": errors.New("fatal: could not read from remote repository"),
		},
	}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-dirty", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if !wt.Resumed || wt.Path != leftover {
		t.Errorf("got %+v, want the dirty leftover %q reused", wt, leftover)
	}
}

func TestWorktreeCreateSkipsDirtyButPublishedLeftover(t *testing.T) {
	repoRoot := t.TempDir()
	root := filepath.Join(filepath.Dir(repoRoot), "wt-pubdirty")
	leftover := filepath.Join(root, "x")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		// Inspection debris dirties a published leftover: every commit is on
		// the remote already, so the dirt is post-publication noise, not
		// interrupted pipeline work.
		"worktree list --porcelain":       porcelainList(repoRoot, "main", leftover, "auto/x"),
		"status --porcelain":              "?? debris.txt\n",
		"rev-parse HEAD":                  "abc123\n",
		"ls-remote --heads origin auto/x": "abc123\trefs/heads/auto/x\n",
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-pubdirty", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Resumed || wt.Path == leftover {
		t.Errorf("got %+v, want the published leftover skipped despite the dirty tree", wt)
	}
	if wt.Branch != "auto/x-2" {
		t.Errorf("Branch = %q, want auto/x-2", wt.Branch)
	}
}

func TestWorktreeCreateResumeRejectsPrimaryCheckoutInAncestorRoot(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		// worktree_dir ".." makes the managed root an ancestor of the
		// repository, so a within-root filter alone would admit the primary
		// checkout, which has the item's branch checked out.
		"worktree list --porcelain": porcelainList(repoRoot, "auto/x"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "..", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "x",
		Status: StatusInProgress,
		Branch: "auto/x",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if samePath(wt.Path, repoRoot) {
		t.Fatalf("Path = %q resumed into the primary checkout, want it rejected (worktree isolation)", wt.Path)
	}
	want := "worktree add " + filepath.Join(filepath.Dir(repoRoot), "x") + " auto/x"
	if len(git.calls) == 0 || git.calls[len(git.calls)-1] != want {
		t.Errorf("git calls = %v, want %q last (reattach beside the repository)", git.calls, want)
	}
}

func TestWorktreeCreateNeverReusesPrimaryCheckoutAsCandidate(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		// With worktree_dir ".." and a slug equal to the repository directory
		// name, the first candidate path is the repository itself.
		"worktree list --porcelain": porcelainList(repoRoot, "auto/repo"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "..", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "repo", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Resumed || samePath(wt.Path, repoRoot) {
		t.Fatalf("got %+v, want the primary checkout never reused as a candidate", wt)
	}
	if wt.Branch != "auto/repo-2" {
		t.Errorf("Branch = %q, want auto/repo-2 (suffixed past the repository directory)", wt.Branch)
	}
}

func TestWorktreeCreateResumeRejectsInvocationWorktreeCheckout(t *testing.T) {
	tmp := t.TempDir()
	mainRepo := filepath.Join(tmp, "main-repo")
	repoRoot := filepath.Join(tmp, "caller")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		// autodev was started inside a linked worktree: git lists the main
		// checkout first and the invoking checkout (repoRoot) as a linked
		// entry, so dropping the first record alone would leave repoRoot
		// eligible under an ancestor worktree_dir.
		"worktree list --porcelain": porcelainList(mainRepo, "main", repoRoot, "auto/x"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "..", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{
		Slug:   "x",
		Status: StatusInProgress,
		Branch: "auto/x",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if samePath(wt.Path, repoRoot) {
		t.Fatalf("Path = %q resumed into the invoking checkout, want it rejected (worktree isolation)", wt.Path)
	}
	want := "worktree add " + filepath.Join(tmp, "x") + " auto/x"
	if len(git.calls) == 0 || git.calls[len(git.calls)-1] != want {
		t.Errorf("git calls = %v, want %q last (reattach beside the invoking checkout)", git.calls, want)
	}
}

func TestWorktreeCreateIgnoresSymlinkedExternalWorktree(t *testing.T) {
	repoRoot := t.TempDir()
	parent := filepath.Dir(repoRoot)
	root := filepath.Join(parent, "wt-symlink")
	external := filepath.Join(parent, "external-x")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	// The candidate path inside the managed root is a symlink to a registered
	// worktree that actually lives outside it; following it would hand an
	// external checkout to the core agent.
	if err := os.Symlink(external, filepath.Join(root, "x")); err != nil {
		t.Fatal(err)
	}
	git := &fakeGit{outputs: map[string]string{
		"worktree list --porcelain": porcelainList(repoRoot, "main", external, "auto/x"),
	}}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-symlink", "main", "origin")

	wt, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if wt.Resumed || samePath(wt.Path, external) {
		t.Fatalf("got %+v, want the symlinked external worktree never reused", wt)
	}
	if wt.Branch != "auto/x-2" || wt.Path != filepath.Join(root, "x-2") {
		t.Errorf("got %+v, want a fresh auto/x-2 worktree at %q", wt, filepath.Join(root, "x-2"))
	}
}

// TestWorktreeCreateDegradedScenariosRealGit exercises the two degraded
// startup states against real git — the fake-backed tests above encode git's
// semantics, this one keeps them honest (a prior version of Create passed its
// fakes while real git rejected the branch collision outright).
func TestWorktreeCreateDegradedScenariosRealGit(t *testing.T) {
	requireBinary(t, "git")
	ctx := context.Background()
	git := NewExecGitRunner()

	repoRoot := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit := func(args ...string) string {
		t.Helper()
		out, err := git.Run(ctx, repoRoot, args...)
		if err != nil {
			t.Fatalf("git %s failed: %v (%s)", strings.Join(args, " "), err, out)
		}
		return out
	}
	mustGit("init")
	mustGit("-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "init")
	mustGit("branch", "-M", "main")
	// The production pipeline always pushes, so a remote always exists; the
	// leftover classification below consults it.
	bare := filepath.Join(t.TempDir(), "origin.git")
	mustGit("init", "--bare", bare)
	mustGit("remote", "add", "origin", bare)

	mgr := NewWorktreeManager(git, repoRoot, "../wt", "main", "origin")
	root := filepath.Clean(filepath.Join(repoRoot, "../wt"))

	t.Run("crashed run is reused", func(t *testing.T) {
		// A prior run provisioned branch + worktree but crashed before the
		// ledger recorded in-progress, so the item arrives as pending.
		leftover := filepath.Join(root, "crashed")
		mustGit("worktree", "add", "-b", "auto/crashed", leftover, "main")

		wt, err := mgr.Create(ctx, LedgerItem{Slug: "crashed", Status: StatusPending})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if !wt.Resumed || wt.Path != leftover || wt.Branch != "auto/crashed" {
			t.Errorf("got %+v, want the crashed run's worktree reused (Resumed, %s, auto/crashed)", wt, leftover)
		}
	})

	t.Run("retained branch without dir is suffixed past", func(t *testing.T) {
		// Remove retains branches after success; a reset ledger replays the
		// item against the existing branch with no worktree directory left.
		mustGit("branch", "auto/stale", "main")

		wt, err := mgr.Create(ctx, LedgerItem{Slug: "stale", Status: StatusPending})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if wt.Branch != "auto/stale-2" || wt.Path != filepath.Join(root, "stale-2") || wt.Resumed {
			t.Errorf("got %+v, want a fresh auto/stale-2 worktree at %s", wt, filepath.Join(root, "stale-2"))
		}
		if out, err := git.Run(ctx, wt.Path, "rev-parse", "--abbrev-ref", "HEAD"); err != nil || strings.TrimSpace(out) != "auto/stale-2" {
			t.Errorf("worktree HEAD = %q (err %v), want auto/stale-2 checked out", strings.TrimSpace(out), err)
		}
	})

	t.Run("interrupted suffixed run resumes at its worktree", func(t *testing.T) {
		// Full round trip: a retained branch forces the suffixed pair, the
		// run is interrupted after the ledger recorded the branch, and the
		// restart must come back to the suffixed worktree — not reconstruct
		// the path from the slug.
		mustGit("branch", "auto/res", "main")
		first, err := mgr.Create(ctx, LedgerItem{Slug: "res", Status: StatusPending})
		if err != nil {
			t.Fatalf("degraded Create returned error: %v", err)
		}
		if first.Branch != "auto/res-2" {
			t.Fatalf("degraded Create branch = %q, want auto/res-2", first.Branch)
		}

		resumed, err := mgr.Create(ctx, LedgerItem{
			Slug:   "res",
			Status: StatusInProgress,
			Branch: first.Branch,
		})
		if err != nil {
			t.Fatalf("resume Create returned error: %v", err)
		}
		if !resumed.Resumed || resumed.Branch != "auto/res-2" {
			t.Errorf("got %+v, want auto/res-2 resumed", resumed)
		}
		gotPath, _ := filepath.EvalSymlinks(resumed.Path)
		wantPath, _ := filepath.EvalSymlinks(first.Path)
		if gotPath != wantPath {
			t.Errorf("resumed path = %q, want the interrupted worktree %q", resumed.Path, first.Path)
		}
	})

	t.Run("published leftover is suffixed past", func(t *testing.T) {
		// A completed run whose worktree removal failed: the worktree sits on
		// auto/pub with its tip already pushed. A ledger reset must not rerun
		// the pipeline onto the published PR branch.
		pubPath := filepath.Join(root, "pub")
		mustGit("worktree", "add", "-b", "auto/pub", pubPath, "main")
		if out, err := git.Run(ctx, pubPath, "-c", "user.name=t", "-c", "user.email=t@t",
			"commit", "--allow-empty", "-m", "work"); err != nil {
			t.Fatalf("commit in worktree failed: %v (%s)", err, out)
		}
		if out, err := git.Run(ctx, pubPath, "push", "origin", "auto/pub"); err != nil {
			t.Fatalf("push failed: %v (%s)", err, out)
		}

		wt, err := mgr.Create(ctx, LedgerItem{Slug: "pub", Status: StatusPending})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if wt.Resumed || wt.Branch != "auto/pub-2" {
			t.Errorf("got %+v, want a fresh auto/pub-2 instead of the published leftover", wt)
		}
	})

	t.Run("primary checkout is never resumed", func(t *testing.T) {
		// The user manually checked the in-progress branch out in the primary
		// repository to inspect it; resuming must fail loudly rather than run
		// the agent inside the user's main checkout.
		mustGit("branch", "auto/pri", "main")
		mustGit("checkout", "auto/pri")
		defer mustGit("checkout", "main")

		wt, err := mgr.Create(ctx, LedgerItem{
			Slug:   "pri",
			Status: StatusInProgress,
			Branch: "auto/pri",
		})
		if err == nil {
			t.Fatalf("Create returned %+v, want an error: the branch is checked out in the primary repository", wt)
		}
		if samePath(wt.Path, repoRoot) {
			t.Errorf("Path = %q, the primary checkout must never be handed to the core agent", wt.Path)
		}
	})

	t.Run("unknown remote state skips clean leftover", func(t *testing.T) {
		// A clean leftover on auto/unk while the remote is unreachable:
		// whether it belongs to a published PR is undecidable, so it must be
		// skipped, never resumed.
		unkPath := filepath.Join(root, "unk")
		mustGit("worktree", "add", "-b", "auto/unk", unkPath, "main")
		mustGit("remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
		defer mustGit("remote", "set-url", "origin", bare)

		wt, err := mgr.Create(ctx, LedgerItem{Slug: "unk", Status: StatusPending})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if wt.Resumed || wt.Branch != "auto/unk-2" {
			t.Errorf("got %+v, want a fresh auto/unk-2 instead of the undecidable leftover", wt)
		}
	})

	t.Run("ancestor root never resumes primary checkout", func(t *testing.T) {
		// With worktree_dir ".." the managed root contains the repository
		// itself; resuming a branch checked out in the primary checkout must
		// still fail loudly instead of handing the repository to the agent.
		mustGit("branch", "auto/anc", "main")
		mustGit("checkout", "auto/anc")
		defer mustGit("checkout", "main")
		ancestor := NewWorktreeManager(git, repoRoot, "..", "main", "origin")

		wt, err := ancestor.Create(ctx, LedgerItem{
			Slug:   "anc",
			Status: StatusInProgress,
			Branch: "auto/anc",
		})
		if err == nil {
			t.Fatalf("Create returned %+v, want an error: the branch is checked out in the primary repository", wt)
		}
		if samePath(wt.Path, repoRoot) {
			t.Errorf("Path = %q, the primary checkout must never be handed to the core agent", wt.Path)
		}
	})

	t.Run("dirty published leftover is suffixed past", func(t *testing.T) {
		// Inspection debris (an untracked file) in a published leftover must
		// not reclassify it as interrupted work: its HEAD is already on the
		// remote, so the dirt is post-publication noise.
		pubdPath := filepath.Join(root, "pubd")
		mustGit("worktree", "add", "-b", "auto/pubd", pubdPath, "main")
		if out, err := git.Run(ctx, pubdPath, "-c", "user.name=t", "-c", "user.email=t@t",
			"commit", "--allow-empty", "-m", "work"); err != nil {
			t.Fatalf("commit in worktree failed: %v (%s)", err, out)
		}
		if out, err := git.Run(ctx, pubdPath, "push", "origin", "auto/pubd"); err != nil {
			t.Fatalf("push failed: %v (%s)", err, out)
		}
		if err := os.WriteFile(filepath.Join(pubdPath, "debris.txt"), []byte("inspection debris"), 0o644); err != nil {
			t.Fatal(err)
		}

		wt, err := mgr.Create(ctx, LedgerItem{Slug: "pubd", Status: StatusPending})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if wt.Resumed || wt.Branch != "auto/pubd-2" {
			t.Errorf("got %+v, want a fresh auto/pubd-2 instead of the dirty published leftover", wt)
		}
	})

	t.Run("linked-worktree invocation never resumes its own checkout", func(t *testing.T) {
		// autodev started inside a linked worktree whose checked-out branch is
		// the in-progress item's branch: the invoking checkout must never be
		// handed to the core agent, so Create must fail loudly.
		callerPath := filepath.Join(root, "caller")
		mustGit("worktree", "add", "-b", "auto/self", callerPath, "main")
		invoked := NewWorktreeManager(git, callerPath, "..", "main", "origin")

		wt, err := invoked.Create(ctx, LedgerItem{
			Slug:   "self",
			Status: StatusInProgress,
			Branch: "auto/self",
		})
		if err == nil {
			t.Fatalf("Create returned %+v, want an error: the branch is checked out in the invoking worktree", wt)
		}
		if samePath(wt.Path, callerPath) {
			t.Errorf("Path = %q, the invoking checkout must never be handed to the core agent", wt.Path)
		}
	})
}

func TestWorktreeRemove(t *testing.T) {
	repoRoot := t.TempDir()
	git := &fakeGit{}
	mgr := NewWorktreeManager(git, repoRoot, "../wt", "main", "origin")

	wt := Worktree{Path: "/abs/wt/engine-memory", Branch: "auto/engine-memory"}
	if err := mgr.Remove(context.Background(), wt); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if len(git.calls) != 1 || git.calls[0] != "worktree remove --force /abs/wt/engine-memory" {
		t.Errorf("git calls = %v, want worktree remove (TC-014; the remote branch is untouched)", git.calls)
	}
	if git.dirs[0] != repoRoot {
		t.Errorf("git dir = %q, want repo root", git.dirs[0])
	}
}

func TestWorktreeCreatePropagatesGitError(t *testing.T) {
	repoRoot := t.TempDir()
	wantPath := filepath.Join(filepath.Dir(repoRoot), "wt-err", "x")
	git := &fakeGit{
		outputs: map[string]string{},
		errs: map[string]error{
			// The add fails for a reason other than a branch collision: the
			// branch-existence probe also fails (branch absent), so the
			// original error must propagate instead of being retried away.
			"worktree add -b auto/x " + wantPath + " main": errors.New("fatal: invalid reference: main"),
			"rev-parse --verify --quiet refs/heads/auto/x": errors.New("exit status 1"),
		},
	}
	mgr := NewWorktreeManager(git, repoRoot, "../wt-err", "main", "origin")

	if _, err := mgr.Create(context.Background(), LedgerItem{Slug: "x", Status: StatusPending}); err == nil {
		t.Fatal("Create returned nil error, want non-collision git failure propagated")
	}
}
