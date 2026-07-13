package permission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// Source identifies the runtime path that produced a tool call.
type Source string

const (
	SourceMain     Source = "main"
	SourceSubagent Source = "subagent"
	SourceSkill    Source = "skill"
	SourceFork     Source = "fork"
)

// Request is the canonical approval input for one tool invocation.
type Request struct {
	ToolCall  schema.ToolCall
	ToolName  string
	Arguments string
	CWD       string
	Workspace string
	Action    string
	Risk      Risk
	Source    Source
}

// GrantKey identifies an exact session authorization scope.
type GrantKey string

// Grant records one typed in-memory session authorization.
type Grant struct {
	Key      GrantKey
	ToolName string
	Action   string
	CWD      string
	Source   Source
}

// GrantKeyFor creates a deterministic key for exact equivalent calls.
func GrantKeyFor(request Request) GrantKey {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		request.ToolName,
		normalizeJSON(request.ToolCall.Arguments),
		cleanPath(request.CWD),
		cleanPath(request.Workspace),
		string(request.Source),
	}, "\x00")))
	return GrantKey(hex.EncodeToString(sum[:]))
}

// GrantForRequest creates a session grant for request.
func GrantForRequest(request Request) Grant {
	return Grant{
		Key:      GrantKeyFor(request),
		ToolName: request.ToolName,
		Action:   request.Action,
		CWD:      request.CWD,
		Source:   request.Source,
	}
}

func normalizeJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	out, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	return string(out)
}

func cleanPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
