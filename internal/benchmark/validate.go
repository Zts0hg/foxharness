package benchmark

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ValidationResult struct {
	Type    string `json:"type"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

func ValidateAll(ctx context.Context, workDir string, validations []Validation) []ValidationResult {
	results := make([]ValidationResult, 0, len(validations))
	for _, v := range validations {
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
			Message: "未知验证类型",
		}
	}
}

func validateCommand(ctx context.Context, workDir, command string) ValidationResult {
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", command)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ValidationResult{
			Type:    "command",
			Passed:  false,
			Message: fmt.Sprintf("命令失败: %v\n%s", err, string(out)),
		}
	}
	return ValidationResult{Type: "command", Passed: true}
}

func validateFileContains(workDir, path, contains string) ValidationResult {
	data, err := os.ReadFile(filepath.Join(workDir, path))
	if err != nil {
		return ValidationResult{
			Type:    "file_contains",
			Passed:  false,
			Message: err.Error(),
		}
	}

	if !strings.Contains(string(data), contains) {
		return ValidationResult{
			Type:    "file_contains",
			Passed:  false,
			Message: fmt.Sprintf("%s 不包括目标文本", path),
		}
	}
	return ValidationResult{Type: "file_contains", Passed: true}
}

func allPassed(results []ValidationResult) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}

	return true
}
