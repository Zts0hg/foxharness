package autodev

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type recordingRemoteEventReporter struct {
	Reporter
	terminal *TerminalReporter
	events   []RemoteEvent
	timeline *[]string
}

func (r *recordingRemoteEventReporter) OnRemoteEvent(ctx context.Context, event RemoteEvent) error {
	r.events = append(r.events, event)
	if r.timeline != nil {
		*r.timeline = append(*r.timeline, "event:"+event.EventID)
	}
	if r.terminal != nil {
		return r.terminal.OnRemoteEvent(ctx, event)
	}
	return nil
}

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

func (g *remoteGit) Run(ctx context.Context, dir string, args ...string) (CommandResult, error) {
	key := strings.Join(args, " ")
	g.calls = append(g.calls, key)
	switch args[0] {
	case "status":
		if g.state.dirty || g.state.staged {
			return stdoutResult(" M foo.go\n"), nil
		}
		return CommandResult{}, nil
	case "diff":
		if g.state.staged {
			return stdoutResult("foo.go\n"), nil
		}
		return CommandResult{}, nil
	case "rev-list":
		return stdoutResult(strconv.Itoa(g.state.commitCount) + "\n"), nil
	case "rev-parse":
		return stdoutResult(g.state.localTip + "\n"), nil
	case "ls-remote":
		if g.state.remoteTip == "" {
			return CommandResult{}, nil
		}
		return stdoutResult(g.state.remoteTip + "\trefs/heads/auto/x\n"), nil
	default:
		g.t.Errorf("control plane ran a non-read-only git command: git %s", key)
		return CommandResult{}, errors.New("forbidden")
	}
}

// remoteGH is a read-only fake ExecRunner answering gh queries from
// repoState. Any gh mutation fails the test.
type remoteGH struct {
	t     *testing.T
	state *repoState
	calls []string
}

