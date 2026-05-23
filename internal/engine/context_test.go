package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

type summaryProvider struct {
	calls int
}

func (p *summaryProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "short persisted summary"},
	}, nil
}

func TestBuildInitialContextPersistsCompactState(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var records []session.MessageRecord
	for i := int64(0); i < 8; i++ {
		records = append(records, session.MessageRecord{
			Seq: i,
			Message: schema.Message{
				Role:    schema.RoleUser,
				Content: strings.Repeat("x", 40),
			},
		})
	}

	p := &summaryProvider{}
	cfg := compaction.DefaultCompactionConfig()
	cfg.Model = "test-model"
	cfg.RecentKeep = 2
	cfg.SummaryMaxTokens = 128
	cfg.ContextWindow = 500
	cfg.Estimator = compaction.RoughEstimator{}
	cfg.AutoCompactThreshold = 250
	compactor, err := compaction.NewCompactor(p, cfg)
	if err != nil {
		t.Fatalf("NewCompactor() error = %v", err)
	}
	eng := &AgentEngine{compactor: compactor}

	current := schema.Message{Role: schema.RoleUser, Content: "current"}
	projected, compacted, err := eng.buildInitialContext(context.Background(), sess, "system", records, current)
	if err != nil {
		t.Fatalf("buildInitialContext() error = %v", err)
	}
	if !compacted {
		t.Fatalf("buildInitialContext() compacted = false, want true")
	}
	if p.calls != 1 {
		t.Fatalf("summary provider calls = %d, want 1", p.calls)
	}
	if len(projected) != 5 {
		t.Fatalf("projected len = %d, want system + summary + 2 recent + current", len(projected))
	}
	if !strings.Contains(projected[1].Content, "short persisted summary") {
		t.Fatalf("projected summary missing persisted text: %q", projected[1].Content)
	}

	state, err := session.LoadCompactState(sess)
	if err != nil {
		t.Fatalf("LoadCompactState() error = %v", err)
	}
	if state.CoveredUntilSeq != 5 {
		t.Fatalf("CoveredUntilSeq = %d, want 5", state.CoveredUntilSeq)
	}

	projectedAgain, compactedAgain, err := eng.buildInitialContext(context.Background(), sess, "system", records, current)
	if err != nil {
		t.Fatalf("second buildInitialContext() error = %v", err)
	}
	if compactedAgain {
		t.Fatalf("second buildInitialContext() compacted = true, want false")
	}
	if p.calls != 1 {
		t.Fatalf("summary provider calls after second projection = %d, want 1", p.calls)
	}
	if len(projectedAgain) != len(projected) {
		t.Fatalf("second projected len = %d, want %d", len(projectedAgain), len(projected))
	}
}
