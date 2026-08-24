package import_module

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/modules/export"
	systemblueprints "github.com/port-labs/port-cli/internal/modules/system_blueprints"
)

// PermissionsChange represents a permissions update for a single resource.
type PermissionsChange struct {
	Identifier  string
	Permissions api.Permissions
}

// DiffResult represents the result of comparing import data with current state.
type DiffResult struct {
	BlueprintsToCreate   []api.Blueprint
	BlueprintsToUpdate   []api.Blueprint
	BlueprintsToSkip     []api.Blueprint
	EntitiesToCreate     []api.Entity
	EntitiesToUpdate     []api.Entity
	EntitiesToSkip       []api.Entity
	ScorecardsToCreate   []api.Scorecard
	ScorecardsToUpdate   []api.Scorecard
	ScorecardsToSkip     []api.Scorecard
	ActionsToCreate      []api.Action
	ActionsToUpdate      []api.Action
	ActionsToSkip        []api.Action
	TeamsToCreate        []api.Team
	TeamsToUpdate        []api.Team
	TeamsToSkip          []api.Team
	UsersToCreate        []api.User
	UsersToUpdate        []api.User
	UsersToSkip          []api.User
	PagesToCreate        []api.Page
	PagesToUpdate        []api.Page
	PagesToSkip          []api.Page
	IntegrationsToUpdate []api.Integration
	IntegrationsToSkip   []api.Integration
	BlueprintPermissions []PermissionsChange
	ActionPermissions    []PermissionsChange
	PagePermissions      []PermissionsChange
}

// DiffComparer compares import data with current organization state.
type DiffComparer struct {
	client *api.Client
}

// NewDiffComparer creates a new diff comparer.
func NewDiffComparer(client *api.Client) *DiffComparer {
	return &DiffComparer{
		client: client,
	}
}

// Compare compares import data with current organization state.
func (d *DiffComparer) Compare(ctx context.Context, importData *export.Data, opts Options) (*DiffResult, error) {
	// Export current state from target organization
	currentData, err := d.exportCurrentState(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to export current state: %w", err)
	}

	result := &DiffResult{}

	// Compare each resource type
	result.BlueprintsToCreate, result.BlueprintsToUpdate, result.BlueprintsToSkip = d.compareBlueprints(importData.Blueprints, currentData.Blueprints, opts.IncludeResources)
	result.EntitiesToCreate, result.EntitiesToUpdate, result.EntitiesToSkip = d.compareEntities(importData.Entities, currentData.Entities, opts.IncludeResources)
	result.ScorecardsToCreate, result.ScorecardsToUpdate, result.ScorecardsToSkip = d.compareScorecards(importData.Scorecards, currentData.Scorecards, opts.IncludeResources)
	result.ActionsToCreate, result.ActionsToUpdate, result.ActionsToSkip = d.compareActions(importData.Actions, currentData.Actions, opts.IncludeResources)
	result.TeamsToCreate, result.TeamsToUpdate, result.TeamsToSkip = d.compareTeams(importData.Teams, currentData.Teams, opts.IncludeResources)
	result.UsersToCreate, result.UsersToUpdate, result.UsersToSkip = d.compareUsers(importData.Users, currentData.Users, opts.IncludeResources)
	result.PagesToCreate, result.PagesToUpdate, result.PagesToSkip = d.comparePages(importData.Pages, currentData.Pages, opts.IncludeResources)
	result.IntegrationsToUpdate, result.IntegrationsToSkip = d.compareIntegrations(importData.Integrations, currentData.Integrations, opts.IncludeResources)

	// Compare permissions when included (or when no --include filter is set)
	if shouldImport("blueprint-permissions", opts.IncludeResources) {
		result.BlueprintPermissions = comparePermissions(currentData.BlueprintPermissions, importData.BlueprintPermissions)
	}
	if shouldImport("action-permissions", opts.IncludeResources) {
		result.ActionPermissions = comparePermissions(currentData.ActionPermissions, importData.ActionPermissions)
	}
	if shouldImport("page-permissions", opts.IncludeResources) {
		result.PagePermissions = comparePermissions(currentData.PagePermissions, importData.PagePermissions)
	}

	return result, nil
}

// exportCurrentState exports current state from target organization.
func (d *DiffComparer) exportCurrentState(ctx context.Context, opts Options) (*export.Data, error) {
	collector := export.NewCollector(d.client)
	exportOpts := export.Options{
		Blueprints:             nil, // Export all
		SkipEntities:           opts.SkipEntities,
		IncludeRuleResults:     opts.IncludeRuleResults,
		IncludeResources:       opts.IncludeResources,
		ExcludeBlueprints:      opts.ExcludeBlueprints,
		ExcludeBlueprintSchema: opts.ExcludeBlueprintSchema,
	}
	return collector.Collect(ctx, exportOpts)
}

