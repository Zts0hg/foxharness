package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestRuntimeHarnessCreatesAndOpensProfileBoundSessions(t *testing.T) {
	store := newLifecycleStore()
	harness, err := NewRuntimeHarness(store)
	if err != nil {
		t.Fatal(err)
	}
	created, err := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if got := created.Snapshot(); got.ID != "session-1" || got.Profile != TUIInteractive || got.Source != session.SOURCECLI || got.WorkDir != "/workspace" {
		t.Fatalf("created session = %#v", got)
	}
	if _, err := harness.OpenSession(context.Background(), TUIInteractive, created.Snapshot().ID); !errors.Is(err, ErrSessionAlreadyOpen) {
		t.Fatalf("duplicate open = %v, want ErrSessionAlreadyOpen", err)
	}
	if _, err := harness.OpenSession(context.Background(), ChildRun, created.Snapshot().ID); err == nil {
		t.Fatal("child profile opened a CLI-source session")
	}
	if err := created.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	opened, err := harness.OpenSession(context.Background(), TUIInteractive, created.Snapshot().ID)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Snapshot() != created.Snapshot() {
		t.Fatalf("opened session = %#v, want %#v", opened.Snapshot(), created.Snapshot())
	}
	if err := opened.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntimeHarness(nil); err == nil {
		t.Fatal("nil session store accepted")
	}
}

func TestRuntimeHarnessRejectsInvalidStoresAndPreservesSessionMetadata(t *testing.T) {
	var typedNil *lifecycleStore
	if _, err := NewRuntimeHarness(typedNil); err == nil {
		t.Fatal("typed nil session store accepted")
	}

	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	options := SessionOptions{
		WorkDir: "/workspace", UserID: "user", ChatID: "chat",
		ParentSessionID: "parent-session", ParentRunID: "parent-run",
		DelegationID: "delegation", Agent: "reviewer",
	}
	agentSession, err := harness.CreateSession(context.Background(), ChildRun, options)
	if err != nil {
		t.Fatal(err)
	}
	gotOptions := store.createOptions()
	if gotOptions.Source != session.SOURCESubagent || gotOptions.WorkDir != options.WorkDir ||
		gotOptions.UserID != options.UserID || gotOptions.ChatID != options.ChatID ||
		gotOptions.ParentSessionID != options.ParentSessionID || gotOptions.ParentRunID != options.ParentRunID ||
		gotOptions.DelegationID != options.DelegationID || gotOptions.Agent != options.Agent {
		t.Fatalf("create options = %#v", gotOptions)
	}
	if got := agentSession.Snapshot(); got.ParentSessionID != options.ParentSessionID || got.ParentRunID != options.ParentRunID {
		t.Fatalf("session snapshot = %#v", got)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	createdBefore := store.sessionCount()
	if _, err := harness.CreateSession(cancelled, CLIExec, SessionOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled create = %v", err)
	}
	if got := store.sessionCount(); got != createdBefore {
		t.Fatalf("cancelled create persisted %d sessions, want %d", got, createdBefore)
	}
}

func TestRuntimeHarnessRejectsMismatchedOpenedSessionIdentity(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	created, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Snapshot().ID
	if err := created.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.returnOpenID("another-session")
	if _, err := harness.OpenSession(context.Background(), CLIExec, id); err == nil {
		t.Fatal("store response with mismatched session identity accepted")
	}
}

func TestAgentSessionSerializesRunsWithContextAwareAdmission(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "first", Model: "model-a"})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := agentSession.BeginRun(cancelled, RunSpec{Prompt: "blocked"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked run error = %v, want context cancellation", err)
	}
	if got := store.startCount(); got != 1 {
		t.Fatalf("start calls while blocked = %d, want 1", got)
	}
	if err := agentSession.FinishRun(first); err != nil {
		t.Fatal(err)
	}
	if err := agentSession.FinishRun(first); err != nil {
		t.Fatalf("idempotent finish = %v", err)
	}
	if got := store.finishCount(); got != 1 {
		t.Fatalf("finish calls = %d, want 1", got)
	}

	second, err := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Snapshot().RunID == first.Snapshot().RunID {
		t.Fatalf("run identity leaked: %#v / %#v", first.Snapshot(), second.Snapshot())
	}
	if err := agentSession.FinishRun(second); err != nil {
		t.Fatal(err)
	}
	if err := agentSession.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := agentSession.Close(context.Background()); err != nil {
		t.Fatalf("idempotent close = %v", err)
	}
	if _, err := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "after close"}); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("run after close = %v, want ErrSessionClosed", err)
	}
}

