package autodev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// repoState models the ground truth the fake core mutates and the fake
// git/gh runners observe — the same separation the real system has.
type repoState struct {
	dirty       bool
	staged      bool
	commitCount int
	localTip    string
	remoteTip   string
	issues      map[int]string
	prBody      string
	prNumber    int
}

// remoteGit is a read-only fake GitRunner over repoState. It fails the test
// on any mutating git invocation (the control plane must never mutate).
type remoteGit struct {
	t     *testing.T
	state *repoState
	calls []string
}

func (g *remoteGit) Run(ctx context.Context, dir string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	g.calls = append(g.calls, key)
	switch args[0] {
	case "status":
		if g.state.dirty || g.state.staged {
			return " M foo.go\n", nil
		}
		return "", nil
	case "diff":
		if g.state.staged {
			return "foo.go\n", nil
		}
		return "", nil
	case "rev-list":
		return strconv.Itoa(g.state.commitCount) + "\n", nil
	case "rev-parse":
		return g.state.localTip + "\n", nil
	case "ls-remote":
		if g.state.remoteTip == "" {
			return "", nil
		}
		return g.state.remoteTip + "\trefs/heads/auto/x\n", nil
	default:
		g.t.Errorf("control plane ran a non-read-only git command: git %s", key)
		return "", errors.New("forbidden")
	}
}

// remoteGH is a read-only fake ExecRunner answering gh queries from
// repoState. Any gh mutation fails the test.
type remoteGH struct {
	t     *testing.T
	state *repoState
	calls []string
}

func (g *remoteGH) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	g.calls = append(g.calls, key)
	if name != "gh" {
		g.t.Errorf("unexpected exec command %q", key)
		return "", errors.New("forbidden")
	}
	switch {
	case len(args) >= 2 && args[0] == "issue" && args[1] == "list":
		out := "["
		first := true
		for n, title := range g.state.issues {
			if !first {
				out += ","
			}
			out += fmt.Sprintf(`{"number":%d,"title":%q}`, n, title)
			first = false
		}
		return out + "]", nil
	case len(args) >= 2 && args[0] == "pr" && args[1] == "view":
		if g.state.prNumber == 0 {
			return "no pull requests found", errors.New("exit status 1")
		}
		return fmt.Sprintf(`{"number":%d,"body":%q}`, g.state.prNumber, g.state.prBody), nil
	default:
		g.t.Errorf("control plane ran a non-read-only gh command: %s", key)
		return "", errors.New("forbidden")
	}
}

// remoteCore is a scripted CoreRunner whose Run applies the next effect to
// the repoState — simulating the core Agent doing the real git/gh work.
type remoteCore struct {
	prompts []string
	effects []func()
}

func (c *remoteCore) Run(ctx context.Context, prompt string, r engine.Reporter) (*engine.RunResult, error) {
	c.prompts = append(c.prompts, prompt)
	if len(c.effects) > 0 {
		effect := c.effects[0]
		c.effects = c.effects[1:]
		if effect != nil {
			effect()
		}
	}
	return &engine.RunResult{FinalMessage: "step attempted"}, nil
}

func (c *remoteCore) SetUserAsker(a tools.UserAsker) {}
func (c *remoteCore) SetModel(model string) error    { return nil }
func (c *remoteCore) WorkDir() string                { return "/wt" }
func (c *remoteCore) StagePrompt(command, args string) (string, error) {
	return fmt.Sprintf("PROMPT[%s|%s]", command, args), nil
}

func remoteConfig() AutodevConfig {
	return AutodevConfig{
		BaseBranch: "main",
		Remote:     "origin",
		RemoteFlow: RemoteFlowConfig{CreateIssue: true, OpenPR: true, LinkIssue: true},
	}
}

func newPublisher(t *testing.T, state *repoState) (*RemotePublisher, *remoteGit, *remoteGH, *reviewingEngineer) {
	t.Helper()
	git := &remoteGit{t: t, state: state}
	gh := &remoteGH{t: t, state: state}
	eng := &reviewingEngineer{}
	machine := NewStageMachine(eng, NewTerminalReporter(io.Discard))
	pub := NewRemotePublisher(machine, git, gh, NewTerminalReporter(io.Discard), remoteConfig())
	return pub, git, gh, eng
}

