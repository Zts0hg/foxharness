# Tasks: Checkpoint/Rewind System

**Input**: `.codexspec/specs/2026-0521-1153h1-checkpoint-rewind/`
**Prerequisites**: plan.md, spec.md

**Tests**: Per constitution TDD mandate, every code component has a RED test task preceding its GREEN implementation task.

**Organization**: Tasks follow the plan's dependency-ordered phases. Each task is tagged with contributing user stories.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no shared dependencies)
- **[Story]**: INFRA (shared), US1 (Manual Rewind), US2 (Restore Options), US3 (Auto Backups), US4 (Auto-Restore), US5 (Cross-Session)

### Plan Phase Mapping

| Plan Phase | Tasks Phase | Task IDs |
|------------|-------------|----------|
| Phase 1: Core Checkpoint (1.1-1.3) | Phase 1: Foundation | T001-T005 |
| Phase 1: Core Checkpoint (1.4-1.15) | Phase 2: Backup TDD | T006-T013 |
| Phase 2: Snapshot & Restore (2.1-2.15) | Phase 3: Snapshot & Restore TDD | T014-T022 |
| Phase 3: Persistence (3.1-3.8) | Phase 4: Persistence TDD | T023-T028 |
| Phase 4: Classification (4.1-4.8) | Phase 5: Classification TDD | T029-T034 |
| Phase 5: Engine Integration (5.1-5.9) | Phase 6: Integration | T035-T044 |
| Phase 6: TUI Selector (6.1-6.10) | Phase 7: TUI Selector | T045-T049 |
| Phase 6-7: Slash Commands + Auto-Restore | Phase 8: Slash Commands & Auto-Restore | T050-T053 |
| Phase 8: Cross-Session (8.1-8.3) | Phase 9: Cross-Session | T054-T056 |
| Phase 9: Edge Cases (9.1-9.9) | Phase 10: Hardening | T057-T063 |

---

## Phase 1: Foundation

**Purpose**: Package structure, data types, interfaces, dependency setup, and session directory access.

- [ ] T001 [INFRA] Create checkpoint package with doc.go and state.go — `internal/checkpoint/doc.go`, `internal/checkpoint/state.go`
  - **Description**: Create package godoc and data structures (FileHistoryState, FileHistorySnapshot, FileHistoryBackup, DiffStats).
  - **Dependencies**: None
  - **Est. Complexity**: Low

- [ ] T002 [P] [INFRA] Create FS interface and osFS implementation — `internal/checkpoint/fs.go`
  - **Description**: Define FS interface with CopyFile(dstPath, srcPath string, perm os.FileMode) error, Stat, ReadFile, WriteFile, Remove, MkdirAll, Open, OpenFile. Provide osFS struct implementing FS using os functions.
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [ ] T003 [P] [INFRA] Create Checkpointer interface and Config — `internal/checkpoint/checkpoint.go`
  - **Description**: Define Checkpointer interface (TrackEdit, MakeSnapshot, Rewind, GetDiffStats, HasAnyChanges, SetDisabled, IsDisabled, RestoreStateFromLog), Config struct, and New() constructor returning a stub implementation.
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [ ] T004 [P] [INFRA] Add go-diff dependency — `go.mod`
  - **Description**: Run `go get github.com/sergi/go-diff/diffmatchpatch` for line-level diff computation in GetDiffStats.
  - **Dependencies**: T001
  - **Est. Complexity**: Low

- [ ] T005 [P] [INFRA] Expose session checkpoints directory — `internal/session/session.go`
  - **Description**: Add method or field to expose session checkpoint storage path (RootDir + "/checkpoints/" and RootDir + "/checkpoints.jsonl"). Enables Checkpointer to resolve storage location from session.
  - **Dependencies**: T001
  - **Est. Complexity**: Low

**Checkpoint**: Package compiles. `go build ./internal/checkpoint/` succeeds.

---

## Phase 2: Core Implementation — Backup (TDD)

**Purpose**: File backup creation, tracking, and change detection.

**TC Coverage**: TC-001, TC-006, TC-012

- [ ] T006 [US3] RED: Write backup creation tests — `internal/checkpoint/backup_test.go`
  - **Description**: Write TestCreateBackup (TC-001: verify backup file created with correct content) and TestBackupFileName (verify SHA-256 hash naming format `{16hex}@v{N}`). Tests MUST fail.
  - **Dependencies**: T002, T003
  - **Est. Complexity**: Low

