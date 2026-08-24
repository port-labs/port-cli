package commands

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/port-labs/port-cli/internal/styles"
	"github.com/spf13/cobra"
)

// RegisterCache registers the cache command group.
func RegisterCache(rootCmd *cobra.Command) {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage locally cached Port data",
		Long:  `Manage data that Port CLI caches and installs locally on your machine.`,
	}

	cacheCmd.AddCommand(registerCacheClear())

	rootCmd.AddCommand(cacheCmd)
}

func registerCacheClear() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Remove Port CLI local hooks, skill files, and skills config",
		Long: `Remove everything that Port CLI has installed or cached locally:

  • Port hook entries from hooks.json / settings.json (other hooks are preserved)
  • Locally synced Port-managed skills
  • The skills section from ~/.port/config.yaml

This is a full cleanup — use 'port skills clear' if you only want to delete
the skill files while keeping hooks and configuration intact.

Use --force to skip the confirmation prompt.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := GetGlobalFlags(cmd.Context())
			mod, _, err := newSkillsModule(flags)
			if err != nil {
				return err
			}

			if !force {
				ok, err := confirmPrompt(
					"Remove everything Port CLI installed locally?",
					"This will remove all Port hooks, skill files, and skills config.\nOther hooks in your AI tool configs will be left untouched.",
				)
				if err != nil {
					return err
				}
				if !ok {
					lipgloss.Printf("%s Cancelled — nothing was removed.\n", styles.ExclamationMark)
					return nil
				}
			}

			result, err := mod.Remove()
			if err != nil {
				return fmt.Errorf("failed to clear Port cache: %w", err)
			}

			for _, t := range result.HooksResult.RemovedFrom {
				lipgloss.Printf("%s Removed Port hook from %s\n", styles.CheckMark, styles.Bold.Render(t))
			}
			for _, t := range result.HooksResult.Skipped {
				lipgloss.Printf("%s Skipped %s (no hook file found)\n", styles.QuestionMark, t)
			}
			for _, t := range result.SkillsResult.DeletedTargets {
				lipgloss.Printf("%s Deleted Port-managed skills from %s\n", styles.CheckMark, styles.Bold.Render(t))
			}
			lipgloss.Printf("%s Skills config cleared.\n", styles.CheckMark)
			lipgloss.Printf("\n%s Port CLI cache fully cleared.\n", styles.CheckMark)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip the confirmation prompt")
	return cmd
}
