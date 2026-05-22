# Implementation Plan: Checkpoint/Rewind System

## 1. Tech Stack

| Category | Technology | Version | Notes |
|----------|------------|---------|-------|
| Language | Go | ≥ 1.22 | Existing project constraint |
| TUI Framework | Bubble Tea | current | Existing `github.com/charmbracelet/bubbletea` |
| TUI Styling | Lipgloss | current | Existing `github.com/charmbracelet/lipgloss` |
| Diff Computation | `github.com/sergi/go-diff/diffmatchpatch` | latest | Line-level diff for GetDiffStats |
| Testing | Go standard `testing` | stdlib | Table-driven tests per constitution |
| Storage | Filesystem (JSONL) | N/A | Reuse existing session directory layout |

## 2. Constitutionality Review

| Principle | Compliance | Notes |
|-----------|------------|-------|
| 1. TDD | ✅ | Each phase starts with Red phase; test files listed per module |
| 2. Code Quality | ✅ | `Checkpointer` interface for DI; filesystem ops behind `FS` interface for testability; single-purpose functions |
| 3. Go Documentation | ✅ | `doc.go` for `internal/checkpoint/`; block comments on all exported identifiers |
| 4. Testing Standards | ✅ | Test files mirror package structure; table-driven tests for multi-scenario coverage |
| 5. Architecture | ✅ | `internal/checkpoint/` owns checkpoint domain; `internal/tui/selector/` owns UI; no cross-concern leaking |
| 6. Performance | ✅ | 4-level change detection with mtime fast path; streaming copy for large files |
| 7. Security | ✅ | SHA-256 file path hashing; permission-preserving copies; backup isolation within session directory |

## 3. Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                         cmd/fox (main)                           │
└───────────────────────────┬──────────────────────────────────────┘
                            │ creates
                            ▼
┌──────────────────────────────────────────────────────────────────┐
│                      internal/app                                │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────────────────┐   │
│  │ runner.go│  │  tui.go  │  │         cli.go               │   │
│  └────┬─────┘  └────┬─────┘  └──────────────────────────────┘   │
│       │              │                                             │
│       │ wires        │ creates Model                              │
│       ▼              ▼                                             │
└───────┬──────────────┬───────────────────────────────────────────┘
        │              │
        ▼              ▼
┌──────────────┐  ┌──────────────────────────────────────────────┐
│   Engine     │  │              TUI Model                       │
│  (loop.go)   │  │  ┌────────────────────────────────────────┐  │
│              │  │  │  /rewind → selector sub-model           │  │
│  ┌────────┐  │  │  │  ┌─────────────────┐                   │  │
│  │snapshot│  │  │  │  │ Message List     │                   │  │
│  │  hook  │  │  │  │  │ Diff Preview     │                   │  │
│  └───┬────┘  │  │  │  │ Restore Options  │                   │  │
│      │       │  │  │  └─────────────────┘                   │  │
│  ┌───▼────┐  │  │  └────────────────────────────────────────┘  │
│  │execute │──┼──┤  Ctrl+C → auto-restore check                 │
│  │ tools  │  │  │                                              │
│  └───┬────┘  │  └──────────────────────────────────────────────┘
│      │       │
└──────┼───────┘
       │ BeforeExecute
       ▼
┌──────────────────┐
│   Middleware     │
│  (checkpoint.go) │──── TrackEdit() before write_file/edit_file
└──────┬───────────┘
       │
       ▼
┌──────────────────────────────────────────────────────────────────┐
│                   internal/checkpoint (NEW)                      │
│                                                                  │
│  ┌─────────┐ ┌──────────┐ ┌───────────┐ ┌────────────────────┐ │
│  │ state.go│ │ backup.go│ │snapshot.go│ │     restore.go     │ │
│  │         │ │          │ │           │ │  Rewind()          │ │
│  │ Data    │ │TrackEdit │ │MakeSnap-  │ │  GetDiffStats()    │ │
│  │ structs │ │createBkp │ │shot()     │ │  HasAnyChanges()   │ │
│  └─────────┘ └──────────┘ └───────────┘ └────────────────────┘ │
│  ┌─────────────┐ ┌──────────────┐ ┌──────────────────────────┐ │
│  │ persist.go  │ │ classify.go  │ │   checkpoint.go          │ │
│  │ RecordSnap- │ │ IsSynthetic()│ │   Checkpointer interface │ │
│  │ shot()      │ │ IsMeaningful │ │   New() constructor      │ │
│  │ RestoreFrom │ │              │ │   SetDisabled()           │ │
│  │ Log()       │ │              │ │                           │ │
│  └─────────────┘ └──────────────┘ └──────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
        │
        ▼
