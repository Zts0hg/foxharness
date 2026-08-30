// Package processtree starts commands inside platform process-tree boundaries.
package processtree

import (
	"fmt"
	"os/exec"
	"time"
)

const treeExitPollInterval = 10 * time.Millisecond

// Tree owns the complete process tree rooted at one started command.
type Tree interface {
	Signal(force bool) error
	/* Release stops owning the tree without terminating it: the direct
	 * command has finished on its own, so detached survivors are allowed to
	 * keep running. */
	Release(timeout time.Duration) error
	Close(timeout time.Duration) error
}

// Start starts cmd only after installing the platform process-tree boundary.
func Start(cmd *exec.Cmd) (Tree, error) {
	return start(cmd)
}

func waitForTreeExit(timeout time.Duration, exited func() (bool, error)) error {
	if timeout <= 0 {
		timeout = time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for {
		done, err := exited()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("process tree was not reaped within %s", timeout)
		}
		delay := treeExitPollInterval
		if remaining < delay {
			delay = remaining
		}
		time.Sleep(delay)
	}
}
