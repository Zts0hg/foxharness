package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestManagerBuildRegistryCombinesParentAndChildEvidence(t *testing.T) {
	workDir := t.TempDir()
	reviewer := &recordingPermissionReviewer{}
	coordinator := permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeApprove, false), Workspace: workDir, CWD: workDir, Reviewer: reviewer,
	})
	manager := NewManager(nil, workDir).WithPermission(coordinator).WithParentEvidence(func(request permission.Request) permission.Evidence {
		return permission.BuildEvidence([]schema.Message{{Role: schema.RoleUser, Content: "inspect docs only"}}, nil, request)
	})
	sess, err := session.NewManagerWithHome(workDir, t.TempDir()).Create(session.CreateOptions{Source: session.SOURCESubagent, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewMessageLog(sess).Append("run-1", schema.Message{Role: schema.RoleUser, Content: "inspect docs then change files"}); err != nil {
		t.Fatal(err)
	}
	registry := manager.buildRegistry(true, nil, sess)

	result := registry.Execute(context.Background(), schema.ToolCall{ID: "1", Name: "bash", Arguments: json.RawMessage(`{"command":"true"}`)})
	if result.IsError {
		t.Fatalf("Execute() error = %s", result.Output)
	}
	if !strings.Contains(reviewer.evidence.Trusted, "inspect docs only") {
		t.Fatalf("parent evidence missing: %q", reviewer.evidence.Text)
	}
	if strings.Contains(reviewer.evidence.Trusted, "change files") || !strings.Contains(reviewer.evidence.Untrusted, "change files") {
		t.Fatalf("child evidence trust is incorrect: %q", reviewer.evidence.Text)
	}
}

type recordingPermissionReviewer struct {
	evidence permission.Evidence
}

func (r *recordingPermissionReviewer) Review(_ context.Context, _ permission.Request, evidence permission.Evidence) (permission.ReviewResult, error) {
	r.evidence = evidence
	return permission.ReviewResult{
		Decision: permission.ReviewApprove, Risk: permission.RiskLow,
		UserAuthorization: permission.AuthorizationMedium, Rationale: "scoped test command",
	}, nil
}