- [ ] T007 [US3] GREEN: Implement createBackup and backupFileName — `internal/checkpoint/backup.go`
  - **Description**: Implement createBackup() using FS.CopyFile for file-to-file copy with permission preservation, and backupFileName() computing SHA-256 of file path, taking first 16 hex chars. All tests in T006 MUST pass.
  - **Dependencies**: T006
  - **Est. Complexity**: Medium

- [ ] T008 [US3] RED: Write TrackEdit tests — `internal/checkpoint/backup_test.go`
  - **Description**: Write TestTrackEdit (verify file tracked and v1 backup created before modification) and TestTrackEditNullBackup (TC-006: verify null backup for non-existent file with empty BackupFileName). Tests MUST fail.
  - **Dependencies**: T007
  - **Est. Complexity**: Low

- [ ] T009 [US3] GREEN: Implement TrackEdit with null backup handling — `internal/checkpoint/backup.go`
  - **Description**: Implement TrackEdit(filePath, messageID) — skip if already tracked in latest snapshot, otherwise create v1 backup via FS.CopyFile. Handle file-not-exists by setting BackupFileName to empty string (null backup). All tests in T008 MUST pass.
  - **Dependencies**: T008
  - **Est. Complexity**: Medium

- [ ] T010 [US3] RED: Write backup permission test — `internal/checkpoint/backup_test.go`
  - **Description**: Write TestBackupPreservesPermissions (TC-012: verify file mode 0755 preserved after backup). Test MUST fail.
  - **Dependencies**: T009
  - **Est. Complexity**: Low

- [ ] T011 [US3] GREEN: Implement permission preservation — `internal/checkpoint/backup.go`
  - **Description**: Ensure createBackup preserves file mode via FS.CopyFile perm parameter. All tests in T010 MUST pass.
  - **Dependencies**: T010
  - **Est. Complexity**: Low

- [ ] T012 [US3] RED: Write change detection test — `internal/checkpoint/snapshot_test.go`
  - **Description**: Write TestChangeDetection verifying 4-level comparison: existence check, stat comparison (mode/size), mtime fast path (skip if mtime < backup time), content comparison (byte equality). Test MUST fail.
  - **Dependencies**: T011
  - **Est. Complexity**: Medium

- [ ] T013 [US3] GREEN: Implement fileChanged — `internal/checkpoint/snapshot.go`
  - **Description**: Implement fileChanged() with 4-level change detection: existence → stat → mtime → content. All tests in T012 MUST pass.
  - **Dependencies**: T012
  - **Est. Complexity**: Medium

---

## Phase 3: Core Implementation — Snapshot & Restore (TDD)

**Purpose**: Snapshot creation with FIFO eviction, code restoration, diff statistics.

**TC Coverage**: TC-002, TC-003, TC-005, TC-006, TC-009, TC-011

- [ ] T014 [US3] RED: Write MakeSnapshot tests — `internal/checkpoint/snapshot_test.go`
  - **Description**: Write TestMakeSnapshot (TC-002: verify snapshot created with correct messageID and file versions) and TestSnapshotFIFOEviction (TC-009: verify oldest snapshot evicted at 101). Tests MUST fail.
  - **Dependencies**: T013
  - **Est. Complexity**: Medium

- [ ] T015 [US3] GREEN: Implement MakeSnapshot with FIFO eviction — `internal/checkpoint/snapshot.go`
  - **Description**: Implement MakeSnapshot(messageID) — iterate tracked files, detect changes via fileChanged(), create new backup versions for changed files, reuse references for unchanged, append FileHistorySnapshot, evict oldest when > maxSnapshots, increment snapshotSequence. All tests in T014 MUST pass.
  - **Dependencies**: T014
  - **Est. Complexity**: High

- [ ] T016 [US2] RED: Write Rewind tests — `internal/checkpoint/restore_test.go`
  - **Description**: Write TestRewind (TC-003: verify files restored to snapshot state) and TestRewindNullBackup (TC-006: verify file deleted when snapshot shows null backup). Tests MUST fail.
  - **Dependencies**: T015
  - **Est. Complexity**: Medium

