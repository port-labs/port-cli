package commands

import (
	"context"
	"fmt"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/skills"
	"github.com/port-labs/port-cli/internal/styles"
)

func configuredHookTargetNames(configManager *config.ConfigManager) ([]string, error) {
	if configManager == nil {
		return nil, nil
	}
	skillsCfg, err := configManager.LoadSkillsConfig()
	if err != nil {
		return nil, err
	}
	return skills.ResolveTargetNames(skillsCfg.Targets, skills.DefaultHookTargets()), nil
}

func unconfiguredHookTargets(configManager *config.ConfigManager) ([]skills.HookTarget, error) {
	configuredNames, err := configuredHookTargetNames(configManager)
	if err != nil {
		return nil, err
	}
	configured := toStringSet(configuredNames)
	allTargets := skills.DefaultHookTargets()
	var out []skills.HookTarget
	for _, t := range allTargets {
		if !configured[t.Name] {
			out = append(out, t)
		}
	}
	return out, nil
}

func resolveTargetsByName(names []string) ([]skills.HookTarget, error) {
	allTargets := skills.DefaultHookTargets()
	byName := make(map[string]skills.HookTarget, len(allTargets))
	for _, t := range allTargets {
		byName[t.Name] = t
	}
	var resolved []skills.HookTarget
	var unknown []string
	for _, name := range names {
		t, ok := byName[name]
		if !ok {
			unknown = append(unknown, name)
			continue
		}
		resolved = append(resolved, t)
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown AI tool(s): %s", strings.Join(unknown, ", "))
	}
	return resolved, nil
}

func promptAddTargetSelection(available []skills.HookTarget, configuredToolNames []string) ([]skills.HookTarget, error) {
	if len(available) == 0 {
		return nil, nil
	}
	targetOptions := make([]huh.Option[string], 0, len(available))
	for _, t := range available {
		label := t.Name
		if t.Note != "" {
			label = fmt.Sprintf("%s (%s)", t.Name, t.Note)
		}
		targetOptions = append(targetOptions, huh.NewOption(label, t.Name))
	}
	description := "Only tools not yet configured are listed. Use space to select, enter to confirm."
	if len(configuredToolNames) > 0 {
		description = fmt.Sprintf(
			"%s\n\nIf you don't select any tools here, added skills will sync to your existing tools: %s.",
			description,
			strings.Join(configuredToolNames, ", "),
		)
	}
	var selectedNames []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Add hooks for which AI tools?").
				Description(description).
				Options(targetOptions...).
				Height(len(targetOptions) + 4).
				Value(&selectedNames),
		),
	).WithHeight(0).WithTheme(&styles.FormTheme{})
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}
	if len(selectedNames) == 0 && len(configuredToolNames) > 0 {
		lipgloss.Printf(
			"\n%s No new tools selected — skills will sync to: %s\n",
			styles.QuestionMark,
			styles.Bold.Render(strings.Join(configuredToolNames, ", ")),
		)
	}
	return resolveTargetsByName(selectedNames)
}

// runMultiSelectPrompt renders a single-group multiselect form and returns the chosen values.
// Shared by the add/remove group and skill prompts so they stay visually and behaviourally consistent.
func runMultiSelectPrompt(title, description string, options []huh.Option[string]) ([]string, error) {
	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(title).
				Description(description).
				Options(options...).
				Height(len(options) + 4).
				Value(&selected),
		),
	).WithHeight(0).WithTheme(&styles.FormTheme{})
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}
	return selected, nil
}

func groupSelectOptions(groups []skills.SkillGroup) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(groups))
	for _, g := range groups {
		options = append(options, huh.NewOption(groupLabel(g), g.Identifier))
	}
	return options
}

func skillSelectOptions(available []skills.Skill) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(available))
	for _, s := range available {
		label := skillLabel(s)
		if len(s.GroupIDs) > 0 {
			label = fmt.Sprintf("%s (%s)", label, strings.Join(s.GroupIDs, ", "))
		}
		options = append(options, huh.NewOption(label, s.Identifier))
	}
	return options
}

