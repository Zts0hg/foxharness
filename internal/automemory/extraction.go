package automemory

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// defaultExtractionTurns bounds how many model turns the extraction loop takes.
const defaultExtractionTurns = 6

// Extractor runs the asynchronous, context-isolated post-run extraction pass
// (PLD-3 / NFR-001). It reads the run's messages as read-only input, runs its own
// short loop over provider.Generate with its own message slice and a memory-dir
// narrowed registry, and writes any missed memories. It never appends to the main
// session message log, transcript, or system prompt: it owns only the slice it
// builds internally.
type Extractor struct {
	provider provider.LLMProvider
	store    *Store
	workDir  string
	maxTurns int
}

// NewExtractor constructs an Extractor bound to a provider, memory store, and the
// project working directory.
func NewExtractor(p provider.LLMProvider, store *Store, workDir string) *Extractor {
	return &Extractor{provider: p, store: store, workDir: absWorkDir(workDir), maxTurns: defaultExtractionTurns}
}

// Run performs the extraction pass. It returns nil and does nothing when the
// tracker reports the main agent already wrote a memory during the run (mutual
// exclusion, REQ-011). All internal failures — provider errors and panics — are
// logged and swallowed so extraction never affects the main run (NFR-001).
func (e *Extractor) Run(ctx context.Context, runMessages []schema.Message, tracker *Tracker) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[automemory] extraction panic recovered: %v", r)
			err = nil
		}
	}()

	if tracker != nil && tracker.WroteMemory() {
		log.Printf("[automemory] extraction skipped: main agent already wrote a memory this run")
		return nil
	}

	registry := e.buildRegistry()
	messages := e.seedMessages(runMessages)
	availableTools := registry.GetAvailableTools()

	for turn := 0; turn < e.maxTurns; turn++ {
		resp, genErr := e.provider.Generate(ctx, messages, availableTools)
		if genErr != nil {
			log.Printf("[automemory] extraction generate failed (swallowed): %v", genErr)
			return nil
		}
		if resp == nil || resp.Message == nil {
			return nil
		}
		assistant := schema.NormalizeMessage(*resp.Message)
		messages = append(messages, assistant)

		if len(assistant.ToolCalls) == 0 {
			return nil
		}
		for _, call := range assistant.ToolCalls {
			result := registry.Execute(ctx, call)
			messages = append(messages, schema.Message{
				Role:       schema.RoleUser,
				Content:    result.Output,
				ToolCallID: call.ID,
			})
		}
	}
	return nil
}

// buildRegistry assembles the narrowed extraction registry: read-only file reads
// plus write_file/edit_file confined to the memory directories by the
// MemoryDirGuard middleware (REQ-013 / PLD-4).
func (e *Extractor) buildRegistry() tools.Registry {
	reg := tools.NewRegistry()
	reg.Register(tools.NewReadFileTool(e.workDir))
	reg.Register(tools.NewWriteFileTool(e.workDir))
	reg.Register(tools.NewEditFileTool(e.workDir))
	reg.Use(middleware.NewMemoryDirGuard(e.workDir, []string{e.store.UserGlobalDir(), e.store.ProjectDir()}))
	return reg
}

// seedMessages builds the extractor's own initial message slice: the extraction
// system instructions (guardrails + manifest + memory directories) followed by a
// read-only rendering of the run's conversation.
func (e *Extractor) seedMessages(runMessages []schema.Message) []schema.Message {
	userRel := e.relToWorkDir(e.store.UserGlobalDir())
	projectRel := e.relToWorkDir(e.store.ProjectDir())
	system := ExtractionInstructions(e.store.Manifest(), userRel, projectRel)

	return []schema.Message{
		{Role: schema.RoleSystem, Content: system},
		{Role: schema.RoleUser, Content: "Conversation to review:\n\n" + renderConversation(runMessages)},
	}
}

func (e *Extractor) relToWorkDir(abs string) string {
	rel, err := filepath.Rel(e.workDir, abs)
	if err != nil {
		return abs
	}
	return rel
}

// renderConversation flattens the run's messages into a readable transcript for
// the extraction model. It is a copy: the input slice is never mutated.
func renderConversation(messages []schema.Message) string {
	var b strings.Builder
	for _, m := range messages {
		switch {
		case m.ToolCallID != "":
			fmt.Fprintf(&b, "TOOL RESULT: %s\n", strings.TrimSpace(m.Content))
		case len(m.ToolCalls) > 0:
			for _, c := range m.ToolCalls {
				fmt.Fprintf(&b, "ASSISTANT TOOL CALL %s(%s)\n", c.Name, strings.TrimSpace(string(c.Arguments)))
			}
			if strings.TrimSpace(m.Content) != "" {
				fmt.Fprintf(&b, "ASSISTANT: %s\n", strings.TrimSpace(m.Content))
			}
		default:
			fmt.Fprintf(&b, "%s: %s\n", strings.ToUpper(string(m.Role)), strings.TrimSpace(m.Content))
		}
	}
	return b.String()
}
