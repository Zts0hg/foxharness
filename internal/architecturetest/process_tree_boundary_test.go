package architecturetest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsProcessTreeConsumersUseSharedPreStartBoundary(t *testing.T) {
	root := moduleRoot(t)
	for _, relative := range []string{
		"internal/autodev/gitexec.go",
		"internal/benchmark/command_validation.go",
		"internal/shellcmd/runner.go",
		"internal/tools/bash_supervisor.go",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "processtree.Start(") {
			t.Errorf("%s does not start commands through the shared process-tree boundary", relative)
		}
	}

	windowsSource, err := os.ReadFile(filepath.Join(root, "internal", "processtree", "tree_windows.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"CREATE_SUSPENDED",
		"CreateJobObject",
		"JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE",
		"AssignProcessToJobObject",
		"ResumeThread",
		"TerminateJobObject",
		"QueryInformationJobObject",
		"ActiveProcesses",
	} {
		if !strings.Contains(string(windowsSource), required) {
			t.Errorf("Windows process-tree boundary is missing %s", required)
		}
	}
	if strings.Contains(string(windowsSource), "WaitForSingleObject") {
		t.Error("Windows process-tree boundary relies on Job signaling instead of active-process accounting")
	}
	windowsText := string(windowsSource)
	if strings.Contains(windowsText, `exec.Command("taskkill"`) {
		t.Error("Windows graceful process-tree termination must bound taskkill with an explicit context")
	}
	if !strings.Contains(windowsText, "exec.CommandContext") {
		t.Error("Windows graceful process-tree termination does not use a bounded CommandContext")
	}

	slashWindowsSource, err := os.ReadFile(filepath.Join(root, "internal", "slash", "shell_process_windows.go"))
	if err != nil {
		t.Fatal(err)
	}
	slashWindowsText := string(slashWindowsSource)
	if strings.Contains(slashWindowsText, `exec.Command("taskkill"`) {
		t.Error("Windows slash shell cancellation must bound taskkill with an explicit context")
	}
	if !strings.Contains(slashWindowsText, "exec.CommandContext") {
		t.Error("Windows slash shell cancellation does not use a bounded CommandContext")
	}

	unixSource, err := os.ReadFile(filepath.Join(root, "internal", "processtree", "tree_unix.go"))
	if err != nil {
		t.Fatal(err)
	}
	unixText := string(unixSource)
	for _, required := range []string{
		"trap '' TERM",
		"Pgid: anchor.Process.Pid",
		"syscall.Kill(-tree.groupID, signal)",
		"waitForAnchorExitLocked(timeout)",
	} {
		if !strings.Contains(unixText, required) {
			t.Errorf("Unix process-tree boundary does not preserve owned-group cleanup via %s", required)
		}
	}
	if strings.Contains(unixText, "syscall.Kill(-tree.cmd.Process.Pid") {
		t.Error("Unix process-tree cleanup signals a reapable command PID instead of its ownership-anchored PGID")
	}
}

func TestFileDeliveryStoreUnsupportedPlatformLockFailsClosed(t *testing.T) {
	root := moduleRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "feishu", "delivery_lock_other.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "operation()") {
		t.Fatal("unsupported delivery-store lock fallback executes without a cross-process lock")
	}
	if !strings.Contains(text, "unsupported") {
		t.Fatal("unsupported delivery-store lock fallback does not fail closed with an explicit unsupported error")
	}
}

func TestWindowsFileDeliveryStoreCommitFlushesRenamedAuthority(t *testing.T) {
	root := moduleRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "internal", "feishu", "delivery_commit_windows.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "syncRootedDeliveryStoreFile(root, targetPath)") {
		t.Fatal("Windows delivery-store commit does not flush the renamed rooted authority to stable storage")
	}
}