func promptAddGroupSelection(groups []skills.SkillGroup) ([]string, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	return runMultiSelectPrompt(
		"Which skill groups would you like to add?",
		"Groups already in your selection are not shown. Use space to select, enter to confirm.",
		groupSelectOptions(groups),
	)
}

func promptAddSkillSelection(available []skills.Skill) ([]string, error) {
	if len(available) == 0 {
		return nil, nil
	}
	return runMultiSelectPrompt(
		"Which skills would you like to add?",
		"Skills already in your selection are not shown. Use space to select, enter to confirm.",
		skillSelectOptions(available),
	)
}

func promptTargetSelection(configManager *config.ConfigManager) ([]skills.HookTarget, error) {
	allTargets := skills.DefaultHookTargets()

	var preSelected []string
	if configManager != nil {
		if skillsCfg, err := configManager.LoadSkillsConfig(); err == nil {
			preSelected = skills.ResolveTargetNames(skillsCfg.Targets, allTargets)
		}
	}

	targetOptions := make([]huh.Option[string], 0, len(allTargets))
	for _, t := range allTargets {
		label := t.Name
		if t.Note != "" {
			label = fmt.Sprintf("%s (%s)", t.Name, t.Note)
		}
		opt := huh.NewOption(label, t.Name)
		for _, ps := range preSelected {
			if ps == t.Name {
				opt = opt.Selected(true)
				break
			}
		}
		targetOptions = append(targetOptions, opt)
	}

	selectedNames := make([]string, len(preSelected))
	copy(selectedNames, preSelected)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which AI tools should receive synced skills?").
				Description("Use space to select/deselect, enter to confirm. Run init with --install-hooks to add session-start hooks.").
				Options(targetOptions...).
				Height(len(targetOptions) + 4).
				Value(&selectedNames),
		),
	).WithHeight(0).WithTheme(&styles.FormTheme{})
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}

	if len(selectedNames) == 0 {
		return nil, fmt.Errorf("no AI tools selected — nothing to install")
	}

	nameSet := make(map[string]bool, len(selectedNames))
	for _, n := range selectedNames {
		nameSet[n] = true
	}
	var targets []skills.HookTarget
	for _, t := range allTargets {
		if nameSet[t.Name] {
			targets = append(targets, t)
		}
	}
	return targets, nil
}

func configuredHookTargets(configManager *config.ConfigManager) ([]skills.HookTarget, error) {
	names, err := configuredHookTargetNames(configManager)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	return resolveTargetsByName(names)
}

func promptRemoveGroupSelection(groups []skills.SkillGroup) ([]string, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	return runMultiSelectPrompt(
		"Which skill groups would you like to remove?",
		"Only groups currently in your selection are listed. Use space to select, enter to confirm.",
		groupSelectOptions(groups),
	)
}

func promptRemoveSkillSelection(available []skills.Skill) ([]string, error) {
	if len(available) == 0 {
		return nil, nil
	}
	return runMultiSelectPrompt(
		"Which skills would you like to remove?",
		"Only skills currently in your selection are listed. Use space to select, enter to confirm.",
		skillSelectOptions(available),
	)
}

func promptRemoveTargetSelection(configured []skills.HookTarget) ([]skills.HookTarget, error) {
	if len(configured) == 0 {
		return nil, nil
	}
	targetOptions := make([]huh.Option[string], 0, len(configured))
	for _, t := range configured {
		label := t.Name
		if t.Note != "" {
			label = fmt.Sprintf("%s (%s)", t.Name, t.Note)
		}
		targetOptions = append(targetOptions, huh.NewOption(label, t.Name))
	}
	var selectedNames []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Remove hooks for which AI tools?").
				Description("Only tools currently configured are listed. Use space to select, enter to confirm.").
				Options(targetOptions...).
				Height(len(targetOptions) + 4).
				Value(&selectedNames),
		),
	).WithHeight(0).WithTheme(&styles.FormTheme{})
	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}
	return resolveTargetsByName(selectedNames)
}

