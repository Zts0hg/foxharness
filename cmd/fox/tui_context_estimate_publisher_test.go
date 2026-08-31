package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/engine"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

/* stubTUIContextCompactor answers one fixed compaction proposal. */
type stubTUIContextCompactor struct {
	proposal foxruntime.ContextCompactionProposal
}

func (s *stubTUIContextCompactor) Compact(context.Context, foxruntime.ContextCompactionRequest) (foxruntime.ContextCompactionProposal, error) {
	return s.proposal, nil
}

func (s *stubTUIContextCompactor) CheckContext(context.Context, foxruntime.ContextBudgetRequest) error {
	return nil
}

/* TestContextEstimatePublisherPublishesPostCompactionProjection pins the
 * baseline estimate publication: the baseline published the context estimate
 * from the compacted history, so a committed run-local compaction
 * republishes from the proposal's projection instead of leaving the sidebar
 * at the pre-compaction usage. */
func TestContextEstimatePublisherPublishesPostCompactionProjection(t *testing.T) {
	mechanism, err := compaction.NewCompactor(&targetTUIProvider{}, compaction.DefaultCompactionConfig())
	if err != nil {
		t.Fatal(err)
	}
	large := []engine.Message{{Role: engine.RoleUser, Content: strings.Repeat("usage probe ", 4000)}}
	compact := []engine.Message{{Role: engine.RoleUser, Content: "summary"}}
	published := make([][2]int, 0, 2)
	publisher := &contextEstimatePublisher{
		inner:     &stubTUIContextCompactor{proposal: foxruntime.ContextCompactionProposal{Changed: true, Messages: compact}},
		mechanism: mechanism,
		publish:   func(used int, window int) { published = append(published, [2]int{used, window}) },
	}
	if _, err := publisher.Compact(context.Background(), foxruntime.ContextCompactionRequest{Messages: large}); err != nil {
		t.Fatal(err)
	}
	if len(published) == 0 {
		t.Fatal("publisher published no estimate")
	}
	last := published[len(published)-1]
	if want := mechanism.Estimate(compact); last[0] != want {
		t.Fatalf("published estimate = %d, want the post-compaction %d", last[0], want)
	}
}
