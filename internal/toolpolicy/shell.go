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
// The verdict is name-based: it proves the process-group hygiene of the
// programs it knows, and cannot prove anything about an untrusted payload a
// known program executes or an arbitrary binary the workspace contains — a
// freshly written script is exactly as opaque as any accepted program name.
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
		case *syntax.DeclClause:
			// declare, export, typeset, and local set arbitrary environment
			// state — including PATH, which decides what every later command
			// word resolves to — so no declaration can be proven synchronous.
			synchronous = false
		case *syntax.CallExpr:
			sawCall = true
			if len(n.Assigns) > 0 && !inertAssignments(n.Assigns) {
				// An assignment prefix could name PATH, a loader variable, or
				// a tool's own configuration channel, any of which changes
				// what the command executes, so only provably inert names
				// pass.
				synchronous = false
				break
			}
			if len(n.Args) == 0 || !synchronousInvocation(n) {
				synchronous = false
			}
		}
		return true
	})
	return synchronous && sawCall, true
}

// inertAssignments reports whether every assignment prefix names a variable
// that cannot change what the command resolves or executes: locale, terminal
// geometry, and color selection. Everything else fails closed.
func inertAssignments(assigns []*syntax.Assign) bool {
	for _, assign := range assigns {
		if assign.Name == nil {
			return false
		}
		name := strings.ToLower(assign.Name.Value)
		switch {
		case name == "columns", name == "lines", name == "no_color",
			name == "term", name == "tz", name == "lang":
		case strings.HasPrefix(name, "lc_"):
		default:
			return false
		}
	}
	return true
}

// synchronousInvocation reports whether one simple command provably stays
// inside the supervised process group for its whole lifetime: the command
// word must be a plain literal naming a synchronous program, and a wrapper
// that executes another program named in its arguments must name a provably
// synchronous one.
func synchronousInvocation(call *syntax.CallExpr) bool {
	if !synchronousCommandName(call.Args[0]) {
		return false
	}
	command := strings.ToLower(baseCommandName(literalWord(call.Args[0])))
	if command == "git" {
		// Git executes repository-planted configuration programs — textconv
		// filters declared by .gitattributes, the fsmonitor daemon, hooks —
		// so no subcommand of an inspected-for tree can be proven
		// synchronous. The read-only tool keeps its own vetted git
		// classification.
		return false
	}
	if !wrapperCommands[command] {
		return true
	}
	if command == "find" || command == "gfind" {
		return findExecWordsAreSynchronous(call.Args)
	}
	// The remaining wrappers take the executed program from their arguments.
	// A word that is not a plain literal could name any program, and the
	// gate fails closed, so every argument must be a plain literal none of
	// which names a detached or scheduler program. A few wrappers take the
	// executed program through a named flag instead of a positional word, so
	// their execution flags are rejected outright.
	for _, arg := range call.Args[1:] {
		text := runtimeStaticText(arg)
		if text != "" {
			if wrapperExecFlagForbidden(command, text) {
				return false
			}
			if !wrapperClusterValueSynchronous(command, text) {
				return false
			}
		}
		if !synchronousProgramWord(arg) {
			return false
		}
	}
	return true
}

/* wrapperClusterValueSynchronous screens the value a bundled short-flag
 * cluster hands to the executed program: tar -zIsh executes sh, su -csh
 * executes sh. The value is the cluster remainder after the execution
 * letter, and it faces the same program screening as a positional word. */
func wrapperClusterValueSynchronous(command string, text string) bool {
	letter, ok := wrapperExecFlagLetters[command]
	if !ok || len(text) < 3 || text[0] != '-' || text[1] == '-' {
		return true
	}
	index := strings.Index(text[1:], letter)
	if index < 0 {
		return true
	}
	value := text[2+index:]
	if value == "" {
		return true
	}
	return programWordText(strings.ToLower(value))
}

// wrapperExecFlagLetters lists, per wrapper, the short option letter whose
// value is a program to execute.
var wrapperExecFlagLetters = map[string]string{
	"flock": "c", "script": "c", "su": "c", "tar": "I",
}

/* wrapperExecFlagForbidden reports whether one argument word names or binds
 * an execution flag of the wrapper. A long flag binds its value with an
 * equals sign, and a short flag bundles the value directly onto itself —
 * tar -Ish executes sh — so short flags match every spelling that starts
 * with them. */
