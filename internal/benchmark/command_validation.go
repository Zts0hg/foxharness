package benchmark

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const maxValidationOutputBytes = 1 << 20

type commandValidationConfig struct {
	timeout        time.Duration
	terminateGrace time.Duration
	reapTimeout    time.Duration
	outputLimit    int
}

func defaultCommandValidationConfig() commandValidationConfig {
	return commandValidationConfig{
		timeout:        2 * time.Minute,
		terminateGrace: 250 * time.Millisecond,
		reapTimeout:    5 * time.Second,
		outputLimit:    maxValidationOutputBytes,
	}
}

func executeCommandValidation(ctx context.Context, workDir, command string, config commandValidationConfig) ValidationResult {
	runCtx, cancel := context.WithTimeout(ctx, config.timeout)
	defer cancel()
	deadline, _ := runCtx.Deadline()

	overflow := make(chan string, 1)
	stdout := newValidationOutput("stdout", config.outputLimit, overflow)
	stderr := newValidationOutput("stderr", config.outputLimit, overflow)
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = workDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	configureValidationCommand(cmd)

	result := ValidationResult{Type: "command", Status: ValidationStatusFailed, Deadline: &deadline}
	if err := cmd.Start(); err != nil {
		result.Message = fmt.Sprintf("命令启动失败: %v", err)
		return result
	}

	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	var waitErr error
	var terminalErr error
	var cleanupErr error
	select {
	case waitErr = <-wait:
		if err := runCtx.Err(); err != nil {
			terminalErr = err
		}
	case <-overflow:
		waitErr, cleanupErr = terminateValidationCommand(cmd, wait, config)
	case <-runCtx.Done():
		terminalErr = runCtx.Err()
		waitErr, cleanupErr = terminateValidationCommand(cmd, wait, config)
	}

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.StdoutOverflow = stdout.Overflowed()
	result.StderrOverflow = stderr.Overflowed()
	if terminalErr != nil {
		terminal := terminalValidationResult("command", terminalErr)
		result.Status = terminal.Status
		result.Message = terminal.Message
	} else if result.StdoutOverflow || result.StderrOverflow {
		result.Message = overflowMessage(result)
	} else if waitErr != nil {
		result.Message = fmt.Sprintf("命令失败: %v\nstdout:\n%s\nstderr:\n%s", waitErr, result.Stdout, result.Stderr)
	} else {
		result.Passed = true
		result.Status = ValidationStatusPassed
	}
	if cleanupErr != nil {
		result.Passed = false
		result.Status = ValidationStatusFailed
		result.Message = errors.Join(errors.New(result.Message), cleanupErr).Error()
	}
	return result
}

func overflowMessage(result ValidationResult) string {
	streams := make([]string, 0, 2)
	if result.StdoutOverflow {
		streams = append(streams, "stdout")
	}
	if result.StderrOverflow {
		streams = append(streams, "stderr")
	}
	return fmt.Sprintf("命令 %s 输出超过 %d 字节限制", strings.Join(streams, " 和 "), maxValidationOutputBytes)
}

func terminateValidationCommand(cmd *exec.Cmd, wait <-chan error, config commandValidationConfig) (error, error) {
	terminateErr := signalValidationProcessTree(cmd, false)
	timer := time.NewTimer(config.terminateGrace)
	defer timer.Stop()
	select {
	case waitErr := <-wait:
		return waitErr, terminateErr
	case <-timer.C:
	}

	killErr := signalValidationProcessTree(cmd, true)
	reapTimer := time.NewTimer(config.reapTimeout)
	defer reapTimer.Stop()
	select {
	case waitErr := <-wait:
		return waitErr, errors.Join(terminateErr, killErr)
	case <-reapTimer.C:
		return nil, errors.Join(terminateErr, killErr, fmt.Errorf("命令进程树未在 %s 内完成回收", config.reapTimeout))
	}
}

type validationOutput struct {
	mu       sync.Mutex
	stream   string
	limit    int
	buf      []byte
	overflow bool
	notify   chan<- string
}

func newValidationOutput(stream string, limit int, notify chan<- string) *validationOutput {
	return &validationOutput{stream: stream, limit: limit, notify: notify}
}

func (output *validationOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	remaining := output.limit - len(output.buf)
	if remaining > len(data) {
		remaining = len(data)
	}
	if remaining > 0 {
		output.buf = append(output.buf, data[:remaining]...)
	}
	if remaining < len(data) && !output.overflow {
		output.overflow = true
		select {
		case output.notify <- output.stream:
		default:
		}
	}
	return len(data), nil
}

func (output *validationOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return string(output.buf)
}

func (output *validationOutput) Overflowed() bool {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.overflow
}
