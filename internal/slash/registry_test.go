package slash

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func newProjectCmd(name string) *Command {
	return &Command{
		Type:        CommandPrompt,
		Name:        name,
		Source:      SourceProject,
		Frontmatter: Frontmatter{UserInvocable: true},
	}
}

func newUserCmd(name string) *Command {
	return &Command{
		Type:        CommandPrompt,
		Name:        name,
		Source:      SourceUser,
		Frontmatter: Frontmatter{UserInvocable: true},
	}
}

func newBuiltinCmd(name string) *Command {
	return &Command{
		Type:        CommandBuiltin,
		Name:        name,
		Source:      SourceBuiltin,
		Frontmatter: Frontmatter{UserInvocable: true},
	}
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry("")
	r.Register(newBuiltinCmd("help"))
	cmd, ok := r.Lookup("help")
	if !ok || cmd.Name != "help" {
		t.Fatalf("Lookup(help) = %+v, ok=%v", cmd, ok)
	}
}

func TestRegistry_LookupAlias(t *testing.T) {
	r := NewRegistry("")
	c := newProjectCmd("review")
	c.Frontmatter.Aliases = []string{"r", "rev"}
	r.Register(c)

	for _, alias := range []string{"r", "rev"} {
		got, ok := r.Lookup(alias)
		if !ok {
			t.Fatalf("alias %q not found", alias)
		}
		if got.Name != "review" {
			t.Errorf("alias %q -> %q", alias, got.Name)
		}
	}
}

func TestRegistry_LookupAliasHonorsPrecedence(t *testing.T) {
	r := NewRegistry("")
	builtin := newBuiltinCmd("builtin-review")
	builtin.Aliases = []string{"r"}
	user := newUserCmd("user-review")
	user.Frontmatter.Aliases = []string{"r"}
	project := newProjectCmd("project-review")
	project.Frontmatter.Aliases = []string{"r"}

	r.Register(builtin)
	r.Register(project)
	r.Register(user)

	got, ok := r.Lookup("r")
	if !ok {
		t.Fatal("alias r not found")
	}
	if got.Name != "project-review" {
		t.Errorf("alias r = %q, want project-review", got.Name)
	}
}

func TestRegistry_LookupAliasTieBreaksDeterministically(t *testing.T) {
	r := NewRegistry("")
	b := newProjectCmd("b-review")
	b.Frontmatter.Aliases = []string{"r"}
	a := newProjectCmd("a-review")
	a.Frontmatter.Aliases = []string{"r"}

	r.Register(b)
	r.Register(a)

	got, ok := r.Lookup("r")
	if !ok {
		t.Fatal("alias r not found")
	}
	if got.Name != "a-review" {
		t.Errorf("alias r = %q, want a-review", got.Name)
	}
}

func TestRegistry_Precedence(t *testing.T) {
	r := NewRegistry("")
	r.Register(newBuiltinCmd("help"))
	r.Register(newUserCmd("help"))
	r.Register(newProjectCmd("help"))

	got, _ := r.Lookup("help")
	if got.Source != SourceProject {
		t.Errorf("expected SourceProject, got %v", got.Source)
	}

	// Adding lower-precedence after higher-precedence should not displace.
	r.Register(newBuiltinCmd("help"))
	got, _ = r.Lookup("help")
	if got.Source != SourceProject {
		t.Errorf("lower-precedence overwrote higher: %v", got.Source)
	}
}

func TestRegistry_All(t *testing.T) {
	r := NewRegistry("")
	r.Register(newBuiltinCmd("a"))
	r.Register(newProjectCmd("b"))
	r.Register(newUserCmd("c"))

	all := r.All()
	if len(all) != 3 {
		t.Errorf("expected 3, got %d", len(all))
	}
}

func TestRegistry_UserInvocable_Filters(t *testing.T) {
	r := NewRegistry("")
	r.Register(newBuiltinCmd("yes"))
	hidden := newProjectCmd("no")
	hidden.Frontmatter.UserInvocable = false
	r.Register(hidden)

	got := r.UserInvocable()
	if len(got) != 1 || got[0].Name != "yes" {
		t.Errorf("UserInvocable = %+v", got)
	}
}

func TestRegistry_ModelInvocable_Filters(t *testing.T) {
	r := NewRegistry("")
	r.Register(newBuiltinCmd("builtin"))
	r.Register(newProjectCmd("yes"))
	internal := newProjectCmd("no")
	internal.Frontmatter.DisableModelInvocation = true
	r.Register(internal)

	got := r.ModelInvocable()
	if len(got) != 1 || got[0].Name != "yes" {
		t.Errorf("ModelInvocable = %+v", got)
	}
}

func TestRegistry_Load_DiscoversFromDisk(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "deploy.md"), "Deploy now")
	writeFile(t, filepath.Join(userHome, ".foxharness", "commands", "global.md"), "Global cmd")

	r := NewRegistry(workDir)
	r.WithUserHome(userHome)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	got, ok := r.Lookup("deploy")
	if !ok || got.Source != SourceProject {
		t.Errorf("deploy lookup failed: %+v ok=%v", got, ok)
	}
	got2, ok2 := r.Lookup("global")
	if !ok2 || got2.Source != SourceUser {
		t.Errorf("global lookup failed: %+v ok=%v", got2, ok2)
	}
}

func TestRegistry_Refresh_ReloadsFromDisk(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	dir := filepath.Join(workDir, ".foxharness", "commands")
	writeFile(t, filepath.Join(dir, "x.md"), "old")

	r := NewRegistry(workDir)
	r.WithUserHome(userHome)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := r.Lookup("x"); !ok {
		t.Fatal("x not loaded")
	}

	// Add a new file then refresh.
	writeFile(t, filepath.Join(dir, "y.md"), "new")
	if err := r.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if _, ok := r.Lookup("y"); !ok {
		t.Error("y not loaded after refresh")
	}

	// Remove a file then refresh — it should no longer be available.
	if err := os.Remove(filepath.Join(dir, "x.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := r.Refresh(); err != nil {
		t.Fatalf("Refresh2: %v", err)
	}
	if _, ok := r.Lookup("x"); ok {
		t.Error("x should be removed after Refresh")
	}
}

func TestRegistry_All_CacheReused(t *testing.T) {
	r := NewRegistry("")
	r.Register(newBuiltinCmd("a"))

	first := r.All()
	second := r.All()
	if len(first) != len(second) {
		t.Fatalf("len mismatch")
	}
	// Cache hit returns the same backing slice header.
	if &first[0] != &second[0] {
		t.Error("expected cached slice pointer reuse")
	}

	// Mutation invalidates the cache.
	r.Register(newBuiltinCmd("b"))
	third := r.All()
	if len(third) != 2 {
		t.Errorf("expected 2 after Register, got %d", len(third))
	}
}

func TestRegistry_All_SortedByName(t *testing.T) {
	r := NewRegistry("")
	r.Register(newBuiltinCmd("c"))
	r.Register(newBuiltinCmd("a"))
	r.Register(newBuiltinCmd("b"))
	all := r.All()
	names := make([]string, len(all))
	for i, c := range all {
		names[i] = c.Name
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	for i := range names {
		if names[i] != sorted[i] {
			t.Errorf("All() must be sorted: got %v want %v", names, sorted)
			break
		}
	}
}
