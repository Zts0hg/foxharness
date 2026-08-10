//go:build windows

package benchmark

import (
	"os/exec"
	"strconv"
)

func configureValidationCommand(*exec.Cmd) {}

func signalValidationProcessTree(cmd *exec.Cmd, force bool) error {
	if cmd.Process == nil {
		return nil
	}
	args := []string{"/T", "/PID", strconv.Itoa(cmd.Process.Pid)}
	if force {
		args = append([]string{"/F"}, args...)
	}
	if err := exec.Command("taskkill", args...).Run(); err != nil && force {
		return cmd.Process.Kill()
	} else {
		return err
	}
}
