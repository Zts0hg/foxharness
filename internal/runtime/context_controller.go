package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* ContextBlockedError reports that a prepared request cannot fit the configured context budget. */
type ContextBlockedError struct {
	UsedTokens int
	Limit      int
}

/* Error describes the context budget that prevented model invocation. */
func (e *ContextBlockedError) Error() string {
	return fmt.Sprintf("runtime context token count (%d) exceeds blocking limit (%d)", e.UsedTokens, e.Limit)
}

/* ContextCollectionRequest contains frozen run values used to resolve prompt fragments. */
type ContextCollectionRequest struct {
	Profile           ProfileName
	Prompt            string
	WorkDir           string
	CollaborationMode string
	AllowedTools      []string
	ReadOnly          bool
}

/* ContextCollector resolves complete prompt fragments without owning their rendering. */
type ContextCollector interface {
	Collect(context.Context, ContextCollectionRequest) ([]prompt.Fragment, error)
}

/* ContextCompactionTrigger identifies one runtime-owned compaction decision point. */
type ContextCompactionTrigger string

const (
	/* ContextCompactionInitialHistory identifies first projection compaction for one run. */
	ContextCompactionInitialHistory ContextCompactionTrigger = "session_history"
	/* ContextCompactionPreTurn identifies ordinary automatic compaction before a later turn. */
	ContextCompactionPreTurn ContextCompactionTrigger = "turn_context"
	/* ContextCompactionReactive identifies prompt-too-long retry compaction. */
	ContextCompactionReactive ContextCompactionTrigger = "reactive"
	/* ContextCompactionManual identifies an explicit session compaction operation. */
	ContextCompactionManual ContextCompactionTrigger = "manual"
)

/* ContextCompactionRequest is an immutable input to a compaction mechanism. */
type ContextCompactionRequest struct {
	Trigger         ContextCompactionTrigger
	Messages        []engine.Message
	ToolDefinitions []engine.ToolDefinition
	Records         []session.MessageRecord
	CompactState    *session.CompactState
	TranscriptPath  string
	Instructions    string
}

/* ContextCompactionProposal describes either a durable state change or a run-local projection change. */
type ContextCompactionProposal struct {
	Changed      bool
	Messages     []engine.Message
	CompactState *session.CompactState
}

/* ContextCompactor proposes context changes and checks budgets without committing recoverable state. */
type ContextCompactor interface {
	Compact(context.Context, ContextCompactionRequest) (ContextCompactionProposal, error)
	CheckContext(context.Context, ContextBudgetRequest) error
}

/* ContextBudgetRequest contains the exact model-visible values used for one blocking decision. */
type ContextBudgetRequest struct {
	Messages        []engine.Message
	ToolDefinitions []engine.ToolDefinition
}

/* ContextController owns one run's collection, transient projection, and compaction decisions. */
type ContextController struct {
	mu        sync.Mutex
	session   *AgentSession
	scope     *RunScope
	collector ContextCollector
	compactor ContextCompactor

	collected    bool
	systemPrompt string
	initialized  bool
	messages     []engine.Message
}

type contextTurnKey struct {
	runID session.RunID
	turn  int
}

/* NewContextController binds one run scope to context collection and compaction ports. */
func (s *AgentSession) NewContextController(scope *RunScope, collector ContextCollector, compactor ContextCompactor) (*ContextController, error) {
	if scope == nil || scope.owner != s {
		return nil, ErrRunScopeOwner
	}
	if collector == nil {
		return nil, errors.New("runtime context collector is required")
	}
	return &ContextController{session: s, scope: scope, collector: collector, compactor: compactor}, nil
}

