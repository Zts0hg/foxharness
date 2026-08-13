package autodev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// RemotePublisher drives the core Agent through the fixed remote sequence
// after the completion gate is green: stage → commit-staged → push → issue
// → PR (REQ-019). Every git/gh mutation is performed by the core Agent via
// its own tools; this module only seeds each step, read-only-verifies the
// ground truth afterwards (REQ-029), routes failures through the engineer
// Agent (REQ-014), and records verified issue/PR numbers. It never merges
// (REQ-020) and skips steps whose ground truth is already satisfied so a
// resumed run is idempotent (PLAN-002).
type RemotePublisher struct {
	machine  *StageMachine
	git      GitRunner
	exec     ExecRunner
	reporter Reporter
	cfg      AutodevConfig
}

// NewRemotePublisher creates a publisher. git serves read-only verification
// queries; exec serves read-only gh queries.
func NewRemotePublisher(machine *StageMachine, git GitRunner, exec ExecRunner, reporter Reporter, cfg AutodevConfig) *RemotePublisher {
	return &RemotePublisher{machine: machine, git: git, exec: exec, reporter: reporter, cfg: cfg}
}

// Publish runs the remote sequence for item inside wt, driving the core
// Agent step by step. record durably persists each intent and verified
// binding before dependent work; it may be nil.
func (p *RemotePublisher) Publish(ctx context.Context, core CoreRunner, wt Worktree, item LedgerItem, record func(operation string, mut func(*LedgerItem)) error) (PublishResult, error) {
	if record == nil {
		record = func(string, func(*LedgerItem)) error { return nil }
	}
	if err := p.deliverPendingEvents(ctx, &item, record); err != nil {
		return PublishResult{Branch: wt.Branch, Issue: item.Issue, PR: item.PR}, err
	}
	sc := &StageContext{
		Item:       Item{Title: item.Title, Description: item.Description},
		ItemID:     item.ItemID,
		Slug:       item.Slug,
		WorkDir:    wt.Path,
		Branch:     wt.Branch,
		BaseBranch: p.cfg.BaseBranch,
		Remote:     p.cfg.Remote,
		Issue:      item.Issue,
		PR:         item.PR,
	}
	bindCoreAttemptRecorder(sc, &item, record)

	steps := p.steps()
	start := remoteResumeIndex(item, steps)
	for i := start; i < len(steps); i++ {
		if err := ctx.Err(); err != nil {
			return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, err
		}
		st := steps[i]
		stage, ok := pipelineStage(st.Name)
		if !ok {
			return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR},
				fmt.Errorf("remote stage %q is not part of the durable stage vocabulary", st.Name)
		}
		resuming := item.Stage == stage && item.StageState == StageStateRunning
		if !resuming {
			if err := record("publish-"+st.Name+"-intent", func(it *LedgerItem) {
				it.Stage = stage
				it.StageState = StageStateRunning
			}); err != nil {
				return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, err
			}
		}
		previousIssue, previousPR := sc.Issue, sc.PR
		var runErr error
		if resuming {
			runErr = p.machine.ResumeStep(ctx, core, sc, st)
		} else {
			runErr = p.machine.RunStep(ctx, core, sc, st)
		}
		verifiedTerminal := false
		if runErr != nil {
			var outcomeErr *CoreOutcomeError
			verifiedTerminal = errors.As(runErr, &outcomeErr) && outcomeErr.Verified
		}
		if runErr != nil && !verifiedTerminal {
			return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, runErr
		}
		issueChanged := sc.Issue != 0 && sc.Issue != previousIssue
		prChanged := sc.PR != 0 && sc.PR != previousPR
		operation := "publish-" + st.Name + "-verified"
		if issueChanged {
			operation = "issue-binding"
		}
		if prChanged {
			operation = "pr-binding"
		}
		if err := record(operation, func(it *LedgerItem) {
			it.Stage = stage
			it.StageState = StageStateVerified
			if issueChanged {
				it.Issue = sc.Issue
				it.Outbox = append(it.Outbox, RemoteEventRecord{
					EventID: issueEventID(it.ItemID, sc.Issue),
					ItemID:  it.ItemID,
					Kind:    RemoteEventIssue,
					Number:  sc.Issue,
				})
			}
			if prChanged {
				it.PR = sc.PR
			}
		}); err != nil {
			return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, err
		}
		if verifiedTerminal {
			return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, runErr
		}
		if issueChanged {
			item.Issue = sc.Issue
			item.Outbox = append(item.Outbox, RemoteEventRecord{
				EventID: issueEventID(item.ItemID, sc.Issue),
				ItemID:  item.ItemID,
				Kind:    RemoteEventIssue,
				Number:  sc.Issue,
			})
			if err := p.deliverPendingEvents(ctx, &item, record); err != nil {
				return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, err
			}
		}
		if prChanged || (stage == StagePR && sc.PR != 0 && previousPR == sc.PR) {
			p.reporter.OnPR(ctx, sc.PR)
		}
	}
	return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, nil
}

