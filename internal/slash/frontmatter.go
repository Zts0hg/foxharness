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
	// explicitly present and to tolerate Claude-style scalar allowed-tools.
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(yamlBlock), &raw); err != nil {
		return defaultFrontmatter(), err
	}
	explicitInvocable, hasExplicit := raw["user-invocable"]

	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		if _, ok := raw["allowed-tools"].(string); !ok {
			return defaultFrontmatter(), err
		}
		fm = frontmatterFromRaw(raw)
	}

	if hasExplicit {
		if v, ok := explicitInvocable.(bool); ok {
			fm.UserInvocable = v
		}
		fm.userInvocableExplicit = true
	} else {
		fm.UserInvocable = true
	}
	fm.AllowedTools = parseAllowedTools(fm.AllowedTools)

	return fm, nil
}

func frontmatterFromRaw(raw map[string]any) Frontmatter {
	fm := defaultFrontmatter()
	fm.Description = rawString(raw, "description")
	fm.Arguments = rawString(raw, "arguments")
	fm.ArgumentHint = rawString(raw, "argument-hint")
	fm.AllowedTools = parseAllowedTools(raw["allowed-tools"])
	fm.Model = rawString(raw, "model")
	fm.Effort = rawString(raw, "effort")
	fm.DisableModelInvocation = rawBool(raw, "disable-model-invocation")
	fm.WhenToUse = rawString(raw, "when_to_use")
	fm.Context = rawString(raw, "context")
	fm.Agent = rawString(raw, "agent")
	fm.Paths = rawStringSlice(raw["paths"])
	fm.Aliases = rawStringSlice(raw["aliases"])
	fm.Version = rawString(raw, "version")
	if hooks, ok := raw["hooks"].(map[string]any); ok {
		fm.Hooks = &FrontmatterHooks{
			Before: rawString(hooks, "before"),
			After:  rawString(hooks, "after"),
		}
	}
	return fm
}

func rawString(raw map[string]any, key string) string {
	if v, ok := raw[key].(string); ok {
		return v
	}
	return ""
}

func rawBool(raw map[string]any, key string) bool {
	if v, ok := raw[key].(bool); ok {
		return v
	}
	return false
}

func rawStringSlice(v any) []string {
	switch typed := v.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{typed}
	default:
		return nil
	}
}

func parseAllowedTools(v any) []string {
	var rawTools []string
	switch typed := v.(type) {
	case string:
		rawTools = splitAllowedToolsString(typed)
	default:
		rawTools = rawStringSlice(v)
	}
	if len(rawTools) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(rawTools))
	out := make([]string, 0, len(rawTools))
	for _, tool := range rawTools {
		normalized := normalizeAllowedTool(tool)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

func splitAllowedToolsString(s string) []string {
	var (
		out   []string
		part  strings.Builder
		depth int
	)
	flush := func() {
		item := strings.TrimSpace(part.String())
		if item != "" {
			out = append(out, item)
		}
		part.Reset()
	}
	for _, r := range s {
		switch r {
		case '(':
			depth++
			part.WriteRune(r)
		case ')':
			if depth > 0 {
				depth--
			}
			part.WriteRune(r)
		case ',':
			if depth == 0 {
				flush()
				continue
			}
			part.WriteRune(r)
		default:
			part.WriteRune(r)
		}
	}
	flush()
	return out
}

func normalizeAllowedTool(tool string) string {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return ""
	}
	if idx := strings.Index(tool, "("); idx >= 0 {
		tool = tool[:idx]
	}
	tool = strings.TrimSpace(tool)
	lower := strings.ToLower(strings.ReplaceAll(tool, "-", "_"))
	switch lower {
	case "read":
		return "read_file"
	case "write":
		return "write_file"
	case "edit", "multiedit":
		return "edit_file"
	case "bash":
		return "bash"
	default:
		return lower
	}
}
