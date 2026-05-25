package slash

import (
	"path/filepath"
	"testing"
)

func TestConditionalSkills_AddAndCheck(t *testing.T) {
	cs := NewConditionalSkills()
	cs.Add("go-test", []string{"*_test.go"}, &Command{Name: "go-test"})

	activated := cs.CheckAndActivate("loop_test.go", "")
	if len(activated) != 1 || activated[0] != "go-test" {
		t.Errorf("activated = %v", activated)
	}
}

func TestConditionalSkills_NoMatch(t *testing.T) {
	cs := NewConditionalSkills()
	cs.Add("go-test", []string{"*_test.go"}, &Command{Name: "go-test"})

	if got := cs.CheckAndActivate("main.go", ""); len(got) != 0 {
		t.Errorf("expected no activations, got %v", got)
	}
}

func TestConditionalSkills_DoubleStarWildcard(t *testing.T) {
	cs := NewConditionalSkills()
	cs.Add("nested", []string{"src/**/*.go"}, &Command{Name: "nested"})

	root := t.TempDir()
	abs := filepath.Join(root, "src", "deep", "nest", "file.go")
	if got := cs.CheckAndActivate(abs, root); len(got) != 1 {
		t.Errorf("expected ** to match nested, got %v", got)
	}
}

func TestConditionalSkills_QuestionMarkWildcard(t *testing.T) {
	cs := NewConditionalSkills()
	cs.Add("single", []string{"a?.go"}, &Command{Name: "single"})
	if got := cs.CheckAndActivate("a1.go", ""); len(got) != 1 {
		t.Errorf("? should match single char, got %v", got)
	}
}

func TestConditionalSkills_MultiplePatternsOR(t *testing.T) {
	cs := NewConditionalSkills()
	cs.Add("multi", []string{"*_test.go", "Makefile"}, &Command{Name: "multi"})

	if got := cs.CheckAndActivate("Makefile", ""); len(got) != 1 {
		t.Errorf("Makefile should match, got %v", got)
	}
	// Re-add since CheckAndActivate marked it activated; create new container.
	cs2 := NewConditionalSkills()
	cs2.Add("multi", []string{"*_test.go", "Makefile"}, &Command{Name: "multi"})
	if got := cs2.CheckAndActivate("loop_test.go", ""); len(got) != 1 {
		t.Errorf("_test.go should match, got %v", got)
	}
}

func TestConditionalSkills_AlreadyActivatedNotReturnedAgain(t *testing.T) {
	cs := NewConditionalSkills()
	cs.Add("g", []string{"*_test.go"}, &Command{Name: "g"})

	first := cs.CheckAndActivate("a_test.go", "")
	second := cs.CheckAndActivate("b_test.go", "")
	if len(first) != 1 {
		t.Errorf("first = %v", first)
	}
	if len(second) != 0 {
		t.Errorf("second should be empty, got %v", second)
	}
}

func TestConditionalSkills_RelativeToProjectRoot(t *testing.T) {
	cs := NewConditionalSkills()
	cs.Add("docs", []string{"docs/*.md"}, &Command{Name: "docs"})

	root := "/proj"
	if got := cs.CheckAndActivate("/proj/docs/foo.md", root); len(got) != 1 {
		t.Errorf("should match relative path, got %v", got)
	}
}

func TestConditionalSkills_Take(t *testing.T) {
	cs := NewConditionalSkills()
	cmd := &Command{Name: "x"}
	cs.Add("x", []string{"*.go"}, cmd)
	got := cs.Take("x")
	if got != cmd {
		t.Errorf("Take returned wrong cmd: %+v", got)
	}
	if cs.Len() != 0 {
		t.Errorf("Take should remove the entry, Len = %d", cs.Len())
	}
}

func TestRegistry_CheckConditional_ActivatesAndMoves(t *testing.T) {
	r := NewRegistry("").WithoutDiscovery()
	cmd := &Command{
		Type: CommandPrompt,
		Name: "go-test",
		Frontmatter: Frontmatter{
			UserInvocable: true,
			Paths:         []string{"*_test.go"},
		},
	}
	r.Register(cmd)
	// Dormant — not in active set.
	if _, ok := r.Lookup("go-test"); ok {
		t.Error("conditional skill should not be active until matched")
	}
	if !r.HasConditional() {
		t.Error("expected HasConditional to be true")
	}

	names := r.CheckConditional("loop_test.go")
	if len(names) != 1 || names[0] != "go-test" {
		t.Errorf("activations = %v", names)
	}
	if _, ok := r.Lookup("go-test"); !ok {
		t.Error("go-test should now be active")
	}
}

func TestConditionalSkills_Add_HigherPrecedenceWins(t *testing.T) {
	cs := NewConditionalSkills()
	userCmd := &Command{Name: "x", Source: SourceUser}
	projectCmd := &Command{Name: "x", Source: SourceProject}

	cs.Add("x", []string{"*.go"}, userCmd)
	cs.Add("x", []string{"*.md"}, projectCmd)

	got := cs.Take("x")
	if got != projectCmd {
		t.Errorf("expected project to win, got %+v", got)
	}
}

func TestConditionalSkills_Add_LowerPrecedenceIgnored(t *testing.T) {
	cs := NewConditionalSkills()
	projectCmd := &Command{Name: "x", Source: SourceProject}
	userCmd := &Command{Name: "x", Source: SourceUser}

	cs.Add("x", []string{"*.md"}, projectCmd)
	cs.Add("x", []string{"*.go"}, userCmd)

	got := cs.Take("x")
	if got != projectCmd {
		t.Errorf("lower-precedence user must not displace project: got %+v", got)
	}
}

func TestRegistry_CheckConditional_PrecedenceProtectsActive(t *testing.T) {
	r := NewRegistry("").WithoutDiscovery()

	// Active project command "shared" — no paths, so it lives in r.commands.
	active := &Command{
		Type:        CommandPrompt,
		Name:        "shared",
		Source:      SourceProject,
		Content:     "from project",
		Frontmatter: Frontmatter{UserInvocable: true},
	}
	r.Register(active)

	// Dormant lower-precedence user conditional skill of the same name.
	user := &Command{
		Type:    CommandPrompt,
		Name:    "shared",
		Source:  SourceUser,
		Content: "from user",
		Frontmatter: Frontmatter{
			UserInvocable: true,
			Paths:         []string{"*.go"},
		},
	}
	r.Register(user)

	activated := r.CheckConditional("trigger.go")
	if len(activated) != 0 {
		t.Errorf("user conditional must be suppressed by active project, got activated=%v", activated)
	}

	got, ok := r.Lookup("shared")
	if !ok {
		t.Fatal("shared missing after conditional check")
	}
	if got.Content != "from project" {
		t.Errorf("project command was overwritten: content=%q", got.Content)
	}
}

func TestRegistry_OnActivateCallback(t *testing.T) {
	r := NewRegistry("").WithoutDiscovery()
	var activated string
	r.OnActivate(func(c *Command) { activated = c.Name })

	r.Register(&Command{
		Type: CommandPrompt,
		Name: "x",
		Frontmatter: Frontmatter{
			UserInvocable: true,
			Paths:         []string{"*.txt"},
		},
	})
	r.CheckConditional("a.txt")
	if activated != "x" {
		t.Errorf("OnActivate not called, activated = %q", activated)
	}
}