- [ ] T017 [US2] GREEN: Implement Rewind — `internal/checkpoint/restore.go`
  - **Description**: Implement Rewind(messageID) → []string — find snapshot by messageID, iterate tracked files: use recorded backup, delete file for null backups (BackupFileName empty), skip if content unchanged, restore from backup via FS.CopyFile for changed files. Pure filesystem operation (does not modify FileHistoryState). All tests in T016 MUST pass.
  - **Dependencies**: T016
  - **Est. Complexity**: High

- [ ] T018 [US1] RED: Write GetDiffStats test — `internal/checkpoint/restore_test.go`
  - **Description**: Write TestGetDiffStats (TC-011: verify diff statistics — files changed count, insertion count, deletion count). Test MUST fail.
  - **Dependencies**: T017
  - **Est. Complexity**: Medium

- [ ] T019 [US1] GREEN: Implement GetDiffStats — `internal/checkpoint/restore.go`
  - **Description**: Implement GetDiffStats(messageID) → *DiffStats — compare current files to snapshot, compute line diffs using go-diff/diffmatchpatch, return DiffStats with FilesChanged, Insertions, Deletions, ChangedFiles. All tests in T018 MUST pass.
  - **Dependencies**: T018
  - **Est. Complexity**: Medium

- [ ] T020 [US4] RED: Write HasAnyChanges test — `internal/checkpoint/restore_test.go`
  - **Description**: Write TestHasAnyChanges (verify lightweight change detection, short-circuits on first changed file) and TestCombinedRestore (TC-005: verify code restore is a pure FS operation). Tests MUST fail.
  - **Dependencies**: T019
  - **Est. Complexity**: Low

- [ ] T021 [US4] GREEN: Implement HasAnyChanges — `internal/checkpoint/restore.go`
  - **Description**: Implement HasAnyChanges(messageID) → bool — iterate tracked files, short-circuit on first file that differs from snapshot. Verify Rewind does not modify state (already pure FS). All tests in T020 MUST pass.
  - **Dependencies**: T020
  - **Est. Complexity**: Low

- [ ] T022 [INFRA] REFACTOR: Review Phase 2–3 code — `internal/checkpoint/`
  - **Description**: Review all backup.go, snapshot.go, restore.go for readability, remove duplication, ensure block comments on exported identifiers per constitution principle 3. All existing tests MUST still pass.
  - **Dependencies**: T021
  - **Est. Complexity**: Low

---

## Phase 4: Core Implementation — Persistence (TDD)

**Purpose**: Snapshot persistence to JSONL with atomic writes and state rebuild.

**TC Coverage**: TC-010

- [ ] T023 [US5] RED: Write RecordSnapshot test — `internal/checkpoint/persist_test.go`
  - **Description**: Write TestRecordSnapshot — verify snapshot written to checkpoints.jsonl as valid JSON line. Test MUST fail.
  - **Dependencies**: T022
  - **Est. Complexity**: Low

- [ ] T024 [US5] GREEN: Implement RecordSnapshot — `internal/checkpoint/persist.go`
  - **Description**: Implement RecordSnapshot(messageID, snapshot, isUpdate) — marshal SnapshotRecord as JSON, append line to checkpoints.jsonl. All tests in T023 MUST pass.
  - **Dependencies**: T023
  - **Est. Complexity**: Low

- [ ] T025 [US5] RED: Write RestoreStateFromLog test — `internal/checkpoint/persist_test.go`
  - **Description**: Write TestRestoreStateFromLog (TC-010: verify state rebuilt from persisted snapshots). Test MUST fail.
  - **Dependencies**: T024
  - **Est. Complexity**: Medium

- [ ] T026 [US5] GREEN: Implement RestoreStateFromLog — `internal/checkpoint/persist.go`
  - **Description**: Implement RestoreStateFromLog() — read checkpoints.jsonl, unmarshal each JSON line into SnapshotRecord, replay into FileHistoryState. All tests in T025 MUST pass.
  - **Dependencies**: T025
  - **Est. Complexity**: Medium

- [ ] T027 [US5] RED: Write atomic persistence and corrupt handling tests — `internal/checkpoint/persist_test.go`
  - **Description**: Write TestAtomicPersistence (verify temp-file-then-rename pattern) and TestCorruptSnapshotLog (verify graceful handling — skip corrupt entries with error logging). Tests MUST fail.
  - **Dependencies**: T026
  - **Est. Complexity**: Medium

