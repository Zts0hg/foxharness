/* Package planruntime owns turn-boundary Formal Plan capability transitions. */
package planruntime

import (
	"context"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type phase int

const (
	formal phase = iota
	checklist
	implementation
)

/* Lifecycle keeps Formal Plan tool transitions at model-turn boundaries. */
type Lifecycle struct {
	mu sync.Mutex

	phase   phase
	pending *phase

	formalRegistry    tools.Registry
	checklistRegistry tools.Registry
	defaultRegistry   tools.Registry

	approvedPlan      string
	approved          bool
	submittedThisTurn bool
	onApproved        func()
}

/* New constructs a lifecycle from its three exact tool surfaces. */
func New(formalRegistry, checklistRegistry, defaultRegistry tools.Registry, onApproved func()) *Lifecycle {
	return &Lifecycle{
		phase: formal, formalRegistry: formalRegistry, checklistRegistry: checklistRegistry,
		defaultRegistry: defaultRegistry, onApproved: onApproved,
	}
}

/* SetFormalRegistry completes construction when submit_plan needs the lifecycle approval callback. */
func (l *Lifecycle) SetFormalRegistry(registry tools.Registry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.formalRegistry = registry
}

/* Approve schedules the checklist surface for the next model turn. */
func (l *Lifecycle) Approve(planMarkdown string) {
	next := checklist
	l.mu.Lock()
	if l.phase != formal {
		l.mu.Unlock()
		return
	}
	l.pending = &next
	l.approvedPlan = planMarkdown
	l.approved = true
	onApproved := l.onApproved
	l.mu.Unlock()
	if onApproved != nil {
		onApproved()
	}
}

/* BeginTurn commits one pending phase transition before tool discovery. */
func (l *Lifecycle) BeginTurn() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.submittedThisTurn = false
	if l.pending == nil {
		return
	}
	l.phase = *l.pending
	l.pending = nil
}

/* Register adds a tool to every lifecycle surface. */
func (l *Lifecycle) Register(tool tools.BaseTool) {
	for _, registry := range l.registries() {
		registry.Register(tool)
	}
}

/* Use adds middleware to every lifecycle surface. */
func (l *Lifecycle) Use(middleware middleware.Middleware) {
	for _, registry := range l.registries() {
		registry.Use(middleware)
	}
}

/* GetAvailableTools returns the active turn's definitions. */
func (l *Lifecycle) GetAvailableTools() []schema.ToolDefinition {
	return l.activeRegistry().GetAvailableTools()
}

/* Execute delegates to the active surface and schedules successful transitions. */
func (l *Lifecycle) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	registry, current := l.registryAndPhase()
	if current == formal && call.Name == "submit_plan" {
		l.mu.Lock()
		alreadySubmitted := l.submittedThisTurn
		l.submittedThisTurn = true
		l.mu.Unlock()
		if alreadySubmitted {
			return schema.ToolResult{
				ToolCallID: call.ID, Output: "submit_plan cannot be called more than once in the same turn", IsError: true,
			}
		}
	}
	result := registry.Execute(ctx, call)
	if current == checklist && call.Name == "update_todo" && !result.IsError {
		next := implementation
		l.mu.Lock()
		if l.phase == checklist {
			l.pending = &next
		}
		l.mu.Unlock()
	}
	return result
}

/* MemoryExtractionAllowed reports whether the user approved a complete plan. */
func (l *Lifecycle) MemoryExtractionAllowed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.approved
}

/* IsParallelSafe delegates safety lookup to the active surface. */
func (l *Lifecycle) IsParallelSafe(toolName string) bool {
	return l.activeRegistry().IsParallelSafe(toolName)
}

/* CompletionReminder returns the current incomplete lifecycle gate. */
func (l *Lifecycle) CompletionReminder() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch l.phase {
	case formal:
		return "Formal Plan mode cannot finish before a complete proposal is successfully submitted through submit_plan and approved by the user."
	case checklist:
		return "The approved plan cannot be implemented or completed until update_todo successfully initializes an ordered, executable, and verifiable checklist."
	default:
		return ""
	}
}

/* RuntimeReminders returns the approved plan while checklist initialization is pending. */
func (l *Lifecycle) RuntimeReminders() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.phase != checklist || l.approvedPlan == "" {
		return nil
	}
	return []string{approvedPlanReminder(l.approvedPlan)}
}

func (l *Lifecycle) activeRegistry() tools.Registry {
	registry, _ := l.registryAndPhase()
	return registry
}

func (l *Lifecycle) registryAndPhase() (tools.Registry, phase) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch l.phase {
	case checklist:
		return l.checklistRegistry, l.phase
	case implementation:
		return l.defaultRegistry, l.phase
	default:
		return l.formalRegistry, l.phase
	}
}

func (l *Lifecycle) registries() []tools.Registry {
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

func approvedPlanReminder(planMarkdown string) string {
	return strings.TrimSpace(`
The user approved the complete plan below. Formal Plan restrictions have ended for this same task, but explicit implementation tools remain unavailable until you successfully call update_todo with an ordered, executable, and verifiable checklist derived from this plan. Read-only revalidation may happen first.

<approved_plan>
`) + "\n" + planMarkdown + "\n</approved_plan>"
}

var _ tools.Registry = (*Lifecycle)(nil)
var _ tools.TurnAwareRegistry = (*Lifecycle)(nil)
