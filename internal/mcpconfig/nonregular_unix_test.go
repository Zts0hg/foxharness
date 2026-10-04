//go:build unix

package mcpconfig

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestLoadRejectsFIFOAndSymlinkToFIFOWithoutBlocking(t *testing.T) {
	for _, scope := range []Scope{UserScope, ProjectScope} {
		for _, link := range []bool{false, true} {
			t.Run(string(scope)+map[bool]string{false: "/fifo", true: "/link"}[link], func(t *testing.T) {
				home, work := t.TempDir(), t.TempDir()
				base := work
				if scope == UserScope {
					base = home
				}
				dir := filepath.Join(base, ".foxharness")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				config, fifo := filepath.Join(dir, "mcp.json"), filepath.Join(dir, "mcp.json")
				if link {
					fifo = filepath.Join(dir, "pipe")
				}
				if err := syscall.Mkfifo(fifo, 0600); err != nil {
					t.Fatal(err)
				}
				if link {
					if err := os.Symlink(fifo, config); err != nil {
						t.Fatal(err)
					}
				}
				done := make(chan Snapshot, 1)
				go func() { done <- Load(home, work, nil) }()
				select {
				case s := <-done:
					if len(s.Errors) != 1 || !strings.Contains(s.Errors[0], "regular file") {
						t.Fatalf("nonregular input not explicit: %+v", s)
					}
				case <-time.After(200 * time.Millisecond):
					writer, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
					if err == nil {
						writer.Close()
					}
					<-done
					t.Fatal("FIFO blocked configuration loading")
				}
			})
		}
	}
}
