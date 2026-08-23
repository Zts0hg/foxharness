//go:build !unix && !windows

package processtree

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"time"
)

type processTree struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	forceCalled bool
}

func start(cmd *exec.Cmd) (Tree, error) {
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &processTree{cmd: cmd}, nil
}

func (tree *processTree) Signal(force bool) error {
	tree.mu.Lock()
	defer tree.mu.Unlock()
	if tree.cmd.Process == nil {
		return nil
	}
	if tree.forceCalled {
		return nil
	}
	var err error
	if force {
		err = tree.cmd.Process.Kill()
	} else {
		err = tree.cmd.Process.Signal(os.Interrupt)
	}
	if errors.Is(err, os.ErrProcessDone) {
		if force {
			tree.forceCalled = true
		}
		return nil
	}
	if err == nil && force {
		tree.forceCalled = true
	}
	return err
}

func (tree *processTree) Close(_ time.Duration) error {
	return tree.Signal(true)
}
