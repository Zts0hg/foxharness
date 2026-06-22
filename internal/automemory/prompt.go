package automemory

import (
	"fmt"
	"strings"
)

// Guardrails returns the shared memory lifecycle rules (REQ-014). The same text
// is embedded verbatim in both the main-agent guidance and the extraction
// instructions so the two write layers apply identical save criteria. It encodes
// all six required elements: what NOT to save, the surprising/non-obvious saving
// heuristic, a drift caveat, a verify-before-recommending rule, the ignore-memory
// directive, and the dedup-first rule.
func Guardrails() string {
	return strings.TrimSpace(`
Memory lifecycle rules:
- Save only what is surprising or non-obvious and durable across sessions: who the user is, their stated preferences and feedback (with the why), and project goals or constraints not derivable from the code.
- Do NOT save what the repository already records: code patterns, file structure, git history, bug-fix recipes, anything already documented in AGENTS.md/CLAUDE.md, or ephemeral task state (that belongs in working_memory.md).
- Memories are possibly stale: they reflect what was true when written. Verify a memory still holds before relying on it.
- Before recommending a file, function, or flag named in a memory, confirm it still exists in the current code.
- When the user says to ignore memory for a request, proceed as if the index were empty: do not apply, cite, or compare against remembered facts for that request.
- Dedup first: when something is already covered by an existing memory, update the existing file instead of creating a duplicate.`)
}

// frontmatterTemplate documents the on-disk memory file format the agent writes
// with the existing file tools.
func frontmatterTemplate() string {
	return strings.TrimSpace(`
Each memory is one Markdown file with YAML frontmatter:

    ---
    name: <short-kebab-case-slug>
    description: <one-line relevance hook, under ~150 chars>
    type: user | feedback | project | reference
    ---

    <the memory body>

For feedback and project memories, the body must also include a "Why:" line and a "How to apply:" line.

Pick the type that best fits the content:
- user — durable facts about the user (role, expertise, preferences); cross-project. Body has no Why/How.
- feedback — guidance or corrections on how the agent should work; body needs Why + How to apply.
- project — this project's goals, ongoing work, or constraints not derivable from the code or git history; body needs Why + How to apply.
- reference — pointers to external resources (URLs, dashboards, tickets); body has no Why/How.`)
}

// MainMemoryGuidance returns the body of the injected "Persistent Memory" section
// for the main agent: where memory files live (paths relative to the working
// directory), how to create/update/remove and read them with the existing file
// tools, the frontmatter format, and the shared guardrails. The caller (Composer)
// prepends the merged index.
func MainMemoryGuidance(userDirRel, projectDirRel string) string {
	var b strings.Builder
	b.WriteString("Persistent memory survives across sessions. It is stored as typed Markdown files in two directories (paths relative to the working directory):\n")
	fmt.Fprintf(&b, "- user-global (type `user`): %s\n", userDirRel)
	fmt.Fprintf(&b, "- project (types `project`, `feedback`, `reference`): %s\n", projectDirRel)
	b.WriteString("\n")
	b.WriteString("Working with memory:\n")
	b.WriteString("- The index above lists every memory by description. When one is relevant, read its full content with read_file using the directory above plus the file name.\n")
	b.WriteString("- Create or update a memory with write_file/edit_file (the index is regenerated automatically — you do not maintain it by hand).\n")
	b.WriteString("- Forget a memory by overwriting its file with empty content using write_file when the user asks you to forget it or it is confirmed stale.\n")
	b.WriteString("\n")
	b.WriteString(frontmatterTemplate())
	b.WriteString("\n\n")
	b.WriteString(Guardrails())
	return b.String()
}

// ReadOnlyMemoryGuidance returns the persistent-memory prompt for delegated
// subagents. They can inspect the same merged memory index as the main agent,
// but memory writes and extraction tracking remain owned by the parent run.
func ReadOnlyMemoryGuidance(userDirRel, projectDirRel string) string {
	var b strings.Builder
	b.WriteString("Persistent memory is read-only for this run. It is stored as typed Markdown files in two directories (paths relative to the working directory):\n")
	fmt.Fprintf(&b, "- user-global (type `user`): %s\n", userDirRel)
	fmt.Fprintf(&b, "- project (types `project`, `feedback`, `reference`): %s\n", projectDirRel)
	b.WriteString("\n")
	b.WriteString("Using memory:\n")
	b.WriteString("- The index above lists every memory by description. When one is relevant, read its full content with read_file using the directory above plus the file name.\n")
	b.WriteString("- Do not create, update, delete, or otherwise persist memory files from this subagent run; report useful durable findings to the parent agent instead.\n")
	b.WriteString("\n")
	b.WriteString("Read-only memory rules:\n")
	b.WriteString("- Memories are possibly stale: they reflect what was true when written. Verify a memory still holds before relying on it.\n")
	b.WriteString("- Before recommending a file, function, or flag named in a memory, confirm it still exists in the current code.\n")
	b.WriteString("- When the user says to ignore memory for a request, proceed as if the index were empty: do not apply, cite, or compare against remembered facts for that request.")
	return b.String()
}

// ExtractionInstructions returns the system instructions for the isolated
// extraction pass: its task framing, the manifest of existing memories for
// dedup (REQ-012), the memory directories, the frontmatter format, and the same
// shared guardrails as the main agent.
func ExtractionInstructions(manifest, userDirRel, projectDirRel string) string {
	var b strings.Builder
	b.WriteString("You are the memory extraction pass. Review the conversation provided below and persist any durable memory the main agent did not already save. Write nothing to the user; your only output is memory files.\n\n")
	fmt.Fprintf(&b, "Memory directories (relative to the working directory):\n- user-global (type `user`): %s\n- project (types `project`, `feedback`, `reference`): %s\n\n", userDirRel, projectDirRel)
	if strings.TrimSpace(manifest) != "" {
		b.WriteString("Existing memories (update these instead of duplicating):\n")
		b.WriteString(manifest)
		b.WriteString("\n\n")
	} else {
		b.WriteString("There are no existing memories yet.\n\n")
	}
	b.WriteString(frontmatterTemplate())
	b.WriteString("\n\n")
	b.WriteString(Guardrails())
	b.WriteString("\n\nWhen there is nothing worth saving, do not call any tools — simply stop.")
	return b.String()
}