// FilterData filters import data to only include resources that need to be created or updated.
func (d *DiffResult) FilterData(original *export.Data) *export.Data {
	return &export.Data{
		Blueprints:   append(d.BlueprintsToCreate, d.BlueprintsToUpdate...),
		Entities:     append(d.EntitiesToCreate, d.EntitiesToUpdate...),
		Scorecards:   append(d.ScorecardsToCreate, d.ScorecardsToUpdate...),
		Actions:      append(d.ActionsToCreate, d.ActionsToUpdate...),
		Teams:        append(d.TeamsToCreate, d.TeamsToUpdate...),
		Users:        append(d.UsersToCreate, d.UsersToUpdate...),
		Folders:      original.Folders,
		Pages:        append(d.PagesToCreate, d.PagesToUpdate...),
		Integrations: d.IntegrationsToUpdate,
	}
}

// normalizeResource normalizes a resource by removing system fields and ensuring consistent structure.
func normalizeResource(resource map[string]interface{}, systemFields []string) map[string]interface{} {
	normalized := make(map[string]interface{})
	removeSet := make(map[string]bool)
	for _, f := range systemFields {
		removeSet[f] = true
	}

	for k, v := range resource {
		if !removeSet[k] {
			normalized[k] = normalizeValue(v)
		}
	}

	return normalized
}

// normalizeValue recursively normalizes a value for comparison.
func normalizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		normalized := make(map[string]interface{})
		for k, v := range val {
			normalized[k] = normalizeValue(v)
		}
		return normalized
	case []interface{}:
		normalized := make([]interface{}, len(val))
		for i, item := range val {
			normalized[i] = normalizeValue(item)
		}
		// Sort slice only if ALL elements are strings
		if len(normalized) > 0 {
			allStrings := true
			for _, item := range normalized {
				if _, ok := item.(string); !ok {
					allStrings = false
					break
				}
			}
			if allStrings {
				sort.Slice(normalized, func(i, j int) bool {
					return normalized[i].(string) < normalized[j].(string)
				})
			}
		}
		return normalized
	default:
		return v
	}
}

// resourcesEqual checks if two resources are equal after normalization.
func resourcesEqual(a, b map[string]interface{}, systemFields []string) bool {
	normA := normalizeResource(a, systemFields)
	normB := normalizeResource(b, systemFields)

	// Use JSON marshaling for deep comparison
	jsonA, errA := json.Marshal(normA)
	jsonB, errB := json.Marshal(normB)

	if errA != nil || errB != nil {
		// Fallback to reflect.DeepEqual if JSON fails
		return reflect.DeepEqual(normA, normB)
	}

	return string(jsonA) == string(jsonB)
}

// portManagedBlueprints are blueprints that are fully managed by Port and cannot be modified.
// These are skipped during import to avoid "protected_blueprint_violation" errors.
var portManagedBlueprints = map[string]bool{
	"_rule": true, // Managed through scorecards, not directly editable
}

// compareBlueprints compares import blueprints with current blueprints.
func (d *DiffComparer) compareBlueprints(importBPs, currentBPs []api.Blueprint, includeResources []string) (create, update, skip []api.Blueprint) {
	if !shouldImport("blueprints", includeResources) {
		return nil, nil, nil
	}

	currentMap := make(map[string]api.Blueprint)
	for _, bp := range currentBPs {
		if identifier, ok := bp["identifier"].(string); ok {
			currentMap[identifier] = bp
		}
	}

	for _, bp := range importBPs {
		identifier, ok := bp["identifier"].(string)
		if !ok || identifier == "" {
			continue
		}

		isSystemPatch := systemblueprints.IsCustomPatch(bp)

		// Skip Port-managed blueprints that cannot be modified directly, unless
		// the input is a minimal custom-property patch.
		if portManagedBlueprints[identifier] && !isSystemPatch {
			skip = append(skip, bp)
			continue
		}

		// Note: Other system blueprints (starting with _) are included in diff comparison
		// so they can be updated with new properties. Creation of feature-flagged
		// system blueprints may fail, which is expected behavior.

		currentBP, exists := currentMap[identifier]
		if !exists {
			if isSystemPatch || systemblueprints.PrefersPatchUpdate(identifier) {
				update = append(update, bp)
				continue
			}
			create = append(create, bp)
		} else if isSystemPatch && !systemblueprints.CustomPatchEqual(bp, currentBP) {
			update = append(update, bp)
		} else if isSystemPatch {
			skip = append(skip, bp)
		} else if !resourcesEqual(bp, currentBP, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}) {
			update = append(update, bp)
		} else {
			skip = append(skip, bp)
		}
	}

	return create, update, skip
}

