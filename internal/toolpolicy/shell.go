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
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Redirect, *syntax.ProcSubst, *syntax.CmdSubst, *syntax.ArithmExp:
			readOnly = false
		case *syntax.CallExpr:
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
		}
		return true
	})
	if readOnly {
		risk = RiskLow
	}
	return readOnly, risk, true
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
		return flag != "--pre" && !strings.HasPrefix(flag, "--pre=") && flag != "--pre-glob" && !strings.HasPrefix(flag, "--pre-glob=")
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
