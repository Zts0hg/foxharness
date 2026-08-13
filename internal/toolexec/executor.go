/*
Package toolexec executes immutable, run-scoped tool capability snapshots.
*/
package toolexec

import (
	"context"
	"fmt"
	"sync"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
)

/* Capability binds one advertised call name to its execution behavior. */
type Capability struct {
	Definition   schema.ToolDefinition
	ParallelSafe bool
	Execute      func(context.Context, schema.ToolCall) engine.ToolExecutionResult
}

/* Executor creates immutable capability snapshots and schedules ordered batches. */
type Executor struct {
	capabilities []Capability
}

/* New constructs an executor from already constrained and alias-resolved capabilities. */
func New(capabilities []Capability) *Executor {
	copied := make([]Capability, len(capabilities))
	for index, capability := range capabilities {
		capability.Definition = cloneDefinition(capability.Definition)
		copied[index] = capability
	}
	return &Executor{capabilities: copied}
}

/* Snapshot freezes advertised definitions and executable capabilities together. */
func (e *Executor) Snapshot(context.Context) (engine.ToolSnapshot, error) {
	if e == nil {
		return nil, fmt.Errorf("tool executor is required")
	}
	snapshot := &capabilitySnapshot{
		owner:        e,
		capabilities: make(map[string]Capability, len(e.capabilities)),
		definitions:  make([]schema.ToolDefinition, 0, len(e.capabilities)),
	}
	for _, capability := range e.capabilities {
		capability.Definition = cloneDefinition(capability.Definition)
		snapshot.capabilities[capability.Definition.Name] = capability
		snapshot.definitions = append(snapshot.definitions, cloneDefinition(capability.Definition))
	}
	return snapshot, nil
}

/* Execute runs calls against the exact snapshot that advertised their names. */
func (e *Executor) Execute(
	ctx context.Context,
	snapshot engine.ToolSnapshot,
	calls []schema.ToolCall,
) (engine.ToolBatch, error) {
	capabilities, ok := snapshot.(*capabilitySnapshot)
	if !ok || capabilities.owner != e {
		return engine.ToolBatch{}, fmt.Errorf("tool snapshot does not belong to executor")
	}
	results := make([]engine.ToolExecutionResult, len(calls))
	for start := 0; start < len(calls); {
		capability, parallel := capabilities.capabilities[calls[start].Name]
		parallel = parallel && capability.ParallelSafe
		if !parallel {
			results[start] = executeOne(ctx, capabilities, calls[start])
			start++
			continue
		}
		end := start + 1
		for end < len(calls) {
			next, exists := capabilities.capabilities[calls[end].Name]
			if !exists || !next.ParallelSafe {
				break
			}
			end++
		}
		var group sync.WaitGroup
		group.Add(end - start)
		for index := start; index < end; index++ {
			index := index
			go func() {
				defer group.Done()
				results[index] = executeOne(ctx, capabilities, calls[index])
			}()
		}
		group.Wait()
		start = end
	}
	return engine.ToolBatch{Results: results}, nil
}

type capabilitySnapshot struct {
	owner        *Executor
	definitions  []schema.ToolDefinition
	capabilities map[string]Capability
}

func (s *capabilitySnapshot) ToolDefinitions() []schema.ToolDefinition {
	definitions := make([]schema.ToolDefinition, len(s.definitions))
	for index, definition := range s.definitions {
		definitions[index] = cloneDefinition(definition)
	}
	return definitions
}

func executeOne(ctx context.Context, snapshot *capabilitySnapshot, call schema.ToolCall) engine.ToolExecutionResult {
	capability, ok := snapshot.capabilities[call.Name]
	if !ok {
		content := fmt.Sprintf("Error: tool '%s' does not exist in the system", call.Name)
		return failureResult(call.ID, content)
	}
	if err := ctx.Err(); err != nil {
		return failureResult(call.ID, err.Error())
	}
	if capability.Execute == nil {
		return failureResult(call.ID, fmt.Sprintf("Error executing %s: executable capability is missing", call.Name))
	}
	result := capability.Execute(ctx, cloneCall(call))
	if result.CallID == "" {
		result.CallID = call.ID
	}
	if result.FullContent == "" {
		result.FullContent = result.ModelContent
	}
	if result.ObserverContent == "" {
		result.ObserverContent = result.ModelContent
	}
	return result
}

func failureResult(callID, content string) engine.ToolExecutionResult {
	return engine.ToolExecutionResult{
		CallID: callID, FullContent: content, ModelContent: content,
		ObserverContent: content, IsError: true,
	}
}

func cloneCall(call schema.ToolCall) schema.ToolCall {
	call.Arguments = append([]byte(nil), call.Arguments...)
	return call
}

func cloneDefinition(definition schema.ToolDefinition) schema.ToolDefinition {
	definition.InputSchema = cloneJSONValue(definition.InputSchema)
	return definition
}

func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = cloneJSONValue(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, item := range value {
			result[index] = cloneJSONValue(item)
		}
		return result
	case []string:
		return append([]string(nil), value...)
	case []byte:
		return append([]byte(nil), value...)
	default:
		return value
	}
}
