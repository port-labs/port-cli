package commands

import (
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/skills"
	"github.com/port-labs/port-cli/internal/styles"
	"github.com/spf13/cobra"
)

// RegisterSkills registers the skills command group.
func RegisterSkills(rootCmd *cobra.Command) {
	var skillsOrg string

	skillsCmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage Port AI skills: sync, selection, and publishing",
	}
	configureSkillsCommandGroups(skillsCmd)
	skillsCmd.PersistentFlags().StringVar(&skillsOrg, "org", "", "Organization name (uses default from config if not specified)")

	skillsCmd.AddCommand(
		withSkillsGroup(registerSkillsInit(), skillsGroupSetup),
		withSkillsGroup(registerSkillsStatus(), skillsGroupSetup),
		withSkillsGroup(registerSkillsSelect(), skillsGroupSelection),
		withSkillsGroup(registerSkillsAdd(), skillsGroupSelection),
		withSkillsGroup(registerSkillsRemove(), skillsGroupSelection),
		withSkillsGroup(registerSkillsSync(), skillsGroupSelection),
		withSkillsGroup(registerSkillsList(), skillsGroupRemote),
		withSkillsGroup(registerSkillsSearch(), skillsGroupRemote),
		withSkillsGroup(registerSkillsUpload(), skillsGroupRemote),
		withSkillsGroup(registerSkillsPublish(), skillsGroupRemote),
		withSkillsGroup(registerSkillsUnpublish(), skillsGroupRemote),
		withSkillsGroup(registerSkillsClear(), skillsGroupLocal),
	)

	rootCmd.AddCommand(skillsCmd)
}

