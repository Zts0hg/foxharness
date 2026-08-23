//go:build unix

package processtree

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type unixTree struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	forceCalled bool
	signalSent  bool
}

func start(cmd *exec.Cmd) (Tree, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &unixTree{cmd: cmd}, nil
}

func (tree *unixTree) Signal(force bool) error {
	tree.mu.Lock()
	defer tree.mu.Unlock()
	return tree.signalLocked(force)
}

func (tree *unixTree) signalLocked(force bool) error {
	if tree.cmd.Process == nil {
		return nil
	}
	if tree.forceCalled {
		return nil
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	err := syscall.Kill(-tree.cmd.Process.Pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		tree.forceCalled = true
		return nil
	}
	if force && tree.signalSent && errors.Is(err, syscall.EPERM) {
		// A successful earlier group signal proves ownership. Once the group
		// disappears, its numeric PGID can be reused before the parent goroutine
		// observes Wait; do not treat that unrelated group as cleanup failure.
		tree.forceCalled = true
		return nil
	}
	if err == nil {
		tree.signalSent = true
	}
	if err == nil && force {
		tree.forceCalled = true
	}
	if err != nil {
		return fmt.Errorf("signal process group %d with %s: %w", tree.cmd.Process.Pid, signal, err)
	}
	return nil
}

func (tree *unixTree) Close(timeout time.Duration) error {
	tree.mu.Lock()
	defer tree.mu.Unlock()
	if tree.cmd.Process == nil {
		return nil
	}
	signalErr := tree.signalLocked(true)
	waitErr := waitForTreeExit(timeout, func() (bool, error) {
		err := syscall.Kill(-tree.cmd.Process.Pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return true, nil
		}
		if tree.forceCalled && tree.signalSent && errors.Is(err, syscall.EPERM) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("query process group %d: %w", tree.cmd.Process.Pid, err)
		}
		return false, nil
	})
	return errors.Join(signalErr, waitErr)
}
