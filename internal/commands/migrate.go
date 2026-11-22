package commands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/migrate"
	"github.com/port-labs/port-cli/internal/output"
	"github.com/spf13/cobra"
)

// migrateResources defines the resources accepted by the migrate command.
// It currently mirrors the shared ValidResources list. To support a different
// set for migrate only, replace the right-hand side with a command-specific
// slice, for example:
//
//	var migrateResources = []string{"blueprints", "entities", "pages"}
var migrateResources = slices.Clone(ValidResources)

// RegisterMigrate registers the migrate command.
func RegisterMigrate(rootCmd *cobra.Command) {
	var (
		sourceOrg                     string
		baseOrg                       string
		targetOrg                     string
		blueprints                    string
		dryRun                        bool
		skipEntities                  bool
		skipSystemBlueprints          bool
		skipSystemBlueprintProperties bool
		includeRuleResults            bool
		include                       string
		outputFormat                  string
		excludeBlueprints             string
		excludeBlueprintSchema        string
		usersAsDisabled               bool
		maxErrors                     int
		onError                       []string

		scorecards   string
		actions      string
		pages        string
		integrations string
		teams        string
		users        string
		entities     string
	)

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate data between Port organizations",
		Long: `Migrate data between Port organizations.

Migrates blueprints, entities, scorecards, actions, teams, users, pages, and integrations from source to target organization.
Use --skip-entities to only migrate configuration without entity data.
Use --include to selectively migrate specific resource types.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateStringEnum("--output-format", outputFormat, []string{"text", "json"}); err != nil {
				return err
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			// Use base-org if provided, otherwise use source-org
			sourceOrgName := baseOrg
			if sourceOrgName == "" {
				sourceOrgName = sourceOrg
			}

			// Validate that source org is provided
			if sourceOrgName == "" {
				return fmt.Errorf("source organization is required. Use --source-org or --base-org")
			}

			// Validate that target org is provided
			if targetOrg == "" {
				return fmt.Errorf("target organization is required. Use --target-org")
			}
			if err := validateMaxErrorsFlag(maxErrors); err != nil {
				return err
			}
			errorHandling, err := buildErrorHandlingOptions(cmd, onError)
			if err != nil {
				return err
			}

			// Use CLI flags if provided, otherwise use org names from config
			baseClientID := flags.ClientID
			baseClientSecret := flags.ClientSecret
			baseAPIURL := flags.APIURL
			targetClientID := flags.TargetClientID
			targetClientSecret := flags.TargetClientSecret
			targetAPIURL := flags.TargetAPIURL

			_, baseOrgConfig, targetOrgConfig, err := configManager.LoadWithDualOverrides(
				baseClientID,
				baseClientSecret,
				baseAPIURL,
				sourceOrgName,
				targetClientID,
				targetClientSecret,
				targetAPIURL,
				targetOrg,
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			if baseOrgConfig == nil {
				return fmt.Errorf("base organization configuration not found")
			}

			if targetOrgConfig == nil {
				return fmt.Errorf("target organization configuration not found")
			}

			// Parse blueprints list
			var blueprintList []string
			if blueprints != "" {
				blueprintList = strings.Split(blueprints, ",")
				for i := range blueprintList {
					blueprintList[i] = strings.TrimSpace(blueprintList[i])
				}
			}

			// Parse per-resource ID filters
			parseCSV := func(s string) []string {
				if s == "" {
					return nil
				}
				parts := strings.Split(s, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				return parts
			}
			entityList := parseCSV(entities)
			scorecardList := parseCSV(scorecards)
			actionList := parseCSV(actions)
			pageList := parseCSV(pages)
			integrationList := parseCSV(integrations)
			teamList := parseCSV(teams)
			userList := parseCSV(users)

			// Parse include list
			var includeList []string
			if include != "" {
				includeList = strings.Split(include, ",")
				for i := range includeList {
					includeList[i] = strings.TrimSpace(includeList[i])
				}

				for _, r := range includeList {
					if err := ValidateResource(r, migrateResources); err != nil {
						return err
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

			// True when the caller explicitly wants blueprint schemas, either via
			// --blueprints or --include blueprints — as opposed to blueprints only
			// being pulled in as a byproduct of --actions/--scorecards/--entities.
			blueprintsExplicitlyRequested := cmd.Flags().Changed("blueprints") || slices.Contains(includeList, "blueprints")

			// Auto-include resource types when per-resource flags are explicitly set
			// (with or without specific IDs — Changed() detects explicit flag usage)
			ensureContains := func(list []string, item string) []string {
				for _, v := range list {
					if v == item {
						return list
					}
				}
				return append(list, item)
			}
			needBlueprints := false
			if len(entityList) > 0 || cmd.Flags().Changed("entities") {
				includeList = ensureContains(includeList, "entities")
				needBlueprints = true
			}
			if len(scorecardList) > 0 || cmd.Flags().Changed("scorecards") {
				includeList = ensureContains(includeList, "scorecards")
				needBlueprints = true
			}
			if len(actionList) > 0 || cmd.Flags().Changed("actions") {
				includeList = ensureContains(includeList, "actions")
				includeList = ensureContains(includeList, "action-permissions")
				needBlueprints = true
			}
			if len(pageList) > 0 || cmd.Flags().Changed("pages") {
				includeList = ensureContains(includeList, "pages")
				includeList = ensureContains(includeList, "page-permissions")
			}
			if len(integrationList) > 0 || cmd.Flags().Changed("integrations") {
				includeList = ensureContains(includeList, "integrations")
			}
			if len(teamList) > 0 || cmd.Flags().Changed("teams") {
				includeList = ensureContains(includeList, "teams")
			}
			if len(userList) > 0 || cmd.Flags().Changed("users") {
				includeList = ensureContains(includeList, "users")
			}
			if cmd.Flags().Changed("blueprints") || (needBlueprints && len(includeList) > 0) {
				includeList = ensureContains(includeList, "blueprints")
			}
			autoScopeBlueprints := needBlueprints && !blueprintsExplicitlyRequested

			// Parse exclude-blueprints flag
			var excludeBlueprintList []string
			if excludeBlueprints != "" {
				for _, id := range strings.Split(excludeBlueprints, ",") {
					if trimmed := strings.TrimSpace(id); trimmed != "" {
						excludeBlueprintList = append(excludeBlueprintList, trimmed)
					}
				}
			}

			// Parse exclude-blueprint-schema flag
			var excludeBlueprintSchemaList []string
			if excludeBlueprintSchema != "" {
				for _, id := range strings.Split(excludeBlueprintSchema, ",") {
					if trimmed := strings.TrimSpace(id); trimmed != "" {
						excludeBlueprintSchemaList = append(excludeBlueprintSchemaList, trimmed)
					}
				}
			}

			// Create migration module
			sourceToken, err := configManager.GetOrRefreshToken(cmd.Context(), sourceOrgName)
			if err != nil {
				if !config.ShouldIgnoreGetOrRefreshTokenError(err) {
					return err
				}
			}
			targetToken, err := configManager.GetOrRefreshToken(cmd.Context(), targetOrg)
			if err != nil {
				if !config.ShouldIgnoreGetOrRefreshTokenError(err) {
					return err
				}
			}
			migrateModule := migrate.NewModule(sourceToken, targetToken, baseOrgConfig, targetOrgConfig)
			defer migrateModule.Close()

			// Show info only if not quiet and output format is text
			if outputFormat != "json" {
				output.Printf("\nMigration:\n")
				output.Printf("  Source (base org): %s\n", sourceOrgName)
				output.Printf("  Target org: %s\n", targetOrg)
				if len(blueprintList) > 0 {
					output.Printf("  Blueprints: %s\n", strings.Join(blueprintList, ", "))
				}
				if len(entityList) > 0 {
					output.Printf("  Entities filter: %s\n", strings.Join(entityList, ", "))
				}
				if len(scorecardList) > 0 {
					output.Printf("  Scorecards filter: %s\n", strings.Join(scorecardList, ", "))
				}
				if len(actionList) > 0 {
					output.Printf("  Actions filter: %s\n", strings.Join(actionList, ", "))
				}
				if len(pageList) > 0 {
					output.Printf("  Pages filter: %s\n", strings.Join(pageList, ", "))
				}
				if len(integrationList) > 0 {
					output.Printf("  Integrations filter: %s\n", strings.Join(integrationList, ", "))
				}
				if len(teamList) > 0 {
					output.Printf("  Teams filter: %s\n", strings.Join(teamList, ", "))
				}
				if len(userList) > 0 {
					output.Printf("  Users filter: %s\n", strings.Join(userList, ", "))
				}
				output.Printf("Diff validation enabled - comparing source with target organization state\n")
				if len(includeList) > 0 {
					output.Printf("  Including only: %s\n", strings.Join(includeList, ", "))
				} else if skipEntities {
					output.Printf("  Skipping entities (schema only)\n")
				}
				if dryRun {
					output.Printf("  Dry run mode - no changes will be applied\n")
				}
			}

			// Execute migration
			result, err := migrateModule.Execute(cmd.Context(), migrate.Options{
				Blueprints:                    blueprintList,
				DryRun:                        dryRun,
				SkipEntities:                  skipEntities,
				SkipSystemBlueprints:          skipSystemBlueprints,
				SkipSystemBlueprintProperties: skipSystemBlueprintProperties,
				IncludeRuleResults:            includeRuleResults,
				IncludeResources:              includeList,
				AutoScopeBlueprints:           autoScopeBlueprints,
				ExcludeBlueprints:             excludeBlueprintList,
				ExcludeBlueprintSchema:        excludeBlueprintSchemaList,
				UsersAsDisabled:               usersAsDisabled,
				ErrorHandling:                 errorHandling,
				Entities:                      entityList,
				Scorecards:                    scorecardList,
				Actions:                       actionList,
				Pages:                         pageList,
				Integrations:                  integrationList,
				Teams:                         teamList,
				Users:                         userList,
			})
			if err != nil {
				failureMessage := migrationExecutionErrorMessage(err, result, maxErrors)
				if outputFormat == "json" {
					jsonData := map[string]interface{}{
						"success": false,
						"error":   failureMessage,
					}
					if result != nil {
						jsonData["blueprints_created"] = result.BlueprintsCreated
						jsonData["blueprints_updated"] = result.BlueprintsUpdated
						jsonData["blueprints_skipped"] = result.BlueprintsSkipped
						jsonData["entities_created"] = result.EntitiesCreated
						jsonData["entities_updated"] = result.EntitiesUpdated
						jsonData["entities_skipped"] = result.EntitiesSkipped
						jsonData["scorecards_created"] = result.ScorecardsCreated
						jsonData["scorecards_updated"] = result.ScorecardsUpdated
						jsonData["scorecards_skipped"] = result.ScorecardsSkipped
						jsonData["actions_created"] = result.ActionsCreated
						jsonData["actions_updated"] = result.ActionsUpdated
						jsonData["actions_skipped"] = result.ActionsSkipped
						jsonData["teams_created"] = result.TeamsCreated
						jsonData["teams_updated"] = result.TeamsUpdated
						jsonData["teams_skipped"] = result.TeamsSkipped
						jsonData["users_created"] = result.UsersCreated
						jsonData["users_updated"] = result.UsersUpdated
						jsonData["users_skipped"] = result.UsersSkipped
						jsonData["pages_created"] = result.PagesCreated
						jsonData["pages_updated"] = result.PagesUpdated
						jsonData["pages_skipped"] = result.PagesSkipped
						jsonData["integrations_updated"] = result.IntegrationsUpdated
						jsonData["integrations_skipped"] = result.IntegrationsSkipped
						if len(result.Errors) > 0 {
							jsonData["errors"] = result.Errors
						}
						if len(result.Warnings) > 0 {
							jsonData["warnings"] = result.Warnings
						}
					}
					output.PrintJSON(jsonData)
					return fmt.Errorf("%s", failureMessage)
				}
				output.ErrorPrintf("%s\n", failureMessage)
				if result != nil {
					output.Printf("\nPartial migration results:\n")
					output.Printf("Blueprints created: %d, updated: %d, skipped: %d\n", result.BlueprintsCreated, result.BlueprintsUpdated, result.BlueprintsSkipped)
					output.Printf("Entities created: %d, updated: %d, skipped: %d\n", result.EntitiesCreated, result.EntitiesUpdated, result.EntitiesSkipped)
					output.Printf("Scorecards created: %d, updated: %d, skipped: %d\n", result.ScorecardsCreated, result.ScorecardsUpdated, result.ScorecardsSkipped)
					output.Printf("Actions created: %d, updated: %d, skipped: %d\n", result.ActionsCreated, result.ActionsUpdated, result.ActionsSkipped)
					output.Printf("Teams created: %d, updated: %d, skipped: %d\n", result.TeamsCreated, result.TeamsUpdated, result.TeamsSkipped)
					output.Printf("Users created: %d, updated: %d, skipped: %d\n", result.UsersCreated, result.UsersUpdated, result.UsersSkipped)
					output.Printf("Pages created: %d, updated: %d, skipped: %d\n", result.PagesCreated, result.PagesUpdated, result.PagesSkipped)
					output.Printf("Integrations updated: %d, skipped: %d\n", result.IntegrationsUpdated, result.IntegrationsSkipped)
				}
				return fmt.Errorf("%s", failureMessage)
			}

			if !result.Success {
				failureMessage := migrationFailureMessage(result, maxErrors)
				if outputFormat == "json" {
					jsonData := map[string]interface{}{
						"success": false,
						"error":   failureMessage,
					}
					if len(result.Errors) > 0 {
						jsonData["errors"] = result.Errors
					}
					if len(result.Warnings) > 0 {
						jsonData["warnings"] = result.Warnings
					}
					output.PrintJSON(jsonData)
					return fmt.Errorf("%s", failureMessage)
				}
				return fmt.Errorf("%s", failureMessage)
			}

			// Output in JSON format if requested
			if outputFormat == "json" {
				jsonData := map[string]interface{}{
					"success":                       true,
					"message":                       result.Message,
					"blueprints_created":            result.BlueprintsCreated,
					"blueprints_updated":            result.BlueprintsUpdated,
					"blueprints_skipped":            result.BlueprintsSkipped,
					"entities_created":              result.EntitiesCreated,
					"entities_updated":              result.EntitiesUpdated,
					"entities_skipped":              result.EntitiesSkipped,
					"scorecards_created":            result.ScorecardsCreated,
					"scorecards_updated":            result.ScorecardsUpdated,
					"scorecards_skipped":            result.ScorecardsSkipped,
					"actions_created":               result.ActionsCreated,
					"actions_updated":               result.ActionsUpdated,
					"actions_skipped":               result.ActionsSkipped,
					"teams_created":                 result.TeamsCreated,
					"teams_updated":                 result.TeamsUpdated,
					"teams_skipped":                 result.TeamsSkipped,
					"users_created":                 result.UsersCreated,
					"users_updated":                 result.UsersUpdated,
					"users_skipped":                 result.UsersSkipped,
					"pages_created":                 result.PagesCreated,
					"pages_updated":                 result.PagesUpdated,
					"pages_skipped":                 result.PagesSkipped,
					"integrations_updated":          result.IntegrationsUpdated,
					"integrations_skipped":          result.IntegrationsSkipped,
					"blueprint_permissions_updated": result.BlueprintPermissionsUpdated,
					"action_permissions_updated":    result.ActionPermissionsUpdated,
					"page_permissions_updated":      result.PagePermissionsUpdated,
				}
				if len(result.Errors) > 0 {
					jsonData["errors"] = result.Errors
				}
				if len(result.Warnings) > 0 {
					jsonData["warnings"] = result.Warnings
				}
				if result.IgnoredRuleResultTargetRelationCount > 0 {
					jsonData["ignored_rule_result_target_relations_count"] = result.IgnoredRuleResultTargetRelationCount
					jsonData["ignored_rule_result_target_relation_keys"] = result.IgnoredRuleResultTargetRelationKeys
				}
				addMigrationDetailJSON(jsonData, result)
				return output.PrintJSON(jsonData)
			}

			// Text output
			output.SuccessPrintln("\n✓ Migration completed successfully!")
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
				output.Printf("\n")
			}

			output.Printf("Blueprints created: %d, updated: %d, skipped: %d\n", result.BlueprintsCreated, result.BlueprintsUpdated, result.BlueprintsSkipped)
			output.Printf("Entities created: %d, updated: %d, skipped: %d\n", result.EntitiesCreated, result.EntitiesUpdated, result.EntitiesSkipped)
			output.Printf("Scorecards created: %d, updated: %d, skipped: %d\n", result.ScorecardsCreated, result.ScorecardsUpdated, result.ScorecardsSkipped)
			output.Printf("Actions created: %d, updated: %d, skipped: %d\n", result.ActionsCreated, result.ActionsUpdated, result.ActionsSkipped)
			output.Printf("Teams created: %d, updated: %d, skipped: %d\n", result.TeamsCreated, result.TeamsUpdated, result.TeamsSkipped)
			output.Printf("Users created: %d, updated: %d, skipped: %d\n", result.UsersCreated, result.UsersUpdated, result.UsersSkipped)
			output.Printf("Pages created: %d, updated: %d, skipped: %d\n", result.PagesCreated, result.PagesUpdated, result.PagesSkipped)
			output.Printf("Integrations updated: %d, skipped: %d\n", result.IntegrationsUpdated, result.IntegrationsSkipped)
			if flags.Verbose {
				printMigrationVerboseDetails(result)
			}
			if result.PagePermissionsUpdated > 0 {
				output.Printf("Page permissions updated: %d\n", result.PagePermissionsUpdated)
			}

			if len(result.Warnings) > 0 {
				output.Printf("\nWarnings:\n")
				for _, w := range result.Warnings {
					output.WarningPrintln(fmt.Sprintf("  ⚠ %s", w))
				}
			}

			if len(result.Errors) > 0 {
				limit := errorLimit(len(result.Errors), maxErrors)
				if limit > 0 {
					output.Printf("\nErrors:\n")
					for i := 0; i < limit; i++ {
						output.Printf("  - %s\n", result.Errors[i])
					}
					if len(result.Errors) > limit {
						output.Printf("  ... and %d more\n", len(result.Errors)-limit)
					}
				}
			}

			return nil
		},
	}

	migrateCmd.Flags().StringVarP(&sourceOrg, "source-org", "s", "", "Source organization name (base org)")
	migrateCmd.Flags().StringVar(&baseOrg, "base-org", "", "Base organization name (alias for --source-org)")
	migrateCmd.Flags().StringVarP(&targetOrg, "target-org", "t", "", "Target organization name")
	migrateCmd.MarkFlagRequired("target-org")
	migrateCmd.Flags().StringVarP(&blueprints, "blueprints", "b", "", "Comma-separated list of blueprint IDs to migrate (restricts migration to blueprints resource type; migrates all blueprints if flag set without IDs; pass this flag explicitly to migrate the full blueprint set even when combined with --actions/--scorecards/--entities)")
	migrateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate migration without applying changes")
	migrateCmd.Flags().BoolVar(&skipEntities, "skip-entities", false, "Skip migrating entities (only migrate schema and configuration)")
	migrateCmd.Flags().BoolVar(&skipSystemBlueprints, "skip-system-blueprints", false, "Skip system blueprint schemas (identifiers starting with _) and their entities")
	migrateCmd.Flags().BoolVar(&skipSystemBlueprintProperties, "skip-system-blueprint-properties", false, "When used with --skip-system-blueprints, do not migrate custom properties on known system blueprints")
	migrateCmd.Flags().BoolVar(&includeRuleResults, "include-rule-results", true, "Include _rule_result system blueprint entities (use --include-rule-results=false to exclude)")
	validResourcesStr := strings.Join(migrateResources, ", ")
	migrateCmd.Flags().StringVar(&include, "include", "", fmt.Sprintf("Comma-separated list of resources to migrate (e.g., 'blueprints,pages'). Available: %s. If not specified, migrates all resources.", validResourcesStr))
	migrateCmd.Flags().StringVar(&excludeBlueprints, "exclude-blueprints", "", "Comma-separated blueprint IDs to exclude entirely (schema + entities + scorecards + actions)")
	migrateCmd.Flags().StringVar(&excludeBlueprintSchema, "exclude-blueprint-schema", "", "Comma-separated blueprint IDs to exclude schema only (entities, scorecards, actions still migrated)")
	migrateCmd.Flags().StringVar(&outputFormat, "output-format", "text", "Output format: text or json")
	migrateCmd.Flags().BoolVar(&usersAsDisabled, "users-as-disabled", false, "Import non-admin users as DISABLED (admin users are imported normally)")
	migrateCmd.Flags().IntVar(&maxErrors, "max-errors", defaultMaxErrors, "Maximum number of errors to show in text output (-1 hides errors, 0 shows all)")
	migrateCmd.Flags().StringArrayVar(&onError, "on-error", nil, "Handle a Port API error type (repeatable, e.g. forbidden_format_change=ignore-property or forbidden_format_change=recreate-property)")

	migrateCmd.Flags().StringVar(&scorecards, "scorecards", "", "Comma-separated scorecard IDs to migrate (restricts migration to scorecards resource type; blueprint schemas migrated alongside are scoped to only the blueprints the selected scorecards belong to — use --blueprints to migrate the full set instead)")
	migrateCmd.Flags().StringVar(&actions, "actions", "", "Comma-separated action IDs to migrate (restricts migration to actions resource type; migrates all actions if flag set without IDs; blueprint schemas migrated alongside are scoped to only the blueprints the selected actions belong to — use --blueprints to migrate the full set instead)")
	migrateCmd.Flags().StringVar(&pages, "pages", "", "Comma-separated page IDs to migrate (restricts migration to pages resource type)")
	migrateCmd.Flags().StringVar(&integrations, "integrations", "", "Comma-separated integration IDs to migrate (restricts migration to integrations resource type; exports integration mapping only)")
	migrateCmd.Flags().StringVar(&teams, "teams", "", "Comma-separated team names to migrate (restricts migration to teams resource type)")
	migrateCmd.Flags().StringVar(&users, "users", "", "Comma-separated user emails to migrate (restricts migration to users resource type)")
	migrateCmd.Flags().StringVar(&entities, "entities", "", "Comma-separated entity IDs to migrate (restricts migration to entities resource type; blueprint schemas migrated alongside are scoped to only the blueprints the selected entities belong to — use --blueprints to migrate the full set instead)")

	rootCmd.AddCommand(migrateCmd)
}

func migrationFailureMessage(result *migrate.Result, maxErrors int) string {
	if result == nil || len(result.Errors) == 0 {
		return "migration failed"
	}
	var b strings.Builder
	if result.Message != "" {
		b.WriteString(result.Message)
	} else {
		b.WriteString("migration failed")
	}
	limit := errorLimit(len(result.Errors), maxErrors)
	if limit == 0 {
		return b.String()
	}
	b.WriteString(":")
	for i := 0; i < limit; i++ {
		b.WriteString("\n  - ")
		b.WriteString(result.Errors[i])
	}
	if len(result.Errors) > limit {
		fmt.Fprintf(&b, "\n  ... and %d more", len(result.Errors)-limit)
	}
	return b.String()
}

func migrationExecutionErrorMessage(err error, result *migrate.Result, maxErrors int) string {
	if result != nil && len(result.Errors) > 0 {
		return migrationFailureMessage(result, maxErrors)
	}
	if err == nil {
		return "migration failed"
	}
	return fmt.Sprintf("migration failed: %v", err)
}
