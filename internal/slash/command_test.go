package slash

import "testing"

func TestCommand_IsUserInvocable(t *testing.T) {
	cases := []struct {
		name     string
		cmd      Command
		expected bool
	}{
		{
			name:     "default is invocable",
			cmd:      Command{Frontmatter: Frontmatter{UserInvocable: true}},
			expected: true,
		},
		{
			name:     "explicitly false",
			cmd:      Command{Frontmatter: Frontmatter{UserInvocable: false}},
			expected: false,
		},
		{
			name:     "hidden takes precedence",
			cmd:      Command{Hidden: true, Frontmatter: Frontmatter{UserInvocable: true}},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cmd.IsUserInvocable(); got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestCommand_IsModelInvocable(t *testing.T) {
	cases := []struct {
		name     string
		cmd      Command
		expected bool
	}{
		{
			name:     "builtin is not model invocable by default",
			cmd:      Command{Type: CommandBuiltin},
			expected: false,
		},
		{
			name:     "prompt with disable false",
			cmd:      Command{Type: CommandPrompt, Frontmatter: Frontmatter{DisableModelInvocation: false}},
			expected: true,
		},
		{
			name:     "prompt with disable true",
			cmd:      Command{Type: CommandPrompt, Frontmatter: Frontmatter{DisableModelInvocation: true}},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cmd.IsModelInvocable(); got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestCommand_MatchesAlias(t *testing.T) {
	cmd := Command{Frontmatter: Frontmatter{Aliases: []string{"r", "rev"}}}

	if !cmd.MatchesAlias("r") {
		t.Error("expected alias r to match")
	}
	if !cmd.MatchesAlias("rev") {
		t.Error("expected alias rev to match")
	}
	if cmd.MatchesAlias("review") {
		t.Error("did not expect review to match aliases")
	}
	if cmd.MatchesAlias("R") {
		t.Error("alias matching is case-sensitive")
	}
}

func TestCommandType_Constants(t *testing.T) {
	if CommandBuiltin == CommandPrompt {
		t.Error("CommandBuiltin must be distinct from CommandPrompt")
	}
}

func TestCommandSource_Ordering(t *testing.T) {
	if !(SourceBuiltin < SourceUser && SourceUser < SourceProject) {
		t.Errorf("precedence ordering broken: builtin=%d user=%d project=%d", SourceBuiltin, SourceUser, SourceProject)
	}
}

func TestFrontmatter_Defaults(t *testing.T) {
	fm := Frontmatter{}
	if fm.Description != "" {
		t.Errorf("expected empty Description, got %q", fm.Description)
	}
	if len(fm.AllowedTools) != 0 {
		t.Errorf("expected empty AllowedTools, got %v", fm.AllowedTools)
	}
	if fm.Context != "" {
		t.Errorf("expected empty Context, got %q", fm.Context)
	}
}

func TestCommand_EmptyContent_StillValid(t *testing.T) {
	cmd := Command{Type: CommandPrompt, Name: "empty", Content: "", Frontmatter: Frontmatter{UserInvocable: true}}
	if cmd.Name != "empty" {
		t.Errorf("expected name 'empty', got %q", cmd.Name)
	}
	if !cmd.IsUserInvocable() {
		t.Error("empty content command should still be user-invocable")
	}
}
