//go:build unix

package processtree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type unixTree struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	groupID       int
	anchorInput   *os.File
	anchorDone    <-chan error
	anchorExited  bool
	anchorExitErr error
	forceCalled   bool
}

/*
start keeps a signal-ignoring ownership anchor in every process group. The
anchor prevents the numeric PGID from being reused after the command leader is
reaped and remains alive until the tree owner sends its final group signal.
*/
func start(cmd *exec.Cmd) (Tree, error) {
	anchorShell, err := processTreeAnchorShell()
	if err != nil {
		return nil, err
	}
	anchorInput, anchorOutput, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create process-tree ownership pipe: %w", err)
	}
	readyInput, readyOutput, err := os.Pipe()
	if err != nil {
		_ = anchorInput.Close()
		_ = anchorOutput.Close()
		return nil, fmt.Errorf("create process-tree readiness pipe: %w", err)
	}
	anchor := exec.Command(anchorShell, "-c", "trap '' TERM; printf x; IFS= read -r _")
	anchor.Stdin = anchorInput
	anchor.Stdout = readyOutput
	anchor.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := anchor.Start(); err != nil {
		_ = anchorInput.Close()
		_ = anchorOutput.Close()
		_ = readyInput.Close()
		_ = readyOutput.Close()
		return nil, fmt.Errorf("start process-tree ownership anchor: %w", err)
	}
	_ = anchorInput.Close()
	_ = readyOutput.Close()
	anchorDone := make(chan error, 1)
	go func() { anchorDone <- anchor.Wait() }()
	readyErr := waitForAnchorReady(readyInput)
	_ = readyInput.Close()
	if readyErr != nil {
		return nil, abortUnixAnchor(anchor.Process.Pid, anchorOutput, anchorDone,
			fmt.Errorf("initialize process-tree ownership anchor: %w", readyErr))
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: anchor.Process.Pid}
	if err := cmd.Start(); err != nil {
		return nil, abortUnixAnchor(anchor.Process.Pid, anchorOutput, anchorDone, err)
	}
	return &unixTree{
		cmd: cmd, groupID: anchor.Process.Pid, anchorInput: anchorOutput, anchorDone: anchorDone,
	}, nil
}

// anchorReadyTimeout bounds how long start waits for the ownership anchor to
// signal readiness. The supervisor holds its admission lock across start, so
// an anchor that never becomes ready must not block cleanup indefinitely.
const anchorReadyTimeout = 2 * time.Second

// waitForAnchorReady reads the anchor's single readiness byte, bounded by
// anchorReadyTimeout. Closing the read end on abort unblocks a timed-out
// reader, so the helper goroutine always exits.
func waitForAnchorReady(readyInput *os.File) error {
	ready := make(chan error, 1)
	go func() {
		var byte [1]byte
		_, err := io.ReadFull(readyInput, byte[:])
		ready <- err
	}()
	timer := time.NewTimer(anchorReadyTimeout)
	defer timer.Stop()
	select {
	case err := <-ready:
		return err
	case <-timer.C:
		return errors.New("process-tree ownership anchor readiness timeout")
	}
}

// processTreeAnchorShell resolves the shell that runs the ownership anchor:
// PATH's sh when available, else the traditional /bin/sh location, so hosts
// with a minimal PATH or without /bin/sh keep process-group ownership.
func processTreeAnchorShell() (string, error) {
	if path, err := exec.LookPath("sh"); err == nil {
		return path, nil
	}
	if info, err := os.Stat("/bin/sh"); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
		return "/bin/sh", nil
	}
	return "", errors.New("no sh executable found for the process-tree ownership anchor")
}

func abortUnixAnchor(groupID int, anchorInput *os.File, anchorDone <-chan error, cause error) error {
	signalErr := syscall.Kill(-groupID, syscall.SIGKILL)
	closeErr := anchorInput.Close()
	waitErr := <-anchorDone
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		waitErr = nil
	}
	return errors.Join(cause, signalErr, closeErr, waitErr)
}

func (tree *unixTree) Signal(force bool) error {
	tree.mu.Lock()
	defer tree.mu.Unlock()
	return tree.signalLocked(force)
}

func (tree *unixTree) signalLocked(force bool) error {
	if tree.cmd.Process == nil || tree.groupID <= 0 {
		return nil
	}
	if tree.forceCalled {
		return nil
	}
	/* An anchor that died early is reported as evidence, but the group
	 * members — the command shell and its children — are still alive, so the
	 * signal goes out regardless of the anchor's fate. */
	anchorErr := tree.observeUnexpectedAnchorExitLocked()
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	err := syscall.Kill(-tree.groupID, signal)
	if err == nil && force {
		tree.forceCalled = true
	}
	err = errors.Join(err, anchorErr)
	if err != nil {
		return fmt.Errorf("signal owned process group %d with %s: %w", tree.groupID, signal, err)
	}
	return nil
}

/* Release hands the group back to its survivors: the anchor's stdin closes so
 * the ownership anchor exits on its own, and no group signal is sent. */
func (tree *unixTree) Release(timeout time.Duration) error {
	tree.mu.Lock()
	defer tree.mu.Unlock()
	if tree.cmd.Process == nil {
		return nil
	}
	closeErr := tree.closeAnchorInputLocked()
	return errors.Join(closeErr, tree.waitForAnchorReleaseLocked(timeout))
}

/* waitForAnchorReleaseLocked waits for the anchor to exit after its stdin
 * closed. The anchor's read returns EOF status on that deliberate close, so
 * the exit status itself is not an ownership failure. */
func (tree *unixTree) waitForAnchorReleaseLocked(timeout time.Duration) error {
	if tree.anchorExited {
		return nil
	}
	if timeout <= 0 {
		timeout = time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-tree.anchorDone:
		tree.anchorExited = true
		return nil
	case <-timer.C:
		return fmt.Errorf("process-tree ownership anchor was not reaped within %s", timeout)
	}
}

func (tree *unixTree) Close(timeout time.Duration) error {
	tree.mu.Lock()
	defer tree.mu.Unlock()
	if tree.cmd.Process == nil {
		return nil
	}
	signalErr := tree.signalLocked(true)
	closeErr := tree.closeAnchorInputLocked()
	waitErr := tree.waitForAnchorExitLocked(timeout)
	return errors.Join(signalErr, closeErr, waitErr)
}

func (tree *unixTree) observeUnexpectedAnchorExitLocked() error {
	if tree.anchorExited {
		return fmt.Errorf("process-tree ownership anchor exited before cleanup: %w", tree.anchorExitErr)
	}
	select {
	case err := <-tree.anchorDone:
		tree.anchorExited = true
		tree.anchorExitErr = err
		return fmt.Errorf("process-tree ownership anchor exited before cleanup: %w", err)
	default:
		return nil
	}
}

func (tree *unixTree) closeAnchorInputLocked() error {
	if tree.anchorInput == nil {
		return nil
	}
	err := tree.anchorInput.Close()
	tree.anchorInput = nil
	return err
}

func (tree *unixTree) waitForAnchorExitLocked(timeout time.Duration) error {
	if tree.anchorExited {
		if tree.forceCalled {
			return nil
		}
		return tree.anchorExitErr
	}
	if timeout <= 0 {
		timeout = time.Millisecond
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-tree.anchorDone:
		tree.anchorExited = true
		tree.anchorExitErr = err
		if tree.forceCalled {
			return nil
		}
		return err
	case <-timer.C:
		return fmt.Errorf("process-tree ownership anchor was not reaped within %s", timeout)
	}
}
