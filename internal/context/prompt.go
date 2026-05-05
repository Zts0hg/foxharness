package context

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Composer struct {
	workDir    string
	memoryPath string
}

func NewComposer(workDir string) *Composer {
	return &Composer{workDir: workDir}
}

func (c *Composer) WithMemory(path string) *Composer {
	clone := *c
	clone.memoryPath = path
	return &clone
}

func (c *Composer) Compose(userPrompt string) (string, error) {
	parts := []string{
		baseSystemPrompt(),
	}

	agents, err := c.loadAgentsFile()
	if err != nil {
		return "", err
	}
	if agents != "" {
		parts = append(parts, section("Project Instructions from AGENTS.md", agents))
	}

	skills, err := c.loadMentionedSkills(userPrompt)
	if err != nil {
		return "", err
	}
	for _, skill := range skills {
		parts = append(parts, section("Loaded Skill: "+skill.Name, skill.Content))
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
	Name    string
	Content string
}

func baseSystemPrompt() string {
	return strings.TrimSpace(`
You are go-tiny-claw, an expert coding assistant running inside an Agent Harness.

Core rules:
- You operate inside the current workspace.
- Prefer reading files before editing them.
- Use edit_file for focused modifications.
- Use write_file only when creating a new file or intentionally replacing a whole file.
- Use bash to inspect, build, test, and verify changes.
- After changing code, verify with the smallest relevant test command.
- If a tool fails, inspect the error and recover instead of blindly repeating the same call.
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

var skillRefPattern = regexp.MustCompile(`\$([a-zA-Z0-9][a-zA-Z0-9_-]*)`)

func (c *Composer) loadMentionedSkills(userPrompt string) ([]loadedSkill, error) {
	names := mentionedSkillNames(userPrompt)
	if len(names) == 0 {
		return nil, nil
	}

	var result []loadedSkill
	for _, name := range names {
		content, err := c.loadSkill(name)
		if err != nil {
			return nil, err
		}
		result = append(result, loadedSkill{Name: name, Content: content})
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

func (c *Composer) loadSkill(name string) (string, error) {
	path := filepath.Join(c.workDir, ".foxharness", "skills", name, "SKILL.md")

	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("用户请求了 Skill $%s，但文件不存在: %s", name, path)
	}
	if err != nil {
		return "", fmt.Errorf("读取 Skill $%s 失败: %w", name, err)
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
