package tools

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

const (
	bashTerminateGrace = 250 * time.Millisecond
	bashReapTimeout    = 5 * time.Second
)

// BashCommandRunner executes one Bash command for a BashTool.
type BashCommandRunner interface {
	Run(ctx context.Context, workDir, command string, timeout time.Duration) BashCommandResult
}

// BashProcessSupervisor owns every shell process started by one ChildRun.
type BashProcessSupervisor struct {
	mu          sync.Mutex
	closed      bool
	active      map[*supervisedBashProcess]struct{}
	beforeStart func(active int)
}

type supervisedBashProcess struct {
	cmd        *exec.Cmd
	done       chan struct{}
	waitErr    error
	cleanupErr error
}

// NewBashProcessSupervisor creates an empty accepting supervisor.
func NewBashProcessSupervisor() *BashProcessSupervisor {
	return &BashProcessSupervisor{active: make(map[*supervisedBashProcess]struct{})}
}

// Run registers, starts, waits for, and removes one contained process group.
func (s *BashProcessSupervisor) Run(ctx context.Context, workDir, command string, timeout time.Duration) BashCommandResult {
	if timeout <= 0 {
		timeout = defaultBashTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(context.Background(), "bash", "-c", command)
	cmd.Dir = workDir
	configureShellCommand(cmd)
	output := newBoundedOutput(MaxBashOutputBytes)
	cmd.Stdout = output
	cmd.Stderr = output
	process := &supervisedBashProcess{cmd: cmd, done: make(chan struct{})}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return BashCommandResult{Err: errors.New("child Bash supervisor is closed")}
	}
	s.active[process] = struct{}{}
	if s.beforeStart != nil {
		s.beforeStart(len(s.active))
	}
	if err := cmd.Start(); err != nil {
		delete(s.active, process)
		s.mu.Unlock()
		return BashCommandResult{Err: err}
	}
	s.mu.Unlock()

	go s.wait(process)
	reaped := false
	select {
	case <-process.done:
		reaped = true
	case <-runCtx.Done():
		// A cancelled tool must stop producing side effects immediately. A TERM
		// grace period lets descendants that ignore TERM continue mutating the
		// workspace after the runtime has already cancelled the call.
		_ = signalShellProcessTree(cmd, true)
		timer := time.NewTimer(bashReapTimeout)
		select {
		case <-process.done:
			reaped = true
			timer.Stop()
		case <-timer.C:
		}
	}
	if !reaped {
		return BashCommandResult{
			Output:    output.String(),
			Truncated: output.Truncated(),
			TimedOut:  errors.Is(runCtx.Err(), context.DeadlineExceeded),
			Err:       errors.Join(runCtx.Err(), fmt.Errorf("child Bash process tree was not reaped within %s", bashReapTimeout)),
		}
	}

	result := BashCommandResult{Output: output.String(), Truncated: output.Truncated(), Err: process.waitErr}
	if runCtx.Err() != nil {
		result.Err = runCtx.Err()
		result.TimedOut = errors.Is(runCtx.Err(), context.DeadlineExceeded)
	}
	if exitErr, ok := process.waitErr.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	if process.cleanupErr != nil {
		result.Err = errors.Join(result.Err, process.cleanupErr)
	}
	return result
}

func (s *BashProcessSupervisor) wait(process *supervisedBashProcess) {
	process.waitErr = process.cmd.Wait()
	process.cleanupErr = signalShellProcessTree(process.cmd, true)
	s.mu.Lock()
	delete(s.active, process)
	close(process.done)
	s.mu.Unlock()
}

// Cleanup stops admission, terminates all active process groups, and waits for
// their wait goroutines before returning.
func (s *BashProcessSupervisor) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	processes := make([]*supervisedBashProcess, 0, len(s.active))
	for process := range s.active {
		processes = append(processes, process)
	}
	s.mu.Unlock()

	var cleanupErr error
	for _, process := range processes {
		cleanupErr = errors.Join(cleanupErr, signalShellProcessTree(process.cmd, false))
	}
	grace := time.NewTimer(bashTerminateGrace)
	defer grace.Stop()
	select {
	case <-allSupervisedProcessesDone(processes):
		return errors.Join(cleanupErr, supervisedCleanupErrors(processes))
	case <-grace.C:
	case <-ctx.Done():
		return errors.Join(cleanupErr, ctx.Err())
	}
	for _, process := range processes {
		cleanupErr = errors.Join(cleanupErr, signalShellProcessTree(process.cmd, true))
	}
	select {
	case <-allSupervisedProcessesDone(processes):
		return errors.Join(cleanupErr, supervisedCleanupErrors(processes))
	case <-ctx.Done():
		return errors.Join(cleanupErr, ctx.Err())
	case <-time.After(bashReapTimeout):
		return errors.Join(cleanupErr, fmt.Errorf("child Bash process tree was not reaped within %s", bashReapTimeout))
	}
}

func (s *BashProcessSupervisor) activeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.active)
}

func allSupervisedProcessesDone(processes []*supervisedBashProcess) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, process := range processes {
			<-process.done
		}
	}()
	return done
}

func supervisedCleanupErrors(processes []*supervisedBashProcess) error {
	var cleanupErr error
	for _, process := range processes {
		cleanupErr = errors.Join(cleanupErr, process.cleanupErr)
	}
	return cleanupErr
}
