package commands

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/compare"
	"github.com/spf13/cobra"
)

// RegisterCompare registers the compare command.
func RegisterCompare(rootCmd *cobra.Command) {
	var (
		source         string
		target         string
		sourceClientID string
		sourceSecret   string
		targetClientID string
		targetSecret   string
		outputFormat   string
		htmlFile       string
		htmlSimple     bool
		verbose        bool
		full           bool
		include        string
		failOnDiff     bool
	)

	compareCmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two Port organizations",
		Long: `Compare two Port organizations and show differences.

Compares blueprints, actions, scorecards, pages, integrations, teams, users, and automations
between a source and target organization. Use --include entities to also compare entities (opt-in, may be slow for large orgs).

Source and target can be:
- Organization names from config (e.g., 'staging', 'production')
- Export file paths (e.g., './staging-export.tar.gz')

Examples:
  # Compare two configured organizations
  port compare --source staging --target production

  # Compare with verbose output (show identifiers)
  port compare --source staging --target production --verbose

  # Compare with full diff (show field-level changes)
  port compare --source staging --target production --full

  # Compare export files
  port compare --source ./staging.tar.gz --target ./prod.tar.gz

  # Output as JSON
  port compare --source staging --target production --output json

  # Generate HTML report
  port compare --source staging --target production --output html

  # CI/CD mode: fail if differences found
  port compare --source staging --target production --fail-on-diff`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateStringEnum("--output", outputFormat, []string{"text", "json", "html"}); err != nil {
				return err
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			// Determine if inputs are files or org names
			sourceFile := ""
			sourceOrg := ""
			if isFilePath(source) {
				sourceFile = source
			} else {
				sourceOrg = source
			}

			targetFile := ""
			targetOrg := ""
			if isFilePath(target) {
				targetFile = target
			} else {
				targetOrg = target
			}

			// Parse and validate include list
			var includeList []string
			if include != "" {
				includeList = strings.Split(include, ",")
				for i := range includeList {
					includeList[i] = strings.TrimSpace(includeList[i])
				}
				validResources := map[string]bool{
					"blueprints": true, "actions": true, "scorecards": true,
					"pages": true, "integrations": true, "teams": true, "users": true,
					"automations": true, "blueprint-permissions": true, "action-permissions": true,
					"page-permissions": true, "entities": true,
				}
				for _, r := range includeList {
					if !validResources[r] {
						return fmt.Errorf("invalid resource: %s. Valid resources: blueprints, actions, automations, scorecards, pages, integrations, teams, users, blueprint-permissions, action-permissions, page-permissions, entities", r)
					}
				}

				if slices.Contains(includeList, "page-permissions") && !slices.Contains(includeList, "pages") {
					return fmt.Errorf("page-permissions requires pages to also be included (add 'pages' to --include)")
				}
			}

			opts := compare.Options{
				SourceOrg:        sourceOrg,
				TargetOrg:        targetOrg,
				SourceFile:       sourceFile,
				TargetFile:       targetFile,
				SourceClientID:   sourceClientID,
				SourceSecret:     sourceSecret,
				TargetClientID:   targetClientID,
				TargetSecret:     targetSecret,
				OutputFormat:     outputFormat,
				HTMLFile:         htmlFile,
				HTMLSimple:       htmlSimple,
				Verbose:          verbose,
				Full:             full,
				IncludeResources: includeList,
				FailOnDiff:       failOnDiff,
			}

			// Create module and execute
			module := compare.NewModule(configManager)
			result, err := module.Execute(cmd.Context(), opts)
			if err != nil {
				return fmt.Errorf("comparison failed: %w", err)
			}

			// Format output
			if err := module.FormatOutput(result, opts); err != nil {
				return fmt.Errorf("failed to format output: %w", err)
			}

			// Exit code handling
			if failOnDiff && !result.Identical {
				os.Exit(1)
			}

			return nil
		},
	}

	compareCmd.Flags().StringVar(&source, "source", "", "Source organization name or export file path (required)")
	compareCmd.Flags().StringVar(&target, "target", "", "Target organization name or export file path (required)")
	compareCmd.MarkFlagRequired("source")
	compareCmd.MarkFlagRequired("target")

	compareCmd.Flags().StringVar(&sourceClientID, "source-client-id", "", "Override source organization client ID")
	compareCmd.Flags().StringVar(&sourceSecret, "source-client-secret", "", "Override source organization client secret")
	compareCmd.Flags().StringVar(&targetClientID, "target-client-id", "", "Override target organization client ID")
	compareCmd.Flags().StringVar(&targetSecret, "target-client-secret", "", "Override target organization client secret")

	compareCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json, html")
	compareCmd.Flags().StringVar(&htmlFile, "html-file", "comparison-report.html", "Output path for HTML report")
	compareCmd.Flags().BoolVar(&htmlSimple, "html-simple", false, "Generate simple HTML (no interactive features)")

	compareCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show changed resource identifiers")
	compareCmd.Flags().BoolVar(&full, "full", false, "Show full field-level differences")
	compareCmd.Flags().StringVar(&include, "include", "", "Comma-separated list of resource types to compare")
	compareCmd.Flags().BoolVar(&failOnDiff, "fail-on-diff", false, "Exit with code 1 if differences found")

	rootCmd.AddCommand(compareCmd)
}

// isFilePath checks if the input looks like a file path.
func isFilePath(input string) bool {
	return strings.HasSuffix(input, ".tar.gz") ||
		strings.HasSuffix(input, ".json") ||
		strings.HasPrefix(input, "/") ||
		strings.HasPrefix(input, "./") ||
		strings.HasPrefix(input, "../") ||
		strings.Contains(input, string(os.PathSeparator))
}
