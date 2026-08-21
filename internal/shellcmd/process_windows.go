//go:build windows

package shellcmd

import (
	"os/exec"
	"strconv"
)

/* ConfigureCommand makes cancellation terminate the command's process tree. */
func ConfigureCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