// buildLoadSkillsOpts fetches skill groups from the Port API for interactive selection,
// then loads the sync catalog with team defaults plus include/exclude adjustments.
func buildLoadSkillsOpts(ctx context.Context, mod *skills.Module, configManager *config.ConfigManager, promptSelection bool) (skills.LoadSkillsOptions, *skills.FetchedSkills, error) {
	if !promptSelection {
		return skills.LoadSkillsOptions{}, nil, nil
	}

	catalogGroups, err := mod.FetchGroupsForInit(ctx)
	if err != nil {
		return skills.LoadSkillsOptions{}, nil, fmt.Errorf("failed to fetch skill groups from Port: %w", err)
	}

	metadataCatalog, err := mod.FetchSkillsWithQuery(ctx, initMetadataCatalogQuery())
	if err != nil {
		return skills.LoadSkillsOptions{}, nil, fmt.Errorf("failed to fetch skills from Port: %w", err)
	}
	printInitCatalogSummary(catalogGroups, metadataCatalog)

	skillsCfg, err := configManager.LoadSkillsConfig()
	if err != nil {
		skillsCfg = &config.SkillsConfig{}
	}
	initialSelected := skills.InitialSelectedGroupIDs(catalogGroups, skillsCfg)
	intents := skills.GroupSyncIntents(catalogGroups, skillsCfg, initialSelected)

	selectedGroups, err := promptGroupSelectionCatalog(catalogGroups, skillsCfg, initialSelected, intents)
	if err != nil {
		return skills.LoadSkillsOptions{}, nil, err
	}

	includeGroups, excludeGroups := skills.GroupSelectionFromCatalog(catalogGroups, selectedGroups)

	ungroupedSkills := skills.UngroupedSkills(metadataCatalog)

	selectAllUngrouped, selectedSkills, err := promptUngroupedSelection(ungroupedSkills, skillsCfg)
	if err != nil {
		return skills.LoadSkillsOptions{}, nil, err
	}

	fetched, err := mod.FetchSkillsWithQuery(ctx, skills.FetchSkillsQuery{
		SkillIdentifiers: selectedSkills,
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
		SelectedSkills:     selectedSkills,
		ReplaceSelection:   true,
	}, fetched, nil
}

func initMetadataCatalogQuery() skills.FetchSkillsQuery {
	return skills.MetadataCatalogQuery()
}

// buildLoadSkillsOptsAllSelected applies the same catalog logic as interactive init
// but selects every group and all ungrouped skills (-y / CI "check all").
func buildLoadSkillsOptsAllSelected(ctx context.Context, mod *skills.Module) (skills.LoadSkillsOptions, *skills.FetchedSkills, error) {
	catalogGroups, err := mod.FetchGroupsForInit(ctx)
	if err != nil {
		return skills.LoadSkillsOptions{}, nil, fmt.Errorf("failed to fetch skill groups from Port: %w", err)
	}

	selectedGroups := make([]string, 0, len(catalogGroups))
	for _, g := range catalogGroups {
		selectedGroups = append(selectedGroups, g.Identifier)
	}
	includeGroups, excludeGroups := skills.GroupSelectionFromCatalog(catalogGroups, selectedGroups)

	fetched, err := mod.FetchSkillsWithQuery(ctx, skills.FetchSkillsQuery{
		IncludeGroups:    includeGroups,
		ExcludeGroups:    excludeGroups,
		TeamsDefault:     skills.BoolPtr(true),
		Exclude:          []string{"internal"},
		ExcludeFiles:     true,
		IncludeUngrouped: true,
	})
	if err != nil {
		return skills.LoadSkillsOptions{}, nil, fmt.Errorf("failed to fetch skills from Port: %w", err)
	}

	return skills.LoadSkillsOptions{
		TeamGroupDefaults:  true,
		IncludeGroups:      includeGroups,
		ExcludeGroups:      excludeGroups,
		SelectAllUngrouped: true,
		ReplaceSelection:   true,
	}, fetched, nil
}

func populateAddAllAvailable(
	addOpts *skills.AddSkillsOptions,
	skillsCfg *config.SkillsConfig,
	fetched *skills.FetchedSkills,
	configManager *config.ConfigManager,
) error {
	for _, g := range skills.AvailableGroupsToAdd(skillsCfg, fetched) {
		addOpts.Groups = append(addOpts.Groups, g.Identifier)
	}
	for _, s := range skills.AvailableSkillsToAdd(skillsCfg, fetched) {
		addOpts.Skills = append(addOpts.Skills, s.Identifier)
	}
	unconfigured, err := unconfiguredHookTargets(configManager)
	if err != nil {
		return err
	}
	addOpts.Targets = unconfigured
	return nil
}

func populateRemoveAll(
	removeOpts *skills.RemoveSkillsOptions,
	skillsCfg *config.SkillsConfig,
	fetched *skills.FetchedSkills,
	configManager *config.ConfigManager,
) error {
	for _, g := range skills.RemovableGroups(skillsCfg, fetched) {
		removeOpts.Groups = append(removeOpts.Groups, g.Identifier)
	}
	for _, s := range skills.RemovableSkills(skillsCfg, fetched) {
		removeOpts.Skills = append(removeOpts.Skills, s.Identifier)
	}
	configuredTargets, err := configuredHookTargets(configManager)
	if err != nil {
		return err
	}
	removeOpts.Targets = configuredTargets
	return nil
}

func promptGroupSelectionCatalog(
	groups []api.SkillGroupAtLatestVersion,
	skillsCfg *config.SkillsConfig,
	initialSelected []string,
	intents map[string]skills.GroupSyncIntent,
) ([]string, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	selected := append([]string(nil), initialSelected...)
	printGroupSelectionIntro(groups, skillsCfg, initialSelected)

	buildGroupOptions := func() []huh.Option[string] {
		opts := make([]huh.Option[string], 0, len(groups))
		for _, g := range groups {
			intent := intents[g.Identifier]
			opts = append(opts, huh.NewOption(
				groupCatalogIntentLabel(g, intent),
				g.Identifier,
			))
		}
		return opts
	}

	pickForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which skill groups would you like to sync?").
				Description("Use space to toggle. Each line shows team ownership and saved include/exclude. Checked items sync to disk.").
				DescriptionFunc(func() string {
					return groupSelectionDescription(groups, intents, selected)
				}, &selected).
				Options(buildGroupOptions()...).
				Height(len(groups) + 4).
				Value(&selected),
		),
	).WithHeight(0).WithTheme(&styles.FormTheme{})
	if err := pickForm.Run(); err != nil {
		return nil, fmt.Errorf("prompt error: %w", err)
	}

	selectedSet := toStringSet(selected)
	include, exclude := skills.GroupSelectionFromCatalog(groups, selected)
	lipgloss.Printf("\n%s Groups (checked = will sync):\n", styles.CheckMark)
	for _, g := range groups {
		intent := intents[g.Identifier]
		marker := styles.Circle
		if selectedSet[g.Identifier] {
			marker = styles.CheckMark
		}
		lipgloss.Printf("  %s %s\n", marker, groupCatalogIntentLabel(g, intent))
	}
	if skillsCfg != nil && skillsCfg.UsesTeamGroupDefaults() && (len(include) > 0 || len(exclude) > 0) {
		lipgloss.Printf("\n%s Saved as team defaults", styles.Faint.Render("→"))
		if len(include) > 0 {
			lipgloss.Printf("  include_groups: %s\n", strings.Join(include, ", "))
		}
		if len(exclude) > 0 {
			lipgloss.Printf("  exclude_groups: %s\n", strings.Join(exclude, ", "))
		}
	}
	fmt.Println()

	return selected, nil
}

