package autodev

import (
	"context"
	"encoding/json"
	"fmt"
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
	sc := &StageContext{
		Item:       Item{Title: item.Title, Description: item.Description},
		Slug:       item.Slug,
		WorkDir:    wt.Path,
		Branch:     wt.Branch,
		BaseBranch: p.cfg.BaseBranch,
		Remote:     p.cfg.Remote,
		Issue:      item.Issue,
		PR:         item.PR,
	}

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
		if runErr != nil {
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
			}
			if prChanged {
				it.PR = sc.PR
			}
		}); err != nil {
			return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, err
		}
		if issueChanged {
			p.reporter.OnIssue(ctx, sc.Issue)
		}
		if prChanged {
			p.reporter.OnPR(ctx, sc.PR)
		}
	}
	return PublishResult{Branch: wt.Branch, Issue: sc.Issue, PR: sc.PR}, nil
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
		steps = append(steps, Stage{
			Name: "issue",
			Prompt: func(sc *StageContext) string {
				return fmt.Sprintf("Create a GitHub issue documenting this requirement before the PR is opened: "+
					"run `gh issue create --title %q --body <a concise summary of the requirement and what was implemented>`.",
					sc.Item.Title)
			},
			Skip: func(ctx context.Context, sc *StageContext) bool {
				if sc.Issue != 0 {
					p.reporter.OnIssue(ctx, sc.Issue)
					return true
				}
				return false
			},
			Verify: p.verifyIssue(),
		})
	}
	if p.cfg.RemoteFlow.OpenPR {
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
			Skip: func(ctx context.Context, sc *StageContext) bool {
				if sc.PR != 0 {
					p.reporter.OnPR(ctx, sc.PR)
					return true
				}
				return false
			},
			Verify: p.verifyPR(),
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
		result, runErr := p.exec.Run(ctx, sc.WorkDir, "gh", "issue", "list",
			"--state", "all", "--search", sc.Item.Title, "--json", "number,title", "--limit", "20")
		out, err := strictCommandStdout(result, runErr)
		if err != nil {
			return false, fmt.Sprintf("cannot query issues via gh: %v", err)
		}
		var issues []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
		}
		if err := json.Unmarshal([]byte(extractJSONArray(out)), &issues); err != nil {
			return false, fmt.Sprintf("cannot parse gh issue list output: %v", err)
		}
		for _, issue := range issues {
			if issue.Title == sc.Item.Title {
				sc.Issue = issue.Number
				return true, ""
			}
		}
		return false, fmt.Sprintf("no GitHub issue titled %q exists yet", sc.Item.Title)
	}
}

// verifyPR queries gh for the branch's PR and enforces the Closes #N link
// when configured (TC-012). Publish persists and reports the verified number.
func (p *RemotePublisher) verifyPR() func(ctx context.Context, sc *StageContext) (bool, string) {
	return func(ctx context.Context, sc *StageContext) (bool, string) {
		result, runErr := p.exec.Run(ctx, sc.WorkDir, "gh", "pr", "view", sc.Branch, "--json", "number,body")
		out, err := strictCommandStdout(result, runErr)
		if err != nil {
			return false, fmt.Sprintf("no pull request exists for branch %s yet", sc.Branch)
		}
		var pr struct {
			Number int    `json:"number"`
			Body   string `json:"body"`
		}
		if err := json.Unmarshal([]byte(extractJSON(out)), &pr); err != nil || pr.Number == 0 {
			return false, fmt.Sprintf("cannot parse gh pr view output for branch %s", sc.Branch)
		}
		if p.cfg.RemoteFlow.LinkIssue && sc.Issue != 0 {
			link := fmt.Sprintf("Closes #%d", sc.Issue)
			if !strings.Contains(pr.Body, link) {
				return false, fmt.Sprintf("the PR body does not contain %q; edit the PR body (gh pr edit %d --body ...) to include it", link, pr.Number)
			}
		}
		sc.PR = pr.Number
		return true, ""
	}
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
