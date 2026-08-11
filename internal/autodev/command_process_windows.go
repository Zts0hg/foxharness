//go:build windows

package autodev

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

func configureCommandProcess(cmd *exec.Cmd) {}

func signalCommandProcessTree(cmd *exec.Cmd, force bool) error {
	if cmd.Process == nil {
		return nil
	}
	args := []string{"/T", "/PID", strconv.Itoa(cmd.Process.Pid)}
	if force {
		args = append([]string{"/F"}, args...)
	}
	if err := exec.Command("taskkill", args...).Run(); err != nil && force {
		killErr := cmd.Process.Kill()
		if errors.Is(killErr, os.ErrProcessDone) {
			return nil
		}
		return killErr
	} else {
		return err
	}
}
