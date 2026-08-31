//go:build !unix

package autodev

func testProcessAlive(int) bool { return false }

func testForceKill(int) {}
