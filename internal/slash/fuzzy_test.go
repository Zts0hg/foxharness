package slash

import "testing"

func TestScore(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		cName  string
		cDesc  string
		alias  []string
		expect int
	}{
		{"exact", "review", "review", "", nil, 100},
		{"prefix", "rev", "review", "", nil, 80},
		{"contains", "view", "review", "", nil, 60},
		{"alias exact (name no match)", "rv", "deploy", "", []string{"rv"}, 50},
		{"description contains", "code", "review", "review code now", nil, 20},
		{"no match", "xyz", "review", "code", nil, 0},
		{"empty query matches all", "", "review", "", nil, 100},
		{"case insensitive exact", "REVIEW", "review", "", nil, 100},
		{"case insensitive prefix", "REV", "review", "", nil, 80},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Score(tc.query, tc.cName, tc.cDesc, tc.alias)
			if got != tc.expect {
				t.Errorf("Score(%q, %q) = %d, want %d", tc.query, tc.cName, got, tc.expect)
			}
		})
	}
}

func TestFilterCommands(t *testing.T) {
	cmds := []*Command{
		{Name: "review", Frontmatter: Frontmatter{UserInvocable: true}},
		{Name: "run", Frontmatter: Frontmatter{UserInvocable: true}},
		{Name: "rebase", Frontmatter: Frontmatter{UserInvocable: true}},
		{Name: "test", Frontmatter: Frontmatter{UserInvocable: true}},
	}

	out := FilterCommands("re", cmds)
	gotNames := map[string]bool{}
	for _, c := range out {
		gotNames[c.Name] = true
	}
	if !gotNames["review"] || !gotNames["rebase"] {
		t.Errorf("expected 're' to match review and rebase, got %v", gotNames)
	}
	if gotNames["test"] {
		t.Errorf("test should not match 're'")
	}

	// Empty query returns all commands.
	if len(FilterCommands("", cmds)) != len(cmds) {
		t.Error("empty query should return all commands")
	}

	// No match returns empty.
	if len(FilterCommands("xyznope", cmds)) != 0 {
		t.Error("non-matching query should return empty")
	}
}

func TestFilterCommands_OrderedByScore(t *testing.T) {
	cmds := []*Command{
		{Name: "review", Frontmatter: Frontmatter{UserInvocable: true}},                           // contains "rev" (prefix score 80)
		{Name: "rev", Frontmatter: Frontmatter{UserInvocable: true}},                              // exact (100)
		{Name: "argument", Description: "rev rev", Frontmatter: Frontmatter{UserInvocable: true}}, // desc contains (20)
	}
	out := FilterCommands("rev", cmds)
	if len(out) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(out))
	}
	if out[0].Name != "rev" {
		t.Errorf("expected rev first, got %s", out[0].Name)
	}
	if out[1].Name != "review" {
		t.Errorf("expected review second, got %s", out[1].Name)
	}
}

func TestFilterCommands_TiesByName(t *testing.T) {
	cmds := []*Command{
		{Name: "zalpha", Frontmatter: Frontmatter{UserInvocable: true}},
		{Name: "aalpha", Frontmatter: Frontmatter{UserInvocable: true}},
		{Name: "malpha", Frontmatter: Frontmatter{UserInvocable: true}},
	}
	out := FilterCommands("alpha", cmds)
	if len(out) != 3 {
		t.Fatalf("expected 3, got %d", len(out))
	}
	if out[0].Name != "aalpha" || out[1].Name != "malpha" || out[2].Name != "zalpha" {
		t.Errorf("ties not broken alphabetically: %v %v %v", out[0].Name, out[1].Name, out[2].Name)
	}
}
