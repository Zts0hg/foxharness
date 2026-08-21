//go:build unix

package shellcmd

import (
	"os/exec"
	"syscall"
)

/* ConfigureCommand makes cancellation terminate the command's process group. */
func ConfigureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