/* CompactContext performs one serialized explicit compaction and commits its durable proposal. */
func (s *AgentSession) CompactContext(ctx context.Context, compactor ContextCompactor, instructions string) (ContextCompactionProposal, error) {
	if err := contextError(ctx); err != nil {
		return ContextCompactionProposal{}, err
	}
	if s.profile.Snapshot().CompactionPolicy != "automatic_and_manual" {
		return ContextCompactionProposal{}, errors.New("runtime profile does not permit manual compaction")
	}
	if compactor == nil {
		return ContextCompactionProposal{}, errors.New("runtime context compactor is required")
	}
	if err := s.acquire(ctx); err != nil {
		return ContextCompactionProposal{}, err
	}
	defer s.release()

	records, state, err := s.contextSnapshot()
	if err != nil {
		return ContextCompactionProposal{}, err
	}
	projected := projectStoredContext("", s.record.TranscriptPath(), state, records)[1:]
	proposal, err := compactor.Compact(ctx, ContextCompactionRequest{
		Trigger: ContextCompactionManual, Messages: cloneContextMessages(projected),
		Records: cloneMessageRecords(records), CompactState: cloneCompactState(state),
		TranscriptPath: s.record.TranscriptPath(), Instructions: instructions,
	})
	if err != nil {
		return ContextCompactionProposal{}, err
	}
	if err := validateCompactionProposal(ContextCompactionManual, proposal, records); err != nil {
		return ContextCompactionProposal{}, err
	}
	if proposal.Changed {
		if err := s.commitCompactState(proposal.CompactState); err != nil {
			return ContextCompactionProposal{}, err
		}
	}
	return cloneCompactionProposal(proposal), nil
}

/* RewindContext invalidates covered compact state and truncates persisted messages. */
func (s *AgentSession) RewindContext(ctx context.Context, seq int64) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if !s.profile.Snapshot().Rewind {
		return errors.New("runtime profile does not permit rewind")
	}
	if seq < 0 {
		return errors.New("runtime rewind sequence must be non-negative")
	}
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()

	s.contextMu.Lock()
	defer s.contextMu.Unlock()
	if err := s.ensureContextLoadedLocked(); err != nil {
		return err
	}
	if state := s.contextCompactState; state != nil && state.Summary != "" && seq <= state.CoveredUntilSeq {
		invalidated := &session.CompactState{CoveredUntilSeq: -1}
		record := s.record
		if err := s.store.SaveContextCompactState(&record, invalidated); err != nil {
			return fmt.Errorf("commit runtime compact state: %w", err)
		}
		s.contextCompactState = cloneCompactState(invalidated)
	}
	record := s.record
	if err := s.store.TruncateMessagesBefore(&record, seq); err != nil {
		return fmt.Errorf("truncate runtime messages: %w", err)
	}
	kept := make([]session.MessageRecord, 0, len(s.contextRecords))
	for _, messageRecord := range s.contextRecords {
		if messageRecord.Seq < seq {
			kept = append(kept, messageRecord)
		}
	}
	s.contextRecords = kept
	return nil
}

