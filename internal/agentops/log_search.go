package agentops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type LogSearchTool struct {
	logDir string
}

func NewLogSearchTool(logDir string) *LogSearchTool {
	return &LogSearchTool{logDir: logDir}
}

func (t *LogSearchTool) Name() string {
	return "log_search"
}

func (t *LogSearchTool) ParallelSafe() bool {
	return true
}

func (t *LogSearchTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "只读搜索指定服务的本地日志文件。用于 AgentOps 日志分布，不允许修改任何资源。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"service": map[string]interface{}{
					"type":        "string",
					"description": "服务名，例如 payment",
				},
				"query": map[string]interface{}{
					"type":        "string",
					"description": "要搜索的关键词，例如 ERROR 或 timeout",
				},
				"limit": map[string]interface{}{
					"type":         "integer",
					"descriptioin": "最多返回多少行，默认50",
				},
			},
			"required": []string{"service", "query"},
		},
	}
}

type logSearchArgs struct {
	Service string `json:"service"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
}

func (t *LogSearchTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var args logSearchArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}

	if args.Service == "" || args.Query == "" {
		return "", fmt.Errorf("service 和 query 不能为空")
	}
	if !validServiceName(args.Service) {
		return "", fmt.Errorf("service 名称非法")
	}
	if args.Limit <= 0 || args.Limit > 200 {
		args.Limit = 50
	}

	path := filepath.Join(t.logDir, args.Service+".log")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取日志失败: %w", err)
	}

	var matched []string
	for _, line := range strings.Split(string(data), "\n") {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		if strings.Contains(strings.ToLower(line), strings.ToLower(args.Query)) {
			matched = append(matched, line)
			if len(matched) >= args.Limit {
				break
			}
		}
	}

	if len(matched) == 0 {
		return "没有匹配日志。", nil
	}

	return strings.Join(matched, "\n"), nil
}

func validServiceName(service string) bool {
	if service == "." || service == ".." {
		return false
	}
	return !strings.ContainsAny(service, `/\`)
}
