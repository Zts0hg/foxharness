package keeprun

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestBackoffPolicyWait(t *testing.T) {
	b := BackoffPolicy{Base: time.Second, Max: 8 * time.Second}
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 8 * time.Second},  // capped at Max
		{60, 8 * time.Second}, // capped, no overflow at large attempt
	}
	for _, tt := range tests {
		if got := b.wait(tt.attempt); got != tt.want {
			t.Errorf("wait(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestBackoffPolicyZeroDefaults(t *testing.T) {
	var b BackoffPolicy
	if d := b.wait(0); d <= 0 {
		t.Errorf("wait(0) with zero policy = %v, want > 0", d)
	}
	if d := b.wait(100); d <= 0 {
		t.Errorf("wait(100) with zero policy = %v, want > 0 (capped, no overflow)", d)
	}
}

// TestOrchestratorRetriesWithoutCap proves a phase that fails many times is
// retried until it succeeds (no retry cap, FR-007), with the wait rate-limited:
// it starts at Base and is capped at Max.
func TestOrchestratorRetriesWithoutCap(t *testing.T) {
	const failures = 30
	repo := setupRepo(t, oneTask(), localConfig)

	var waits []time.Duration
	sleeper := func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	runner := &fakeRun{}
	verify := &fakeVerify{fn: func(p Phase, n int) error {
		if p.Command == "codexspec:specify" && n < failures {
			return fmt.Errorf("transient failure %d", n)
		}
		return nil
	}}
	o := NewOrchestrator(repo, runner,
		WithWorktrees(&fakeWT{repoDir: repo}), WithVerifier(verify),
		WithBackoff(BackoffPolicy{Base: time.Millisecond, Max: 100 * time.Millisecond}),
		WithSleeper(sleeper))
	if err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	runs := 0
	for _, c := range runner.calls {
		if c.command == "codexspec:specify" {
			runs++
		}
	}
	if runs != failures+1 {
		t.Errorf("specify runs = %d, want %d (no retry cap)", runs, failures+1)
	}
	if len(waits) < failures {
		t.Fatalf("recorded %d backoff waits, want >= %d", len(waits), failures)
	}
	if waits[0] != time.Millisecond {
		t.Errorf("first wait = %v, want 1ms (Base)", waits[0])
	}
	if last := waits[len(waits)-1]; last != 100*time.Millisecond {
		t.Errorf("late wait = %v, want 100ms (Max cap)", last)
	}
}