func wrapperExecFlagForbidden(command string, text string) bool {
	for flag := range wrapperExecFlags[command] {
		if strings.HasPrefix(flag, "--") {
			if text == flag || strings.HasPrefix(text, flag+"=") {
				return true
			}
			continue
		}
		if strings.HasPrefix(text, flag) {
			return true
		}
	}
	return false
}

// wrapperExecFlags lists, per wrapper, the flags whose value is a program to
// execute. A positional argument reaches the argument screening, but these
// values would hide behind a flag name, so the flag itself is denied. The
// match is case-sensitive: tar's -I names the compression program while -i
// ignores zero blocks.
var wrapperExecFlags = map[string]map[string]bool{
	"tar": {
		"-I": true, "--to-command": true, "--use-compress-program": true,
	},
	"bsdtar": {
		"--to-command": true, "--use-compress-program": true,
	},
	"cpio": {
		"--to-command": true,
	},
	"gcpio": {
		"--to-command": true,
	},
	"gsort": {
		"--compress-program": true,
	},
	"gtar": {
		"-I": true, "--to-command": true, "--use-compress-program": true,
	},
	"sort": {
		"--compress-program": true,
	},
}

// wrapperCommands lists commands that execute another program named in their
// arguments, so a detached or scheduler command smuggled into the arguments
// escapes the killed process group exactly as if named directly. busybox and
// toybox are multi-call binaries whose first argument names the program.
var wrapperCommands = map[string]bool{
	"arch": true, "bsdtar": true, "bwrap": true, "busybox": true,
	"caffeinate": true, "chroot": true, "chrt": true, "cpio": true,
	"doas": true, "fakeroot": true, "faketime": true, "firejail": true,
	"find": true, "flock": true, "gcpio": true, "gfind": true, "gnice": true,
	"gruncon": true, "gstdbuf": true, "ionice": true, "ltrace": true,
	"nice": true, "nsenter": true, "numactl": true,
	"perf": true, "proot": true, "proxychains": true,
	"proxychains4": true, "runcon": true,
	"runuser": true, "script": true, "setarch": true, "setpriv": true,
	"gsort": true, "gtar": true, "socat": true, "sort": true, "sshpass": true, "strace": true, "sudo": true,
	"stdbuf": true,
	"su":     true, "systemd-nspawn": true, "tar": true, "taskset": true, "time": true,
	"toybox": true, "unshare": true, "valgrind": true, "watch": true,
}

// interpreterNamePrefixes lists interpreter and shell families whose
// versioned or variant binaries (python3.12, python3.13t, perl5, pypy3,
// php-cgi, ksh93, zsh-5.9, ghci-9.4, octave-cli, wish8.6, pwsh-preview)
// resolve to the same arbitrary-code capability, which includes detaching
// from the process group, so no such form can be proven synchronous. The
// match is prefix-only on purpose: suffix spellings are unbounded, while the
// cost of a false positive is one rejected command in a conservative gate.
// Short generic names (sh, env, open, trap) stay exact matches in
// detachedShellCommands so unrelated words sharing the prefix stay usable.
var interpreterNamePrefixes = []string{
	"awk", "bash", "cabal", "ccl", "chezscheme", "clisp", "clojure",
	"csc", "csh", "csi", "dash", "dart", "dotnet", "elixir", "emacs",
	"erb", "erl", "escript", "gforth", "ghc", "gdb", "gnuplot",
	"groovy", "guile", "ipython", "irb", "janet", "java", "jshell",
	"jrunscript", "julia", "kotlin", "ksh",
	"lua", "lisp", "lldb", "mono", "mysql", "newlisp", "node", "npm",
	"npx", "nvim", "ocaml", "octave", "perl", "php", "pnpm", "psql",
	"python", "pypy", "pwsh", "racket", "runghc", "runhaskell", "ruby",
	"scala", "scheme", "sqlite", "stack", "swift", "tclsh", "tsx",
	"ts-node", "vim", "wish", "yarn", "zsh",
}

