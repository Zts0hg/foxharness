/*
Package toolruntime composes immutable tool execution with the runtime-visible
full, model, observer, and artifact result forms.
*/
package toolruntime

import (
	"context"
	"fmt"
	"sync"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolexec"
	"github.com/Zts0hg/foxharness/internal/toolresult"
)

const observerOutputLimit = 800

/* Executor owns run-local tool-result persistence and prompt-budget state. */
type Executor struct {
	base    *toolexec.Executor
	fs      toolresult.FileSystem
	dir     string
	mu      sync.Mutex
	seenIDs map[string]bool
}

/* New constructs a run-local tool executor from constrained capabilities. */
func New(capabilities []toolexec.Capability, fs toolresult.FileSystem, artifactDir string) *Executor {
	return &Executor{
		base: toolexec.New(capabilities), fs: fs, dir: artifactDir,
		seenIDs: make(map[string]bool),
	}
}

/* Snapshot freezes the advertised and executable capability surface. */
func (e *Executor) Snapshot(ctx context.Context) (engine.ToolSnapshot, error) {
	if e == nil || e.base == nil || e.fs == nil {
		return nil, fmt.Errorf("runtime tool executor is not configured")
	}
	base, err := e.base.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &snapshot{owner: e, base: base}, nil
}

/* Execute runs one ordered batch and derives its bounded result forms. */
func (e *Executor) Execute(ctx context.Context, frozen engine.ToolSnapshot, calls []schema.ToolCall) (engine.ToolBatch, error) {
	owned, ok := frozen.(*snapshot)
	if !ok || owned.owner != e {
		return engine.ToolBatch{}, fmt.Errorf("runtime tool snapshot does not belong to executor")
	}
	batch, err := e.base.Execute(ctx, owned.base, calls)
	if err != nil {
		return batch, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	persisted := make([]toolresult.PersistedResult, len(batch.Results))
	full := make([]string, len(batch.Results))
	for index, result := range batch.Results {
		full[index] = toolresult.TruncateToCap(result.FullContent)
		persisted[index] = toolresult.PersistIfNeeded(e.fs, e.dir, schema.ToolResult{
			ToolCallID: result.CallID, Output: full[index], IsError: result.IsError,
		})
	}
	persisted = toolresult.EnforceBudget(e.fs, e.dir, persisted, e.seenIDs)
	for index := range batch.Results {
		result := &batch.Results[index]
		result.FullContent = full[index]
		if persisted[index].Persisted {
			result.ModelContent = persisted[index].Preview
			result.ObserverContent = truncateObserverOutput(full[index])
			result.ArtifactPath = persisted[index].FilePath
		}
		e.seenIDs[result.CallID] = true
	}
	return batch, nil
}

type snapshot struct {
	owner *Executor
	base  engine.ToolSnapshot
}

func (s *snapshot) ToolDefinitions() []schema.ToolDefinition {
	return s.base.ToolDefinitions()
}

func truncateObserverOutput(value string) string {
	runes := []rune(value)
	if len(runes) <= observerOutputLimit {
		return value
	}
	return fmt.Sprintf("%s\n... (已截断，原始输出约 %d 字节)", string(runes[:observerOutputLimit]), len(value))
}

var _ engine.ToolExecutor = (*Executor)(nil)
