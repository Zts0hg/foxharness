package slash

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const frontmatterDelimiter = "---"

// ParseFrontmatter splits a .md command file into its YAML frontmatter and
// body content. Files that begin with a `---` delimiter line are parsed as
// frontmatter until a matching closing delimiter; everything after the
// closing delimiter is returned as the body.
//
// Behavior on degenerate input:
//   - No opening delimiter on the first line: the entire content is the body
//     and a zero-value Frontmatter is returned with defaults applied.
//   - Opening delimiter but no closing delimiter: a non-fatal error is
//     returned, the body contains the original input, and defaults are used.
//   - Invalid YAML inside a valid frontmatter block: a non-fatal error is
//     returned, the body is preserved, and defaults are used.
//
// In every case the returned Frontmatter has UserInvocable defaulted to true
// unless the YAML explicitly set it to false.
func ParseFrontmatter(content []byte) (Frontmatter, string, error) {
	fm := defaultFrontmatter()

	if !hasOpeningDelimiter(content) {
		return fm, string(content), nil
	}

	yamlBlock, body, ok := extractFrontmatterBlock(content)
	if !ok {
		return fm, string(content), fmt.Errorf("frontmatter missing closing %q delimiter", frontmatterDelimiter)
	}

	parsed, err := unmarshalFrontmatter(yamlBlock)
	if err != nil {
		return fm, body, fmt.Errorf("frontmatter YAML parse failed: %w", err)
	}

	return parsed, body, nil
}

func defaultFrontmatter() Frontmatter {
	return Frontmatter{UserInvocable: true}
}

func hasOpeningDelimiter(content []byte) bool {
	trimmed := bytes.TrimLeft(content, " \t")
	return bytes.HasPrefix(trimmed, []byte(frontmatterDelimiter))
}

func extractFrontmatterBlock(content []byte) (yamlBlock string, body string, ok bool) {
	text := string(content)
	lines := strings.Split(text, "\n")

	if len(lines) == 0 || strings.TrimSpace(lines[0]) != frontmatterDelimiter {
		return "", text, false
	}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontmatterDelimiter {
			closingIdx = i
			break
		}
	}

	if closingIdx == -1 {
		return "", text, false
	}

	yamlBlock = strings.Join(lines[1:closingIdx], "\n")
	body = strings.Join(lines[closingIdx+1:], "\n")
	body = strings.TrimPrefix(body, "\n")
	return yamlBlock, body, true
}

func unmarshalFrontmatter(yamlBlock string) (Frontmatter, error) {
	fm := defaultFrontmatter()

	// First decode into a generic map to detect whether user-invocable was
	// explicitly present, then decode again into the typed struct.
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(yamlBlock), &raw); err != nil {
		return defaultFrontmatter(), err
	}
	explicitInvocable, hasExplicit := raw["user-invocable"]

	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return defaultFrontmatter(), err
	}

	if hasExplicit {
		if v, ok := explicitInvocable.(bool); ok {
			fm.UserInvocable = v
		}
		fm.userInvocableExplicit = true
	} else {
		fm.UserInvocable = true
	}

	return fm, nil
}
