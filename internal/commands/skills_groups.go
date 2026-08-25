package commands

import "github.com/spf13/cobra"

const (
	skillsGroupSetup     = "setup"
	skillsGroupSelection = "selection"
	skillsGroupRemote    = "remote"
	skillsGroupLocal     = "local"
)

const skillsCommandOverview = `Manage Port AI skills: sync catalog skills from Port to local AI tool directories
(Cursor, Claude Code, Codex, etc.) and publish custom skills back to Port.

What changes where:
  • ~/.port/config.yaml — which skill groups/skills and AI tools you sync (selection commands)
  • Port (remote) — skill entities and versions (upload, publish, unpublish)
  • Local disk — files under <tool>/skills/ (sync, add, remove, clear)

Quick start:
  port skills init                    Pick tools, choose skills, sync to disk
  port skills init --install-hooks    Also auto-run sync when an AI session starts

See command groups below. Use 'port skills <command> --help' for flags and examples.`

func configureSkillsCommandGroups(skillsCmd *cobra.Command) {
	skillsCmd.Long = skillsCommandOverview
	skillsCmd.AddGroup(
		&cobra.Group{
			ID:    skillsGroupSetup,
			Title: "Setup:",
		},
		&cobra.Group{
			ID:    skillsGroupSelection,
			Title: "Selection & sync (updates ~/.port/config.yaml):",
		},
		&cobra.Group{
			ID:    skillsGroupRemote,
			Title: "Port catalog (in your organization):",
		},
		&cobra.Group{
			ID:    skillsGroupLocal,
			Title: "Local files (on disk only):",
		},
	)
}

func withSkillsGroup(cmd *cobra.Command, groupID string) *cobra.Command {
	cmd.GroupID = groupID
	return cmd
}
