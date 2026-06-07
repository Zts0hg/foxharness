package keeprun

import "strings"

// MergeProhibited reports whether a shell command performs a branch or PR merge
// into an integration branch — an operation keep-run must never perform (spec
// FR-010). It is the policy primitive the bash guard uses so the merge
// prohibition holds by construction rather than by prompt.
//
// It splits the command on shell separators and checks each segment's leading
// command (its prefix): a git subcommand of "merge" (allowing only git global
// options in between), or "gh pr merge" / "glab mr merge". Checking the prefix
// rather than substring means a merge hidden in a chain ("cd x && git merge") is
// caught, while incidental mentions of "merge" — a commit message, or
// "git log --merges" — are not.
func MergeProhibited(command string) bool {
	for _, segment := range splitShellSegments(command) {
		if segmentIsMerge(strings.Fields(segment)) {
			return true
		}
	}
	return false
}

// segmentIsMerge reports whether the leading tokens of one command segment form
// a merge command.
func segmentIsMerge(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	switch tokens[0] {
	case "git":
		// Skip git global options to reach the subcommand. -C and -c take a
		// following argument; other options are single tokens.
		i := 1
		for i < len(tokens) && strings.HasPrefix(tokens[i], "-") {
			if tokens[i] == "-C" || tokens[i] == "-c" {
				i++
			}
			i++
		}
		return i < len(tokens) && tokens[i] == "merge"
	case "gh":
		return len(tokens) >= 3 && tokens[1] == "pr" && tokens[2] == "merge"
	case "glab":
		return len(tokens) >= 3 && tokens[1] == "mr" && tokens[2] == "merge"
	default:
		return false
	}
}

// splitShellSegments splits a command on the common shell separators (&&, ||, |,
// ;, and newlines) so each chained or piped sub-command is checked on its own.
func splitShellSegments(command string) []string {
	r := strings.NewReplacer("&&", "\n", "||", "\n", "|", "\n", ";", "\n")
	return strings.Split(r.Replace(command), "\n")
}
