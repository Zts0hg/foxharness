package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
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

func TestManualCompactionContinuationReopenAndRepeatPreserveProjection(t *testing.T) {
	sess := copyContextLifecycleFixture(t)
	modelProvider := &manualContextProvider{summaries: []string{
		"<summary>manual summary one</summary>",
		"<summary>manual summary two</summary>",
	}}
	firstRunner := &AgentRunner{currentSession: sess, llmProvider: modelProvider, model: "fixture-model"}
	firstResult, err := firstRunner.CompactNow(context.Background(), "focus on runtime boundaries")
	if err != nil {
		t.Fatalf("first CompactNow() error = %v", err)
	}
	if firstResult.MessagesSummarized != 3 {
		t.Fatalf("first MessagesSummarized = %d, want summary plus two active messages", firstResult.MessagesSummarized)
	}
	state, records := loadContextStateAndRecords(t, sess)
	if state.Summary != "manual summary one" || state.CoveredUntilSeq != 6 {
		t.Fatalf("first compact state = %#v", state)
	}
	assertAppProjectedContents(t, projectedMessages(state, records), []string{"manual summary one"})

	log := session.NewMessageLog(sess)
	if seq, err := log.Append("run-continuation", schema.Message{Role: schema.RoleUser, Content: "continue after manual compact"}); err != nil || seq != 7 {
		t.Fatalf("Append(user) seq/error = %d, %v", seq, err)
	}
	if seq, err := log.Append("run-continuation", schema.Message{Role: schema.RoleAssistant, Content: "continued result"}); err != nil || seq != 8 {
		t.Fatalf("Append(assistant) seq/error = %d, %v", seq, err)
	}
	state, records = loadContextStateAndRecords(t, sess)
	assertAppProjectedContents(t, projectedMessages(state, records), []string{
		"manual summary one", "continue after manual compact", "continued result",
	})

	reopened := &AgentRunner{currentSession: sess, llmProvider: modelProvider, model: "fixture-model"}
	state, records = loadContextStateAndRecords(t, sess)
	assertAppProjectedContents(t, projectedMessages(state, records), []string{
		"manual summary one", "continue after manual compact", "continued result",
	})
	secondResult, err := reopened.CompactNow(context.Background(), "")
	if err != nil {
		t.Fatalf("second CompactNow() error = %v", err)
	}
	if secondResult.MessagesSummarized != 3 {
		t.Fatalf("second MessagesSummarized = %d, want prior summary plus continuation", secondResult.MessagesSummarized)
	}
	state, records = loadContextStateAndRecords(t, sess)
	if state.Summary != "manual summary two" || state.CoveredUntilSeq != 8 {
		t.Fatalf("second compact state = %#v", state)
	}
	assertAppProjectedContents(t, projectedMessages(state, records), []string{"manual summary two"})
	if modelProvider.calls != 2 || len(modelProvider.prompts) != 2 {
		t.Fatalf("summary provider calls/prompts = %d/%d, want 2/2", modelProvider.calls, len(modelProvider.prompts))
	}
	if !strings.Contains(modelProvider.prompts[0], "focus on runtime boundaries") || !strings.Contains(modelProvider.prompts[0], "Continue with the migration plan") {
		t.Fatalf("first manual compact prompt missing instructions or resumed context:\n%s", modelProvider.prompts[0])
	}
	if !strings.Contains(modelProvider.prompts[1], "manual summary one") || !strings.Contains(modelProvider.prompts[1], "continue after manual compact") || !strings.Contains(modelProvider.prompts[1], "continued result") {
		t.Fatalf("second manual compact prompt lost projection content:\n%s", modelProvider.prompts[1])
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

func loadContextStateAndRecords(t *testing.T, sess *session.Session) (*session.CompactState, []session.MessageRecord) {
	t.Helper()
	state, err := session.LoadCompactState(sess)
	if err != nil {
		t.Fatalf("LoadCompactState() error = %v", err)
	}
	records, err := session.NewMessageLog(sess).LoadRecords()
	if err != nil {
		t.Fatalf("LoadRecords() error = %v", err)
	}
	return state, records
}

func assertAppProjectedContents(t *testing.T, messages []schema.Message, want []string) {
	t.Helper()
	if len(messages) != len(want) {
		t.Fatalf("projected messages = %#v, want %d entries", messages, len(want))
	}
	for index := range want {
		if messages[index].Content != want[index] {
			t.Fatalf("projected[%d] = %q, want %q", index, messages[index].Content, want[index])
		}
	}
}

type manualContextProvider struct {
	summaries []string
	calls     int
	prompts   []string
}

func (p *manualContextProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	if p.calls >= len(p.summaries) {
		return nil, os.ErrInvalid
	}
	if len(messages) != 1 {
		return nil, os.ErrInvalid
	}
	p.prompts = append(p.prompts, messages[0].Content)
	content := p.summaries[p.calls]
	p.calls++
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: content}}, nil
}
