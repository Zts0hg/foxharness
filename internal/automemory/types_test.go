package automemory

import (
	"strings"
	"testing"
)

func TestParseMemoryValidFrontmatter(t *testing.T) {
	raw := `---
name: user-role
description: The user is a staff engineer who prefers terse answers.
type: user
---

The user is a staff backend engineer.
`
	mem, err := ParseMemory([]byte(raw))
	if err != nil {
		t.Fatalf("ParseMemory() error = %v", err)
	}
	if mem.Name != "user-role" {
		t.Fatalf("Name = %q, want user-role", mem.Name)
	}
	if mem.Type != TypeUser {
		t.Fatalf("Type = %q, want user", mem.Type)
	}
	if !strings.Contains(mem.Body, "staff backend engineer") {
		t.Fatalf("Body missing content: %q", mem.Body)
	}
	if err := mem.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestParseMemoryMissingFrontmatter(t *testing.T) {
	if _, err := ParseMemory([]byte("no frontmatter here")); err == nil {
		t.Fatalf("ParseMemory() expected error for missing frontmatter")
	}
}

func TestParseMemoryRequiresExactClosingDelimiter(t *testing.T) {
	raw := `---
name: user-role
description: d
type: user
---not-a-delimiter
body
`
	if _, err := ParseMemory([]byte(raw)); err == nil {
		t.Fatalf("ParseMemory() expected error for non-delimiter closing line")
	}
}

func TestValidateRejectsMissingFields(t *testing.T) {
	cases := map[string]Memory{
		"missing name":        {Description: "d", Type: TypeUser, Body: "b"},
		"missing description": {Name: "n", Type: TypeUser, Body: "b"},
		"invalid type":        {Name: "n", Description: "d", Type: MemoryType("bogus"), Body: "b"},
	}
	for name, mem := range cases {
		if err := mem.Validate(); err == nil {
			t.Fatalf("%s: Validate() expected error", name)
		}
	}
}

func TestValidateRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"../outside", "a/b", "..", "foo/../bar", `a\b`, "/abs", "bad\nname", "bad](", "has space", strings.Repeat("a", 129)} {
		mem := Memory{Name: name, Description: "d", Type: TypeUser, Body: "b"}
		if err := mem.Validate(); err == nil {
			t.Fatalf("Validate(name=%q) expected a path-safety error", name)
		}
	}
	for _, name := range []string{"user-role", "feedback-no-mock-db", "mem-0001", "ref_dash"} {
		mem := Memory{Name: name, Description: "d", Type: TypeUser, Body: "b"}
		if err := mem.Validate(); err != nil {
			t.Fatalf("Validate(name=%q) unexpected error: %v", name, err)
		}
	}
}

func TestValidateRequiresWhyAndHowToApply(t *testing.T) {
	for _, typ := range []MemoryType{TypeFeedback, TypeProject} {
		missing := Memory{Name: "n", Description: "d", Type: typ, Body: "just a rule, no structure"}
		if err := missing.Validate(); err == nil {
			t.Fatalf("type %s: Validate() expected error when Why/How to apply absent", typ)
		}
		ok := Memory{
			Name:        "n",
			Description: "d",
			Type:        typ,
			Body:        "The rule.\n\n**Why:** because.\n**How to apply:** do it.",
		}
		if err := ok.Validate(); err != nil {
			t.Fatalf("type %s: Validate() unexpected error = %v", typ, err)
		}
	}
	// user/reference do not require the structure.
	plain := Memory{Name: "n", Description: "d", Type: TypeReference, Body: "https://example.com"}
	if err := plain.Validate(); err != nil {
		t.Fatalf("reference Validate() unexpected error = %v", err)
	}
}

func TestValidateRejectsOversizeContent(t *testing.T) {
	big := Memory{
		Name:        "n",
		Description: "d",
		Type:        TypeUser,
		Body:        strings.Repeat("x", MaxContentChars+1),
	}
	if err := big.Validate(); err == nil {
		t.Fatalf("Validate() expected error for body exceeding %d chars", MaxContentChars)
	}
}

// TestValidateContentCapCountsRunesNotBytes ensures the body cap is enforced in
// characters (runes), not UTF-8 bytes, so large non-ASCII (e.g. Chinese)
// memories under the character limit are not wrongly rejected.
func TestValidateContentCapCountsRunesNotBytes(t *testing.T) {
	// 40000 Chinese runes = 120000 bytes: within the character cap, over the
	// byte count, so it must be accepted.
	atLimit := Memory{
		Name:        "n",
		Description: "d",
		Type:        TypeUser,
		Body:        strings.Repeat("世", MaxContentChars),
	}
	if err := atLimit.Validate(); err != nil {
		t.Fatalf("Validate() at-rune-limit unexpected error: %v", err)
	}
	overLimit := Memory{
		Name:        "n",
		Description: "d",
		Type:        TypeUser,
		Body:        strings.Repeat("世", MaxContentChars+1),
	}
	if err := overLimit.Validate(); err == nil {
		t.Fatalf("Validate() expected error for body exceeding %d runes", MaxContentChars)
	}
}

func TestMarshalRoundTrips(t *testing.T) {
	mem := Memory{
		Name:        "feedback-tests",
		Description: "Run tests before claiming done.",
		Type:        TypeFeedback,
		Body:        "Always run go test.\n\n**Why:** avoids regressions.\n**How to apply:** run before reporting done.",
	}
	data, err := mem.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	got, err := ParseMemory(data)
	if err != nil {
		t.Fatalf("ParseMemory(Marshal()) error = %v", err)
	}
	if got.Name != mem.Name || got.Description != mem.Description || got.Type != mem.Type {
		t.Fatalf("round-trip frontmatter mismatch: got %+v want %+v", got, mem)
	}
	if strings.TrimSpace(got.Body) != strings.TrimSpace(mem.Body) {
		t.Fatalf("round-trip body mismatch:\n got %q\nwant %q", got.Body, mem.Body)
	}
}
