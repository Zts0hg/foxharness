# Feature: Checkpoint/Rewind System

## Overview

Implement a full-featured checkpoint/rewind system for foxharness-go that enables users to restore code and conversation to any previous point in an agent session. The system combines an independent file backup mechanism with message history truncation, allowing granular recovery from unwanted agent actions.

## Goals

- Allow users to undo agent file modifications by restoring to any prior checkpoint
- Allow users to rewind conversation history to re-prompt from a previous point
- Provide an interactive TUI message selector for choosing rewind targets
- Persist checkpoint data so it survives session resume (`--resume`)
- Auto-restore on Ctrl+C when no meaningful work was produced after the last user message

## User Stories

### Story 1: Manual Rewind via Slash Command

**As a** user running the TUI agent
**I want** to type `/rewind` (or `/checkpoint`) and select a previous message to restore to
**So that** I can undo unwanted code changes and continue from an earlier point

**Acceptance Criteria:**
- [ ] `/rewind` opens an interactive message selector showing all selectable user messages
- [ ] `/checkpoint` is an alias that behaves identically to `/rewind`
- [ ] The selector displays each message with its text (truncated) and timestamp
- [ ] A virtual "current position" entry appears at the bottom of the list
- [ ] Selecting a message shows diff statistics before presenting restore options
- [ ] User can cancel the rewind at any point

### Story 2: Restore Code and Conversation

**As a** user who selected a rewind target
**I want** to choose what to restore (code only, conversation only, or both)
**So that** I have fine-grained control over what gets reverted

**Acceptance Criteria:**
- [ ] "Restore code and conversation" is the default option
- [ ] "Restore conversation only" truncates message history but leaves files as-is
- [ ] "Restore code only" restores files from backups but leaves message history intact
- [ ] "Cancel" returns to the message list or closes the selector
- [ ] After restoration, the TUI reflects the new state immediately

### Story 3: Automatic File Backups

**As a** user interacting with the agent
**I want** file modifications to be automatically backed up before they happen
**So that** every change is reversible without explicit action on my part

**Acceptance Criteria:**
- [ ] Before `write_file` executes, the current file content is backed up
- [ ] Before `edit_file` executes, the current file content is backed up
- [ ] Files that don't yet exist are recorded as null backups (deletion markers)
- [ ] Backups preserve file permissions
- [ ] Backup failures do not block tool execution (logged as warnings)

### Story 4: Auto-Restore on Cancel

**As a** user who pressed Ctrl+C to cancel a running turn
**I want** the session to automatically rewind to my last message if no meaningful work was done
**So that** I don't have to manually clean up after a cancelled turn

**Acceptance Criteria:**
- [ ] When Ctrl+C cancels a turn, check if all messages after the last user message are synthetic/empty
- [ ] If only synthetic content exists, auto-restore conversation to the last user message
- [ ] If any meaningful assistant content (text or tool results) exists, do not auto-restore
- [ ] The user's original input text is restored to the input field
- [ ] Auto-restore only restores conversation, not code — because if files were modified, tool calls must have executed, producing meaningful content that prevents auto-restore from triggering in the first place

### Story 5: Cross-Session Persistence

**As a** user who resumes a previous session via `--resume`
**I want** all checkpoints from the previous session to still be available
**So that** I can rewind even after restarting the agent

**Acceptance Criteria:**
- [ ] Checkpoint metadata is persisted to the session's JSONL transcript
- [ ] When resuming a session, checkpoint state is rebuilt from the persisted log
- [ ] Each session's checkpoint history is isolated — no cross-session access

## Functional Requirements

### FR-001: File Backup System

A new package `internal/checkpoint/` shall provide the core backup and snapshot functionality.

**Data Structures:**

```
FileHistoryState:
  snapshots: []FileHistorySnapshot     // Ordered list of snapshots (max 100)
  trackedFiles: map[string]bool        // Set of all ever-tracked file paths
  snapshotSequence: int                // Monotonically increasing counter

FileHistorySnapshot:
  messageID: string                    // Associated user message ID
  trackedFileBackups: map[string]FileHistoryBackup  // file path → backup info
  timestamp: time.Time

FileHistoryBackup:
  backupFileName: string               // Backup file name (empty string = file didn't exist at this version; single canonical null indicator)
  version: int                         // Version number (starts at 1)
  backupTime: time.Time
```

