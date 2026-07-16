package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
)

// Risk is the coarse severity the reviewer and TUI display for a request.
type Risk = toolpolicy.Risk

const (
	RiskLow      = toolpolicy.RiskLow
	RiskMedium   = toolpolicy.RiskMedium
	RiskHigh     = toolpolicy.RiskHigh
	RiskCritical = toolpolicy.RiskCritical
)

// PolicyDecision classifies a request before reviewer or user approval.
type PolicyDecision struct {
	AllowFastPath  bool
	RequiresReview bool
	HumanOnly      bool
	Request        Request
	Reason         string
}

// Classify maps a tool-owned capability assessment into the deterministic
// permission path. Missing or invalid assessments fail closed to human review.
func Classify(workspace string, cwd string, source Source, call schema.ToolCall, assessments ...toolpolicy.Assessment) PolicyDecision {
	workspace = cleanPath(workspace)
	cwd = cleanPath(firstNonEmpty(cwd, workspace))
	assessment := missingAssessment(call.Name)
	if len(assessments) > 0 {
		assessment = normalizeAssessment(call.Name, assessments[0])
	}
	arguments := normalizeJSON(call.Arguments)
	if assessment.Behavior != toolpolicy.BehaviorFastAllow && assessment.Action == call.Name {
		assessment.Action = strings.TrimSpace(call.Name + " " + arguments)
	}
	req := Request{
		ToolCall:     call,
		ToolName:     call.Name,
		Arguments:    arguments,
		CWD:          cwd,
		Workspace:    workspace,
		Source:       source,
		Action:       assessment.Action,
		Risk:         assessment.RiskHint,
		Capabilities: assessment,
	}
	switch assessment.Behavior {
	case toolpolicy.BehaviorFastAllow:
		return PolicyDecision{AllowFastPath: true, Request: req, Reason: assessment.Reason}
	case toolpolicy.BehaviorReviewable:
		return PolicyDecision{RequiresReview: true, Request: req, Reason: assessment.Reason}
	default:
		return PolicyDecision{HumanOnly: true, Request: req, Reason: assessment.Reason}
	}
}

func normalizeAssessment(toolName string, assessment toolpolicy.Assessment) toolpolicy.Assessment {
	switch assessment.Behavior {
	case toolpolicy.BehaviorFastAllow, toolpolicy.BehaviorReviewable, toolpolicy.BehaviorHumanOnly:
	default:
		return missingAssessment(toolName)
	}
	if strings.TrimSpace(assessment.Action) == "" {
		assessment.Action = toolName
	}
	switch assessment.RiskHint {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
	default:
		assessment.Behavior = toolpolicy.BehaviorHumanOnly
		assessment.RiskHint = RiskHigh
		assessment.Reason = "tool capability risk is invalid"
	}
	if strings.TrimSpace(assessment.Reason) == "" {
		assessment.Reason = "tool capability assessment"
	}
	if assessment.Behavior != toolpolicy.BehaviorHumanOnly {
		if reason := invalidAssessmentReason(assessment); reason != "" {
			assessment.Behavior = toolpolicy.BehaviorHumanOnly
			assessment.RiskHint = RiskHigh
			assessment.Reason = reason
		}
	}
	return assessment
}

