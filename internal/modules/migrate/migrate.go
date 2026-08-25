package migrate

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/auth"
	"github.com/port-labs/port-cli/internal/config"
	entitystream "github.com/port-labs/port-cli/internal/modules/entity_stream"
	"github.com/port-labs/port-cli/internal/modules/export"
	"github.com/port-labs/port-cli/internal/modules/import_module"
	systemblueprints "github.com/port-labs/port-cli/internal/modules/system_blueprints"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// maxConcurrentBlueprints caps how many blueprints exportFromSource fetches
// scorecards/actions/permissions/entity-relevance for in parallel, to avoid
// firing one goroutine per blueprint (100+ simultaneous requests in large
// orgs) and exhausting the read-side rate limit before any response arrives.
// Mirrors export/collector.go's identical bound for the same reason.
const maxConcurrentBlueprints = 10

var blueprintSystemFields = []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}

func stripBlueprintSystemFields(bp api.Blueprint) api.Blueprint {
	cleaned := make(api.Blueprint, len(bp))
	for k, v := range bp {
		skip := false
		for _, f := range blueprintSystemFields {
			if k == f {
				skip = true
				break
			}
		}
		if !skip {
			cleaned[k] = v
		}
	}
	return cleaned
}

func blueprintUpdateMode(identifier string) import_module.BlueprintUpdateMode {
	if systemblueprints.PrefersPatchUpdate(identifier) {
		return import_module.BlueprintUpdatePATCH
	}
	return import_module.BlueprintUpdatePUT
}

// Module handles migration between Port organizations.
type Module struct {
	sourceClient *api.Client
	targetClient *api.Client
}

// NewModule creates a new migration module.
func NewModule(sourceToken, targetToken *auth.Token, sourceConfig, targetConfig *config.OrganizationConfig) *Module {
	return &Module{
		sourceClient: api.NewClient(api.ClientOpts{
			Token:        sourceToken,
			ClientID:     sourceConfig.ClientID,
			ClientSecret: sourceConfig.ClientSecret,
			APIURL:       sourceConfig.APIURL,
			Timeout:      0,
		}),
		targetClient: api.NewClient(api.ClientOpts{
			Token:        targetToken,
			ClientID:     targetConfig.ClientID,
			ClientSecret: targetConfig.ClientSecret,
			APIURL:       targetConfig.APIURL,
			Timeout:      0,
		}),
	}
}

// Options represents migration options.
type Options struct {
	Blueprints                    []string
	DryRun                        bool
	SkipEntities                  bool
	SkipSystemBlueprints          bool // skip _* blueprint schemas and their entities
	SkipSystemBlueprintProperties bool
	IncludeRuleResults            bool // include _rule_result system blueprint entities (included by default)
	IncludeResources              []string
	ExcludeBlueprints             []string // deep: exclude blueprint schema + all its resources
	ExcludeBlueprintSchema        []string // shallow: exclude only the blueprint schema, keep resources
	UsersAsDisabled               bool     // import non-admin users as DISABLED after staging
	ErrorHandling                 import_module.ErrorHandlingOptions

	// AutoScopeBlueprints, when true, narrows the blueprint schemas returned by
	// exportFromSource to only the blueprints referenced by a matching
	// scorecard, action, or entity (see FilterBlueprintsToReferenced and
	// blueprintHasMatchingEntity). It is false whenever the caller explicitly
	// requested blueprints via --blueprints or --include blueprints.
	AutoScopeBlueprints bool

	// Per-resource ID filters (client-side, applied after bulk fetch)
	Entities     []string
	Scorecards   []string
	Actions      []string
	Pages        []string
	Integrations []string
	Teams        []string
	Users        []string
}

// Result represents the result of a migration operation.
type Result struct {
	Success                              bool
	Message                              string
	BlueprintsCreated                    int
	BlueprintsUpdated                    int
	BlueprintsSkipped                    int
	EntitiesCreated                      int
	EntitiesUpdated                      int
	EntitiesSkipped                      int
	ScorecardsCreated                    int
	ScorecardsUpdated                    int
	ScorecardsSkipped                    int
	ActionsCreated                       int
	ActionsUpdated                       int
	ActionsSkipped                       int
	TeamsCreated                         int
	TeamsUpdated                         int
	TeamsSkipped                         int
	UsersCreated                         int
	UsersUpdated                         int
	UsersSkipped                         int
	PagesCreated                         int
	PagesUpdated                         int
	PagesSkipped                         int
	IntegrationsUpdated                  int
	IntegrationsSkipped                  int
	BlueprintPermissionsUpdated          int
	ActionPermissionsUpdated             int
	PagePermissionsUpdated               int
	BlueprintsToCreate                   []string
	BlueprintsToUpdate                   []string
	BlueprintsToSkip                     []string
	BlueprintPermissionsToUpdate         []string
	ActionPermissionsToUpdate            []string
	PagePermissionsToUpdate              []string
	Errors                               []string
	Warnings                             []string
	DiffResult                           *import_module.DiffResult
	IgnoredRuleResultTargetRelationCount int
	IgnoredRuleResultTargetRelationKeys  []string
}

// Execute performs the migration operation.
func (m *Module) Execute(ctx context.Context, opts Options) (*Result, error) {
	// Export from source
	sourceData, entityBlueprints, cachedMatchedEntities, err := m.exportFromSource(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to export from source: %w", err)
	}
	streamEntities := !opts.SkipEntities && shouldCollect("entities", opts.IncludeResources)

	// Diff validation - compare source data with target organization's current state
	comparer := import_module.NewDiffComparer(m.targetClient)
	diffOpts := import_module.Options{
		SkipEntities:                  opts.SkipEntities || streamEntities,
		SkipSystemBlueprints:          opts.SkipSystemBlueprints,
		SkipSystemBlueprintProperties: opts.SkipSystemBlueprintProperties,
		IncludeRuleResults:            opts.IncludeRuleResults,
		IncludeResources:              opts.IncludeResources,
		ExcludeBlueprints:             opts.ExcludeBlueprints,
		ExcludeBlueprintSchema:        opts.ExcludeBlueprintSchema,
	}
	diffResult, err := comparer.Compare(ctx, sourceData, diffOpts)
	if err != nil {
		return nil, fmt.Errorf("diff comparison failed: %w", err)
	}

	// Use diff result to filter data - only migrate what needs to be created or updated
	filteredData := diffResult.FilterData(sourceData)

	// Dry run - show what would happen
	if opts.DryRun {
		result := m.generateDryRunResult(diffResult)
		if streamEntities {
			if err := m.migrateEntities(ctx, entityBlueprints, opts, result, true, cachedMatchedEntities); err != nil {
				markMigrationStopped(result, diffResult, err)
				return result, fmt.Errorf("streaming entity dry run failed: %w", err)
			}
		}
		return result, nil
	}

	// Import to target using filtered data
	result, err := m.importToTarget(ctx, filteredData, diffResult, opts.UsersAsDisabled, opts.ErrorHandling)
	if err != nil {
		return nil, fmt.Errorf("failed to import to target: %w", err)
	}
	if streamEntities {
		if err := m.migrateEntities(ctx, entityBlueprints, opts, result, false, cachedMatchedEntities); err != nil {
			markMigrationStopped(result, diffResult, err)
			return result, fmt.Errorf("failed to migrate entities: %w", err)
		}
	}

	if len(result.Errors) > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("Migration completed with %d error(s)", len(result.Errors))
	} else {
		result.Success = true
		result.Message = "Migration completed successfully"
	}
	result.DiffResult = diffResult
	return result, nil
}

func markMigrationStopped(result *Result, diffResult *import_module.DiffResult, err error) {
	if result == nil {
		return
	}
	if len(result.Errors) == 0 && err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	result.Success = false
	result.Message = fmt.Sprintf("Migration stopped with %d error(s)", len(result.Errors))
	result.DiffResult = diffResult
}

func migrateHandledErrorWarningCallback(result *Result, existing func(string)) func(string) {
	var mu sync.Mutex
	return func(message string) {
		mu.Lock()
		defer mu.Unlock()
		result.Warnings = append(result.Warnings, message)
		if existing != nil {
			existing(message)
		}
	}
}

