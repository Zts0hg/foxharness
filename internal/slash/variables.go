package slash

import "strings"

// Built-in variable names that the executor populates by default before
// invoking ReplaceVariables. Skills may rely on these always being defined.
const (
	VarSkillDir  = "FOXHARNESS_SKILL_DIR"
	VarSessionID = "FOXHARNESS_SESSION_ID"
)

// ReplaceVariables substitutes ${NAME} occurrences in content with the
// values from vars. Names not in vars are left untouched. Empty values are
// substituted as empty strings (the placeholder is removed).
func ReplaceVariables(content string, vars map[string]string) string {
	if len(vars) == 0 || !strings.Contains(content, "${") {
		return content
	}
	out := content
	for name, value := range vars {
		out = strings.ReplaceAll(out, "${"+name+"}", value)
	}
	return out
}