func registerSkillsInit() *cobra.Command {
	var (
		tools              []string
		installHooks       bool
		groups             []string
		skillsIDs          []string
		selectAllGroups    bool
		selectAllUngrouped bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "First-time setup: pick AI tools and choose skills",
		Long: `First-time setup for Port skills on this machine.

Choose which AI tools receive synced skills and save that configuration.

Supported tools include Agents (cross-platform), Cursor, Claude Code, Gemini CLI,
OpenAI Codex, Windsurf, and GitHub Copilot. Skills go under each tool's skills/
tree (and ~/.agents / <project>/.agents for Agents per agentskills.io).

By default init does not modify hooks.json or settings.json. Pass --install-hooks
to merge a session-start hook that runs 'port skills sync --quiet --org <org>' into
each selected tool (global home dirs for most tools; GitHub Copilot is repo-scoped
under <repo>/.github — run init from the repository root). The org is the one
resolved for this init (--org flag or default_org), so later default_org changes
do not redirect hook syncs.

Skills are placed based on each skill's Port 'location' property ("global" → tool
directories, "project" → tool directory inside each registered project directory).

Scripts and CI: pass explicit flags (--tool, --group, --skill, …) or -y/--yes to
select every option (all tools, groups, and ungrouped skills) without prompts.
Run 'port skills sync' afterwards to write skills to disk.

Examples:
  port skills init
  port skills init -y
  port skills init --tool Cursor --select-all-groups --select-all-ungrouped
  port skills init --tool "Agents (cross-platform)" --tool Cursor --tool "Claude Code"
  port skills init --tool Cursor --tool "Gemini CLI" --tool Windsurf --install-hooks`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)
			configManager := config.NewConfigManager(flags.ConfigFile)
			org := skillsOrgName(cmd)

			explicitTools := cmd.Flags().Changed("tool")
			explicitSelection := skillsSelectionNonInteractive(cmd, selectAllGroups, selectAllUngrouped)
			acceptAll := skillsAcceptAll(cmd)
			usePrompts := skillsUseInteractivePrompts(cmd) && !explicitTools && !explicitSelection

			var targets []skills.HookTarget
			switch {
			case usePrompts:
				var err error
				targets, err = promptTargetSelection(configManager)
				if err != nil {
					return err
				}
			case explicitTools:
				resolved, err := resolveTargetsByName(tools)
				if err != nil {
					return err
				}
				targets = resolved
			case acceptAll:
				targets = skills.DefaultHookTargets()
			default:
				return fmt.Errorf("non-interactive init requires %s", skillsNonInteractiveHint)
			}

			mod, configManager, err := newSkillsModuleWithFlags(ctx, flags, org)
			if err != nil {
				return err
			}

			if installHooks {
				initResult, err := mod.Init(ctx, skills.InitOptions{Targets: targets})
				if err != nil {
					return fmt.Errorf("failed to install hooks: %w", err)
				}
				for _, t := range initResult.InstalledTargets {
					lipgloss.Printf("%s Hook installed in %s\n", styles.CheckMark, styles.Bold.Render(t))
				}
			} else if err := mod.RegisterTargets(ctx, targets); err != nil {
				return fmt.Errorf("failed to save tool targets: %w", err)
			}

			var rawFetched *skills.FetchedSkills
			var loadOpts skills.LoadSkillsOptions
			switch {
			case usePrompts:
				loadOpts, rawFetched, err = buildLoadSkillsOpts(ctx, mod, configManager, true)
				if err != nil {
					return err
				}
			case explicitSelection:
				loadOpts, rawFetched, err = buildNonInteractiveSelectLoadOpts(ctx, mod, configManager, groups, skillsIDs, selectAllGroups, selectAllUngrouped)
				if err != nil {
					return err
				}
			case acceptAll:
				loadOpts, rawFetched, err = buildLoadSkillsOptsAllSelected(ctx, mod)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("non-interactive init requires selection flags or -y")
			}
			loadOpts.Fetched = rawFetched

			if err := mod.ConfigureSelection(loadOpts); err != nil {
				return err
			}
			lipgloss.Printf("%s Skills configuration initialized. Run %s to sync skills.\n", styles.CheckMark, styles.Bold.Render("port skills sync"))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&tools, "tool", nil, "AI tool name to configure (repeatable, e.g. \"Cursor\")")
	cmd.Flags().BoolVar(&installHooks, "install-hooks", false, "Write session-start hooks (hooks.json / settings.json) for selected tools")
	cmd.Flags().StringArrayVar(&groups, "group", nil, "Skill group identifier to sync (repeatable)")
	cmd.Flags().StringArrayVar(&skillsIDs, "skill", nil, "Skill identifier to sync (repeatable)")
	cmd.Flags().BoolVar(&selectAllGroups, "select-all-groups", false, "Sync all skill groups")
	cmd.Flags().BoolVar(&selectAllUngrouped, "select-all-ungrouped", false, "Sync all ungrouped skills")
	return cmd
}

func registerSkillsSelect() *cobra.Command {
	var (
		groups             []string
		skillsIDs          []string
		selectAllGroups    bool
		selectAllUngrouped bool
	)

	cmd := &cobra.Command{
		Use:   "select",
		Short: "Replace your full skill/group selection and re-sync",
		Long: `Replace the entire skill and group selection saved in ~/.port/config.yaml.

Re-runs the selection flow from 'port skills init' without reinstalling hooks.
Clears previously synced skills from disk, then syncs the new selection.

Use 'port skills add' or 'port skills remove' for incremental changes instead.

Interactive: run in a terminal to pick groups and ungrouped skills.

Non-interactive: pass --group, --skill, --select-all-groups, and/or
--select-all-ungrouped (same flags as 'port skills init').`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)

			mod, configManager, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}

			skillsCfg, err := configManager.LoadSkillsConfig()
			if err != nil || len(skillsCfg.Targets) == 0 {
				return fmt.Errorf("no skills configuration found — run 'port skills init' first")
			}

			nonInteractive := skillsSelectionNonInteractive(cmd, selectAllGroups, selectAllUngrouped)
			if !nonInteractive {
				if err := RequireInteractive(); err != nil {
					return err
				}
			}

			var rawFetched *skills.FetchedSkills
			loadOpts := skills.LoadSkillsOptions{}
			if nonInteractive {
				loadOpts, rawFetched, err = buildNonInteractiveSelectLoadOpts(ctx, mod, configManager, groups, skillsIDs, selectAllGroups, selectAllUngrouped)
				if err != nil {
					return err
				}
			}
			loadOpts.Fetched = rawFetched

			return runSkillsSelect(cmd, mod, configManager, !nonInteractive, loadOpts)
		},
	}

	cmd.Flags().StringArrayVar(&groups, "group", nil, "Skill group identifier to sync (repeatable)")
	cmd.Flags().StringArrayVar(&skillsIDs, "skill", nil, "Ungrouped skill identifier to sync (repeatable)")
	cmd.Flags().BoolVar(&selectAllGroups, "select-all-groups", false, "Sync all skill groups")
	cmd.Flags().BoolVar(&selectAllUngrouped, "select-all-ungrouped", false, "Sync all ungrouped skills")
	return cmd
}