func printInitCatalogSummary(catalogGroups []api.SkillGroupAtLatestVersion, catalog *skills.FetchedSkills) {
	stats := skills.InitCatalogStatsFrom(catalogGroups, catalog)
	lipgloss.Printf("\n%s Skills in Port (published, metadata only)\n", styles.Bold.Render("Catalog"))
	lipgloss.Printf("  %d skill group(s)\n", stats.GroupCount)
	for _, g := range stats.Groups {
		label := g.Identifier
		if t := strings.TrimSpace(g.Title); t != "" && t != g.Identifier {
			label = fmt.Sprintf("%s (%s)", t, g.Identifier)
		}
		lipgloss.Printf("    • %s: %d skill(s)\n", label, g.SkillCount)
		for _, s := range g.Skills {
			lipgloss.Printf("        - %s\n", formatInitSkillLine(s))
		}
	}
	lipgloss.Printf("  %d ungrouped skill(s)\n", stats.UngroupedCount)
	for _, s := range stats.Ungrouped {
		lipgloss.Printf("    - %s\n", formatInitSkillLine(s))
	}
	fmt.Println()
}

func formatInitSkillLine(s skills.InitSkillSummary) string {
	name := strings.TrimSpace(s.Title)
	if name == "" {
		name = s.Identifier
	} else if name != s.Identifier {
		name = fmt.Sprintf("%s (%s)", name, s.Identifier)
	}
	version := strings.TrimSpace(s.Version)
	if version == "" {
		version = "—"
	}
	return fmt.Sprintf("%s — v%s", name, version)
}

