package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// EditFileTool performs targeted edits to files by replacing old text with new text.
// It supports multiple matching strategies for robustness:
//  1. Exact match
//  2. Newline normalization
//  3. Line-level whitespace normalization
//  4. Fuzzy matching with similarity scoring
type EditFileTool struct {
	// workDir is the base directory for resolving relative file paths.
	workDir string
}

// NewEditFileTool creates a new EditFileTool that edits files relative to the specified directory.
// The workDir parameter sets the base directory for file path resolution.
// Returns a configured EditFileTool.
func NewEditFileTool(workDir string) *EditFileTool {
	return &EditFileTool{workDir: workDir}
}

// Name returns the tool identifier "edit_file".
func (t *EditFileTool) Name() string {
	return "edit_file"
}

// Definition returns the tool schema for the edit_file tool.
// It describes the tool's capabilities and expected input format.
func (t *EditFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "Perform targeted edits to a file by replacing old_string with new_string. Uses exact match first; falls back to newline normalization, line-level whitespace normalization, and high-confidence fuzzy matching. old_string should be unique.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to edit, e.g., internal/foo/bar.go. Use relative paths from working directory.",
				},
				"old_string": map[string]interface{}{
					"type":        "string",
					"description": "The original text fragment to replace. Provide sufficient unique context.",
				},

				"new_string": map[string]interface{}{
					"type":        "string",
					"description": "The new text fragment to replace with.",
				},
			},
			"required": []string{"path", "old_string", "new_string"},
		},
	}
}

// editFileArgs represents the input arguments for the edit_file tool.
type editFileArgs struct {
	// Path is the relative path to the file to edit.
	Path string `json:"path"`
	// OldString is the original text to be replaced.
	OldString string `json:"old_string"`
	// NewString is the replacement text.
	NewString string `json:"new_string"`
}

// Execute performs a targeted edit on the specified file.
// Uses multiple matching strategies in order:
//  1. Exact match of old_string
//  2. Match after normalizing line endings
//  3. Match after trimming line-level whitespace
//  4. Fuzzy matching with similarity scoring
//
// Returns a success message with match strategy and line number,
// or an error if the edit cannot be performed safely.
func (t *EditFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input editFileArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	if strings.TrimSpace(input.Path) == "" {
		return "", fmt.Errorf("path 不能为空")
	}

	if input.OldString == "" {
		return "", fmt.Errorf("old_string 不能为空。请先读取文件，并提供要替换的旧文本片段")
	}

	if input.OldString == input.NewString {
		return "", fmt.Errorf("old_string 与 new_string 完全相同，无需修改")
	}

	fullPath := filepath.Join(t.workDir, input.Path)

	info, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取文件信息失败: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("目标路径是目录，不是普通文件: %s", input.Path)
	}

	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	updated, match, err := applyBestEffortEdit(string(raw), input.OldString, input.NewString)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(fullPath, []byte(updated), info.Mode()); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return fmt.Sprintf(
		"成功修改文件: %s\n匹配策略: %s\n起始行号: %d",
		input.Path,
		match.Strategy,
		match.Line,
	), nil
}

// editMatch describes the result of a successful edit operation.
type editMatch struct {
	// Strategy is the matching method used (e.g., "exact", "fuzzy_line_window").
	Strategy string
	// Line is the 1-based line number where the edit was applied.
	Line int
	// Score is the similarity score for fuzzy matches (1.0 = exact).
	Score float64
}

// applyBestEffortEdit attempts to replace old_string with newString in the content.
// Tries multiple matching strategies in order of precision.
// Returns the updated content, match information, or an error if no unique match is found.
func applyBestEffortEdit(content, oldString, newString string) (string, editMatch, error) {
	if count := strings.Count(content, oldString); count == 1 {
		idx := strings.Index(content, oldString)
		updated := content[:idx] + newString + content[idx+len(oldString):]
		return updated, editMatch{
			Strategy: "exact",
			Line:     lineNumberAt(content, idx),
			Score:    1.0,
		}, nil
	} else if count > 1 {
		return "", editMatch{}, fmt.Errorf(
			"old_string 在文件中出现了 %d 次。为了避免误改，请提供更长、更唯一的上下文",
			count,
		)
	}

	eol := detectLineEnding(content)
	normalizedContent := normalizeNewLines(content)
	normalizedOld := normalizeNewLines(oldString)
	normalizedNew := normalizeNewLines(newString)

	if count := strings.Count(normalizedContent, normalizedOld); count == 1 {
		idx := strings.Index(normalizedContent, normalizedOld)
		updated := normalizedContent[:idx] + normalizedNew + normalizedContent[idx+len(oldString):]
		return restoreLineEnding(updated, eol), editMatch{
			Strategy: "exact_after_new_normalization",
			Line:     lineNumberAt(normalizedContent, idx),
			Score:    1.0,
		}, nil
	} else if count > 1 {
		return "", editMatch{}, fmt.Errorf(
			"换行归一化后 old_string 在文件中出现了 %d 次。为了避免误改，请提供更长、更唯一的上下文",
			count,
		)
	}

	if updated, line, ok, ambiguous := replaceByTrimmedLines(normalizedContent, normalizedOld, normalizedNew); ok {
		return restoreLineEnding(updated, eol), editMatch{
			Strategy: "line_trimmed",
			Line:     line,
			Score:    1.0,
		}, nil
	} else if ambiguous {
		return "", editMatch{}, fmt.Errorf("行级空白归一化后出现多个候选位置。为了避免误改，请提供更长、更唯一的上下文")
	}

	updated, line, score, secondScore, ok, ambiguous := replaceByFuzzyLines(normalizedContent, normalizedOld, normalizedNew)
	if ok {
		return restoreLineEnding(updated, eol), editMatch{
			Strategy: fmt.Sprintf("fuzzy_line_window(score=%.3f, second=%.3f)", score, secondScore),
			Line:     line,
			Score:    score,
		}, nil
	}

	if ambiguous {
		return "", editMatch{}, fmt.Errorf(
			"模糊匹配结果存在歧义：最佳候选相似度 %.3f，第二候选 %.3f。为了避免误改，请提供更长、更唯一的上下文",
			score,
			secondScore,
		)
	}

	return "", editMatch{}, fmt.Errorf(
		"无法在文件中定位 old_string。请先调用 read_file 获取最新内容，再使用更准确的 old_string 重试",
	)
}

