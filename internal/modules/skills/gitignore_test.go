package skills

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSkills_AddsProjectSkillDirToGitignore(t *testing.T) {
	mod, _, baseDir := newTestModule(t)
	repo := initGitRepo(t, filepath.Join(baseDir, "repo"))
	fetched := fetchedProjectSkill()

	_, err := mod.LoadSkills(context.Background(), LoadSkillsOptions{
		SelectedGroups:      []string{"platform"},
		TargetOverrides:     []string{filepath.Join(baseDir, ".cursor")},
		ProjectDirOverrides: []string{repo},
		Fetched:             fetched,
		NoSave:              true,
	})
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}

	assertFileExists(t, filepath.Join(repo, ".cursor", "skills", "project-skill", "SKILL.md"))
	assertFileContent(t, filepath.Join(repo, ".gitignore"), ".cursor/skills/\n")
}

func TestLoadSkills_DoesNotDuplicateExistingProjectSkillGitignoreEntry(t *testing.T) {
	mod, _, baseDir := newTestModule(t)
	repo := initGitRepo(t, filepath.Join(baseDir, "repo"))
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".cursor/skills/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		_, err := mod.LoadSkills(context.Background(), LoadSkillsOptions{
			SelectedGroups:      []string{"platform"},
			TargetOverrides:     []string{filepath.Join(baseDir, ".cursor")},
			ProjectDirOverrides: []string{repo},
			Fetched:             fetchedProjectSkill(),
			NoSave:              true,
		})
		if err != nil {
			t.Fatalf("LoadSkills run %d: %v", i+1, err)
		}
	}

	assertFileContent(t, filepath.Join(repo, ".gitignore"), ".cursor/skills/\n")
}

func TestLoadSkills_SkipsGitignoreWhenProjectSkillDirAlreadyIgnored(t *testing.T) {
	mod, _, baseDir := newTestModule(t)
	repo := initGitRepo(t, filepath.Join(baseDir, "repo"))
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".cursor/skills/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := mod.LoadSkills(context.Background(), LoadSkillsOptions{
		SelectedGroups:      []string{"platform"},
		TargetOverrides:     []string{filepath.Join(baseDir, ".cursor")},
		ProjectDirOverrides: []string{repo},
		Fetched:             fetchedProjectSkill(),
		NoSave:              true,
	})
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}

	assertFileContent(t, filepath.Join(repo, ".gitignore"), ".cursor/skills/\n")
}

func TestLoadSkills_NoGitignoreLeavesProjectGitignoreUnchanged(t *testing.T) {
	mod, _, baseDir := newTestModule(t)
	repo := initGitRepo(t, filepath.Join(baseDir, "repo"))

	_, err := mod.LoadSkills(context.Background(), LoadSkillsOptions{
		SelectedGroups:      []string{"platform"},
		TargetOverrides:     []string{filepath.Join(baseDir, ".cursor")},
		ProjectDirOverrides: []string{repo},
		Fetched:             fetchedProjectSkill(),
		NoGitignore:         true,
		NoSave:              true,
	})
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}

	assertFileExists(t, filepath.Join(repo, ".cursor", "skills", "project-skill", "SKILL.md"))
	assertFileAbsent(t, filepath.Join(repo, ".gitignore"))
}

func TestLoadSkills_GitignoreFailureReturnsWarningNotError(t *testing.T) {
	mod, _, baseDir := newTestModule(t)
	repo := initGitRepo(t, filepath.Join(baseDir, "repo"))
	if err := os.Mkdir(filepath.Join(repo, ".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := mod.LoadSkills(context.Background(), LoadSkillsOptions{
		SelectedGroups:      []string{"platform"},
		TargetOverrides:     []string{filepath.Join(baseDir, ".cursor")},
		ProjectDirOverrides: []string{repo},
		Fetched:             fetchedProjectSkill(),
		NoSave:              true,
	})
	if err != nil {
		t.Fatalf("LoadSkills should warn instead of failing: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want one warning", result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], ".gitignore") {
		t.Fatalf("warning should mention .gitignore, got %q", result.Warnings[0])
	}
}

func TestLoadSkills_NonGitProjectDirDoesNotCreateGitignore(t *testing.T) {
	mod, _, baseDir := newTestModule(t)
	projectDir := filepath.Join(baseDir, "repo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := mod.LoadSkills(context.Background(), LoadSkillsOptions{
		SelectedGroups:      []string{"platform"},
		TargetOverrides:     []string{filepath.Join(baseDir, ".cursor")},
		ProjectDirOverrides: []string{projectDir},
		Fetched:             fetchedProjectSkill(),
		NoSave:              true,
	})
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}

	assertFileExists(t, filepath.Join(projectDir, ".cursor", "skills", "project-skill", "SKILL.md"))
	assertFileAbsent(t, filepath.Join(projectDir, ".gitignore"))
}

func TestLoadSkills_GlobalOnlyDoesNotCreateGitignore(t *testing.T) {
	mod, _, baseDir := newTestModule(t)
	repo := initGitRepo(t, filepath.Join(baseDir, "repo"))
	globalTarget := filepath.Join(repo, ".cursor")
	fetched := &FetchedSkills{
		Groups: []SkillGroup{{Identifier: "platform", SkillIDs: []string{"global-skill"}}},
		Skills: []Skill{
			{
				Identifier: "global-skill",
				GroupIDs:   []string{"platform"},
				Location:   SkillLocationGlobal,
				Files:      []SkillFile{{Path: "SKILL.md", Content: "# global"}},
			},
		},
	}

	_, err := mod.LoadSkills(context.Background(), LoadSkillsOptions{
		SelectedGroups:  []string{"platform"},
		TargetOverrides: []string{globalTarget},
		Fetched:         fetched,
		NoSave:          true,
	})
	if err != nil {
		t.Fatalf("LoadSkills: %v", err)
	}

	assertFileExists(t, filepath.Join(globalTarget, "skills", "global-skill", "SKILL.md"))
	assertFileAbsent(t, filepath.Join(repo, ".gitignore"))
}

func fetchedProjectSkill() *FetchedSkills {
	return &FetchedSkills{
		Groups: []SkillGroup{{Identifier: "platform", SkillIDs: []string{"project-skill"}}},
		Skills: []Skill{
			{
				Identifier: "project-skill",
				GroupIDs:   []string{"platform"},
				Location:   SkillLocationProject,
				Files:      []SkillFile{{Path: "SKILL.md", Content: "# project"}},
			},
		},
	}
}

func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}
