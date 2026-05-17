package tui

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	maxFileMentionIndex   = 5000
	maxFileMentionMatches = 8
)

type fileMention struct {
	Path string
}

func loadFileMentions(workDir string) []fileMention {
	mentions, err := discoverFileMentions(workDir)
	if err != nil {
		return nil
	}
	return mentions
}

func discoverFileMentions(workDir string) ([]fileMention, error) {
	root := strings.TrimSpace(workDir)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	ignore := loadGitIgnore(absRoot)
	var mentions []fileMention
	err = filepath.WalkDir(absRoot, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if current == absRoot {
			return nil
		}

		rel, err := filepath.Rel(absRoot, current)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".git/") || ignore.ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignore.ignored(rel, false) {
			return nil
		}

		mentions = append(mentions, fileMention{Path: rel})
		if len(mentions) >= maxFileMentionIndex {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(mentions, func(i, j int) bool {
		return mentions[i].Path < mentions[j].Path
	})
	return mentions, nil
}

func (m Model) activeFileMention() (start int, end int, query string, ok bool) {
	if len(m.input) == 0 {
		return 0, 0, "", false
	}

	end = len(m.input)
	start = end
	for start > 0 && !unicode.IsSpace(m.input[start-1]) {
		start--
	}
	token := m.input[start:end]
	if len(token) == 0 || token[0] != '@' {
		return 0, 0, "", false
	}

	query = strings.TrimPrefix(string(token[1:]), "./")
	return start, end, query, true
}

func matchFileMentions(files []fileMention, query string) []fileMention {
	query = normalizeFileMentionQuery(query)
	if len(files) == 0 {
		return nil
	}
	if query == "" {
		return firstFileMentions(files, maxFileMentionMatches)
	}

	var matches []fileMention
	addMatches := func(match func(fileMention) bool) {
		for _, file := range files {
			if len(matches) >= maxFileMentionMatches {
				return
			}
			if containsFileMention(matches, file) || !match(file) {
				continue
			}
			matches = append(matches, file)
		}
	}

	addMatches(func(file fileMention) bool {
		return strings.HasPrefix(strings.ToLower(file.Path), query)
	})
	addMatches(func(file fileMention) bool {
		return strings.HasPrefix(strings.ToLower(path.Base(file.Path)), query)
	})
	addMatches(func(file fileMention) bool {
		return strings.Contains(strings.ToLower(file.Path), query)
	})
	return matches
}

func normalizeFileMentionQuery(query string) string {
	query = strings.TrimSpace(query)
	query = strings.TrimPrefix(query, "@")
	query = strings.TrimPrefix(query, "./")
	query = filepath.ToSlash(query)
	return strings.ToLower(query)
}

func firstFileMentions(files []fileMention, limit int) []fileMention {
	if limit > len(files) {
		limit = len(files)
	}
	if limit <= 0 {
		return nil
	}
	return append([]fileMention(nil), files[:limit]...)
}

func containsFileMention(files []fileMention, target fileMention) bool {
	for _, file := range files {
		if file.Path == target.Path {
			return true
		}
	}
	return false
}

type gitIgnore struct {
	rules []gitIgnoreRule
}

type gitIgnoreRule struct {
	pattern       string
	negated       bool
	directoryOnly bool
	anchored      bool
	hasSlash      bool
	regex         *regexp.Regexp
}

func loadGitIgnore(root string) gitIgnore {
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return gitIgnore{}
	}

	var rules []gitIgnoreRule
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		rule, ok := parseGitIgnoreRule(raw)
		if ok {
			rules = append(rules, rule)
		}
	}
	return gitIgnore{rules: rules}
}

func parseGitIgnoreRule(raw string) (gitIgnoreRule, bool) {
	pattern := strings.TrimSpace(raw)
	if pattern == "" {
		return gitIgnoreRule{}, false
	}
	if strings.HasPrefix(pattern, `\#`) {
		pattern = strings.TrimPrefix(pattern, `\`)
	} else if strings.HasPrefix(pattern, "#") {
		return gitIgnoreRule{}, false
	}

	negated := false
	if strings.HasPrefix(pattern, `\!`) {
		pattern = strings.TrimPrefix(pattern, `\`)
	} else if strings.HasPrefix(pattern, "!") {
		negated = true
		pattern = strings.TrimPrefix(pattern, "!")
	}

	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	directoryOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	if pattern == "" {
		return gitIgnoreRule{}, false
	}

	regex, err := regexp.Compile("^" + gitIgnoreGlobRegex(pattern) + "$")
	if err != nil {
		return gitIgnoreRule{}, false
	}
	return gitIgnoreRule{
		pattern:       pattern,
		negated:       negated,
		directoryOnly: directoryOnly,
		anchored:      anchored,
		hasSlash:      strings.Contains(pattern, "/"),
		regex:         regex,
	}, true
}

func (g gitIgnore) ignored(rel string, isDir bool) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" {
		return false
	}

	ignored := false
	for _, rule := range g.rules {
		if rule.matches(rel, isDir) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func (r gitIgnoreRule) matches(rel string, isDir bool) bool {
	rel = strings.TrimSuffix(filepath.ToSlash(rel), "/")
	if r.directoryOnly && !isDir {
		return false
	}
	if r.hasSlash {
		return r.matchesPath(rel)
	}
	return r.matchesSegment(rel)
}

func (r gitIgnoreRule) matchesPath(rel string) bool {
	if r.anchored {
		return r.regex.MatchString(rel)
	}
	if r.regex.MatchString(rel) {
		return true
	}
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		if r.regex.MatchString(strings.Join(parts[i:], "/")) {
			return true
		}
	}
	return false
}

func (r gitIgnoreRule) matchesSegment(rel string) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if r.regex.MatchString(part) {
			return true
		}
	}
	return false
}

func gitIgnoreGlobRegex(pattern string) string {
	var out strings.Builder
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; r {
		case '*':
			if i+1 < len(runes) && runes[i+1] == '*' {
				out.WriteString(".*")
				i++
			} else {
				out.WriteString("[^/]*")
			}
		case '?':
			out.WriteString("[^/]")
		case '\\':
			if i+1 < len(runes) {
				i++
				out.WriteString(regexp.QuoteMeta(string(runes[i])))
			} else {
				out.WriteString(regexp.QuoteMeta(string(r)))
			}
		default:
			out.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return out.String()
}
