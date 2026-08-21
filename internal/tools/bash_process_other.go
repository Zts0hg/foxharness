//go:build !unix && !windows

package tools

import (
	"errors"
	"os"
	"os/exec"

	"github.com/Zts0hg/foxharness/internal/shellcmd"
)

func configureShellCommand(cmd *exec.Cmd) {
	shellcmd.ConfigureCommand(cmd)
}

func signalShellProcessTree(cmd *exec.Cmd, force bool) error {
	if cmd.Process == nil {
		return nil
	}
	if force {
		err := cmd.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
	return cmd.Process.Signal(os.Interrupt)
}