/* Prepare implements engine.Conversation with one immutable invocation projection. */
func (c *ContextController) Prepare(ctx context.Context, request engine.ConversationRequest) (engine.ConversationProjection, error) {
	if err := contextError(ctx); err != nil {
		return engine.ConversationProjection{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if request.Input.Prompt != c.scope.resolved.Snapshot().Prompt {
		return engine.ConversationProjection{}, fmt.Errorf("conversation prompt %q does not match admitted run", request.Input.Prompt)
	}
	if err := c.initialize(ctx); err != nil {
		return engine.ConversationProjection{}, err
	}

	trigger, shouldCompact := c.session.claimContextPreparation(c.scope.run.ID, request.Turn, request.Preparation)
	compactions := make([]engine.ConversationCompaction, 0, 1)
	if shouldCompact {
		compaction, err := c.compact(ctx, request, trigger)
		if err != nil {
			return engine.ConversationProjection{}, err
		}
		if compaction != nil {
			compactions = append(compactions, *compaction)
		}
	}
	if request.Phase == engine.PhaseAction && len(compactions) == 0 {
		if err := c.checkBudget(ctx, request); err != nil {
			return engine.ConversationProjection{}, err
		}
	}
	run := c.scope.resolved.Snapshot()
	return engine.ConversationProjection{
		Context: engine.RunContext{
			Messages: cloneContextMessages(c.messages),
			Model:    run.Model, Provider: run.ProviderProtocol, Effort: run.Effort,
		},
		Compactions: compactions,
	}, nil
}

func (c *ContextController) checkBudget(ctx context.Context, request engine.ConversationRequest) error {
	if c.compactor == nil {
		return nil
	}
	return c.compactor.CheckContext(ctx, ContextBudgetRequest{
		Messages:        cloneContextMessages(c.messages),
		ToolDefinitions: cloneContextToolDefinitions(request.ToolDefinitions),
	})
}

/* RequestChanges commits persistent changes and appends transient changes in exact order. */
func (c *ContextController) RequestChanges(ctx context.Context, changes []engine.ConversationChange) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.initialized {
		return errors.New("runtime context is not prepared")
	}
	for _, change := range changes {
		switch change.Kind {
		case engine.ConversationAppendMessage:
		case engine.ConversationAppendContextMessage:
			if change.Source == "" {
				return errors.New("transient conversation change requires a source")
			}
		default:
			return fmt.Errorf("unsupported conversation change kind %q", change.Kind)
		}
	}
	for _, change := range changes {
		message := cloneContextMessage(change.Message)
		if change.Kind == engine.ConversationAppendMessage {
			if _, err := c.session.commitMessage(c.scope, message, ""); err != nil {
				return err
			}
		}
		c.messages = append(c.messages, message)
	}
	return nil
}

func (c *ContextController) initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}
	if !c.collected {
		run := c.scope.resolved.Snapshot()
		fragments, err := c.collector.Collect(ctx, ContextCollectionRequest{
			Profile: run.Profile, Prompt: run.Prompt, WorkDir: run.WorkDir,
			CollaborationMode: run.CollaborationMode, AllowedTools: c.scope.AllowedTools(), ReadOnly: run.ReadOnly,
		})
		if err != nil {
			return fmt.Errorf("collect runtime context: %w", err)
		}
		c.systemPrompt = prompt.Render(append([]prompt.Fragment(nil), fragments...))
		c.collected = true
	}
	run := c.scope.resolved.Snapshot()
	if err := c.session.ensureInitialMessage(c.scope, engine.Message{Role: engine.RoleUser, Content: run.Prompt}, run.DisplayPrompt); err != nil {
		return err
	}
	records, state, err := c.session.contextSnapshot()
	if err != nil {
		return err
	}
	c.messages = projectStoredContext(c.systemPrompt, c.session.record.TranscriptPath(), state, records)
	c.initialized = true
	return nil
}

func (c *ContextController) compact(ctx context.Context, request engine.ConversationRequest, trigger ContextCompactionTrigger) (*engine.ConversationCompaction, error) {
	if c.compactor == nil {
		return nil, nil
	}
	records, state, err := c.session.contextSnapshot()
	if err != nil {
		return nil, err
	}
	proposal, err := c.compactor.Compact(ctx, ContextCompactionRequest{
		Trigger: trigger, Messages: cloneContextMessages(c.messages),
		ToolDefinitions: cloneContextToolDefinitions(request.ToolDefinitions),
		Records:         cloneMessageRecords(records), CompactState: cloneCompactState(state),
		TranscriptPath: c.session.record.TranscriptPath(),
	})
	if err != nil {
		var blocked *ContextBlockedError
		if trigger != ContextCompactionInitialHistory && !errors.As(err, &blocked) {
			return nil, nil
		}
		return nil, err
	}
	if err := validateCompactionProposal(trigger, proposal, records); err != nil {
		return nil, err
	}
	if !proposal.Changed {
		return nil, nil
	}
	if proposal.CompactState != nil {
		if err := c.session.commitCompactState(proposal.CompactState); err != nil {
			return nil, err
		}
		records, state, err = c.session.contextSnapshot()
		if err != nil {
			return nil, err
		}
		c.messages = projectStoredContext(c.systemPrompt, c.session.record.TranscriptPath(), state, records)
	} else {
		c.messages = cloneContextMessages(proposal.Messages)
	}
	return &engine.ConversationCompaction{Trigger: string(trigger)}, nil
}