// isDetachedOrInterpreterCommand reports whether a base command name is a
// detached form, a scheduler or launcher family, or an interpreter. Wrapper
// commands are deliberately excluded: executing one is allowed, naming a
// detached program in its execution positions is not.
func isDetachedOrInterpreterCommand(name string) bool {
	if detachedShellCommands[name] {
		return true
	}
	for _, prefix := range interpreterNamePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// findExecWordsAreSynchronous checks the words find uses to execute another
// program. The execution flag is matched on its runtime text so quoted or
// escaped spellings cannot hide it, and the whole command span after the
// flag — the executed program and its arguments, up to the ; or + terminator
// — must be plain literals none of which names a detached, scheduler, or
// wrapper program, so wrapper chains cannot reach a detached program through
// the executed command's arguments.
func findExecWordsAreSynchronous(args []*syntax.Word) bool {
	span := false
	for _, arg := range args[1:] {
		text := strings.ToLower(runtimeStaticText(arg))
		if !span {
			switch text {
			case "-exec", "-execdir", "-ok", "-okdir":
				span = true
			case "":
				// A word whose runtime text is unknowable could expand to
				// an execution flag, which would hide the executed program
				// from every check below, so it fails closed.
				return false
			default:
				// An unquoted word carrying brace or glob metacharacters
				// expands into different or multiple runtime words, so its
				// static text proves nothing about what find will receive.
				if wordBraceOrGlobProne(arg) {
					return false
				}
			}
			continue
		}
		if text == ";" || text == "+" {
			// A terminator ends this executed span; find allows further
			// execution flags after it, so scanning continues.
			span = false
			continue
		}
		if text == "{}" {
			continue
		}
		if !isPlainCommandName(text) {
			return false
		}
		base := baseCommandName(text)
		if isDetachedOrInterpreterCommand(base) || wrapperCommands[base] {
			return false
		}
	}
	return true
}

// synchronousCommandName reports whether a command word names a synchronous
// program. The word must be a single unquoted literal whose characters are
// all plain command-name characters: quoted, escaped, brace, and glob forms
// expand to a runtime name different from their literal text, so they cannot
// be proven synchronous and are rejected. Absolute and relative paths resolve
// to their base name before the detached-command lookup, and a word that
// names no executable at all (a trailing slash) is rejected outright.
func synchronousCommandName(word *syntax.Word) bool {
	name := strings.ToLower(literalWord(word))
	if !isPlainCommandName(name) {
		return false
	}
	base := baseCommandName(name)
	if base == "" {
		return false
	}
	return !isDetachedOrInterpreterCommand(base)
}

// synchronousProgramWord reports whether an argument word could name a
// program a wrapper executes. Path operands that name no executable
// (".", "..", or a trailing slash) cannot smuggle a program and pass; a word
// that is not a plain literal could name anything and fails closed; a plain
// literal fails only when its base name is a detached or scheduler program.
func synchronousProgramWord(word *syntax.Word) bool {
	return programWordText(strings.ToLower(literalWord(word)))
}

/* programWordText classifies one runtime program word: a word that is not a
 * plain literal could name any program and fails closed, path operands that
 * name no executable pass, and everything else must not name a detached or
 * interpreter program. */
func programWordText(name string) bool {
	if !isPlainCommandName(name) {
		return false
	}
	base := baseCommandName(name)
	if base == "" || base == "." || base == ".." {
		return true
	}
	return !isDetachedOrInterpreterCommand(base)
}

// isPlainCommandName reports whether the name consists only of characters
// bash treats literally in a command word, so the runtime command name
// cannot differ from the literal text.
func isPlainCommandName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-' || r == '/' || r == '+':
		default:
			return false
		}
	}
	return true
}

func baseCommandName(name string) string {
	if index := strings.LastIndex(name, "/"); index >= 0 {
		return name[index+1:]
	}
	return name
}

