package keeprun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSelectSpecDir(t *testing.T) {
	cases := []struct {
		name      string
		untracked []string
		want      string
	}{
		{
			name:      "single new feature dir",
			untracked: []string{".codexspec/specs/2026-0604-1200zz-new/spec.md"},
			want:      "2026-0604-1200zz-new",
		},
		{
			name: "picks newest among several untracked",
			untracked: []string{
				".codexspec/specs/2026-0101-0000aa-old/spec.md",
				".codexspec/specs/2026-0604-1200zz-new/plan.md",
			},
			want: "2026-0604-1200zz-new",
		},
		{
			name:      "ignores non-conforming directory names",
			untracked: []string{".codexspec/specs/engine-writes-bare-slug/spec.md"},
			want:      "",
		},
		{
			name:      "ignores paths outside specs",
			untracked: []string{"internal/foo.go", ".codexspec/memory/x.md"},
			want:      "",
		},
		{
			name:      "empty",
			untracked: nil,
			want:      "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := selectSpecDir(c.untracked); got != c.want {
				t.Errorf("selectSpecDir(%v) = %q, want %q", c.untracked, got, c.want)
			}
		})
	}
}

func TestResolveSpecDirDetectsUntrackedNotInherited(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	runGit("init")
	// An inherited, committed feature dir — detection must ignore it.
	writeFileT(t, filepath.Join(repo, ".codexspec/specs/2026-0101-0000aa-inherited/spec.md"), "old")
	runGit("add", "-A")
	runGit("commit", "-m", "inherited spec")
	// The task's new feature dir, untracked (as it is before the commit phase).
	writeFileT(t, filepath.Join(repo, ".codexspec/specs/2026-0604-1200zz-task/spec.md"), "new")

	got, err := NewManager(repo).ResolveSpecDir(context.Background(), repo)
	if err != nil {
		t.Fatalf("ResolveSpecDir: %v", err)
	}
	want := filepath.Join(repo, ".codexspec", "specs", "2026-0604-1200zz-task")
	if got != want {
		t.Errorf("ResolveSpecDir = %q, want %q (must detect the untracked dir, not the inherited committed one)", got, want)
	}
}