// compareEntities compares import entities with current entities.
func (d *DiffComparer) compareEntities(importEnts, currentEnts []api.Entity, includeResources []string) (create, update, skip []api.Entity) {
	if !shouldImport("entities", includeResources) {
		return nil, nil, nil
	}

	currentMap := make(map[string]api.Entity)
	for _, ent := range currentEnts {
		blueprintID, ok1 := ent["blueprint"].(string)
		entityID, ok2 := ent["identifier"].(string)
		if ok1 && ok2 {
			key := fmt.Sprintf("%s:%s", blueprintID, entityID)
			currentMap[key] = ent
		}
	}

	for _, ent := range importEnts {
		blueprintID, ok1 := ent["blueprint"].(string)
		entityID, ok2 := ent["identifier"].(string)
		if !ok1 || !ok2 || blueprintID == "" || entityID == "" {
			continue
		}

		key := fmt.Sprintf("%s:%s", blueprintID, entityID)
		currentEnt, exists := currentMap[key]
		if !exists {
			create = append(create, ent)
		} else if !resourcesEqual(ent, currentEnt, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}) {
			update = append(update, ent)
		} else {
			skip = append(skip, ent)
		}
	}

	return create, update, skip
}

// compareScorecards compares import scorecards with current scorecards.
func (d *DiffComparer) compareScorecards(importScs, currentScs []api.Scorecard, includeResources []string) (create, update, skip []api.Scorecard) {
	if !shouldImport("scorecards", includeResources) {
		return nil, nil, nil
	}

	currentMap := make(map[string]api.Scorecard)
	for _, sc := range currentScs {
		blueprintID, ok1 := sc["blueprintIdentifier"].(string)
		scorecardID, ok2 := sc["identifier"].(string)
		if ok1 && ok2 {
			key := fmt.Sprintf("%s:%s", blueprintID, scorecardID)
			currentMap[key] = sc
		}
	}

	for _, sc := range importScs {
		blueprintID, ok1 := sc["blueprintIdentifier"].(string)
		scorecardID, ok2 := sc["identifier"].(string)
		if !ok1 || !ok2 || blueprintID == "" || scorecardID == "" {
			continue
		}

		key := fmt.Sprintf("%s:%s", blueprintID, scorecardID)
		currentSc, exists := currentMap[key]
		if !exists {
			create = append(create, sc)
		} else if !resourcesEqual(sc, currentSc, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}) {
			update = append(update, sc)
		} else {
			skip = append(skip, sc)
		}
	}

	return create, update, skip
}

// compareActions compares import actions with current actions.
func (d *DiffComparer) compareActions(importActs, currentActs []api.Action, includeResources []string) (create, update, skip []api.Action) {
	if !shouldImport("actions", includeResources) && !shouldImport("automations", includeResources) {
		return nil, nil, nil
	}

	currentMap := make(map[string]api.Action)
	for _, act := range currentActs {
		if identifier, ok := act["identifier"].(string); ok {
			currentMap[identifier] = act
		}
	}

	for _, act := range importActs {
		identifier, ok := act["identifier"].(string)
		if !ok || identifier == "" {
			continue
		}

		currentAct, exists := currentMap[identifier]
		if !exists {
			create = append(create, act)
		} else if !resourcesEqual(act, currentAct, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}) {
			update = append(update, act)
		} else {
			skip = append(skip, act)
		}
	}

	return create, update, skip
}

// compareTeams compares import teams with current teams.
func (d *DiffComparer) compareTeams(importTeams, currentTeams []api.Team, includeResources []string) (create, update, skip []api.Team) {
	if !shouldImport("teams", includeResources) {
		return nil, nil, nil
	}

	currentMap := make(map[string]api.Team)
	for _, team := range currentTeams {
		if name, ok := team["name"].(string); ok {
			currentMap[name] = team
		}
	}

	for _, team := range importTeams {
		name, ok := team["name"].(string)
		if !ok || name == "" {
			continue
		}

		currentTeam, exists := currentMap[name]
		if !exists {
			create = append(create, team)
		} else if !resourcesEqual(team, currentTeam, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}) {
			update = append(update, team)
		} else {
			skip = append(skip, team)
		}
	}

	return create, update, skip
}

