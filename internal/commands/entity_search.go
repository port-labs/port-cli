package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/spf13/cobra"
)

func registerEntitySearchCommands() []*cobra.Command {
	return []*cobra.Command{
		registerEntitySearch(),
		registerEntityCount(),
		registerEntitySearchGlobal(),
	}
}

type entitySearchCommonFlags struct {
	org                         string
	format                      string
	unwrap                      string
	query                       string
	queryFile                   string
	include                     []string
	exclude                     []string
	limit                       int
	from                        string
	all                         bool
	attachTitleToRelation       bool
	excludeCalculatedProperties bool
	identifiers                 []string
	countOnly                   bool
	groupBy                     string
	groupSort                   string
	sort                        []string
}

func (f entitySearchCommonFlags) queryParams() api.EntitySearchQueryParams {
	return api.EntitySearchQueryParams{
		AttachTitleToRelation:       f.attachTitleToRelation,
		ExcludeCalculatedProperties: f.excludeCalculatedProperties,
	}
}

func (f entitySearchCommonFlags) resolveQuery() (map[string]interface{}, error) {
	queryObj, err := api.LoadJSONObjectFromFlagOrFile(f.query, f.queryFile)
	if err != nil {
		return nil, err
	}
	if queryObj == nil {
		if len(f.identifiers) == 0 {
			return api.MatchAllEntityQuery(), nil
		}
		return nil, nil
	}
	return queryObj, nil
}

func (f entitySearchCommonFlags) bodyOptions(query map[string]interface{}) (api.BlueprintEntitySearchBodyOptions, error) {
	if f.limit < 1 || f.limit > api.MaxEntitySearchLimit {
		return api.BlueprintEntitySearchBodyOptions{}, fmt.Errorf("--limit must be between 1 and %d", api.MaxEntitySearchLimit)
	}
	if len(f.identifiers) > 100 {
		return api.BlueprintEntitySearchBodyOptions{}, fmt.Errorf("--identifiers accepts at most 100 identifiers")
	}
	var groupBy map[string]interface{}
	if f.groupBy != "" {
		parsed, err := api.ParseGroupBySpec(f.groupBy)
		if err != nil {
			return api.BlueprintEntitySearchBodyOptions{}, err
		}
		groupBy = parsed
	}
	var groupSort map[string]interface{}
	if f.groupSort != "" {
		parsed, err := api.ParseGroupSortSpec(f.groupSort)
		if err != nil {
			return api.BlueprintEntitySearchBodyOptions{}, err
		}
		groupSort = parsed
	}
	sortSpecs := make([]map[string]interface{}, 0, len(f.sort))
	for _, spec := range f.sort {
		parsed, err := api.ParseSortSpec(spec)
		if err != nil {
			return api.BlueprintEntitySearchBodyOptions{}, err
		}
		sortSpecs = append(sortSpecs, parsed)
	}
	return api.BlueprintEntitySearchBodyOptions{
		Query:       query,
		Identifiers: f.identifiers,
		Include:     f.include,
		Exclude:     f.exclude,
		Limit:       f.limit,
		From:        f.from,
		GroupBy:     groupBy,
		GroupSort:   groupSort,
		Sort:        sortSpecs,
		CountOnly:   f.countOnly,
	}, nil
}

func writeEntityAPIResult(result map[string]interface{}, unwrap, format string) error {
	if unwrap != "" {
		value, ok := result[unwrap]
		if !ok {
			return fmt.Errorf("failed to unwrap %q from response", unwrap)
		}
		return formatOutput(value, format)
	}
	return formatOutput(result, format)
}

