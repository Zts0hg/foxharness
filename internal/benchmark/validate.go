package benchmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ValidationResult records the outcome of a single validation check, including
// whether it passed and a human-readable message on failure.
type ValidationResult struct {
	Type           string           `json:"type"`
	Passed         bool             `json:"passed"`
	Status         ValidationStatus `json:"status"`
	Deadline       *time.Time       `json:"deadline,omitempty"`
	Message        string           `json:"message,omitempty"`
	Stdout         string           `json:"stdout,omitempty"`
	Stderr         string           `json:"stderr,omitempty"`
	StdoutOverflow bool             `json:"stdout_overflow,omitempty"`
	StderrOverflow bool             `json:"stderr_overflow,omitempty"`
}

// ValidationStatus identifies the terminal state of one ordered validation.
type ValidationStatus string

const (
	ValidationStatusPassed    ValidationStatus = "passed"
	ValidationStatusFailed    ValidationStatus = "failed"
	ValidationStatusCancelled ValidationStatus = "cancelled"
	ValidationStatusTimedOut  ValidationStatus = "timed_out"
)

// ValidateAll runs every validation in order against the workspace directory
// and returns one ValidationResult per entry. Command validations are
// executed with a two-minute timeout.
func ValidateAll(ctx context.Context, workDir string, validations []Validation) []ValidationResult {
	results := make([]ValidationResult, 0, len(validations))
	for _, v := range validations {
		if err := ctx.Err(); err != nil {
			results = append(results, terminalValidationResult(v.Type, err, contextDeadline(ctx)))
			continue
		}
		results = append(results, validateOne(ctx, workDir, v))
	}

	return results
}

func validateOne(ctx context.Context, workDir string, v Validation) ValidationResult {
	switch v.Type {
	case "command":
		return validateCommand(ctx, workDir, v.Command)
	case "file_contains":
		return validateFileContains(workDir, v.Path, v.Contains)
	default:
		return ValidationResult{
			Type:    v.Type,
			Passed:  false,
			Status:  ValidationStatusFailed,
			Message: "未知验证类型",
		}
	}
}

func validateCommand(ctx context.Context, workDir, command string) ValidationResult {
	return executeCommandValidation(ctx, workDir, command, defaultCommandValidationConfig())
}

func validateFileContains(workDir, path, contains string) ValidationResult {
	data, err := readRootedRegularFile(workDir, path)
	if err != nil {
		return ValidationResult{
			Type:    "file_contains",
			Passed:  false,
			Status:  ValidationStatusFailed,
			Message: err.Error(),
		}
	}

	if !strings.Contains(string(data), contains) {
		return ValidationResult{
			Type:    "file_contains",
			Passed:  false,
			Status:  ValidationStatusFailed,
			Message: fmt.Sprintf("%s 不包括目标文本", path),
		}
	}
	return ValidationResult{Type: "file_contains", Passed: true, Status: ValidationStatusPassed}
}

func readRootedRegularFile(workDir, path string) ([]byte, error) {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, `\`) {
		return nil, fmt.Errorf("validation path must be a relative workspace path")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("validation path escapes workspace")
	}
	root, err := os.OpenRoot(workDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	current := ""
	for _, component := range strings.Split(filepath.ToSlash(clean), "/") {
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("validation path contains a symlink")
		}
	}
	file, err := root.Open(clean)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("validation target is not a regular file")
	}
	finalInfo, err := root.Lstat(clean)
	if err != nil {
		return nil, err
	}
	if finalInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, finalInfo) {
		return nil, fmt.Errorf("validation target changed while being opened")
	}
	return io.ReadAll(file)
}

func terminalValidationResult(validationType string, err error, deadlines ...time.Time) ValidationResult {
	status := ValidationStatusCancelled
	if errors.Is(err, context.DeadlineExceeded) {
		status = ValidationStatusTimedOut
	}
	result := ValidationResult{Type: validationType, Status: status, Message: err.Error()}
	if len(deadlines) > 0 && !deadlines[0].IsZero() {
		deadline := deadlines[0]
		result.Deadline = &deadline
	}
	return result
}

func contextDeadline(ctx context.Context) time.Time {
	deadline, _ := ctx.Deadline()
	return deadline
}

func allPassed(results []ValidationResult) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}

	return true
}
