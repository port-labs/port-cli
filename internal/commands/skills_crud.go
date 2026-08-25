package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/skills"
	"github.com/port-labs/port-cli/internal/styles"
	"github.com/spf13/cobra"
)

func registerSkillsUpload() *cobra.Command {
	var (
		identifier  string
		title       string
		description string
		location    string
		publish     bool
		published   bool
		versionBump string
		groups      []string
	)

	cmd := &cobra.Command{
		Use:   "upload <path-to-skill-folder-or-bundle>",
		Short: "Create or update skills in Port from local folders",
		Long: `Upload skill content to Port (create a skill or add a new version).

Accepts either a single skill directory (SKILL.md at the root) or a bundle directory
that contains one or more skill folders at any depth (e.g. ./claude/skills). Symlinks
to skill directories are supported. Search stops at each folder that contains SKILL.md.

The folder name must match the SKILL.md frontmatter name: field and follow the
Agent Skills name format (lowercase letters, numbers, and hyphens).
Re-uploading an existing skill creates a new patch version instead of failing.

Non-interactive example:
  port skills upload ./my-skill --identifier my-skill --publish --location project
  port skills upload ./claude/skills`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)
			mod, _, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}

			roots, err := skills.DiscoverSkillRoots(args[0])
			if err != nil {
				return err
			}

			packLocation, err := packSkillLocationFromFlag(cmd, location)
			if err != nil {
				return err
			}

			publishFlag := resolveSkillsPublishFlag(cmd, publish, published)
			bump, err := parseVersionBump(versionBump)
			if err != nil {
				return err
			}
			writeOpts := skills.UploadSkillWriteOptions{
				Publish:     publishFlag,
				VersionBump: bump,
				GroupIDs:    groups,
			}

			packOpts := skills.PackSkillFolderOptions{
				Title:       title,
				Description: description,
				Location:    packLocation,
			}

			if len(roots) == 1 {
				if identifier != "" {
					packOpts.Identifier = identifier
				}
				result, err := mod.UploadSkillFromFolder(ctx, roots[0], packOpts, writeOpts)
				if err != nil {
					return err
				}
				printSkillUploadSuccess(result, packLocation, publishFlag)
				return nil
			}

			if identifier != "" {
				return fmt.Errorf("--identifier cannot be used when uploading multiple skills from a bundle directory")
			}

			packs := make([]skills.SkillPackWithFolder, 0, len(roots))
			for _, root := range roots {
				pack, err := skills.PackSkillFolder(root, packOpts)
				if err != nil {
					return fmt.Errorf("%s: %w", root, err)
				}
				packs = append(packs, skills.SkillPackWithFolder{
					Pack:       pack,
					FolderBase: filepath.Base(root),
				})
			}

			batch, err := mod.UploadSkillsBatch(ctx, packs, writeOpts)
			if err != nil {
				return err
			}
			return printBatchUploadResults(batch)
		},
	}

	cmd.Flags().StringVar(&identifier, "identifier", "", "Skill identifier for single-skill upload (must be an Agent Skills name and match folder name)")
	cmd.Flags().StringVar(&title, "title", "", "Skill title (default: identifier or SKILL.md title)")
	cmd.Flags().StringVar(&description, "description", "", "Skill description (default: SKILL.md frontmatter)")
	cmd.Flags().StringVar(&location, "location", "global", "Skill location: global or project (default: global; SKILL.md frontmatter used when flag omitted)")
	cmd.Flags().BoolVar(&publish, "publish", false, "Set the new version as the skill active version")
	cmd.Flags().BoolVar(&published, "published", false, "Deprecated alias for --publish")
	cmd.Flags().StringVar(&versionBump, "version-bump", "patch", "Semver increment for the new version: patch, minor, or major")
	cmd.Flags().StringArrayVar(&groups, "group", nil, "Skill group identifier to link on upload (repeatable)")
	_ = cmd.Flags().MarkHidden("published")
	return cmd
}

func registerSkillsPublish() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <skill-identifier>",
		Short: "Make the latest existing version the active published version",
		Long: `Set the active published version to the highest semver version already in Port.

Does not upload files or create a new version. To upload content and publish in
one step, use 'port skills upload <dir> --publish' instead.

Example:
  port skills publish my-skill`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)
			mod, _, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}
			result, err := mod.PublishSkill(ctx, args[0])
			if err != nil {
				return err
			}
			lipgloss.Printf("%s Published skill %s (version %s)\n",
				styles.CheckMark,
				styles.Bold.Render(result.SkillIdentifier),
				result.Version,
			)
			return nil
		},
	}
	return cmd
}

