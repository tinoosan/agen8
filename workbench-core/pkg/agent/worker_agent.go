package agent

import "strings"

// Deprecated: prefer NewAgent with WorkerConfig.
//
// NewWorkerAgent constructs a worker-specialized agent (execution-focused prompt).
// Tooling is inherited from DefaultAgent; caller can still pass allowedTools/skills
// via task metadata (handled in worker loop).
func NewWorkerAgent(opts ...Option) (Agent, error) {
	opts = append([]Option{WithSystemPrompt(WorkerSystemPrompt())}, opts...)
	return NewDefaultAgent(opts...)
}

func WorkerSystemPrompt() string {
	return strings.TrimSpace(`<system>
  <identity>You are a focused execution agent (worker).</identity>
  <core_rules>
    <rule>Your role: execute the assigned task efficiently and safely.</rule>
    <rule>Respect provided task metadata (skills, allowed tools).</rule>
    <rule>Report findings and results clearly; keep responses concise.</rule>
    <rule>Avoid broad refactors unless explicitly requested.</rule>
  </core_rules>
</system>`)
}