- [ ] T028 [US5] GREEN: Implement atomic writes and corrupt handling — `internal/checkpoint/persist.go`
  - **Description**: Refine RecordSnapshot to write to temp file then os.Rename for atomicity. Add corrupt entry handling in RestoreStateFromLog — skip malformed JSON lines, log errors, continue. All tests in T027 MUST pass.
  - **Dependencies**: T027
  - **Est. Complexity**: Medium

---

## Phase 5: Core Implementation — Classification (TDD) [P with Phases 2–4]

**Purpose**: Message classification for auto-restore logic and selectable message filtering.

**TC Coverage**: TC-007 (logic), TC-008 (logic)

> **Note**: This phase is fully independent of Phases 2–4 and can execute in parallel after Phase 1.

- [ ] T029 [P] [US4] RED: Write IsSynthetic test — `internal/checkpoint/classify_test.go`
  - **Description**: Write TestIsSynthetic — verify progress, system, empty tool results, meta user messages (isMeta=true), compact summaries (isCompactSummary=true), transcript-only (isVisibleInTranscriptOnly=true) classified as synthetic per FR-007. Test MUST fail.
  - **Dependencies**: T001
  - **Est. Complexity**: Medium

- [ ] T030 [P] [US4] GREEN: Implement IsSynthetic — `internal/checkpoint/classify.go`
  - **Description**: Implement IsSynthetic(rec session.MessageRecord) bool — check message kind and content flags per FR-007 definitions. All tests in T029 MUST pass.
  - **Dependencies**: T029
  - **Est. Complexity**: Medium

- [ ] T031 [P] [US4] RED: Write IsMeaningful test — `internal/checkpoint/classify_test.go`
  - **Description**: Write TestIsMeaningful — verify assistant messages with non-empty text or tool_use blocks, and tool results with non-empty output, classified as meaningful. Test MUST fail.
  - **Dependencies**: T030
  - **Est. Complexity**: Low

- [ ] T032 [P] [US4] GREEN: Implement IsMeaningful — `internal/checkpoint/classify.go`
  - **Description**: Implement IsMeaningful(rec session.MessageRecord) bool — check for non-empty text or tool_use blocks in assistant messages, non-empty output in tool results. All tests in T031 MUST pass.
  - **Dependencies**: T031
  - **Est. Complexity**: Low

- [ ] T033 [P] [US1,US4] RED: Write SelectableMessages and MessagesAfterAreOnlySynthetic tests — `internal/checkpoint/classify_test.go`
  - **Description**: Write TestSelectableMessages (verify only non-meta user messages pass filter) and TestMessagesAfterAreOnlySynthetic (verify auto-restore trigger logic — true when all subsequent messages are synthetic). Tests MUST fail.
  - **Dependencies**: T032
  - **Est. Complexity**: Medium

- [ ] T034 [P] [US1,US4] GREEN: Implement SelectableMessages and MessagesAfterAreOnlySynthetic — `internal/checkpoint/classify.go`
  - **Description**: Implement SelectableMessages(records) → []SelectableMessage (filter user messages, exclude meta) and MessagesAfterAreOnlySynthetic(records, index) → bool (iterate from index, return true if all synthetic). All tests in T033 MUST pass.
  - **Dependencies**: T033
  - **Est. Complexity**: Medium

---

## Phase 6: Integration

**Purpose**: Wire checkpoint system into engine loop, tool middleware, and app startup.

**TC Coverage**: TC-013

- [ ] T035 [US3] Create checkpoint middleware stub — `internal/middleware/checkpoint.go`
  - **Description**: Create checkpointMiddleware struct with checkpointer and getMessageID fields. Implement BeforeExecute as a stub that always returns DecisionAllow (no TrackEdit call yet). Include NewCheckpointMiddleware constructor.
  - **Dependencies**: T022, T034
  - **Est. Complexity**: Low

