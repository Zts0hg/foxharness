package configcmd

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Prompter abstracts the wizard's interactive I/O so the flow is unit-testable
// against a scripted fake instead of a real terminal.
type Prompter interface {
	// Line shows a prompt with an editable default and returns the trimmed
	// input, or def when the input is empty.
	Line(prompt, def string) (string, error)
	// Secret reads a value without echoing it back.
	Secret(prompt string) (string, error)
	// Choose lists options and returns the selected 0-based index.
	Choose(prompt string, options []string) (int, error)
	// YesNo asks for confirmation; def is returned on empty input.
	YesNo(prompt string, def bool) (bool, error)
}

// terminalPrompter is the production Prompter backed by a buffered reader and a
// writer. Secret input disables echo via the supplied file descriptor.
type terminalPrompter struct {
	in  *bufio.Reader
	out io.Writer
	fd  int
}

// newTerminalPrompter wraps the given input reader, prompt output writer, and
// file descriptor (used for no-echo secret reads).
func newTerminalPrompter(in io.Reader, out io.Writer, fd int) *terminalPrompter {
	return &terminalPrompter{in: bufio.NewReader(in), out: out, fd: fd}
}

func (p *terminalPrompter) Line(prompt, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", prompt, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", prompt)
	}
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	if value := strings.TrimSpace(line); value != "" {
		return value, nil
	}
	return def, nil
}

func (p *terminalPrompter) Secret(prompt string) (string, error) {
	fmt.Fprintf(p.out, "%s: ", prompt)
	raw, err := term.ReadPassword(p.fd)
	fmt.Fprintln(p.out)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func (p *terminalPrompter) Choose(prompt string, options []string) (int, error) {
	fmt.Fprintln(p.out, prompt)
	for i, opt := range options {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, opt)
	}
	for {
		fmt.Fprintf(p.out, "Select [1-%d]: ", len(options))
		line, err := p.in.ReadString('\n')
		if err != nil && line == "" {
			return 0, err
		}
		if n, perr := strconv.Atoi(strings.TrimSpace(line)); perr == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		fmt.Fprintf(p.out, "invalid choice; enter a number 1-%d\n", len(options))
	}
}

func (p *terminalPrompter) YesNo(prompt string, def bool) (bool, error) {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	fmt.Fprintf(p.out, "%s [%s]: ", prompt, hint)
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer == "" {
		return def, nil
	}
	return answer == "y" || answer == "yes", nil
}
