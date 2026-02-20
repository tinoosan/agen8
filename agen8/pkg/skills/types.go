package skills

// Skill describes the metadata for a registered skill.
type Skill struct {
	Name          string
	Description   string
	Compatibility string // environment requirements (e.g. "Requires python3, pandas")
	Dir           string
	Path          string // absolute path to SKILL.md
}

type ScriptEntry struct {
	Name string // e.g. "fetch_stock_data.py"
	Rel  string // e.g. "scripts/fetch_stock_data.py"
}

type SkillScripts struct {
	Skill   string
	Scripts []ScriptEntry
}
