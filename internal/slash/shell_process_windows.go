//go:build windows

package slash

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"time"
)

const windowsEmbeddedShellTaskkillTimeout = 250 * time.Millisecond

func configureEmbeddedShellCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), windowsEmbeddedShellTaskkillTimeout)
		defer cancel()
		err := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		if err == nil {
			return nil
		}
		killErr := cmd.Process.Kill()
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err(), killErr)
		}
		return errors.Join(err, killErr)
	}
}
