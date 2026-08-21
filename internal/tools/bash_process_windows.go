//go:build windows

package tools

import (
	"errors"
	"os"
	"os/exec"
	"strconv"

	"github.com/Zts0hg/foxharness/internal/shellcmd"
)

func configureShellCommand(cmd *exec.Cmd) {
	shellcmd.ConfigureCommand(cmd)
}

func signalShellProcessTree(cmd *exec.Cmd, force bool) error {
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
