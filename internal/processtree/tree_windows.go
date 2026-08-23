//go:build windows

package processtree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsTaskkillTimeout = 250 * time.Millisecond

type windowsTree struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	job windows.Handle
}

type jobAccountingInformation struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

func start(cmd *exec.Cmd) (Tree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create process-tree job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("configure process-tree job object: %w", err)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, process)
		_ = windows.CloseHandle(process)
	}
	if err != nil {
		return nil, abortSuspendedCommand(cmd, job, fmt.Errorf("assign process to job object: %w", err))
	}
	if err := resumeProcess(cmd.Process.Pid); err != nil {
		return nil, abortSuspendedCommand(cmd, job, err)
	}
	return &windowsTree{cmd: cmd, job: job}, nil
}

func resumeProcess(processID int) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot process threads: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(processID) {
			continue
		}
		thread, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return fmt.Errorf("open suspended process thread: %w", openErr)
		}
		_, resumeErr := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if resumeErr != nil {
			return fmt.Errorf("resume process: %w", resumeErr)
		}
		return nil
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return errors.New("suspended process has no primary thread")
	}
	return fmt.Errorf("enumerate suspended process threads: %w", err)
}

func abortSuspendedCommand(cmd *exec.Cmd, job windows.Handle, cause error) error {
	terminateErr := windows.TerminateJobObject(job, 1)
	killErr := cmd.Process.Kill()
	waitErr := cmd.Wait()
	closeErr := windows.CloseHandle(job)
	if errors.Is(killErr, os.ErrProcessDone) {
		killErr = nil
	}
	return errors.Join(cause, terminateErr, killErr, waitErr, closeErr)
}

func (tree *windowsTree) Signal(force bool) error {
	tree.mu.Lock()
	defer tree.mu.Unlock()
	if tree.cmd.Process == nil {
		return nil
	}
	if tree.job == 0 {
		return nil
	}
	if force {
		return windows.TerminateJobObject(tree.job, 1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), windowsTaskkillTimeout)
	defer cancel()
	err := exec.CommandContext(ctx, "taskkill", "/T", "/PID", strconv.Itoa(tree.cmd.Process.Pid)).Run()
	if ctx.Err() != nil {
		return errors.Join(err, ctx.Err())
	}
	return err
}

func (tree *windowsTree) Close(timeout time.Duration) error {
	tree.mu.Lock()
	defer tree.mu.Unlock()
	if tree.job == 0 {
		return nil
	}
	terminateErr := windows.TerminateJobObject(tree.job, 0)
	waitErr := waitForTreeExit(timeout, func() (bool, error) {
		var accounting jobAccountingInformation
		if err := windows.QueryInformationJobObject(
			tree.job,
			windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&accounting)),
			uint32(unsafe.Sizeof(accounting)),
			nil,
		); err != nil {
			return false, fmt.Errorf("query process-tree job accounting: %w", err)
		}
		return accounting.ActiveProcesses == 0, nil
	})
	closeErr := windows.CloseHandle(tree.job)
	tree.job = 0
	if errors.Is(terminateErr, os.ErrProcessDone) {
		terminateErr = nil
	}
	if errors.Is(closeErr, os.ErrProcessDone) {
		closeErr = nil
	}
	return errors.Join(terminateErr, waitErr, closeErr)
}
