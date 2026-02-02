package skills

// Skill describes the metadata for a registered skill.
type Skill struct {
	Name        string
	Description string
	Dir         string
	Path        string // absolute path to SKILL.md
}
