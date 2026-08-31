package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Zts0hg/foxharness/internal/prompt"
)

/* AutoMemoryStore supplies persistent-memory values without exposing its concrete storage mechanism. */
type AutoMemoryStore interface {
	/* MergedIndexString returns the merged two-tier description index or an empty string. */
	MergedIndexString() string
	/* UserGlobalDir returns the absolute user-global memory directory. */
	UserGlobalDir() string
	/* ProjectDir returns the absolute project-scoped memory directory. */
	ProjectDir() string
}

/* AutoMemoryGuidance renders already-resolved persistent-memory locations without granting runtime storage ownership. */
type AutoMemoryGuidance func(userDirRel, projectDirRel string) string

/* PromptCollector resolves ordered prompt fragments from one frozen runtime request and injected sources. */
type PromptCollector struct {
	workDir            string
	memoryPath         string
	memoryRO           bool
	skillListFn        func() string
	interactiveAsk     bool
	collaborationMode  string
	autoMemory         AutoMemoryStore
	autoMemoryGuidance AutoMemoryGuidance
	toolCapabilities   map[string]struct{}
}

/* WithSkillList returns a copy that obtains the formatted model-invocable skill list from fn. */
func (c *PromptCollector) WithSkillList(fn func() string) *PromptCollector {
	clone := *c
	clone.skillListFn = fn
	return &clone
}

/* NewPromptCollector creates a collector rooted at workDir. */
func NewPromptCollector(workDir string) *PromptCollector {
	return &PromptCollector{workDir: workDir}
}

/* WithMemory returns a copy that loads writable session working memory from path. */
func (c *PromptCollector) WithMemory(path string) *PromptCollector {
	clone := *c
	clone.memoryPath = path
	clone.memoryRO = false
	return &clone
}

/* WithReadOnlyMemory returns a copy that injects session working memory with read-only guidance. */
func (c *PromptCollector) WithReadOnlyMemory(path string) *PromptCollector {
	clone := *c
	clone.memoryPath = path
	clone.memoryRO = true
	return &clone
}

/* WithAutoMemory returns a copy that injects persistent-memory values and caller-supplied guidance. */
func (c *PromptCollector) WithAutoMemory(store AutoMemoryStore, guidance AutoMemoryGuidance) *PromptCollector {
	clone := *c
	clone.autoMemory = store
	clone.autoMemoryGuidance = guidance
	return &clone
}

/* WithInteractiveAsk returns a copy that may advertise the interactive question capability. */
func (c *PromptCollector) WithInteractiveAsk(enabled bool) *PromptCollector {
	clone := *c
	clone.interactiveAsk = enabled
	return &clone
}

/* WithToolCapabilities returns a copy whose guidance is restricted to the supplied model-visible tools. */
func (c *PromptCollector) WithToolCapabilities(names []string) *PromptCollector {
	clone := *c
	clone.toolCapabilities = make(map[string]struct{}, len(names))
	for _, name := range names {
		clone.toolCapabilities[name] = struct{}{}
	}
	return &clone
}

func (c *PromptCollector) withCollaborationMode(mode string) *PromptCollector {
	clone := *c
	clone.collaborationMode = normalizePromptCollaborationMode(mode)
	return &clone
}

/* Collect resolves the complete fragment set from one frozen runtime request. */
func (c *PromptCollector) Collect(_ context.Context, request ContextCollectionRequest) ([]prompt.Fragment, error) {
	clone := *c
	if request.WorkDir != "" {
		clone.workDir = request.WorkDir
	}
	clone.collaborationMode = normalizePromptCollaborationMode(request.CollaborationMode)
	if request.RestrictedTools {
		clone = *clone.WithToolCapabilities(request.AllowedTools)
	}
	fragments, err := clone.composeFragments(request.Prompt)
	if err != nil {
		return nil, fmt.Errorf("组装系统提示词失败: %w", err)
	}
	return fragments, nil
}

func (c *PromptCollector) compose(userPrompt string) (string, error) {
	parts, err := c.composeFragments(userPrompt)
	if err != nil {
		return "", err
	}
	return prompt.Render(parts), nil
}