func printGroupSelectionIntro(groups []api.SkillGroupAtLatestVersion, skillsCfg *config.SkillsConfig, initialSelected []string) {
	teamDefault := skills.PreselectedGroupIDs(groups)
	if skillsCfg != nil && skillsCfg.UsesTeamGroupDefaults() &&
		(len(skillsCfg.IncludeGroups) > 0 || len(skillsCfg.ExcludeGroups) > 0) {
		lipgloss.Printf(
			"\n%s Restored %d group(s) from your saved selection (team defaults + include/exclude). Adjust below.\n\n",
			styles.CheckMark,
			len(initialSelected),
		)
		return
	}
	if skillsCfg != nil && len(skillsCfg.SelectedGroups) > 0 {
		lipgloss.Printf(
			"\n%s Restored %d group(s) from selected_groups in config. Adjust below.\n\n",
			styles.CheckMark,
			len(initialSelected),
		)
		return
	}
	if len(initialSelected) > 0 && stringSetsEqual(initialSelected, teamDefault) {
		lipgloss.Printf(
			"\n%s Pre-selected %d group(s) owned by your team(s). Adjust below.\n\n",
			styles.CheckMark,
			len(initialSelected),
		)
		return
	}
	if len(initialSelected) > 0 {
		lipgloss.Printf(
			"\n%s Pre-selected %d group(s). Adjust below.\n\n",
			styles.CheckMark,
			len(initialSelected),
		)
	}
}

func stringSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := toStringSet(a)
	for _, id := range b {
		if !set[id] {
			return false
		}
	}
	return true
}

func groupSelectionDescription(
	groups []api.SkillGroupAtLatestVersion,
	intents map[string]skills.GroupSyncIntent,
	selected []string,
) string {
	selectedSet := toStringSet(selected)
	if len(selectedSet) == 0 {
		return "No groups checked — no grouped skills will sync."
	}
	var willSync, wontSync []string
	for _, g := range groups {
		short := groupCatalogLabel(g)
		if selectedSet[g.Identifier] {
			willSync = append(willSync, short)
		} else {
			wontSync = append(wontSync, short)
		}
	}
	out := fmt.Sprintf("Will sync (%d): %s", len(willSync), strings.Join(willSync, ", "))
	if len(wontSync) > 0 {
		out += fmt.Sprintf("\nWon't sync (%d): %s", len(wontSync), strings.Join(wontSync, ", "))
	}
	return out
}

func ungroupedSelectionDescription(ungroupedSkills []skills.Skill, selected []string) string {
	selectedSet := toStringSet(selected)
	if len(selectedSet) == 0 {
		return "No ungrouped skills checked — none will sync."
	}
	var names []string
	for _, s := range ungroupedSkills {
		if selectedSet[s.Identifier] {
			names = append(names, skillLabel(s))
		}
	}
	return fmt.Sprintf("Will sync (%d): %s", len(names), strings.Join(names, ", "))
}

