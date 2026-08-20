package agentops

import "github.com/Zts0hg/foxharness/internal/agentops/logsearch"

/* LogSearchTool retains the AgentOps-owned public capability name. */
type LogSearchTool = logsearch.Tool

const maxLogSearchLineBytes = logsearch.MaxLineBytes

/* NewLogSearchTool creates the AgentOps-owned read-only log-search capability. */
func NewLogSearchTool(logDir string) *LogSearchTool {
	return logsearch.New(logDir)
}