func happyItem() LedgerItem {
	return LedgerItem{Slug: "x", Title: "Engine memory writes", Status: StatusInProgress, Branch: "auto/x"}
}

func TestPublishDrivesOrderedSequence(t *testing.T) {
	state := &repoState{dirty: true, localTip: "aaa111", issues: map[int]string{}}
	pub, _, _, _ := newPublisher(t, state)
	core := &remoteCore{effects: []func(){
		func() { state.staged = true },
		func() { state.staged = false; state.dirty = false; state.commitCount = 1; state.localTip = "bbb222" },
		func() { state.remoteTip = state.localTip },
		func() { state.issues[31] = "Engine memory writes" },
		func() { state.prNumber = 32; state.prBody = "Implements it.\n\nCloses #31" },
	}}

	var recorded []LedgerItem
	item := happyItem()
	record := func(mut func(*LedgerItem)) {
		mut(&item)
		recorded = append(recorded, item)
	}

	result, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, record)
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if len(core.prompts) != 5 {
		t.Fatalf("core runs = %d, want 5 ordered steps (TC-011): %q", len(core.prompts), core.prompts)
	}
	wantOrder := []string{"git add", "codexspec:commit-staged", "git push", "issue", "codexspec:pr"}
	for i, marker := range wantOrder {
		if !strings.Contains(strings.ToLower(core.prompts[i]), strings.ToLower(marker)) {
			t.Errorf("prompt[%d] = %q, want step %q (TC-011)", i, core.prompts[i], marker)
		}
	}

	if result.Issue != 31 || result.PR != 32 || result.Branch != "auto/x" {
		t.Errorf("result = %+v, want issue 31, pr 32, branch auto/x", result)
	}
	if item.Issue != 31 || item.PR != 32 {
		t.Errorf("recorded item = %+v, want issue/pr recorded via callback", item)
	}

	// The issue number must be durably recorded before the PR step runs so
	// an interrupted run reuses it (Edge Cases).
	foundIssueBeforePR := false
	for _, snap := range recorded {
		if snap.Issue == 31 && snap.PR == 0 {
			foundIssueBeforePR = true
		}
	}
	if !foundIssueBeforePR {
		t.Error("issue number was not recorded before the PR step completed")
	}
}

func TestPublishNothingToCommitEngineerSteers(t *testing.T) {
	state := &repoState{dirty: true, localTip: "aaa111", issues: map[int]string{}}
	pub, _, _, eng := newPublisher(t, state)
	eng.reviews = []string{"Nothing was committed. Run git add -A, then git commit with the generated message."}

	core := &remoteCore{effects: []func(){
		func() { state.staged = true },
		nil, // commit-staged run produces no commit (TC-024: "nothing to commit")
		func() { state.staged = false; state.dirty = false; state.commitCount = 1; state.localTip = "bbb222" },
		func() { state.remoteTip = state.localTip },
		func() { state.issues[31] = "Engine memory writes" },
		func() { state.prNumber = 32; state.prBody = "Closes #31" },
	}}

	item := happyItem()
	if _, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, func(mut func(*LedgerItem)) { mut(&item) }); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if len(core.prompts) != 6 {
		t.Fatalf("core runs = %d, want 6 (commit retried once, TC-024): %q", len(core.prompts), core.prompts)
	}
	if core.prompts[2] != eng.gapsCorrection() {
		t.Errorf("retry prompt = %q, want the engineer correction verbatim", core.prompts[2])
	}
	if eng.reviewCalls != 1 {
		t.Errorf("Review calls = %d, want 1", eng.reviewCalls)
	}
	if !strings.Contains(strings.ToLower(eng.gaps[0]), "commit") {
		t.Errorf("gap = %q, want commit gap routed to the engineer (REQ-014)", eng.gaps[0])
	}
}

// gapsCorrection returns the correction the engineer issued for the test
// above; centralizing it keeps the assertion in sync with the script.
func (r *reviewingEngineer) gapsCorrection() string {
	return "Nothing was committed. Run git add -A, then git commit with the generated message."
}

