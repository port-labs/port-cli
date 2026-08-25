package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func skillWithMD(id, title, groupID, body string) Skill {
	s := Skill{
		Identifier: id,
		Title:      title,
		GroupIDs:   []string{groupID},
		Files:      []SkillFile{{Path: "SKILL.md", Content: body}},
	}
	if groupID == "" {
		s.GroupIDs = nil
	}
	return s
}

func TestWriteSkills_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	skills := []Skill{
		skillWithMD("my-skill", "my-skill", "my-group", "---\nname: my-skill\ndescription: does stuff\n---\n\nstep 1\nstep 2\n"),
	}
	if err := WriteSkills(skills, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	content, err := os.ReadFile(skillMDPath(dir, "my-group", "my-skill"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	body := string(content)
	for _, want := range []string{"name: my-skill", "description: does stuff", "step 1"} {
		if !containsStr(body, want) {
			t.Errorf("SKILL.md missing %q", want)
		}
	}
}

func TestWriteSkills_NormalizesSkillDirectoryAndFrontmatterNameFromIdentifier(t *testing.T) {
	dir := t.TempDir()
	skills := []Skill{
		{
			Identifier:  "deploy_helper",
			Title:       "Deploy Helper",
			Description: "Deploys services safely.",
			GroupIDs:    []string{"platform"},
			Files: []SkillFile{
				{Path: "SKILL.md", Content: "---\ndescription: Deploys services safely.\n---\nRun the deploy steps."},
			},
		},
	}

	if err := WriteSkills(skills, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	content, err := os.ReadFile(skillMDPath(dir, "platform", "deploy-helper"))
	if err != nil {
		t.Fatalf("read normalized SKILL.md: %v", err)
	}
	body := string(content)
	for _, want := range []string{"name: deploy-helper", "description: Deploys services safely.", "Run the deploy steps."} {
		if !containsStr(body, want) {
			t.Errorf("SKILL.md missing %q", want)
		}
	}
	assertFileAbsent(t, filepath.Join(dir, "skills", "Deploy Helper"))
}

func TestWriteSkills_UsesExplicitSkillNameForLongPortIdentifier(t *testing.T) {
	dir := t.TempDir()
	longIdentifier := "customer-platform-observability-data-pipeline-runtime-change-review-automation"
	if len(longIdentifier) <= 64 {
		t.Fatalf("test identifier must be longer than 64 characters, got %d", len(longIdentifier))
	}
	skills := []Skill{
		{
			Identifier:  longIdentifier,
			Title:       "Network Core Architecture",
			Description: "Documents network core architecture.",
			GroupIDs:    []string{"platform"},
			Files: []SkillFile{
				{Path: "SKILL.md", Content: "---\nname: network-core\ndescription: Documents network core architecture.\n---\n\n# Network core architecture"},
			},
		},
	}

	if err := WriteSkills(skills, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	content, err := os.ReadFile(skillMDPath(dir, "platform", "network-core"))
	if err != nil {
		t.Fatalf("read explicit-name SKILL.md: %v", err)
	}
	body := string(content)
	for _, want := range []string{"name: network-core", "description: Documents network core architecture.", "# Network core architecture"} {
		if !strings.Contains(body, want) {
			t.Errorf("SKILL.md missing %q", want)
		}
	}
}

func TestWriteSkills_ParsesQuotedSkillNameWithInlineComment(t *testing.T) {
	dir := t.TempDir()
	longIdentifier := "customer-platform-observability-data-pipeline-runtime-change-review-automation"
	skills := []Skill{
		{
			Identifier: longIdentifier,
			Title:      "Network Core Architecture",
			GroupIDs:   []string{"platform"},
			Files: []SkillFile{
				{Path: "SKILL.md", Content: "---\nname: \"network-core\" # synced from Port\ndescription: Documents network core architecture.\n---\n\n# Network core architecture"},
			},
		},
	}

	if err := WriteSkills(skills, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	assertFileExists(t, skillMDPath(dir, "platform", "network-core"))
}

func TestFrontmatterSkillName(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "finds header after another delimiter block",
			content: "# Notes\n\n---\nnot: skill metadata\n---\n\n---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
			want:    "deploy-service",
		},
		{
			name:    "allows extra header fields",
			content: "---\nname: deploy-service\ndescription: Deploy service\nallowed-tools: bash\nlicense: MIT\n---\n\nDeploy it.",
			want:    "deploy-service",
		},
		{
			name:    "ignores missing description",
			content: "---\nname: deploy-service\n---\n\nDeploy it.",
		},
		{
			name:    "ignores unicode skill name",
			content: "---\nname: déployer\ndescription: Déployer le service\n---\n\nDeploy it.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frontmatterSkillName(tt.content); got != tt.want {
				t.Fatalf("frontmatterSkillName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFrontmatterDescription(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "extracts description from the leading block",
			content: "---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
			want:    "Deploy service",
		},
		{
			name:    "finds description in a block after another delimiter section",
			content: "# Notes\n\n---\nnot: skill metadata\n---\n\n---\nname: deploy-service\ndescription: Deploy service\n---\n\nDeploy it.",
			want:    "Deploy service",
		},
		{
			name:    "handles quoted description with inline comment",
			content: "---\nname: deploy-service\ndescription: \"Deploy service\" # from Port\n---\n\nDeploy it.",
			want:    "Deploy service",
		},
		{
			name:    "returns empty when no description is present",
			content: "---\nname: deploy-service\n---\n\nDeploy it.",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frontmatterDescription(tt.content); got != tt.want {
				t.Fatalf("frontmatterDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteSkills_UngroupedUsesNoGroupDir(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSkills([]Skill{skillWithMD("solo-skill", "solo-skill", "", "# Solo")}, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	assertFileExists(t, skillMDPath(dir, "", "solo-skill"))
}

func TestWriteSkills_WritesBundledFiles(t *testing.T) {
	dir := t.TempDir()
	skills := []Skill{
		{
			Identifier: "skill-files",
			Title:      "skill-files",
			GroupIDs:   []string{"grp"},
			Files: []SkillFile{
				{Path: "SKILL.md", Content: "# Skill"},
				{Path: "references/guide.md", Content: "# Guide"},
				{Path: "assets/config.yaml", Content: "key: value"},
				{Path: "scripts/run.sh", Content: "#!/bin/sh\n"},
				{Path: "NOTICE", Content: "MIT"},
			},
		},
	}
	if err := WriteSkills(skills, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	base := filepath.Join(dir, "skills", "skill-files")
	assertFileExists(t, filepath.Join(base, "references", "guide.md"))
	assertFileExists(t, filepath.Join(base, "assets", "config.yaml"))
	assertFileExists(t, filepath.Join(base, "scripts", "run.sh"))
	assertFileExists(t, filepath.Join(base, "NOTICE"))
}

func TestWriteSkills_DefaultSyncTargetsWritesAgentsAndClaude(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	targets := TargetPaths(DefaultSyncTargets(), home, repo)
	skills := []Skill{skillWithMD("sk", "sk", "g", "# x")}
	if err := WriteSkills(skills, nil, targets, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	for _, target := range targets {
		assertFileExists(t, skillMDPath(target, "g", "sk"))
	}
}

func TestWriteSkills_MultipleTargets(t *testing.T) {
	dir1, dir2 := t.TempDir(), t.TempDir()
	skills := []Skill{skillWithMD("sk", "sk", "g", "# x")}
	if err := WriteSkills(skills, nil, []string{dir1, dir2}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	assertFileExists(t, skillMDPath(dir1, "g", "sk"))
	assertFileExists(t, skillMDPath(dir2, "g", "sk"))
}

func TestWriteSkills_ReconcileRemovesStaleSkillAndEmptyGroup(t *testing.T) {
	dir := t.TempDir()
	initial := []Skill{
		skillWithMD("keep", "keep", "grp", "# keep"),
		skillWithMD("stale", "stale", "grp", "# stale"),
		skillWithMD("sk", "sk", "gone-group", "# z"),
	}
	if err := WriteSkills(initial, nil, []string{dir}, nil); err != nil {
		t.Fatalf("initial WriteSkills: %v", err)
	}

	updated := []Skill{skillWithMD("keep", "keep", "grp", "# keep")}
	if err := WriteSkills(updated, nil, []string{dir}, nil); err != nil {
		t.Fatalf("second WriteSkills: %v", err)
	}

	assertFileAbsent(t, filepath.Join(dir, "skills", "stale"))
	assertFileAbsent(t, filepath.Join(dir, "skills", "gone-group"))
	assertFileExists(t, skillMDPath(dir, "grp", "keep"))
}

func TestWriteSkills_ReconcileKeepsUserSkillDirs(t *testing.T) {
	dir := t.TempDir()
	userSkillDir := filepath.Join(dir, "skills", "manual-skill")
	if err := os.MkdirAll(userSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userSkillDir, "SKILL.md"), []byte("# manual"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteSkills([]Skill{skillWithMD("keep", "keep", "grp", "# keep")}, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	assertFileExists(t, filepath.Join(userSkillDir, "SKILL.md"))
}

func TestWriteSkills_RemovesLegacyPortTreeAfterSuccessfulSync(t *testing.T) {
	dir := t.TempDir()
	legacyDir := filepath.Join(dir, "skills", PortSkillsDir, "grp", "old-skill")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "SKILL.md"), []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteSkills([]Skill{skillWithMD("new-skill", "new-skill", "grp", "# new")}, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	assertFileExists(t, skillMDPath(dir, "grp", "new-skill"))
	assertFileAbsent(t, filepath.Join(dir, "skills", PortSkillsDir))
}

func TestWriteSkills_MultiGroupSkillWrittenOnce(t *testing.T) {
	dir := t.TempDir()
	skills := []Skill{
		{
			Identifier: "shared-skill",
			Title:      "shared-skill",
			GroupIDs:   []string{"group-a", "group-b"},
			Files:      []SkillFile{{Path: "SKILL.md", Content: "# x"}},
		},
	}
	if err := WriteSkills(skills, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	assertFileExists(t, skillMDPath(dir, "group-a", "shared-skill"))
	assertFileAbsent(t, filepath.Join(dir, "skills", "group-a"))
	assertFileAbsent(t, filepath.Join(dir, "skills", "group-b"))
}

func TestWriteSkills_WritesFilesUnderSpecSafeSkillName(t *testing.T) {
	dir := t.TempDir()
	skills := []Skill{
		{
			Identifier: "org/platform/deploy-helper",
			Title:      "Deploy Helper",
			GroupIDs:   []string{"org/platform"},
			Files: []SkillFile{
				{Path: "SKILL.md", Content: "versioned skill"},
				{Path: "references/runbook.md", Content: "# Runbook"},
			},
		},
	}
	groups := []SkillGroup{{Identifier: "org/platform", Title: "platform"}}

	if err := WriteSkills(skills, groups, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	assertFileContent(t, filepath.Join(dir, "skills", "deploy-helper", "SKILL.md"), "---\nname: deploy-helper\ndescription: Port skill deploy-helper.\n---\n\nversioned skill")
	assertFileContent(t, filepath.Join(dir, "skills", "deploy-helper", "references", "runbook.md"), "# Runbook")
}

func TestWriteSkills_NormalizesSourceStylePathsUsingSkillTitle(t *testing.T) {
	dir := t.TempDir()
	skills := []Skill{
		{
			Identifier: "org/platform/deploy-helper",
			Title:      "deploy-helper",
			GroupIDs:   []string{"org/platform"},
			Files: []SkillFile{
				{Path: ".cursor/skills/engineering/deploy-helper/SKILL.md", Content: "source style path"},
			},
		},
	}
	groups := []SkillGroup{{Identifier: "org/platform", Title: "platform"}}

	if err := WriteSkills(skills, groups, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	assertFileContent(t, filepath.Join(dir, "skills", "deploy-helper", "SKILL.md"), "---\nname: deploy-helper\ndescription: Port skill deploy-helper.\n---\n\nsource style path")
	assertFileAbsent(t, filepath.Join(dir, "skills", "deploy-helper", "engineering"))
}

func TestWriteSkills_NormalizesSourceStylePathsUsingIdentifierBase(t *testing.T) {
	dir := t.TempDir()
	skills := []Skill{
		{
			Identifier: "org/platform/deploy-helper",
			Title:      "Deploy Helper",
			GroupIDs:   []string{"org/platform"},
			Files: []SkillFile{
				{Path: ".cursor/skills/engineering/deploy-helper/SKILL.md", Content: "source style path"},
			},
		},
	}
	groups := []SkillGroup{{Identifier: "org/platform", Title: "platform"}}

	if err := WriteSkills(skills, groups, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	assertFileContent(t, filepath.Join(dir, "skills", "deploy-helper", "SKILL.md"), "---\nname: deploy-helper\ndescription: Port skill deploy-helper.\n---\n\nsource style path")
	assertFileAbsent(t, filepath.Join(dir, "skills", "deploy-helper", "engineering"))
}

func TestWriteSkills_IgnoresSourceStyleOrphanFiles(t *testing.T) {
	dir := t.TempDir()
	skills := []Skill{
		{
			Identifier: "deploy-helper",
			Title:      "deploy-helper",
			GroupIDs:   []string{"platform"},
			Files: []SkillFile{
				{Path: ".cursor/skills/engineering/orphan-file", Content: "ignored"},
				{Path: "SKILL.md", Content: "kept"},
			},
		},
	}

	if err := WriteSkills(skills, nil, []string{dir}, nil); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}
	assertFileContent(t, filepath.Join(dir, "skills", "deploy-helper", "SKILL.md"), "---\nname: deploy-helper\ndescription: Port skill deploy-helper.\n---\n\nkept")
	assertFileAbsent(t, filepath.Join(dir, "skills", "deploy-helper", "orphan-file"))
}

func TestWriteSkills_GlobalAndProjectSamePortDirPreservesBoth(t *testing.T) {
	workdir := t.TempDir()
	cursorTarget := filepath.Join(workdir, ".cursor")
	global := skillWithMD("global-skill", "global-skill", "grp-a", "name: global-skill\n---\n# Global")
	global.Location = SkillLocationGlobal
	project := skillWithMD("project-skill", "project-skill", "grp-b", "name: project-skill\n---\n# Project")
	project.Location = SkillLocationProject

	if err := WriteSkills(
		[]Skill{global, project},
		[]SkillGroup{{Identifier: "grp-a"}, {Identifier: "grp-b"}},
		[]string{cursorTarget},
		[]string{workdir},
	); err != nil {
		t.Fatalf("WriteSkills: %v", err)
	}

	assertFileExists(t, skillMDPath(cursorTarget, "grp-a", "global-skill"))
	assertFileExists(t, skillMDPath(cursorTarget, "grp-b", "project-skill"))
}
