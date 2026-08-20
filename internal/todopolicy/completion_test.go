package todopolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompletionReminderReturnsCurrentIncompleteItemsOnly(t *testing.T) {
	root := t.TempDir()
	content := "# TODO\n- [x] done\n- [ ] first item\n- [ ] not recorded\n- [ ] second item\n"
	if err := os.WriteFile(filepath.Join(root, "TODO.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := CompletionReminder(root, true)
	for _, fragment := range []string{"TODO.md still has incomplete checklist items", "- first item", "- second item"} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("reminder missing %q:\n%s", fragment, got)
		}
	}
	if strings.Contains(got, "not recorded") || strings.Contains(got, "done") {
		t.Fatalf("reminder contains ignored item:\n%s", got)
	}
}

func TestCompletionReminderRequiresUpdateCapabilityAndIncompleteFile(t *testing.T) {
	root := t.TempDir()
	if got := CompletionReminder(root, true); got != "" {
		t.Fatalf("missing TODO reminder = %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "TODO.md"), []byte("- [ ] task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CompletionReminder(root, false); got != "" {
		t.Fatalf("disabled capability reminder = %q", got)
	}
}
