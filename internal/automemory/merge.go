package automemory

import (
	"fmt"
	"strings"
)

// Labels prefixing each tier in the merged injected index.
const (
	userTierLabel    = "User-global memories:"
	projectTierLabel = "Project memories:"
)

// MergedIndexString composes the two-tier index injected into the system prompt
// each turn (REQ-006): the user-global index followed by the project index, each
// under a short label. Empty tiers are omitted; when no memories exist anywhere
// the result is the empty string. Filesystem errors degrade to an empty tier
// rather than failing prompt composition.
func (s *Store) MergedIndexString() string {
	userIndex, _ := s.BuildIndex(ScopeUserGlobal)
	projectIndex, _ := s.BuildIndex(ScopeProject)

	var blocks []string
	if strings.TrimSpace(userIndex) != "" {
		blocks = append(blocks, userTierLabel+"\n"+userIndex)
	}
	if strings.TrimSpace(projectIndex) != "" {
		blocks = append(blocks, projectTierLabel+"\n"+projectIndex)
	}
	return strings.Join(blocks, "\n\n")
}

// Manifest lists every existing memory across both scopes as
// "- [type] file.md: description" lines. It is pre-injected into the extraction
// pass so the LLM updates an existing memory instead of creating a duplicate
// (REQ-012). Empty when no memories exist.
func (s *Store) Manifest() string {
	var lines []string
	for _, scope := range []Scope{ScopeUserGlobal, ScopeProject} {
		memories, _ := s.Load(scope)
		for _, m := range memories {
			desc := strings.Join(strings.Fields(m.Description), " ")
			lines = append(lines, fmt.Sprintf("- [%s] %s.md: %s", m.Type, m.Name, desc))
		}
	}
	return strings.Join(lines, "\n")
}
