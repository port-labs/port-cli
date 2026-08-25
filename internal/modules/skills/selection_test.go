package skills

import (
	"testing"

	"github.com/port-labs/port-cli/internal/config"
)

func TestApplySelectionToConfig_ReplaceSelection(t *testing.T) {
	cfg := &config.SkillsConfig{
		SelectAll:          true,
		SelectAllGroups:    true,
		SelectAllUngrouped: true,
		SelectedGroups:     []string{"old-group"},
		SelectedSkills:     []string{"old-skill"},
	}

	applySelectionToConfig(cfg, LoadSkillsOptions{
		ReplaceSelection:   true,
		SelectedGroups:     []string{"new-group"},
		SelectedSkills:     []string{"new-skill"},
		SelectAll:          false,
		SelectAllGroups:    false,
		SelectAllUngrouped: false,
	})

	if cfg.SelectAll || cfg.SelectAllGroups || cfg.SelectAllUngrouped {
		t.Fatalf("expected select-all flags cleared, got %+v", cfg)
	}
	if len(cfg.SelectedGroups) != 1 || cfg.SelectedGroups[0] != "new-group" {
		t.Fatalf("SelectedGroups: %v", cfg.SelectedGroups)
	}
	if len(cfg.SelectedSkills) != 1 || cfg.SelectedSkills[0] != "new-skill" {
		t.Fatalf("SelectedSkills: %v", cfg.SelectedSkills)
	}
}

func TestMergeSelection_AddsGroupsAndSkills(t *testing.T) {
	fetched := &FetchedSkills{
		Groups: []SkillGroup{
			{Identifier: "group-a"},
			{Identifier: "group-b"},
		},
		Skills: []Skill{
			{Identifier: "skill-1"},
			{Identifier: "skill-2", GroupIDs: []string{"group-a"}},
		},
	}
	cfg := &config.SkillsConfig{
		SelectedGroups: []string{"group-a"},
		SelectedSkills: []string{"skill-1"},
	}

	result, err := MergeSelection(cfg, fetched, []string{"group-b"}, []string{"skill-1"})
	if err != nil {
		t.Fatalf("MergeSelection: %v", err)
	}
	if !result.HasChanges() {
		t.Fatal("expected changes")
	}
	if len(result.AddedGroups) != 1 || result.AddedGroups[0] != "group-b" {
		t.Errorf("AddedGroups: %v", result.AddedGroups)
	}
	if len(result.AddedSkills) != 0 {
		t.Errorf("AddedSkills: %v", result.AddedSkills)
	}
	if len(result.SkippedSkills) != 1 || result.SkippedSkills[0] != "skill-1" {
		t.Errorf("expected skill-1 skipped (already selected), got skips=%v", result.SkippedSkills)
	}
	if !contains(cfg.SelectedGroups, "group-b") {
		t.Errorf("config groups: %v", cfg.SelectedGroups)
	}
}

func TestMergeSelection_SkipsAlreadySelected(t *testing.T) {
	fetched := &FetchedSkills{
		Groups: []SkillGroup{{Identifier: "group-a"}},
		Skills: []Skill{{Identifier: "skill-1"}},
	}
	cfg := &config.SkillsConfig{
		SelectAllGroups:    true,
		SelectAllUngrouped: true,
	}

	result, err := MergeSelection(cfg, fetched, []string{"group-a"}, []string{"skill-1"})
	if err != nil {
		t.Fatalf("MergeSelection: %v", err)
	}
	if result.HasChanges() {
		t.Fatal("expected no changes")
	}
	if len(result.SkippedGroups) != 1 || len(result.SkippedSkills) != 1 {
		t.Errorf("skips: groups=%v skills=%v", result.SkippedGroups, result.SkippedSkills)
	}
}

func TestMergeSelection_UnknownIdentifiers(t *testing.T) {
	cfg := &config.SkillsConfig{}
	_, err := MergeSelection(cfg, &FetchedSkills{}, []string{"missing-group"}, []string{"missing-skill"})
	if err == nil {
		t.Fatal("expected error for unknown identifiers")
	}
}

