package skills

import (
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestInitCatalogStatsFrom(t *testing.T) {
	catalog := &FetchedSkills{
		Groups: []SkillGroup{
			{Identifier: "g1", Title: "Group One", SkillIDs: []string{"a", "b"}},
			{Identifier: "g2", SkillIDs: []string{"c"}},
		},
		Skills: []Skill{
			{Identifier: "a", Title: "Skill A", Version: "2.0.0", GroupIDs: []string{"g1"}},
			{Identifier: "b", Title: "Skill B", Version: "1.0.0", GroupIDs: []string{"g1"}},
			{Identifier: "c", Title: "Skill C", Version: "3.0.0", GroupIDs: []string{"g2"}},
			{Identifier: "solo", Title: "Solo", Version: "1.0.0"},
		},
	}
	stats := InitCatalogStatsFrom([]api.SkillGroupAtLatestVersion{
		{Identifier: "g1", Title: "Group One"},
		{Identifier: "g2", Title: "Group Two"},
	}, catalog)

	if stats.GroupCount != 2 || stats.UngroupedCount != 1 {
		t.Fatalf("counts: groups=%d ungrouped=%d", stats.GroupCount, stats.UngroupedCount)
	}
	if len(stats.Groups[0].Skills) != 2 || stats.Groups[0].Skills[0].Version != "2.0.0" {
		t.Fatalf("g1 skills: %+v", stats.Groups[0].Skills)
	}
	if len(stats.Ungrouped) != 1 || stats.Ungrouped[0].Identifier != "solo" {
		t.Fatalf("ungrouped: %+v", stats.Ungrouped)
	}
}
