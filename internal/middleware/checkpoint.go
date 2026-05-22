package middleware

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type checkpointMiddleware struct {
	checkpointer checkpoint.Checkpointer
	getMessageID func() string
	workDir      string
}

// NewCheckpointMiddleware creates middleware that backs up files before write
// and edit tools execute.
func NewCheckpointMiddleware(cp checkpoint.Checkpointer, getMessageID func() string, workDir ...string) Middleware {
	dir := ""
	if len(workDir) > 0 {
		dir = workDir[0]
	}
	return &checkpointMiddleware{
		checkpointer: cp,
		getMessageID: getMessageID,
		workDir:      dir,
	}
}

func (m *checkpointMiddleware) BeforeExecute(ctx context.Context, call schema.ToolCall) (Decision, error) {
	_ = ctx
	if m.checkpointer == nil || (call.Name != "write_file" && call.Name != "edit_file") {
		return Allow(), nil
	}

	filePath := m.filePath(call)
	if filePath == "" {
		return Allow(), nil
	}
	messageID := ""
	if m.getMessageID != nil {
		messageID = m.getMessageID()
	}
	if err := m.checkpointer.TrackEdit(filePath, messageID); err != nil {
		log.Printf("[Checkpoint] failed to track %s before %s: %v", filePath, call.Name, err)
	}
	return Allow(), nil
}

func (m *checkpointMiddleware) filePath(call schema.ToolCall) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return ""
	}
	path := strings.TrimSpace(args.Path)
	if path == "" {
		return ""
	}
	if m.workDir != "" && !filepath.IsAbs(path) {
		return filepath.Join(m.workDir, path)
	}
	return path
}