// compareUsers compares import users with current users.
func (d *DiffComparer) compareUsers(importUsers, currentUsers []api.User, includeResources []string) (create, update, skip []api.User) {
	if !shouldImport("users", includeResources) {
		return nil, nil, nil
	}

	currentMap := make(map[string]api.User)
	for _, user := range currentUsers {
		if email, ok := user["email"].(string); ok {
			currentMap[email] = user
		}
	}

	for _, user := range importUsers {
		email, ok := user["email"].(string)
		if !ok || email == "" {
			continue
		}

		currentUser, exists := currentMap[email]
		if !exists {
			create = append(create, user)
		} else if !resourcesEqual(user, currentUser, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}) {
			update = append(update, user)
		} else {
			skip = append(skip, user)
		}
	}

	return create, update, skip
}

// pagesEqual compares two pages for equality.
//
// Nav fields that are nil/null in the import page are excluded from comparison —
// we don't send null nav fields to Port (sending null clears existing values),
// so a null source nav field should not trigger an update.
//
// requiredQueryParams: null and [] are both treated as "empty" and excluded
// when the source value is empty, since we strip it before sending.
func pagesEqual(importPage, currentPage api.Page) bool {
	exclude := []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id", "protected"}

	for _, field := range pageNavFields {
		if field == "requiredQueryParams" {
			importVal := importPage[field]
			importEmpty := importVal == nil || (func() bool {
				s, ok := importVal.([]interface{})
				return ok && len(s) == 0
			}())
			if importEmpty {
				exclude = append(exclude, field)
			}
		} else if v, exists := importPage[field]; exists && v == nil {
			exclude = append(exclude, field)
		}
	}

	return resourcesEqual(map[string]interface{}(importPage), map[string]interface{}(currentPage), exclude)
}

// comparePages compares import pages with current pages.
func (d *DiffComparer) comparePages(importPages, currentPages []api.Page, includeResources []string) (create, update, skip []api.Page) {
	if !shouldImport("pages", includeResources) {
		return nil, nil, nil
	}

	currentMap := make(map[string]api.Page)
	for _, page := range currentPages {
		if identifier, ok := page["identifier"].(string); ok {
			currentMap[identifier] = page
		}
	}

	for _, page := range importPages {
		identifier, ok := page["identifier"].(string)
		if !ok || identifier == "" {
			continue
		}

		// Skip protected pages — these are Port system pages (e.g. $run) that are
		// org-specific and cannot be meaningfully migrated between organizations.
		if protected, _ := page["protected"].(bool); protected {
			skip = append(skip, page)
			continue
		}

		currentPage, exists := currentMap[identifier]
		if !exists {
			create = append(create, page)
		} else if !pagesEqual(page, currentPage) {
			update = append(update, page)
		} else {
			skip = append(skip, page)
		}
	}

	return create, update, skip
}

// compareIntegrations compares import integrations with current integrations.
func (d *DiffComparer) compareIntegrations(importInts, currentInts []api.Integration, includeResources []string) (update, skip []api.Integration) {
	if !shouldImport("integrations", includeResources) {
		return nil, nil
	}

	currentMap := make(map[string]api.Integration)
	for _, integ := range currentInts {
		if identifier, ok := integ["identifier"].(string); ok {
			currentMap[identifier] = integ
		}
	}

	for _, integ := range importInts {
		identifier, ok := integ["identifier"].(string)
		if !ok || identifier == "" {
			continue
		}

		currentInteg, exists := currentMap[identifier]
		if !exists {
			// Integration doesn't exist, skip (can't create integrations)
			continue
		} else if !resourcesEqual(integ, currentInteg, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}) {
			update = append(update, integ)
		} else {
			skip = append(skip, integ)
		}
	}

	return update, skip
}

// comparePermissions compares desired permissions against current permissions and
// returns a slice of changes for entries that are new or differ from current state.
func comparePermissions(current, desired map[string]api.Permissions) []PermissionsChange {
	var changes []PermissionsChange
	for id, desiredPerms := range desired {
		currentPerms, exists := current[id]
		if !exists || !resourcesEqual(
			map[string]interface{}(desiredPerms),
			map[string]interface{}(currentPerms),
			nil,
		) {
			changes = append(changes, PermissionsChange{Identifier: id, Permissions: desiredPerms})
		}
	}
	return changes
}
