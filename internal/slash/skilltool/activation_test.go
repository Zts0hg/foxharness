package skilltool

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/slash"
)

func TestFormatActivationReminderPreservesSkillMetadata(t *testing.T) {
	command := &slash.Command{
		Name: "review", Description: "Review changes",
		Frontmatter: slash.Frontmatter{WhenToUse: "after editing", ArgumentHint: "[path]"},
	}
	got := FormatActivationReminder(command)
	for _, fragment := range []string{"`review`", "Description: Review changes", "When to use: after editing", "Arguments: [path]", `name="review"`} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("activation reminder missing %q:\n%s", fragment, got)
		}
	}
	if got := FormatActivationReminder(nil); got != "" {
		t.Fatalf("nil reminder = %q", got)
	}
}