func validateCompactionProposal(trigger ContextCompactionTrigger, proposal ContextCompactionProposal, records []session.MessageRecord) error {
	if !proposal.Changed {
		return nil
	}
	if proposal.Messages != nil && proposal.CompactState != nil {
		return errors.New("context compaction proposal contains two authorities")
	}
	durable := trigger == ContextCompactionInitialHistory || trigger == ContextCompactionManual
	if durable && proposal.CompactState == nil {
		return errors.New("durable context compaction returned no compact state")
	}
	if !durable && proposal.Messages == nil {
		return errors.New("run-local context compaction returned no messages")
	}
	if proposal.CompactState == nil {
		return nil
	}
	if proposal.CompactState.Summary == "" {
		return errors.New("context compaction returned an empty durable summary")
	}
	maxSeq := int64(-1)
	for _, record := range records {
		if record.Seq > maxSeq {
			maxSeq = record.Seq
		}
	}
	if proposal.CompactState.CoveredUntilSeq < 0 || proposal.CompactState.CoveredUntilSeq > maxSeq {
		return fmt.Errorf("context compaction coverage %d is outside persisted range through %d", proposal.CompactState.CoveredUntilSeq, maxSeq)
	}
	return nil
}

func (s *AgentSession) claimContextPreparation(runID session.RunID, turn int, preparation engine.ConversationPreparation) (ContextCompactionTrigger, bool) {
	if preparation == engine.ConversationPrepareReactive {
		return ContextCompactionReactive, true
	}
	s.contextMu.Lock()
	defer s.contextMu.Unlock()
	key := contextTurnKey{runID: runID, turn: turn}
	if !s.contextInitialPrepared[runID] {
		s.contextInitialPrepared[runID] = true
		s.contextPreparedTurns[key] = true
		return ContextCompactionInitialHistory, true
	}
	if s.contextPreparedTurns[key] {
		return "", false
	}
	s.contextPreparedTurns[key] = true
	return ContextCompactionPreTurn, true
}

func (s *AgentSession) releaseRunContext(runID session.RunID) {
	s.contextMu.Lock()
	defer s.contextMu.Unlock()
	delete(s.contextInitialPrepared, runID)
	for key := range s.contextPreparedTurns {
		if key.runID == runID {
			delete(s.contextPreparedTurns, key)
		}
	}
}

func (s *AgentSession) ensureInitialMessage(scope *RunScope, message engine.Message, display string) error {
	if scope == nil || scope.owner != s {
		return ErrRunScopeOwner
	}
	s.contextMu.Lock()
	defer s.contextMu.Unlock()
	if err := s.ensureContextLoadedLocked(); err != nil {
		return err
	}
	for _, record := range s.contextRecords {
		if record.RunID == scope.run.ID {
			return nil
		}
	}
	return s.appendMessageLocked(scope.run.ID, message, display)
}

func (s *AgentSession) commitMessage(scope *RunScope, message engine.Message, display string) (session.MessageRecord, error) {
	if scope == nil || scope.owner != s {
		return session.MessageRecord{}, ErrRunScopeOwner
	}
	s.contextMu.Lock()
	defer s.contextMu.Unlock()
	if err := s.ensureContextLoadedLocked(); err != nil {
		return session.MessageRecord{}, err
	}
	if err := s.appendMessageLocked(scope.run.ID, message, display); err != nil {
		return session.MessageRecord{}, err
	}
	return cloneMessageRecords(s.contextRecords[len(s.contextRecords)-1:])[0], nil
}

func (s *AgentSession) appendMessageLocked(runID session.RunID, message engine.Message, display string) error {
	record := s.record
	persisted, err := s.store.AppendMessage(&record, runID, cloneContextMessage(message), display)
	if err != nil {
		return fmt.Errorf("commit runtime message: %w", err)
	}
	persisted.Message = cloneContextMessage(persisted.Message)
	s.contextRecords = append(s.contextRecords, persisted)
	if persisted.RunID != runID {
		return fmt.Errorf("persisted runtime message run ID %q does not match %q", persisted.RunID, runID)
	}
	return nil
}