**Backup Storage:**

```
~/.foxharness/projects/{encoded-workdir}/sessions/{session-id}/checkpoints/
  ├── {sha256hash}@v1                  // File version 1 backup
  ├── {sha256hash}@v2                  // File version 2 backup
  └── ...
```

- Backup file name: first 16 hex characters of SHA256(file path) + `@v` + version number
- Backups created via `io.Copy` preserving file permissions
- Null backups (file didn't exist): `backupFileName` is empty string

**Backup Creation:**

- `TrackEdit(state, filePath, messageID)` — called before each file-modifying tool execution
  - If file is already tracked in the latest snapshot, skip (v1 backup is immutable)
  - Otherwise, create backup of current file content as v1
  - Update latest snapshot's trackedFileBackups

**Snapshot Creation:**

- `MakeSnapshot(state, messageID)` — called at each user message boundary
  - For each tracked file, check if content changed since last backup
  - Changed files: create new backup version
  - Unchanged files: reuse previous backup reference
  - Create new FileHistorySnapshot and append to state
  - If snapshots exceed 100, evict oldest (FIFO)
  - Increment snapshotSequence

**Change Detection (4 levels):**

1. Existence check: one side missing → changed
2. Stat comparison: file mode or size differs → changed
3. Mtime fast path: if file mtime < backup time → unchanged (skip content comparison)
4. Content comparison: read and compare bytes

### FR-002: Snapshot Persistence

- `RecordSnapshot(messageID, snapshot, isUpdate)` — persist snapshot to session transcript
- `RestoreStateFromLog(snapshots)` — rebuild FileHistoryState from persisted snapshot log

### FR-003: Code Restoration

- `Rewind(state, messageID) -> filesChanged []string` — restore files to snapshot state
  - Find target snapshot by messageID
  - For each tracked file:
    - Snapshot has record: use recorded backup
    - Snapshot has no record (file tracked later): look for earliest v1 backup
    - No backup found: skip file
    - Backup is null (file didn't exist): delete the file
    - Backup exists but content unchanged: skip
    - Backup exists and content changed: restore from backup
  - Does NOT modify FileHistoryState (pure filesystem operation)

- `GetDiffStats(state, messageID) -> DiffStats` — preview changes before restore
  - Returns: files changed count, insertion count, deletion count
  - Used by message selector to display diff summary

- `HasAnyChanges(state, messageID) -> bool` — lightweight change check
  - Returns true if any file differs from the snapshot
  - Used for auto-restore decision

### FR-004: Conversation Restoration

- Truncate message history at the target user message index
- Restore the user's original input text to the TUI input field
- Clear any pending tool results or assistant partial responses
- Does NOT reset session ID (foxharness-go has no server-side session correlation)

### FR-005: TUI Message Selector

A Bubble Tea sub-model (`internal/tui/selector/`) that:

1. **Message List View:**
   - Displays selectable user messages from the session history
   - Each entry shows: truncated message text, timestamp
   - A virtual "current position" entry at the bottom
   - Filter: exclude non-selectable messages (see FR-007 message classification for definitions)
   - Navigate with up/down arrows or j/k
   - Select with Enter, cancel with Escape/q

2. **Diff Preview View (after selecting a message):**
   - Shows diff statistics: N files changed, M insertions, D deletions
   - Lists changed file paths
   - Presents restore options:
     - Restore code and conversation (default)
     - Restore conversation only
     - Restore code only
     - Cancel

3. **Restoration Execution:**
   - Code restore: calls `checkpoint.Rewind()`
   - Conversation restore: truncates message history, restores input
   - Both: executes code restore first, then conversation restore
   - Returns control to main TUI model after completion

### FR-006: Slash Commands

- `/rewind` — primary command, opens message selector
- `/checkpoint` — alias for `/rewind`
- Both commands only available in interactive TUI mode
- Disabled while a run is actively executing

### FR-007: Auto-Restore on Cancel

When the user presses Ctrl+C to cancel a running turn:

1. Find the last selectable user message
2. Check if all messages after that point are synthetic (see definitions below)
3. If only synthetic messages follow the last user message:
   - Auto-restore conversation only (truncate message history to last user message)
   - Restore input text to input field
   - Code restore is unnecessary: if files were modified, tool calls must have executed, producing meaningful assistant content that would have prevented this auto-restore from triggering
4. If any meaningful content exists after the last user message:
   - Do not auto-restore; let the user decide via `/rewind`

**Message classification definitions (aligned with foxharness-go message types):**

A message is considered **synthetic** (non-meaningful) if any of:
- It is a progress/status message (`type === "progress"`)
- It is a system message (`type === "system"`)
- It is a tool result message with no text content (empty output)
- It is a meta user message: a user message programmatically injected by the engine (e.g., auto-generated system prompts, system reminders, context compaction summaries, or other non-human-authored messages). These carry an `isMeta` flag set to `true`.
- It is a compact summary message (`isCompactSummary === true`)
- It is a transcript-only message (`isVisibleInTranscriptOnly === true`)

A message has **meaningful content** if:
- It is an assistant message containing non-empty text content OR any `tool_use` blocks
- It is a tool result message with non-empty output (indicating a tool was actually executed)

### FR-008: Engine Integration

**Track Edit Hook:**
- Before `write_file`, `edit_file` tool execution: call `TrackEdit()`
- Bash tool is NOT tracked — bash file modifications are not detectable with sufficient reliability
- Hook implemented via the existing middleware `BeforeExecute` mechanism

**Snapshot Creation Hook:**
- At the start of processing each user message: call `MakeSnapshot()`
- Hook implemented in the engine loop, after loading context and before model call

## Non-Functional Requirements

### NFR-001: Performance

- File backup creation must not add more than 50ms latency per tool call
- Snapshot creation for < 10 tracked files must complete within 200ms
- Change detection (mtime fast path) must skip content comparison in > 90% of cases

### NFR-002: Reliability

- Backup failures must not block tool execution (log warning, continue)
- Corrupt or missing backup files must be handled gracefully (skip affected file, log error)
- Snapshot persistence must be atomic (write to temp file, then rename)

### NFR-003: Storage

- Maximum 100 snapshots per session (FIFO eviction)
- Backup directory size should not exceed 500MB per session (soft limit, log warning)
- Old backup files from evicted snapshots are NOT deleted (conservative approach)

### NFR-004: Security

- Backup file permissions must match original file permissions
- Backup files must not be accessible outside the session directory
- File path hashing prevents exposure of directory structure in backup names

### NFR-005: Compatibility

- Feature must be enabled by default
- Must be disableable via config setting or environment variable `FOXHARNESS_DISABLE_FILE_CHECKPOINTING`
- Must work correctly with existing session resume functionality

## Acceptance Criteria (Test Cases)

### TC-001: Basic File Backup
- Agent modifies `main.go` via `write_file`
- Verify backup exists in session checkpoints directory
- Verify backup content matches pre-edit file content

### TC-002: Snapshot Creation at User Message
- User sends two messages, agent modifies files after each
- Verify two snapshots exist, each with correct messageID association
- Verify each snapshot contains the correct file versions

### TC-003: Code Restore via Rewind
- Agent modifies 3 files across 2 turns
- User rewinds to first message
- Verify all 3 files restored to their state at that snapshot
- Verify backup files are not deleted

### TC-004: Conversation Restore
- User sends 3 messages, agent responds to each
- User rewinds to message 2
- Verify message history truncated at message 2
- Verify input field contains message 2's text

### TC-005: Combined Restore
- Agent modifies files and conversation progresses
- User selects "Restore code and conversation"
- Verify both file restoration and conversation truncation occur

### TC-006: Null Backup (New File)
- Agent creates a new file `new.go` via `write_file`
- Verify null backup recorded for the file (file didn't exist before)
- User rewinds to before file creation
- Verify `new.go` is deleted

### TC-007: Auto-Restore on Cancel
- User sends a message, agent starts but user presses Ctrl+C before any tool execution
- Verify conversation auto-restores to last user message
- Verify input field contains the cancelled message text

### TC-008: No Auto-Restore with Meaningful Content
- User sends a message, agent modifies 2 files, then user presses Ctrl+C
- Verify NO auto-restore occurs (meaningful content exists)

### TC-009: Snapshot FIFO Eviction
- Create 101 user messages with snapshots
- Verify only 100 snapshots remain
- Verify oldest snapshot was evicted

### TC-010: Cross-Session Persistence
- Session 1: agent modifies files, creates snapshots, session ends
- Session 2: resume session 1 via `--resume`
- Verify all snapshots from session 1 are available
- Verify `/rewind` can restore to any previous checkpoint

### TC-011: Diff Statistics Preview
- Agent modifies 3 files with known changes
- User opens `/rewind` and selects a message
- Verify diff stats show correct file count and line changes

### TC-012: File Permission Preservation
- Agent modifies a file with mode 0755
- Verify backup has same mode
- After restore, verify restored file has mode 0755

### TC-013: Disabled Checkpointing
- Set `FOXHARNESS_DISABLE_FILE_CHECKPOINTING=1`
- Verify no backups are created
- Verify `/rewind` shows conversation-only restore option

### TC-014: Slash Command Registration
- Verify `/rewind` is registered as a slash command in the TUI
- Verify `/checkpoint` is registered as an alias and behaves identically to `/rewind`
- Verify both commands are disabled while a run is actively executing
- Verify both commands are only available in interactive TUI mode (not in `exec` mode)

## Edge Cases

- **Large files**: Files exceeding available memory must be handled via streaming copy, not reading entire content into memory
- **Binary files**: Backup and restore must handle binary content correctly (no text encoding issues)
- **Concurrent tool execution**: Multiple parallel-safe tools modifying different files must each create independent backups without conflict
- **Missing backup on restore**: If a backup file is missing or corrupt, skip that file and log an error rather than failing the entire restore
- **Symlinks**: Follow symlinks when backing up; restore the target file content
- **Empty files**: Back up empty files as empty files (not null backups)
- **Deleted files after backup**: If a tracked file is deleted externally between backup and snapshot, record null backup
- **Session directory missing**: If checkpoints directory is removed externally, recreate it lazily on next backup
- **Snapshot with no tracked files**: First snapshot may have empty trackedFileBackups — handle gracefully
- **Rewind during active run**: `/rewind` must be disabled while a run is executing

## Out of Scope

- Summarize/compact conversation from a checkpoint (separate feature)
- Hard link optimization for backup migration (file copy is sufficient)
- Backup migration across session directories (foxharness-go reuses the same session ID and directory on resume; no migration needed)
- Multi-threaded race condition handling for backup writes (TUI is single-threaded)
- Bash tool file modification tracking (bash commands can modify files in unpredictable ways; only `write_file` and `edit_file` are tracked)
- Git integration (checkpoints are independent of version control)
- Backup garbage collection (old backup files from evicted snapshots are not deleted)
- Network file system considerations
- Encryption of backup files
- Backup integrity checksums (beyond file permission/size/mtime checks)
- CLI `--rewind-files` flag (users should use `--resume` to restore a session, then use `/rewind` or `/checkpoint` interactively)

## Reference

This specification is based on the Claude Code `/rewind` feature analysis document at:
`/Users/xiaoming/code/000 blogs/claude-code-analysis/claude-code-rewind-checkpoint-implementation.md`

## Clarifications

### Session 2026-05-21

**Q1**: Should we include the `--rewind-files` CLI flag for non-interactive rewind?
**A1**: No. The correct flow is to use `--resume` to restore a session, then use `/rewind` or `/checkpoint` interactively in the TUI to rewind to a specific checkpoint.
**Impact**: Removed FR-009, Story 6, TC-011. Added `--rewind-files` to Out of Scope.

**Q2**: Is `CopyBackupsForResume` needed for session resume?
**A2**: No. Verified against Claude Code source: normal `--resume` preserves the original session ID, so no backup migration occurs. foxharness-go also reuses the same session directory on resume (`manager.Latest()` returns the existing session). `CopyBackupsForResume` only exists in Claude Code for `--fork-session`, which foxharness-go does not have.
**Impact**: Removed `CopyBackupsForResume` from FR-002, removed "backup files are migrated" AC from Story 5, added backup migration to Out of Scope.
