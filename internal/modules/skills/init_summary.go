package skills

import (
	"sort"

	"github.com/port-labs/port-cli/internal/api"
)

// InitSkillSummary is one published skill row for the interactive init catalog overview.
type InitSkillSummary struct {
	Identifier string
	Title      string
	Version    string
}

// GroupSkillCount is one skill group with its member skills for init overview.
type GroupSkillCount struct {
	Identifier string
	Title      string
	SkillCount int
	Skills     []InitSkillSummary
}

// InitCatalogStats summarizes the published skills catalog for interactive init.
type InitCatalogStats struct {
	GroupCount     int
	Groups         []GroupSkillCount
	Ungrouped      []InitSkillSummary
	UngroupedCount int
}

// InitCatalogStatsFrom builds group and ungrouped summaries from a metadata-only catalog
// (GET /v1/skills?exclude=files) and the group list from GET /v1/skills/groups.
func InitCatalogStatsFrom(catalogGroups []api.SkillGroupAtLatestVersion, catalog *FetchedSkills) InitCatalogStats {
	byID := skillSummariesByID(catalog)

	counts := make(map[string]int, len(catalog.Groups))
	titles := make(map[string]string, len(catalog.Groups))
	skillIDsByGroup := make(map[string][]string, len(catalog.Groups))
	for _, g := range catalog.Groups {
		counts[g.Identifier] = len(g.SkillIDs)
		if g.Title != "" {
			titles[g.Identifier] = g.Title
		}
		skillIDsByGroup[g.Identifier] = append([]string(nil), g.SkillIDs...)
	}

	seen := make(map[string]bool, len(catalogGroups))
	groups := make([]GroupSkillCount, 0, len(catalogGroups)+len(catalog.Groups))
	for _, g := range catalogGroups {
		seen[g.Identifier] = true
		title := g.Title
		if title == "" {
			title = titles[g.Identifier]
		}
		groups = append(groups, GroupSkillCount{
			Identifier: g.Identifier,
			Title:      title,
			SkillCount: counts[g.Identifier],
			Skills:     summariesForIDs(skillIDsByGroup[g.Identifier], byID),
		})
	}
	for _, g := range catalog.Groups {
		if seen[g.Identifier] {
			continue
		}
		groups = append(groups, GroupSkillCount{
			Identifier: g.Identifier,
			Title:      g.Title,
			SkillCount: len(g.SkillIDs),
			Skills:     summariesForIDs(g.SkillIDs, byID),
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Identifier < groups[j].Identifier
	})

	ungrouped := summariesFromSkills(UngroupedSkills(catalog))

	return InitCatalogStats{
		GroupCount:     len(groups),
		Groups:         groups,
		Ungrouped:      ungrouped,
		UngroupedCount: len(ungrouped),
	}
}

func skillSummariesByID(catalog *FetchedSkills) map[string]InitSkillSummary {
	out := make(map[string]InitSkillSummary)
	if catalog == nil {
		return out
	}
	for _, s := range catalog.Skills {
		out[s.Identifier] = initSkillSummaryFromSkill(s)
	}
	return out
}

func initSkillSummaryFromSkill(s Skill) InitSkillSummary {
	return InitSkillSummary{
		Identifier: s.Identifier,
		Title:      s.Title,
		Version:    s.Version,
	}
}

func summariesForIDs(ids []string, byID map[string]InitSkillSummary) []InitSkillSummary {
	out := make([]InitSkillSummary, 0, len(ids))
	for _, id := range ids {
		if s, ok := byID[id]; ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Identifier < out[j].Identifier
	})
	return out
}

func summariesFromSkills(skills []Skill) []InitSkillSummary {
	out := make([]InitSkillSummary, 0, len(skills))
	for _, s := range skills {
		out = append(out, initSkillSummaryFromSkill(s))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Identifier < out[j].Identifier
	})
	return out
}
