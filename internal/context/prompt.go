// Package context assembles the system prompt for foxharness agent sessions.
// A Composer loads project-level instructions (AGENTS.md), optional skill
// files referenced via $name syntax in the user prompt, and session working
// memory, combining them into a single system prompt ready for the LLM.
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Composer builds the full system prompt by layering base instructions,
// project-level AGENTS.md, referenced skills, and session working memory.
type Composer struct {
	workDir    string
	memoryPath string
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
// resolving $name skill references found in the user prompt, and appending
// session working memory when available.
func (c *Composer) Compose(userPrompt string) (string, error) {
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

	skills, err := c.loadMentionedSkills(userPrompt)
	if err != nil {
		return "", err
	}
	for _, skill := range skills {
		parts = append(parts, skillSection(skill))
	}

	memory, err := c.loadWorkingMemory()
	if err != nil {
		return "", err
	}
	if memory != "" {
		parts = append(parts, section("Session Working Memory", memory))
	}

	return strings.Join(parts, "\n\n"), nil

}

type loadedSkill struct {
	RequestedName string
	Name          string
	Description   string
	Content       string
}

func memoryInstructions() string {
	return strings.TrimSpace(`
Persistent files:
- Session PLAN.md stores the high-level plan for the current session.
- Session TODO.md stores concrete checklist items for the current session.
- Project MEMORY.md stores durable project facts that are useful across sessions.

Rules:
- Use the current session plan and todo to track complex multi-step tasks.
- Add only durable, high-value facts to MEMORY.md.
- DO not dump raw logs or large file contents into memory files.
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

var skillRefPattern = regexp.MustCompile(`\$([a-zA-Z0-9][a-zA-Z0-9_-]*)`)

func (c *Composer) loadMentionedSkills(userPrompt string) ([]loadedSkill, error) {
	names := mentionedSkillNames(userPrompt)
	if len(names) == 0 {
		return nil, nil
	}

	var result []loadedSkill
	for _, name := range names {
		skill, err := c.loadSkill(name)
		if err != nil {
			return nil, err
		}
		result = append(result, skill)
	}

	return result, nil

}

func mentionedSkillNames(input string) []string {
	matches := skillRefPattern.FindAllStringSubmatch(input, -1)
	seen := map[string]bool{}

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		seen[m[1]] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func (c *Composer) loadSkill(name string) (loadedSkill, error) {
	path := filepath.Join(c.workDir, ".foxharness", "skills", name, "SKILL.md")

	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return loadedSkill{}, fmt.Errorf("用户请求了 Skill $%s，但文件不存在: %s", name, path)
	}
	if err != nil {
		return loadedSkill{}, fmt.Errorf("读取 Skill $%s 失败: %w", name, err)
	}

	return parseSkillMarkdown(name, string(content)), nil
}

func parseSkillMarkdown(requestedName, content string) loadedSkill {
	skill := loadedSkill{
		RequestedName: requestedName,
		Name:          requestedName,
		Content:       strings.TrimSpace(content),
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return skill
	}

	closeIndex := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIndex = i
			break
		}
	}
	if closeIndex == -1 {
		return skill
	}

	frontmatter := strings.Join(lines[1:closeIndex], "\n")
	body := strings.Join(lines[closeIndex+1:], "\n")
	for _, line := range strings.Split(frontmatter, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "name":
			if value != "" {
				skill.Name = value
			}
		case "description":
			skill.Description = value
		}
	}

	skill.Content = strings.TrimSpace(body)
	return skill
}

func skillSection(skill loadedSkill) string {
	var b strings.Builder
	if skill.RequestedName != "" && skill.RequestedName != skill.Name {
		b.WriteString(fmt.Sprintf("Requested as: $%s\n\n", skill.RequestedName))
	}
	if skill.Description != "" {
		b.WriteString("Description:\n")
		b.WriteString(skill.Description)
		b.WriteString("\n\n")
	}
	b.WriteString(skill.Content)

	return section("Loaded Skill: "+skill.Name, b.String())
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
