package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type planLifecyclePhase int

const (
	planLifecycleFormal planLifecyclePhase = iota
	planLifecycleChecklist
	planLifecycleDefault
)

var formalPlanCanonicalTools = []string{
	"read_file",
	"bash",
	"ask_user_question",
	"submit_plan",
}

// planLifecycle keeps tool-surface transitions at model-turn boundaries. A
// pending transition never changes the registry used by remaining calls from
// the response that requested it.
type planLifecycle struct {
	mu sync.Mutex

	phase   planLifecyclePhase
	pending *planLifecyclePhase

	formalRegistry    tools.Registry
	checklistRegistry tools.Registry
	defaultRegistry   tools.Registry

	approvedPlan   string
	reminderQueued bool
	onApproved     func()
}

func newPlanLifecycle(formal, checklist, defaultRegistry tools.Registry, onApproved func()) *planLifecycle {
	return &planLifecycle{
		phase:             planLifecycleFormal,
		formalRegistry:    formal,
		checklistRegistry: checklist,
		defaultRegistry:   defaultRegistry,
		onApproved:        onApproved,
	}
}

func (l *planLifecycle) setFormalRegistry(registry tools.Registry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.formalRegistry = registry
}

func (l *planLifecycle) approve(planMarkdown string) {
	next := planLifecycleChecklist
	l.mu.Lock()
	if l.phase != planLifecycleFormal {
		l.mu.Unlock()
		return
	}
	l.pending = &next
	l.approvedPlan = planMarkdown
	l.reminderQueued = true
	onApproved := l.onApproved
	l.mu.Unlock()

	if onApproved != nil {
		onApproved()
	}
}

// BeginTurn commits a pending approval or checklist transition before the
// engine discovers the next turn's tools.
func (l *planLifecycle) BeginTurn() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pending == nil {
		return
	}
	l.phase = *l.pending
	l.pending = nil
}

func (l *planLifecycle) Register(tool tools.BaseTool) {
	for _, registry := range l.registries() {
		registry.Register(tool)
	}
}

func (l *planLifecycle) Use(m middleware.Middleware) {
	for _, registry := range l.registries() {
		registry.Use(m)
	}
}

func (l *planLifecycle) GetAvailableTools() []schema.ToolDefinition {
	return l.activeRegistry().GetAvailableTools()
}

func (l *planLifecycle) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	registry, phase := l.registryAndPhase()
	if phase == planLifecycleFormal && call.Name == "submit_plan" && l.transitionPending() {
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     "submit_plan cannot be called again after a plan was approved in this turn",
			IsError:    true,
		}
	}
	result := registry.Execute(ctx, call)
	if phase == planLifecycleChecklist && call.Name == "update_todo" && !result.IsError {
		next := planLifecycleDefault
		l.mu.Lock()
		if l.phase == planLifecycleChecklist {
			l.pending = &next
		}
		l.mu.Unlock()
	}
	return result
}

func (l *planLifecycle) transitionPending() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pending != nil
}

func (l *planLifecycle) IsParallelSafe(toolName string) bool {
	return l.activeRegistry().IsParallelSafe(toolName)
}

func (l *planLifecycle) activeRegistry() tools.Registry {
	registry, _ := l.registryAndPhase()
	return registry
}

func (l *planLifecycle) registryAndPhase() (tools.Registry, planLifecyclePhase) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch l.phase {
	case planLifecycleChecklist:
		return l.checklistRegistry, l.phase
	case planLifecycleDefault:
		return l.defaultRegistry, l.phase
	default:
		return l.formalRegistry, l.phase
	}
}

func (l *planLifecycle) registries() []tools.Registry {
	l.mu.Lock()
	defer l.mu.Unlock()
	seen := map[tools.Registry]bool{}
	registries := make([]tools.Registry, 0, 3)
	for _, registry := range []tools.Registry{l.formalRegistry, l.checklistRegistry, l.defaultRegistry} {
		if registry == nil || seen[registry] {
			continue
		}
		seen[registry] = true
		registries = append(registries, registry)
	}
	return registries
}

func (l *planLifecycle) completionReminder() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch l.phase {
	case planLifecycleFormal:
		return "Formal Plan mode cannot finish before a complete proposal is successfully submitted through submit_plan and approved by the user."
	case planLifecycleChecklist:
		return "The approved plan cannot be implemented or completed until update_todo successfully initializes an ordered, executable, and verifiable checklist."
	default:
		return ""
	}
}

func (l *planLifecycle) drainReminders() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.reminderQueued {
		return nil
	}
	l.reminderQueued = false
	return []string{approvedPlanReminder(l.approvedPlan)}
}

func approvedPlanReminder(planMarkdown string) string {
	return strings.TrimSpace(`
The user approved the complete plan below. Formal Plan restrictions have ended for this same task, but explicit implementation tools remain unavailable until you successfully call update_todo with an ordered, executable, and verifiable checklist derived from this plan. Read-only revalidation may happen first.

<approved_plan>
`) + "\n" + planMarkdown + "\n</approved_plan>"
}

func validateFormalPlanAllowedTools(allowed []string) error {
	set := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		set[name] = true
	}
	required := append(append([]string(nil), formalPlanCanonicalTools...), "update_todo")
	var missing []string
	for _, name := range required {
		if !set[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("restricted Formal Plan run is missing required lifecycle tools: %s", strings.Join(missing, ", "))
	}
	return nil
}

var _ tools.Registry = (*planLifecycle)(nil)
var _ tools.TurnAwareRegistry = (*planLifecycle)(nil)
