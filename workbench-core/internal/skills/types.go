package skills

// Skill describes the metadata for a registered skill.
type Skill struct {
	// Name is the optional frontmatter-provided skill name.
	Name string

	// Description is the optional frontmatter-provided summary.
	Description string

	// Path is the absolute host directory that contains the skill implementation.
	Path string
}