func registerSkillsAdd() *cobra.Command {
	var (
		groups    []string
		skillsIDs []string
		tools     []string
	)

	cmd := &cobra.Command{
		Use:   "add [skill-identifier...]",
		Short: "Add groups, skills, or AI tools to your saved selection",
		Long: `Incrementally extend what you sync — without redoing 'port skills select'.

Adds skill groups, individual skills, or AI tool hook targets to ~/.port/config.yaml.
Does not remove anything already selected. After saving, runs the same sync as
'port skills sync' so new items appear under skills/ on disk.

Interactive mode lists only groups, skills, and tools not already configured.
Scripts and CI: pass --group, --skill, --tool, positional skill IDs, or -y/--yes
to select every available option without prompts.

Examples:
  port skills add --group security
  port skills add --skill integrations-overview
  port skills add integrations-overview
  port skills add -y
  port skills add --skill my-skill --tool Cursor
  port skills add --group operations --group security --tool "Claude Code"`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)

			mod, configManager, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}

			skillsCfg, err := configManager.LoadSkillsConfig()
			if err != nil {
				skillsCfg = &config.SkillsConfig{}
			}

			addOpts := skills.AddSkillsOptions{
				Groups: groups,
				Skills: append(append([]string(nil), skillsIDs...), args...),
			}

			explicit := skillsIncrementalExplicit(cmd, args)
			acceptAll := skillsAcceptAll(cmd)
			usePrompts := skillsUseInteractivePrompts(cmd) && !explicit

			switch {
			case usePrompts:
				fetched, err := mod.FetchSkillsMetadata(ctx)
				if err != nil {
					return fmt.Errorf("failed to fetch skills from Port: %w", err)
				}

				availableGroups := skills.AvailableGroupsToAdd(skillsCfg, fetched)
				if len(availableGroups) > 0 {
					selected, err := promptAddGroupSelection(availableGroups)
					if err != nil {
						return err
					}
					addOpts.Groups = append(addOpts.Groups, selected...)
				}

				availableSkills := skills.AvailableSkillsToAdd(skillsCfg, fetched)
				if len(availableSkills) > 0 {
					selected, err := promptAddSkillSelection(availableSkills)
					if err != nil {
						return err
					}
					addOpts.Skills = append(addOpts.Skills, selected...)
				}

				unconfigured, err := unconfiguredHookTargets(configManager)
				if err != nil {
					return err
				}
				if len(unconfigured) > 0 {
					configuredTools, err := configuredHookTargetNames(configManager)
					if err != nil {
						return err
					}
					targets, err := promptAddTargetSelection(unconfigured, configuredTools)
					if err != nil {
						return err
					}
					addOpts.Targets = targets
				}

				if len(addOpts.Groups) == 0 && len(addOpts.Skills) == 0 && len(addOpts.Targets) == 0 {
					lipgloss.Printf("%s Nothing new to add — your current selection already includes all optional skills and configured tools.\n", styles.QuestionMark)
					return nil
				}
			case explicit:
				if len(tools) > 0 {
					resolved, err := resolveTargetsByName(tools)
					if err != nil {
						return err
					}
					addOpts.Targets = resolved
				}
			case acceptAll:
				fetched, err := mod.FetchSkillsMetadata(ctx)
				if err != nil {
					return fmt.Errorf("failed to fetch skills from Port: %w", err)
				}
				if err := populateAddAllAvailable(&addOpts, skillsCfg, fetched, configManager); err != nil {
					return err
				}
				if len(addOpts.Groups) == 0 && len(addOpts.Skills) == 0 && len(addOpts.Targets) == 0 {
					lipgloss.Printf("%s Nothing new to add — your current selection already includes all optional skills and configured tools.\n", styles.QuestionMark)
					return nil
				}
			default:
				return fmt.Errorf("non-interactive add requires %s", skillsNonInteractiveHint)
			}

			if !skillsCfg.HasSelection() && len(addOpts.Targets) == 0 &&
				len(addOpts.Groups) == 0 && len(addOpts.Skills) == 0 {
				return fmt.Errorf("no skill selection configured — run 'port skills init' first")
			}
			if !usePrompts && explicit && len(addOpts.Groups) == 0 && len(addOpts.Skills) == 0 && len(addOpts.Targets) == 0 {
				return fmt.Errorf("specify at least one of --group, --skill, or --tool")
			}

			result, err := mod.AddSkills(ctx, addOpts)
			if err != nil {
				return err
			}

			for _, t := range result.NewTargets {
				lipgloss.Printf("%s Hook installed for %s\n", styles.CheckMark, styles.Bold.Render(t))
			}
			for _, g := range result.Merge.AddedGroups {
				lipgloss.Printf("%s Added group %s\n", styles.CheckMark, styles.Bold.Render(g))
			}
			for _, s := range result.Merge.AddedSkills {
				lipgloss.Printf("%s Added skill %s\n", styles.CheckMark, styles.Bold.Render(s))
			}
			for _, g := range result.Merge.SkippedGroups {
				lipgloss.Printf("%s Group %s already in your selection\n", styles.QuestionMark, g)
			}
			for _, s := range result.Merge.SkippedSkills {
				lipgloss.Printf("%s Skill %s already in your selection\n", styles.QuestionMark, s)
			}

			if result.Sync != nil {
				printLoadResult(result.Sync)
			} else if !result.Merge.HasChanges() && len(result.NewTargets) == 0 {
				lipgloss.Printf("%s No changes were made.\n", styles.QuestionMark)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&groups, "group", nil, "Skill group identifier to add (repeatable)")
	cmd.Flags().StringArrayVar(&skillsIDs, "skill", nil, "Ungrouped or individual skill identifier to add (repeatable)")
	cmd.Flags().StringArrayVar(&tools, "tool", nil, "AI tool name to install hooks for (repeatable, e.g. \"Cursor\")")
	return cmd
}

