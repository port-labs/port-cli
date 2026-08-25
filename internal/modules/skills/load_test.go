package skills

import (
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

func TestUngroupedSkills(t *testing.T) {
	fetched := &FetchedSkills{
		Groups: []SkillGroup{{Identifier: "g1", SkillIDs: []string{"in-group", "also-grouped"}}},
		Skills: []Skill{
			{Identifier: "in-group", GroupIDs: []string{"g1"}},
			{Identifier: "standalone", GroupIDs: nil},
			{Identifier: "also-grouped", GroupIDs: nil}, // empty GroupIDs but listed in group
		},
	}
	got := UngroupedSkills(fetched)
	if len(got) != 1 || got[0].Identifier != "standalone" {
		t.Fatalf("got %+v", got)
	}
}

func TestUngroupedSkills_ExcludesGroupedListedAsUngrouped(t *testing.T) {
	catalog := CatalogFromAPI(&api.GroupedSkillsResponse{
		Groups: []api.SkillGroupAtLatestVersion{{
			Identifier: "platform-engineering",
			Skills: []api.SkillAtLatestVersion{
				{Identifier: "local-dev-setup"},
				{Identifier: "port-api-client"},
			},
		}},
		UngroupedSkills: []api.SkillAtLatestVersion{
			{Identifier: "local-dev-setup"},
			{Identifier: "port-api-client"},
			{Identifier: "integrations-overview"},
		},
	})
	got := UngroupedSkills(catalog)
	if len(got) != 1 || got[0].Identifier != "integrations-overview" {
		t.Fatalf("got %+v", got)
	}
}

func TestCatalogFromAPI_GroupIdentifiersOnUngrouped(t *testing.T) {
	catalog := CatalogFromAPI(&api.GroupedSkillsResponse{
		Groups: []api.SkillGroupAtLatestVersion{},
		UngroupedSkills: []api.SkillAtLatestVersion{
			{
				Identifier:       "local-dev-setup",
				Title:            "Local dev setup",
				GroupIdentifiers: []string{"platform-engineering"},
			},
		},
	})
	if len(catalog.Skills) != 1 {
		t.Fatalf("skills: %+v", catalog.Skills)
	}
	if got := catalog.Skills[0].GroupIDs; len(got) != 1 || got[0] != "platform-engineering" {
		t.Fatalf("GroupIDs: %v", got)
	}
}

func TestSkillFromAPI_MapsVersion(t *testing.T) {
	s := skillFromAPI(api.SkillAtLatestVersion{
		Identifier: "demo-skill",
		Title:      "Demo",
		Version:    "2.0.0",
		Location:   "global",
	}, nil)
	if s.Version != "2.0.0" {
		t.Fatalf("got %+v", s)
	}
}

func TestCatalogFromAPI_MultiGroupSkillMergesAllGroups(t *testing.T) {
	catalog := CatalogFromAPI(&api.GroupedSkillsResponse{
		Groups: []api.SkillGroupAtLatestVersion{
			{
				Identifier: "group-a",
				Skills: []api.SkillAtLatestVersion{
					{Identifier: "shared-skill", Title: "Shared"},
				},
			},
			{
				Identifier: "group-b",
				Skills: []api.SkillAtLatestVersion{
					{Identifier: "shared-skill", Title: "Shared"},
				},
			},
		},
	})
	if len(catalog.Skills) != 1 {
		t.Fatalf("expected one skill, got %d", len(catalog.Skills))
	}
	got := catalog.Skills[0].GroupIDs
	if len(got) != 2 || got[0] != "group-a" || got[1] != "group-b" {
		t.Fatalf("GroupIDs: %v", got)
	}
}

func TestCatalogFromAPI_UngroupedSeparateFromGroups(t *testing.T) {
	resp := &api.GroupedSkillsResponse{
		Groups: []api.SkillGroupAtLatestVersion{
			{
				Identifier: "g1",
				Skills: []api.SkillAtLatestVersion{
					{Identifier: "grouped-skill"},
				},
			},
		},
		UngroupedSkills: []api.SkillAtLatestVersion{
			{Identifier: "standalone"},
		},
	}
	catalog := CatalogFromAPI(resp)
	ungrouped := UngroupedSkills(catalog)
	if len(ungrouped) != 1 || ungrouped[0].Identifier != "standalone" {
		t.Fatalf("ungrouped: %+v", ungrouped)
	}
}