// generateDryRunResult generates a dry run result with accurate predictions.
func (m *Module) generateDryRunResult(diffResult *import_module.DiffResult) *Result {
	return &Result{
		Success:                      true,
		Message:                      "Migration validation passed (dry run - no changes applied)",
		BlueprintsCreated:            len(diffResult.BlueprintsToCreate),
		BlueprintsUpdated:            len(diffResult.BlueprintsToUpdate),
		BlueprintsSkipped:            len(diffResult.BlueprintsToSkip),
		EntitiesCreated:              len(diffResult.EntitiesToCreate),
		EntitiesUpdated:              len(diffResult.EntitiesToUpdate),
		EntitiesSkipped:              len(diffResult.EntitiesToSkip),
		ScorecardsCreated:            len(diffResult.ScorecardsToCreate),
		ScorecardsUpdated:            len(diffResult.ScorecardsToUpdate),
		ScorecardsSkipped:            len(diffResult.ScorecardsToSkip),
		ActionsCreated:               len(diffResult.ActionsToCreate),
		ActionsUpdated:               len(diffResult.ActionsToUpdate),
		ActionsSkipped:               len(diffResult.ActionsToSkip),
		TeamsCreated:                 len(diffResult.TeamsToCreate),
		TeamsUpdated:                 len(diffResult.TeamsToUpdate),
		TeamsSkipped:                 len(diffResult.TeamsToSkip),
		UsersCreated:                 len(diffResult.UsersToCreate),
		UsersUpdated:                 len(diffResult.UsersToUpdate),
		UsersSkipped:                 len(diffResult.UsersToSkip),
		PagesCreated:                 len(diffResult.PagesToCreate),
		PagesUpdated:                 len(diffResult.PagesToUpdate),
		PagesSkipped:                 len(diffResult.PagesToSkip),
		IntegrationsUpdated:          len(diffResult.IntegrationsToUpdate),
		IntegrationsSkipped:          len(diffResult.IntegrationsToSkip),
		BlueprintPermissionsUpdated:  len(diffResult.BlueprintPermissions),
		ActionPermissionsUpdated:     len(diffResult.ActionPermissions),
		PagePermissionsUpdated:       len(diffResult.PagePermissions),
		BlueprintsToCreate:           blueprintIdentifiers(diffResult.BlueprintsToCreate),
		BlueprintsToUpdate:           blueprintIdentifiers(diffResult.BlueprintsToUpdate),
		BlueprintsToSkip:             blueprintIdentifiers(diffResult.BlueprintsToSkip),
		BlueprintPermissionsToUpdate: permissionsChangeIdentifiers(diffResult.BlueprintPermissions),
		ActionPermissionsToUpdate:    permissionsChangeIdentifiers(diffResult.ActionPermissions),
		PagePermissionsToUpdate:      permissionsChangeIdentifiers(diffResult.PagePermissions),
		DiffResult:                   diffResult,
	}
}

// shouldCollect checks if a resource type should be collected.
func shouldCollect(resourceType string, includeResources []string) bool {
	if len(includeResources) == 0 {
		return true
	}

	for _, r := range includeResources {
		if r == resourceType {
			return true
		}
	}
	return false
}

