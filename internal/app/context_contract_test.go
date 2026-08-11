package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/testsupport/entryfixture"
)

func TestRewindNeverReintroducesFutureCompactedContent(t *testing.T) {
	testCases := []struct {
		name             string
		seq              int64
		wantRecordCount  int
		wantSummary      bool
		forbiddenContent []string
	}{
		{
			name: "before compact coverage", seq: 0, wantRecordCount: 0,
			forbiddenContent: []string{"runtime boundary", "migration plan", "characterization coverage"},
		},
		{
			name: "within compact coverage", seq: 3, wantRecordCount: 3,
			forbiddenContent: []string{"runtime boundary is documented", "migration plan", "characterization coverage"},
		},
		{
			name: "after compact coverage", seq: 6, wantRecordCount: 6, wantSummary: true,
			forbiddenContent: []string{"characterization coverage"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			sess := copyContextLifecycleFixture(t)
			runner := &AgentRunner{currentSession: sess}

			if err := runner.TruncateMessageHistory(testCase.seq); err != nil {
				t.Fatalf("TruncateMessageHistory(%d) error = %v", testCase.seq, err)
			}
			records, err := session.NewMessageLog(sess).LoadRecords()
			if err != nil {
				t.Fatalf("LoadRecords() error = %v", err)
			}
			if len(records) != testCase.wantRecordCount {
				t.Fatalf("records after rewind = %d, want %d", len(records), testCase.wantRecordCount)
			}
			state, err := session.LoadCompactState(sess)
			if err != nil {
				t.Fatalf("LoadCompactState() error = %v", err)
			}
			if got := state.Summary != ""; got != testCase.wantSummary {
				t.Fatalf("compact summary retained = %v, want %v; state = %#v", got, testCase.wantSummary, state)
			}
			projected := projectedMessages(state, records)
			var contents []string
			for _, message := range projected {
				contents = append(contents, message.Content)
			}
			joined := strings.Join(contents, "\n")
			for _, forbidden := range testCase.forbiddenContent {
				if strings.Contains(joined, forbidden) {
					t.Fatalf("model-visible projection reintroduced future content %q:\n%s", forbidden, joined)
				}
			}
		})
	}
}

func copyContextLifecycleFixture(t *testing.T) *session.Session {
	t.Helper()
	fixtureRoot := filepath.Join("..", "..", "testdata", "characterization", "v1")
	manifest, err := entryfixture.Load(fixtureRoot)
	if err != nil {
		t.Fatalf("Load fixture manifest error = %v", err)
	}
	destination := t.TempDir()
	for _, fixture := range manifest.Fixtures {
		if !strings.HasPrefix(fixture.Path, "sessions/context-lifecycle/") {
			continue
		}
		if _, err := entryfixture.CopyFixture(fixtureRoot, fixture.Path, destination); err != nil {
			t.Fatalf("CopyFixture(%q) error = %v", fixture.Path, err)
		}
	}
	sessionRoot := filepath.Join(destination, "sessions", "context-lifecycle")
	data, err := os.ReadFile(filepath.Join(sessionRoot, "session.json"))
	if err != nil {
		t.Fatalf("ReadFile(session.json) error = %v", err)
	}
	var sess session.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		t.Fatalf("Unmarshal(session.json) error = %v", err)
	}
	sess.RootDir = sessionRoot
	return &sess
}
