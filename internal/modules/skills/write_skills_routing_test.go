package skills

import (
	"path/filepath"
	"testing"
)

func TestWriteSkills_LocationRouting(t *testing.T) {
	tests := []struct {
		name          string
		location      SkillLocation
		hasProjectDir bool
		wantInGlobal  bool
		wantInProject bool
	}{
		{
			name:          "global skill goes to global only",
			location:      SkillLocationGlobal,
			hasProjectDir: true,
			wantInGlobal:  true,
			wantInProject: false,
		},
		{
			name:          "project skill goes to project only",
			location:      SkillLocationProject,
			hasProjectDir: true,
			wantInGlobal:  false,
			wantInProject: true,
		},
		{
			name:          "default (empty) location is global",
			location:      "",
			hasProjectDir: true,
			wantInGlobal:  true,
			wantInProject: false,
		},
		{
			name:          "project skill skipped when no projectDirs",
			location:      SkillLocationProject,
			hasProjectDir: false,
			wantInGlobal:  false,
			wantInProject: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			globalTarget := filepath.Join(homeDir, ".cursor")

			var projectDirs []string
			var projectDir string
			if tt.hasProjectDir {
				projectDir = t.TempDir()
				projectDirs = []string{projectDir}
			}

			skills := []Skill{{
				Identifier: "skill",
				Title:      "skill",
				GroupIDs:   []string{"grp"},
				Location:   tt.location,
				Files:      []SkillFile{{Path: "SKILL.md", Content: "# x"}},
			}}
			if err := WriteSkills(skills, nil, []string{globalTarget}, projectDirs); err != nil {
				t.Fatalf("WriteSkills: %v", err)
			}

			globalPath := skillMDPath(globalTarget, "grp", "skill")
			if tt.wantInGlobal {
				assertFileExists(t, globalPath)
			} else {
				assertFileAbsent(t, globalPath)
			}

			if projectDir != "" {
				projectPath := skillMDPath(filepath.Join(projectDir, ".cursor"), "grp", "skill")
				if tt.wantInProject {
					assertFileExists(t, projectPath)
				} else {
					assertFileAbsent(t, projectPath)
				}
			}
		})
	}
}

func TestWriteSkills_LocationChangeCleansUpOldLocation(t *testing.T) {
	homeDir := t.TempDir()
	globalTarget := filepath.Join(homeDir, ".cursor")
	projectDir := t.TempDir()

	syncWithLocation := func(location SkillLocation) {
		t.Helper()
		skill := skillWithMD("moved-skill", "moved-skill", "grp", "# x")
		skill.Location = location
		if err := WriteSkills([]Skill{skill}, nil, []string{globalTarget}, []string{projectDir}); err != nil {
			t.Fatalf("WriteSkills: %v", err)
		}
	}

	projectPath := skillMDPath(filepath.Join(projectDir, ".cursor"), "grp", "moved-skill")
	globalPath := skillMDPath(globalTarget, "grp", "moved-skill")

	// First sync: skill starts out project-scoped.
	syncWithLocation(SkillLocationProject)
	assertFileExists(t, projectPath)
	assertFileAbsent(t, globalPath)

	// Second sync: the skill's location moved to global (it was the only
	// project-scoped skill). The stale project copy must be removed even
	// though no project-scoped skills remain.
	syncWithLocation(SkillLocationGlobal)
	assertFileExists(t, globalPath)
	assertFileAbsent(t, projectPath)
}

func TestWriteSkills_PathTraversalPrevention(t *testing.T) {
	tests := []struct {
		name        string
		skill       Skill
		wantErrFrag string
	}{
		{
			name: "traversal in skill directory name",
			skill: Skill{
				Identifier: "..",
				Title:      "..",
				GroupIDs:   []string{"grp"},
				Files:      []SkillFile{{Path: "SKILL.md", Content: "# x"}},
			},
			wantErrFrag: "invalid skill directory name",
		},
		{
			name: "traversal in file path",
			skill: Skill{
				Identifier: "sk",
				Title:      "sk",
				GroupIDs:   []string{"grp"},
				Files: []SkillFile{
					{Path: "SKILL.md", Content: "# x"},
					{Path: "../../../../tmp/evil", Content: "pwned"},
				},
			},
			wantErrFrag: "escapes skill directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			err := WriteSkills([]Skill{tt.skill}, nil, []string{dir}, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !containsStr(err.Error(), tt.wantErrFrag) {
				t.Errorf("want error containing %q, got: %v", tt.wantErrFrag, err)
			}
		})
	}
}

func TestWriteSkills_IgnoresInvalidExplicitSkillName(t *testing.T) {
	dir := t.TempDir()
	skill := Skill{
		Identifier: "ok-skill",
		Title:      "ok-skill",
		GroupIDs:   []string{"grp"},
		Files:      []SkillFile{{Path: "SKILL.md", Content: "---\nname: ../etc\ndescription: bad\n---\n# x"}},
	}

	if err := WriteSkills([]Skill{skill}, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	assertFileContent(t, skillMDPath(dir, "grp", "ok-skill"), "---\nname: ok-skill\ndescription: bad\n---\n# x")
}

func TestGitHubCopilot_SkillRouting(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()
	copilotTarget := filepath.Join(repoDir, ".github")
	// Use .codex (not .cursor) so tests run in sandboxes that block creating `.cursor`.
	codexTarget := filepath.Join(homeDir, ".codex")

	t.Run("global skills go to repo .github", func(t *testing.T) {
		skills := []Skill{skillWithMD("global-skill", "global-skill", "grp", "# x")}
		skills[0].Location = SkillLocationGlobal
		if err := WriteSkills(skills, nil, []string{copilotTarget}, nil); err != nil {
			t.Fatalf("WriteSkills: %v", err)
		}
		assertFileExists(t, skillMDPath(copilotTarget, "grp", "global-skill"))
	})

	t.Run("project skills go to repo/.github", func(t *testing.T) {
		skills := []Skill{skillWithMD("proj-skill", "proj-skill", "grp", "# x")}
		skills[0].Location = SkillLocationProject
		if err := WriteSkills(skills, nil, []string{copilotTarget}, []string{repoDir}); err != nil {
			t.Fatalf("WriteSkills: %v", err)
		}
		assertFileExists(t, skillMDPath(filepath.Join(repoDir, ".github"), "grp", "proj-skill"))
	})

	t.Run("multiple tools write to correct project dirs", func(t *testing.T) {
		skills := []Skill{skillWithMD("multi-skill", "multi-skill", "grp", "# x")}
		skills[0].Location = SkillLocationProject
		if err := WriteSkills(skills, nil, []string{codexTarget, copilotTarget}, []string{repoDir}); err != nil {
			t.Fatalf("WriteSkills: %v", err)
		}
		assertFileExists(t, skillMDPath(filepath.Join(repoDir, ".codex"), "grp", "multi-skill"))
		assertFileExists(t, skillMDPath(filepath.Join(repoDir, ".github"), "grp", "multi-skill"))
	})
}
