package processtree

import (
	"errors"
	"testing"
	"time"
)

func TestWaitForTreeExitPollsUntilNoActiveProcessesRemain(t *testing.T) {
	queries := 0
	err := waitForTreeExit(100*time.Millisecond, func() (bool, error) {
		queries++
		return queries >= 3, nil
	})
	if err != nil {
		t.Fatalf("waitForTreeExit() error = %v", err)
	}
	if queries != 3 {
		t.Fatalf("queries = %d, want 3", queries)
	}
}

func TestWaitForTreeExitPropagatesQueryFailure(t *testing.T) {
	queryErr := errors.New("query active processes")
	err := waitForTreeExit(time.Second, func() (bool, error) { return false, queryErr })
	if !errors.Is(err, queryErr) {
		t.Fatalf("waitForTreeExit() error = %v, want %v", err, queryErr)
	}
}
