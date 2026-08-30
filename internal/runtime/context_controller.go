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
	return fmt.Sprintf("上下文 token 数 (%d) 超过阻塞阈值 (%d)，无法继续发送请求", e.UsedTokens, e.Limit)
}

/* ContextCollectionRequest contains frozen run values used to resolve prompt fragments. */
type ContextCollectionRequest struct {
	Profile           ProfileName
	Prompt            string
	WorkDir           string
	CollaborationMode string
	AllowedTools      []string
	/* RestrictedTools reports a ChildRun profile run, whose prompt guidance
	 * is always scoped to the resolved AllowedTools snapshot. Main-surface
	 * runs keep the full base prompt regardless of tool restrictions. */
	RestrictedTools bool
	ReadOnly        bool
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
	/* injected mirrors messages one-to-one and marks transient context notices
	 * that were appended after the previous prepare, so the pre-turn compaction
	 * input can exclude them the way the baseline did by appending its notices
	 * after the compaction ran. */
	injected []bool
	/* injectedTurn mirrors injected one-to-one and records the producing turn
	 * each notice carries, so the pre-turn compaction input can exclude only
	 * the current turn's notices the way the baseline loop carried the
	 * previous turn's notices into its turn-start decision. */
	injectedTurn []int
	/* compactedTurn records the turn that already performed a run-local
	 * compaction, mirroring the baseline justCompacted flag: the turn's budget
	 * check is suppressed once that compaction committed. */
	compactedTurn int
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
	if isNilRuntimeDependency(collector) {
		return nil, errors.New("runtime context collector is required")
	}
	if isNilRuntimeDependency(compactor) {
		compactor = nil
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
	if isNilRuntimeDependency(compactor) {
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
	projected := projectCompactionInput(state, records)
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
		before := len(c.messages)
		compaction, err := c.compact(ctx, request, trigger)
		if err != nil {
			return engine.ConversationProjection{}, err
		}
		if compaction != nil {
			if trigger == ContextCompactionPreTurn || trigger == ContextCompactionReactive {
				c.compactedTurn = request.Turn
			}
			compaction.BeforeMessages = before
			compaction.AfterMessages = len(c.messages)
			compactions = append(compactions, *compaction)
		}
	}
	if request.Phase == engine.PhaseAction && c.compactedTurn != request.Turn {
		if err := c.checkBudget(ctx, request); err != nil {
			return engine.ConversationProjection{}, err
		}
	}
	/* The projection the engine invoked now covers every pending notice, so
	 * the next compaction decision may include them again. */
	c.injected = make([]bool, len(c.messages))
	c.injectedTurn = make([]int, len(c.messages))
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
	/* The engine requests a turn's policy notices before that turn's first
	 * prepare, so a run's first notices arrive before any preparation and
	 * must initialize the projection themselves, the way the baseline
	 * carried its turn-one reminders on top of the built initial context. */
	if !c.initialized {
		if err := c.initialize(ctx); err != nil {
			return err
		}
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
		c.injected = append(c.injected, change.Kind == engine.ConversationAppendContextMessage)
		c.injectedTurn = append(c.injectedTurn, change.Turn)
	}
	return nil
}

/* pendingInjections returns the transient notices appended since the previous
 * prepare, in conversation order. */
func (c *ContextController) pendingInjections() []engine.Message {
	notices := make([]engine.Message, 0, len(c.messages))
	for index, message := range c.messages {
		if c.injected[index] {
			notices = append(notices, message)
		}
	}
	return notices
}

/* pendingInjectionsForTurn returns the pending transient notices the given
 * turn produced, in conversation order. A notice with an unknown producing
 * turn is attributed to every turn, so it stays excluded wherever the turn's
 * own notices are excluded. */
func (c *ContextController) pendingInjectionsForTurn(turn int) []engine.Message {
	notices := make([]engine.Message, 0, len(c.messages))
	for index, message := range c.messages {
		if c.injected[index] && (c.injectedTurn[index] == 0 || c.injectedTurn[index] == turn) {
			notices = append(notices, message)
		}
	}
	return notices
}

/* preparedMessages returns the projection without the pending transient
 * notices, which the baseline appends only after its compaction decisions. */
func (c *ContextController) preparedMessages() []engine.Message {
	messages := make([]engine.Message, 0, len(c.messages))
	for index, message := range c.messages {
		if !c.injected[index] {
			messages = append(messages, message)
		}
	}
	return messages
}

/* messagesWithoutTurnNotices returns the projection without the notices the
 * given turn produced, keeping everything appended through the end of the
 * previous turn inside the projection. */
func (c *ContextController) messagesWithoutTurnNotices(turn int) []engine.Message {
	messages := make([]engine.Message, 0, len(c.messages))
	for index, message := range c.messages {
		if !c.injected[index] || (c.injectedTurn[index] != 0 && c.injectedTurn[index] != turn) {
			messages = append(messages, message)
		}
	}
	return messages
}

func (c *ContextController) initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}
	if !c.collected {
		run := c.scope.resolved.Snapshot()
		fragments, err := c.collector.Collect(ctx, ContextCollectionRequest{
			Profile: run.Profile, Prompt: run.Prompt, WorkDir: run.WorkDir,
			CollaborationMode: run.CollaborationMode, AllowedTools: c.scope.AllowedTools(),
			RestrictedTools: c.scope.resolved.restrictedTools, ReadOnly: run.ReadOnly,
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
	c.injected = make([]bool, len(c.messages))
	c.injectedTurn = make([]int, len(c.messages))
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
	/* Each decision compacts the projection the baseline fed the matching
	 * decision point: the durable session-history decision sees the clean
	 * stored projection, the run-local decision sees everything through the
	 * end of the previous turn, and the recovery decision sees the live
	 * projection. The notices each input excluded follow the compaction. */
	var messages []engine.Message
	var reappend []engine.Message
	switch trigger {
	case ContextCompactionInitialHistory:
		messages = c.preparedMessages()
		reappend = c.pendingInjections()
	case ContextCompactionPreTurn:
		messages = c.messagesWithoutTurnNotices(request.Turn)
		reappend = c.pendingInjectionsForTurn(request.Turn)
	default:
		messages = c.messages
	}
	proposal, err := c.compactor.Compact(ctx, ContextCompactionRequest{
		Trigger: trigger, Messages: cloneContextMessages(messages),
		ToolDefinitions: cloneContextToolDefinitions(request.ToolDefinitions),
		Records:         cloneMessageRecords(records), CompactState: cloneCompactState(state),
		TranscriptPath: c.session.record.TranscriptPath(),
	})
	if err != nil {
		var blocked *ContextBlockedError
		if trigger != ContextCompactionInitialHistory && !errors.As(err, &blocked) {
			return nil, nil
		}
		return nil, c.wrapInitialCompactionError(trigger, err)
	}
	if err := validateCompactionProposal(trigger, proposal, records); err != nil {
		return nil, err
	}
	if !proposal.Changed {
		return nil, nil
	}
	if proposal.CompactState != nil {
		if err := c.session.commitCompactState(proposal.CompactState); err != nil {
			return nil, c.wrapInitialCompactionError(trigger, err)
		}
		records, state, err = c.session.contextSnapshot()
		if err != nil {
			return nil, err
		}
		c.messages = projectStoredContext(c.systemPrompt, c.session.record.TranscriptPath(), state, records)
	} else {
		c.messages = cloneContextMessages(proposal.Messages)
	}
	c.messages = append(c.messages, reappend...)
	return &engine.ConversationCompaction{Trigger: string(trigger)}, nil
}

/* wrapInitialCompactionError keeps the baseline assembly chain on the durable
 * session-history decision point, which the baseline performed while building
 * the run's initial context. */
func (c *ContextController) wrapInitialCompactionError(trigger ContextCompactionTrigger, err error) error {
	if trigger != ContextCompactionInitialHistory {
		return err
	}
	return fmt.Errorf("组装 Session 上下文失败: %w", err)
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
		return fmt.Errorf("%s: %w", baselineMessageWriteLabel(message), err)
	}
	persisted.Message = cloneContextMessage(persisted.Message)
	s.contextRecords = append(s.contextRecords, persisted)
	if persisted.RunID != runID {
		return fmt.Errorf("persisted runtime message run ID %q does not match %q", persisted.RunID, runID)
	}
	return nil
}

/* baselineMessageWriteLabel names the persisted message kind in the baseline
 * persistence error chain: user messages, assistant messages, and tool
 * results each carry their own wording. */
func baselineMessageWriteLabel(message engine.Message) string {
	switch {
	case message.ToolCallID != "":
		return "写入 Session 工具结果失败"
	case message.Role == engine.RoleAssistant:
		return "写入 Session 助手消息失败"
	default:
		return "写入 Session 用户消息失败"
	}
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
		return fmt.Errorf("读取 Session 消息历史失败: %w", err)
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

/* projectCompactionInput renders the summarizer input for one explicit
 * compaction: the raw durable summary text followed by the active records,
 * without the projected system message or summary wrapper. */
func projectCompactionInput(state *session.CompactState, records []session.MessageRecord) []engine.Message {
	messages := make([]engine.Message, 0, len(records)+1)
	covered := int64(-1)
	if state != nil && state.Summary != "" {
		covered = state.CoveredUntilSeq
		messages = append(messages, engine.Message{Role: engine.RoleUser, Content: state.Summary})
	}
	for _, record := range records {
		if record.Seq > covered {
			messages = append(messages, cloneContextMessage(record.Message))
		}
	}
	return messages
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
