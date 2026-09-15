package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	MaxEntitySearchLimit        = 1000
	DefaultEntitySearchLimit    = 10
	DefaultEntityListLimit      = 100
	MaxEntitySearchPages        = 1000
	entitySearchCountPathSuffix = "/entities/search/count"
)

// EntitySearchQueryParams are passed as query-string parameters on entity search routes.
type EntitySearchQueryParams struct {
	AttachTitleToRelation        bool
	ExcludeCalculatedProperties bool
}

func (p EntitySearchQueryParams) ToMap() map[string]string {
	if !p.AttachTitleToRelation && !p.ExcludeCalculatedProperties {
		return nil
	}
	out := make(map[string]string)
	if p.AttachTitleToRelation {
		out["attach_title_to_relation"] = "true"
	}
	if p.ExcludeCalculatedProperties {
		out["exclude_calculated_properties"] = "true"
	}
	return out
}

// BlueprintEntitySearchBodyOptions configures a blueprint-scoped entity search request body.
type BlueprintEntitySearchBodyOptions struct {
	Query      map[string]interface{}
	Identifiers []string
	Include    []string
	Exclude    []string
	Limit      int
	From       string
	GroupBy    map[string]interface{}
	GroupSort  map[string]interface{}
	Sort       []map[string]interface{}
	CountOnly  bool
}

// BuildBlueprintEntitySearchBody builds the JSON body for blueprint entity search / top-search.
func BuildBlueprintEntitySearchBody(opts BlueprintEntitySearchBodyOptions) map[string]interface{} {
	body := make(map[string]interface{})
	if opts.Query != nil {
		body["query"] = opts.Query
	}
	if len(opts.Identifiers) > 0 {
		body["identifiers"] = opts.Identifiers
	}
	if len(opts.Include) > 0 {
		body["include"] = opts.Include
	}
	if len(opts.Exclude) > 0 {
		body["exclude"] = opts.Exclude
	}
	if opts.Limit > 0 {
		body["limit"] = opts.Limit
	}
	if opts.From != "" {
		body["from"] = opts.From
	}
	if opts.GroupBy != nil {
		body["groupBy"] = opts.GroupBy
	}
	if opts.GroupSort != nil {
		body["groupSort"] = opts.GroupSort
	}
	if len(opts.Sort) > 0 {
		body["sort"] = opts.Sort
	}
	if opts.CountOnly {
		body["countOnly"] = true
	}
	return body
}

// UsesTopSearchEntityRoute reports whether the request should use the top-search route
// for MCP parity (groupBy, sort, identifiers, countOnly combined with list semantics).
func UsesTopSearchEntityRoute(opts BlueprintEntitySearchBodyOptions) bool {
	return len(opts.Identifiers) > 0 ||
		opts.GroupBy != nil ||
		opts.GroupSort != nil ||
		len(opts.Sort) > 0 ||
		(opts.CountOnly && opts.GroupBy != nil)
}

func blueprintEntitySearchPath(blueprint string, topSearch bool) string {
	if topSearch {
		return fmt.Sprintf("/blueprints/%s/entities/top-search", blueprint)
	}
	return fmt.Sprintf("/blueprints/%s/entities/search", blueprint)
}

// BlueprintEntitySearch performs one page of blueprint-scoped entity search.
func (c *Client) BlueprintEntitySearch(
	ctx context.Context,
	blueprint string,
	opts BlueprintEntitySearchBodyOptions,
	queryParams EntitySearchQueryParams,
) (map[string]interface{}, error) {
	body := BuildBlueprintEntitySearchBody(opts)
	path := blueprintEntitySearchPath(blueprint, UsesTopSearchEntityRoute(opts))
	resp, err := c.request(ctx, "POST", path, body, queryParams.ToMap())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode entity search response: %w", err)
	}
	return result, nil
}

// BlueprintEntitySearchAll follows pagination cursors until exhausted or MaxEntitySearchPages.
func (c *Client) BlueprintEntitySearchAll(
	ctx context.Context,
	blueprint string,
	opts BlueprintEntitySearchBodyOptions,
	queryParams EntitySearchQueryParams,
) (map[string]interface{}, error) {
	if opts.GroupBy != nil {
		return c.BlueprintEntitySearch(ctx, blueprint, opts, queryParams)
	}

	merged := make([]Entity, 0, 256)
	var lastEnvelope map[string]interface{}
	from := opts.From
	for page := 0; page < MaxEntitySearchPages; page++ {
		pageOpts := opts
		pageOpts.From = from
		envelope, err := c.BlueprintEntitySearch(ctx, blueprint, pageOpts, queryParams)
		if err != nil {
			return nil, err
		}
		lastEnvelope = envelope
		entities, _ := envelope["entities"].([]interface{})
		for _, raw := range entities {
			if m, ok := raw.(map[string]interface{}); ok {
				merged = append(merged, m)
			}
		}
		next, _ := envelope["next"].(string)
		if next == "" {
			break
		}
		from = next
	}
	if lastEnvelope == nil {
		return map[string]interface{}{"ok": true, "entities": []Entity{}}, nil
	}
	lastEnvelope["entities"] = merged
	delete(lastEnvelope, "next")
	return lastEnvelope, nil
}