func (c *PromptCollector) composeFragments(userPrompt string) ([]prompt.Fragment, error) {
	basePrompt := baseSystemPrompt()
	if c.toolCapabilities != nil {
		basePrompt = capabilityScopedSystemPrompt(c.toolCapabilities)
	}
	parts := []prompt.Fragment{prompt.Text(basePrompt)}
	if c.collaborationMode == formalPlanCollaborationMode {
		parts = append(parts, prompt.Section("Formal Plan Collaboration Mode", formalPlanGuidance()))
	}
	if c.interactiveAsk && (c.toolCapabilities == nil || hasCapability(c.toolCapabilities, "ask_user_question")) {
		parts = append(parts, prompt.Section("Asking the User", askGuidance()))
	}
	memoryGuidance := memoryInstructions()
	if c.collaborationMode == formalPlanCollaborationMode {
		memoryGuidance = formalPlanMemoryInstructions()
	} else if c.toolCapabilities != nil {
		memoryGuidance = capabilityScopedTodoInstructions(c.toolCapabilities)
	}
	if memoryGuidance != "" {
		parts = append(parts, prompt.Section("Session Plan and Todo Files", memoryGuidance))
	}

	if c.autoMemory != nil && (c.toolCapabilities == nil || hasCapability(c.toolCapabilities, "read_file")) {
		parts = append(parts, prompt.Section("Persistent Memory", c.persistentMemoryBody()))
	}

	agents, err := c.loadAgentsFile()
	if err != nil {
		return nil, err
	}
	if agents != "" {
		parts = append(parts, prompt.Section("Project Instructions from AGENTS.md", agents))
	}

	skills, err := c.loadMentionedSkills(userPrompt)
	if err != nil {
		return nil, err
	}
	for _, skill := range skills {
		parts = append(parts, skillFragment(skill))
	}

	if c.memoryPath != "" {
		memory, err := c.loadWorkingMemory()
		if err != nil {
			return nil, err
		}
		parts = append(parts, prompt.Section("Session Working Memory", workingMemoryBody(
			memory,
			c.relToWorkDir(c.memoryPath),
			c.memoryRO,
			c.collaborationMode == formalPlanCollaborationMode,
		)))
	}

	if c.skillListFn != nil && (c.toolCapabilities == nil || hasCapability(c.toolCapabilities, "skill")) {
		if list := strings.TrimSpace(c.skillListFn()); list != "" {
			parts = append(parts, prompt.Section("Available Skills (invoke via the `skill` tool)", list))
		}
	}

	return parts, nil
}

const formalPlanCollaborationMode = "formal_plan"

func normalizePromptCollaborationMode(mode string) string {
	if strings.TrimSpace(mode) == formalPlanCollaborationMode {
		return formalPlanCollaborationMode
	}
	return ""
}

func capabilityScopedSystemPrompt(capabilities map[string]struct{}) string {
	lines := []string{
		"You are fox-harness, an expert coding assistant running inside an Agent Harness.",
		"",
		"Core rules:",
		"- You operate inside the current workspace.",
	}
	if hasCapability(capabilities, "read_file") {
		lines = append(lines, "- Use read_file to inspect files before making claims or edits.")
	}
	if hasCapability(capabilities, "edit_file") {
		lines = append(lines, "- Use edit_file for focused modifications.")
	}
	if hasCapability(capabilities, "write_file") {
		lines = append(lines, "- Use write_file only when creating a new file or intentionally replacing a whole file.")
	}
	if hasCapability(capabilities, "bash") {
		lines = append(lines, "- Use bash only for commands permitted by its active capability policy.")
	}
	if len(capabilities) > 0 {
		lines = append(lines, "- If a tool fails, inspect the error and recover instead of blindly repeating the same call.")
	}
	lines = append(lines, "- Keep changes small, explicit, and aligned with the assigned task.")
	return strings.Join(lines, "\n")
}

func capabilityScopedTodoInstructions(capabilities map[string]struct{}) string {
	read := hasCapability(capabilities, "read_todo")
	update := hasCapability(capabilities, "update_todo")
	if !read && !update {
		return ""
	}
	lines := []string{"Session TODO.md stores concrete checklist items for the current session."}
	if read {
		lines = append(lines, "- Use read_todo to inspect Session TODO.md.")
	}
	if update {
		lines = append(lines, "- Use update_todo to maintain Session TODO.md.")
	}
	return strings.Join(lines, "\n")
}

