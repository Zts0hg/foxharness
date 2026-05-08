package middleware

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type ApprovalRequest struct {
	ToolName  string
	Arguments string
	Risk      string
}

type Approver interface {
	Approve(ctx context.Context, req ApprovalRequest) (bool, string, error)
}

type DangerMiddle struct {
	approver Approver
}

func NewDangerMiddleware(approver Approver) *DangerMiddle {
	return &DangerMiddle{approver: approver}
}

func (m *DangerMiddle) BeforeExecute(ctx context.Context, call schema.ToolCall) (Decision, error) {
	risk := classifyRisk(call)
	if risk == "" {
		return Allow(), nil
	}

	approved, reason, err := m.approver.Approve(ctx, ApprovalRequest{
		ToolName:  call.Name,
		Arguments: string(call.Arguments),
		Risk:      risk,
	})
	if err != nil {
		return Deny("审批系统错误: " + err.Error()), nil
	}

	if !approved {
		if reason == "" {
			reason = "人工审核拒绝或超时"
		}
		return Deny(reason), nil
	}

	return Allow(), nil
}

func classifyRisk(call schema.ToolCall) string {
	if call.Name != "bash" {
		return ""
	}

	var args struct {
		Command string `json:"command"`
	}

	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return "无法解析 bash 参数，拒绝执行"
	}

	cmd := strings.ToLower(args.Command)
	patterns := map[string]string{
		"rm -rf":          "递归强制删除文件",
		"sudo ":           "提权执行命令",
		"kubectl delete":  "删除 Kubernetes 资源",
		"terraform apply": "变更基础设施",
		"git push":        "推送代码到远端",
		"chmod -r":        "递归修改文件权限",
		"chown -r":        "递归修改文件属主",
	}

	for pattern, risk := range patterns {
		if strings.Contains(cmd, pattern) {
			return risk + ": " + args.Command
		}
	}

	return ""
}
