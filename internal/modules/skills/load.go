// Package skills syncs Port catalog skills to local AI tool directories.
// Catalog reads and writes go through internal/api skills routes.
package skills

import (
	"context"
	"fmt"

	"github.com/port-labs/port-cli/internal/api"
)

// CatalogFromAPI maps the grouped skills API response to FetchedSkills.
func CatalogFromAPI(resp *api.GroupedSkillsResponse) *FetchedSkills {
	if resp == nil {
		return &FetchedSkills{}
	}
	result := &FetchedSkills{}
	byID := make(map[string]int)
	for _, g := range resp.Groups {
		group := SkillGroup{
			Identifier:       g.Identifier,
			Title:            g.Title,
			MatchesUserTeams: g.MatchesUserTeams,
			SkillIDs:         make([]string, 0, len(g.Skills)),
		}
		for _, s := range g.Skills {
			group.SkillIDs = append(group.SkillIDs, s.Identifier)
			if idx, ok := byID[s.Identifier]; ok {
				result.Skills[idx].GroupIDs = mergeSkillGroupIDs(result.Skills[idx].GroupIDs, g.Identifier, s.GroupIdentifiers)
				continue
			}
			sk := skillFromAPI(s, []string{g.Identifier})
			byID[s.Identifier] = len(result.Skills)
			result.Skills = append(result.Skills, sk)
		}
		result.Groups = append(result.Groups, group)
	}
	for _, s := range resp.UngroupedSkills {
		if idx, ok := byID[s.Identifier]; ok {
			result.Skills[idx].GroupIDs = mergeSkillGroupIDs(result.Skills[idx].GroupIDs, "", s.GroupIdentifiers)
			continue
		}
		sk := skillFromAPI(s, nil)
		byID[s.Identifier] = len(result.Skills)
		result.Skills = append(result.Skills, sk)
	}
	return result
}

func mergeSkillGroupIDs(existing []string, groupID string, fromSkill []string) []string {
	return coalesceGroupIDs(append(existing, groupID), fromSkill)
}

func coalesceGroupIDs(fromGroup, fromSkill []string) []string {
	seen := make(map[string]bool, len(fromGroup)+len(fromSkill))
	out := make([]string, 0, len(fromGroup)+len(fromSkill))
	for _, id := range fromGroup {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, id := range fromSkill {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func skillFromAPI(s api.SkillAtLatestVersion, groupIDs []string) Skill {
	groupIDs = coalesceGroupIDs(groupIDs, s.GroupIdentifiers)
	files := make([]SkillFile, 0, len(s.Files))
	for _, f := range s.Files {
		path, _ := f.Properties["path"].(string)
		content, _ := f.Properties["content"].(string)
		if path == "" {
			continue
		}
		files = append(files, SkillFile{Path: path, Content: content})
	}
	return Skill{
		Identifier:     s.Identifier,
		AgentSkillName: s.AgentSkillName,
		Title:          s.Title,
		Description:    s.Description,
		Version:        s.Version,
		GroupIDs:       append([]string(nil), groupIDs...),
		Location:       parseSkillLocation(s.Location),
		Files:          files,
	}
}

// FetchSkillsQuery optional filters for loading the sync catalog.
type FetchSkillsQuery struct {
	SkillIdentifiers   []string
	IncludeGroups      []string
	ExcludeGroups      []string
	TeamsDefault       *bool
	Exclude            []string
	ExcludeFiles       bool
	IncludeUngrouped   bool
	IncludeUnpublished bool
}

// ExcludeSkillFiles is the exclude query value for omitting file content.
const ExcludeSkillFiles = "files"

// FetchSkillsFromAPI loads the skill catalog.
func FetchSkillsFromAPI(ctx context.Context, client *api.Client, query FetchSkillsQuery) (*FetchedSkills, error) {
	if client == nil {
		return nil, fmt.Errorf("API client is not configured")
	}
	skillQuery := api.GetSkillsQuery{
		SkillIdentifiers:   query.SkillIdentifiers,
		IncludeGroups:      query.IncludeGroups,
		ExcludeGroups:      query.ExcludeGroups,
		TeamsDefault:       query.TeamsDefault,
		Exclude:            append([]string(nil), query.Exclude...),
		IncludeUngrouped:   query.IncludeUngrouped,
		IncludeUnpublished: query.IncludeUnpublished,
	}
	if query.ExcludeFiles {
		skillQuery.Exclude = append(skillQuery.Exclude, ExcludeSkillFiles)
	}
	resp, err := client.GetSkillsGrouped(ctx, skillQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch skills: %w", err)
	}
	return CatalogFromAPI(resp), nil
}

// UngroupedSkills returns skills that are not members of any group in the catalog.
func UngroupedSkills(fetched *FetchedSkills) []Skill {
	if fetched == nil {
		return nil
	}
	inGroup := groupedSkillIDSet(fetched.Groups)
	var out []Skill
	seen := make(map[string]bool)
	for _, s := range fetched.Skills {
		if inGroup[s.Identifier] || seen[s.Identifier] {
			continue
		}
		seen[s.Identifier] = true
		out = append(out, s)
	}
	return out
}

func groupedSkillIDSet(groups []SkillGroup) map[string]bool {
	set := make(map[string]bool)
	for _, g := range groups {
		for _, id := range g.SkillIDs {
			set[id] = true
		}
	}
	return set
}