func hasCapability(capabilities map[string]struct{}, name string) bool {
	_, ok := capabilities[name]
	return ok
}

type loadedSkill struct {
	RequestedName string
	Name          string
	Description   string
	Content       string
}

func askGuidance() string {
	return strings.TrimSpace(`
- When the user's request is ambiguous, underspecified, or hinges on a decision only they can make (scope, tech choice, trade-offs, which-of-several), call the ask_user_question tool with structured multiple-choice options instead of asking in free-form prose.
- Prefer ask_user_question whenever the clarification reduces to a small set of discrete choices; reserve prose for genuinely open-ended discussion.
- Do not guess on a material decision that is the user's to make — ask first, then proceed.
`)
}

func formalPlanGuidance() string {
	return strings.TrimSpace(`
These Formal Plan instructions override the general execution guidance until the user approves a submitted plan.

Before approval:
- Work only on understanding, read-only exploration, clarification, and a complete implementation proposal.
- Do not create or modify project files, including generated files, configuration, tests, or source code.
- Do not change Git state: do not checkout or switch branches, commit, clean, reset, stage, stash, merge, rebase, or otherwise mutate the repository.
- Do not run commands whose purpose is to implement the solution or cause other side effects.
- Bash is available only for read-only repository, Git, system, environment, network-status, and feasibility inspection. This is an instruction boundary, not a security sandbox.
- Use ask_user_question when a material user decision is required.
- When exploration and clarification are complete, call submit_plan with one complete Markdown proposal. Do not edit PLAN.md directly and do not submit incremental plan fragments.

After submit_plan:
- If the user continues planning or supplies feedback, remain read-only, incorporate that feedback, and submit a complete replacement proposal when ready.
- If the user approves, the runtime transitions this same task to Default mode and supplies the complete approved plan again.
- After approval, read-only revalidation is allowed, but before any implementation action you must derive an ordered, executable, verifiable checklist from the approved plan and successfully call update_todo with the complete TODO.md content.
`)
}

func memoryInstructions() string {
	return strings.TrimSpace(`
Session-scoped files (they perish with the session):
- Session PLAN.md stores the high-level plan for the current session.
- Session TODO.md stores concrete checklist items for the current session.

Rules:
- Use the current session plan and todo to track complex multi-step tasks.
- Use read_todo and update_todo to inspect and maintain Session TODO.md.
- Do not use bash, write_file, or edit_file to modify Session TODO.md.
- Do not dump raw logs or large file contents into these files.
`)
}

func formalPlanMemoryInstructions() string {
	return strings.TrimSpace(`
Session-scoped files (they perish with the session):
- Session PLAN.md stores the latest complete proposal submitted through submit_plan.
- Session TODO.md stores the execution checklist created after approval.

Rules:
- Before approval, do not modify PLAN.md or TODO.md directly and do not use update_todo.
- submit_plan is the only mechanism that may replace Session PLAN.md.
- Revision feedback does not edit PLAN.md; respond with a complete replacement through submit_plan.
- After approval, use update_todo to initialize and maintain Session TODO.md. It never edits PLAN.md.
- Do not use bash, write_file, or edit_file to modify either session artifact.
`)
}

/*
workingMemoryGuidance distinguishes the session scratchpad from persistent memory
and names its workspace-relative path for file-tool updates.
*/
func workingMemoryGuidance(relPath string) string {
	var b strings.Builder
	b.WriteString("working_memory.md is your session-scoped scratchpad. It perishes when this session ends and is separate from the cross-session Persistent Memory above — do not put durable cross-session knowledge here.\n")
	fmt.Fprintf(&b, "Keep it current as you work by editing the session file at relative path %q using the write_file and edit_file tools (paths resolve against the working directory). Maintain these sections:\n", relPath)
	b.WriteString("- Goal: what the user ultimately wants from this session.\n")
	b.WriteString("- Known Facts: facts you have confirmed this session.\n")
	b.WriteString("- Current Plan: the approach you are taking.\n")
	b.WriteString("- Next Step: the immediate next action.\n")
	b.WriteString("Current contents:")
	return b.String()
}

