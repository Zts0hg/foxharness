package automemory

import (
	"context"
	"log"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

// PerRunHooks bundles the per-run persistent-memory wiring shared by every
// runner that drives the engine: a Tracker to attach to the main run's registry
// (mutual exclusion) and an async extraction trigger fired at run end. It keeps
// the CLI, Feishu, and AgentOps runners consistent without duplicating the
// launch glue.
type PerRunHooks struct {
	provider provider.LLMProvider
	store    *Store
	workDir  string

	// FireFunc, when non-nil, replaces the default async extraction launcher.
	// Tests use it to observe or simulate the hook synchronously.
	FireFunc func(sess *session.Session, sinceSeq int64, tracker *Tracker)
}

// NewPerRunHooks builds hooks bound to a provider, memory store, and working
// directory. The provider is read at fire time; callers that swap the provider
// between runs (e.g. a /model switch) should construct fresh hooks per run.
func NewPerRunHooks(p provider.LLMProvider, store *Store, workDir string) *PerRunHooks {
	h := &PerRunHooks{provider: p, store: store, workDir: workDir}
	h.FireFunc = h.fireExtractionAsync
	return h
}

// NewTracker returns a fresh memory-write tracker for one run, watching both
// memory directories.
func (h *PerRunHooks) NewTracker() *Tracker {
	return NewTracker(h.workDir, []string{h.store.UserGlobalDir(), h.store.ProjectDir()})
}

// RecordCallback returns an engine.OnToolCalled callback that records successful
// memory-directory writes on the given tracker. It is the success-gated seam
// runners attach so a failed write never sets the mutual-exclusion flag (the
// Middleware interface only inspects calls pre-execution and cannot see results).
// A nil tracker yields a no-op callback.
func (h *PerRunHooks) RecordCallback(tracker *Tracker) func(schema.ToolCall, schema.ToolResult) {
	return func(call schema.ToolCall, result schema.ToolResult) {
		if tracker != nil {
			tracker.MarkSuccess(call, result)
		}
	}
}

// Fire launches the extraction hook over the messages appended since sinceSeq.
// It is fire-and-forget: the actual work runs in a detached goroutine and never
// blocks the caller. When the tracker reports an inline memory write this run,
// the extractor skips itself (mutual exclusion). The launch call itself does
// not recover; callers that want launch-panic isolation wrap the call.
func (h *PerRunHooks) Fire(sess *session.Session, sinceSeq int64, tracker *Tracker) {
	if h.FireFunc != nil {
		h.FireFunc(sess, sinceSeq, tracker)
	}
}

// fireExtractionAsync is the default launcher: it loads just this run's messages
// (Seq >= sinceSeq) and runs the isolated Extractor in a detached goroutine.
func (h *PerRunHooks) fireExtractionAsync(sess *session.Session, sinceSeq int64, tracker *Tracker) {
	messages, err := session.NewMessageLog(sess).LoadMessagesSince(sinceSeq)
	if err != nil {
		log.Printf("[automemory] extraction skipped: failed to load run messages: %v", err)
		return
	}

	provider := h.provider
	store := h.store
	workDir := h.workDir
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[automemory] extraction goroutine panic recovered: %v", rec)
			}
		}()
		if err := NewExtractor(provider, store, workDir).Run(context.Background(), messages, tracker); err != nil {
			log.Printf("[automemory] extraction error (swallowed): %v", err)
		}
	}()
}