- [ ] T036 [US3] RED: Write middleware test — `internal/middleware/checkpoint_test.go`
  - **Description**: Write TestCheckpointMiddleware — verify TrackEdit called for write_file and edit_file, NOT called for bash tool. Verify middleware always returns DecisionAllow. Test MUST fail (stub doesn't call TrackEdit yet).
  - **Dependencies**: T035
  - **Est. Complexity**: Low

- [ ] T037 [US3] GREEN: Complete middleware implementation — `internal/middleware/checkpoint.go`
  - **Description**: Implement BeforeExecute to check tool name and call TrackEdit only for write_file and edit_file. Always return DecisionAllow. All tests in T036 MUST pass.
  - **Dependencies**: T036
  - **Est. Complexity**: Low

- [ ] T038a [P] [INFRA] RED: Write MessageLog.Append return Seq test — `internal/session/message_log_test.go`
  - **Description**: Write TestMessageLogAppendReturnsSeq — verify Append() returns the assigned int64 Seq value and that sequential calls return monotonically increasing values. Test MUST fail (Append doesn't return Seq yet).
  - **Dependencies**: T022, T034
  - **Est. Complexity**: Low

- [ ] T038b [P] [INFRA] GREEN: Modify MessageLog.Append to return Seq — `internal/session/message_log.go`
  - **Description**: Change MessageLog.Append() to return the assigned int64 Seq value. This Seq is used as the messageID in snapshots. Update callers if signature changed. All tests in T038a MUST pass.
  - **Dependencies**: T038a
  - **Est. Complexity**: Low

- [ ] T039 [US3] RED: Write engine snapshot hook test — `internal/engine/loop_test.go`
  - **Description**: Write TestEngineMakeSnapshotOnUserMessage — verify that when a user message is processed, the engine calls checkpointer.MakeSnapshot() with the correct message Seq. Test MUST fail.
  - **Dependencies**: T038b
  - **Est. Complexity**: Medium

- [ ] T040 [US3] GREEN: Add checkpointer to engine and MakeSnapshot hook — `internal/engine/loop.go`
  - **Description**: Add optional checkpointer field to AgentEngine. In the engine loop, after appending user message to MessageLog and before calling model, call checkpointer.MakeSnapshot() with the message Seq. Guard with nil check. All tests in T039 MUST pass.
  - **Dependencies**: T039
  - **Est. Complexity**: Medium

- [ ] T041 [NFR-005] RED: Write disabled checkpointing test — `internal/checkpoint/checkpoint_test.go`
  - **Description**: Write TestDisabledCheckpointing (TC-013: verify no backups created when FOXHARNESS_DISABLE_FILE_CHECKPOINTING=1). Test MUST fail.
  - **Dependencies**: T040
  - **Est. Complexity**: Low

- [ ] T042 [NFR-005] GREEN: Implement disabled mode in Checkpointer — `internal/checkpoint/checkpoint.go`
  - **Description**: Implement SetDisabled/IsDisabled. When disabled, TrackEdit and MakeSnapshot return nil without creating backups. All tests in T041 MUST pass.
  - **Dependencies**: T041
  - **Est. Complexity**: Low

- [ ] T043 [INFRA] Wire Checkpointer in runner — `internal/app/runner.go`
  - **Description**: Create checkpoint.New() with session directory from T005. Read FOXHARNESS_DISABLE_FILE_CHECKPOINTING env var. Register checkpoint middleware in tool registry. Pass checkpointer to engine config.
  - **Dependencies**: T042
  - **Est. Complexity**: Medium

- [ ] T044 [INFRA] Pass Checkpointer to TUI model — `internal/app/tui.go`
  - **Description**: Add checkpointer field to TUI model creation in tui.go. Pass the checkpointer instance from T043 to the TUI model constructor.
  - **Dependencies**: T043
  - **Est. Complexity**: Low

**Checkpoint**: `go build ./...` succeeds. Backup middleware fires on write_file/edit_file. MakeSnapshot called at user message boundary.

---

## Phase 7: Interface — TUI Selector

**Purpose**: Interactive message selector sub-model as Bubble Tea component.

- [ ] T045 [US1] Create selector types — `internal/tui/selector/types.go`
  - **Description**: Define RestoreAction enum (ActionNone, ActionRestoreBoth, ActionRestoreConversation, ActionRestoreCode, ActionCancelled), ViewState enum (listView, previewView), and ResultMsg struct (Action, MessageID). Import SelectableMessage from `internal/checkpoint` — do not redefine.
  - **Dependencies**: T034
  - **Est. Complexity**: Low

- [ ] T046 [US1] Create selector key bindings — `internal/tui/selector/keys.go`
  - **Description**: Define key bindings: up/down arrows and j/k for navigation, Enter for selection, Escape/q for cancel. Use Bubble Tea key binding pattern.
  - **Dependencies**: T045
  - **Est. Complexity**: Low

- [ ] T047 [US1] RED: Write selector model tests — `internal/tui/selector/model_test.go`
  - **Description**: Write TestSelectorStateTransition (verify list → preview transition on Enter, preview → list on Escape, cancel returns ActionCancelled) and TestSelectorResultMsg (verify selecting a message and choosing restore returns correct ResultMsg with Action and MessageID). Tests MUST fail.
  - **Dependencies**: T046
  - **Est. Complexity**: Medium

- [ ] T048 [US1] GREEN: Implement selector model — `internal/tui/selector/model.go`
  - **Description**: Implement Bubble Tea Model with state (listView/previewView), messages list, cursor, diffStats, selected message, checkpointer reference. Implement Init(), Update() handling key events and view transitions. All tests in T047 MUST pass.
  - **Dependencies**: T047
  - **Est. Complexity**: High

- [ ] T049a [US1] RED: Write selector view tests — `internal/tui/selector/view_test.go`
  - **Description**: Write TestSelectorListView (verify list view contains message text, timestamp, cursor indicator) and TestSelectorPreviewView (verify preview view contains diff stats, file paths, restore options). Tests MUST fail.
  - **Dependencies**: T048
  - **Est. Complexity**: Low

- [ ] T049b [US1] GREEN: Implement selector view — `internal/tui/selector/view.go`
  - **Description**: Implement View() rendering: message list view (truncated text + timestamp per entry, current position entry, cursor highlight), diff preview view (file count, insertions/deletions, file paths, restore options). Use Lipgloss for styling consistent with existing TUI. All tests in T049a MUST pass.
  - **Dependencies**: T049a
  - **Est. Complexity**: Medium

---

## Phase 8: Interface — Slash Commands & Auto-Restore

**Purpose**: /rewind and /checkpoint commands plus automatic conversation restore on Ctrl+C.

**TC Coverage**: TC-004, TC-007, TC-008, TC-014

- [ ] T050 [US1] RED: Write slash command tests — `internal/tui/model_test.go`
  - **Description**: Write TestSlashCommands (TC-014: verify /rewind and /checkpoint registered in slashCommands, both open selector sub-model, disabled while m.running is true). Test MUST fail.
  - **Dependencies**: T049b
  - **Est. Complexity**: Medium

- [ ] T051 [US1] GREEN: Add /rewind and /checkpoint to TUI model — `internal/tui/model.go`
  - **Description**: Add /rewind and /checkpoint to slashCommands slice. Add handler in handleSlashCommand that opens selector sub-model (delegates to same function for both commands). Handle selector.ResultMsg to execute Rewind and/or conversation truncation based on RestoreAction. Disable both commands while m.running is true. All tests in T050 MUST pass.
  - **Dependencies**: T050
  - **Est. Complexity**: High

- [ ] T052 [US4] RED: Write auto-restore tests — `internal/tui/model_test.go`
  - **Description**: Write TestAutoRestoreOnCancel (TC-007: verify conversation auto-restores to last user message when only synthetic content after cancel) and TestNoAutoRestoreWithMeaningfulContent (TC-008: verify NO auto-restore when tool results exist). Tests MUST fail.
  - **Dependencies**: T051
  - **Est. Complexity**: Medium

- [ ] T053 [US4] GREEN: Implement auto-restore check — `internal/tui/model.go`
  - **Description**: Modify Ctrl+C handler: after m.cancelRun(), call checkpoint.MessagesAfterAreOnlySynthetic(). If true, truncate message history to last user message index, restore original input text to input field. If false (meaningful content exists), do nothing — user can use /rewind manually. All tests in T052 MUST pass.
  - **Dependencies**: T052
  - **Est. Complexity**: High

**Checkpoint**: `/rewind` opens selector. Ctrl+C with no meaningful work auto-restores input. Ctrl+C after tool execution does NOT auto-restore.

---

## Phase 9: Interface — Cross-Session

**Purpose**: Checkpoint state survives session resume via --resume.

**TC Coverage**: TC-010 (integration)

- [ ] T054 [US5] Wire RestoreStateFromLog on session resume — `internal/app/runner.go`
  - **Description**: When resuming a session (existing session found via manager.Latest()), call checkpointer.RestoreStateFromLog() to rebuild FileHistoryState from persisted checkpoints.jsonl.
  - **Dependencies**: T044
  - **Est. Complexity**: Medium

- [ ] T055 [US5] RED: Write cross-session persistence integration test — `internal/checkpoint/checkpoint_test.go`
  - **Description**: Write TestCrossSessionPersistence (TC-010: session 1 creates snapshots, session 2 resumes and verifies all snapshots available, /rewind can restore to any checkpoint). Test MUST fail.
  - **Dependencies**: T054
  - **Est. Complexity**: High

- [ ] T056 [US5] GREEN: Verify and complete cross-session wiring — `internal/app/runner.go`
  - **Description**: Ensure session resume path correctly initializes checkpointer with session directory and calls RestoreStateFromLog before engine starts. All tests in T055 MUST pass.
  - **Dependencies**: T055
  - **Est. Complexity**: Medium

**Checkpoint**: Resume a session with `--resume`, run `/rewind`, verify checkpoints from previous session available.

---

## Phase 10: Edge Cases & Benchmarks

**Purpose**: Handle all spec edge cases with TDD and validate performance NFRs.

- [ ] T057 [US3] RED: Write backup edge case tests — `internal/checkpoint/backup_test.go`
  - **Description**: Write TestLargeFileBackup (streaming copy), TestBinaryFileBackup (no text encoding issues), TestEmptyFileBackup (empty file ≠ null backup), and TestSymlinkBackup (follow symlinks). Tests MUST fail.
  - **Dependencies**: T022
  - **Est. Complexity**: Medium

- [ ] T058 [US3] GREEN: Implement backup edge cases — `internal/checkpoint/backup.go`
  - **Description**: Ensure createBackup uses io.Copy (streaming) not ReadFile+WriteFile for large files. Verify binary content handled correctly (no encoding assumptions). Empty files backed up as empty files (not null backups). Use os.Stat (not os.Lstat) to follow symlinks. All tests in T057 MUST pass.
  - **Dependencies**: T057
  - **Est. Complexity**: Medium

- [ ] T059a [US2] RED: Write restore edge case tests — `internal/checkpoint/restore_test.go`
  - **Description**: Write TestMissingBackupOnRestore (skip file, log error, continue) and TestDeletedTrackedFileBetweenBackupAndSnapshot (record null backup). Tests MUST fail.
  - **Dependencies**: T058
  - **Est. Complexity**: Low

- [ ] T059b [P] [US3] RED: Write snapshot edge case tests — `internal/checkpoint/snapshot_test.go`
  - **Description**: Write TestMissingCheckpointsDir (recreate lazily) and TestFirstSnapshotNoTrackedFiles (empty trackedFileBackups handled). Tests MUST fail.
  - **Dependencies**: T058
  - **Est. Complexity**: Low

- [ ] T060a [US2] GREEN: Implement restore edge cases — `internal/checkpoint/restore.go`
  - **Description**: Handle missing backup: skip file, log error, continue. Handle deleted tracked file: record null backup. All tests in T059a MUST pass.
  - **Dependencies**: T059a
  - **Est. Complexity**: Low

- [ ] T060b [P] [US3] GREEN: Implement snapshot edge cases — `internal/checkpoint/snapshot.go`
  - **Description**: Handle missing checkpoints directory: recreate lazily via FS.MkdirAll. Handle first snapshot with no tracked files: empty trackedFileBackups. All tests in T059b MUST pass.
  - **Dependencies**: T059b
  - **Est. Complexity**: Low

- [ ] T061 [US3] RED: Write concurrent backup test — `internal/checkpoint/backup_test.go`
  - **Description**: Write TestConcurrentTrackEdit (verify multiple parallel-safe tools modifying different files create independent backups without conflict). Test MUST fail.
  - **Dependencies**: T060a, T060b
  - **Est. Complexity**: Medium

- [ ] T062 [US3] GREEN: Ensure concurrent backup safety — `internal/checkpoint/backup.go`
  - **Description**: Verify TrackEdit is safe for concurrent calls on different files. Each file gets its own backup without conflict. All tests in T061 MUST pass.
  - **Dependencies**: T061
  - **Est. Complexity**: Low

- [ ] T063 [INFRA] Write benchmarks — `internal/checkpoint/benchmark_test.go`
  - **Description**: Write BenchmarkTrackEdit, BenchmarkMakeSnapshot, BenchmarkChangeDetection, BenchmarkGetDiffStats. Validate NFR-001: backup creation < 50ms, snapshot for < 10 files < 200ms, mtime fast path skips content comparison in > 90% of cases.
  - **Dependencies**: T062
  - **Est. Complexity**: Medium

---

## Dependencies & Execution Order

### Phase Dependencies

```
Phase 1 (Foundation)
  ├── Phase 2 (Backup TDD)
  │     └── Phase 3 (Snapshot & Restore TDD)
  │           └── Phase 4 (Persistence TDD)
  └── Phase 5 (Classification TDD) [P with Phases 2-4]

Phase 2-5 complete → Phase 6 (Integration)
Phase 6 complete → Phase 7 (TUI Selector)
Phase 7 complete → Phase 8 (Slash Commands & Auto-Restore)
Phase 6 complete → Phase 9 (Cross-Session) [after Phase 8, sequential due to runner.go]
All phases complete → Phase 10 (Edge Cases & Benchmarks)
```

### Parallel Opportunities

1. **Phase 5 (Classification) is fully independent** of Phases 2–4 after Phase 1. Can execute in parallel.
2. **Phase 1 tasks T002, T003, T004, T005** can execute in parallel after T001.
3. **T038a/T038b (MessageLog.Append)** can execute in parallel with T035–T037 after Phases 2–5 complete — modifies a different file (session vs middleware).
4. **Phase 9 (Cross-Session) must follow Phase 8** — both modify runner.go so sequential ordering required.
5. **Phase 10: T059a/T060a (restore) and T059b/T060b (snapshot)** are independent edge case groups that can execute in parallel after T058.

### Execution Order

```
T001 ──► ┌── T002 [P]
         ├── T003 [P]
         ├── T004 [P]
         └── T005 [P]
               │
               ▼
         T006–T013 (Backup TDD)
               │
               ▼
         T014–T022 (Snapshot & Restore TDD)
               │
               ▼
         T023–T028 (Persistence TDD)
               │
         T029–T034 (Classification TDD) [P — starts after T001]
               │
               ▼
         ┌── T035–T037 (Middleware TDD) ──► T039–T044
         └── T038a→T038b [P] (MessageLog) ──► T039
               │
               ▼
         T045–T049b (TUI Selector)
               │
               ▼
         T050–T053 (Slash Commands & Auto-Restore)
               │
               ▼
         T054–T056 (Cross-Session)
               │
               ▼
         T057–T058 ──► ┌── T059a → T060a [P]
                       └── T059b → T060b [P]
                                    │
                                    ▼
                              T061–T063
```

---

## Checkpoints

- [ ] **Checkpoint 1** (after Phase 1): `go build ./internal/checkpoint/` succeeds
- [ ] **Checkpoint 2** (after Phase 3): All backup, snapshot, restore tests pass — `go test ./internal/checkpoint/...`
- [ ] **Checkpoint 3** (after Phase 6): `go build ./...` succeeds; middleware and engine hooks wired
- [ ] **Checkpoint 4** (after Phase 7): Selector sub-model works in isolation
- [ ] **Checkpoint 5** (after Phase 8): `/rewind` opens selector; Ctrl+C auto-restore works end-to-end
- [ ] **Checkpoint 6** (after Phase 9): Cross-session resume preserves checkpoints
- [ ] **Checkpoint 7** (after Phase 10): All edge case tests pass; benchmarks validate NFR-001

---

## Summary

| Metric | Value |
|--------|-------|
| Total tasks | 67 |
| TDD task pairs (RED+GREEN) | 23 pairs (46 tasks) |
| Setup/structure tasks | 5 |
| Integration tasks | 10 |
| Interface tasks | 10 |
| Hardening tasks | 8 |
| Parallelizable tasks | 16 |
| Estimated phases | 10 |

### Test Case Coverage

| Test Case | Tasks |
|-----------|-------|
| TC-001: Basic File Backup | T006, T007 |
| TC-002: Snapshot Creation | T014, T015 |
| TC-003: Code Restore | T016, T017 |
| TC-004: Conversation Restore | T050, T051 |
| TC-005: Combined Restore | T020, T021 |
| TC-006: Null Backup | T008, T009, T016, T017 |
| TC-007: Auto-Restore | T029, T030, T052, T053 |
| TC-008: No Auto-Restore | T031, T032, T052, T053 |
| TC-009: FIFO Eviction | T014, T015 |
| TC-010: Cross-Session | T025, T026, T055, T056 |
| TC-011: Diff Stats | T018, T019 |
| TC-012: Permission Preservation | T010, T011 |
| TC-013: Disabled Checkpointing | T041, T042 |
| TC-014: Slash Commands | T050, T051 |