┌──────────────────────────────────────────────────────────────────┐
│                        Filesystem                                │
│  ~/.foxharness/projects/{workdir}/sessions/{id}/                │
│    checkpoints/          checkpoints.jsonl                       │
│      {hash}@v1           {"seq":1,"message_id":"3",             │
│      {hash}@v2            "snapshot":{...}}                      │
└──────────────────────────────────────────────────────────────────┘
```

## 4. Component Structure

### New Files

```
internal/checkpoint/
├── doc.go                    // Package godoc
├── checkpoint.go             // Checkpointer interface, Config, New() constructor
├── state.go                  // FileHistoryState, FileHistorySnapshot, FileHistoryBackup, DiffStats
├── backup.go                 // TrackEdit, createBackup, backupFileName (SHA256 hash)
├── snapshot.go               // MakeSnapshot, change detection (4 levels)
├── restore.go                // Rewind, GetDiffStats, HasAnyChanges
├── persist.go                // RecordSnapshot, RestoreStateFromLog
├── classify.go               // IsSynthetic, IsMeaningful, SelectableMessages
├── fs.go                     // FS interface for testability
├── checkpoint_test.go        // Integration tests
├── backup_test.go            // TC-001, TC-006, TC-012, TC-013
├── snapshot_test.go          // TC-002, TC-009
├── restore_test.go           // TC-003, TC-005, TC-011
├── persist_test.go           // TC-010
├── classify_test.go          // Message classification tests

internal/tui/selector/
├── model.go                  // Bubble Tea model (states: list, preview, confirm)
├── view.go                   // View rendering
├── keys.go                   // Key bindings
├── types.go                  // SelectableMessage, RestoreAction, ResultMsg
```

### Modified Files

```
internal/tui/model.go         // Add /rewind, /checkpoint commands; auto-restore on Ctrl+C
internal/engine/loop.go       // Snapshot hook at user message boundary
internal/middleware/           // New checkpoint middleware
internal/app/runner.go        // Wire Checkpointer into engine and TUI
internal/app/tui.go           // Pass Checkpointer to TUI Model
internal/session/session.go   // Expose session directory for checkpoint storage
```

## 5. Module Dependency Graph

```
cmd/fox
  └── internal/app
        ├── internal/session
        │     └── internal/schema
        ├── internal/engine
        │     ├── internal/checkpoint  (MakeSnapshot)
        │     │   └── internal/session
        │     │         └── internal/schema
        │     ├── internal/session
        │     ├── internal/tools
        │     └── internal/middleware
        │           └── internal/checkpoint  (TrackEdit via middleware)
        └── internal/tui
              ├── internal/tui/selector
              │     └── internal/checkpoint  (Rewind, GetDiffStats)
              └── internal/checkpoint  (IsSynthetic, auto-restore)
```

**Key rule**: `internal/checkpoint` depends only on `internal/session` (MessageRecord type, session directory paths) and `internal/schema` (message role types). It has NO dependency on `internal/engine`, `internal/tui`, or `internal/middleware`.

## 6. Module Specifications

### Module: `internal/checkpoint`

- **Responsibility**: Core checkpoint/rewind domain logic — file backup, snapshot management, code restoration, message classification, and persistence
- **Dependencies**: `internal/schema` (message types), `internal/session` (session directory paths), `crypto/sha256`, `encoding/hex`, `io`, `os`
- **Interface**: `Checkpointer` interface consumed by engine, middleware, and TUI

```go
// Checkpointer provides checkpoint and rewind functionality for a session.
type Checkpointer interface {
    TrackEdit(filePath, messageID string) error
    MakeSnapshot(messageID string) error
    Rewind(messageID string) ([]string, error)
    GetDiffStats(messageID string) (*DiffStats, error)
    HasAnyChanges(messageID string) (bool, error)
    SetDisabled(disabled bool)
    IsDisabled() bool
    RestoreStateFromLog() error
}

