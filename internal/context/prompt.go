// Package context assembles the system prompt for foxharness agent sessions.
// A Composer loads project-level instructions (AGENTS.md), project and session
// memory, and the list of model-invocable skills, combining them into a single
// system prompt ready for the LLM.
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Composer builds the full system prompt by layering base instructions,
// project-level AGENTS.md, project/session memory, and the available skill
// list.
type Composer struct {
	workDir     string
	memoryPath  string
	skillListFn func() string
}

// WithSkillList registers a function that returns the formatted list of
// model-invocable skills. When set, Compose appends the rendered list as a
// dedicated section so the LLM can decide when to invoke the `skill` tool.
// Pass nil to clear.
func (c *Composer) WithSkillList(fn func() string) *Composer {
	clone := *c
	clone.skillListFn = fn
	return &clone
}

// NewComposer creates a Composer rooted at the given workspace directory.
func NewComposer(workDir string) *Composer {
	return &Composer{workDir: workDir}
}

// WithMemory returns a copy of the Composer configured to load session
// working memory from the given file path.
func (c *Composer) WithMemory(path string) *Composer {
	clone := *c
	clone.memoryPath = path
	return &clone
}

// Compose assembles the full system prompt string by loading AGENTS.md,
// project and session memory, and the available skill list.
func (c *Composer) Compose() (string, error) {
	parts := []string{
		baseSystemPrompt(),
	}
	parts = append(parts, section("Persistent File Memory", memoryInstructions()))

	agents, err := c.loadAgentsFile()
	if err != nil {
		return "", err
	}
	if agents != "" {
		parts = append(parts, section("Project Instructions from AGENTS.md", agents))
	}

	projectMemory, err := c.loadProjectMemory()
	if err != nil {
		return "", err
	}
	if projectMemory != "" {
		parts = append(parts, section("Project Memory from MEMORY.md", projectMemory))
	}

	memory, err := c.loadWorkingMemory()
	if err != nil {
		return "", err
	}
	if memory != "" {
		parts = append(parts, section("Session Working Memory", memory))
	}

	if c.skillListFn != nil {
		if list := strings.TrimSpace(c.skillListFn()); list != "" {
			parts = append(parts, section("Available Skills (invoke via the `skill` tool)", list))
		}
	}

	return strings.Join(parts, "\n\n"), nil
}

func memoryInstructions() string {
	return strings.TrimSpace(`
Persistent files:
- Session PLAN.md stores the high-level plan for the current session.
- Session TODO.md stores concrete checklist items for the current session.
- Project MEMORY.md stores durable project facts that are useful across sessions.

Rules:
- Use the current session plan and todo to track complex multi-step tasks.
- Use read_todo and update_todo to inspect and maintain Session TODO.md.
- Do not use bash, write_file, or edit_file to modify Session TODO.md.
- Add only durable, high-value facts to MEMORY.md.
- Do not dump raw logs or large file contents into memory files.
- Prefer edit_file for focused updates to project MEMORY.md.
`)
}

func baseSystemPrompt() string {
	return strings.TrimSpace(`
You are fox-harness, an expert coding assistant running inside an Agent Harness.

Core rules:
- You operate inside the current workspace.
- Prefer reading files before editing them.
- Use edit_file for focused modifications.
- Use write_file only when creating a new file or intentionally replacing a whole file.
- Use bash to inspect, build, test, and verify changes.
- After changing code, verify with the smallest relevant test command.
- If a tool fails, inspect the error and recover instead of blindly repeating the same call.
- Treat @path tokens in user messages as project-relative file references; read referenced files before making claims or edits about them.
- Keep changes small, explicit, and aligned with the user's request.
`)
}

func section(title, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	return fmt.Sprintf("## %s\n\n%s", title, body)
}

func (c *Composer) loadAgentsFile() (string, error) {
	path := filepath.Join(c.workDir, "AGENTS.md")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取 AGENTS.md 失败: %w", err)
	}

	return string(content), nil
}

func (c *Composer) loadProjectMemory() (string, error) {
	path := filepath.Join(c.workDir, "MEMORY.md")
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取 MEMORY.md 失败: %w", err)
	}

	return string(content), nil
}

func (c *Composer) loadWorkingMemory() (string, error) {
	if c.memoryPath == "" {
		return "", nil
	}

	data, err := os.ReadFile(c.memoryPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取 Working Memory 失败: %w", err)
	}

	return string(data), nil
}
