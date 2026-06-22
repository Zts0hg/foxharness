package automemory

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

// extractionTimeout bounds every extraction pass so a slow or unreachable
// extraction provider cannot hang a caller that drains the launch (notably the
// one-shot CLI). It is a var so tests can shrink it.
var extractionTimeout = 2 * time.Minute

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
// It is fire-and-forget: the actual work runs in a detached, timeout-bounded
// goroutine and never blocks the caller, suited to long-lived runners (the
// interactive TUI) where the process outlives the run. When the tracker reports
// an inline memory write this run, the extractor skips itself (mutual
// exclusion). The launch call does not recover; callers that want launch-panic
// isolation wrap the call.
func (h *PerRunHooks) Fire(sess *session.Session, sinceSeq int64, tracker *Tracker) {
	if h.FireFunc != nil {
		h.FireFunc(sess, sinceSeq, tracker)
		return
	}
	go h.fireWithTimeout(sess, sinceSeq, tracker)
}

// FireTracked is like Fire but registers the launch on the provided WaitGroup so
// a short-lived runner (e.g. the one-shot CLI) can Wait for extraction to finish
// before the process exits, preventing the detached goroutine from being killed
// mid-call. The wait is bounded by extractionTimeout. Long-lived runners should
// use Fire instead.
func (h *PerRunHooks) FireTracked(wg *sync.WaitGroup, sess *session.Session, sinceSeq int64, tracker *Tracker) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		h.fireWithTimeout(sess, sinceSeq, tracker)
	}()
}

// fireWithTimeout runs the extraction pass under a timeout-bounded context so a
// slow or unreachable provider cannot make the pass (or a caller draining it)
// hang indefinitely.
func (h *PerRunHooks) fireWithTimeout(sess *session.Session, sinceSeq int64, tracker *Tracker) {
	ctx, cancel := context.WithTimeout(context.Background(), extractionTimeout)
	defer cancel()
	h.RunExtraction(ctx, sess, sinceSeq, tracker)
}

// RunExtraction is the synchronous extraction core: it loads just this run's
// messages (Seq >= sinceSeq) and runs the isolated Extractor under ctx.
// Extractor.Run recovers its own panics, so this does not propagate them.
func (h *PerRunHooks) RunExtraction(ctx context.Context, sess *session.Session, sinceSeq int64, tracker *Tracker) {
	messages, err := session.NewMessageLog(sess).LoadMessagesSince(sinceSeq)
	if err != nil {
		log.Printf("[automemory] extraction skipped: failed to load run messages: %v", err)
		return
	}
	if err := NewExtractor(h.provider, h.store, h.workDir).Run(ctx, messages, tracker); err != nil {
		log.Printf("[automemory] extraction error (swallowed): %v", err)
	}
}

// fireExtractionAsync is retained as the historical default FireFunc for tests
// that override Fire via the FireFunc field.
func (h *PerRunHooks) fireExtractionAsync(sess *session.Session, sinceSeq int64, tracker *Tracker) {
	go h.fireWithTimeout(sess, sinceSeq, tracker)
}
