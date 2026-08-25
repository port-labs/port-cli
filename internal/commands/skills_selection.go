package commands

import (
	"context"
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/skills"
	"github.com/port-labs/port-cli/internal/styles"
	"github.com/spf13/cobra"
)

func skillsSelectionNonInteractive(cmd *cobra.Command, selectAllGroups, selectAllUngrouped bool) bool {
	return cmd.Flags().Changed("group") || cmd.Flags().Changed("skill") ||
		cmd.Flags().Changed("select-all-groups") || cmd.Flags().Changed("select-all-ungrouped") ||
		selectAllGroups || selectAllUngrouped
}

func skillsIncrementalExplicit(cmd *cobra.Command, args []string) bool {
	return cmd.Flags().Changed("group") || cmd.Flags().Changed("skill") ||
		cmd.Flags().Changed("tool") || len(args) > 0
}

func skillsSkipConfirm(cmd *cobra.Command) bool {
	return ShouldSkipConfirm(cmd, false)
}

// skillsAcceptAll is true when -y/--yes should select every option (all tools,
// groups, skills, etc.) instead of showing interactive prompts.
func skillsAcceptAll(cmd *cobra.Command) bool {
	return skillsSkipConfirm(cmd)
}

// skillsUseInteractivePrompts is true only in a TTY when not using -y and no
// explicit flags force a non-interactive path.
func skillsUseInteractivePrompts(cmd *cobra.Command) bool {
	if skillsAcceptAll(cmd) {
		return false
	}
	return IsInteractive()
}

const skillsNonInteractiveHint = "provide flags (--tool, --group, --skill, …) or -y to accept all options"

func loadSkillsOptsFromSelectionFlags(
	groups, skillsIDs []string,
	selectAllGroups, selectAllUngrouped bool,
	replaceSelection bool,
) (skills.LoadSkillsOptions, error) {
	opts := skills.LoadSkillsOptions{
		SelectAllGroups:    selectAllGroups,
		SelectAllUngrouped: selectAllUngrouped,
		SelectedGroups:     groups,
		SelectedSkills:     skillsIDs,
		ReplaceSelection:   replaceSelection,
	}
	if selectAllGroups && selectAllUngrouped {
		opts.SelectAll = true
	}
	if len(opts.SelectedGroups) == 0 && len(opts.SelectedSkills) == 0 &&
		!opts.SelectAllGroups && !opts.SelectAllUngrouped && !opts.SelectAll {
		return opts, fmt.Errorf("non-interactive selection requires --group, --skill, --select-all-groups, and/or --select-all-ungrouped")
	}
	return opts, nil
}

// buildNonInteractiveSelectLoadOpts builds team-aware or classic selection for
// non-interactive port skills init and port skills select.
func buildNonInteractiveSelectLoadOpts(
	ctx context.Context,
	mod *skills.Module,
	configManager *config.ConfigManager,
	groups, skillsIDs []string,
	selectAllGroups, selectAllUngrouped bool,
) (skills.LoadSkillsOptions, *skills.FetchedSkills, error) {
	cfg, err := configManager.LoadSkillsConfig()
	if err != nil {
		cfg = &config.SkillsConfig{}
	}

	useTeamMode := cfg.UsesTeamGroupDefaults() || len(groups) > 0 || selectAllGroups
	if !useTeamMode {
		opts, err := loadSkillsOptsFromSelectionFlags(groups, skillsIDs, selectAllGroups, selectAllUngrouped, true)
		return opts, nil, err
	}

	catalogGroups, err := mod.FetchGroupsForInit(ctx)
	if err != nil {
		return skills.LoadSkillsOptions{}, nil, fmt.Errorf("failed to fetch skill groups from Port: %w", err)
	}

	var selectedGroups []string
	if selectAllGroups {
		selectedGroups = make([]string, 0, len(catalogGroups))
		for _, g := range catalogGroups {
			selectedGroups = append(selectedGroups, g.Identifier)
		}
	} else {
		selectedGroups = append([]string(nil), groups...)
	}

	if len(selectedGroups) == 0 && len(skillsIDs) == 0 && !selectAllUngrouped {
		return skills.LoadSkillsOptions{}, nil, fmt.Errorf("non-interactive selection requires --group, --skill, --select-all-groups, and/or --select-all-ungrouped")
	}

	includeGroups, excludeGroups := skills.GroupSelectionFromCatalog(catalogGroups, selectedGroups)
	fetched, err := mod.FetchSkillsWithQuery(ctx, skills.FetchSkillsQuery{
		SkillIdentifiers: skillsIDs,
		IncludeGroups:    includeGroups,
		ExcludeGroups:    excludeGroups,
		TeamsDefault:     skills.BoolPtr(true),
		Exclude:          []string{"internal"},
		ExcludeFiles:     true,
		IncludeUngrouped: selectAllUngrouped,
	})
	if err != nil {
		return skills.LoadSkillsOptions{}, nil, fmt.Errorf("failed to fetch skills from Port: %w", err)
	}

	return skills.LoadSkillsOptions{
		TeamGroupDefaults:  true,
		IncludeGroups:      includeGroups,
		ExcludeGroups:      excludeGroups,
		SelectAllUngrouped: selectAllUngrouped,
		SelectedSkills:     skillsIDs,
		ReplaceSelection:   true,
	}, fetched, nil
}

func runSkillsSelect(cmd *cobra.Command, mod *skills.Module, configManager *config.ConfigManager, interactive bool, loadOpts skills.LoadSkillsOptions) error {
	ctx := cmd.Context()

	if interactive {
		var err error
		loadOpts, _, err = buildLoadSkillsOpts(ctx, mod, configManager, true)
		if err != nil {
			return err
		}
	} else if loadOpts.Fetched == nil && len(loadOpts.IncludeGroups) == 0 && len(loadOpts.SelectedGroups) == 0 &&
		!loadOpts.SelectAll && !loadOpts.SelectAllGroups && !loadOpts.TeamGroupDefaults {
		return fmt.Errorf("non-interactive selection requires --group, --skill, --select-all-groups, and/or --select-all-ungrouped")
	}

	loadOpts.ReplaceSelection = true

	if clearResult, err := mod.ClearSkills(); err != nil {
		return fmt.Errorf("failed to clear existing skills: %w", err)
	} else {
		for _, t := range clearResult.DeletedTargets {
			lipgloss.Printf("%s Cleared existing skills from %s\n", styles.CheckMark, styles.Bold.Render(t))
		}
	}

	result, err := mod.LoadSkills(ctx, loadOpts)
	if err != nil {
		return fmt.Errorf("failed to sync skills: %w", err)
	}
	printLoadResult(result)
	return nil
}
