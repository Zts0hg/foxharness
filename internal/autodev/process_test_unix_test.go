//go:build unix

package autodev

import "syscall"

func testProcessAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func testForceKill(pid int) { _ = syscall.Kill(pid, syscall.SIGKILL) }