// detachedShellCommands lists shell forms that escape the supervised process
// tree by design: interpreters and nested shells (including the Korn and
// POSIX family members mksh, oksh, yash, and posh), session multiplexers
// (screen, tmux, dtach, abduco) that park their payload in a detached
// session, remote-execution clients (ssh, autossh) whose payload never runs
// inside this process group, scheduler and launcher families (at/batch,
// cron, systemd-run, launchctl, schtasks, open, xdg-open, gio,
// start-stop-daemon) whose payload is executed by a system daemon,
// LaunchServices, or a detached service outside the killed process group,
// and structural escapers such as timeout that relocate themselves and their
// payload into a new process group.
// The list is closed on purpose: supervised Bash rejects anything it cannot
// prove synchronous, so a missed name weakens the cancellation guarantee and
// must be added as soon as a family is identified.
var detachedShellCommands = map[string]bool{
	".": true, "abduco": true, "anacron": true, "ash": true, "at": true,
	"atq": true, "atrm": true, "autossh": true, "bash": true,
	"batch": true, "bbsh": true, "bg": true, "bmake": true, "brew": true,
	"builtin": true, "bun": true, "byobu": true, "command": true,
	"coproc": true, "crond": true, "crontab": true, "csh": true,
	"daemon": true, "dash": true,
	"deno": true, "disown": true, "docker": true, "dtach": true,
	"dvtm": true, "elvish": true, "env": true, "es": true, "eval": true,
	"daemonize": true, "dc": true, "exec": true, "ex": true,
	"expect": true, "fish": true, "gawk": true,
	"ghci": true, "gio": true, "gmake": true, "gpg-agent": true,
	"jjs": true, "jruby": true,
	"kill": true, "killall": true, "ksh": true, "kubectl": true,
	"launchctl": true, "lftp": true, "lksh": true, "make": true,
	"mapfile": true, "mawk": true, "mksh": true, "mosh": true,
	"nc": true, "ncat": true, "netcat": true,
	"nawk": true, "nodejs": true, "nohup": true, "nu": true,
	"pdksh": true, "parallel": true, "pkill": true, "prlimit": true,
	"octave": true, "oksh": true, "open": true, "osascript": true,
	"osh": true, "podman": true, "posh": true, "powershell": true,
	"psql": true, "pwsh": true, "pythonw": true, "r": true, "rc": true,
	"readarray": true, "rbash": true, "rksh": true, "rsh": true, "rsync": true, "rscript": true,
	"rvim": true, "rview": true, "sash": true, "sbcl": true, "scp": true,
	"sed":    true,
	"screen": true, "schtasks": true, "setsid": true, "sftp": true,
	"sh": true, "sh.exe": true, "shopt": true, "source": true,
	"ssh": true, "ssh-agent": true,
	"start-stop-daemon": true, "systemctl": true, "systemd-run": true,
	"tcsh": true, "timeout": true, "tmux": true, "trap": true,
	"vi": true, "view": true, "wish": true, "xdg-open": true,
	"xargs": true, "zellij": true,
	"xonsh": true,
	"genv":  true, "gnohup": true, "gtimeout": true, "gxargs": true,
	"yash": true,
	"zsh":  true,
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
		/* Escaped literals carry their runtime text after backslash
		 * resolution, so the unescaped form is what the read-only tables and
		 * flag checks must classify. */
		text := unescapeShellText(literalWord(arg))
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
	case "-L", "-H", "-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls":
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

// runtimeStaticText returns the text a word contributes at runtime when all
// of its parts are literal or quoted-literal, with quotes removed and
// backslash escapes resolved to the escaped character. It returns "" when
// any part is a dynamic expansion whose value is unknowable at review time.
func runtimeStaticText(word *syntax.Word) string {
	var b strings.Builder
	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(unescapeShellText(p.Value))
		case *syntax.SglQuoted:
			if p.Dollar && strings.Contains(p.Value, "\\") {
				// ANSI-C quoting decodes backslash escapes at runtime, so
				// the literal text with escapes is not the runtime text; a
				// form like $'\x2dexec' would hide a decoded flag. Escape
				// content is unknowable here and fails closed.
				return ""
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return ""
				}
				b.WriteString(lit.Value)
			}
		default:
			return ""
		}
	}
	return b.String()
}

// wordBraceOrGlobProne reports whether an unquoted part of the word carries
// brace or glob metacharacters, so bash expands it into different or
// multiple runtime words regardless of its literal text. Quoted and dynamic
// parts are classified by their own rules.
func wordBraceOrGlobProne(word *syntax.Word) bool {
	for _, part := range word.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok {
			continue
		}
		if strings.ContainsAny(lit.Value, "{*?[") {
			return true
		}
	}
	return false
}

// unescapeShellText resolves backslash escapes the way bash resolves them in
func unescapeShellText(text string) string {
	if !strings.Contains(text, "\\") {
		return text
	}
	var b strings.Builder
	for index := 0; index < len(text); index++ {
		if text[index] == '\\' && index+1 < len(text) {
			b.WriteByte(text[index+1])
			index++
			continue
		}
		b.WriteByte(text[index])
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
