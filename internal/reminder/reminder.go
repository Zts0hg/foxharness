// Package reminder provides a system-reminder manager that monitors agent tool
// usage and injects contextual prompts when it detects loops, missing
// verification after edits, or excessive turns. Reminders are rate-limited by
// a cooldown to avoid flooding the conversation.
package reminder

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// ToolAction records a single tool invocation for pattern analysis.
type ToolAction struct {
	Turn      int
	ToolName  string
	Arguments string
	IsError   bool
}

// Manager tracks tool actions across turns and builds system reminders when
// it detects repetition, missing verification, or prolonged execution.
type Manager struct {
	actions          []ToolAction
	lastReminderTurn int
	cooldownTurns    int
}

// NewManager creates a Manager with a default cooldown of 3 turns between
// consecutive reminders.
func NewManager() *Manager {
	return &Manager{
		cooldownTurns: 3,
	}
}

// Record logs a tool call and its result for the given turn. The Manager
// retains up to 30 recent actions for pattern analysis.
func (m *Manager) Record(turn int, call schema.ToolCall, result schema.ToolResult) {
	m.actions = append(m.actions, ToolAction{
		Turn:      turn,
		ToolName:  call.Name,
		Arguments: string(call.Arguments),
		IsError:   result.IsError,
	})

	if len(m.actions) > 30 {
		m.actions = m.actions[len(m.actions)-30:]
	}
}

func (m *Manager) repeatedSameAction() (ToolAction, int, bool) {
	if len(m.actions) < 3 {
		return ToolAction{}, 0, false
	}

	counts := map[string]int{}
	lastByKey := map[string]ToolAction{}

	for _, action := range m.actions[len(m.actions)-min(8, len(m.actions)):] {
		key := action.ToolName + ":" + fingerprint(action.Arguments)
		counts[key]++
		lastByKey[key] = action
	}

	for key, count := range counts {
		if count >= 3 {
			return lastByKey[key], count, true
		}
	}

	return ToolAction{}, 0, false
}

func fingerprint(s string) string {
	h := sha1.Sum([]byte(strings.TrimSpace(s)))
	return hex.EncodeToString(h[:])
}

func (m *Manager) editedWithoutVerification() bool {
	lastEdit := -1
	lastVerify := -1

	for i, action := range m.actions {
		if action.ToolName == "edit_file" || action.ToolName == "write_file" {
			lastEdit = i
		}
		if action.ToolName == "bash" && looksLikeVerification(action.Arguments) {
			lastVerify = i
		}

	}
	return lastEdit >= 0 && lastEdit > lastVerify && len(m.actions)-lastEdit >= 4
}

func looksLikeVerification(args string) bool {
	return strings.Contains(args, "test") ||
		strings.Contains(args, "go test") ||
		strings.Contains(args, "npm test") ||
		strings.Contains(args, "pytest") ||
		strings.Contains(args, "cargo test")
}

// MaybeBuild checks whether a system reminder should be injected at the
// given turn. It returns the reminder text and true when one is warranted, or
// an empty string and false otherwise. Reminders are suppressed when the
// cooldown since the last reminder has not elapsed.
func (m *Manager) MaybeBuild(turn int) (string, bool) {
	if turn-m.lastReminderTurn < m.cooldownTurns {
		return "", false
	}

	if action, count, ok := m.repeatedSameAction(); ok {
		m.lastReminderTurn = turn
		return fmt.Sprintf(`
## System Reminder: Possible Loop Detected

你最近重复执行了同一个工具动作 %d 次：

- Tool: %s
- Arguments: %s

你必须停止原样重复该动作。请先说明：
1. 这个动作为什么没有带来新信息？
2. 当前任务目标是什么？
3. 下一步要采取哪一个不同策略？

除非你获得了新的证据，否则不要再次调用完全相同的工具和参数。
`, count, action.ToolName, action.Arguments), true
	}

	if m.editedWithoutVerification() {
		m.lastReminderTurn = turn
		return `
## System Reminder: Verification Needed

你已经修改了文件，但最近几步没有运行测试、构建或者其他验证命令。

在继续扩大修改范围前，请优先使用 bash 运行最小相关验证命令，并根据结果决定下一步。
`, true
	}

	if turn > 0 && turn%12 == 0 {
		m.lastReminderTurn = turn
		return `
## System Reminder: Re-anchor

任务已经运行了较多轮。请重新对齐：
1. 用户原始目标是什么？
2. 已完成哪些关键步骤？
3. 还有什么未解决？
4. 下一步最小有效动作是什么？
`, true
	}

	return "", false
}
