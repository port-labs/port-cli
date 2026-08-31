package commands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/export"
	"github.com/port-labs/port-cli/internal/output"
	"github.com/spf13/cobra"
)

// exportResources defines the resources accepted by the export command.
// It currently mirrors the shared ValidResources list. To support a different
// set for export only, replace the right-hand side with a command-specific
// slice, for example:
//
//	var exportResources = []string{"blueprints", "entities", "pages"}
var exportResources = slices.Clone(ValidResources)

// RegisterExport registers the export command.
func RegisterExport(rootCmd *cobra.Command) {
	var (
		outputPath                    string
		org                           string
		baseOrg                       string
		blueprints                    string
		excludeBlueprints             string
		excludeBlueprintSchema        string
		format                        string
		skipEntities                  bool
		skipSystemBlueprints          bool
		skipSystemBlueprintProperties bool
		includeRuleResults            bool
		include                       string
		outputFormat                  string
		maxErrors                     int

		scorecards   string
		actions      string
		pages        string
		integrations string
		teams        string
		users        string
		entities     string
	)

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export data from Port",
		Long: `Export data from Port organization.

Exports blueprints, entities, scorecards, actions, and teams to a file.
Use --skip-entities to only export configuration without entity data.
Use --include to selectively export specific resource types.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateStringEnum("--output-format", outputFormat, []string{"text", "json"}); err != nil {
				return err
			}
			if format != "" {
				if err := validateStringEnum("--format", format, []string{"tar", "json"}); err != nil {
					return err
				}
			}

			flags := GetGlobalFlags(cmd.Context())
			configManager := config.NewConfigManager(flags.ConfigFile)

			// Use base-org if provided, otherwise use org
			orgName := baseOrg
			if orgName == "" {
				orgName = org
			}

			_, baseOrgConfig, _, err := configManager.LoadWithDualOverrides(
				flags.ClientID,
				flags.ClientSecret,
				flags.APIURL,
				orgName,
				"", "", "", "", // No target org for export
			)
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			if baseOrgConfig == nil {
				return fmt.Errorf("base organization configuration not found")
			}
			if err := validateMaxErrorsFlag(maxErrors); err != nil {
				return err
			}

			orgConfig := baseOrgConfig

			// Parse blueprints list
			var blueprintList []string
			if blueprints != "" {
				blueprintList = strings.Split(blueprints, ",")
				for i := range blueprintList {
					blueprintList[i] = strings.TrimSpace(blueprintList[i])
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
					if err := ValidateResource(r, exportResources); err != nil {
						return err
					}
				}

				// page-permissions requires pages so that page identifiers can be collected
				hasPagePerms := slices.Contains(includeList, "page-permissions")
				hasPages := slices.Contains(includeList, "pages")
				if hasPagePerms && !hasPages {
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
			// --blueprints also restricts to blueprints resource type, consistent with other per-resource flags
			if cmd.Flags().Changed("blueprints") || (needBlueprints && len(includeList) > 0) {
				includeList = ensureContains(includeList, "blueprints")
			}
			autoScopeBlueprints := needBlueprints && !blueprintsExplicitlyRequested

			token, err := configManager.GetOrRefreshToken(cmd.Context(), orgName)
			if err != nil {
				if !config.ShouldIgnoreGetOrRefreshTokenError(err) {
					return err
				}
			}
			// Create export module
			exportModule := export.NewModule(token, orgConfig)
			defer exportModule.Close()

			// Show info only if not quiet and output format is text
			if outputFormat != "json" {
				output.Printf("\nExporting data from base organization: %s\n", orgName)
				if orgName == "" {
					output.Printf("(using default organization)\n")
				}
				output.Printf("Output file: %s\n", outputPath)
				if len(blueprintList) > 0 {
					output.Printf("Blueprints filter: %s\n", strings.Join(blueprintList, ", "))
				}
				if len(entityList) > 0 {
					output.Printf("Entities filter: %s\n", strings.Join(entityList, ", "))
				}
				if len(scorecardList) > 0 {
					output.Printf("Scorecards filter: %s\n", strings.Join(scorecardList, ", "))
				}
				if len(actionList) > 0 {
					output.Printf("Actions filter: %s\n", strings.Join(actionList, ", "))
				}
				if len(pageList) > 0 {
					output.Printf("Pages filter: %s\n", strings.Join(pageList, ", "))
				}
				if len(integrationList) > 0 {
					output.Printf("Integrations filter: %s\n", strings.Join(integrationList, ", "))
				}
				if len(teamList) > 0 {
					output.Printf("Teams filter: %s\n", strings.Join(teamList, ", "))
				}
				if len(userList) > 0 {
					output.Printf("Users filter: %s\n", strings.Join(userList, ", "))
				}
				if len(includeList) > 0 {
					output.Printf("Including only: %s\n", strings.Join(includeList, ", "))
				} else if skipEntities {
					output.Printf("Skipping entities (schema only)\n")
				}
			}

			// Execute export
			result, err := exportModule.Execute(cmd.Context(), export.Options{
				OutputPath:                    outputPath,
				Blueprints:                    blueprintList,
				ExcludeBlueprints:             excludeBlueprintList,
				ExcludeBlueprintSchema:        excludeBlueprintSchemaList,
				Format:                        format,
				SkipEntities:                  skipEntities,
				SkipSystemBlueprints:          skipSystemBlueprints,
				SkipSystemBlueprintProperties: skipSystemBlueprintProperties,
				IncludeRuleResults:            includeRuleResults,
				IncludeResources:              includeList,
				AutoScopeBlueprints:           autoScopeBlueprints,
				Entities:                      entityList,
				Scorecards:                    scorecardList,
				Actions:                       actionList,
				Pages:                         pageList,
				Integrations:                  integrationList,
				Teams:                         teamList,
				Users:                         userList,
			})
			if err != nil {
				if outputFormat == "json" {
					jsonResult := output.JSONResult{
						Success: false,
						Error:   err.Error(),
					}
					output.PrintJSON(jsonResult)
					return err
				}
				return fmt.Errorf("export failed: %w", err)
			}

			if !result.Success {
				if outputFormat == "json" {
					jsonResult := output.JSONResult{
						Success: false,
						Error:   fmt.Sprintf("%v", result.Error),
					}
					output.PrintJSON(jsonResult)
					return fmt.Errorf("export failed: %v", result.Error)
				}
				return fmt.Errorf("export failed: %v", result.Error)
			}

			// Output in JSON format if requested
			if outputFormat == "json" {
				jsonData := exportJSONSummary(result, exportJSONSummaryOptions{
					SkipEntities:             skipEntities,
					IncludedResources:        includeList,
					ExcludedBlueprints:       excludeBlueprintList,
					SchemaExcludedBlueprints: excludeBlueprintSchemaList,
				})
				if len(result.TimeoutErrors) > 0 {
					jsonData["timeout_errors"] = result.TimeoutErrors
					jsonData["warnings"] = fmt.Sprintf("%d blueprint(s) timed out during export", len(result.TimeoutErrors))
				}
				jsonResult := output.JSONResult{
					Success: true,
					Message: result.Message,
					Data:    jsonData,
				}
				return output.PrintJSON(jsonResult)
			}

			// Text output
			output.SuccessPrintln("\n✓ Export completed successfully!")
			output.Printf("%s\n", result.Message)
			output.Printf("Blueprints: %d\n", result.BlueprintsCount)
			output.Printf("Entities: %d\n", result.EntitiesCount)
			output.Printf("Actions: %d\n", result.ActionsCount)
			output.Printf("Users: %d\n", result.UsersCount)
			output.Printf("Teams: %d\n", result.TeamsCount)
			output.Printf("Pages: %d\n", result.PagesCount)
			output.Printf("Integrations: %d\n", result.IntegrationsCount)

			// Display timeout warnings if any
			if len(result.TimeoutErrors) > 0 && shouldPrintErrors(len(result.TimeoutErrors), maxErrors) {
				output.WarningPrintln("\n⚠ Warning: Some blueprints timed out during export:")
				limit := errorLimit(len(result.TimeoutErrors), maxErrors)
				for i := 0; i < limit; i++ {
					output.WarningPrintf("  - %s\n", result.TimeoutErrors[i])
				}
				if len(result.TimeoutErrors) > limit {
					output.WarningPrintf("  ... and %d more\n", len(result.TimeoutErrors)-limit)
				}
				output.WarningPrintln("These blueprints were skipped. Consider exporting them separately or contact Port support if this persists.")
			}

			return nil
		},
	}

	exportCmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (e.g., backup.tar.gz or backup.json)")
	exportCmd.MarkFlagRequired("output")
	exportCmd.Flags().StringVar(&org, "org", "", "Base organization name (uses default if not specified, deprecated: use --base-org)")
	exportCmd.Flags().StringVar(&baseOrg, "base-org", "", "Base organization name (uses default if not specified)")
	exportCmd.Flags().StringVarP(&blueprints, "blueprints", "b", "", "Comma-separated list of blueprint IDs to export (restricts export to blueprints resource type; exports all blueprints if flag set without IDs; pass this flag explicitly to export the full blueprint set even when combined with --actions/--scorecards/--entities)")
	exportCmd.Flags().StringVar(&excludeBlueprints, "exclude-blueprints", "", "Comma-separated blueprint IDs to exclude entirely (schema + entities + scorecards + actions)")
	exportCmd.Flags().StringVar(&excludeBlueprintSchema, "exclude-blueprint-schema", "", "Comma-separated blueprint IDs to exclude schema only (entities, scorecards, actions still exported)")
	exportCmd.Flags().StringVarP(&format, "format", "f", "", "Export format: tar (tar.gz) or json")
	exportCmd.Flags().BoolVar(&skipEntities, "skip-entities", false, "Skip exporting entities (only export schema and configuration)")
	exportCmd.Flags().BoolVar(&skipSystemBlueprints, "skip-system-blueprints", false, "Skip system blueprint schemas (identifiers starting with _) and their entities")
	exportCmd.Flags().BoolVar(&skipSystemBlueprintProperties, "skip-system-blueprint-properties", false, "When used with --skip-system-blueprints, do not export custom properties on known system blueprints")
	exportCmd.Flags().BoolVar(&includeRuleResults, "include-rule-results", true, "Include _rule_result system blueprint entities (use --include-rule-results=false to exclude)")
	validResourcesStr := strings.Join(exportResources, ", ")
	exportCmd.Flags().StringVar(&include, "include", "", fmt.Sprintf("Comma-separated list of resources to export (e.g., 'blueprints,pages'). Available: %s. If not specified, exports all resources.", validResourcesStr))
	exportCmd.Flags().StringVar(&outputFormat, "output-format", "text", "Output format: text or json")
	exportCmd.Flags().IntVar(&maxErrors, "max-errors", defaultMaxErrors, "Maximum number of errors to show in text output (-1 hides errors, 0 shows all)")

	exportCmd.Flags().StringVar(&scorecards, "scorecards", "", "Comma-separated scorecard IDs to export (restricts export to scorecards resource type; blueprint schemas exported alongside are scoped to only the blueprints the selected scorecards belong to — use --blueprints to export the full set instead)")
	exportCmd.Flags().StringVar(&actions, "actions", "", "Comma-separated action IDs to export (restricts export to actions resource type; exports all actions if flag set without IDs; blueprint schemas exported alongside are scoped to only the blueprints the selected actions belong to — use --blueprints to export the full set instead)")
	exportCmd.Flags().StringVar(&pages, "pages", "", "Comma-separated page IDs to export (restricts export to pages resource type)")
	exportCmd.Flags().StringVar(&integrations, "integrations", "", "Comma-separated integration IDs to export (restricts export to integrations resource type; exports integration mapping only)")
	exportCmd.Flags().StringVar(&teams, "teams", "", "Comma-separated team names to export (restricts export to teams resource type)")
	exportCmd.Flags().StringVar(&users, "users", "", "Comma-separated user emails to export (restricts export to users resource type)")
	exportCmd.Flags().StringVar(&entities, "entities", "", "Comma-separated entity IDs to export (restricts export to entities resource type; blueprint schemas exported alongside are scoped to only the blueprints the selected entities belong to — use --blueprints to export the full set instead)")

	rootCmd.AddCommand(exportCmd)
}
