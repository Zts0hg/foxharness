package keeprun

import "testing"

func TestMergeProhibited(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{"git_merge", "git merge feature", true},
		{"git_merge_with_dir_flag", "git -C /w merge origin/main", true},
		{"git_merge_global_flag", "git --no-pager merge main", true},
		{"git_merge_ff", "git merge --ff-only main", true},
		{"gh_pr_merge", "gh pr merge 12 --squash", true},
		{"glab_mr_merge", "glab mr merge 7", true},
		{"chained_merge", "cd /w && git merge main", true},
		{"or_chained_merge", "git status || git merge main", true},
		{"semicolon_chained_merge", "git fetch; git merge origin/main", true},
		{"piped_not_merge", "git log | grep merge", false},
		// Not merges:
		{"git_commit_with_merge_in_message", "git commit -m 'fix merge conflict notes'", false},
		{"git_log_merges_flag", "git log --merges", false},
		{"git_push_branch", "git push -u origin keep-run-add-dark-mode", false},
		{"go_test", "go test ./...", false},
		{"plain_echo", "echo merge", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MergeProhibited(tt.command); got != tt.want {
				t.Errorf("MergeProhibited(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}
