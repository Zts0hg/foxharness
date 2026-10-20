//go:build unix

package mcpconfig

import (
	"os"
	"syscall"
)

/* Nonblocking open prevents a concurrently substituted FIFO from blocking before f.Stat. */
const configurationReadFlags = os.O_RDONLY | syscall.O_NONBLOCK
