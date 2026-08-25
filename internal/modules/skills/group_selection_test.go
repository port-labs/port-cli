package skills

import (
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
)

func TestGroupSelectionFromCatalog(t *testing.T) {
	groups := []api.SkillGroupAtLatestVersion{
		{Identifier: "g-team", MatchesUserTeams: true},
		{Identifier: "g-other", MatchesUserTeams: false},
		{Identifier: "g-team2", MatchesUserTeams: true},
	}
	include, exclude := GroupSelectionFromCatalog(groups, []string{"g-team", "g-other"})
	if len(include) != 1 || include[0] != "g-other" {
		t.Fatalf("include: %v", include)
	}
	if len(exclude) != 1 || exclude[0] != "g-team2" {
		t.Fatalf("exclude: %v", exclude)
	}
}

func TestInitialSelectedGroupIDs_TeamIncludeExclude(t *testing.T) {
	groups := []api.SkillGroupAtLatestVersion{
		{Identifier: "operations", MatchesUserTeams: true},
		{Identifier: "platform-engineering", MatchesUserTeams: false},
		{Identifier: "security", MatchesUserTeams: false},
	}
	cfg := &config.SkillsConfig{
		TeamGroupDefaults: true,
		IncludeGroups:     []string{"security"},
		ExcludeGroups:     []string{"operations"},
		Targets:           []string{"/tmp/.cursor"},
	}
	got := InitialSelectedGroupIDs(groups, cfg)
	want := []string{"security"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestGroupSyncIntents(t *testing.T) {
	groups := []api.SkillGroupAtLatestVersion{
		{Identifier: "g-team", MatchesUserTeams: true},
		{Identifier: "g-extra", MatchesUserTeams: false},
	}
	cfg := &config.SkillsConfig{
		TeamGroupDefaults: true,
		IncludeGroups:     []string{"g-extra"},
		ExcludeGroups:     []string{"g-team"},
		Targets:           []string{"/tmp"},
	}
	initial := InitialSelectedGroupIDs(groups, cfg)
	intents := GroupSyncIntents(groups, cfg, initial)
	if !intents["g-extra"].SavedInclude || intents["g-extra"].InitiallySync != true {
		t.Fatalf("g-extra: %+v", intents["g-extra"])
	}
	if !intents["g-team"].SavedExclude || intents["g-team"].InitiallySync != false {
		t.Fatalf("g-team: %+v", intents["g-team"])
	}
}

func TestInitialUngroupedSelection(t *testing.T) {
	cfg := &config.SkillsConfig{
		SelectAllUngrouped: false,
		SelectedSkills:     []string{"integrations-overview"},
	}
	all, ids := InitialUngroupedSelection(cfg)
	if all || len(ids) != 1 || ids[0] != "integrations-overview" {
		t.Fatalf("got all=%v ids=%v", all, ids)
	}
}