func registerSkillsUnpublish() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unpublish <skill-identifier>",
		Short: "Unpublish a skill in Port (clear active version)",
		Long:  `Clear the active published version in Port. The skill and its versions remain but nothing is live until you run 'port skills publish' or upload with --publish.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)
			mod, _, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}
			if err := mod.UnpublishSkill(ctx, args[0]); err != nil {
				return err
			}
			lipgloss.Printf("%s Unpublished skill %s\n", styles.CheckMark, styles.Bold.Render(args[0]))
			return nil
		},
	}
	return cmd
}

// packSkillLocationFromFlag returns a location for PackSkillFolder when --location was set.
func packSkillLocationFromFlag(cmd *cobra.Command, location string) (string, error) {
	if cmd == nil || !cmd.Flags().Changed("location") {
		return "", nil
	}
	return skills.NormalizeSkillLocation(location)
}

func parseVersionBump(value string) (api.VersionBump, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "patch":
		return api.VersionBumpPatch, nil
	case "minor":
		return api.VersionBumpMinor, nil
	case "major":
		return api.VersionBumpMajor, nil
	default:
		return "", fmt.Errorf("version-bump must be patch, minor, or major")
	}
}

func resolveSkillsPublishFlag(cmd *cobra.Command, publish, published bool) bool {
	if cmd != nil && cmd.Flags().Changed("publish") {
		return publish
	}
	if cmd != nil && cmd.Flags().Changed("published") {
		return published
	}
	return publish || published
}

func skillActiveVersionStatus(active bool) string {
	if active {
		return "active version set"
	}
	return "active version unchanged"
}

func printSkillUploadSuccess(result *api.SkillVersionWriteResponse, location string, publish bool) {
	locLabel := ""
	if location != "" {
		locLabel = ", location " + styles.Faint.Render(location)
	}
	_ = publish
	lipgloss.Printf("%s Uploaded skill %s (version %s, %s%s)\n",
		styles.CheckMark,
		styles.Bold.Render(result.SkillIdentifier),
		result.Version,
		skillActiveVersionStatus(result.ActiveVersionSet),
		locLabel,
	)
}

func printBatchUploadResults(batch *api.BatchUploadSkillsResponse) error {
	if batch == nil {
		return fmt.Errorf("empty batch upload response")
	}

	var failed []string
	for _, item := range batch.Results {
		if item.OK {
			version := ""
			if item.Result != nil {
				version = item.Result.Version
			}
			lipgloss.Printf("%s Uploaded skill %s (version %s)\n",
				styles.CheckMark,
				styles.Bold.Render(item.Identifier),
				version,
			)
			continue
		}
		msg := item.Identifier
		if item.Error != nil {
			msg = fmt.Sprintf("%s: %s", item.Identifier, item.Error.Message)
		}
		failed = append(failed, msg)
		lipgloss.Fprintf(os.Stderr, "%s Failed %s\n", styles.ExclamationMark, msg)
	}

	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("failed to upload %d skill(s): %s", len(failed), strings.Join(failed, "; "))
}

func registerSkillsList() *cobra.Command {
	var (
		jsonOut            bool
		includeUnpublished bool
		all                bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Preview skills that would be synced (read-only)",
		Long: `Preview skills from your Port organization.

Shows the skills that match your saved init configuration — the same skills
that 'port skills sync' would download to disk.

Use --all to see every available skill regardless of your saved selection.
Use --include-unpublished to include skills without an active version.

This command never writes any local files.
Run 'port skills sync' to download skill files to your machine.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)

			mod, _, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}

			resp, err := mod.PreviewSkills(ctx, skills.PreviewSkillsOptions{
				All:                all,
				IncludeUnpublished: includeUnpublished,
			})
			if err != nil {
				return fmt.Errorf("failed to list skills: %w", err)
			}

			if len(resp.Groups) == 0 && len(resp.UngroupedSkills) == 0 {
				if !all {
					fmt.Println("No skills match your saved configuration. Run 'port skills init' to configure, assign skill groups to teams for default list/sync behavior, or use --all to see all available skills.")
				} else {
					fmt.Println("No skills found.")
				}
				return nil
			}

			if jsonOut {
				return printGroupedSkillsPreviewJSON(resp)
			}
			printGroupedSkillsPreview(resp)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print catalog as JSON")
	cmd.Flags().BoolVar(&includeUnpublished, "include-unpublished", false, "Include skills without an active published version")
	cmd.Flags().BoolVar(&all, "all", false, "Show all available skills, ignoring saved init configuration")
	return cmd
}

func registerSkillsSearch() *cobra.Command {
	var (
		jsonOut bool
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search skills by identifier or title substring",
		Long: `Search skills in your Port organization.

Matches the query as a case-insensitive substring against each skill's
identifier and title. Use quotes in the shell for multi-word queries, or pass
words as separate arguments (they are joined with spaces).

Examples:
  port skills search api
  port skills search demo onboard
  port skills search "api guide" --json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := GetGlobalFlags(ctx)
			query := strings.TrimSpace(strings.Join(args, " "))
			if query == "" {
				return fmt.Errorf("search query is required")
			}

			mod, _, err := newSkillsModuleWithFlags(ctx, flags, skillsOrgName(cmd))
			if err != nil {
				return err
			}

			entries, err := mod.SearchSkills(ctx, api.SearchSkillsQuery{
				Query: query,
				Limit: limit,
			})
			if err != nil {
				return fmt.Errorf("failed to search skills: %w", err)
			}

			if len(entries) == 0 {
				fmt.Printf("No skills matching %q.\n", query)
				return nil
			}
			if jsonOut {
				return printSkillsCatalogJSON(&api.SkillsSummaryResponse{
					OK:     true,
					Skills: entries,
				})
			}
			printSkillsSearchResults(entries, query)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print matches as JSON")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of skills to return (0 = no limit)")
	return cmd
}