func TestPublishPRMustLinkIssue(t *testing.T) {
	state := &repoState{dirty: true, localTip: "aaa111", issues: map[int]string{}}
	pub, _, _, eng := newPublisher(t, state)
	eng.reviews = []string{"Edit the PR body to include the line Closes #31."}

	core := &remoteCore{effects: []func(){
		func() { state.staged = true },
		func() { state.staged = false; state.dirty = false; state.commitCount = 1; state.localTip = "bbb222" },
		func() { state.remoteTip = state.localTip },
		func() { state.issues[31] = "Engine memory writes" },
		func() { state.prNumber = 32; state.prBody = "no link here" },
		func() { state.prBody = "Fixed.\n\nCloses #31" },
	}}

	item := happyItem()
	result, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, func(mut func(*LedgerItem)) { mut(&item) })
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if result.PR != 32 {
		t.Errorf("result.PR = %d, want 32", result.PR)
	}
	if len(core.prompts) != 6 {
		t.Fatalf("core runs = %d, want 6 (PR body fixed once, TC-012): %q", len(core.prompts), core.prompts)
	}
}

func TestPublishNeverMerges(t *testing.T) {
	state := &repoState{dirty: true, localTip: "aaa111", issues: map[int]string{}}
	pub, git, gh, _ := newPublisher(t, state)
	core := &remoteCore{effects: []func(){
		func() { state.staged = true },
		func() { state.staged = false; state.dirty = false; state.commitCount = 1; state.localTip = "bbb222" },
		func() { state.remoteTip = state.localTip },
		func() { state.issues[31] = "Engine memory writes" },
		func() { state.prNumber = 32; state.prBody = "Closes #31" },
	}}

	item := happyItem()
	if _, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, func(mut func(*LedgerItem)) { mut(&item) }); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	all := strings.ToLower(strings.Join(core.prompts, "\n") + strings.Join(git.calls, "\n") + strings.Join(gh.calls, "\n"))
	if strings.Contains(all, "merge") {
		t.Error("publish flow mentions merge, want no merge anywhere (TC-021, REQ-020)")
	}
}

func TestPublishIdempotentOnResume(t *testing.T) {
	// Ground truth: commit + push already done; issue already recorded in
	// the ledger; only the PR step still needs the core Agent (PLAN-002).
	state := &repoState{
		dirty:       false,
		commitCount: 1,
		localTip:    "bbb222",
		remoteTip:   "bbb222",
		issues:      map[int]string{31: "Engine memory writes"},
	}
	pub, _, _, _ := newPublisher(t, state)
	core := &remoteCore{effects: []func(){
		func() { state.prNumber = 32; state.prBody = "Closes #31" },
	}}

	item := happyItem()
	item.Issue = 31
	result, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, func(mut func(*LedgerItem)) { mut(&item) })
	if err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}

	if len(core.prompts) != 1 {
		t.Fatalf("core runs = %d, want 1 (completed steps skipped on resume): %q", len(core.prompts), core.prompts)
	}
	if !strings.Contains(core.prompts[0], "codexspec:pr") {
		t.Errorf("prompt = %q, want only the PR step", core.prompts[0])
	}
	if result.Issue != 31 || result.PR != 32 {
		t.Errorf("result = %+v, want recorded issue 31 reused and pr 32", result)
	}
}

func TestPublishEngineerApprovalCannotSkipPush(t *testing.T) {
	state := &repoState{dirty: false, commitCount: 1, localTip: "bbb222", issues: map[int]string{}}
	pub, _, _, _ := newPublisher(t, state)
	// The engineer always approves, but the push has not reached the
	// remote: the loop must keep driving the push step (TC-025).
	core := &remoteCore{effects: []func(){
		nil,
		func() { state.remoteTip = state.localTip },
		func() { state.issues[31] = "Engine memory writes" },
		func() { state.prNumber = 32; state.prBody = "Closes #31" },
	}}

	item := happyItem()
	if _, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, func(mut func(*LedgerItem)) { mut(&item) }); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if len(core.prompts) != 4 {
		t.Fatalf("core runs = %d, want 4 (push retried despite approval): %q", len(core.prompts), core.prompts)
	}
}