func TestAvailableGroupsToAdd(t *testing.T) {
	fetched := &FetchedSkills{
		Groups: []SkillGroup{
			{Identifier: "a"},
			{Identifier: "b"},
		},
	}
	cfg := &config.SkillsConfig{SelectedGroups: []string{"a"}}

	got := AvailableGroupsToAdd(cfg, fetched)
	if len(got) != 1 || got[0].Identifier != "b" {
		t.Errorf("AvailableGroupsToAdd: got %v", got)
	}
}

func TestAvailableGroupsToAdd_TeamDefaultsExcludeAlreadySyncedGroups(t *testing.T) {
	fetched := &FetchedSkills{
		Groups: []SkillGroup{
			{Identifier: "team-owned", MatchesUserTeams: true},
			{Identifier: "included", MatchesUserTeams: false},
			{Identifier: "excluded", MatchesUserTeams: true},
			{Identifier: "available", MatchesUserTeams: false},
		},
	}
	cfg := &config.SkillsConfig{
		TeamGroupDefaults: true,
		IncludeGroups:     []string{"included"},
		ExcludeGroups:     []string{"excluded"},
	}

	got := AvailableGroupsToAdd(cfg, fetched)
	ids := make([]string, 0, len(got))
	for _, group := range got {
		ids = append(ids, group.Identifier)
	}
	if !equalStrings(ids, []string{"excluded", "available"}) {
		t.Fatalf("AvailableGroupsToAdd: got %v", ids)
	}
}

func TestAvailableSkillsToAdd_TeamDefaultsExcludeSkillsCoveredBySyncedGroups(t *testing.T) {
	fetched := &FetchedSkills{
		Groups: []SkillGroup{
			{Identifier: "team-owned", MatchesUserTeams: true},
			{Identifier: "included", MatchesUserTeams: false},
			{Identifier: "excluded", MatchesUserTeams: true},
		},
		Skills: []Skill{
			{Identifier: "team-skill", GroupIDs: []string{"team-owned"}},
			{Identifier: "included-skill", GroupIDs: []string{"included"}},
			{Identifier: "excluded-skill", GroupIDs: []string{"excluded"}},
			{Identifier: "available-skill", GroupIDs: []string{"available"}},
		},
	}
	cfg := &config.SkillsConfig{
		TeamGroupDefaults: true,
		IncludeGroups:     []string{"included"},
		ExcludeGroups:     []string{"excluded"},
	}

	got := AvailableSkillsToAdd(cfg, fetched)
	ids := make([]string, 0, len(got))
	for _, skill := range got {
		ids = append(ids, skill.Identifier)
	}
	if !equalStrings(ids, []string{"excluded-skill", "available-skill"}) {
		t.Fatalf("AvailableSkillsToAdd: got %v", ids)
	}
}

func TestMergeSelection_TeamDefaultsSkipTeamOwnedGroupAndSkill(t *testing.T) {
	fetched := &FetchedSkills{
		Groups: []SkillGroup{{Identifier: "team-owned", MatchesUserTeams: true}},
		Skills: []Skill{{Identifier: "team-skill", GroupIDs: []string{"team-owned"}}},
	}
	cfg := &config.SkillsConfig{TeamGroupDefaults: true}

	result, err := MergeSelection(cfg, fetched, []string{"team-owned"}, []string{"team-skill"})
	if err != nil {
		t.Fatalf("MergeSelection: %v", err)
	}
	if result.HasChanges() {
		t.Fatalf("expected no changes, got %+v cfg=%+v", result, cfg)
	}
	if !equalStrings(result.SkippedGroups, []string{"team-owned"}) {
		t.Fatalf("SkippedGroups: %v", result.SkippedGroups)
	}
	if !equalStrings(result.SkippedSkills, []string{"team-skill"}) {
		t.Fatalf("SkippedSkills: %v", result.SkippedSkills)
	}
}
