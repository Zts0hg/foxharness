package toolpolicy

import "testing"

func TestAssessShellRejectsRipgrepHelperExecutionOptions(t *testing.T) {
	workspace := t.TempDir()
	tests := []struct {
		name    string
		command string
	}{
		{name: "hostname executable attached", command: "rg --hostname-bin=./evil needle ."},
		{name: "hostname executable separate", command: "rg --hostname-bin ./evil needle ."},
		{name: "archive helpers long", command: "rg --search-zip needle ."},
		{name: "archive helpers short", command: "rg -z needle ."},
		{name: "archive helpers short cluster", command: "rg -nz needle ."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readOnly, _, parsed := AssessShell(tt.command, workspace, workspace)
			if !parsed {
				t.Fatalf("AssessShell(%q) parsed = false, want true", tt.command)
			}
			if readOnly {
				t.Fatalf("AssessShell(%q) readOnly = true, want false", tt.command)
			}
		})
	}
}

func TestAssessShellAllowsExplicitlyDisabledRipgrepArchiveSearch(t *testing.T) {
	workspace := t.TempDir()
	readOnly, _, parsed := AssessShell("rg --no-search-zip needle .", workspace, workspace)
	if !parsed || !readOnly {
		t.Fatalf("AssessShell() = readOnly %t, parsed %t; want true, true", readOnly, parsed)
	}
}

func TestAssessSynchronousShellRejectsDetachedSchedulerForms(t *testing.T) {
	commands := []struct {
		command     string
		synchronous bool
	}{
		{"go test ./...", true},
		{"ls -la", true},
		{"VAR=value go test ./...", true},
		{"sleep 1 &", false},
		{"nohup sleep 1", false},
		{"at now -f script.sh", false},
		{`"at" now -f script.sh`, false},
		{`'at' now`, false},
		{"/usr/bin/at now", false},
		{"./at now", false},
		{"/bin/bash -c 'sleep 1 &'", false},
		{"$(at now) x", false},
		{`"open" -a Terminal`, false},
		{"systemd-run --collect sleep 1", false},
		{"launchctl submit -l x -- sleep 1", false},
		{"schtasks /create /tn demo /tr calc", false},
		{"crontab -", false},
	}
	for _, item := range commands {
		t.Run(item.command, func(t *testing.T) {
			synchronous, parsed := AssessSynchronousShell(item.command)
			if !parsed {
				t.Fatalf("AssessSynchronousShell(%q) did not parse", item.command)
			}
			if synchronous != item.synchronous {
				t.Fatalf("AssessSynchronousShell(%q) = %v, want %v", item.command, synchronous, item.synchronous)
			}
		})
	}
}

