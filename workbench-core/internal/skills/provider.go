package skills

// SkillsProvider abstracts skill discovery for prompt injection.
// This allows ContextConstructor to remain decoupled from the concrete Manager.
type SkillsProvider interface {
	Entries() []SkillEntry
}
