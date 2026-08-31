package skilltool

import "github.com/Zts0hg/foxharness/internal/slash"

/* FormatActivationReminder renders one newly activated model-invocable skill. */
func FormatActivationReminder(command *slash.Command) string {
	if command == nil {
		return ""
	}
	result := "A new skill became available for the rest of this session: `" + command.Name + "`"
	if command.Description != "" {
		result += "\n  Description: " + command.Description
	}
	if command.Frontmatter.WhenToUse != "" {
		result += "\n  When to use: " + command.Frontmatter.WhenToUse
	}
	if command.Frontmatter.ArgumentHint != "" {
		result += "\n  Arguments: " + command.Frontmatter.ArgumentHint
	} else if command.Frontmatter.Arguments != "" {
		result += "\n  Arguments: " + command.Frontmatter.Arguments
	}
	result += "\nInvoke it via the `skill` tool with name=\"" + command.Name + "\"."
	return result
}