// exportFromSource exports metadata from the source organization and returns
// the blueprints eligible for streaming entity migration, plus any entities
// already fetched for those blueprints while deciding AutoScopeBlueprints
// relevance (see blueprintHasMatchingEntity) — migrateEntities reuses these
// instead of re-fetching.
func (m *Module) exportFromSource(ctx context.Context, opts Options) (*export.Data, []api.Blueprint, map[string][]api.Entity, error) {
	// Collect blueprints first
	allBlueprints, err := m.sourceClient.GetBlueprints(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get blueprints: %w", err)
	}

	// Filter blueprints if specified
	var selectedBlueprints []api.Blueprint
	if len(opts.Blueprints) > 0 {
		blueprintSet := make(map[string]bool)
		for _, bpID := range opts.Blueprints {
			blueprintSet[bpID] = true
		}

		for _, bp := range allBlueprints {
			if identifier, ok := bp["identifier"].(string); ok && blueprintSet[identifier] {
				selectedBlueprints = append(selectedBlueprints, bp)
			}
		}
	} else {
		selectedBlueprints = allBlueprints
	}

	// Resolve dependencies
	resolvedBlueprints := m.resolveDependencies(allBlueprints, selectedBlueprints)

	// Apply exclusions: iterBlueprints is used to fetch entities/scorecards/actions,
	// dataBlueprints is what ends up in data.Blueprints (schema output).
	excludeDeep := opts.ExcludeBlueprints
	if !opts.IncludeRuleResults {
		excludeDeep = append(excludeDeep, "_rule_result")
	}
	iterBlueprints, dataBlueprints := systemblueprints.ApplyExclusions(
		resolvedBlueprints,
		excludeDeep,
		opts.ExcludeBlueprintSchema,
		opts.SkipSystemBlueprints,
		opts.SkipSystemBlueprintProperties,
	)
	if !shouldCollect("blueprints", opts.IncludeResources) {
		dataBlueprints = []api.Blueprint{}
	}
	// scopeBlueprintsToReferenced narrows dataBlueprints, once collection below
	// completes, to only the blueprints that produced a matching
	// scorecard/action/entity — see Options.AutoScopeBlueprints doc comment.
	scopeBlueprintsToReferenced := opts.AutoScopeBlueprints && shouldCollect("blueprints", opts.IncludeResources)
	entityBlueprints := make([]api.Blueprint, 0, len(iterBlueprints))
	if !opts.SkipEntities && shouldCollect("entities", opts.IncludeResources) {
		for _, blueprint := range iterBlueprints {
			bpID, _ := blueprint["identifier"].(string)
			if bpID == "" {
				continue
			}
			if opts.SkipSystemBlueprints && strings.HasPrefix(bpID, "_") {
				continue
			}
			entityBlueprints = append(entityBlueprints, blueprint)
		}
	}

	data := &export.Data{
		Blueprints:           dataBlueprints,
		Entities:             []api.Entity{},
		Scorecards:           []api.Scorecard{},
		Actions:              []api.Action{},
		Teams:                []api.Team{},
		Users:                []api.User{},
		Folders:              []api.Folder{},
		Pages:                []api.Page{},
		Integrations:         []api.Integration{},
		BlueprintPermissions: make(map[string]api.Permissions),
		ActionPermissions:    make(map[string]api.Permissions),
		PagePermissions:      make(map[string]api.Permissions),
	}

	// Use errgroup for concurrent collection, bounded by semaphore (see
	// maxConcurrentBlueprints doc comment).
	g, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(maxConcurrentBlueprints)
	var mu sync.Mutex
	referencedBlueprintIDs := make(map[string]bool)

	// Collect scorecards and actions concurrently per blueprint. Entities are
	// migrated later with a bounded pull/push loop per blueprint.
	for _, blueprint := range iterBlueprints {
		bp := blueprint
		bpID, ok := bp["identifier"].(string)
		if !ok {
			continue
		}

		// Collect scorecards
		if shouldCollect("scorecards", opts.IncludeResources) {
			if err := sem.Acquire(ctx, 1); err != nil {
				return nil, nil, nil, err
			}
			g.Go(func() error {
				defer sem.Release(1)
				scorecards, err := m.sourceClient.GetScorecards(ctx, bpID)
				if err != nil {
					if !strings.Contains(err.Error(), "410 Gone") {
						return fmt.Errorf("failed to get scorecards for blueprint %s: %w", bpID, err)
					}
					return nil
				}

				// Ensure scorecards have blueprintIdentifier field
				for i := range scorecards {
					if _, exists := scorecards[i]["blueprintIdentifier"]; !exists {
						scorecards[i]["blueprintIdentifier"] = bpID
					}
				}

				scorecards = export.FilterByField(scorecards, opts.Scorecards, "identifier")
				mu.Lock()
				data.Scorecards = append(data.Scorecards, scorecards...)
				if scopeBlueprintsToReferenced && len(scorecards) > 0 {
					referencedBlueprintIDs[bpID] = true
				}
				mu.Unlock()
				return nil
			})
		}

		// Collect blueprint permissions
		if shouldCollect("blueprint-permissions", opts.IncludeResources) || len(opts.IncludeResources) == 0 {
			bpIDCopy := bpID
			if err := sem.Acquire(ctx, 1); err != nil {
				return nil, nil, nil, err
			}
			g.Go(func() error {
				defer sem.Release(1)
				perms, err := m.sourceClient.GetBlueprintPermissions(ctx, bpIDCopy)
				if err != nil {
					mu.Lock()
					data.Warnings = append(data.Warnings, fmt.Sprintf("failed to fetch permissions for blueprint %s: %v", bpIDCopy, err))
					mu.Unlock()
					return nil
				}
				mu.Lock()
				data.BlueprintPermissions[bpIDCopy] = perms
				mu.Unlock()
				return nil
			})
		}
	}

	// cachedMatchedEntities holds, per blueprint, the entities found by the
	// relevance pre-scan below when opts.Entities is set. migrateEntities
	// reuses these instead of re-fetching from the source for the same
	// blueprint — see blueprintHasMatchingEntity's doc comment.
	cachedMatchedEntities := make(map[string][]api.Entity)

	// When AutoScopeBlueprints narrowing is active, check each entity-eligible
	// blueprint for at least one matching entity now, so blueprints needed only
	// for --entities are known before the diff/import phase runs — entities
	// themselves are migrated later, by migrateEntities, which runs after
	// blueprint schemas have already been diffed and imported to the target.
	if scopeBlueprintsToReferenced && !opts.SkipEntities && shouldCollect("entities", opts.IncludeResources) {
		for _, blueprint := range entityBlueprints {
			bpID, _ := blueprint["identifier"].(string)
			if bpID == "" {
				continue
			}
			bpIDCopy := bpID
			if err := sem.Acquire(ctx, 1); err != nil {
				return nil, nil, nil, err
			}
			g.Go(func() error {
				defer sem.Release(1)
				found, matched, err := m.blueprintHasMatchingEntity(ctx, bpIDCopy, opts.Entities)
				if err != nil {
					return fmt.Errorf("failed to check entities for blueprint %s: %w", bpIDCopy, err)
				}
				if found {
					mu.Lock()
					referencedBlueprintIDs[bpIDCopy] = true
					if len(matched) > 0 {
						cachedMatchedEntities[bpIDCopy] = matched
					}
					mu.Unlock()
				}
				return nil
			})
		}
	}

	// Collect organization-wide resources
	if !opts.SkipEntities && shouldCollect("teams", opts.IncludeResources) {
		g.Go(func() error {
			teams, err := m.sourceClient.GetTeams(ctx)
			if err != nil {
				return nil // Non-fatal
			}

			teams = export.FilterByField(teams, opts.Teams, "name")
			mu.Lock()
			data.Teams = teams
			mu.Unlock()
			return nil
		})
	}

	if !opts.SkipEntities && shouldCollect("users", opts.IncludeResources) {
		g.Go(func() error {
			users, err := m.sourceClient.GetUsers(ctx)
			if err != nil {
				return nil // Non-fatal
			}

			users = export.FilterByField(users, opts.Users, "email")
			mu.Lock()
			data.Users = users
			mu.Unlock()
			return nil
		})
	}

	// Collect actions and automations from the organization-wide /actions endpoint.
	if shouldCollect("actions", opts.IncludeResources) || shouldCollect("automations", opts.IncludeResources) {
		g.Go(func() error {
			allActions, err := m.sourceClient.GetAllActions(ctx)
			if err != nil {
				return nil // Non-fatal
			}

			selectedActions := export.SelectActionsForResources(
				allActions,
				shouldCollect("actions", opts.IncludeResources),
				shouldCollect("automations", opts.IncludeResources),
				opts.Actions,
			)
			mu.Lock()
			data.Actions = append(data.Actions, selectedActions...)
			if scopeBlueprintsToReferenced {
				for _, action := range selectedActions {
					if bpID := api.ActionBlueprintID(action); bpID != "" {
						referencedBlueprintIDs[bpID] = true
					}
				}
			}
			mu.Unlock()

			// Fetch permissions for each selected action/automation
			if shouldCollect("action-permissions", opts.IncludeResources) || len(opts.IncludeResources) == 0 {
				for _, action := range selectedActions {
					actionID, ok := action["identifier"].(string)
					if !ok {
						continue
					}
					aID := actionID
					g.Go(func() error {
						perms, err := m.sourceClient.GetActionPermissions(ctx, aID)
						if err != nil {
							mu.Lock()
							data.Warnings = append(data.Warnings, fmt.Sprintf("failed to fetch permissions for action %s: %v", aID, err))
							mu.Unlock()
							return nil
						}
						mu.Lock()
						data.ActionPermissions[aID] = perms
						mu.Unlock()
						return nil
					})
				}
			}
			return nil
		})
	}

	if shouldCollect("pages", opts.IncludeResources) {
		g.Go(func() error {
			folders, err := m.sourceClient.GetFolders(ctx)
			if err != nil {
				return nil // Non-fatal
			}
			pages, err := m.sourceClient.GetPages(ctx)
			if err != nil {
				return nil // Non-fatal
			}

			pages = export.FilterByField(pages, opts.Pages, "identifier")
			if len(opts.Pages) > 0 {
				folders = export.FilterFoldersToAncestors(folders, pages)
			}

			mu.Lock()
			data.Folders = folders
			data.Pages = pages
			mu.Unlock()

			// Fetch permissions for each page
			if shouldCollect("page-permissions", opts.IncludeResources) || len(opts.IncludeResources) == 0 {
				for _, page := range pages {
					pageID, ok := page["identifier"].(string)
					if !ok || pageID == "" {
						continue
					}
					pID := pageID
					g.Go(func() error {
						perms, err := m.sourceClient.GetPagePermissions(ctx, pID)
						if err != nil {
							mu.Lock()
							data.Warnings = append(data.Warnings, fmt.Sprintf("failed to fetch permissions for page %s: %v", pID, err))
							mu.Unlock()
							return nil
						}
						mu.Lock()
						data.PagePermissions[pID] = perms
						mu.Unlock()
						return nil
					})
				}
			}
			return nil
		})
	}

	if shouldCollect("integrations", opts.IncludeResources) {
		g.Go(func() error {
			integrations, err := m.sourceClient.GetIntegrations(ctx)
			if err != nil {
				return nil // Non-fatal
			}

			integrations = export.FilterByField(integrations, opts.Integrations, "installationId")
			mu.Lock()
			data.Integrations = integrations
			mu.Unlock()
			return nil
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return nil, nil, nil, err
	}

	if scopeBlueprintsToReferenced {
		// Both the schema list and the entity-streaming candidate list narrow
		// to exactly what was referenced (scorecard/action/entity match) — no
		// relation targets are pulled in here. A referenced blueprint's
		// relation target that isn't itself referenced doesn't need its
		// schema included in this migration at all; importToTarget's relation
		// validation checks the target's actual state directly instead (see
		// existingInTarget below), so an already-existing target blueprint is
		// correctly recognized without being part of this run's diff.
		data.Blueprints = export.FilterBlueprintsToReferenced(dataBlueprints, referencedBlueprintIDs)
		entityBlueprints = export.FilterBlueprintsToReferenced(entityBlueprints, referencedBlueprintIDs)
	}

	return data, entityBlueprints, cachedMatchedEntities, nil
}

// blueprintHasMatchingEntity checks whether bpID has at least one entity
// matching entityIDs (or any entity at all, when entityIDs is empty).
//
// When entityIDs is empty, this is answered with a single entities-count API
// call — no entity payload is fetched, so there's nothing to reuse later and
// nothing wasted.
//
// When entityIDs is non-empty, it must page through bpID's entities to find
// every match (not just the first — migrateEntities needs the complete
// matching set later, so stopping early would lose entries). The matches
// found are returned so the caller can cache them: migrateEntities filters
// to this same entityIDs set anyway, so a matched blueprint's entities never
// need to be fetched from the source a second time. This is safe to cache in
// full because it's bounded by len(entityIDs) (a small, caller-provided
// list), not by blueprint size — unlike the "any entity" case, which this
// function never materializes at all.
func (m *Module) blueprintHasMatchingEntity(ctx context.Context, bpID string, entityIDs []string) (bool, []api.Entity, error) {
	if len(entityIDs) == 0 {
		count, err := m.sourceClient.GetEntitiesCount(ctx, bpID)
		if err != nil {
			if strings.Contains(err.Error(), "410 Gone") {
				return false, nil, nil
			}
			return false, nil, err
		}
		return count > 0, nil, nil
	}

	entitySet := make(map[string]bool, len(entityIDs))
	for _, id := range entityIDs {
		entitySet[id] = true
	}
	var matched []api.Entity
	err := m.sourceClient.ForEachEntity(ctx, bpID, func(batch []api.Entity) error {
		for _, entity := range batch {
			id, _ := entity["identifier"].(string)
			if entitySet[id] {
				matched = append(matched, entity)
			}
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "410 Gone") {
			return false, nil, nil
		}
		return false, nil, err
	}
	if len(matched) == 0 {
		return false, nil, nil
	}
	return true, matched, nil
}

// resolveDependencies resolves blueprint dependencies.
// If a blueprint has relations to other blueprints, ensure those blueprints are also included.
func (m *Module) resolveDependencies(allBlueprints, selectedBlueprints []api.Blueprint) []api.Blueprint {
	selectedIDs := make(map[string]bool)
	allBlueprintsMap := make(map[string]api.Blueprint)

	for _, bp := range allBlueprints {
		if identifier, ok := bp["identifier"].(string); ok {
			allBlueprintsMap[identifier] = bp
		}
	}

	for _, bp := range selectedBlueprints {
		if identifier, ok := bp["identifier"].(string); ok {
			selectedIDs[identifier] = true
		}
	}

	result := make([]api.Blueprint, len(selectedBlueprints))
	copy(result, selectedBlueprints)

	toCheck := make([]string, 0, len(selectedIDs))
	for id := range selectedIDs {
		toCheck = append(toCheck, id)
	}

	checked := make(map[string]bool)

	for len(toCheck) > 0 {
		blueprintID := toCheck[len(toCheck)-1]
		toCheck = toCheck[:len(toCheck)-1]

		if checked[blueprintID] {
			continue
		}
		checked[blueprintID] = true

		blueprint, ok := allBlueprintsMap[blueprintID]
		if !ok {
			continue
		}

		// Check relations
		relations, ok := blueprint["relations"].(map[string]interface{})
		if !ok {
			continue
		}

		for _, relation := range relations {
			relationMap, ok := relation.(map[string]interface{})
			if !ok {
				continue
			}

			target, ok := relationMap["target"].(string)
			if !ok || target == "" {
				continue
			}

			if !selectedIDs[target] {
				// Add dependency
				if depBlueprint, exists := allBlueprintsMap[target]; exists {
					result = append(result, depBlueprint)
					selectedIDs[target] = true
					toCheck = append(toCheck, target)
				}
			}
		}
	}

	return result
}

// importToTarget imports data to the target organization using diff result.
func (m *Module) importToTarget(ctx context.Context, data *export.Data, diffResult *import_module.DiffResult, usersAsDisabled bool, errorHandlingOpts ...import_module.ErrorHandlingOptions) (*Result, error) {
	result := &Result{
		Errors: []string{},
	}
	var errorHandling import_module.ErrorHandlingOptions
	if len(errorHandlingOpts) > 0 {
		errorHandling = errorHandlingOpts[0]
	}
	errorHandling.AddWarning = migrateHandledErrorWarningCallback(result, errorHandling.AddWarning)
	updater := import_module.NewBlueprintUpdater(m.targetClient, errorHandling)

	// origCtx is used to create fresh errgroups. After errgroup.Wait() returns, the
	// derived context is canceled, so we must always derive from the original context
	// rather than re-using the shadowed variable across passes.
	origCtx := ctx

	// Create maps to quickly check if items should be created or updated
	blueprintsToCreate := make(map[string]bool)
	blueprintsToUpdate := make(map[string]bool)
	for _, bp := range diffResult.BlueprintsToCreate {
		if id, ok := bp["identifier"].(string); ok {
			blueprintsToCreate[id] = true
		}
	}
	for _, bp := range diffResult.BlueprintsToUpdate {
		if id, ok := bp["identifier"].(string); ok {
			blueprintsToUpdate[id] = true
		}
	}

	entitiesToCreate := make(map[string]bool)
	entitiesToUpdate := make(map[string]bool)
	for _, ent := range diffResult.EntitiesToCreate {
		bpID, ok1 := ent["blueprint"].(string)
		entID, ok2 := ent["identifier"].(string)
		if ok1 && ok2 {
			entitiesToCreate[fmt.Sprintf("%s:%s", bpID, entID)] = true
		}
	}
	for _, ent := range diffResult.EntitiesToUpdate {
		bpID, ok1 := ent["blueprint"].(string)
		entID, ok2 := ent["identifier"].(string)
		if ok1 && ok2 {
			entitiesToUpdate[fmt.Sprintf("%s:%s", bpID, entID)] = true
		}
	}

	// Import blueprints first (needed for other resources) using two-pass strategy
	g, ctx := errgroup.WithContext(origCtx)
	var mu sync.Mutex

	// Store each field type separately for ordered phase updates.
	// Ordering mirrors import.go: relations → calcProps → mirrorProps → aggProps.
	// This is required because:
	//   - mirrorProperties paths may traverse relations on OTHER blueprints
	//   - aggregationProperties reference properties (calcProps/aggProps) on OTHER blueprints
	// Running them as a single concurrent batch causes race conditions.
	blueprintRelations := make(map[string]map[string]interface{})
	blueprintCalcProps := make(map[string]map[string]interface{})
	blueprintMirrorProps := make(map[string]map[string]interface{})
	blueprintAggProps := make(map[string]map[string]interface{})
	blueprintOwnership := make(map[string]interface{})
	strippedBlueprints := make([]api.Blueprint, 0, len(data.Blueprints))
	blueprintActions := make(map[string]string) // "create" or "update"

	for _, blueprint := range data.Blueprints {
		identifier, ok := blueprint["identifier"].(string)
		if !ok || identifier == "" {
			continue
		}

		// Only process blueprints that need to be created or updated
		shouldProcess := blueprintsToCreate[identifier] || blueprintsToUpdate[identifier]
		if !shouldProcess {
			continue
		}

		// Extract and store each field type separately
		if relations := import_module.ExtractRelations(blueprint); len(relations) > 0 {
			rels, ignored := systemblueprints.FilterManagedRelations(identifier, relations)
			if len(ignored) > 0 {
				result.IgnoredRuleResultTargetRelationCount += len(ignored)
				result.IgnoredRuleResultTargetRelationKeys = append(result.IgnoredRuleResultTargetRelationKeys, ignored...)
			}
			if len(rels) > 0 {
				blueprintRelations[identifier] = rels
			}
		}
		if v, ok := blueprint["calculationProperties"].(map[string]interface{}); ok && len(v) > 0 {
			blueprintCalcProps[identifier] = v
		}
		if v, ok := blueprint["mirrorProperties"].(map[string]interface{}); ok && len(v) > 0 {
			blueprintMirrorProps[identifier] = v
		}
		if v, ok := blueprint["aggregationProperties"].(map[string]interface{}); ok && len(v) > 0 {
			blueprintAggProps[identifier] = v
		}
		if v, ok := blueprint["ownership"]; ok && v != nil {
			blueprintOwnership[identifier] = v
		}

		// Strip relations and all dependent fields for first pass
		strippedBp := import_module.StripRelations(blueprint)
		strippedBp = import_module.StripDependentFields(strippedBp)
		strippedBlueprints = append(strippedBlueprints, strippedBp)

		// Track what action to take
		if blueprintsToCreate[identifier] {
			blueprintActions[identifier] = "create"
		} else {
			blueprintActions[identifier] = "update"
		}
	}

	// First pass: Import blueprints without relations
	failedBlueprints := make(map[string]api.Blueprint)
	failedBlueprintActions := make(map[string]string)
	successfulBlueprints := make(map[string]bool)

	for _, blueprint := range strippedBlueprints {
		bp := blueprint
		g.Go(func() error {
			identifier, ok := bp["identifier"].(string)
			if !ok || identifier == "" {
				return nil
			}

			apiBp := api.Blueprint(bp)
			action := blueprintActions[identifier]

			//nolint:staticcheck
			if action == "create" {
				_, err := m.targetClient.CreateBlueprint(ctx, apiBp)
				if err != nil {
					mu.Lock()
					// Check if it's a relation error - if so, we'll retry in second pass
					if import_module.IsRelationError(err) {
						failedBlueprints[identifier] = bp
						failedBlueprintActions[identifier] = action
					} else {
						result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s: %v", identifier, err))
					}
					mu.Unlock()
					return nil
				}
				mu.Lock()
				result.BlueprintsCreated++
				successfulBlueprints[identifier] = true
				mu.Unlock()
			} else if action == "update" {
				var err error
				if systemblueprints.PrefersPatchUpdate(identifier) {
					err = updater.Update(ctx, identifier, apiBp, blueprintUpdateMode(identifier))
				} else {
					existing, fetchErr := m.targetClient.GetBlueprint(ctx, identifier)
					if fetchErr != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s: %v", identifier, fetchErr))
						mu.Unlock()
						return nil
					}
					for k, v := range apiBp {
						existing[k] = v
					}
					err = updater.Update(ctx, identifier, stripBlueprintSystemFields(existing), blueprintUpdateMode(identifier))
				}
				if err != nil {
					mu.Lock()
					if import_module.IsRelationError(err) {
						failedBlueprints[identifier] = bp
						failedBlueprintActions[identifier] = action
					} else {
						result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s: %v", identifier, err))
					}
					mu.Unlock()
					return nil
				}
				mu.Lock()
				result.BlueprintsUpdated++
				successfulBlueprints[identifier] = true
				mu.Unlock()
			}
			return nil
		})
	}

	// Wait for first pass to complete
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Retry failed blueprints (they might have succeeded now that dependencies exist)
	if len(failedBlueprints) > 0 {
		g, ctx = errgroup.WithContext(origCtx)
		for identifier, bp := range failedBlueprints {
			bpID := identifier
			bpCopy := bp
			action := failedBlueprintActions[bpID]
			g.Go(func() error {
				apiBp := api.Blueprint(bpCopy)
				//nolint:staticcheck
				if action == "create" {
					_, err := m.targetClient.CreateBlueprint(ctx, apiBp)
					if err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s: %v", bpID, err))
						mu.Unlock()
						return nil
					}
					mu.Lock()
					result.BlueprintsCreated++
					successfulBlueprints[bpID] = true
					mu.Unlock()
				} else if action == "update" {
					var err error
					if systemblueprints.PrefersPatchUpdate(bpID) {
						err = updater.Update(ctx, bpID, apiBp, blueprintUpdateMode(bpID))
					} else {
						existing, fetchErr := m.targetClient.GetBlueprint(ctx, bpID)
						if fetchErr != nil {
							mu.Lock()
							result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s: %v", bpID, fetchErr))
							mu.Unlock()
							return nil
						}
						for k, v := range apiBp {
							existing[k] = v
						}
						err = updater.Update(ctx, bpID, stripBlueprintSystemFields(existing), blueprintUpdateMode(bpID))
					}
					if err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s: %v", bpID, err))
						mu.Unlock()
						return nil
					}
					mu.Lock()
					result.BlueprintsUpdated++
					successfulBlueprints[bpID] = true
					mu.Unlock()
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
	}

	// Multi-phase second pass — mirrors import.go's phased approach.
	// Ordering is critical because cross-blueprint dependencies require:
	//   Phase 2a: relations        (no cross-blueprint deps)
	//   Phase 2b: calcProps        (self-contained jq expressions)
	//   Phase 2c: mirrorProperties (paths traverse relations on other blueprints)
	//   Phase 2d: aggregationProperties (reference properties on other blueprints)
	//   Phase 2e: ownership        (Inherited type references a relation path)
	//
	// Build the full set of blueprints known to exist in the target by asking
	// the target directly (run after Phase 1, so anything just created above
	// is included too). This is deliberately NOT limited to blueprints this
	// run touched (successfulBlueprints) or diffed as identical
	// (diffResult.BlueprintsToSkip): a scoped migration (AutoScopeBlueprints)
	// may never include a relation target in its own diff at all, even though
	// it already exists in the target — querying the target's real state
	// avoids a false "missing target blueprint" error in that case.
	//
	// Only fetched when there are relations to validate (Phase 2a below) —
	// migrations with no blueprintRelations (pages/integrations/teams/users-only,
	// or scoped runs that touch no relations) shouldn't gain a new failure point
	// from an unrelated target-blueprints listing call.
	existingInTarget := make(map[string]bool)
	if len(blueprintRelations) > 0 {
		targetBlueprints, err := m.targetClient.GetBlueprints(origCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch target blueprints for relation validation: %w", err)
		}
		for _, bp := range targetBlueprints {
			if id, ok := bp["identifier"].(string); ok {
				existingInTarget[id] = true
			}
		}
		for id := range successfulBlueprints {
			existingInTarget[id] = true
		}
		for _, sysID := range import_module.CommonSystemBlueprints() {
			existingInTarget[sysID] = true
		}
	}

	// runBlueprintPhase applies a single field to all blueprints that have it,
	// concurrently. It fetches the existing blueprint and merges the field in.
	runBlueprintPhase := func(phaseName string, fieldsByID map[string]map[string]interface{}) error {
		if len(fieldsByID) == 0 {
			return nil
		}
		g, gCtx := errgroup.WithContext(origCtx)
		for identifier, fields := range fieldsByID {
			if !successfulBlueprints[identifier] {
				continue
			}
			bpID := identifier
			fieldsCopy := fields
			g.Go(func() error {
				existing, err := m.targetClient.GetBlueprint(gCtx, bpID)
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s (%s): failed to fetch: %v", bpID, phaseName, err))
					mu.Unlock()
					return nil
				}
				for k, v := range fieldsCopy {
					existing[k] = v
				}
				cleaned := stripBlueprintSystemFields(existing)
				updateErr := updater.Update(gCtx, bpID, cleaned, blueprintUpdateMode(bpID))
				if updateErr != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s (%s): %v", bpID, phaseName, updateErr))
					mu.Unlock()
				}
				return nil
			})
		}
		return g.Wait()
	}

	// Phase 2a: relations
	if err := runBlueprintPhase("relations", func() map[string]map[string]interface{} {
		out := make(map[string]map[string]interface{})
		for id, rels := range blueprintRelations {
			missing := import_module.ValidateRelationTargets(api.Blueprint{"relations": rels}, existingInTarget)
			if len(missing) > 0 {
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s (relations): missing target blueprints: %v", id, missing))
				mu.Unlock()
				continue
			}
			out[id] = map[string]interface{}{"relations": rels}
		}
		return out
	}()); err != nil {
		return nil, err
	}

	// Phase 2b: calculationProperties
	if err := runBlueprintPhase("calculationProperties", func() map[string]map[string]interface{} {
		out := make(map[string]map[string]interface{})
		for id, v := range blueprintCalcProps {
			out[id] = map[string]interface{}{"calculationProperties": v}
		}
		return out
	}()); err != nil {
		return nil, err
	}

	// Phase 2c: mirrorProperties (depend on relations existing across blueprints).
	// Failures are collected for retry after Phase 2d, because some mirror props
	// reference agg props that don't exist until Phase 2d.
	failedMirrorProps := make(map[string]map[string]interface{})
	if len(blueprintMirrorProps) > 0 {
		mirrorFields := make(map[string]map[string]interface{})
		for id, v := range blueprintMirrorProps {
			if successfulBlueprints[id] {
				mirrorFields[id] = map[string]interface{}{"mirrorProperties": v}
			}
		}
		if len(mirrorFields) > 0 {
			g, gCtx := errgroup.WithContext(origCtx)
			for identifier, fields := range mirrorFields {
				bpID := identifier
				fieldsCopy := fields
				g.Go(func() error {
					existing, err := m.targetClient.GetBlueprint(gCtx, bpID)
					if err != nil {
						mu.Lock()
						failedMirrorProps[bpID] = blueprintMirrorProps[bpID]
						mu.Unlock()
						return nil
					}
					for k, v := range fieldsCopy {
						existing[k] = v
					}
					updateErr := updater.Update(gCtx, bpID, stripBlueprintSystemFields(existing), blueprintUpdateMode(bpID))
					if updateErr != nil {
						mu.Lock()
						failedMirrorProps[bpID] = blueprintMirrorProps[bpID]
						mu.Unlock()
					}
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				return nil, err
			}
		}
	}

	// Phase 2d: aggregationProperties in topological order so that agg props
	// referencing another blueprint's agg props run after their dependencies.
	// Failures are collected for retry after system blueprint updates.
	failedAggProps := make(map[string]map[string]interface{})
	if len(blueprintAggProps) > 0 {
		levels := import_module.TopologicalSortAggProps(blueprintAggProps)
		for _, level := range levels {
			g, gCtx := errgroup.WithContext(origCtx)
			for _, id := range level {
				if !successfulBlueprints[id] {
					continue
				}
				bpID := id
				aggProps := blueprintAggProps[bpID]
				g.Go(func() error {
					existing, err := m.targetClient.GetBlueprint(gCtx, bpID)
					if err != nil {
						mu.Lock()
						failedAggProps[bpID] = aggProps
						mu.Unlock()
						return nil
					}
					existing["aggregationProperties"] = aggProps
					updateErr := updater.Update(gCtx, bpID, stripBlueprintSystemFields(existing), blueprintUpdateMode(bpID))
					if updateErr != nil {
						mu.Lock()
						failedAggProps[bpID] = aggProps
						mu.Unlock()
					}
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				return nil, err
			}
		}
	}

	// Phase 2c retry: mirrorProperties that failed earlier may now succeed
	// because Phase 2d created the agg props they reference.
	if len(failedMirrorProps) > 0 {
		retryFields := make(map[string]map[string]interface{})
		for id, v := range failedMirrorProps {
			retryFields[id] = map[string]interface{}{"mirrorProperties": v}
		}
		if err := runBlueprintPhase("mirrorProperties pass 2/2", retryFields); err != nil {
			return nil, err
		}
	}

	// Phase 2e: ownership in topological order. Inherited ownership references
	// a relation path; the target blueprint's ownership must exist first when
	// multiple blueprints form an ownership chain.
	if len(blueprintOwnership) > 0 {
		var ownershipBlueprints []api.Blueprint
		for _, bp := range data.Blueprints {
			id, ok := bp["identifier"].(string)
			if !ok || id == "" {
				continue
			}
			if _, has := blueprintOwnership[id]; has && successfulBlueprints[id] {
				ownershipBlueprints = append(ownershipBlueprints, bp)
			}
		}

		levels, cyclic := import_module.TopologicalSortOwnership(ownershipBlueprints)

		for _, levelBPs := range append(levels, cyclic) {
			g, gCtx := errgroup.WithContext(origCtx)
			for _, bp := range levelBPs {
				bpID := bp["identifier"].(string)
				ownershipVal := blueprintOwnership[bpID]
				g.Go(func() error {
					existing, err := m.targetClient.GetBlueprint(gCtx, bpID)
					if err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s (ownership): failed to fetch: %v", bpID, err))
						mu.Unlock()
						return nil
					}
					existing["ownership"] = ownershipVal
					updateErr := updater.Update(gCtx, bpID, stripBlueprintSystemFields(existing), blueprintUpdateMode(bpID))
					if updateErr != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Blueprint %s (ownership): %v", bpID, updateErr))
						mu.Unlock()
					}
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				return nil, err
			}
		}
	}

	// Phase 3 retry: aggregationProperties that failed in Phase 2d. Some agg
	// props reference path filters through relations that weren't ready earlier.
	if len(failedAggProps) > 0 {
		retryFields := make(map[string]map[string]interface{})
		for id, v := range failedAggProps {
			retryFields[id] = map[string]interface{}{"aggregationProperties": v}
		}
		if err := runBlueprintPhase("aggregationProperties pass 2/2", retryFields); err != nil {
			return nil, err
		}
	}

	// Import other resources concurrently
	g, ctx = errgroup.WithContext(origCtx)

	// filterEntitiesByDiff limits to only entities that differ from the target (create or update).
	entityImporter := import_module.NewImporter(m.targetClient)
	importResult := &import_module.Result{}
	filtered := filterEntitiesByDiff(data.Entities, entitiesToCreate, entitiesToUpdate)
	// Entity errors are always soft (collected, not fatal) — ImportEntities never returns non-nil.
	_ = entityImporter.ImportEntities(ctx, filtered, false, importResult)
	result.EntitiesCreated += importResult.EntitiesCreated
	result.EntitiesUpdated += importResult.EntitiesUpdated
	result.Errors = append(result.Errors, entityImporter.CollectedErrors()...)

	// Group scorecards by blueprint and separate into create/update
	scorecardsToCreate := make(map[string]bool)
	scorecardsToUpdate := make(map[string]bool)
	for _, sc := range diffResult.ScorecardsToCreate {
		bpID, ok1 := sc["blueprintIdentifier"].(string)
		scID, ok2 := sc["identifier"].(string)
		if ok1 && ok2 {
			scorecardsToCreate[fmt.Sprintf("%s:%s", bpID, scID)] = true
		}
	}
	for _, sc := range diffResult.ScorecardsToUpdate {
		bpID, ok1 := sc["blueprintIdentifier"].(string)
		scID, ok2 := sc["identifier"].(string)
		if ok1 && ok2 {
			scorecardsToUpdate[fmt.Sprintf("%s:%s", bpID, scID)] = true
		}
	}

	scorecardsByBlueprint := make(map[string][]api.Scorecard)
	stripFields := map[string]bool{"createdBy": true, "updatedBy": true, "createdAt": true, "updatedAt": true, "id": true, "blueprint": true, "blueprintIdentifier": true}
	for _, scorecard := range data.Scorecards {
		sc := scorecard
		blueprintID, ok1 := sc["blueprintIdentifier"].(string)
		scorecardID, ok2 := sc["identifier"].(string)
		if !ok1 || !ok2 || blueprintID == "" || scorecardID == "" {
			continue
		}

		key := fmt.Sprintf("%s:%s", blueprintID, scorecardID)
		if scorecardsToCreate[key] || scorecardsToUpdate[key] {
			cleaned := make(api.Scorecard)
			for k, v := range sc {
				if !stripFields[k] {
					cleaned[k] = v
				}
			}
			scorecardsByBlueprint[blueprintID] = append(scorecardsByBlueprint[blueprintID], cleaned)
		}
	}

	for blueprintID, scorecards := range scorecardsByBlueprint {
		bpID := blueprintID
		scs := scorecards
		g.Go(func() error {
			var toMerge []api.Scorecard
			for _, sc := range scs {
				scID, _ := sc["identifier"].(string)
				key := fmt.Sprintf("%s:%s", bpID, scID)

				if scorecardsToCreate[key] {
					_, err := m.targetClient.CreateScorecard(ctx, bpID, sc)
					if err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Scorecard %s: %v", scID, err))
						mu.Unlock()
						continue
					}
					mu.Lock()
					result.ScorecardsCreated++
					mu.Unlock()
				} else if scorecardsToUpdate[key] {
					toMerge = append(toMerge, sc)
				}
			}

			// Port has no PATCH endpoint for individual scorecards, so we
			// fetch the full set, merge in our updates, and bulk PUT.
			if len(toMerge) > 0 {
				existing, fetchErr := m.targetClient.GetScorecards(ctx, bpID)
				if fetchErr != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("Scorecards fetch %s: %v", bpID, fetchErr))
					mu.Unlock()
					return nil
				}

				mergeSet := make(map[string]api.Scorecard, len(toMerge))
				for _, sc := range toMerge {
					mergeSet[sc["identifier"].(string)] = sc
				}

				stripFields := map[string]bool{"createdBy": true, "updatedBy": true, "createdAt": true, "updatedAt": true, "id": true, "blueprint": true, "blueprintIdentifier": true}
				merged := make([]api.Scorecard, 0, len(existing))
				for _, ex := range existing {
					exID, _ := ex["identifier"].(string)
					if replacement, ok := mergeSet[exID]; ok {
						merged = append(merged, replacement)
						delete(mergeSet, exID)
					} else {
						cleaned := make(api.Scorecard)
						for k, v := range ex {
							if !stripFields[k] {
								cleaned[k] = v
							}
						}
						merged = append(merged, cleaned)
					}
				}
				for _, sc := range mergeSet {
					merged = append(merged, sc)
				}

				_, putErr := m.targetClient.UpdateScorecards(ctx, bpID, merged)
				mu.Lock()
				if putErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Scorecards bulk-put %s: %v", bpID, putErr))
				} else {
					result.ScorecardsUpdated += len(toMerge)
				}
				mu.Unlock()
			}

			return nil
		})
	}

	// Import actions
	actionsToCreate := make(map[string]bool)
	actionsToUpdate := make(map[string]bool)
	for _, act := range diffResult.ActionsToCreate {
		if id, ok := act["identifier"].(string); ok {
			actionsToCreate[id] = true
		}
	}
	for _, act := range diffResult.ActionsToUpdate {
		if id, ok := act["identifier"].(string); ok {
			actionsToUpdate[id] = true
		}
	}

	for _, action := range data.Actions {
		act := action
		g.Go(func() error {
			identifier, ok := act["identifier"].(string)
			if !ok || identifier == "" {
				return nil
			}

			cleaned := import_module.CleanActionForCreate(act)
			apiAction := api.Automation(cleaned)

			if actionsToCreate[identifier] {
				_, err := m.targetClient.CreateAutomation(ctx, apiAction)
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("Action %s: %v", identifier, err))
					mu.Unlock()
					return nil
				}
				mu.Lock()
				result.ActionsCreated++
				mu.Unlock()
			} else if actionsToUpdate[identifier] {
				_, err := m.targetClient.UpdateAutomation(ctx, identifier, apiAction)
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("Action %s: %v", identifier, err))
					mu.Unlock()
					return nil
				}
				mu.Lock()
				result.ActionsUpdated++
				mu.Unlock()
			}
			return nil
		})
	}

	// Import teams
	teamsToCreate := make(map[string]bool)
	teamsToUpdate := make(map[string]bool)
	for _, t := range diffResult.TeamsToCreate {
		if name, ok := t["name"].(string); ok {
			teamsToCreate[name] = true
		}
	}
	for _, t := range diffResult.TeamsToUpdate {
		if name, ok := t["name"].(string); ok {
			teamsToUpdate[name] = true
		}
	}

	for _, team := range data.Teams {
		t := team
		g.Go(func() error {
			teamName, ok := t["name"].(string)
			if !ok || teamName == "" {
				return nil
			}

			apiTeam := api.Team(t)

			if teamsToCreate[teamName] {
				_, err := m.targetClient.CreateTeam(ctx, apiTeam)
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("Team %s: %v", teamName, err))
					mu.Unlock()
					return nil
				}
				mu.Lock()
				result.TeamsCreated++
				mu.Unlock()
			} else if teamsToUpdate[teamName] {
				_, err := m.targetClient.UpdateTeam(ctx, teamName, apiTeam)
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("Team %s: %v", teamName, err))
					mu.Unlock()
					return nil
				}
				mu.Lock()
				result.TeamsUpdated++
				mu.Unlock()
			}
			return nil
		})
	}

	// Import users via _user blueprint entity API.
	// New users are staged (or disabled for non-admins when usersAsDisabled is true).
	// Existing users are updated with source data as-is.
	usersToCreate := make(map[string]bool)
	usersToUpdate := make(map[string]bool)
	for _, u := range diffResult.UsersToCreate {
		if email, ok := u["email"].(string); ok {
			usersToCreate[email] = true
		}
	}
	for _, u := range diffResult.UsersToUpdate {
		if email, ok := u["email"].(string); ok {
			usersToUpdate[email] = true
		}
	}

	// Separate users by operation
	var toCreate, toUpdate []api.User
	for _, u := range data.Users {
		email, ok := u["email"].(string)
		if !ok || email == "" {
			continue
		}
		if usersToCreate[email] {
			toCreate = append(toCreate, u)
		} else if usersToUpdate[email] {
			toUpdate = append(toUpdate, u)
		}
	}

	// Bulk-create new users in batches
	for start := 0; start < len(toCreate); start += import_module.UserBatchSize {
		end := start + import_module.UserBatchSize
		if end > len(toCreate) {
			end = len(toCreate)
		}
		batch := toCreate[start:end]

		entities := make([]api.Entity, 0, len(batch))
		byEmail := make(map[string]api.User, len(batch))
		for _, u := range batch {
			email, _ := u["email"].(string)
			byEmail[email] = u
			status := import_module.UserStatusForCreate(u, usersAsDisabled)
			entities = append(entities, import_module.UserToEntity(u, status))
		}

		bulkErrs, err := m.targetClient.CreateUserEntitiesBulk(ctx, entities, false)
		if err != nil {
			mu.Lock()
			for _, e := range entities {
				if email, ok := e["identifier"].(string); ok {
					result.Errors = append(result.Errors, fmt.Sprintf("User %s: %v", email, err))
				}
			}
			mu.Unlock()
			continue
		}

		mu.Lock()
		result.UsersCreated += len(entities) - len(bulkErrs)
		mu.Unlock()

		// Re-POST with upsert=true for conflicts, source data as-is
		var conflictEntities []api.Entity
		for _, be := range bulkErrs {
			if int(be.StatusCode) == 409 {
				if orig, ok := byEmail[be.Identifier]; ok {
					conflictEntities = append(conflictEntities, import_module.UserToEntity(orig, ""))
				}
			} else {
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("User %s: %s: %s", be.Identifier, be.Error, be.Message))
				mu.Unlock()
			}
		}
		if len(conflictEntities) > 0 {
			updateErrs, updateErr := m.targetClient.CreateUserEntitiesBulk(ctx, conflictEntities, true)
			mu.Lock()
			if updateErr != nil {
				for _, e := range conflictEntities {
					if email, ok := e["identifier"].(string); ok {
						result.Errors = append(result.Errors, fmt.Sprintf("User %s: %v", email, updateErr))
					}
				}
			} else {
				result.UsersUpdated += len(conflictEntities) - len(updateErrs)
				for _, be := range updateErrs {
					result.Errors = append(result.Errors, fmt.Sprintf("User %s: %s: %s", be.Identifier, be.Error, be.Message))
				}
			}
			mu.Unlock()
		}
	}

	// Update existing users via POST upsert with source data as-is (no status override)
	for start := 0; start < len(toUpdate); start += import_module.UserBatchSize {
		end := start + import_module.UserBatchSize
		if end > len(toUpdate) {
			end = len(toUpdate)
		}
		batch := toUpdate[start:end]

		entities := make([]api.Entity, 0, len(batch))
		for _, u := range batch {
			entities = append(entities, import_module.UserToEntity(u, ""))
		}

		updateErrs, err := m.targetClient.CreateUserEntitiesBulk(ctx, entities, true)
		mu.Lock()
		if err != nil {
			for _, e := range entities {
				if email, ok := e["identifier"].(string); ok {
					result.Errors = append(result.Errors, fmt.Sprintf("User %s: %v", email, err))
				}
			}
		} else {
			result.UsersUpdated += len(entities) - len(updateErrs)
			for _, be := range updateErrs {
				result.Errors = append(result.Errors, fmt.Sprintf("User %s: %s: %s", be.Identifier, be.Error, be.Message))
			}
		}
		mu.Unlock()
	}

	pagesToCreate := make(map[string]bool)
	pagesToUpdate := make(map[string]bool)
	for _, p := range diffResult.PagesToCreate {
		if id, ok := p["identifier"].(string); ok {
			pagesToCreate[id] = true
		}
	}
	for _, p := range diffResult.PagesToUpdate {
		if id, ok := p["identifier"].(string); ok {
			pagesToUpdate[id] = true
		}
	}

	for _, step := range import_module.PlanSidebarPipeline(data.Folders, data.Pages) {
		stepGroup, stepCtx := errgroup.WithContext(ctx)
		for _, op := range step.Operations {
			op := op
			stepGroup.Go(func() error {
				switch op.ResourceType {
				case "folder":
					folderID := op.Identifier
					if err := m.targetClient.CreateFolder(stepCtx, import_module.CleanFolderForCreate(op.Folder)); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate") && !strings.Contains(strings.ToLower(err.Error()), "already exists") && !strings.Contains(err.Error(), "409") && !strings.Contains(strings.ToLower(err.Error()), "conflict") {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Folder %s: %v", folderID, err))
						mu.Unlock()
					}
					return nil
				case "page":
					p := op.Page
					pageID, ok := p["identifier"].(string)
					if !ok || pageID == "" {
						return nil
					}

					apiPage := api.Page(p)
					addPageError := func(err error) {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Page %s: %v", pageID, err))
						mu.Unlock()
					}
					markPageCreated := func() {
						mu.Lock()
						result.PagesCreated++
						mu.Unlock()
					}
					markPageUpdated := func() {
						mu.Lock()
						result.PagesUpdated++
						mu.Unlock()
					}
					updateExistingPage := func() {
						cleanedPage := import_module.CleanPageForUpdate(apiPage)
						noNavPage := import_module.CleanPageForUpdateNoNav(apiPage)
						_, err := m.targetClient.UpdatePage(stepCtx, pageID, cleanedPage)
						if err == nil {
							markPageUpdated()
						} else if import_module.IsSidebarParentNotFound(err) || import_module.IsAdditionalPropertyError(err) {
							_, retryErr := m.targetClient.UpdatePage(stepCtx, pageID, noNavPage)
							if retryErr != nil {
								addPageError(retryErr)
							} else {
								markPageUpdated()
							}
						} else if import_module.IsAgentIdentifierError(err) {
							existingPage, fetchErr := m.targetClient.GetPage(stepCtx, pageID)
							if fetchErr == nil && existingPage != nil {
								if existingWidgets, ok := existingPage["widgets"].([]interface{}); ok {
									if newWidgets, ok := cleanedPage["widgets"].([]interface{}); ok {
										cleanedPage["widgets"] = import_module.MergeWidgetAgentIdentifiers(newWidgets, existingWidgets)
									}
								}
								_, retryErr := m.targetClient.UpdatePage(stepCtx, pageID, cleanedPage)
								if retryErr != nil {
									noWidgets := make(api.Page)
									for k, v := range cleanedPage {
										if k != "widgets" {
											noWidgets[k] = v
										}
									}
									_, lastErr := m.targetClient.UpdatePage(stepCtx, pageID, noWidgets)
									if lastErr != nil {
										addPageError(lastErr)
									} else {
										markPageUpdated()
									}
								} else {
									markPageUpdated()
								}
							} else {
								addPageError(err)
							}
						} else {
							addPageError(err)
						}
					}

					if pagesToCreate[pageID] {
						cleanedPage := import_module.CleanPageForCreate(apiPage)
						_, err := m.targetClient.CreatePage(stepCtx, cleanedPage)
						if err == nil {
							markPageCreated()
						} else if import_module.IsConflictError(err) {
							updateExistingPage()
						} else if import_module.IsSidebarParentNotFound(err) || import_module.IsAdditionalPropertyError(err) {
							noNavPage := import_module.CleanPageForCreateNoNav(apiPage)
							_, retryErr := m.targetClient.CreatePage(stepCtx, noNavPage)
							if import_module.IsConflictError(retryErr) {
								updateExistingPage()
							} else if retryErr != nil {
								addPageError(retryErr)
							} else {
								markPageCreated()
							}
						} else if import_module.IsAfterItemNotInParent(err) {
							noAfterPage := import_module.CleanPageForCreate(apiPage)
							delete(noAfterPage, "after")
							_, retryErr := m.targetClient.CreatePage(stepCtx, noAfterPage)
							if import_module.IsConflictError(retryErr) {
								updateExistingPage()
							} else if retryErr != nil {
								addPageError(retryErr)
							} else {
								markPageCreated()
							}
						} else if import_module.IsAgentIdentifierError(err) {
							noWidgets := import_module.CleanPageForCreate(apiPage)
							delete(noWidgets, "widgets")
							_, retryErr := m.targetClient.CreatePage(stepCtx, noWidgets)
							if import_module.IsConflictError(retryErr) {
								updateExistingPage()
							} else if retryErr != nil {
								addPageError(retryErr)
							} else {
								markPageCreated()
							}
						} else {
							addPageError(err)
						}
					} else if pagesToUpdate[pageID] {
						updateExistingPage()
					}
					return nil
				}
				return nil
			})
		}
		if err := stepGroup.Wait(); err != nil {
			return nil, err
		}
	}

	// Import integrations
	integrationsToUpdate := make(map[string]bool)
	for _, integ := range diffResult.IntegrationsToUpdate {
		if id, ok := integ["identifier"].(string); ok {
			integrationsToUpdate[id] = true
		}
	}

	for _, integration := range data.Integrations {
		integ := integration
		g.Go(func() error {
			integrationID, ok := integ["identifier"].(string)
			if !ok || integrationID == "" {
				return nil
			}

			if integrationsToUpdate[integrationID] {
				// The integration config endpoint expects {"config": {...}} wrapper — only send config.
				config, ok := integ["config"].(map[string]interface{})
				if !ok || config == nil {
					return nil // No config to update
				}
				configMap := map[string]interface{}{"config": config}

				_, err := m.targetClient.UpdateIntegrationConfig(ctx, integrationID, configMap)
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("Integration %s: %v", integrationID, err))
					mu.Unlock()
					return nil
				}
				mu.Lock()
				result.IntegrationsUpdated++
				mu.Unlock()
			}
			return nil
		})
	}

	// Wait for all imports to complete
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Import permissions (blueprint and action permissions depend on resources existing)
	for _, change := range diffResult.BlueprintPermissions {
		perms := change.Permissions
		_, err := m.targetClient.UpdateBlueprintPermissions(origCtx, change.Identifier, perms)
		if err != nil && import_module.IsInvalidPermissionsError(err) {
			relations, properties := import_module.ParseInvalidPermissionFields(err)
			if len(relations) > 0 || len(properties) > 0 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Stripped orphaned fields from %s permissions: relations=%v properties=%v", change.Identifier, relations, properties))
				perms = import_module.SanitizePermissions(perms, relations, properties)
				_, err = m.targetClient.UpdateBlueprintPermissions(origCtx, change.Identifier, perms)
			}
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Blueprint permissions %s: %v", change.Identifier, err))
		} else {
			result.BlueprintPermissionsUpdated++
		}
	}
	for _, change := range diffResult.ActionPermissions {
		perms := change.Permissions
		_, err := m.targetClient.UpdateActionPermissions(origCtx, change.Identifier, perms)
		if err != nil && import_module.IsInvalidPermissionsError(err) {
			relations, properties := import_module.ParseInvalidPermissionFields(err)
			if len(relations) > 0 || len(properties) > 0 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Stripped orphaned fields from %s action permissions: relations=%v properties=%v", change.Identifier, relations, properties))
				perms = import_module.SanitizePermissions(perms, relations, properties)
				_, err = m.targetClient.UpdateActionPermissions(origCtx, change.Identifier, perms)
			}
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Action permissions %s: %v", change.Identifier, err))
		} else {
			result.ActionPermissionsUpdated++
		}
	}
	for _, change := range diffResult.PagePermissions {
		perms := change.Permissions
		_, err := m.targetClient.UpdatePagePermissions(origCtx, change.Identifier, perms)
		if err != nil && import_module.IsInvalidPermissionsError(err) {
			relations, properties := import_module.ParseInvalidPermissionFields(err)
			if len(relations) > 0 || len(properties) > 0 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Stripped orphaned fields from %s page permissions: relations=%v properties=%v", change.Identifier, relations, properties))
				perms = import_module.SanitizePermissions(perms, relations, properties)
				_, err = m.targetClient.UpdatePagePermissions(origCtx, change.Identifier, perms)
			}
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Page permissions %s: %v", change.Identifier, err))
		} else {
			result.PagePermissionsUpdated++
		}
	}

	// Set skipped counts from diff result
	result.BlueprintsSkipped = len(diffResult.BlueprintsToSkip)
	result.EntitiesSkipped = len(diffResult.EntitiesToSkip)
	result.ScorecardsSkipped = len(diffResult.ScorecardsToSkip)
	result.ActionsSkipped = len(diffResult.ActionsToSkip)
	result.TeamsSkipped = len(diffResult.TeamsToSkip)
	result.UsersSkipped = len(diffResult.UsersToSkip)
	result.PagesSkipped = len(diffResult.PagesToSkip)
	result.IntegrationsSkipped = len(diffResult.IntegrationsToSkip)

	if len(result.IgnoredRuleResultTargetRelationKeys) > 0 {
		sort.Strings(result.IgnoredRuleResultTargetRelationKeys)
	}

	return result, nil
}

