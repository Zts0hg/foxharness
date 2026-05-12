package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
)

func WriteJSON(path string, results []*Result) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 benchmark 结果失败: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func PrintSummary(results []*Result) {
	total := len(results)
	passed := 0
	for _, r := range results {
		if r.Success {
			passed++
		}
	}

	fmt.Printf("Benchmark Summary: %d/%d passed\n", passed, total)
	for _, r := range results {
		status := "FAIL"
		if r.Success {
			status = "PASS"
		}
		fmt.Printf("- [%s] %s duration=%dms session=%s workspace=%s\n", status, r.CaseID, r.DurationMS, r.SessionID, r.WorkSpace)
	}
}