func TestAssessSynchronousShellRejectsEscapeWrapperAndExpansionForms(t *testing.T) {
	rejected := []string{
		`\at now -f script.sh`,
		`\setsid sleep 1`,
		`/usr/bin/\at now`,
		`{a,}t now -f script.sh`,
		`a?t now`,
		`[a]t now`,
		"builtin eval 'payload'",
		"trap 'at now' EXIT",
		"nice at now",
		"timeout 5 at now",
		"stdbuf -o0 at now",
		"sudo at now",
		"xargs at now",
		"find . -name x -exec at now ;",
		"watch -n1 at now",
		"at/",
		`find . -e"x"ec grep pattern . ;`,
		"find . -exec nice at now ;",
		"taskset -c 0 setsid sleep 1000",
		"ionice -c 2 at now",
		"chrt -r 1 setsid sleep 1000",
		"script -qc 'setsid sleep 1000' /dev/null",
		"python3 -c 'print(1)'",
		"perl -e 'exit 0'",
		"node -e 'console.log(1)'",
		"awk '{print $1}' file",
		"xargs grep foo",
		"make deploy",
		`find . -maxdepth 1 -exec true \; -exec setsid sleep 5 {} \;`,
		"tcsh -c 'sleep 1'",
		"csh -c 'sleep 1'",
		"pythonw -c 'print(1)'",
		"pypy3 -c 'print(1)'",
		"julia script.jl",
		"busybox setsid sleep 5",
		"toybox setsid sleep 5",
		"gmake deploy",
		"bmake deploy",
		"wish script.tcl",
		"ghci script.hs",
		"sbcl --script script.lisp",
		"octave script.m",
		"xonsh script.xsh",
		"pwsh script.ps1",
		"Rscript script.R",
		"R -f script.R",
		"shopt -s expand_aliases",
		"python3.13t -c 'print(1)'",
		"julia-1.11 script.jl",
		"luajit script.lua",
		"nawk '{print}' file",
		"jruby script.rb",
		"powershell -Command Get-Date",
		"php-cgi script.php",
		"elvish script.elv",
		"find . $'\\x2dexec' sh -c 'setsid sleep 5' ;",
		"find . $'-exec' sh -c 'setsid sleep 5' ;",
		"gtimeout 5 sleep 1",
		"find . -name x -exec gtimeout 5 sleep 1 ;",
		"nice gtimeout 5 sleep 1",
		"genv gtimeout 5 sleep 1",
		"gnohup sleep 1",
		"genv gtimeout 5 sleep 1",
		"gfind . -name x -exec setsid sleep 5 ;",
		"gxargs at now",
		"doas timeout 5 sleep 1",
		"su root -c 'setsid sleep 5'",
		"chroot /tmp /usr/bin/timeout 5 sleep 1",
		"nsenter -t 1 timeout 5 sleep 1",
		"setpriv --reuid 0 timeout 5 sleep 1",
		"setarch -R timeout 5 sleep 1",
		"strace -f timeout 5 sleep 1",
		"caffeinate -i timeout 5 sleep 1",
		"runuser -u root timeout 5 sleep 1",
		"runcon system_u:r:cron_t:s0 setsid sleep 5",
		"gruncon system_u:r:cron_t:s0 setsid sleep 5",
		"unshare -f setsid sleep 5",
		"ltrace -f timeout 5 sleep 1",
		"valgrind timeout 5 sleep 1",
		"time -p timeout 5 sleep 1",
		"mapfile -C 'setsid sleep 5'",
		"readarray -e 'setsid sleep 5'",
		"arch -x86_64 screen -dm session 'setsid sleep 5'",
		"arch -arm64 at now",
		"timeout 300 go test ./...",
		"timeout 5 grep pattern file",
		"SUDO at now",
		"NICE python3 -c 'print(1)'",
		"FIND . -exec sh -c 'setsid sleep 5' ;",
		"find . \"$(echo -exec)\" sh -c 'setsid sleep 5' ;",
		"ksh93 script.ksh",
		"zsh-5.9 -c 'sleep 1'",
		"octave-cli script.m",
		"wish8.6 script.tcl",
		"ghci-9.4 script.hs",
		"pwsh-preview -Command Get-Date",
		`find . {,-exec} python3 -c 'import os;os.setsid()' ;`,
		"find . *(-exec) sh -c 'setsid sleep 5' ;",
	}
	accepted := []string{
		"go test ./...",
		"find . -name '*.go'",
		"nice -n 5 grep -r pattern .",
		"gnice sleep 1",
		"gstdbuf -o0 cat file",
		"arch -x86_64 ls -la",
		"sudo -l",
		"watch ls",
		"stdbuf -o0 cat file",
	}
	for _, command := range rejected {
		t.Run("reject "+command, func(t *testing.T) {
			if synchronous, parsed := AssessSynchronousShell(command); !parsed || synchronous {
				t.Fatalf("AssessSynchronousShell(%q) = %v, want rejected", command, synchronous)
			}
		})
	}
	for _, command := range accepted {
		t.Run("accept "+command, func(t *testing.T) {
			if synchronous, parsed := AssessSynchronousShell(command); !parsed || !synchronous {
				t.Fatalf("AssessSynchronousShell(%q) = %v, want accepted", command, synchronous)
			}
		})
	}
}

/* TestAssessShellRejectsFindWritePrimaries pins the read-only classification
 * of the GNU find primaries that write their report to a file: -fls belongs
 * to the same rejected class as -fprint, -fprint0, and -fprintf. */
func TestAssessShellRejectsFindWritePrimaries(t *testing.T) {
	workspace := t.TempDir()
	for _, command := range []string{
		"find . -fls pwn.txt",
		"find . -name x -fls pwn.txt",
		"find . -fprint pwn.txt",
		"find . -fprint0 pwn.txt",
		"find . -fprintf pwn.txt %p\\n",
	} {
		readOnly, _, parsed := AssessShell(command, workspace, workspace)
		if !parsed {
			t.Fatalf("AssessShell(%q) parsed = false, want true", command)
		}
		if readOnly {
			t.Fatalf("AssessShell(%q) readOnly = true, want false", command)
		}
	}
}