// Config holds checkpoint configuration.
type Config struct {
    SessionDir    string    // ~/.foxharness/projects/{workdir}/sessions/{id}
    MaxSnapshots  int       // Default: 100
    FS            FS        // Filesystem abstraction (real FS in prod, memory FS in tests)
}
```

### Module: `internal/checkpoint/fs.go`

- **Responsibility**: Filesystem abstraction for testability (constitution principle 2 — dependencies MUST be injectable)

```go
// FS abstracts filesystem operations for checkpoint testing.
type FS interface {
    Stat(name string) (os.FileInfo, error)
    ReadFile(name string) ([]byte, error)
    WriteFile(name string, data []byte, perm os.FileMode) error
    CopyFile(dstPath, srcPath string, perm os.FileMode) error
    Remove(name string) error
    MkdirAll(path string, perm os.FileMode) error
    Open(name string) (fs.File, error)
    OpenFile(name string, flag int, perm os.FileMode) (File, error)
}
```

### Module: `internal/checkpoint/classify.go`

- **Responsibility**: Message classification for FR-007 (synthetic vs meaningful) and FR-005 (selectable messages)

```go
// IsSynthetic returns true if a message is non-meaningful (progress, system,
// empty tool result, meta, compact summary, or transcript-only).
func IsSynthetic(rec session.MessageRecord) bool

// IsMeaningful returns true if a message contains real assistant content.
func IsMeaningful(rec session.MessageRecord) bool

// SelectableMessages filters message records to only user messages
// suitable as rewind targets.
func SelectableMessages(records []session.MessageRecord) []SelectableMessage

type SelectableMessage struct {
    Seq       int64
    Content   string
    Timestamp time.Time
    IsCurrent bool
}
```

### Module: `internal/tui/selector`

- **Responsibility**: Bubble Tea sub-model for interactive message selection, diff preview, and restore option choice
- **Dependencies**: `internal/checkpoint` (GetDiffStats, Rewind), `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`
- **Interface**: Returns `ResultMsg` to parent model

```go
// Model is the selector sub-model with three view states.
type Model struct {
    state       ViewState    // listView, previewView, confirmView
    messages    []SelectableMessage
    cursor      int
    diffStats   *checkpoint.DiffStats
    selected    SelectableMessage
    checkpointer checkpoint.Checkpointer
}

type ViewState int

const (
    listView      ViewState = iota
    previewView
)

// ResultMsg is sent to the parent TUI model when the selector completes.
type ResultMsg struct {
    Action    RestoreAction
    MessageID string
}

type RestoreAction int

const (
    ActionNone                RestoreAction = iota
    ActionRestoreBoth
    ActionRestoreConversation
    ActionRestoreCode
    ActionCancelled
)
```

### Module: `internal/middleware/checkpoint.go` (NEW)

- **Responsibility**: BeforeExecute middleware that calls `TrackEdit()` before `write_file` and `edit_file` tool execution
- **Dependencies**: `internal/checkpoint`, `internal/middleware`

```go
type checkpointMiddleware struct {
    checkpointer checkpoint.Checkpointer
    getMessageID func() string
}

