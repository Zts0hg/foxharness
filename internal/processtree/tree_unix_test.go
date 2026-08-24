//go:build unix

package processtree

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestUnixTreeKeepsOwnedGroupIdentityUntilClose(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	owned, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	tree := owned.(*unixTree)
	if tree.groupID == cmd.Process.Pid {
		t.Fatal("process leader remains the process-group identity owner")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(-tree.groupID, 0); err != nil {
		t.Fatalf("owned process group disappeared before Close: %v", err)
	}
	if err := tree.Close(time.Second); err != nil {
		t.Fatal(err)
	}
}