func invalidAssessmentReason(assessment toolpolicy.Assessment) string {
	if len(assessment.Effects) == 0 {
		return "tool capability effects are missing"
	}
	switch assessment.Scope {
	case toolpolicy.ScopeWorkspace, toolpolicy.ScopeExternal, toolpolicy.ScopeMixed:
	default:
		return "tool capability scope is invalid"
	}
	if assessment.Behavior == toolpolicy.BehaviorFastAllow && assessment.Scope != toolpolicy.ScopeWorkspace {
		return "fast-allow capability is not workspace-scoped"
	}
	hasDelegate := false
	hasExecute := false
	for _, effect := range assessment.Effects {
		switch effect {
		case toolpolicy.EffectObserve, toolpolicy.EffectMutate, toolpolicy.EffectExecute,
			toolpolicy.EffectNetwork, toolpolicy.EffectDelegate, toolpolicy.EffectWorkflow,
			toolpolicy.EffectInteract:
		default:
			return "tool capability effect is invalid"
		}
		if effect == toolpolicy.EffectDelegate {
			hasDelegate = true
		}
		if effect == toolpolicy.EffectExecute {
			hasExecute = true
		}
		if effect == toolpolicy.EffectMutate && assessment.ReadOnly {
			return "read-only capability declares mutation"
		}
	}
	if hasDelegate && !assessment.NestedEnforcement {
		return "delegating capability lacks nested permission enforcement"
	}
	if hasExecute && len(assessment.Commands) == 0 {
		return "executing capability does not identify commands"
	}
	return ""
}

func missingAssessment(toolName string) toolpolicy.Assessment {
	return toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorHumanOnly,
		Action:   toolName,
		Effects:  []toolpolicy.Effect{toolpolicy.EffectUnknown},
		Scope:    toolpolicy.ScopeUnknown,
		RiskHint: toolpolicy.RiskHigh,
		Reason:   "registered tool does not declare permission capabilities",
	}
}

// IsReadOnlyBash validates a conservative read-only subset of shell commands.
func IsReadOnlyBash(command string, workspace string, cwd string) bool {
	readOnly, _, parsed := toolpolicy.AssessShell(command, workspace, cwd)
	return parsed && readOnly
}

func readOnlyCall(call *syntax.CallExpr, workspace string, cwd string) bool {
	if len(call.Assigns) > 0 {
		return false
	}
	if len(call.Args) == 0 {
		return false
	}
	name := literalWord(call.Args[0])
	if name == "" || !readOnlyCommand[name] {
		return false
	}
	args := make([]string, 0, len(call.Args)-1)
	for _, arg := range call.Args[1:] {
		text := literalWord(arg)
		if text == "" {
			return false
		}
		args = append(args, text)
		if strings.HasPrefix(text, "-") {
			if !flagAllowed(name, text) {
				return false
			}
			continue
		}
		if strings.ContainsAny(text, "*?[{") {
			return false
		}
		if expandsOutsideWorkspace(text) {
			return false
		}
		if pathOperandRequiresContainment(name, args, text) && !containedInWorkspace(workspace, cwd, text) {
			return false
		}
	}
	if !commandArgsAllowed(name, args) {
		return false
	}
	return true
}

var readOnlyCommand = map[string]bool{
	"cat": true, "find": true, "git": true, "grep": true,
	"head": true, "ls": true, "pwd": true, "rg": true,
	"tail": true, "test": true, "wc": true,
}

func flagAllowed(command string, flag string) bool {
	if flag == "--" {
		return true
	}
	if strings.ContainsAny(flag, "wioxz") && (command == "sed" || command == "grep" || command == "rg") {
		return false
	}
	switch command {
	case "git":
		return gitFlagAllowed(flag)
	case "rg":
		return rgFlagAllowed(flag)
	case "find":
		return !findDangerousArg(flag)
	default:
		return !strings.Contains(flag, "f") || command == "ls"
	}
}

func commandArgsAllowed(command string, args []string) bool {
	switch command {
	case "git":
		return gitArgsAllowed(args)
	case "find":
		for _, arg := range args {
			if findDangerousArg(arg) {
				return false
			}
		}
	}
	return true
}

func rgFlagAllowed(flag string) bool {
	switch {
	case flag == "--pre", strings.HasPrefix(flag, "--pre="), flag == "--pre-glob", strings.HasPrefix(flag, "--pre-glob="):
		return false
	default:
		return true
	}
}

func gitArgsAllowed(args []string) bool {
	subcommand := ""
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		subcommand = arg
		break
	}
	switch subcommand {
	case "status", "diff", "log", "show", "rev-parse", "ls-files", "grep":
		return true
	default:
		return false
	}
}