func (s *AgentSession) contextSnapshot() ([]session.MessageRecord, *session.CompactState, error) {
	s.contextMu.Lock()
	defer s.contextMu.Unlock()
	if err := s.ensureContextLoadedLocked(); err != nil {
		return nil, nil, err
	}
	return cloneMessageRecords(s.contextRecords), cloneCompactState(s.contextCompactState), nil
}

func (s *AgentSession) ensureContextLoadedLocked() error {
	if s.contextLoaded {
		return nil
	}
	record := s.record
	records, err := s.store.LoadMessageRecords(&record)
	if err != nil {
		return fmt.Errorf("load runtime messages: %w", err)
	}
	state, err := s.store.LoadContextCompactState(&record)
	if err != nil {
		return fmt.Errorf("load runtime compact state: %w", err)
	}
	s.contextRecords = cloneMessageRecords(records)
	s.contextCompactState = cloneCompactState(state)
	s.contextLoaded = true
	return nil
}

func (s *AgentSession) commitCompactState(state *session.CompactState) error {
	s.contextMu.Lock()
	defer s.contextMu.Unlock()
	if err := s.ensureContextLoadedLocked(); err != nil {
		return err
	}
	record := s.record
	copy := cloneCompactState(state)
	if err := s.store.SaveContextCompactState(&record, copy); err != nil {
		return fmt.Errorf("commit runtime compact state: %w", err)
	}
	s.contextCompactState = cloneCompactState(copy)
	return nil
}

func projectStoredContext(systemPrompt, transcriptPath string, state *session.CompactState, records []session.MessageRecord) []engine.Message {
	messages := []engine.Message{{Role: engine.RoleSystem, Content: systemPrompt}}
	covered := int64(-1)
	if state != nil && state.Summary != "" {
		covered = state.CoveredUntilSeq
		messages = append(messages, engine.Message{Role: engine.RoleUser, Content: prompt.CompactedSummary(state.Summary, transcriptPath)})
	}
	for _, record := range records {
		if record.Seq > covered {
			messages = append(messages, cloneContextMessage(record.Message))
		}
	}
	return messages
}

func cloneContextMessages(messages []engine.Message) []engine.Message {
	result := make([]engine.Message, len(messages))
	for index, message := range messages {
		result[index] = cloneContextMessage(message)
	}
	return result
}

func cloneContextMessage(message engine.Message) engine.Message {
	copy := message
	copy.ToolCalls = append([]engine.ToolCall(nil), message.ToolCalls...)
	for index := range copy.ToolCalls {
		copy.ToolCalls[index].Arguments = append([]byte(nil), message.ToolCalls[index].Arguments...)
	}
	return copy
}

func cloneContextToolDefinitions(definitions []engine.ToolDefinition) []engine.ToolDefinition {
	result := make([]engine.ToolDefinition, len(definitions))
	for index, definition := range definitions {
		result[index] = definition
		if definition.InputSchema == nil {
			continue
		}
		encoded, err := json.Marshal(definition.InputSchema)
		if err != nil {
			continue
		}
		var cloned any
		if json.Unmarshal(encoded, &cloned) == nil {
			result[index].InputSchema = cloned
		}
	}
	return result
}

func cloneMessageRecords(records []session.MessageRecord) []session.MessageRecord {
	result := make([]session.MessageRecord, len(records))
	for index, record := range records {
		result[index] = record
		result[index].Message = cloneContextMessage(record.Message)
	}
	return result
}

func cloneCompactState(state *session.CompactState) *session.CompactState {
	if state == nil {
		return nil
	}
	copy := *state
	return &copy
}

func cloneCompactionProposal(proposal ContextCompactionProposal) ContextCompactionProposal {
	proposal.Messages = cloneContextMessages(proposal.Messages)
	proposal.CompactState = cloneCompactState(proposal.CompactState)
	return proposal
}
