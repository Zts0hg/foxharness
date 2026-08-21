//go:build unix

package tools

import (
	"errors"
	"os/exec"
	"syscall"

	"github.com/Zts0hg/foxharness/internal/shellcmd"
)

func configureShellCommand(cmd *exec.Cmd) {
	shellcmd.ConfigureCommand(cmd)
}

func signalShellProcessTree(cmd *exec.Cmd, force bool) error {
	if cmd.Process == nil {
		return nil
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	err := syscall.Kill(-cmd.Process.Pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
