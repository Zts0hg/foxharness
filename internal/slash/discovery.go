package slash

import (
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxCommandFileSize is the maximum byte length of a .md file that the
// discovery layer will load. Files larger than this limit are skipped with
// a warning log to prevent runaway resource use.
const maxCommandFileSize = 1024 * 1024

const (
	// foxharnessDir is the native directory name searched at both user-level
	// and project-level for command and skill files.
	foxharnessDir = ".foxharness"

	// claudeDir is searched for compatibility with Claude Code slash commands.
	claudeDir = ".claude"
)

const (
	commandsSubdir = "commands"
	skillsSubdir   = "skills"
	skillFileName  = "SKILL.md"
	mdExt          = ".md"
)

// DiscoverCommands walks the user-level and project-level .foxharness/
// directories and returns the prompt commands loaded from each.
//
// Project discovery searches from workDir upward to the git root (the
// nearest ancestor containing .git). Only the closest .foxharness/ directory
// is considered. If workDir or userHome are empty, the corresponding source
// is skipped.
//
// File system errors during traversal are logged and the offending file is
// skipped — discovery never returns a fatal error to its caller.
func DiscoverCommands(workDir, userHome string) (userCmds []*Command, projectCmds []*Command, err error) {
	userCmds = append(userCmds, loadCommandsFromRoot(userHome, claudeDir, SourceClaudeUser)...)
	userCmds = append(userCmds, loadCommandsFromRoot(userHome, foxharnessDir, SourceFoxUser)...)

	if projectRoot := findProjectConfigRoot(workDir, claudeDir); projectRoot != "" {
		projectCmds = append(projectCmds, loadCommandsFromRoot(projectRoot, claudeDir, SourceClaudeProject)...)
	}
	if projectRoot := findProjectConfigRoot(workDir, foxharnessDir); projectRoot != "" {
		projectCmds = append(projectCmds, loadCommandsFromRoot(projectRoot, foxharnessDir, SourceFoxProject)...)
	}

	userCmds = dedupCommands(userCmds)
	projectCmds = dedupCommands(projectCmds)

	sortCommands(userCmds)
	sortCommands(projectCmds)

	return userCmds, projectCmds, nil
}

func findProjectFoxharness(workDir string) string {
	return findProjectConfigRoot(workDir, foxharnessDir)
}

func findProjectConfigRoot(workDir, configDir string) string {
	if workDir == "" {
		return ""
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return ""
	}
	for dir := abs; ; {
		candidate := filepath.Join(dir, configDir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return dir
		}
		// Stop at the git root if encountered without a .foxharness directory.
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func loadCommandsFromRoot(root, configDir string, source CommandSource) []*Command {
	if root == "" {
		return nil
	}
	base := filepath.Join(root, configDir)
	if info, err := os.Stat(base); err != nil || !info.IsDir() {
		return nil
	}

	var commands []*Command
	commands = append(commands, loadFromCommandsDir(filepath.Join(base, commandsSubdir), source)...)
	commands = append(commands, loadFromSkillsDir(filepath.Join(base, skillsSubdir), source)...)
	return commands
}

func loadFromCommandsDir(dir string, source CommandSource) []*Command {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil
	}
	var out []*Command
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("[slash] walk error at %s: %v", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), mdExt) {
			return nil
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}

		// SKILL.md inside commands/ also collapses to the directory name.
		var name string
		if d.Name() == skillFileName {
			name = pathToName(filepath.Dir(rel))
		} else {
			name = pathToName(strings.TrimSuffix(rel, mdExt))
		}
		if name == "" {
			return nil
		}

		cmd, ok := loadCommandFile(path, name, source)
		if !ok {
			return nil
		}
		if d.Name() == skillFileName {
			cmd.SkillDir = filepath.Dir(path)
		}
		out = append(out, cmd)
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		log.Printf("[slash] walking %s failed: %v", dir, walkErr)
	}
	return out
}

func loadFromSkillsDir(dir string, source CommandSource) []*Command {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil
	}
	var out []*Command
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("[slash] walk error at %s: %v", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != skillFileName {
			if strings.HasSuffix(d.Name(), mdExt) {
				rel, _ := filepath.Rel(dir, path)
				if !strings.Contains(rel, string(filepath.Separator)) {
					log.Printf("[slash] ignoring loose .md file in skills/ (directory format required): %s", path)
				}
			}
			return nil
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		name := pathToName(filepath.Dir(rel))
		if name == "" || name == "." {
			return nil
		}

		cmd, ok := loadCommandFile(path, name, source)
		if !ok {
			return nil
		}
		cmd.SkillDir = filepath.Dir(path)
		out = append(out, cmd)
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		log.Printf("[slash] walking %s failed: %v", dir, walkErr)
	}
	return out
}

func pathToName(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return ""
	}
	return strings.ReplaceAll(rel, "/", ":")
}

func loadCommandFile(path, name string, source CommandSource) (*Command, bool) {
	info, err := os.Stat(path)
	if err != nil {
		log.Printf("[slash] stat %s failed: %v", path, err)
		return nil, false
	}
	if info.Size() > maxCommandFileSize {
		log.Printf("[slash] skipping %s: size %d exceeds limit %d", path, info.Size(), maxCommandFileSize)
		return nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[slash] read %s failed: %v", path, err)
		return nil, false
	}

	fm, body, ferr := ParseFrontmatter(data)
	if ferr != nil {
		log.Printf("[slash] frontmatter warning for %s: %v (using defaults)", path, ferr)
	}

	description := fm.Description
	if description == "" {
		description = firstNonBlankLine(body)
	}

	cmd := &Command{
		Type:        CommandPrompt,
		Name:        name,
		Description: description,
		Aliases:     append([]string(nil), fm.Aliases...),
		Source:      source,
		Frontmatter: fm,
		Content:     body,
		FilePath:    path,
	}

	if len(body) == 0 {
		log.Printf("[slash] warning: command %q at %s has empty content", name, path)
	}

	return cmd, true
}

func firstNonBlankLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)
		if trim != "" {
			return trim
		}
	}
	return ""
}

func dedupCommands(in []*Command) []*Command {
	if len(in) <= 1 {
		return in
	}
	seenFiles := make([]os.FileInfo, 0, len(in))
	out := make([]*Command, 0, len(in))
	for _, cmd := range in {
		if info, ok := commandFileInfo(cmd.FilePath); ok {
			if seenSameFile(seenFiles, info) {
				continue
			}
			seenFiles = append(seenFiles, info)
		}
		out = append(out, cmd)
	}
	return out
}

func commandFileInfo(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}

func seenSameFile(seen []os.FileInfo, info os.FileInfo) bool {
	for _, existing := range seen {
		if os.SameFile(existing, info) {
			return true
		}
	}
	return false
}

func sortCommands(cmds []*Command) {
	sort.SliceStable(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
}