func registerSkillsRemove() *cobra.Command {
	var (
		groups    []string
		skillsIDs []string
		tools     []string
	)

	cmd := &cobra.Command{
		Use:   "remove [skill-identifier...]",
		Short: "Remove groups, skills, or AI tools from your saved selection",
		Long: `Incrementally shrink what you sync — without redoing 'port skills select'.

Removes skill groups, individual skills, or AI tool targets from ~/.port/config.yaml.
Removed tools also have hooks uninstalled and their Port-managed skills deleted.
Remaining selection is re-synced so pruned skills disappear from disk.

If you previously chose "all groups" or "all ungrouped skills", removing one item
materializes the selection into an explicit list. New skills added in Port will
not auto-sync until you 'port skills add' them again.

Scripts and CI: pass --group, --skill, --tool, positional skill IDs, or -y/--yes
to select every removable option without prompts (skips confirmation).

Examples:
  port skills remove --group legacy
  port skills remove --skill integrations-overview
  port skills remove integrations-overview
  port skills remove -y
  port skills remove --tool Windsurf
  port skills remove --group operations --skill old-skill`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)

			mod, configManager, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}

			skillsCfg, err := configManager.LoadSkillsConfig()
			if err != nil || (!skillsCfg.HasSelection() && len(skillsCfg.Targets) == 0) {
				return fmt.Errorf("no skills configuration found — run 'port skills init' first")
			}

			removeOpts := skills.RemoveSkillsOptions{
				Groups: groups,
				Skills: append(append([]string(nil), skillsIDs...), args...),
			}

			explicit := skillsIncrementalExplicit(cmd, args)
			acceptAll := skillsAcceptAll(cmd)
			usePrompts := skillsUseInteractivePrompts(cmd) && !explicit

			switch {
			case usePrompts:
				fetched, err := mod.FetchSkillsMetadata(ctx)
				if err != nil {
					return fmt.Errorf("failed to fetch skills from Port: %w", err)
				}

				removableGroups := skills.RemovableGroups(skillsCfg, fetched)
				if len(removableGroups) > 0 {
					selected, err := promptRemoveGroupSelection(removableGroups)
					if err != nil {
						return err
					}
					removeOpts.Groups = append(removeOpts.Groups, selected...)
				}

				removableSkills := skills.RemovableSkills(skillsCfg, fetched)
				if len(removableSkills) > 0 {
					selected, err := promptRemoveSkillSelection(removableSkills)
					if err != nil {
						return err
					}
					removeOpts.Skills = append(removeOpts.Skills, selected...)
				}

				configuredTargets, err := configuredHookTargets(configManager)
				if err != nil {
					return err
				}
				if len(configuredTargets) > 0 {
					selected, err := promptRemoveTargetSelection(configuredTargets)
					if err != nil {
						return err
					}
					removeOpts.Targets = selected
				}

				if len(removeOpts.Groups) == 0 && len(removeOpts.Skills) == 0 && len(removeOpts.Targets) == 0 {
					lipgloss.Printf("%s Nothing selected — no changes made.\n", styles.QuestionMark)
					return nil
				}

				ok, err := confirmPrompt(
					"Apply these removals?",
					"Hooks for selected tools will be uninstalled and their synced skills deleted. Removed groups/skills will be pruned from local AI tool directories.",
				)
				if err != nil {
					return err
				}
				if !ok {
					lipgloss.Printf("%s Cancelled — no changes made.\n", styles.ExclamationMark)
					return nil
				}
			case explicit:
				if len(tools) > 0 {
					resolved, err := resolveTargetsByName(tools)
					if err != nil {
						return err
					}
					removeOpts.Targets = resolved
				}
				if len(removeOpts.Groups) == 0 && len(removeOpts.Skills) == 0 && len(removeOpts.Targets) == 0 {
					return fmt.Errorf("specify at least one of --group, --skill, or --tool")
				}
			case acceptAll:
				fetched, err := mod.FetchSkillsMetadata(ctx)
				if err != nil {
					return fmt.Errorf("failed to fetch skills from Port: %w", err)
				}
				if err := populateRemoveAll(&removeOpts, skillsCfg, fetched, configManager); err != nil {
					return err
				}
				if len(removeOpts.Groups) == 0 && len(removeOpts.Skills) == 0 && len(removeOpts.Targets) == 0 {
					lipgloss.Printf("%s Nothing selected — no changes made.\n", styles.QuestionMark)
					return nil
				}
			default:
				return fmt.Errorf("non-interactive remove requires %s", skillsNonInteractiveHint)
			}

			result, err := mod.RemoveSkills(ctx, removeOpts)
			if err != nil {
				return err
			}

			if result.Remove.Materialized {
				lipgloss.Printf(
					"%s Selection switched from \"all\" to specific items. Future groups or skills added in Port will not sync until you run 'port skills add'.\n",
					styles.ExclamationMark,
				)
			}
			for _, t := range result.RemovedTargets {
				lipgloss.Printf("%s Hook removed from %s\n", styles.CheckMark, styles.Bold.Render(t))
			}
			for _, g := range result.Remove.RemovedGroups {
				lipgloss.Printf("%s Removed group %s\n", styles.CheckMark, styles.Bold.Render(g))
			}
			for _, s := range result.Remove.RemovedSkills {
				lipgloss.Printf("%s Removed skill %s\n", styles.CheckMark, styles.Bold.Render(s))
			}
			for _, g := range result.Remove.SkippedGroups {
				lipgloss.Printf("%s Skipped group %s (not in selection)\n", styles.QuestionMark, g)
			}
			for _, s := range result.Remove.SkippedSkills {
				lipgloss.Printf("%s Skipped skill %s (not in selection)\n", styles.QuestionMark, s)
			}

			if result.Sync != nil {
				printLoadResult(result.Sync)
			} else if !result.Remove.HasChanges() && len(result.RemovedTargets) == 0 {
				lipgloss.Printf("%s No changes were made.\n", styles.QuestionMark)
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&groups, "group", nil, "Skill group identifier to remove (repeatable)")
	cmd.Flags().StringArrayVar(&skillsIDs, "skill", nil, "Skill identifier to remove (repeatable)")
	cmd.Flags().StringArrayVar(&tools, "tool", nil, "AI tool name to remove hooks for (repeatable, e.g. \"Cursor\")")
	return cmd
}

