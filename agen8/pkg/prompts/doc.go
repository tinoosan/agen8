// Package prompts provides built-in system prompts for the agent (base, autonomous, subagent, team mode).
// It is the single source of truth for default prompts so new modes or features can add prompts in one place.
//
// How to add a prompt: add a new exported function that returns a string. Build on DefaultSystemPrompt().
// For task-runner modes, optionally include sharedTaskRunnerBlock and reportingBlock(includeEmail, forSubAgent).
// Keep mode-specific content in modes.go or a dedicated file (e.g. team.go).
package prompts
