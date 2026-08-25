package skills

// SkillFile is one file in a skill directory tree.
type SkillFile struct {
	Path    string
	Content string
}

// SkillLocation controls where a skill is written on disk.
type SkillLocation string

const (
	SkillLocationGlobal  SkillLocation = "global"
	SkillLocationProject SkillLocation = "project"

	NoGroupDir    = "_skills_without_group"
	PortSkillsDir = "port"
)

// Skill is a catalog skill ready to sync to disk.
type Skill struct {
	Identifier     string
	AgentSkillName string
	Title          string
	Description    string
	Version        string
	GroupIDs       []string
	Location       SkillLocation
	Files          []SkillFile
}

// SkillGroup is a skill group from the catalog.
type SkillGroup struct {
	Identifier       string
	Title            string
	MatchesUserTeams bool
	SkillIDs         []string
}

// FetchedSkills contains the skill catalog from Port.
type FetchedSkills struct {
	Skills []Skill
	Groups []SkillGroup
}

func parseSkillLocation(raw string) SkillLocation {
	if raw == string(SkillLocationProject) {
		return SkillLocationProject
	}
	return SkillLocationGlobal
}
