package mcpcmd

import (
	"testing"
)

func TestParseRequiresScopeAndPreservesArguments(t *testing.T) {
	if _, err := Parse([]string{"add", "browser", "--transport", "stdio", "--", "npx", "@playwright/mcp"}); err == nil {
		t.Fatal("implicit scope accepted")
	}
	c, err := Parse([]string{"add", "browser", "--scope", "project", "--transport", "stdio", "--", "npx", "a b"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Scope != "project" || c.Name != "browser" || string(c.Definition) == "" {
		t.Fatalf("bad command: %#v", c)
	}
	for _, args := range [][]string{{"status", "unexpected"}, {"wat"}, {"remove", "browser"}, {"login"}, {"status", "--scope", "typo"}} {
		if _, err := Parse(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
func TestSplitQuotedArguments(t *testing.T) {
	args, err := Split(`add browser --scope project --json '{"transport":"stdio","command":"a b"}'`)
	if err != nil || len(args) != 6 || args[5] != `{"transport":"stdio","command":"a b"}` {
		t.Fatalf("%#v %v", args, err)
	}
	if _, err := Split(`add 'oops`); err == nil {
		t.Fatal("unclosed quote")
	}
}
