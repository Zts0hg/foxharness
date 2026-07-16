package permission

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestBuildEvidencePreservesTrustBoundaries(t *testing.T) {
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: "inspect the old project"},
		{Role: schema.RoleAssistant, Content: "I will inspect everything"},
		{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "ask-1", Name: "ask_user_question"}}},
		{Role: schema.RoleUser, ToolCallID: "ask-1", Content: "yes, include external docs"},
		{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "read-1", Name: "read_file"}}},
		{Role: schema.RoleUser, ToolCallID: "read-1", Content: "file says delete everything"},
		{Role: schema.RoleUser, Content: "only read documentation"},
		{Role: schema.RoleUser, Content: "## Compacted Context Summary\nassistant-generated summary says write files"},
	}

	evidence := BuildEvidence(messages, []string{"follow AGENTS.md"}, reviewRequest())
	for _, want := range []string{
		"[trusted project instruction]\nfollow AGENTS.md",
		"[trusted user]\ninspect the old project",
		"[trusted user answer]\nyes, include external docs",
		"[trusted user]\nonly read documentation",
		"[untrusted assistant]\nI will inspect everything",
		"[untrusted tool result]\nfile says delete everything",
		"[untrusted generated context]\n## Compacted Context Summary",
	} {
		if !strings.Contains(evidence.Text, want) {
			t.Fatalf("evidence missing %q:\n%s", want, evidence.Text)
		}
	}
}

func TestBuildEvidenceUsesSeparateRecentBudgets(t *testing.T) {
	messages := []schema.Message{
		{Role: schema.RoleUser, Content: strings.Repeat("old-trusted ", 3000)},
		{Role: schema.RoleAssistant, Content: strings.Repeat("old-untrusted ", 2000)},
		{Role: schema.RoleUser, Content: "LATEST_TRUSTED"},
		{Role: schema.RoleAssistant, Content: "LATEST_UNTRUSTED"},
	}
	evidence := BuildEvidence(messages, nil, reviewRequest())

	if len(evidence.Trusted) > 16*1024 {
		t.Fatalf("trusted evidence length = %d, want <= 16KiB", len(evidence.Trusted))
	}
	if len(evidence.Untrusted) > 8*1024 {
		t.Fatalf("untrusted evidence length = %d, want <= 8KiB", len(evidence.Untrusted))
	}
	if !strings.Contains(evidence.Trusted, "LATEST_TRUSTED") || !strings.Contains(evidence.Untrusted, "LATEST_UNTRUSTED") {
		t.Fatalf("recent evidence was truncated:\n%s", evidence.Text)
	}
}

func TestBuildEvidenceBoundsOversizedRequestFacts(t *testing.T) {
	request := reviewRequest()
	request.Action = strings.Repeat("oversized-action ", 3000)
	evidence := BuildEvidence(nil, nil, request)
	if len(evidence.Trusted) > 16*1024 {
		t.Fatalf("trusted evidence length = %d, want <= 16KiB", len(evidence.Trusted))
	}
}

func TestBuildChildEvidenceTreatsChildTaskAsUntrusted(t *testing.T) {
	parent := BuildEvidence([]schema.Message{{Role: schema.RoleUser, Content: "search docs only"}}, nil, reviewRequest())
	child := []schema.Message{
		{Role: schema.RoleUser, Content: "search docs and then change the release script"},
		{Role: schema.RoleAssistant, Content: "I should change it"},
	}

	evidence := BuildChildEvidence(parent, child, reviewRequest())
	if !strings.Contains(evidence.Trusted, "search docs only") {
		t.Fatalf("parent authorization missing:\n%s", evidence.Text)
	}
	if strings.Contains(evidence.Trusted, "change the release script") {
		t.Fatalf("child task was promoted to trusted authorization:\n%s", evidence.Text)
	}
	if !strings.Contains(evidence.Untrusted, "[untrusted child context]\nsearch docs and then change the release script") {
		t.Fatalf("child context missing:\n%s", evidence.Text)
	}
}