func registerSkillsSync() *cobra.Command {
	var (
		includeGroups         []string
		excludeGroups         []string
		excludeLegacySkills   bool
		includeInternalSkills bool
		tools                 []string
		installHooks          bool
		groups                []string
		skillsIDs             []string
		selectAllGroups       bool
		selectAllUngrouped    bool
		noGitignore           bool
		failOnSkillError      bool
	)

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Download skills from Port to local AI tool directories",
		Long: `Refresh local skill files from Port and write them under each tool's skills/ tree.

After 'port skills init', sync uses the targets and selection saved in
~/.port/config.yaml. Without init, pass --tool to choose where files go for this run.

Runtime flags (--tool, --group, --skill, select-all, include-group, exclude-group)
apply to this sync only and are not written to config.yaml. Use 'port skills init',
'select', 'add', or 'remove' to persist tools and selection.

Skills with location="global" go to tool home directories; location="project" go
under each registered project directory (or the current directory when using --tool).

By default your organization's skills are synced; Port built-in registry skills are
excluded unless you pass --include-internal. Use --exclude-legacy to omit older
catalog skills that use the previous data model.

By default, skills with invalid or incomplete files are skipped with a warning.
Pass --fail-on-skill-error to fail the entire sync on the first invalid skill.

Examples:
  # Re-sync using saved config from 'port skills init'
  port skills sync

  # One tool (repeat --tool for each; names must match init prompts exactly)
  port skills sync --tool "Agents (cross-platform)"   # ~/.agents/skills/
  port skills sync --tool Cursor                     # ~/.cursor/skills/
  port skills sync --tool "Claude Code"              # ~/.claude/skills/
  port skills sync --tool "Gemini CLI"               # ~/.gemini/skills/
  port skills sync --tool "OpenAI Codex"             # ~/.codex/skills/
  port skills sync --tool Windsurf                   # ~/.codeium/windsurf/skills/
  port skills sync --tool "GitHub Copilot"           # <repo>/.github/skills/ (run from repo root)

  # Multiple tools in one sync
  port skills sync --tool Cursor --tool "Claude Code"
  port skills sync --tool Cursor --tool "Gemini CLI" --tool "OpenAI Codex"
  port skills sync --tool "Agents (cross-platform)" --tool Cursor --tool Windsurf
  port skills sync --tool Cursor --tool "Claude Code" --tool "GitHub Copilot"

  # One-off selection (not saved to config)
  port skills sync --tool Cursor --group operations --group security
  port skills sync --tool Cursor --skill integrations-overview
  port skills sync --tool Cursor --select-all-groups --select-all-ungrouped

  # Catalog filters for this run
  port skills sync --include-internal
  port skills sync --exclude-legacy
  port skills sync --tool Cursor --include-internal --exclude-legacy

  # Adjust team group defaults for this run only
  port skills sync --include-group operations --exclude-group legacy

  # Install hooks and sync in one step (writes hooks; sync args stay ephemeral)
  port skills sync --tool Cursor --install-hooks
  port skills sync --tool Cursor --tool "Claude Code" --install-hooks

  # Silent sync (used by session-start hooks)
  port skills sync --quiet`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)

			mod, configManager, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}

			skillsCfg, err := configManager.LoadSkillsConfig()
			if err != nil {
				skillsCfg = &config.SkillsConfig{}
			}

			loadOpts := skills.LoadSkillsOptions{
				ExcludeLegacySkills:   excludeLegacySkills,
				IncludeInternalSkills: includeInternalSkills,
				NoGitignore:           noGitignore,
				FailOnSkillError:      failOnSkillError,
			}
			if len(includeGroups) > 0 || len(excludeGroups) > 0 {
				loadOpts.IncludeGroups = mergeStringLists(skillsCfg.IncludeGroups, includeGroups)
				loadOpts.ExcludeGroups = mergeStringLists(skillsCfg.ExcludeGroups, excludeGroups)
				loadOpts.TeamGroupDefaults = skillsCfg.TeamGroupDefaults || skillsCfg.UsesTeamGroupDefaults() || len(includeGroups) > 0 || len(excludeGroups) > 0
			}
			selectionFlagsChanged := skillsSelectionNonInteractive(cmd, selectAllGroups, selectAllUngrouped)
			if selectionFlagsChanged {
				selectionOpts, err := loadSkillsOptsFromSelectionFlags(groups, skillsIDs, selectAllGroups, selectAllUngrouped, false)
				if err != nil {
					return err
				}
				loadOpts.SelectAll = selectionOpts.SelectAll
				loadOpts.SelectAllGroups = selectionOpts.SelectAllGroups
				loadOpts.SelectAllUngrouped = selectionOpts.SelectAllUngrouped
				loadOpts.SelectedGroups = selectionOpts.SelectedGroups
				loadOpts.SelectedSkills = selectionOpts.SelectedSkills
			}
			if len(tools) > 0 {
				resolved, err := resolveTargetsByName(tools)
				if err != nil {
					return err
				}
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("failed to get home directory: %w", err)
				}
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("failed to get working directory: %w", err)
				}
				if installHooks {
					if err := skills.InstallHooks(resolved, home, cwd, mod.OrgName()); err != nil {
						return fmt.Errorf("failed to install hooks: %w", err)
					}
				}
				loadOpts.TargetOverrides = skills.TargetPaths(resolved, home, cwd)
				loadOpts.ProjectDirOverrides = []string{cwd}
			} else if installHooks {
				return fmt.Errorf("--install-hooks requires at least one --tool")
			}
			if len(tools) > 0 || selectionFlagsChanged || len(includeGroups) > 0 || len(excludeGroups) > 0 {
				loadOpts.NoSave = true
			}

			result, err := mod.LoadSkills(ctx, loadOpts)
			if err != nil {
				return fmt.Errorf("failed to sync skills: %w", err)
			}

			quiet, _ := cmd.Flags().GetBool("quiet")
			if !quiet {
				printLoadResult(result)
			}
			return nil
		},
	}
	cmd.Flags().BoolP("quiet", "q", false, "Suppress output (used automatically by AI tool hooks)")
	cmd.Flags().StringArrayVar(&includeGroups, "include-group", nil, "Additional skill group(s) to sync (repeatable)")
	cmd.Flags().StringArrayVar(&excludeGroups, "exclude-group", nil, "Skill group(s) to exclude from sync (repeatable)")
	cmd.Flags().BoolVar(&excludeLegacySkills, "exclude-legacy", false, "Omit skills that use the previous Port catalog data model")
	cmd.Flags().BoolVar(&includeInternalSkills, "include-internal", false, "Include Port built-in registry skills (excluded by default)")
	cmd.Flags().StringArrayVar(&tools, "tool", nil, "AI tool name to sync to for this run (repeatable, e.g. \"Cursor\")")
	cmd.Flags().BoolVar(&installHooks, "install-hooks", false, "Write session-start hooks for --tool targets before syncing")
	cmd.Flags().StringArrayVar(&groups, "group", nil, "Skill group identifier to sync for this run (repeatable)")
	cmd.Flags().StringArrayVar(&skillsIDs, "skill", nil, "Skill identifier to sync for this run (repeatable)")
	cmd.Flags().BoolVar(&selectAllGroups, "select-all-groups", false, "Sync all skill groups for this run")
	cmd.Flags().BoolVar(&selectAllUngrouped, "select-all-ungrouped", false, "Sync all ungrouped skills for this run")
	cmd.Flags().BoolVar(&noGitignore, "no-gitignore", false, "Skip adding project-scoped skills paths to repository .gitignore files")
	cmd.Flags().BoolVar(&failOnSkillError, "fail-on-skill-error", false, "Fail sync instead of warning when one skill cannot be written")
	return cmd
}

