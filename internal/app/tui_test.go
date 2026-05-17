package app

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedirectTUILogsWritesToSessionLogAndRestoresOutput(t *testing.T) {
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	var terminal bytes.Buffer
	log.SetOutput(&terminal)
	log.SetFlags(0)
	log.SetPrefix("")

	sessionDir := t.TempDir()
	restore := redirectTUILogs(sessionDir)
	log.Print("engine log should not reach terminal")
	restore()
	log.Print("terminal log restored")

	if strings.Contains(terminal.String(), "engine log should not reach terminal") {
		t.Fatalf("TUI log leaked to terminal buffer: %q", terminal.String())
	}
	if !strings.Contains(terminal.String(), "terminal log restored") {
		t.Fatalf("logger output was not restored: %q", terminal.String())
	}

	data, err := os.ReadFile(filepath.Join(sessionDir, "tui.log"))
	if err != nil {
		t.Fatalf("ReadFile(tui.log) error = %v", err)
	}
	if !strings.Contains(string(data), "engine log should not reach terminal") {
		t.Fatalf("tui.log missing redirected log: %q", string(data))
	}
}