func groupCatalogIntentLabel(g api.SkillGroupAtLatestVersion, intent skills.GroupSyncIntent) string {
	base := groupCatalogLabel(g)
	var parts []string
	if intent.TeamOwned {
		parts = append(parts, "your team")
	} else {
		parts = append(parts, "not your team")
	}
	switch {
	case intent.SavedInclude:
		parts = append(parts, "saved include")
	case intent.SavedExclude:
		parts = append(parts, "saved exclude")
	case intent.TeamOwned:
		parts = append(parts, "team default")
	}
	return base + " — " + strings.Join(parts, " · ")
}

func groupCatalogLabel(g api.SkillGroupAtLatestVersion) string {
	title := strings.TrimSpace(g.Title)
	if title != "" && title != g.Identifier {
		return fmt.Sprintf("%s (%s)", title, g.Identifier)
	}
	return g.Identifier
}

func promptUngroupedSelection(ungroupedSkills []skills.Skill, skillsCfg *config.SkillsConfig) (selectAll bool, selected []string, err error) {
	if len(ungroupedSkills) == 0 {
		return false, nil, nil
	}

	savedAll, savedIDs := skills.InitialUngroupedSelection(skillsCfg)
	savedIDs = filterIDsToUngrouped(savedIDs, ungroupedSkills)
	syncAll := savedAll
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Sync all skills without a group?").
				Description(fmt.Sprintf("%d skill(s) in Port are not assigned to any skill group. Yes = sync all, No = pick specific ones.", len(ungroupedSkills))).
				Value(&syncAll),
		),
	).WithTheme(&styles.FormTheme{})
	if err := form.Run(); err != nil {
		return false, nil, fmt.Errorf("prompt error: %w", err)
	}

	if syncAll {
		lipgloss.Printf("\n%s All ungrouped skills selected:\n", styles.CheckMark)
		for _, s := range ungroupedSkills {
			lipgloss.Printf("  %s %s\n", styles.CheckMark, skillLabel(s))
		}
		fmt.Println()
		return true, nil, nil
	}

	selected = append([]string(nil), savedIDs...)
	buildUngroupedOptions := func() []huh.Option[string] {
		opts := make([]huh.Option[string], 0, len(ungroupedSkills))
		for _, s := range ungroupedSkills {
			opts = append(opts, huh.NewOption(skillLabel(s), s.Identifier))
		}
		return opts
	}

	pickForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which ungrouped skills would you like to sync?").
				Description("These skills are not in any Port skill group (independent of your group selection above). Use space to toggle.").
				DescriptionFunc(func() string {
					return ungroupedSelectionDescription(ungroupedSkills, selected)
				}, &selected).
				Options(buildUngroupedOptions()...).
				Height(len(ungroupedSkills) + 4).
				Value(&selected),
		),
	).WithHeight(0).WithTheme(&styles.FormTheme{})
	if err := pickForm.Run(); err != nil {
		return false, nil, fmt.Errorf("prompt error: %w", err)
	}

	selectedSet := toStringSet(selected)
	lipgloss.Printf("\n%s Ungrouped skills (checked = will sync):\n", styles.CheckMark)
	for _, s := range ungroupedSkills {
		marker := styles.Circle
		if selectedSet[s.Identifier] {
			marker = styles.CheckMark
		}
		lipgloss.Printf("  %s %s\n", marker, skillLabel(s))
	}
	fmt.Println()

	return false, selected, nil
}

func groupLabel(g skills.SkillGroup) string {
	if g.Title != "" {
		return g.Title
	}
	return g.Identifier
}

func skillLabel(s skills.Skill) string {
	if s.Title != "" {
		return s.Title
	}
	return s.Identifier
}

func filterIDsToUngrouped(ids []string, ungrouped []skills.Skill) []string {
	valid := make(map[string]bool, len(ungrouped))
	for _, s := range ungrouped {
		valid[s.Identifier] = true
	}
	var out []string
	for _, id := range ids {
		if valid[id] {
			out = append(out, id)
		}
	}
	return out
}

func toStringSet(slice []string) map[string]bool {
	s := make(map[string]bool, len(slice))
	for _, v := range slice {
		s[v] = true
	}
	return s
}

func valueOrNone(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}
