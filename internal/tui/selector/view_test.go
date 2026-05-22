package selector

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
)

func TestSelectorListView(t *testing.T) {
	view := New(selectorMessages(), selectorCheckpointer{stats: &checkpoint.DiffStats{
		FilesChanged: 1,
		Insertions:   2,
		Deletions:    2,
		ChangedFiles: []string{"internal/xlsx_generator.py"},
	}}).View()
	plain := stripSelectorANSI(view)
	for _, want := range []string{
		"Rewind",
		"Restore the code and/or conversation to the point before...",
		"change the parser",
		"xlsx_generator.py +2 -2",
		"❯ (current)",
		"Enter to continue · Esc/Q to exit",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("list view missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "12:00") {
		t.Fatalf("list view should not render timestamps:\n%s", plain)
	}
}

func TestSelectorListViewNoCodeChanges(t *testing.T) {
	view := New(selectorMessages(), selectorCheckpointer{stats: &checkpoint.DiffStats{}}).View()
	plain := stripSelectorANSI(view)
	if !strings.Contains(plain, "No code changes") {
		t.Fatalf("list view missing no-code-changes summary:\n%s", plain)
	}
}

func TestSelectorPreviewView(t *testing.T) {
	cp := selectorCheckpointer{stats: &checkpoint.DiffStats{
		FilesChanged: 1,
		Insertions:   2,
		Deletions:    3,
		ChangedFiles: []string{"internal/main.go"},
	}}
	m := New(selectorMessages(), cp)
	next, _ := m.Update(keyMsg("up"))
	m = next.(Model)
	next, _ = m.Update(keyMsg("enter"))
	m = next.(Model)
	plain := stripSelectorANSI(m.View())
	for _, want := range []string{
		"1 files changed, +2 -3",
		"internal/main.go",
		"Restore code and conversation",
		"Restore conversation only",
		"Restore code only",
		"Cancel",
		"Enter to choose · Esc back · Q to exit",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("preview view missing %q:\n%s", want, plain)
		}
	}
}

func stripSelectorANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inEsc {
			if ch >= '@' && ch <= '~' {
				inEsc = false
			}
			continue
		}
		if ch == 0x1b {
			inEsc = true
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}
