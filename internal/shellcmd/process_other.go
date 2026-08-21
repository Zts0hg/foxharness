//go:build !unix && !windows

package shellcmd

import "os/exec"

/* ConfigureCommand leaves platform-default process cancellation in place. */
func ConfigureCommand(_ *exec.Cmd) {}