func (g *remoteGH) Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error) {
	key := name + " " + strings.Join(args, " ")
	g.calls = append(g.calls, key)
	if name != "gh" {
		g.t.Errorf("unexpected exec command %q", key)
		return CommandResult{}, errors.New("forbidden")
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
		return stdoutResult(out + "]"), nil
	case len(args) >= 3 && args[0] == "issue" && args[1] == "view":
		number, err := strconv.Atoi(args[2])
		if err != nil {
			return CommandResult{}, err
		}
		title, ok := g.state.issues[number]
		if !ok {
			return stdoutResult("issue not found"), errors.New("exit status 1")
		}
		return stdoutResult(fmt.Sprintf(`{"number":%d,"title":%q,"body":%q,"state":"CLOSED"}`,
			number, title, issueMarker("item-test"))), nil
	case len(args) >= 2 && args[0] == "api" && args[1] == "--paginate":
		marker := markerFromSearchArgs(args)
		items := ""
		for n := range g.state.issues {
			if items != "" {
				items += ","
			}
			items += fmt.Sprintf(`{"number":%d,"body":%q,"state":"OPEN"}`, n, marker)
		}
		return stdoutResult(`[{"items":[` + items + `]}]`), nil
	case len(args) >= 2 && args[0] == "pr" && args[1] == "view":
		if g.state.prNumber == 0 {
			return stdoutResult("no pull requests found"), errors.New("exit status 1")
		}
		return stdoutResult(fmt.Sprintf(`{"number":%d,"body":%q}`, g.state.prNumber, g.state.prBody)), nil
	default:
		g.t.Errorf("control plane ran a non-read-only gh command: %s", key)
		return CommandResult{}, errors.New("forbidden")
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

func (c *remoteCore) Drain(context.Context) error { return nil }
func (c *remoteCore) Close(context.Context) error { return nil }

func (c *remoteCore) SetUserAsker(a tools.UserAsker) {}
func (c *remoteCore) SetModel(model string) error    { return nil }
func (c *remoteCore) WorkDir() string                { return "/wt" }
func (c *remoteCore) StagePrompt(ctx context.Context, command, args string) (string, error) {
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
	return LedgerItem{ItemID: "item-test", Slug: "x", Title: "Engine memory writes", Status: StatusInProgress, Branch: "auto/x"}
}

func recordRemoteItem(item *LedgerItem, snapshots *[]LedgerItem) func(string, func(*LedgerItem)) error {
	return func(_ string, mut func(*LedgerItem)) error {
		mut(item)
		if snapshots != nil {
			*snapshots = append(*snapshots, *item)
		}
		return nil
	}
}

func TestPublishDrivesOrderedSequence(t *testing.T) {
	state := &repoState{dirty: true, localTip: "aaa111", issues: map[int]string{}}
	git := &remoteGit{t: t, state: state}
	gh := &remoteGH{t: t, state: state}
	baseReporter := NewTerminalReporter(io.Discard)
	var timeline []string
	reporter := &recordingRemoteEventReporter{Reporter: baseReporter, timeline: &timeline}
	machine := NewStageMachine(&reviewingEngineer{}, reporter)
	pub := NewRemotePublisher(machine, git, gh, reporter, remoteConfig())
	core := &remoteCore{effects: []func(){
		func() { state.staged = true },
		func() { state.staged = false; state.dirty = false; state.commitCount = 1; state.localTip = "bbb222" },
		func() { state.remoteTip = state.localTip },
		func() { state.issues[31] = "Engine memory writes" },
		func() { state.prNumber = 32; state.prBody = "Implements it.\n\nCloses #31" },
	}}

	var recorded []LedgerItem
	var operations []string
	bindingWasPending := false
	item := happyItem()
	record := func(operation string, mut func(*LedgerItem)) error {
		mut(&item)
		if operation == "issue-binding" {
			bindingWasPending = len(item.Outbox) == 1 && !item.Outbox[0].Delivered
		}
		operations = append(operations, operation)
		timeline = append(timeline, "record:"+operation)
		recorded = append(recorded, item)
		return nil
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
	if !bindingWasPending {
		t.Fatal("issue binding did not persist the issue and pending event atomically")
	}
	if len(item.Outbox) != 1 || !item.Outbox[0].Delivered {
		t.Fatalf("outbox = %+v, want one delivered issue event", item.Outbox)
	}
	event := item.Outbox[0]
	if event.EventID != "issue:item-test:31" || event.ItemID != item.ItemID || event.Kind != RemoteEventIssue || event.Number != 31 {
		t.Errorf("outbox event = %+v, want stable issue identity", event)
	}
	if len(reporter.events) != 1 || reporter.events[0].EventID != event.EventID {
		t.Errorf("reported events = %+v, want one event matching the durable outbox", reporter.events)
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
	wantOperations := []string{
		"publish-stage-changes-intent",
		"publish-stage-changes-verified",
		"publish-commit-staged-intent",
		"publish-commit-staged-verified",
		"publish-push-intent",
		"publish-push-verified",
		"publish-issue-intent",
		"issue-binding",
		"issue-event-delivered",
		"publish-pr-intent",
		"pr-binding",
	}
	if !reflect.DeepEqual(operations, wantOperations) {
		t.Errorf("record operations = %v, want %v", operations, wantOperations)
	}
	wantIssueSequence := []string{
		"record:issue-binding",
		"event:issue:item-test:31",
		"record:issue-event-delivered",
		"record:publish-pr-intent",
	}
	var issueSequence []string
	for _, entry := range timeline {
		if entry == "record:issue-binding" || entry == "event:issue:item-test:31" ||
			entry == "record:issue-event-delivered" || entry == "record:publish-pr-intent" {
			issueSequence = append(issueSequence, entry)
		}
	}
	if !reflect.DeepEqual(issueSequence, wantIssueSequence) {
		t.Errorf("issue publication sequence = %v, want %v", issueSequence, wantIssueSequence)
	}
}

func TestPublishReplaysPendingIssueEventBeforePRWithStableIdentity(t *testing.T) {
	state := &repoState{
		dirty:       false,
		commitCount: 1,
		localTip:    "bbb222",
		remoteTip:   "bbb222",
		issues:      map[int]string{31: "renamed and closed"},
	}
	git := &remoteGit{t: t, state: state}
	gh := &remoteGH{t: t, state: state}
	var output bytes.Buffer
	terminal := NewTerminalReporter(&output)
	reporter := &recordingRemoteEventReporter{Reporter: terminal, terminal: terminal}
	pub := NewRemotePublisher(NewStageMachine(&reviewingEngineer{}, reporter), git, gh, reporter, remoteConfig())
	item := happyItem()
	item.Stage = StageIssue
	item.StageState = StageStateVerified
	item.Issue = 31
	item.Outbox = []RemoteEventRecord{{
		EventID: "issue:item-test:31",
		ItemID:  item.ItemID,
		Kind:    RemoteEventIssue,
		Number:  31,
	}}

	core := &remoteCore{effects: []func(){func() {
		state.prNumber = 32
		state.prBody = "Closes #31"
	}}}
	ackAttempts := 0
	record := func(operation string, mut func(*LedgerItem)) error {
		if operation == "issue-event-delivered" {
			ackAttempts++
			if ackAttempts == 1 {
				return errors.New("simulated delivery ack failure")
			}
		}
		mut(&item)
		return nil
	}

	if _, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, record); err == nil {
		t.Fatal("first Publish returned nil, want delivery ack failure")
	}
	if len(core.prompts) != 0 {
		t.Fatalf("core runs after failed delivery ack = %d, want 0 before PR work", len(core.prompts))
	}
	if item.Outbox[0].Delivered {
		t.Fatal("failed delivery ack marked the event delivered")
	}

	result, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, record)
	if err != nil {
		t.Fatalf("second Publish returned error: %v", err)
	}
	if result.PR != 32 || !item.Outbox[0].Delivered {
		t.Errorf("resumed result/item = %+v / %+v, want PR 32 and delivered event", result, item.Outbox)
	}
	if len(reporter.events) != 2 || reporter.events[0].EventID != reporter.events[1].EventID {
		t.Errorf("delivery attempts = %+v, want two attempts with one stable EventID", reporter.events)
	}
	if got := strings.Count(output.String(), "[remote] issue #31"); got != 1 {
		t.Errorf("terminal issue observations = %d, want one idempotent logical output; output=%q", got, output.String())
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
	if _, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, recordRemoteItem(&item, nil)); err != nil {
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
	result, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, recordRemoteItem(&item, nil))
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
	if _, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, recordRemoteItem(&item, nil)); err != nil {
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
	result, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, recordRemoteItem(&item, nil))
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
	if _, err := pub.Publish(context.Background(), core, Worktree{Path: "/wt", Branch: "auto/x", Slug: "x"}, item, recordRemoteItem(&item, nil)); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if len(core.prompts) != 4 {
		t.Fatalf("core runs = %d, want 4 (push retried despite approval): %q", len(core.prompts), core.prompts)
	}
}
