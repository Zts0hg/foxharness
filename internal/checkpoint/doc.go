// Package checkpoint provides file checkpoints and rewind support for
// foxharness sessions.
//
// A checkpointer stores file backups under the active session directory,
// records snapshots in a JSONL log, and can restore files to the state they
// had at a previous user-message boundary. It also owns the message
// classification helpers used by the TUI rewind flow.
package checkpoint