func TestAgentSessionRejectsForeignAndMismatchedRunScopes(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	first, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/one"})
	second, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/two"})
	scope, err := first.BeginRun(context.Background(), RunSpec{Prompt: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.FinishRun(scope); !errors.Is(err, ErrRunScopeOwner) {
		t.Fatalf("foreign finish = %v, want ErrRunScopeOwner", err)
	}
	if err := first.FinishRun(scope); err != nil {
		t.Fatal(err)
	}

	store.returnRunFor("wrong-session")
	if _, err := first.BeginRun(context.Background(), RunSpec{Prompt: "mismatched"}); err == nil {
		t.Fatal("mismatched stored run accepted")
	}
	next, err := first.BeginRun(context.Background(), RunSpec{Prompt: "after mismatch"})
	if err != nil {
		t.Fatalf("mismatched run leaked admission: %v", err)
	}
	_ = first.FinishRun(next)
}

func TestDistinctAgentSessionsHaveIndependentRunAdmission(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	first, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/one"})
	second, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/two"})
	firstScope, err := first.BeginRun(context.Background(), RunSpec{Prompt: "first"})
	if err != nil {
		t.Fatal(err)
	}
	secondScope, err := second.BeginRun(context.Background(), RunSpec{Prompt: "second"})
	if err != nil {
		t.Fatalf("distinct session blocked: %v", err)
	}
	if firstScope.Snapshot().SessionID == secondScope.Snapshot().SessionID {
		t.Fatal("distinct sessions share identity")
	}
	if err := first.FinishRun(firstScope); err != nil {
		t.Fatal(err)
	}
	if err := second.FinishRun(secondScope); err != nil {
		t.Fatal(err)
	}
}

func TestRunScopeFreezesRunOwnedValues(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), ChildRun, SessionOptions{WorkDir: "/child"})
	turns := 9
	allowed := []string{"read_file", "bash"}
	observer := lifecycleObserver{}
	scope, err := agentSession.BeginRun(context.Background(), RunSpec{
		Prompt: "inspect", Model: "child-model", Effort: "high", MaxTurns: &turns,
		AllowedTools: allowed, DelegationDepth: intPointer(1), Observer: observer,
	})
	if err != nil {
		t.Fatal(err)
	}
	turns = 99
	allowed[0] = "write_file"
	got := scope.Snapshot()
	if got.Profile != ChildRun || got.SessionID != agentSession.Snapshot().ID || got.Model != "child-model" || got.Effort != "high" || got.MaxTurns != 9 || got.PermissionPolicy != "parent_ceiling" {
		t.Fatalf("run scope snapshot = %#v", got)
	}
	tools := scope.AllowedTools()
	tools[0] = "write_file"
	if want := []string{"bash", "read_file"}; !reflect.DeepEqual(scope.AllowedTools(), want) {
		t.Fatalf("scope tools = %v, want %v", scope.AllowedTools(), want)
	}
	if scope.Observer() != observer {
		t.Fatal("run observer was not frozen")
	}
	scope.Cancel()
	if !errors.Is(scope.Context().Err(), context.Canceled) {
		t.Fatalf("scope context = %v", scope.Context().Err())
	}
	if err := agentSession.FinishRun(scope); err != nil {
		t.Fatal(err)
	}
}

func TestRunStorageFailuresAreRetriableAndBlockSessionProgress(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	store.failNextStart(errors.New("start failed"))
	if _, err := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "first"}); err == nil {
		t.Fatal("start failure was lost")
	}
	scope, err := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "second"})
	if err != nil {
		t.Fatalf("start failure leaked admission: %v", err)
	}
	store.failNextFinish(errors.New("finish failed"))
	if err := agentSession.FinishRun(scope); err == nil {
		t.Fatal("finish failure was lost")
	}
	if _, err := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "blocked by recovery"}); !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("run after failed finish = %v, want ErrSessionRecoveryRequired", err)
	}
	if err := agentSession.Close(context.Background()); !errors.Is(err, ErrSessionRecoveryRequired) {
		t.Fatalf("close after failed finish = %v, want ErrSessionRecoveryRequired", err)
	}
	if err := agentSession.FinishRun(scope); err != nil {
		t.Fatalf("retry finish = %v", err)
	}
	next, err := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "third"})
	if err != nil {
		t.Fatalf("finish failure leaked admission: %v", err)
	}
	_ = agentSession.FinishRun(next)
}

func TestFileStoreSatisfiesRuntimeSessionStoreAndPersistsRunMetadata(t *testing.T) {
	workDir := t.TempDir()
	fileStore := session.NewFileStoreWithHome(workDir, t.TempDir())
	var _ SessionStore = fileStore
	harness, err := NewRuntimeHarness(fileStore)
	if err != nil {
		t.Fatal(err)
	}
	agentSession, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "persist me"})
	if err != nil {
		t.Fatal(err)
	}
	if err := agentSession.FinishRun(scope); err != nil {
		t.Fatal(err)
	}
	runPath := filepath.Join(scope.Snapshot().RootDir, "run.json")
	contents, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatal(err)
	}
	var storedRun session.StoredRun
	if err := json.Unmarshal(contents, &storedRun); err != nil {
		t.Fatal(err)
	}
	if storedRun.SessionID != agentSession.Snapshot().ID || storedRun.Prompt != "persist me" || storedRun.EndedAt == nil {
		t.Fatalf("stored run = %#v", storedRun)
	}
}

type lifecycleObserver struct{}

func (lifecycleObserver) ObserveRunFact(context.Context, RuntimeFact) {}

