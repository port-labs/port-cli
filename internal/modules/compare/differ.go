// Package compare provides functionality for comparing two Port organizations.
package compare

import (
	"reflect"
	"sort"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/export"
)

// ExcludedFields contains fields to exclude from comparison.
// These fields are typically auto-generated or organization-specific.
var ExcludedFields = map[string]bool{
	"_id":       true,
	"id":        true,
	"orgId":     true,
	"createdAt": true,
	"createdBy": true,
	"updatedAt": true,
	"updatedBy": true,
	"icon":      true,
	"color":     true,
}

// Differ computes differences between two organizations.
type Differ struct {
	excludedFields map[string]bool
}

// NewDiffer creates a new differ with default excluded fields.
func NewDiffer() *Differ {
	return &Differ{
		excludedFields: ExcludedFields,
	}
}

// Diff compares source and target org data, optionally filtered to specific resource types.
// An empty include slice means all resource types are compared.
func (d *Differ) Diff(source, target *export.Data, include []string) *CompareResult {
	result := &CompareResult{
		Identical: true,
	}

	// Compare each resource type, skipping those not in the include list
	if shouldInclude("blueprints", include) {
		result.Blueprints = d.diffBlueprints(source.Blueprints, target.Blueprints)
	}
	if shouldInclude("actions", include) || shouldInclude("automations", include) {
		result.Actions = d.diffActions(source.Actions, target.Actions)
	}
	if shouldInclude("scorecards", include) {
		result.Scorecards = d.diffScorecards(source.Scorecards, target.Scorecards)
	}
	if shouldInclude("pages", include) {
		result.Pages = d.diffPages(source.Pages, target.Pages)
	}
	if shouldInclude("integrations", include) {
		result.Integrations = d.diffIntegrations(source.Integrations, target.Integrations)
	}
	if shouldInclude("teams", include) {
		result.Teams = d.diffTeams(source.Teams, target.Teams)
	}
	if shouldInclude("users", include) {
		result.Users = d.diffUsers(source.Users, target.Users)
	}
	// Note: automations are fetched via the collector's "automations" keyword but merged
	// into Actions in export.Data; result.Automations remains a zero-value placeholder.

	if shouldInclude("blueprint-permissions", include) {
		result.BlueprintPermissions = d.diffPermissions(source.BlueprintPermissions, target.BlueprintPermissions)
	}
	if shouldInclude("action-permissions", include) {
		result.ActionPermissions = d.diffPermissions(source.ActionPermissions, target.ActionPermissions)
	}
	if shouldIncludeEntities(include) {
		result.Entities = d.diffEntities(source.Entities, target.Entities)
	}

	// Check if any differences exist
	result.Identical = d.isIdentical(result)

	return result
}

// shouldInclude reports whether a resource type should be compared.
// It returns true when the include list is empty (compare all) or contains the resource.
func shouldInclude(resource string, include []string) bool {
	if len(include) == 0 {
		return true
	}
	for _, r := range include {
		if r == resource {
			return true
		}
	}
	return false
}

// shouldIncludeEntities checks whether entities should be compared.
// Unlike other resource types, entities are never compared by default (opt-in only).
func shouldIncludeEntities(include []string) bool {
	for _, r := range include {
		if r == "entities" {
			return true
		}
	}
	return false
}

func (d *Differ) isIdentical(r *CompareResult) bool {
	checks := []DiffSummary{
		r.Blueprints.Summary, r.Actions.Summary, r.Scorecards.Summary,
		r.Pages.Summary, r.Integrations.Summary, r.Teams.Summary, r.Users.Summary,
		r.BlueprintPermissions.Summary, r.ActionPermissions.Summary, r.Entities.Summary,
	}
	for _, s := range checks {
		if s.Added > 0 || s.Modified > 0 || s.Removed > 0 {
			return false
		}
	}
	return true
}

func (d *Differ) diffBlueprints(source, target []api.Blueprint) ResourceDiff {
	return diffResources(toMaps(source), toMaps(target), "identifier")
}

func (d *Differ) diffActions(source, target []api.Action) ResourceDiff {
	return diffResources(toMaps(source), toMaps(target), "identifier")
}

func (d *Differ) diffScorecards(source, target []api.Scorecard) ResourceDiff {
	return diffResources(toMaps(source), toMaps(target), "identifier")
}

func (d *Differ) diffPages(source, target []api.Page) ResourceDiff {
	return diffResources(toMaps(source), toMaps(target), "identifier")
}

func (d *Differ) diffIntegrations(source, target []api.Integration) ResourceDiff {
	return diffResources(toMaps(source), toMaps(target), "installationId")
}

func (d *Differ) diffTeams(source, target []api.Team) ResourceDiff {
	return diffResources(toMaps(source), toMaps(target), "name")
}

func (d *Differ) diffUsers(source, target []api.User) ResourceDiff {
	return diffResources(toMaps(source), toMaps(target), "email")
}

