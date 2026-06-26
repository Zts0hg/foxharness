//go:build windows

package tools

import (
	"os/exec"
	"strconv"
)

func configureShellCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		if err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
