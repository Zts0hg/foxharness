package tracing

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// Load reads a JSONL trace file and returns all parsed SpanEvent records
// in file order.
func Load(path string) ([]SpanEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 trace 文件失败: %w", err)
	}
	defer f.Close()

	var events []SpanEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event SpanEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("解析 trace 事件失败: %w", err)
		}
		events = append(events, event)
	}

	return events, scanner.Err()
}
