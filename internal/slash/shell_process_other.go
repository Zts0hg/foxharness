//go:build !unix && !windows

package slash

import "os/exec"

func configureEmbeddedShellCommand(cmd *exec.Cmd) {
}
