//go:build !unix && !windows

package benchmark

import (
	"os"
	"os/exec"
)

func configureValidationCommand(*exec.Cmd) {}

func signalValidationProcessTree(cmd *exec.Cmd, force bool) error {
	if cmd.Process == nil {
		return nil
	}
	if force {
		return cmd.Process.Kill()
	}
	return cmd.Process.Signal(os.Interrupt)
}
