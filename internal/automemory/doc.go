// Package automemory implements the cross-session persistent memory layer for
// the foxharness agent.
//
// Memories are typed Markdown files with YAML frontmatter, stored in two tiers
// under the user home directory:
//
//   - user-global  ~/.foxharness/memory/                 (type "user")
//   - project      ~/.foxharness/projects/{key}/memory/  (types "project", "feedback", "reference")
//
// Each scope owns a system-generated MEMORY.md index of one-line pointers to its
// memory files. The index is always regenerated from the files on disk, so it can
// never drift from the actual memories. The merged two-tier index is injected into
// the agent's system prompt every turn; the agent expands a specific memory's full
// content on demand via read_file.
//
// Memories are written by two complementary layers that share one set of
// lifecycle guardrails: the main agent writes inline with the existing file
// tools, and an asynchronous, context-isolated post-run extraction hook backstops
// signals the main agent missed.
package automemory