func registerEntitySearch() *cobra.Command {
	var flags entitySearchCommonFlags
	var blueprint string

	cmd := &cobra.Command{
		Use:   "search [blueprint]",
		Short: "Search entities in a blueprint",
		Long: `Search entities using POST /v1/blueprints/:blueprint/entities/search.

Advanced filters (--group-by, --sort, --identifiers, --count-only) use the top-search route for MCP parity when the public search schema rejects those fields.`,
		Example: `port api entities search planning_priority \
  --query '{"combinator":"and","rules":[{"relation":"quarter_r","operator":"=","value":"2026_q_4"}]}' \
  --limit 100 --include '$identifier' '$title' --unwrap entities`,
		RunE: func(cmd *cobra.Command, args []string) error {
			bp := blueprint
			if bp == "" && len(args) > 0 {
				bp = args[0]
			}
			if bp == "" {
				return fmt.Errorf("blueprint is required (positional argument or --blueprint)")
			}

			query, err := flags.resolveQuery()
			if err != nil {
				return err
			}
			bodyOpts, err := flags.bodyOptions(query)
			if err != nil {
				return err
			}

			client, err := newAPIClientFromCommand(cmd, flags.org)
			if err != nil {
				return err
			}
			defer client.Close()

			if flags.countOnly && bodyOpts.GroupBy == nil && len(bodyOpts.Sort) == 0 && len(bodyOpts.Identifiers) == 0 {
				result, err := client.BlueprintEntitySearchCount(cmd.Context(), bp, query, flags.queryParams())
				if err != nil {
					return fmt.Errorf("entity count failed: %w", err)
				}
				unwrap := flags.unwrap
				if unwrap == "" {
					unwrap = "count"
				}
				return writeEntityAPIResult(result, unwrap, flags.format)
			}

			var result map[string]interface{}
			if flags.all {
				result, err = client.BlueprintEntitySearchAll(cmd.Context(), bp, bodyOpts, flags.queryParams())
			} else {
				result, err = client.BlueprintEntitySearch(cmd.Context(), bp, bodyOpts, flags.queryParams())
			}
			if err != nil {
				return fmt.Errorf("entity search failed: %w", err)
			}
			return writeEntityAPIResult(result, flags.unwrap, flags.format)
		},
	}

	cmd.Flags().StringVar(&flags.org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&blueprint, "blueprint", "b", "", "Blueprint identifier")
	cmd.Flags().StringVar(&flags.query, "query", "", "Search query JSON ({\"combinator\",\"rules\"})")
	cmd.Flags().StringVar(&flags.queryFile, "query-file", "", "Path to search query JSON file")
	cmd.Flags().StringArrayVar(&flags.include, "include", nil, "Fields to include in entity payloads (repeatable)")
	cmd.Flags().StringArrayVar(&flags.exclude, "exclude", nil, "Fields to exclude (repeatable)")
	cmd.Flags().IntVar(&flags.limit, "limit", api.DefaultEntitySearchLimit, fmt.Sprintf("Page size (max %d)", api.MaxEntitySearchLimit))
	cmd.Flags().StringVar(&flags.from, "from", "", "Pagination cursor from a previous response next field")
	cmd.Flags().BoolVar(&flags.all, "all", false, "Follow pagination until all entities are retrieved")
	cmd.Flags().BoolVar(&flags.attachTitleToRelation, "attach-title-to-relation", false, "Attach related entity titles in relation fields")
	cmd.Flags().BoolVar(&flags.excludeCalculatedProperties, "exclude-calculated-properties", false, "Exclude calculated properties from results")
	cmd.Flags().StringSliceVar(&flags.identifiers, "identifiers", nil, "Pre-filter to entity identifiers (comma-separated, max 100)")
	cmd.Flags().BoolVar(&flags.countOnly, "count-only", false, "Return count only (uses top-search / MCP parity route)")
	cmd.Flags().StringVar(&flags.groupBy, "group-by", "", "Group results (property:<name>, relation:<name>:<$identifier|$title>, scorecard:<id>)")
	cmd.Flags().StringVar(&flags.groupSort, "group-sort", "", "Sort groups (by:count|value order:asc|desc)")
	cmd.Flags().StringArrayVar(&flags.sort, "sort", nil, "Sort order (property:<name>:<asc|desc>, repeatable)")
	cmd.Flags().StringVar(&flags.unwrap, "unwrap", "", "Print only this top-level response field (entities, groups, count, next, ok)")
	cmd.Flags().StringVarP(&flags.format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

func registerEntityCount() *cobra.Command {
	var org, format, unwrap, query, queryFile, blueprint string
	var attachTitleToRelation, excludeCalculatedProperties bool

	cmd := &cobra.Command{
		Use:   "count [blueprint]",
		Short: "Count entities matching a search query",
		RunE: func(cmd *cobra.Command, args []string) error {
			bp := blueprint
			if bp == "" && len(args) > 0 {
				bp = args[0]
			}
			if bp == "" {
				return fmt.Errorf("blueprint is required (positional argument or --blueprint)")
			}

			queryObj, err := api.LoadJSONObjectFromFlagOrFile(query, queryFile)
			if err != nil {
				return err
			}
			if queryObj == nil {
				queryObj = api.MatchAllEntityQuery()
			}

			client, err := newAPIClientFromCommand(cmd, org)
			if err != nil {
				return err
			}
			defer client.Close()

			params := api.EntitySearchQueryParams{
				AttachTitleToRelation:       attachTitleToRelation,
				ExcludeCalculatedProperties: excludeCalculatedProperties,
			}
			result, err := client.BlueprintEntitySearchCount(cmd.Context(), bp, queryObj, params)
			if err != nil {
				return fmt.Errorf("entity count failed: %w", err)
			}
			return writeEntityAPIResult(result, unwrap, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&blueprint, "blueprint", "b", "", "Blueprint identifier")
	cmd.Flags().StringVar(&query, "query", "", "Search query JSON")
	cmd.Flags().StringVar(&queryFile, "query-file", "", "Path to search query JSON file")
	cmd.Flags().BoolVar(&attachTitleToRelation, "attach-title-to-relation", false, "Attach related entity titles in relation fields")
	cmd.Flags().BoolVar(&excludeCalculatedProperties, "exclude-calculated-properties", false, "Exclude calculated properties from count")
	cmd.Flags().StringVar(&unwrap, "unwrap", "count", "Print only this top-level response field")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

func registerEntitySearchGlobal() *cobra.Command {
	var org, format, unwrap, query, queryFile string
	var attachTitleToRelation, excludeCalculatedProperties bool

	cmd := &cobra.Command{
		Use:   "search-global",
		Short: "Search entities across all blueprints",
		Long:  "Search entities using POST /v1/entities/search. The request body uses top-level combinator and rules (no query wrapper).",
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := api.LoadJSONObjectFromFlagOrFile(query, queryFile)
			if err != nil {
				return err
			}
			if body == nil {
				return fmt.Errorf("--query or --query-file is required")
			}
			if body["combinator"] == nil {
				return fmt.Errorf("global search body must include top-level combinator and rules (not wrapped in query)")
			}

			client, err := newAPIClientFromCommand(cmd, org)
			if err != nil {
				return err
			}
			defer client.Close()

			params := api.EntitySearchQueryParams{
				AttachTitleToRelation:       attachTitleToRelation,
				ExcludeCalculatedProperties: excludeCalculatedProperties,
			}
			result, err := client.GlobalEntitySearch(cmd.Context(), body, params)
			if err != nil {
				return fmt.Errorf("global entity search failed: %w", err)
			}
			return writeEntityAPIResult(result, unwrap, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVar(&query, "query", "", "Global search JSON ({\"combinator\",\"rules\"} at top level)")
	cmd.Flags().StringVar(&queryFile, "query-file", "", "Path to global search JSON file")
	cmd.Flags().BoolVar(&attachTitleToRelation, "attach-title-to-relation", false, "Attach related entity titles in relation fields")
	cmd.Flags().BoolVar(&excludeCalculatedProperties, "exclude-calculated-properties", false, "Exclude calculated properties from results")
	cmd.Flags().StringVar(&unwrap, "unwrap", "", "Print only this top-level response field")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")

	return cmd
}

func newAPIClientFromCommand(cmd *cobra.Command, org string) (*api.Client, error) {
	flags := GetGlobalFlags(cmd.Context())
	configManager := config.NewConfigManager(flags.ConfigFile)

	cfg, err := configManager.LoadWithOverrides(
		flags.ClientID,
		flags.ClientSecret,
		flags.APIURL,
		org,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	useOrg := cfg.GetOrgOrDefault(org)
	orgConfig, err := cfg.GetOrgConfig(useOrg)
	if err != nil {
		return nil, err
	}
	token, err := getOrRefreshCommandToken(cmd, configManager, useOrg)
	if err != nil {
		return nil, err
	}
	return api.NewClient(api.ClientOpts{
		Token:        token,
		ClientID:     orgConfig.ClientID,
		ClientSecret: orgConfig.ClientSecret,
		APIURL:       orgConfig.APIURL,
		Timeout:      0,
	}), nil
}

func registerEntityListSearchBacked() *cobra.Command {
	var org, format, blueprint string
	var limit int
	var all bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entities (search-backed; prefer entities search for large blueprints)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "Note: entities list uses search with a default limit. Use `port api entities search` for filters, pagination, and counts.")

			client, err := newAPIClientFromCommand(cmd, org)
			if err != nil {
				return err
			}
			defer client.Close()

			query := api.MatchAllEntityQuery()
			bodyOpts := api.BlueprintEntitySearchBodyOptions{
				Query: query,
				Limit: limit,
			}

			if blueprint != "" {
				var result map[string]interface{}
				if all {
					result, err = client.BlueprintEntitySearchAll(cmd.Context(), blueprint, bodyOpts, api.EntitySearchQueryParams{})
				} else {
					result, err = client.BlueprintEntitySearch(cmd.Context(), blueprint, bodyOpts, api.EntitySearchQueryParams{})
				}
				if err != nil {
					return fmt.Errorf("failed to list entities: %w", err)
				}
				entities, _ := result["entities"].([]interface{})
				if !all && result["next"] != nil && result["next"] != "" {
					fmt.Fprintf(os.Stderr, "warning: more entities available; use --all or `port api entities search` with --from\n")
				}
				return formatOutput(entities, format)
			}

			blueprints, err := client.GetBlueprints(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get blueprints: %w", err)
			}

			var result []api.Entity
			for _, bp := range blueprints {
				identifier, ok := bp["identifier"].(string)
				if !ok || strings.HasPrefix(identifier, "_") {
					continue
				}
				pageOpts := bodyOpts
				var envelope map[string]interface{}
				if all {
					envelope, err = client.BlueprintEntitySearchAll(cmd.Context(), identifier, pageOpts, api.EntitySearchQueryParams{})
				} else {
					envelope, err = client.BlueprintEntitySearch(cmd.Context(), identifier, pageOpts, api.EntitySearchQueryParams{})
				}
				if err != nil {
					continue
				}
				raw, _ := envelope["entities"].([]interface{})
				for _, item := range raw {
					if m, ok := item.(map[string]interface{}); ok {
						result = append(result, m)
					}
				}
			}
			return formatOutput(result, format)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "Organization name (uses default if not specified)")
	cmd.Flags().StringVarP(&format, "format", "f", "json", "Output format: json, yaml")
	cmd.Flags().StringVarP(&blueprint, "blueprint", "b", "", "Filter by blueprint ID")
	cmd.Flags().IntVar(&limit, "limit", api.DefaultEntityListLimit, fmt.Sprintf("Maximum entities per blueprint (max %d)", api.MaxEntitySearchLimit))
	cmd.Flags().BoolVar(&all, "all", false, "Follow pagination for each blueprint")

	return cmd
}
