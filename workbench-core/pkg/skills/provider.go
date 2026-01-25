package skills

// SkillsProvider abstracts skill discovery for prompt injection.
type SkillsProvider interface {
	Entries() []SkillEntry
}
