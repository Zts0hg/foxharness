package toolpolicy

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// AssessShell classifies a shell command and reports whether it parsed.
func AssessShell(command, workspace, cwd string) (readOnly bool, risk Risk, parsed bool) {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return false, RiskHigh, false
	}

	readOnly = true
	risk = RiskMedium
	sawCall := false
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Stmt:
			if n.Background || n.Coprocess {
				readOnly = false
			}
		case *syntax.Redirect, *syntax.ProcSubst, *syntax.CmdSubst, *syntax.ArithmExp:
			readOnly = false
		case *syntax.CallExpr:
			sawCall = true
			if !readOnlyCall(n, workspace, cwd) {
				readOnly = false
			}
			if len(n.Args) == 0 {
				return true
			}
			name := strings.ToLower(literalWord(n.Args[0]))
			switch name {
			case "rm", "sudo", "chmod", "chown":
				risk = RiskCritical
			case "curl":
				if risk != RiskCritical {
					risk = RiskHigh
				}
			case "git":
				if len(n.Args) > 1 {
					subcommand := strings.ToLower(literalWord(n.Args[1]))
					if (subcommand == "push" || subcommand == "commit") && risk != RiskCritical {
						risk = RiskHigh
					}
				}
			}
		default:
			if _, command := node.(syntax.Command); command {
				if _, binary := node.(*syntax.BinaryCmd); !binary {
					readOnly = false
				}
			}
		}
		return true
	})
	if !sawCall {
		readOnly = false
	}
	if readOnly {
		risk = RiskLow
	}
	return readOnly, risk, true
}

// AssessSynchronousShell reports whether a complete shell parse excludes
// background, detach, process-substitution, and nested interpreter forms.
func AssessSynchronousShell(command string) (synchronous bool, parsed bool) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return false, false
	}
	synchronous = true
	sawCall := false
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Stmt:
			if n.Background || n.Coprocess {
				synchronous = false
			}
		case *syntax.ProcSubst:
			synchronous = false
		case *syntax.CoprocClause:
			synchronous = false
		case *syntax.CallExpr:
			sawCall = true
			if len(n.Args) == 0 || !synchronousCommandName(n.Args[0]) {
				synchronous = false
			}
		}
		return true
	})
	return synchronous && sawCall, true
}

// synchronousCommandName reports whether a command word names a synchronous
// program. The word must be a single plain unquoted literal: quoted or
// expanded words could hide any command name and cannot be proven
// synchronous, so they are rejected. Absolute and relative paths resolve to
// their base name before the detached-command lookup.
func synchronousCommandName(word *syntax.Word) bool {
	name := strings.ToLower(literalWord(word))
	if name == "" {
		return false
	}
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	return !detachedShellCommands[name]
}

// detachedShellCommands lists shell forms that escape the supervised process
// tree by design: interpreters and nested shells, plus scheduler and launcher
// families (at/batch, cron, systemd-run, launchctl, schtasks, open) whose
// payload is executed by a system daemon or LaunchServices outside the killed
// process group, so a cancelled call could still produce later side effects.
// The list is closed on purpose: supervised Bash rejects anything it cannot
// prove synchronous, so a missed name weakens the cancellation guarantee and
// must be added as soon as a family is identified.
var detachedShellCommands = map[string]bool{
	".": true, "at": true, "atq": true, "atrm": true, "bash": true,
	"batch": true, "bg": true, "command": true, "coproc": true,
	"crontab": true, "daemon": true, "dash": true, "disown": true,
	"env": true, "eval": true, "exec": true, "fish": true,
	"ksh": true, "launchctl": true, "nohup": true, "open": true,
	"screen": true, "schtasks": true, "setsid": true, "sh": true,
	"source": true, "systemd-run": true, "tmux": true, "zsh": true,
}

func readOnlyCall(call *syntax.CallExpr, workspace, cwd string) bool {
	if len(call.Assigns) > 0 || len(call.Args) == 0 {
		return false
	}
	name := literalWord(call.Args[0])
	if name == "" || !readOnlyCommands[name] {
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
		if strings.ContainsAny(text, "*?[{") || strings.HasPrefix(text, "~") {
			return false
		}
		if pathOperandRequiresContainment(name, args, text) {
			scope, ok := PathScope(workspace, cwd, text)
			if !ok || scope != ScopeWorkspace {
				return false
			}
		}
	}
	return commandArgsAllowed(name, args)
}

var readOnlyCommands = map[string]bool{
	"cat": true, "find": true, "git": true, "grep": true,
	"head": true, "ls": true, "pwd": true, "rg": true,
	"tail": true, "test": true, "wc": true,
}

func flagAllowed(command, flag string) bool {
	if flag == "--" {
		return true
	}
	switch command {
	case "git":
		return gitFlagAllowed(flag)
	case "rg":
		return ripgrepFlagAllowed(flag)
	case "find":
		return !findDangerousArg(flag)
	default:
		return !strings.Contains(flag, "f") || command == "ls"
	}
}

func ripgrepFlagAllowed(flag string) bool {
	switch {
	case flag == "--pre", strings.HasPrefix(flag, "--pre="):
		return false
	case flag == "--pre-glob", strings.HasPrefix(flag, "--pre-glob="):
		return false
	case flag == "--hostname-bin", strings.HasPrefix(flag, "--hostname-bin="):
		return false
	case flag == "--search-zip", strings.HasPrefix(flag, "--search-zip="):
		return false
	case strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--"):
		return !strings.Contains(strings.TrimPrefix(flag, "-"), "z")
	default:
		return true
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

func pathOperandRequiresContainment(command string, args []string, arg string) bool {
	if strings.Contains(arg, "/") || strings.HasPrefix(arg, ".") {
		return true
	}
	switch command {
	case "cat", "head", "tail", "wc", "ls", "find", "test":
		return true
	case "grep", "rg":
		count := 0
		for _, value := range args {
			if !strings.HasPrefix(value, "-") {
				count++
			}
		}
		return count > 1
	default:
		return false
	}
}