func gitFlagAllowed(flag string) bool {
	if strings.Contains(flag, "=") {
		return false
	}
	switch flag {
	case "--", "--short", "--porcelain", "--branch", "--stat", "--name-only", "--name-status",
		"--cached", "--staged", "--color", "--no-color", "--oneline", "--decorate",
		"--all", "--remotes", "--tags", "--show-current", "--abbrev-ref", "--verify",
		"-s", "-b", "-p", "-u", "-U", "-M", "-C", "-n", "-1", "-2", "-3", "-4", "-5":
		return true
	default:
		return false
	}
}

func findDangerousArg(arg string) bool {
	switch arg {
	case "-L", "-H", "-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf":
		return true
	default:
		return false
	}
}

func literalWord(word *syntax.Word) string {
	var b strings.Builder
	for _, part := range word.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok {
			return ""
		}
		b.WriteString(lit.Value)
	}
	return b.String()
}

func looksLikePath(arg string) bool {
	return strings.Contains(arg, "/") || strings.HasPrefix(arg, ".")
}

func pathOperandRequiresContainment(command string, args []string, arg string) bool {
	if looksLikePath(arg) {
		return true
	}
	switch command {
	case "cat", "head", "tail", "wc", "ls", "find", "test":
		return true
	case "grep", "rg":
		return nonFlagArgs(args) > 1
	default:
		return false
	}
}

func nonFlagArgs(args []string) int {
	count := 0
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		count++
	}
	return count
}

func expandsOutsideWorkspace(arg string) bool {
	return strings.HasPrefix(arg, "~")
}

func containedInWorkspace(workspace string, cwd string, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	var full string
	if filepath.IsAbs(path) {
		full = cleanPath(path)
	} else {
		full = cleanPath(filepath.Join(cwd, path))
	}
	resolved, ok := resolvePathForContainment(full)
	if !ok {
		return false
	}
	full = resolved
	workspaceResolved, ok := resolvePathForContainment(workspace)
	if !ok {
		return false
	}
	workspace = workspaceResolved
	rel, err := filepath.Rel(workspace, full)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func resolvePathForContainment(path string) (string, bool) {
	path = cleanPath(path)
	existing := path
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", false
		}
		suffix = append([]string{filepath.Base(existing)}, suffix...)
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", false
	}
	parts := append([]string{resolved}, suffix...)
	return cleanPath(filepath.Join(parts...)), true
}

func toolPath(raw json.RawMessage) (string, bool) {
	var data struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", false
	}
	data.Path = strings.TrimSpace(data.Path)
	return data.Path, data.Path != ""
}

func bashCommand(raw json.RawMessage) string {
	var data struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(raw, &data)
	return strings.TrimSpace(data.Command)
}

func riskForBash(command string) Risk {
	risk := RiskMedium
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return fallbackRiskForBash(command)
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		name := strings.ToLower(literalWord(call.Args[0]))
		switch name {
		case "rm", "sudo", "chmod", "chown":
			risk = RiskCritical
			return false
		case "curl":
			if risk != RiskCritical {
				risk = RiskHigh
			}
		case "git":
			if len(call.Args) > 1 {
				subcommand := strings.ToLower(literalWord(call.Args[1]))
				if subcommand == "push" || subcommand == "commit" {
					if risk != RiskCritical {
						risk = RiskHigh
					}
				}
			}
		}
		return true
	})
	return risk
}

func fallbackRiskForBash(command string) Risk {
	lower := strings.ToLower(command)
	for _, needle := range []string{"rm", "sudo", "chmod", "chown"} {
		if strings.Contains(lower, needle) {
			return RiskCritical
		}
	}
	for _, needle := range []string{"git push", "git commit", "curl"} {
		if strings.Contains(lower, needle) {
			return RiskHigh
		}
	}
	return RiskMedium
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
