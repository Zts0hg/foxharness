// Package configcmd implements the interactive `fox config` subcommand and the
// first-run onboarding guidance shown when no LLM provider is configured.
//
// The wizard guides a user through adding an LLM provider profile, using a
// built-in preset catalog to pre-fill common connection details and reusing the
// existing internal/llmconfig resolution and internal/settings persistence. The
// package is intentionally free of vendor-specific code paths: presets are plain
// template data, and provider construction stays protocol-based.
package configcmd
