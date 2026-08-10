package agentops

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
)

// LogSearchTool is a read-only tool that searches local service log files for
// lines matching a keyword query.  It is the primary evidence-gathering tool
// for AgentOps incident analysis and is registered as parallel-safe.
type LogSearchTool struct {
	logDir  string
	openLog func(string) (io.ReadCloser, error)
}

const maxLogSearchLineBytes = 1 << 20

// NewLogSearchTool creates a LogSearchTool rooted at logDir.  Log files are
// expected at <logDir>/<service>.log.
func NewLogSearchTool(logDir string) *LogSearchTool {
	return &LogSearchTool{logDir: logDir}
}

// Name returns the tool identifier "log_search".
func (t *LogSearchTool) Name() string {
	return "log_search"
}

// ParallelSafe reports true, indicating this tool may execute concurrently
// with other tools.
func (t *LogSearchTool) ParallelSafe() bool {
	return true
}

// Definition returns the JSON-schema tool definition accepted by the LLM.
// Required parameters are "service" and "query"; "limit" is optional and
// defaults to 50.
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
					"type":        "integer",
					"description": "最多返回多少行，默认50",
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

// AssessPermission declares log_search as a read-only AgentOps evidence lookup
// when its bounded service/query arguments are valid.
func (t *LogSearchTool) AssessPermission(_ toolpolicy.Context, raw json.RawMessage) (toolpolicy.Assessment, error) {
	var args logSearchArgs
	if err := json.Unmarshal(raw, &args); err != nil || strings.TrimSpace(args.Service) == "" || strings.TrimSpace(args.Query) == "" {
		return toolpolicy.Assessment{
			Behavior: toolpolicy.BehaviorHumanOnly,
			Action:   t.Name(),
			Effects:  []toolpolicy.Effect{toolpolicy.EffectUnknown},
			Scope:    toolpolicy.ScopeUnknown,
			RiskHint: toolpolicy.RiskHigh,
			Reason:   "invalid or missing log search arguments",
		}, nil
	}
	if !validServiceName(args.Service) {
		return toolpolicy.Assessment{
			Behavior: toolpolicy.BehaviorHumanOnly,
			Action:   t.Name() + " " + args.Service,
			Effects:  []toolpolicy.Effect{toolpolicy.EffectUnknown},
			Scope:    toolpolicy.ScopeUnknown,
			RiskHint: toolpolicy.RiskHigh,
			Reason:   "invalid log service name",
		}, nil
	}
	return toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorFastAllow,
		Action:   t.Name() + " " + args.Service,
		Effects:  []toolpolicy.Effect{toolpolicy.EffectObserve},
		Scope:    toolpolicy.ScopeWorkspace,
		ReadOnly: true,
		RiskHint: toolpolicy.RiskLow,
		Reason:   "read-only configured log search",
		Target:   args.Service,
	}, nil
}

// Execute deserialises the arguments, reads the matching log file, and
// returns up to args.Limit lines containing the query (case-insensitive).
// It respects context cancellation during line scanning.
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

	reader, err := t.open(args.Service + ".log")
	if err != nil {
		return "", fmt.Errorf("读取日志失败: %w", err)
	}
	defer reader.Close()

	var matched []string
	query := strings.ToLower(args.Query)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLogSearchLineBytes)
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.Contains(strings.ToLower(line), query) {
			matched = append(matched, line)
			if len(matched) >= args.Limit {
				break
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("读取日志失败: %w", err)
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

func (t *LogSearchTool) open(name string) (io.ReadCloser, error) {
	if t.openLog != nil {
		return t.openLog(name)
	}
	root, err := os.OpenRoot(t.logDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("log target is not a regular file")
	}
	return file, nil
}
