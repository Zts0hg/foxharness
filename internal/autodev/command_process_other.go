//go:build !unix && !windows

package autodev

import (
	"errors"
	"os"
	"os/exec"
)

func configureCommandProcess(cmd *exec.Cmd) {}

func signalCommandProcessTree(cmd *exec.Cmd, force bool) error {
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