func NewCheckpointMiddleware(cp checkpoint.Checkpointer, getMessageID func() string) middleware.Middleware
```

### Module changes: `internal/engine/loop.go`

- **Changes**: Add `checkpointer` field to `AgentEngine`; call `MakeSnapshot()` after appending user message to MessageLog, before `callModel()`

### Module changes: `internal/tui/model.go`

- **Changes**:
  - Add `checkpointer checkpoint.Checkpointer` field
  - Add `/rewind` and `/checkpoint` to `slashCommands` slice and `handleSlashCommand` switch
  - Modify Ctrl+C handler: after cancel, check if auto-restore should trigger
  - Add selector sub-model state management
  - Handle `selector.ResultMsg` to execute restore

### Module changes: `internal/app/runner.go`

- **Changes**: Create `checkpoint.New()` with session directory; pass to engine config and TUI model

## 7. Data Models

### FileHistoryState

```go
type FileHistoryState struct {
    Snapshots        []FileHistorySnapshot
    TrackedFiles     map[string]bool
    SnapshotSequence int
}
```

### FileHistorySnapshot

```go
type FileHistorySnapshot struct {
    MessageID          string
    TrackedFileBackups map[string]FileHistoryBackup
    Timestamp          time.Time
}
```

### FileHistoryBackup

```go
type FileHistoryBackup struct {
    BackupFileName string    // Empty = null backup (file didn't exist)
    Version        int
    BackupTime     time.Time
}
```

### DiffStats

```go
type DiffStats struct {
    FilesChanged int
    Insertions   int
    Deletions    int
    ChangedFiles []string
}
```

### SnapshotRecord (persistence format)

```go
// Written to checkpoints.jsonl, one JSON object per line.
type SnapshotRecord struct {
    Seq      int64               `json:"seq"`
    Action   string              `json:"action"`   // "snapshot" or "snapshot_update"
    Snapshot FileHistorySnapshot `json:"snapshot"`
}
```

### Storage Layout

```
~/.foxharness/projects/{encoded-workdir}/sessions/{session-id}/
├── session.json
├── messages.jsonl
├── transcript.jsonl
├── working_memory.md
├── checkpoints.jsonl          // NEW: snapshot records
└── checkpoints/               // NEW: backup file storage
    ├── a1b2c3d4e5f67890@v1
    ├── a1b2c3d4e5f67890@v2
    └── f9e8d7c6b5a43210@v1
