package automemory

import (
	"strings"
	"testing"
)

func TestGuardrailsContainSixElements(t *testing.T) {
	g := Guardrails()
	elements := map[string]string{
		"what-NOT-to-save":           "Do NOT save",
		"surprising/non-obvious":     "surprising or non-obvious",
		"drift caveat":               "possibly stale",
		"verify-before-recommending": "still exists",
		"ignore directive":           "ignore memory",
		"dedup-first":                "update the existing",
	}
	for label, marker := range elements {
		if !strings.Contains(g, marker) {
			t.Fatalf("guardrails missing %s element (marker %q):\n%s", label, marker, g)
		}
	}
}

func TestGuardrailSourceSharedAcrossVariants(t *testing.T) {
	g := Guardrails()
	main := MainMemoryGuidance("../../.foxharness/memory", "../../.foxharness/projects/key/memory")
	extraction := ExtractionInstructions("- [user] x.md: y", "../../.foxharness/memory", "../../.foxharness/projects/key/memory")

	if !strings.Contains(main, g) {
		t.Fatalf("main guidance does not reuse the shared guardrail source:\n%s", main)
	}
	if !strings.Contains(extraction, g) {
		t.Fatalf("extraction instructions do not reuse the shared guardrail source:\n%s", extraction)
	}
}

func TestMainGuidanceMentionsDirectoriesAndFrontmatter(t *testing.T) {
	main := MainMemoryGuidance("../../.foxharness/memory", "../../.foxharness/projects/key/memory")
	for _, want := range []string{
		"../../.foxharness/memory",
		"../../.foxharness/projects/key/memory",
		"name",
		"description",
		"type",
		"read_file",
		"empty content",
		"write_file",
	} {
		if !strings.Contains(main, want) {
			t.Fatalf("main guidance missing %q:\n%s", want, main)
		}
	}
}

func TestExtractionInstructionsEmbedManifest(t *testing.T) {
	manifest := "- [feedback] testing.md: run tests first"
	got := ExtractionInstructions(manifest, "../m", "../p")
	if !strings.Contains(got, manifest) {
		t.Fatalf("extraction instructions must embed the manifest:\n%s", got)
	}
}

func TestFrontmatterTemplateGuidesTypeChoice(t *testing.T) {
	tpl := frontmatterTemplate()
	for _, want := range []string{
		"Pick the type that best fits",
		"user — durable facts about the user",
		"feedback — guidance or corrections on how the agent should work",
		"project — this project's goals",
		"reference — pointers to external resources",
	} {
		if !strings.Contains(tpl, want) {
			t.Fatalf("frontmatterTemplate() missing type guide %q:\n%s", want, tpl)
		}
	}
}

func TestTypeGuideReachesBothWriteLayers(t *testing.T) {
	main := MainMemoryGuidance("../m", "../p")
	extraction := ExtractionInstructions("", "../m", "../p")
	for _, want := range []string{
		"how the agent should work",
		"pointers to external resources",
	} {
		if !strings.Contains(main, want) {
			t.Fatalf("main guidance missing type guide %q:\n%s", want, main)
		}
		if !strings.Contains(extraction, want) {
			t.Fatalf("extraction guidance missing type guide %q:\n%s", want, extraction)
		}
	}
}
