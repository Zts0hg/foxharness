// Package skilltool exposes the slash command system to the LLM agent.
//
// It provides:
//   - SkillTool — a tools.BaseTool implementation named "skill" that lets
//     the model invoke a registry-resident prompt command on demand. The
//     model picks the skill by name and supplies arguments as a single
//     string; the tool runs the same execution pipeline as a user-typed
//     slash command.
//   - FormatSkillsWithinBudget — a helper for producing the formatted
//     list of model-invocable skills injected into the system prompt.
//     The output stays inside a per-call character budget derived from
//     the active model's context window, using a 3-level truncation
//     strategy modeled after Claude Code's implementation.
package skilltool