```

## 8. Implementation Phases

### Phase 1: Core Checkpoint Package (TDD)

**Deliverables**: `internal/checkpoint/` with data structures, backup, change detection, and tests.

**TDD Cycle**: Each function starts with a failing test, then minimal implementation, then refactor.

**Tasks**:

- [ ] **1.1** Create `internal/checkpoint/doc.go` with package documentation
- [ ] **1.2** Create `internal/checkpoint/state.go` — data structures (FileHistoryState, FileHistorySnapshot, FileHistoryBackup, DiffStats)
- [ ] **1.3** Create `internal/checkpoint/fs.go` — FS interface and `osFS` real implementation
- [ ] **1.4** RED: Write `TestCreateBackup` — verify backup file created with correct content (TC-001)
- [ ] **1.5** GREEN: Implement `createBackup()` in `internal/checkpoint/backup.go` — io.Copy preserving permissions
- [ ] **1.6** RED: Write `TestBackupFileName` — verify SHA-256 hash naming format `{16hex}@v{N}`
- [ ] **1.7** GREEN: Implement `backupFileName()` — SHA-256 of file path, first 16 hex chars
- [ ] **1.8** RED: Write `TestTrackEdit` — verify file tracked and backup created before modification (TC-001)
- [ ] **1.9** GREEN: Implement `TrackEdit()` — skip if already tracked, otherwise create v1 backup
- [ ] **1.10** RED: Write `TestTrackEditNullBackup` — verify null backup for non-existent file (TC-006)
- [ ] **1.11** GREEN: Handle file-not-exists in `TrackEdit()` — set `BackupFileName` to empty string
- [ ] **1.12** RED: Write `TestChangeDetection` — verify 4-level comparison logic
- [ ] **1.13** GREEN: Implement `fileChanged()` in `internal/checkpoint/snapshot.go` — existence → stat → mtime → content
- [ ] **1.14** RED: Write `TestBackupPreservesPermissions` — verify file mode preserved (TC-012)
- [ ] **1.15** GREEN: Use `io.Copy` with `os.File` to preserve mode via `os.Chmod`

**Test Coverage**: TC-001, TC-006, TC-012

### Phase 2: Snapshot & Restoration (TDD)

**Deliverables**: Snapshot creation, code restoration, diff statistics.

**Tasks**:

- [ ] **2.1** RED: Write `TestMakeSnapshot` — verify snapshot created with correct messageID and file versions (TC-002)
- [ ] **2.2** GREEN: Implement `MakeSnapshot()` — iterate tracked files, detect changes, create new backup versions, append snapshot
- [ ] **2.3** RED: Write `TestSnapshotFIFOEviction` — verify oldest snapshot evicted at 101 (TC-009)
- [ ] **2.4** GREEN: Add FIFO eviction logic to `MakeSnapshot()` when len > maxSnapshots
- [ ] **2.5** RED: Write `TestRewind` — verify files restored to snapshot state (TC-003)
- [ ] **2.6** GREEN: Implement `Rewind()` — find snapshot, iterate tracked files, restore or delete
- [ ] **2.7** RED: Write `TestRewindNullBackup` — verify file deleted when snapshot shows null (TC-006)
- [ ] **2.8** GREEN: Handle null backup in `Rewind()` — `os.Remove()` when BackupFileName is empty
- [ ] **2.9** RED: Write `TestGetDiffStats` — verify diff statistics computed correctly (TC-011)
- [ ] **2.10** GREEN: Implement `GetDiffStats()` — compare current files to snapshot, compute line diffs
- [ ] **2.11** RED: Write `TestHasAnyChanges` — verify lightweight change detection
- [ ] **2.12** GREEN: Implement `HasAnyChanges()` — short-circuit on first changed file
- [ ] **2.13** RED: Write `TestCombinedRestore` — verify code + conversation restore (TC-005)
- [ ] **2.14** GREEN: Ensure `Rewind()` is a pure filesystem operation (does not modify state)
- [ ] **2.15** REFACTOR: Review all Phase 1–2 code for readability, remove duplication

**Test Coverage**: TC-002, TC-003, TC-005, TC-006, TC-009, TC-011

### Phase 3: Persistence (TDD)

**Deliverables**: Snapshot persistence to JSONL, state rebuild on resume.

**Tasks**:

- [ ] **3.1** RED: Write `TestRecordSnapshot` — verify snapshot written to checkpoints.jsonl
- [ ] **3.2** GREEN: Implement `RecordSnapshot()` — append JSON line to checkpoints.jsonl (atomic: write temp, rename)
- [ ] **3.3** RED: Write `TestRestoreStateFromLog` — verify state rebuilt from persisted snapshots (TC-010)
- [ ] **3.4** GREEN: Implement `RestoreStateFromLog()` — read checkpoints.jsonl, replay into FileHistoryState
- [ ] **3.5** RED: Write `TestAtomicPersistence` — verify temp-file-then-rename pattern
- [ ] **3.6** GREEN: Ensure `RecordSnapshot` uses `os.Rename` for atomicity
- [ ] **3.7** RED: Write `TestCorruptSnapshotLog` — verify graceful handling of corrupt JSONL entries
- [ ] **3.8** GREEN: Skip corrupt entries with error logging in `RestoreStateFromLog()`

**Test Coverage**: TC-010

> **Note**: No backup file migration is needed on resume. foxharness-go reuses the same session ID and directory when resuming (`manager.Latest()` returns the existing session), so backup files remain in their original location.

### Phase 4: Message Classification (TDD)

**Deliverables**: Synthetic/meaningful message detection, selectable message filtering.

**Tasks**:

- [ ] **4.1** RED: Write `TestIsSynthetic` — verify progress, system, empty tool results classified as synthetic
- [ ] **4.2** GREEN: Implement `IsSynthetic()` — check message kind and content per FR-007 definitions
- [ ] **4.3** RED: Write `TestIsMeaningful` — verify assistant text and non-empty tool results classified as meaningful
- [ ] **4.4** GREEN: Implement `IsMeaningful()` — check for non-empty text or tool_use blocks
- [ ] **4.5** RED: Write `TestSelectableMessages` — verify only user messages with non-meta content pass filter
- [ ] **4.6** GREEN: Implement `SelectableMessages()` — filter and format message list for selector
- [ ] **4.7** RED: Write `TestMessagesAfterAreOnlySynthetic` — verify auto-restore trigger logic
- [ ] **4.8** GREEN: Implement `MessagesAfterAreOnlySynthetic()` — iterate from index, return true if all synthetic

**Test Coverage**: TC-007 (logic), TC-008 (logic)

### Phase 5: Engine Integration

**Deliverables**: Middleware for TrackEdit, snapshot hook in engine loop, env toggle.

**Tasks**:

- [ ] **5.1** Create `internal/middleware/checkpoint.go` — `BeforeExecute` middleware calling `TrackEdit()`
- [ ] **5.2** Wire middleware into tool registry in `internal/app/runner.go`
- [ ] **5.3** Modify `internal/engine/loop.go` — add `checkpointer` field, call `MakeSnapshot()` after user message append
- [ ] **5.4** Modify `session.MessageLog.Append` to return the assigned `Seq` (needed as messageID)
- [ ] **5.5** Handle `FOXHARNESS_DISABLE_FILE_CHECKPOINTING` env var in runner initialization
- [ ] **5.6** RED: Write `TestCheckpointMiddleware` — verify TrackEdit called for write_file and edit_file, not for bash
- [ ] **5.7** GREEN: Implement middleware — check tool name, call TrackEdit only for tracked tools
- [ ] **5.8** RED: Write `TestDisabledCheckpointing` — verify no backups when disabled (TC-013)
- [ ] **5.9** GREEN: `Checkpointer.SetDisabled(true)` skips all operations

**Test Coverage**: TC-013

### Phase 6: TUI Selector & Slash Commands

**Deliverables**: Interactive message selector, `/rewind` and `/checkpoint` commands.

**Tasks**:

- [ ] **6.1** Create `internal/tui/selector/types.go` — SelectableMessage, RestoreAction, ResultMsg
- [ ] **6.2** Create `internal/tui/selector/keys.go` — key bindings (up/down/j/k, Enter, Esc/q)
- [ ] **6.3** Create `internal/tui/selector/model.go` — Bubble Tea model with list and preview states
- [ ] **6.4** Create `internal/tui/selector/view.go` — render message list, diff preview, restore options
- [ ] **6.5** Add `/rewind` and `/checkpoint` to `slashCommands` slice in `internal/tui/model.go`
- [ ] **6.6** Add `/rewind` and `/checkpoint` cases in `handleSlashCommand` — open selector sub-model
- [ ] **6.7** Handle `selector.ResultMsg` in main TUI model — execute rewind and/or conversation truncate
- [ ] **6.8** RED: Write `TestSlashCommands` — verify /rewind and /checkpoint registered and behave identically (TC-014)
- [ ] **6.9** GREEN: Both commands delegate to same handler
- [ ] **6.10** Disable `/rewind` and `/checkpoint` while run is active (check `m.running`)

**Test Coverage**: TC-004, TC-014

### Phase 7: Auto-Restore on Ctrl+C

**Deliverables**: Automatic conversation restore when no meaningful content produced after cancel.

**Tasks**:

- [ ] **7.1** Modify Ctrl+C handler in `internal/tui/model.go` — after `m.cancelRun()`, call auto-restore logic
- [ ] **7.2** RED: Write `TestAutoRestoreOnCancel` — verify conversation truncated when only synthetic content (TC-007)
- [ ] **7.3** GREEN: Implement auto-restore check — find last user message, check if all subsequent are synthetic
- [ ] **7.4** RED: Write `TestNoAutoRestoreWithMeaningfulContent` — verify no restore when tool results exist (TC-008)
- [ ] **7.5** GREEN: Return input text to input field on auto-restore
- [ ] **7.6** Implement conversation truncation — slice message history, restore input text

**Test Coverage**: TC-007, TC-008

### Phase 8: Cross-Session Integration

**Deliverables**: Checkpoint state survives session resume.

**Tasks**:

- [ ] **8.1** In `internal/app/runner.go`, call `checkpointer.RestoreStateFromLog()` on session resume
- [ ] **8.2** RED: Write `TestCrossSessionPersistence` — verify checkpoints available after resume (TC-010)
- [ ] **8.3** GREEN: Wire state restoration into app startup

**Test Coverage**: TC-010 (integration)

### Phase 9: Edge Cases & Benchmarks

**Deliverables**: Edge case handling, performance benchmarks.

**Tasks**:

- [ ] **9.1** Handle large files via streaming copy (io.Copy, not ReadFile + WriteFile)
- [ ] **9.2** Handle binary files correctly (no text encoding assumptions)
- [ ] **9.3** Handle missing backup on restore — skip file, log error, continue
- [ ] **9.4** Handle symlinks — follow symlinks with `os.Stat` (not `os.Lstat`)
- [ ] **9.5** Handle empty files — backup as empty file, not null backup
- [ ] **9.6** Handle deleted tracked file between backup and snapshot — record null backup
- [ ] **9.7** Handle missing checkpoints directory — recreate lazily
- [ ] **9.8** Handle first snapshot with no tracked files
- [ ] **9.9** Write benchmarks: `BenchmarkTrackEdit`, `BenchmarkMakeSnapshot`, `BenchmarkChangeDetection`

## 9. Technical Decisions

### Decision 1: Separate `checkpoints.jsonl` instead of using `transcript.jsonl`

- **Choice**: New `checkpoints.jsonl` file in session directory
- **Rationale**: Keeps checkpoint data isolated from general session events; makes `RestoreStateFromLog()` a simple file read without filtering; avoids bloating transcript with large snapshot payloads
- **Alternatives**: Reuse `transcript.jsonl` with new event types
- **Trade-offs**: One extra file per session (negligible), but cleaner separation of concerns

### Decision 2: `MessageRecord.Seq` (int64) as message identifier

- **Choice**: Use `Seq` converted to `string` as the messageID in snapshots
- **Rationale**: Seq is already unique and sequential within a session; no need to add UUID fields to MessageRecord; simplifies snapshot-to-message mapping
- **Alternatives**: Add UUID field to MessageRecord
- **Trade-offs**: Seq is not globally unique, but checkpoints are session-scoped so this doesn't matter

### Decision 3: FS interface for filesystem operations

- **Choice**: Define `FS` interface in `internal/checkpoint/fs.go` with `osFS` as production implementation
- **Rationale**: Constitution principle 2 (testability) — dependencies MUST be injectable; enables in-memory testing without temp directories; proven pattern from Go standard library (`fs.FS`)
- **Alternatives**: Use temp directories in tests; use `afero.Fs`
- **Trade-offs**: Small interface maintenance cost, but significant test quality improvement

### Decision 4: Bubble Tea sub-model pattern for selector

- **Choice**: `internal/tui/selector/` as independent Bubble Tea model, communicating via `ResultMsg`
- **Rationale**: Clean separation of concerns; selector has its own state machine (list → preview → confirm); standard Bubble Tea composition pattern
- **Alternatives**: Inline selector state into main TUI model
- **Trade-offs**: Slightly more indirection, but keeps main model manageable

### Decision 5: Middleware-based TrackEdit hook

- **Choice**: Implement `TrackEdit` as a `middleware.Middleware` that allows execution (never denies) but creates backup as side effect
- **Rationale**: Uses existing middleware infrastructure; no engine modifications needed for tool hooks; clean separation
- **Alternatives**: Add TrackEdit call directly in engine's `executeToolCalls()`
- **Trade-offs**: Middleware is conceptually for gatekeeping, but the `DecisionAllow` pattern works for side-effect-only middleware

### Decision 6: Single `backupFileName` as null indicator

- **Choice**: Empty string `""` in `BackupFileName` means file didn't exist (aligned with Claude Code's `null`)
- **Rationale**: Single source of truth; eliminates dual-indicator confusion; matches Claude Code's proven pattern
- **Alternatives**: Separate `fileExisted` boolean field
- **Trade-offs**: Requires checking string emptiness rather than boolean, but simpler data model

### Decision 7: External diff library for GetDiffStats

- **Choice**: `github.com/sergi/go-diff/diffmatchpatch` for line-level diff computation
- **Rationale**: Battle-tested diff library; provides insertion/deletion counts directly; avoids implementing diff algorithm from scratch
- **Alternatives**: Custom diff implementation; `github.com/go-git/go-git/v5/utils/diff`
- **Trade-offs**: One additional dependency, but significantly reduces implementation complexity

### Decision 8: Session directory resolution for checkpoint storage

- **Choice**: Use `session.Session.RootDir` + `/checkpoints/` and `/checkpoints.jsonl` for storage
- **Rationale**: Reuses existing session directory infrastructure; no new config needed; natural cleanup with session
- **Alternatives**: Separate checkpoint storage root
- **Trade-offs**: Checkpoint lifetime tied to session directory; acceptable since checkpoints are session-scoped

### Decision 9: No backup migration on session resume

- **Choice**: No `CopyBackupsForResume` function — backup files stay in the original session directory
- **Rationale**: foxharness-go's `manager.Latest()` returns the existing session with the same ID and `RootDir` on resume. Unlike Claude Code's `--fork-session` mode, the session directory never changes, so no migration is needed. Verified against Claude Code source: normal `--resume` also preserves session ID (`switchSession` with original ID); `copyFileHistoryForResume` has an early return when `previousSessionId === sessionId`.
- **Alternatives**: Implement migration for a hypothetical fork-session feature
- **Trade-offs**: If a fork-session feature is ever added, `CopyBackupsForResume` would need to be implemented at that time