func (d *Differ) diffEntities(source, target []api.Entity) ResourceDiff {
	toCompositeMap := func(items []api.Entity) []map[string]interface{} {
		result := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			m := map[string]interface{}(item)
			bpID, _ := m["blueprint"].(string)
			id, _ := m["identifier"].(string)
			if bpID == "" || id == "" {
				continue
			}
			entry := make(map[string]interface{}, len(m))
			for k, v := range m {
				entry[k] = v
			}
			entry["identifier"] = bpID + "/" + id
			result = append(result, entry)
		}
		return result
	}
	return diffResources(toCompositeMap(source), toCompositeMap(target), "identifier")
}

func (d *Differ) diffPermissions(source, target map[string]api.Permissions) ResourceDiff {
	toSlice := func(m map[string]api.Permissions) []map[string]interface{} {
		var result []map[string]interface{}
		for id, perms := range m {
			entry := make(map[string]interface{})
			for k, v := range perms {
				entry[k] = v
			}
			entry["identifier"] = id
			result = append(result, entry)
		}
		return result
	}
	return diffResources(toSlice(source), toSlice(target), "identifier")
}

// toMaps converts a slice of typed maps to []map[string]interface{}.
func toMaps[T ~map[string]interface{}](items []T) []map[string]interface{} {
	result := make([]map[string]interface{}, len(items))
	for i, item := range items {
		result[i] = map[string]interface{}(item)
	}
	return result
}

// diffResources compares two slices of resources by identifier.
func diffResources(source, target []map[string]interface{}, idField string) ResourceDiff {
	result := ResourceDiff{}

	// Build lookup maps
	sourceMap := make(map[string]map[string]interface{})
	targetMap := make(map[string]map[string]interface{})

	for _, item := range source {
		if id, ok := item[idField].(string); ok {
			sourceMap[id] = item
		}
	}
	for _, item := range target {
		if id, ok := item[idField].(string); ok {
			targetMap[id] = item
		}
	}

	// Find added (in target but not in source)
	for id, targetItem := range targetMap {
		if _, exists := sourceMap[id]; !exists {
			result.Added = append(result.Added, ResourceChange{
				Identifier: id,
				TargetData: targetItem,
			})
		}
	}

	// Find removed (in source but not in target)
	for id, sourceItem := range sourceMap {
		if _, exists := targetMap[id]; !exists {
			result.Removed = append(result.Removed, ResourceChange{
				Identifier: id,
				SourceData: sourceItem,
			})
		}
	}

	// Find modified (in both, but different)
	for id, sourceItem := range sourceMap {
		if targetItem, exists := targetMap[id]; exists {
			fieldDiffs := diffFields(sourceItem, targetItem, "")
			if len(fieldDiffs) > 0 {
				result.Modified = append(result.Modified, ResourceChange{
					Identifier: id,
					SourceData: sourceItem,
					TargetData: targetItem,
					FieldDiffs: fieldDiffs,
				})
			}
		}
	}

	// Sort for consistent output
	sort.Slice(result.Added, func(i, j int) bool { return result.Added[i].Identifier < result.Added[j].Identifier })
	sort.Slice(result.Removed, func(i, j int) bool { return result.Removed[i].Identifier < result.Removed[j].Identifier })
	sort.Slice(result.Modified, func(i, j int) bool { return result.Modified[i].Identifier < result.Modified[j].Identifier })

	result.Summary = DiffSummary{
		Added:    len(result.Added),
		Modified: len(result.Modified),
		Removed:  len(result.Removed),
	}

	return result
}

// diffFields recursively compares two maps and returns field differences.
func diffFields(source, target map[string]interface{}, prefix string) []FieldDiff {
	var diffs []FieldDiff

	// Collect all keys from both maps
	allKeys := make(map[string]bool)
	for k := range source {
		allKeys[k] = true
	}
	for k := range target {
		allKeys[k] = true
	}

	for key := range allKeys {
		// Skip excluded fields
		if ExcludedFields[key] {
			continue
		}

		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		sourceVal, sourceExists := source[key]
		targetVal, targetExists := target[key]

		if !sourceExists {
			diffs = append(diffs, FieldDiff{
				Path:        path,
				SourceValue: nil,
				TargetValue: targetVal,
			})
		} else if !targetExists {
			diffs = append(diffs, FieldDiff{
				Path:        path,
				SourceValue: sourceVal,
				TargetValue: nil,
			})
		} else if !reflect.DeepEqual(sourceVal, targetVal) {
			// Check if both are maps for recursive comparison
			sourceMap, sourceIsMap := sourceVal.(map[string]interface{})
			targetMap, targetIsMap := targetVal.(map[string]interface{})

			if sourceIsMap && targetIsMap {
				diffs = append(diffs, diffFields(sourceMap, targetMap, path)...)
			} else {
				diffs = append(diffs, FieldDiff{
					Path:        path,
					SourceValue: sourceVal,
					TargetValue: targetVal,
				})
			}
		}
	}

	// Sort for consistent output
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].Path < diffs[j].Path
	})

	return diffs
}

// FormatPath formats a diff path for display.
func FormatPath(path string) string {
	if len(path) > 0 && path[0] == '.' {
		return path[1:]
	}
	return path
}
