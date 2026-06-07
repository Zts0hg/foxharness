package tui

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestRunKeepRunHeadlessRejectsNonRestricted(t *testing.T) {
	err := RunKeepRunHeadless(context.Background(), &fakeRunner{}, t.TempDir(), nil, nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "RunRestricted") {
		t.Fatalf("want RunRestricted guard error, got %v", err)
	}
}