func detectLineEnding(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}

	return "\n"
}

func normalizeNewLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func restoreLineEnding(s, eol string) string {
	if eol == "\r\n" {
		return strings.ReplaceAll(s, "\n", "\r\n")
	}

	return s
}

func lineNumberAt(s string, byteIndex int) int {
	if byteIndex <= 0 {
		return 1
	}
	if byteIndex > len(s) {
		byteIndex = len(s)
	}

	return strings.Count(s[:byteIndex], "\n") + 1
}

func replaceByTrimmedLines(content, oldString, newString string) (string, int, bool, bool) {
	lines := splitLogicalLines(content)
	oldLines := splitLogicalLines(oldString)
	if len(oldLines) == 0 || len(oldLines) > len(lines) {
		return "", 0, false, false
	}

	target := normalizeTrimmedBlock(oldLines)
	var matches []int

	for i := 0; i+len(oldLines) <= len(lines); i++ {
		candidates := normalizeTrimmedBlock(lines[i : i+len(oldLines)])
		if candidates == target {
			matches = append(matches, i)
		}
	}

	if len(matches) == 0 {
		return "", 0, false, false
	}

	if len(matches) > 1 {
		return "", 0, false, true
	}

	startLine := matches[0]
	updated := replaceLineRange(content, startLine, startLine+len(oldLines), newString)
	return updated, startLine + 1, true, false
}

func splitLogicalLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}

	return strings.Split(s, "\n")
}

func normalizeTrimmedBlock(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(strings.TrimSpace(line))
		b.WriteByte('\n')
	}
	return b.String()
}

func replaceLineRange(content string, startLine, endLine int, replacement string) string {
	start, end := lineByteRange(content, startLine, endLine)

	replacedRange := content[start:end]
	if (end < len(content) || strings.HasSuffix(replacedRange, "\n")) && !strings.HasSuffix(replacement, "\n") {
		replacement += "\n"
	}

	return content[:start] + replacement + content[endLine:]
}

func lineByteRange(content string, startLine, endLine int) (int, int) {
	offsets := lineStartOffset(content)

	if startLine < 0 {
		startLine = 0
	}

	if endLine < startLine {
		endLine = startLine
	}

	if startLine >= len(offsets) {
		return len(content), len(content)
	}

	start := offsets[startLine]
	end := len(content)
	if endLine < len(offsets) {
		end = offsets[endLine]
	}
	return start, end
}

func lineStartOffset(content string) []int {
	offsets := []int{0}
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' && i+1 < len(content) {
			offsets = append(offsets, i+1)
		}
	}

	return offsets
}

func replaceByFuzzyLines(content, oldString, newString string) (string, int, float64, float64, bool, bool) {
	lines := splitLogicalLines(content)
	oldLines := splitLogicalLines(oldString)
	if len(lines) == 0 || len(oldLines) == 0 {
		return "", 0, 0, 0, false, false
	}

	target := normalizeFuzzyText(strings.Join(oldLines, "\n"))
	targetLen := len(oldLines)

	minWindow := targetLen - 2
	if minWindow < 1 {
		minWindow = 1
	}

	maxWindow := targetLen + 2
	if maxWindow > len(lines) {
		maxWindow = len(lines)
	}

	bestScore := 0.0
	secondScore := 0.0
	bestStart := -1
	bestEnd := -1

	for window := minWindow; window <= maxWindow; window++ {
		for i := 0; i+window < len(lines); i++ {
			candidates := normalizeFuzzyText(strings.Join(lines[i:i+window], "\n"))
			score := similarity(target, candidates)

			if score > bestScore {
				secondScore = bestScore
				bestScore = score
				bestStart = i
				bestEnd = i + window
			} else if score > secondScore {
				secondScore = score
			}
		}
	}

	const minScore = 0.88
	const minGap = 0.04
	if bestScore < minScore {
		return "", 0, bestScore, secondScore, false, false
	}

	if math.Abs(bestScore-secondScore) < minGap {
		return "", 0, bestScore, secondScore, false, true
	}

	updated := replaceLineRange(content, bestStart, bestEnd, newString)
	return updated, bestStart + 1, bestScore, secondScore, true, false

}

func normalizeFuzzyText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}

	if a == "" || b == "" {
		return 0.0
	}

	ar := []rune(a)
	br := []rune(b)
	dist := levenshtein(ar, br)
	maxLen := len(ar)
	if len(br) > maxLen {
		maxLen = len(br)
	}

	return 1.0 - float64(dist)/float64(maxLen)
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr := make([]int, len(b)+1)
		curr[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}

		prev = curr
	}

	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