func mergeStringLists(base, extra []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range append(append([]string(nil), base...), extra...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func registerSkillsClear() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Delete all local Port-managed skill files (keeps config and hooks)",
		Long: `Delete every Port skill file synced to local AI tool directories.

Removes Port-managed skills from each configured target (e.g. ~/.cursor/skills/)
and registered project directories. Does not change ~/.port/config.yaml selection
and does not remove session hooks.

Skills will be re-downloaded on the next 'port skills sync' or session-start hook.
Use 'port cache clear' to also remove hooks and wipe skills config.

Use --force to skip the confirmation prompt.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			mod, _, err := newSkillsModule(flags)
			if err != nil {
				return err
			}

			if !ShouldSkipConfirm(cmd, force) {
				ok, err := confirmPrompt(
					"Delete all locally synced Port skills?",
					"This will remove Port-managed skills from all configured AI tool directories.\nHooks will remain in place — skills will be re-synced on the next session start.",
				)
				if err != nil {
					return err
				}
				if !ok {
					lipgloss.Printf("%s Cancelled — no skills were deleted.\n", styles.ExclamationMark)
					return nil
				}
			}

			result, err := mod.ClearSkills()
			if err != nil {
				return fmt.Errorf("failed to clear skills: %w", err)
			}

			for _, t := range result.DeletedTargets {
				lipgloss.Printf("%s Deleted Port-managed skills from %s\n", styles.CheckMark, styles.Bold.Render(t))
			}
			for _, t := range result.SkippedTargets {
				lipgloss.Printf("%s Skipped %s (no skills directory found)\n", styles.QuestionMark, t)
			}
			if len(result.DeletedTargets) == 0 {
				lipgloss.Printf("%s No Port skills found locally — nothing to delete.\n", styles.QuestionMark)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip the confirmation prompt")
	return cmd
}

func registerSkillsStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show saved selection, hook targets, and last sync time",
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			mod, _, err := newSkillsModule(flags)
			if err != nil {
				return err
			}

			status, err := mod.Status()
			if err != nil {
				return fmt.Errorf("failed to get skills status: %w", err)
			}

			printSkillsStatus(status)
			return nil
		},
	}
}

// --- shared helpers ---
