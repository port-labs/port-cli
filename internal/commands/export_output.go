package commands

import exportmodule "github.com/port-labs/port-cli/internal/modules/export"

type exportJSONSummaryOptions struct {
	SkipEntities             bool
	IncludedResources        []string
	ExcludedBlueprints       []string
	SchemaExcludedBlueprints []string
}

func exportJSONSummary(result *exportmodule.Result, opts exportJSONSummaryOptions) map[string]interface{} {
	return map[string]interface{}{
		"output_path":          result.OutputPath,
		"format":               result.Format,
		"blueprints_count":     result.BlueprintsCount,
		"entities_count":       result.EntitiesCount,
		"actions_count":        result.ActionsCount,
		"users_count":          result.UsersCount,
		"teams_count":          result.TeamsCount,
		"folders_count":        result.FoldersCount,
		"pages_count":          result.PagesCount,
		"integrations_count":   result.IntegrationsCount,
		"skipped_entities":     opts.SkipEntities,
		"included_resources":   opts.IncludedResources,
		"excluded_blueprints":  opts.ExcludedBlueprints,
		"schema_only_excluded": opts.SchemaExcludedBlueprints,
	}
}