func issueEventID(itemID ItemID, number int) string {
	return fmt.Sprintf("issue:%s:%d", itemID, number)
}

func (p *RemotePublisher) deliverPendingEvents(ctx context.Context, item *LedgerItem, record func(string, func(*LedgerItem)) error) error {
	for i := range item.Outbox {
		if item.Outbox[i].Delivered {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		event := item.Outbox[i].event()
		if p.reporter == nil {
			return fmt.Errorf("deliver remote event %s: reporter is unavailable", event.EventID)
		}
		if err := p.reporter.OnRemoteEvent(ctx, event); err != nil {
			return fmt.Errorf("deliver remote event %s: %w", event.EventID, err)
		}
		if err := record("issue-event-delivered", func(it *LedgerItem) {
			for j := range it.Outbox {
				if it.Outbox[j].EventID == event.EventID {
					it.Outbox[j].Delivered = true
				}
			}
		}); err != nil {
			return err
		}
		item.Outbox[i].Delivered = true
	}
	return nil
}

func remoteResumeIndex(item LedgerItem, steps []Stage) int {
	currentRank := remoteStageRank(item.Stage)
	if currentRank < 0 {
		return 0
	}
	for i, step := range steps {
		rank := remoteStageRank(PipelineStage(step.Name))
		if item.StageState == StageStateVerified && rank > currentRank {
			return i
		}
		if item.StageState != StageStateVerified && rank >= currentRank {
			return i
		}
	}
	return len(steps)
}

func remoteStageRank(stage PipelineStage) int {
	switch stage {
	case StagePublish:
		return 0
	case StageStageChanges:
		return 1
	case StageCommitStaged:
		return 2
	case StagePush:
		return 3
	case StageIssue:
		return 4
	case StagePR:
		return 5
	default:
		return -1
	}
}

// steps assembles the remote pipeline honoring the remote_flow toggles.
func (p *RemotePublisher) steps() []Stage {
	steps := []Stage{
		{
			Name: "stage-changes",
			Prompt: func(sc *StageContext) string {
				return "The implementation for this item is complete and the completion gate is green. " +
					"Stage every change in this worktree for commit: run `git add -A` from the worktree root, " +
					"then show `git status --short` to confirm what is staged. Do not commit yet."
			},
			Skip:   p.skipWhenCommitted,
			Verify: p.verifyStaged,
		},
		{
			Name:    "commit-staged",
			Command: "codexspec:commit-staged",
			Append: func(sc *StageContext) string {
				return "After authoring the commit message, actually create the commit by running " +
					"`git commit` with that message. Do not push yet."
			},
			Skip:   p.skipWhenCommitted,
			Verify: p.verifyCommitted,
		},
		{
			Name: "push",
			Prompt: func(sc *StageContext) string {
				return fmt.Sprintf("Push this item's branch to the remote: run `git push -u %s %s` from the worktree root.",
					sc.Remote, sc.Branch)
			},
			Skip: func(ctx context.Context, sc *StageContext) bool {
				ok, _ := p.verifyPushed(ctx, sc)
				return ok
			},
			Verify: p.verifyPushed,
		},
	}

	if p.cfg.RemoteFlow.CreateIssue {
		resolve := p.resolveIssue
		steps = append(steps, Stage{
			Name: "issue",
			Prompt: func(sc *StageContext) string {
				return fmt.Sprintf("Create a GitHub issue documenting this requirement before the PR is opened: "+
					"run `gh issue create --title %q --body <a concise summary followed by the exact marker %q>`. "+
					"The marker must appear verbatim in the issue body.", sc.Item.Title, issueMarker(sc.ItemID))
			},
			Preflight: func(ctx context.Context, sc *StageContext) (bool, error) {
				ok, _, err := resolve(ctx, sc)
				return ok, err
			},
			VerifyWithError: resolve,
		})
	}
	if p.cfg.RemoteFlow.OpenPR {
		resolve := p.resolvePR
		steps = append(steps, Stage{
			Name:    "pr",
			Command: "codexspec:pr",
			Append: func(sc *StageContext) string {
				out := fmt.Sprintf("Then open the pull request now: run `gh pr create --base %s --head %s` "+
					"with the generated description as the body.", sc.BaseBranch, sc.Branch)
				if p.cfg.RemoteFlow.LinkIssue && sc.Issue != 0 {
					out += fmt.Sprintf(" The PR body MUST contain the exact line `Closes #%d` so the issue is linked.", sc.Issue)
				}
				return out
			},
			Preflight: func(ctx context.Context, sc *StageContext) (bool, error) {
				ok, _, err := resolve(ctx, sc)
				return ok, err
			},
			VerifyWithError: resolve,
		})
	}
	return steps
}

// committedAndClean is the commit ground truth: the worktree has no
// uncommitted changes and the branch holds at least one commit beyond the
// base branch. It is monotonic across the publish flow, which makes it a
// safe skip predicate for the stage and commit steps on resume.
func (p *RemotePublisher) committedAndClean(ctx context.Context, sc *StageContext) bool {
	result, runErr := p.git.Run(ctx, sc.WorkDir, "status", "--porcelain")
	status, err := strictCommandStdout(result, runErr)
	if err != nil || strings.TrimSpace(status) != "" {
		return false
	}
	result, runErr = p.git.Run(ctx, sc.WorkDir, "rev-list", "--count", sc.BaseBranch+"..HEAD")
	count, err := strictCommandStdout(result, runErr)
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(count)
	return trimmed != "" && trimmed != "0"
}

func (p *RemotePublisher) skipWhenCommitted(ctx context.Context, sc *StageContext) bool {
	return p.committedAndClean(ctx, sc)
}

func (p *RemotePublisher) verifyStaged(ctx context.Context, sc *StageContext) (bool, string) {
	result, runErr := p.git.Run(ctx, sc.WorkDir, "diff", "--cached", "--name-only")
	staged, err := strictCommandStdout(result, runErr)
	if err == nil && strings.TrimSpace(staged) != "" {
		return true, ""
	}
	if p.committedAndClean(ctx, sc) {
		return true, ""
	}
	return false, "no changes are staged for commit (git diff --cached is empty); run git add on the changed files"
}

func (p *RemotePublisher) verifyCommitted(ctx context.Context, sc *StageContext) (bool, string) {
	if p.committedAndClean(ctx, sc) {
		return true, ""
	}
	return false, fmt.Sprintf(
		"the commit was not created: either the worktree still has uncommitted changes or HEAD has not advanced beyond %s",
		sc.BaseBranch)
}

func (p *RemotePublisher) verifyPushed(ctx context.Context, sc *StageContext) (bool, string) {
	result, runErr := p.git.Run(ctx, sc.WorkDir, "rev-parse", "HEAD")
	local, err := strictCommandStdout(result, runErr)
	if err != nil {
		return false, fmt.Sprintf("cannot resolve local HEAD: %v", err)
	}
	result, runErr = p.git.Run(ctx, sc.WorkDir, "ls-remote", "--heads", sc.Remote, sc.Branch)
	remote, err := strictCommandStdout(result, runErr)
	if err != nil {
		return false, fmt.Sprintf("cannot query remote %s: %v", sc.Remote, err)
	}
	remoteTip := strings.Fields(remote)
	localTip := strings.TrimSpace(local)
	if len(remoteTip) > 0 && localTip != "" && remoteTip[0] == localTip {
		return true, ""
	}
	return false, fmt.Sprintf("the remote branch %s/%s does not match local HEAD — the push has not completed", sc.Remote, sc.Branch)
}

// verifyIssue queries gh for an issue whose title matches the item exactly.
// Publish persists and reports the verified number before the PR step runs.
func (p *RemotePublisher) verifyIssue() func(ctx context.Context, sc *StageContext) (bool, string) {
	return func(ctx context.Context, sc *StageContext) (bool, string) {
		ok, gap, err := p.resolveIssue(ctx, sc)
		if err != nil {
			return false, err.Error()
		}
		return ok, gap
	}
}

// IssueIdentityConflictError reports durable marker ambiguity or a recorded
// number that no longer identifies its item.
type IssueIdentityConflictError struct{ Reason string }

func (e *IssueIdentityConflictError) Error() string {
	return "GitHub issue identity conflict: " + e.Reason
}

func issueMarker(itemID ItemID) string {
	return fmt.Sprintf("<!-- fox-autodev-item-id:%s -->", itemID)
}

func (p *RemotePublisher) resolveIssue(ctx context.Context, sc *StageContext) (bool, string, error) {
	if sc.ItemID == "" {
		return false, "", &IssueIdentityConflictError{Reason: "item ID is empty"}
	}
	marker := issueMarker(sc.ItemID)
	type issueRecord struct {
		Number      int             `json:"number"`
		Title       string          `json:"title"`
		Body        string          `json:"body"`
		State       string          `json:"state"`
		PullRequest json.RawMessage `json:"pull_request"`
	}
	if sc.Issue != 0 {
		result, runErr := p.exec.Run(ctx, sc.WorkDir, "gh", "issue", "view", fmt.Sprint(sc.Issue), "--json", "number,title,body,state")
		out, err := strictCommandStdout(result, runErr)
		if err != nil {
			return false, "", &IssueIdentityConflictError{Reason: fmt.Sprintf("cannot verify recorded issue #%d: %v", sc.Issue, err)}
		}
		var issue issueRecord
		if err := json.Unmarshal([]byte(extractJSON(out)), &issue); err != nil || issue.Number != sc.Issue || !strings.Contains(issue.Body, marker) {
			return false, "", &IssueIdentityConflictError{Reason: fmt.Sprintf("recorded issue #%d does not carry marker %s", sc.Issue, marker)}
		}
		return true, "", nil
	}
	query := url.QueryEscape(marker+" in:body type:issue") + "+repo%3A{owner}%2F{repo}"
	result, runErr := p.exec.Run(ctx, sc.WorkDir, "gh", "api", "--paginate", "--slurp", "search/issues?q="+query+"&per_page=100")
	out, err := strictCommandStdout(result, runErr)
	if err != nil {
		return false, "", fmt.Errorf("query all GitHub issues: %w", err)
	}
	var pages []struct {
		Items []issueRecord `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &pages); err != nil {
		return false, "", fmt.Errorf("parse paginated GitHub issues: %w", err)
	}
	var matches []issueRecord
	for _, page := range pages {
		for _, issue := range page.Items {
			if len(issue.PullRequest) == 0 && strings.Contains(issue.Body, marker) {
				matches = append(matches, issue)
			}
		}
	}
	switch len(matches) {
	case 0:
		return false, fmt.Sprintf("no GitHub issue carrying marker %s exists yet", marker), nil
	case 1:
		sc.Issue = matches[0].Number
		return true, "", nil
	default:
		return false, "", &IssueIdentityConflictError{Reason: fmt.Sprintf("marker %s matches %d issues", marker, len(matches))}
	}
}

// PRIdentityConflictError reports ambiguous discovery or a recorded PR whose
// immutable identity no longer matches the item publication contract.
type PRIdentityConflictError struct{ Reason string }

func (e *PRIdentityConflictError) Error() string {
	return "GitHub pull request identity conflict: " + e.Reason
}

type pullRequestRecord struct {
	Number      int    `json:"number"`
	Body        string `json:"body"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
}

// verifyPR adapts the error-capable resolver to the legacy Verify shape used
// by focused tests.
func (p *RemotePublisher) verifyPR() func(ctx context.Context, sc *StageContext) (bool, string) {
	return func(ctx context.Context, sc *StageContext) (bool, string) {
		ok, gap, err := p.resolvePR(ctx, sc)
		if err != nil {
			return false, err.Error()
		}
		return ok, gap
	}
}

func (p *RemotePublisher) resolvePR(ctx context.Context, sc *StageContext) (bool, string, error) {
	const fields = "number,body,baseRefName,headRefName"
	recorded := sc.PR != 0
	var pr pullRequestRecord
	if recorded {
		result, runErr := p.exec.Run(ctx, sc.WorkDir, "gh", "pr", "view", fmt.Sprint(sc.PR), "--json", fields)
		out, err := strictCommandStdout(result, runErr)
		if err != nil {
			return false, "", &PRIdentityConflictError{Reason: fmt.Sprintf("cannot verify recorded PR #%d: %v", sc.PR, err)}
		}
		if err := json.Unmarshal([]byte(extractJSON(out)), &pr); err != nil || pr.Number != sc.PR {
			return false, "", &PRIdentityConflictError{Reason: fmt.Sprintf("recorded PR #%d returned malformed or mismatched identity", sc.PR)}
		}
	} else {
		result, runErr := p.exec.Run(ctx, sc.WorkDir, "gh", "pr", "list", "--head", sc.Branch, "--state", "all", "--json", fields)
		out, err := strictCommandStdout(result, runErr)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return false, "", ctxErr
			}
			return false, "", fmt.Errorf("query pull requests for branch %s: %w", sc.Branch, err)
		}
		var matches []pullRequestRecord
		if err := json.Unmarshal([]byte(extractJSONArray(out)), &matches); err != nil {
			return false, "", fmt.Errorf("cannot parse pull requests for branch %s: %w", sc.Branch, err)
		}
		switch len(matches) {
		case 0:
			return false, fmt.Sprintf("no pull request exists for branch %s yet", sc.Branch), nil
		case 1:
			pr = matches[0]
		default:
			return false, "", &PRIdentityConflictError{Reason: fmt.Sprintf("branch %s matches %d pull requests", sc.Branch, len(matches))}
		}
	}
	if pr.Number == 0 {
		return false, "", &PRIdentityConflictError{Reason: fmt.Sprintf("branch %s resolved to zero PR identity", sc.Branch)}
	}
	if pr.BaseRefName != sc.BaseBranch || pr.HeadRefName != sc.Branch {
		gap := fmt.Sprintf("the pull request targets %s from %s; retarget it to %s from %s",
			pr.BaseRefName, pr.HeadRefName, sc.BaseBranch, sc.Branch)
		if recorded {
			return false, "", &PRIdentityConflictError{Reason: gap}
		}
		return false, gap, nil
	}
	if p.cfg.RemoteFlow.LinkIssue && sc.Issue != 0 {
		link := fmt.Sprintf("Closes #%d", sc.Issue)
		if !strings.Contains(pr.Body, link) {
			return false, fmt.Sprintf("the PR body does not contain %q; edit the PR body (gh pr edit %d --body ...) to include it", link, pr.Number), nil
		}
	}
	sc.PR = pr.Number
	return true, "", nil
}

// extractJSONArray returns the first top-level [...] block in s so banners
// or prose around gh output do not break parsing.
func extractJSONArray(s string) string {
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}
