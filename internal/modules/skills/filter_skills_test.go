package skills

import "testing"

func TestFilterSkills(t *testing.T) {
	tests := []struct {
		name               string
		fetched            *FetchedSkills
		selectAll          bool
		selectAllGroups    bool
		selectAllUngrouped bool
		selectedGroups     []string
		selectedSkills     []string
		wantIDs            []string
	}{
		{
			name: "SelectAll includes everything",
			fetched: &FetchedSkills{
				Skills: []Skill{{Identifier: "opt-1", GroupIDs: []string{"group-a"}}, {Identifier: "opt-2"}},
			},
			selectAll: true,
			wantIDs:   []string{"opt-1", "opt-2"},
		},
		{
			name: "no selection returns nothing",
			fetched: &FetchedSkills{
				Skills: []Skill{{Identifier: "opt-1", GroupIDs: []string{"group-a"}}},
			},
			wantIDs: nil,
		},
		{
			name: "SelectAllGroups includes grouped only",
			fetched: &FetchedSkills{
				Skills: []Skill{{Identifier: "opt-grouped", GroupIDs: []string{"group-a"}}, {Identifier: "opt-ungrouped"}},
			},
			selectAllGroups: true,
			wantIDs:         []string{"opt-grouped"},
		},
		{
			name: "SelectAllUngrouped includes ungrouped only",
			fetched: &FetchedSkills{
				Skills: []Skill{
					{Identifier: "grouped", GroupIDs: []string{"group-a"}},
					{Identifier: "ungrouped-1"},
					{Identifier: "ungrouped-2"},
				},
			},
			selectAllUngrouped: true,
			wantIDs:            []string{"ungrouped-1", "ungrouped-2"},
		},
		{
			name: "specific groups",
			fetched: &FetchedSkills{
				Skills: []Skill{
					{Identifier: "skill-a", GroupIDs: []string{"group-a"}},
					{Identifier: "skill-b", GroupIDs: []string{"group-b"}},
					{Identifier: "skill-c", GroupIDs: []string{"group-c"}},
				},
			},
			selectedGroups: []string{"group-a", "group-b"},
			wantIDs:        []string{"skill-a", "skill-b"},
		},
		{
			name: "specific skills",
			fetched: &FetchedSkills{
				Skills: []Skill{
					{Identifier: "skill-1"},
					{Identifier: "skill-2"},
					{Identifier: "skill-3"},
				},
			},
			selectedSkills: []string{"skill-1", "skill-3"},
			wantIDs:        []string{"skill-1", "skill-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterSkills(tt.fetched, tt.selectAll, tt.selectAllGroups, tt.selectAllUngrouped, tt.selectedGroups, tt.selectedSkills, false)
			ids := identifiers(result)
			if len(ids) != len(tt.wantIDs) {
				t.Fatalf("want %d skills (%v), got %d (%v)", len(tt.wantIDs), tt.wantIDs, len(ids), ids)
			}
			for _, want := range tt.wantIDs {
				if !contains(ids, want) {
					t.Errorf("expected %q in result %v", want, ids)
				}
			}
		})
	}
}

func TestFilterOrphanSkillFiles_IgnoresStandaloneFilesInAnySkillsFolder(t *testing.T) {
	files := filterOrphanSkillFiles(Skill{Identifier: "real-skill", Title: "real-skill"}, []SkillFile{
		{Path: ".cursor/skills/standalone-file", Content: "ignored"},
		{Path: ".cursor/skills/port/standalone-file", Content: "ignored"},
		{Path: ".cursor/skills/engineering/real-skill/SKILL.md", Content: "kept"},
		{Path: ".cursor/skills/real-skill/references/guide.md", Content: "also kept"},
		{Path: "scripts/run.sh", Content: "relative path kept"},
	})

	if len(files) != 3 {
		t.Fatalf("want 3 files, got %+v", files)
	}
	if files[0].Path != ".cursor/skills/engineering/real-skill/SKILL.md" {
		t.Fatalf("unexpected first file: %+v", files[0])
	}
	if files[1].Path != ".cursor/skills/real-skill/references/guide.md" {
		t.Fatalf("unexpected second file: %+v", files[1])
	}
	if files[2].Path != "scripts/run.sh" {
		t.Fatalf("unexpected third file: %+v", files[2])
	}
}