func readOnlyWorkingMemoryGuidance(relPath string) string {
	var b strings.Builder
	b.WriteString("working_memory.md is this run's session scratchpad. It is read-only in this run because the available tools do not include working-memory write access.\n")
	fmt.Fprintf(&b, "Use the current contents below for context. If the scratchpad should change, include the update in your final report instead of editing the file at relative path %q.\n", relPath)
	b.WriteString("Current contents:")
	return b.String()
}

func formalPlanWorkingMemoryGuidance(relPath string) string {
	var b strings.Builder
	b.WriteString("Before approval, working_memory.md is read-only. Use its current contents for planning context, but do not edit the file.\n")
	fmt.Fprintf(&b, "After approval, resume normal working_memory.md maintenance at relative path %q with write_file or edit_file once the lifecycle exposes those tools. Maintain these sections:\n", relPath)
	b.WriteString("- Goal: what the user ultimately wants from this session.\n")
	b.WriteString("- Known Facts: facts you have confirmed this session.\n")
	b.WriteString("- Current Plan: the approach you are taking.\n")
	b.WriteString("- Next Step: the immediate next action.\n")
	b.WriteString("Current contents:")
	return b.String()
}

/* workingMemoryBody combines maintenance guidance with the current file contents. */
func workingMemoryBody(current, relPath string, readOnly bool, formalPlan bool) string {
	current = strings.TrimSpace(current)
	if current == "" {
		current = "(empty)"
	}
	if formalPlan {
		return formalPlanWorkingMemoryGuidance(relPath) + "\n\n" + current
	}
	if readOnly {
		return readOnlyWorkingMemoryGuidance(relPath) + "\n\n" + current
	}
	return workingMemoryGuidance(relPath) + "\n\n" + current
}

/* persistentMemoryBody renders the persistent index and injected lifecycle guidance. */
func (c *PromptCollector) persistentMemoryBody() string {
	index := strings.TrimSpace(c.autoMemory.MergedIndexString())
	userRel := c.relToWorkDir(c.autoMemory.UserGlobalDir())
	projectRel := c.relToWorkDir(c.autoMemory.ProjectDir())

	guidance := ""
	if c.autoMemoryGuidance != nil {
		guidance = c.autoMemoryGuidance(userRel, projectRel)
	}
	if c.collaborationMode == formalPlanCollaborationMode {
		guidance = strings.TrimSpace(`
Before approval, persistent memory is read-only. You may inspect relevant memories with read_file, but do not create, update, delete, or otherwise persist memory files.
After approval, resume normal persistent memory maintenance once the lifecycle exposes write tools. The write instructions below apply only after approval.
`) + "\n\n" + guidance
	}
	if index == "" {
		return "No memories saved yet.\n\n" + guidance
	}
	return "Current memory index (read a file for its full content when relevant):\n" + index + "\n\n" + guidance
}

/* relToWorkDir selects a normalized tool-usable path relative to the collector workspace. */
func (c *PromptCollector) relToWorkDir(abs string) string {
	workDir := normalizeForRel(c.workDir)
	targets := []string{normalizeForRel(abs)}
	if resolved, err := filepath.EvalSymlinks(targets[0]); err == nil {
		resolved = filepath.Clean(resolved)
		if resolved != targets[0] {
			targets = append(targets, resolved)
		}
	}

	best := ""
	for _, target := range targets {
		rel, err := filepath.Rel(workDir, target)
		if err != nil {
			continue
		}
		if best == "" || len(rel) < len(best) {
			best = rel
		}
	}
	if best != "" {
		return best
	}
	return filepath.Clean(targets[0])
}

func normalizeForRel(path string) string {
	normalized := path
	if abs, err := filepath.Abs(path); err == nil {
		normalized = abs
	}
	return filepath.Clean(normalized)
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

func (c *PromptCollector) loadAgentsFile() (string, error) {
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

func (c *PromptCollector) loadMentionedSkills(userPrompt string) ([]loadedSkill, error) {
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

func (c *PromptCollector) loadSkill(name string) (loadedSkill, error) {
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

func skillFragment(skill loadedSkill) prompt.Fragment {
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

	return prompt.Section("Loaded Skill: "+skill.Name, b.String())
}

func (c *PromptCollector) loadWorkingMemory() (string, error) {
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