// migrateEntities migrates entities for blueprints one at a time. cachedEntities
// holds, per blueprint, entities already fetched from the source during the
// AutoScopeBlueprints relevance pre-scan (see blueprintHasMatchingEntity) —
// when present for a blueprint, it's used in place of a fresh source fetch.
func (m *Module) migrateEntities(ctx context.Context, blueprints []api.Blueprint, opts Options, result *Result, dryRun bool, cachedEntities map[string][]api.Entity) error {
	if len(blueprints) == 0 {
		return nil
	}
	tempDir, err := os.MkdirTemp("", "port-cli-migrate-entities-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	entityImporter := import_module.NewImporter(m.targetClient)
	importCtx := entityImporter.NewEntityImportContext(ctx)
	importResult := &import_module.Result{}
	flushed := false
	flushImportResult := func() {
		if flushed {
			return
		}
		flushed = true
		result.EntitiesCreated += importResult.EntitiesCreated
		result.EntitiesUpdated += importResult.EntitiesUpdated
		result.Errors = append(result.Errors, entityImporter.CollectedErrors()...)
	}
	currentSource := entitystream.FromAPI(m.targetClient)
	source := entitystream.FromAPI(m.sourceClient)
	streamOpts := import_module.EntityStreamOptions{
		IncludeRuleResults: opts.IncludeRuleResults,
		EntityIDs:          opts.Entities,
		OnEntitySkipped: func(api.Entity) {
			result.EntitiesSkipped++
		},
	}

	for _, blueprint := range blueprints {
		bpID, _ := blueprint["identifier"].(string)
		if bpID == "" {
			continue
		}
		if opts.SkipSystemBlueprints && strings.HasPrefix(bpID, "_") {
			continue
		}
		var iterator entitystream.PageIterator
		if cached, ok := cachedEntities[bpID]; ok {
			iterator = entitystream.EntityIterator(0, func(yield func(api.Entity) error) error {
				for _, entity := range cached {
					if err := yield(entity); err != nil {
						return err
					}
				}
				return nil
			})
		} else {
			iterator = entitystream.BlueprintIterator(source, bpID)
		}
		if err := entityImporter.ImportBlueprintEntities(ctx, bpID, iterator, currentSource, streamOpts, importResult, dryRun, importCtx, tempDir); err != nil {
			flushImportResult()
			result.Errors = append(result.Errors, fmt.Sprintf("Entities %s: %v", bpID, err))
			return fmt.Errorf("entities %s: %w", bpID, err)
		}
	}

	flushImportResult()
	return nil
}

// Close closes both API clients.
func (m *Module) Close() error {
	var errs []error
	if m.sourceClient != nil {
		if err := m.sourceClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if m.targetClient != nil {
		if err := m.targetClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing clients: %v", errs)
	}
	return nil
}

// filterEntitiesByDiff returns only entities present in entitiesToCreate or entitiesToUpdate.
func filterEntitiesByDiff(entities []api.Entity, entitiesToCreate, entitiesToUpdate map[string]bool) []api.Entity {
	out := make([]api.Entity, 0, len(entities))
	for _, e := range entities {
		bp, ok1 := e["blueprint"].(string)
		id, ok2 := e["identifier"].(string)
		if !ok1 || !ok2 || bp == "" || id == "" {
			continue
		}
		key := fmt.Sprintf("%s:%s", bp, id)
		if entitiesToCreate[key] || entitiesToUpdate[key] {
			out = append(out, e)
		}
	}
	return out
}
