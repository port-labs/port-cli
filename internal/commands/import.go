package commands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/import_module"
	"github.com/port-labs/port-cli/internal/output"
	"github.com/spf13/cobra"
)

// RegisterImport registers the import command.
func RegisterImport(rootCmd *cobra.Command) {
	var (
		input                         string
		org                           string
		targetOrg                     string
		dryRun                        bool
		skipEntities                  bool
		skipSystemBlueprints          bool
		skipSystemBlueprintProperties bool
		includeRuleResults            bool
		include                       string
		outputFormat                  string
		verbose                       bool
		showPagesPipeline             bool
		excludeBlueprints             string
		excludeBlueprintSchema        string
		usersAsDisabled               bool
		maxErrors                     int
		onError                       []string
	)

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import data to Port",
		Long: `Import data to Port organization.

Imports blueprints, entities, scorecards, actions, teams, automations, pages, and integrations from a file.
Use --skip-entities to only import configuration without entity data.
Use --include to selectively import specific resource types.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateStringEnum("--output-format", outputFormat, []string{"text", "json"}); err != nil {
				return err
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			// Use target-org if provided, otherwise use org
			orgName := targetOrg
			if orgName == "" {
				orgName = org
			}

			// Use target org flags if provided, otherwise fall back to base flags
			targetClientID := flags.TargetClientID
			targetClientSecret := flags.TargetClientSecret
			targetAPIURL := flags.TargetAPIURL
			if targetClientID == "" {
				targetClientID = flags.ClientID
				targetClientSecret = flags.ClientSecret
				targetAPIURL = flags.APIURL
			}

			_, _, targetOrgConfig, err := configManager.LoadWithDualOverrides(
				"", "", "", "", // No base org for import
				targetClientID,
				targetClientSecret,
				targetAPIURL,
				orgName,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			if targetOrgConfig == nil {
				return fmt.Errorf("target organization configuration not found")
			}
			if err := validateMaxErrorsFlag(maxErrors); err != nil {
				return err
			}
			errorHandling, err := buildErrorHandlingOptions(cmd, onError)
			if err != nil {
				return err
			}

			orgConfig := targetOrgConfig

			// Parse include list
			var includeList []string
			if include != "" {
				includeList = strings.Split(include, ",")
				for i := range includeList {
					includeList[i] = strings.TrimSpace(includeList[i])
				}

				// Validate resource types
				validResources := map[string]bool{
					"blueprints":            true,
					"entities":              true,
					"scorecards":            true,
					"actions":               true,
					"teams":                 true,
					"users":                 true,
					"automations":           true,
					"pages":                 true,
					"integrations":          true,
					"blueprint-permissions": true,
					"action-permissions":    true,
					"page-permissions":      true,
				}

				for _, r := range includeList {
					if !validResources[r] {
						return fmt.Errorf("invalid resource: %s. Valid resources: blueprints, entities, scorecards, actions, teams, users, automations, pages, integrations, blueprint-permissions, action-permissions, page-permissions", r)
					}
				}

				if slices.Contains(includeList, "page-permissions") && !slices.Contains(includeList, "pages") {
					return fmt.Errorf("page-permissions requires pages to also be included (add 'pages' to --include)")
				}

				// Handle conflict between skip_entities and include
				if skipEntities {
					for _, r := range includeList {
						if r == "entities" {
							output.WarningPrintln("Warning: --skip-entities conflicts with --include entities, ignoring --skip-entities")
							skipEntities = false
							break
						}
					}
				}
				if skipEntities {
					for _, r := range includeList {
						if r == "users" {
							output.WarningPrintln("Warning: --skip-entities conflicts with --include users, ignoring --skip-entities")
							skipEntities = false
							break
						}
						if r == "teams" {
							output.WarningPrintln("Warning: --skip-entities conflicts with --include teams, ignoring --skip-entities")
							skipEntities = false
							break
						}
					}
				}
			}

			// Parse exclude-blueprints (deep)
			var excludeBlueprintList []string
			if excludeBlueprints != "" {
				excludeBlueprintList = strings.Split(excludeBlueprints, ",")
				for i := range excludeBlueprintList {
					excludeBlueprintList[i] = strings.TrimSpace(excludeBlueprintList[i])
				}
			}

			// Parse exclude-blueprint-schema (schema-only)
			var excludeBlueprintSchemaList []string
			if excludeBlueprintSchema != "" {
				excludeBlueprintSchemaList = strings.Split(excludeBlueprintSchema, ",")
				for i := range excludeBlueprintSchemaList {
					excludeBlueprintSchemaList[i] = strings.TrimSpace(excludeBlueprintSchemaList[i])
				}
			}

			token, err := configManager.GetOrRefreshToken(cmd.Context(), orgName)
			if err != nil {
				if !config.ShouldIgnoreGetOrRefreshTokenError(err) {
					return err
				}
			}
			// Create import module
			importModule := import_module.NewModule(token, orgConfig)
			defer importModule.Close()

			// Show info only if not quiet and output format is text
			if outputFormat != "json" {
				output.Printf("\nImporting data to target organization: %s\n", orgName)
				if orgName == "" {
					output.Printf("(using default organization)\n")
				}
				output.Printf("Input file: %s\n", input)
				if dryRun {
					output.Printf("Dry run mode - no changes will be applied\n")
				}
				output.Printf("Diff validation enabled - comparing with current organization state\n")
				if len(includeList) > 0 {
					output.Printf("Including only: %s\n", strings.Join(includeList, ", "))
				} else if skipEntities {
					output.Printf("Skipping entities (schema only)\n")
				}
			}

			// Progress callback for real-time updates
			var progressCallback import_module.ProgressCallback
			var logCallback func(string)
			if outputFormat != "json" {
				lastPhase := ""
				progressCallback = func(phase string, current, total int) {
					if phase != lastPhase {
						if lastPhase != "" {
							output.Printf("\n")
						}
						lastPhase = phase
					}
					output.Printf("\r  %s: %d/%d", phase, current, total)
				}
				if showPagesPipeline || verbose {
					logCallback = func(message string) {
						output.Printf("%s\n", message)
					}
				}
			}

			// Execute import
			result, err := importModule.Execute(cmd.Context(), import_module.Options{
				InputPath:                     input,
				DryRun:                        dryRun,
				SkipEntities:                  skipEntities,
				SkipSystemBlueprints:          skipSystemBlueprints,
				SkipSystemBlueprintProperties: skipSystemBlueprintProperties,
				IncludeRuleResults:            includeRuleResults,
				IncludeResources:              includeList,
				ExcludeBlueprints:             excludeBlueprintList,
				ExcludeBlueprintSchema:        excludeBlueprintSchemaList,
				UsersAsDisabled:               usersAsDisabled,
				Verbose:                       verbose,
				ShowPagesPipeline:             showPagesPipeline,
				ProgressCallback:              progressCallback,
				LogCallback:                   logCallback,
				ErrorHandling:                 errorHandling,
			})

			// Clear progress line
			if outputFormat != "json" && progressCallback != nil {
				output.Printf("\n")
			}

			if err != nil {
				if outputFormat == "json" {
					jsonResult := output.JSONResult{
						Success: false,
						Error:   err.Error(),
					}
					output.PrintJSON(jsonResult)
					return err
				}
				return fmt.Errorf("import failed: %w", err)
			}

			// Output in JSON format if requested
			if outputFormat == "json" {
				jsonData := map[string]interface{}{
					"success":                       result.Success,
					"message":                       result.Message,
					"blueprints_created":            result.BlueprintsCreated,
					"blueprints_updated":            result.BlueprintsUpdated,
					"entities_created":              result.EntitiesCreated,
					"entities_updated":              result.EntitiesUpdated,
					"scorecards_created":            result.ScorecardsCreated,
					"scorecards_updated":            result.ScorecardsUpdated,
					"actions_created":               result.ActionsCreated,
					"actions_updated":               result.ActionsUpdated,
					"teams_created":                 result.TeamsCreated,
					"teams_updated":                 result.TeamsUpdated,
					"users_created":                 result.UsersCreated,
					"users_updated":                 result.UsersUpdated,
					"pages_created":                 result.PagesCreated,
					"pages_updated":                 result.PagesUpdated,
					"integrations_updated":          result.IntegrationsUpdated,
					"blueprint_permissions_updated": result.BlueprintPermissionsUpdated,
					"action_permissions_updated":    result.ActionPermissionsUpdated,
					"page_permissions_updated":      result.PagePermissionsUpdated,
				}
				if len(result.Errors) > 0 {
					jsonData["errors"] = result.Errors
				}
				if result.IgnoredRuleResultTargetRelationCount > 0 {
					jsonData["ignored_rule_result_target_relations_count"] = result.IgnoredRuleResultTargetRelationCount
					jsonData["ignored_rule_result_target_relation_keys"] = result.IgnoredRuleResultTargetRelationKeys
				}
				if showPagesPipeline && len(result.SidebarPipeline) > 0 {
					jsonData["sidebar_pipeline"] = result.SidebarPipeline
				}
				output.PrintJSON(jsonData)
				if !result.Success {
					return fmt.Errorf("import completed with errors")
				}
				return nil
			}

			// Text output
			if result.Success {
				output.SuccessPrintln("\n✓ Import completed successfully!")
			} else {
				output.WarningPrintln("\n⚠ Import completed with errors")
			}
			output.Printf("%s\n", result.Message)
			if result.IgnoredRuleResultTargetRelationCount > 0 {
				output.Printf("\n_rule_result: ignored %d relation(s) with type rule_result_target (not sent to API): %s\n",
					result.IgnoredRuleResultTargetRelationCount,
					strings.Join(result.IgnoredRuleResultTargetRelationKeys, ", "))
			}

			// Show diff stats (always available now)
			if result.DiffResult != nil {
				output.Printf("\nDiff analysis:\n")
				if len(result.DiffResult.BlueprintsToCreate) > 0 || len(result.DiffResult.BlueprintsToUpdate) > 0 || len(result.DiffResult.BlueprintsToSkip) > 0 {
					output.Printf("  Blueprints: %d new, %d updated, %d skipped (identical)\n",
						len(result.DiffResult.BlueprintsToCreate),
						len(result.DiffResult.BlueprintsToUpdate),
						len(result.DiffResult.BlueprintsToSkip))
				}
				if len(result.DiffResult.EntitiesToCreate) > 0 || len(result.DiffResult.EntitiesToUpdate) > 0 || len(result.DiffResult.EntitiesToSkip) > 0 {
					output.Printf("  Entities: %d new, %d updated, %d skipped (identical)\n",
						len(result.DiffResult.EntitiesToCreate),
						len(result.DiffResult.EntitiesToUpdate),
						len(result.DiffResult.EntitiesToSkip))
				}
				if len(result.DiffResult.ScorecardsToCreate) > 0 || len(result.DiffResult.ScorecardsToUpdate) > 0 || len(result.DiffResult.ScorecardsToSkip) > 0 {
					output.Printf("  Scorecards: %d new, %d updated, %d skipped (identical)\n",
						len(result.DiffResult.ScorecardsToCreate),
						len(result.DiffResult.ScorecardsToUpdate),
						len(result.DiffResult.ScorecardsToSkip))
				}
				if len(result.DiffResult.ActionsToCreate) > 0 || len(result.DiffResult.ActionsToUpdate) > 0 || len(result.DiffResult.ActionsToSkip) > 0 {
					output.Printf("  Actions: %d new, %d updated, %d skipped (identical)\n",
						len(result.DiffResult.ActionsToCreate),
						len(result.DiffResult.ActionsToUpdate),
						len(result.DiffResult.ActionsToSkip))
				}
				if len(result.DiffResult.TeamsToCreate) > 0 || len(result.DiffResult.TeamsToUpdate) > 0 || len(result.DiffResult.TeamsToSkip) > 0 {
					output.Printf("  Teams: %d new, %d updated, %d skipped (identical)\n",
						len(result.DiffResult.TeamsToCreate),
						len(result.DiffResult.TeamsToUpdate),
						len(result.DiffResult.TeamsToSkip))
				}
				if len(result.DiffResult.UsersToCreate) > 0 || len(result.DiffResult.UsersToUpdate) > 0 || len(result.DiffResult.UsersToSkip) > 0 {
					output.Printf("  Users: %d new, %d updated, %d skipped (identical)\n",
						len(result.DiffResult.UsersToCreate),
						len(result.DiffResult.UsersToUpdate),
						len(result.DiffResult.UsersToSkip))
				}
				if len(result.DiffResult.PagesToCreate) > 0 || len(result.DiffResult.PagesToUpdate) > 0 || len(result.DiffResult.PagesToSkip) > 0 {
					output.Printf("  Pages: %d new, %d updated, %d skipped (identical)\n",
						len(result.DiffResult.PagesToCreate),
						len(result.DiffResult.PagesToUpdate),
						len(result.DiffResult.PagesToSkip))
				}
				if len(result.DiffResult.IntegrationsToUpdate) > 0 || len(result.DiffResult.IntegrationsToSkip) > 0 {
					output.Printf("  Integrations: %d updated, %d skipped (identical)\n",
						len(result.DiffResult.IntegrationsToUpdate),
						len(result.DiffResult.IntegrationsToSkip))
				}
				if len(result.DiffResult.BlueprintPermissions) > 0 {
					output.Printf("  Blueprint permissions: %d to update\n",
						len(result.DiffResult.BlueprintPermissions))
				}
				if len(result.DiffResult.ActionPermissions) > 0 {
					output.Printf("  Action permissions: %d to update\n",
						len(result.DiffResult.ActionPermissions))
				}
				if len(result.DiffResult.PagePermissions) > 0 {
					output.Printf("  Page permissions: %d to update\n",
						len(result.DiffResult.PagePermissions))
				}
				output.Printf("\n")
			}

			output.Printf("Blueprints created: %d, updated: %d\n", result.BlueprintsCreated, result.BlueprintsUpdated)
			output.Printf("Entities created: %d, updated: %d\n", result.EntitiesCreated, result.EntitiesUpdated)
			output.Printf("Scorecards created: %d, updated: %d\n", result.ScorecardsCreated, result.ScorecardsUpdated)
			output.Printf("Actions created: %d, updated: %d\n", result.ActionsCreated, result.ActionsUpdated)
			output.Printf("Teams created: %d, updated: %d\n", result.TeamsCreated, result.TeamsUpdated)
			output.Printf("Users created: %d, updated: %d\n", result.UsersCreated, result.UsersUpdated)
			output.Printf("Pages created: %d, updated: %d\n", result.PagesCreated, result.PagesUpdated)
			output.Printf("Integrations updated: %d\n", result.IntegrationsUpdated)
			if result.BlueprintPermissionsUpdated > 0 || result.ActionPermissionsUpdated > 0 || result.PagePermissionsUpdated > 0 {
				output.Printf("Blueprint permissions updated: %d\n", result.BlueprintPermissionsUpdated)
				output.Printf("Action permissions updated: %d\n", result.ActionPermissionsUpdated)
				output.Printf("Page permissions updated: %d\n", result.PagePermissionsUpdated)
			}

			if showPagesPipeline && len(result.SidebarPipeline) > 0 {
				output.Printf("\nSidebar pipeline used:\n")
				for _, step := range result.SidebarPipeline {
					output.Printf("  %s\n", step)
				}
			}

			// Show warnings (cycle detection, etc.)
			if len(result.Warnings) > 0 {
				output.Printf("\nWarnings:\n")
				for _, warning := range result.Warnings {
					output.WarningPrintln(fmt.Sprintf("  ⚠ %s", warning.Message))
					if verbose && len(warning.Details) > 0 {
						for _, detail := range warning.Details {
							output.Printf("      - %s\n", detail)
						}
					}
				}
			}

			// Show errors
			if len(result.Errors) > 0 && shouldPrintErrors(len(result.Errors), maxErrors) {
				limit := errorLimit(len(result.Errors), maxErrors)
				if verbose && len(result.ErrorsByCategory) > 0 {
					// Verbose output: show errors grouped by category
					output.Printf("\nErrors by category:\n")
					categoryOrder := []string{"DEPENDENCY", "VALIDATION", "SCHEMA_MISMATCH", "BLUEPRINT_CONFIG", "AUTH", "NOT_FOUND", "CONFLICT", "RATE_LIMIT", "NETWORK", "UNKNOWN"}
					displayed := 0
				categories:
					for _, category := range categoryOrder {
						if errs, ok := result.ErrorsByCategory[category]; ok && len(errs) > 0 {
							output.Printf("\n  %s (%d):\n", category, len(errs))
							for _, errMsg := range errs {
								if displayed >= limit {
									break categories
								}
								output.Printf("    - %s\n", errMsg)
								displayed++
							}
						}
					}
					if len(result.Errors) > displayed {
						output.Printf("\n  ... and %d more\n", len(result.Errors)-displayed)
					}
				} else {
					// Standard output: simple error list
					output.Printf("\nErrors encountered:\n")
					for i := 0; i < limit; i++ {
						output.Printf("  - %s\n", result.Errors[i])
					}
					if len(result.Errors) > limit {
						output.Printf("  ... and %d more\n", len(result.Errors)-limit)
					}
				}
			}

			if !result.Success {
				return fmt.Errorf("import completed with errors")
			}
			return nil
		},
	}

	importCmd.Flags().StringVarP(&input, "input", "i", "", "Input file path (e.g., backup.tar.gz or backup.json)")
	importCmd.MarkFlagRequired("input")
	importCmd.Flags().StringVar(&org, "org", "", "Target organization name (uses default if not specified, deprecated: use --target-org)")
	importCmd.Flags().StringVar(&targetOrg, "target-org", "", "Target organization name (uses default if not specified)")
	importCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate import without applying changes")
	importCmd.Flags().BoolVar(&skipEntities, "skip-entities", false, "Skip importing entities (only import schema and configuration)")
	importCmd.Flags().BoolVar(&skipSystemBlueprints, "skip-system-blueprints", false, "Skip system blueprint schemas (identifiers starting with _) and their entities")
	importCmd.Flags().BoolVar(&skipSystemBlueprintProperties, "skip-system-blueprint-properties", false, "When used with --skip-system-blueprints, do not import custom properties on known system blueprints")
	importCmd.Flags().BoolVar(&includeRuleResults, "include-rule-results", true, "Include _rule_result system blueprint entities (use --include-rule-results=false to exclude)")
	importCmd.Flags().StringVar(&include, "include", "", "Comma-separated list of resources to import (e.g., 'blueprints,pages'). Available: blueprints, entities, scorecards, actions, teams, users, automations, pages, integrations. If not specified, imports all resources.")
	importCmd.Flags().StringVar(&excludeBlueprints, "exclude-blueprints", "", "Comma-separated blueprint IDs to exclude entirely (schema + entities + scorecards + actions)")
	importCmd.Flags().StringVar(&excludeBlueprintSchema, "exclude-blueprint-schema", "", "Comma-separated blueprint IDs to exclude schema only (entities, scorecards, actions still imported)")
	importCmd.Flags().StringVar(&outputFormat, "output-format", "text", "Output format: text or json")
	importCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed error information with categorization")
	importCmd.Flags().BoolVar(&showPagesPipeline, "show-pages-pipeline", false, "Show the planned sidebar pages/folders pipeline before execution and include the pipeline used in the output")
	importCmd.Flags().BoolVar(&usersAsDisabled, "users-as-disabled", false, "Import non-admin users as DISABLED (admin users are imported normally)")
	importCmd.Flags().IntVar(&maxErrors, "max-errors", defaultMaxErrors, "Maximum number of errors to show in text output (-1 hides errors, 0 shows all)")
	importCmd.Flags().StringArrayVar(&onError, "on-error", nil, "Handle a Port API error type (repeatable, e.g. forbidden_format_change=ignore-property or forbidden_format_change=recreate-property)")

	rootCmd.AddCommand(importCmd)
}
