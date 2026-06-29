package configcmd

import (
	"io"
	"strings"
	"testing"
)

func TestTerminalPrompterLineReturnsDefaultOnEmpty(t *testing.T) {
	p := newTerminalPrompter(strings.NewReader("\n"), io.Discard, -1)
	got, err := p.Line("name", "default-id")
	if err != nil {
		t.Fatalf("Line() error = %v", err)
	}
	if got != "default-id" {
		t.Fatalf("Line() = %q, want default on empty input", got)
	}
}

func TestTerminalPrompterLineReturnsValue(t *testing.T) {
	p := newTerminalPrompter(strings.NewReader("  my-id \n"), io.Discard, -1)
	got, err := p.Line("name", "default-id")
	if err != nil {
		t.Fatalf("Line() error = %v", err)
	}
	if got != "my-id" {
		t.Fatalf("Line() = %q, want trimmed my-id", got)
	}
}

func TestTerminalPrompterChooseParsesIndex(t *testing.T) {
	p := newTerminalPrompter(strings.NewReader("2\n"), io.Discard, -1)
	idx, err := p.Choose("pick", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if idx != 1 {
		t.Fatalf("Choose() = %d, want 1 (b)", idx)
	}
}

func TestTerminalPrompterChooseRetriesOnInvalid(t *testing.T) {
	p := newTerminalPrompter(strings.NewReader("9\nfoo\n1\n"), io.Discard, -1)
	idx, err := p.Choose("pick", []string{"a", "b"})
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if idx != 0 {
		t.Fatalf("Choose() = %d, want 0 after retries", idx)
	}
}

func TestTerminalPrompterYesNo(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"\n", true, true},
		{"\n", false, false},
		{"junk\n", true, false},
	}
	for _, tc := range cases {
		p := newTerminalPrompter(strings.NewReader(tc.in), io.Discard, -1)
		got, err := p.YesNo("ok?", tc.def)
		if err != nil {
			t.Fatalf("YesNo(%q) error = %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("YesNo(%q) def=%v = %v, want %v", tc.in, tc.def, got, tc.want)
		}
	}
}

// fakePrompter returns scripted responses in call order. It is shared by the
// wizard tests to drive the flow without a real terminal.
type fakePrompter struct {
	lines   []string
	secrets []string
	choices []int
	yesnos  []bool

	li, si, ci, yi int
}

func (f *fakePrompter) Line(prompt, def string) (string, error) {
	v := ""
	if f.li < len(f.lines) {
		v = f.lines[f.li]
		f.li++
	}
	if v == "" {
		return def, nil
	}
	return v, nil
}

func (f *fakePrompter) Secret(prompt string) (string, error) {
	v := ""
	if f.si < len(f.secrets) {
		v = f.secrets[f.si]
		f.si++
	}
	return v, nil
}

func (f *fakePrompter) Choose(prompt string, options []string) (int, error) {
	v := 0
	if f.ci < len(f.choices) {
		v = f.choices[f.ci]
		f.ci++
	}
	return v, nil
}

func (f *fakePrompter) YesNo(prompt string, def bool) (bool, error) {
	v := def
	if f.yi < len(f.yesnos) {
		v = f.yesnos[f.yi]
		f.yi++
	}
	return v, nil
}

func TestFakePrompterReturnsScriptedResponses(t *testing.T) {
	f := &fakePrompter{
		lines:   []string{"my-id"},
		secrets: []string{"shh"},
		choices: []int{2},
		yesnos:  []bool{true},
	}
	if got, _ := f.Line("n", ""); got != "my-id" {
		t.Errorf("Line = %q, want my-id", got)
	}
	if got, _ := f.Line("n", "d"); got != "d" {
		t.Errorf("Line default = %q, want d", got)
	}
	if got, _ := f.Secret("k"); got != "shh" {
		t.Errorf("Secret = %q, want shh", got)
	}
	if got, _ := f.Choose("p", []string{"a", "b", "c"}); got != 2 {
		t.Errorf("Choose = %d, want 2", got)
	}
	if got, _ := f.YesNo("p", false); got != true {
		t.Errorf("YesNo = %v, want true", got)
	}
}