// BlueprintEntitySearchCount returns the count of entities matching a search query.
func (c *Client) BlueprintEntitySearchCount(
	ctx context.Context,
	blueprint string,
	query map[string]interface{},
	queryParams EntitySearchQueryParams,
) (map[string]interface{}, error) {
	body := map[string]interface{}{"query": query}
	path := fmt.Sprintf("/blueprints/%s%s", blueprint, entitySearchCountPathSuffix)
	resp, err := c.request(ctx, "POST", path, body, queryParams.ToMap())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode entity count response: %w", err)
	}
	return result, nil
}

// GlobalEntitySearch performs one request against POST /entities/search.
func (c *Client) GlobalEntitySearch(
	ctx context.Context,
	body map[string]interface{},
	queryParams EntitySearchQueryParams,
) (map[string]interface{}, error) {
	resp, err := c.request(ctx, "POST", "/entities/search", body, queryParams.ToMap())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode global entity search response: %w", err)
	}
	return result, nil
}

// LoadJSONObjectFromFlagOrFile loads a JSON object from an inline string or file path.
func LoadJSONObjectFromFlagOrFile(inline, filePath string) (map[string]interface{}, error) {
	var raw []byte
	switch {
	case inline != "" && filePath != "":
		return nil, fmt.Errorf("use only one of --query or --query-file")
	case inline != "":
		raw = []byte(inline)
	case filePath != "":
		var err error
		raw, err = os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read query file: %w", err)
		}
	default:
		return nil, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("failed to parse query JSON: %w", err)
	}
	return obj, nil
}

// MatchAllEntityQuery returns a query that matches all entities in a blueprint.
func MatchAllEntityQuery() map[string]interface{} {
	return map[string]interface{}{
		"combinator": "and",
		"rules":      []interface{}{},
	}
}

// ParseGroupBySpec parses CLI values like property:status, relation:quarter_r:$identifier.
func ParseGroupBySpec(spec string) (map[string]interface{}, error) {
	parts := strings.Split(spec, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid group-by %q: expected property:<name>, relation:<name>:<$identifier|$title>, or scorecard:<id>[:rule]", spec)
	}
	switch parts[0] {
	case "property":
		return map[string]interface{}{"property": parts[1]}, nil
	case "relation":
		out := map[string]interface{}{"relation": parts[1]}
		if len(parts) >= 3 {
			out["targetProperty"] = parts[2]
		} else {
			out["targetProperty"] = "$identifier"
		}
		return out, nil
	case "scorecard":
		out := map[string]interface{}{"scorecard": parts[1]}
		if len(parts) >= 3 {
			out["scorecardRule"] = parts[2]
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid group-by prefix %q", parts[0])
	}
}

// ParseSortSpec parses CLI values like property:allocation:desc.
func ParseSortSpec(spec string) (map[string]interface{}, error) {
	parts := strings.Split(spec, ":")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid sort %q: expected property:<name>:<asc|desc>, relation:<name>:<$identifier|$title>:<asc|desc>, or scorecard:<id>:<asc|desc>", spec)
	}
	order := parts[len(parts)-1]
	order = strings.ToLower(order)
	if order != "asc" && order != "desc" {
		return nil, fmt.Errorf("sort order must be asc or desc")
	}
	switch parts[0] {
	case "property":
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid property sort %q", spec)
		}
		return map[string]interface{}{"property": parts[1], "order": order}, nil
	case "relation":
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid relation sort %q", spec)
		}
		return map[string]interface{}{
			"relation":       parts[1],
			"targetProperty": parts[2],
			"order":          order,
		}, nil
	case "scorecard":
		if len(parts) == 3 {
			return map[string]interface{}{"scorecard": parts[1], "order": order}, nil
		}
		if len(parts) == 4 {
			return map[string]interface{}{"scorecard": parts[1], "scorecardRule": parts[2], "order": order}, nil
		}
		return nil, fmt.Errorf("invalid scorecard sort %q", spec)
	default:
		return nil, fmt.Errorf("invalid sort prefix %q", parts[0])
	}
}

// ParseGroupSortSpec parses CLI values like by:count order:desc.
func ParseGroupSortSpec(spec string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for _, field := range strings.Fields(spec) {
		kv := strings.SplitN(field, ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid group-sort field %q", field)
		}
		switch kv[0] {
		case "by":
			if kv[1] != "count" && kv[1] != "value" {
				return nil, fmt.Errorf("group-sort by must be count or value")
			}
			out["by"] = kv[1]
		case "order":
			order := strings.ToLower(kv[1])
			if order != "asc" && order != "desc" {
				return nil, fmt.Errorf("group-sort order must be asc or desc")
			}
			out["order"] = order
		default:
			return nil, fmt.Errorf("unknown group-sort key %q", kv[0])
		}
	}
	if out["by"] == nil || out["order"] == nil {
		return nil, fmt.Errorf("group-sort requires by:<count|value> and order:<asc|desc>")
	}
	return out, nil
}

// NormalizeAPIPath ensures API paths begin with "/" so they join correctly with the API base URL.
func NormalizeAPIPath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}
