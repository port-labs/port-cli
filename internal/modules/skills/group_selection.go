package skills

import (
	"sort"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
)

// GroupSelectionFromCatalog computes include/exclude deltas vs team-owned groups.
func GroupSelectionFromCatalog(groups []api.SkillGroupAtLatestVersion, selected []string) (include, exclude []string) {
	teamBaseline := make(map[string]bool)
	for _, g := range groups {
		if g.MatchesUserTeams {
			teamBaseline[g.Identifier] = true
		}
	}

	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		selectedSet[id] = true
	}

	for id := range selectedSet {
		if !teamBaseline[id] {
			include = append(include, id)
		}
	}
	for id := range teamBaseline {
		if !selectedSet[id] {
			exclude = append(exclude, id)
		}
	}
	return include, exclude
}

// PreselectedGroupIDs returns group identifiers that match the user's teams.
func PreselectedGroupIDs(groups []api.SkillGroupAtLatestVersion) []string {
	var out []string
	for _, g := range groups {
		if g.MatchesUserTeams {
			out = append(out, g.Identifier)
		}
	}
	sort.Strings(out)
	return out
}

// GroupSyncIntent describes team ownership, saved include/exclude deltas, and the initial checkbox state.
type GroupSyncIntent struct {
	TeamOwned     bool
	SavedInclude  bool
	SavedExclude  bool
	InitiallySync bool
}

// InitialSelectedGroupIDs returns multiselect defaults from saved config when present, else team-owned groups.
func InitialSelectedGroupIDs(groups []api.SkillGroupAtLatestVersion, cfg *config.SkillsConfig) []string {
	if cfg == nil || !hasSavedGroupSelection(cfg) {
		return PreselectedGroupIDs(groups)
	}
	if cfg.UsesTeamGroupDefaults() {
		return selectedIDsForTeamConfig(groups, cfg.IncludeGroups, cfg.ExcludeGroups)
	}
	if cfg.SelectAllGroups {
		ids := make([]string, 0, len(groups))
		for _, g := range groups {
			ids = append(ids, g.Identifier)
		}
		sort.Strings(ids)
		return ids
	}
	if len(cfg.SelectedGroups) > 0 {
		out := append([]string(nil), cfg.SelectedGroups...)
		sort.Strings(out)
		return out
	}
	return PreselectedGroupIDs(groups)
}

func hasSavedGroupSelection(cfg *config.SkillsConfig) bool {
	return cfg.HasSelection() &&
		(cfg.UsesTeamGroupDefaults() || cfg.SelectAllGroups || len(cfg.SelectedGroups) > 0)
}

func selectedIDsForTeamConfig(groups []api.SkillGroupAtLatestVersion, include, exclude []string) []string {
	excludeSet := toSet(exclude)
	includeSet := toSet(include)
	var out []string
	for _, g := range groups {
		id := g.Identifier
		if excludeSet[id] {
			continue
		}
		if includeSet[id] || g.MatchesUserTeams {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// GroupSyncIntents maps each catalog group to display/sync metadata for interactive init.
func GroupSyncIntents(groups []api.SkillGroupAtLatestVersion, cfg *config.SkillsConfig, initialSelected []string) map[string]GroupSyncIntent {
	selectedSet := toSet(initialSelected)
	includeSet := toSet(nil)
	excludeSet := toSet(nil)
	if cfg != nil && cfg.UsesTeamGroupDefaults() {
		includeSet = toSet(cfg.IncludeGroups)
		excludeSet = toSet(cfg.ExcludeGroups)
	}
	out := make(map[string]GroupSyncIntent, len(groups))
	for _, g := range groups {
		id := g.Identifier
		out[id] = GroupSyncIntent{
			TeamOwned:     g.MatchesUserTeams,
			SavedInclude:  includeSet[id],
			SavedExclude:  excludeSet[id],
			InitiallySync: selectedSet[id],
		}
	}
	return out
}

// InitialUngroupedSelection returns saved ungrouped sync defaults for interactive prompts.
func InitialUngroupedSelection(cfg *config.SkillsConfig) (selectAll bool, skillIDs []string) {
	if cfg == nil {
		return false, nil
	}
	if cfg.SelectAllUngrouped {
		return true, nil
	}
	if len(cfg.SelectedSkills) > 0 {
		return false, append([]string(nil), cfg.SelectedSkills...)
	}
	return false, nil
}
