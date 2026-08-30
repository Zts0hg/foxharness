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

/* TestUnixTreeSignalsTheGroupAfterAnUnexpectedAnchorExit pins the cleanup
 * guarantee: the ownership anchor dying early is evidence, but the group
 * signal on cancellation still reaches every surviving member instead of
 * becoming a no-op. */
func TestUnixTreeSignalsTheGroupAfterAnUnexpectedAnchorExit(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	owned, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	tree := owned.(*unixTree)
	/* The anchor is the group leader; a plain-PID kill removes only the
	 * anchor and leaves the command's group running. */
	if err := syscall.Kill(tree.groupID, syscall.SIGKILL); err != nil {
		t.Fatalf("external anchor kill = %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := tree.Signal(true); err == nil {
		t.Fatal("unexpected anchor exit was not reported as evidence")
	}
	if err := tree.Close(2 * time.Second); err != nil {
		t.Fatalf("close after the group kill = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(-tree.groupID, 0); err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the process group survived the forced signal after the anchor died")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