type lifecycleStore struct {
	mu          sync.Mutex
	sessions    map[session.ID]*session.StoredSession
	starts      int
	finishes    int
	startErr    error
	finishErr   error
	lastCreate  session.CreateOptions
	runSession  session.ID
	openID      session.ID
	messages    map[session.ID][]session.MessageRecord
	compact     map[session.ID]*session.CompactState
	messageErr  error
	loadErr     error
	messageTime time.Time
}

func newLifecycleStore() *lifecycleStore {
	return &lifecycleStore{
		sessions: map[session.ID]*session.StoredSession{},
		messages: map[session.ID][]session.MessageRecord{}, compact: map[session.ID]*session.CompactState{},
	}
}

func (s *lifecycleStore) Create(options session.CreateOptions) (*session.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := session.ID(fmt.Sprintf("session-%d", len(s.sessions)+1))
	s.lastCreate = options
	stored := &session.StoredSession{
		ID: id, Source: options.Source, WorkDir: options.WorkDir, RootDir: "/sessions/" + string(id),
		UserID: options.UserID, ChatID: options.ChatID, ParentSessionID: options.ParentSessionID,
		ParentRunID: options.ParentRunID, DelegationID: options.DelegationID, Agent: options.Agent,
	}
	s.sessions[id] = stored
	copy := *stored
	return &copy, nil
}

func (s *lifecycleStore) Open(id session.ID) (*session.StoredSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.sessions[id]
	if !ok {
		return nil, session.ErrNotFound
	}
	copy := *stored
	if s.openID != "" {
		copy.ID = s.openID
		s.openID = ""
	}
	return &copy, nil
}

func (s *lifecycleStore) StartRun(storedSession *session.StoredSession, prompt string) (*session.StoredRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startErr != nil {
		err := s.startErr
		s.startErr = nil
		return nil, err
	}
	s.starts++
	id := session.RunID(fmt.Sprintf("run-%d", s.starts))
	runSession := storedSession.ID
	if s.runSession != "" {
		runSession = s.runSession
		s.runSession = ""
	}
	return &session.StoredRun{ID: id, SessionID: runSession, RootDir: "/runs/" + string(id), Prompt: prompt}, nil
}

func (s *lifecycleStore) FinishRun(*session.StoredRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishes++
	if s.finishErr != nil {
		err := s.finishErr
		s.finishErr = nil
		return err
	}
	return nil
}

func (s *lifecycleStore) LoadMessageRecords(storedSession *session.StoredSession) ([]session.MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		err := s.loadErr
		s.loadErr = nil
		return nil, err
	}
	return cloneMessageRecords(s.messages[storedSession.ID]), nil
}

func (s *lifecycleStore) AppendMessage(storedSession *session.StoredSession, runID session.RunID, message engine.Message, display string) (session.MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messageErr != nil {
		err := s.messageErr
		s.messageErr = nil
		return session.MessageRecord{}, err
	}
	records := s.messages[storedSession.ID]
	seq := int64(0)
	for _, record := range records {
		if record.Seq >= seq {
			seq = record.Seq + 1
		}
	}
	persisted := session.MessageRecord{
		Seq: seq, RunID: runID, Kind: session.MessageKindNormal,
		Time: s.messageTime, Message: cloneContextMessage(message), DisplayContent: display,
	}
	records = append(records, persisted)
	s.messages[storedSession.ID] = records
	return persisted, nil
}

func (s *lifecycleStore) LoadContextCompactState(storedSession *session.StoredSession) (*session.CompactState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneCompactState(s.compact[storedSession.ID]), nil
}

func (s *lifecycleStore) SaveContextCompactState(storedSession *session.StoredSession, state *session.CompactState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compact[storedSession.ID] = cloneCompactState(state)
	return nil
}

func (s *lifecycleStore) TruncateMessagesBefore(storedSession *session.StoredSession, seq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := s.messages[storedSession.ID]
	kept := make([]session.MessageRecord, 0, len(records))
	for _, record := range records {
		if record.Seq < seq {
			kept = append(kept, record)
		}
	}
	s.messages[storedSession.ID] = kept
	return nil
}

func (s *lifecycleStore) startCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts
}

func (s *lifecycleStore) finishCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finishes
}

func (s *lifecycleStore) failNextStart(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startErr = err
}

func (s *lifecycleStore) failNextFinish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishErr = err
}

func (s *lifecycleStore) createOptions() session.CreateOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCreate
}

func (s *lifecycleStore) sessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *lifecycleStore) returnRunFor(id session.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runSession = id
}

func (s *lifecycleStore) returnOpenID(id session.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.openID = id
}

func (s *lifecycleStore) messageRecords(id session.ID) []session.MessageRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneMessageRecords(s.messages[id])
}

func (s *lifecycleStore) seedMessages(id session.ID, records []session.MessageRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[id] = cloneMessageRecords(records)
}

func (s *lifecycleStore) seedCompactState(id session.ID, state *session.CompactState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compact[id] = cloneCompactState(state)
}

func (s *lifecycleStore) compactState(id session.ID) *session.CompactState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneCompactState(s.compact[id])
}

func (s *lifecycleStore) failNextMessage(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageErr = err
}

func (s *lifecycleStore) failNextLoad(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr = err
}

func intPointer(value int) *int { return &value }
