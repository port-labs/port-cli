package export

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/port-labs/port-cli/internal/api"
	systemblueprints "github.com/port-labs/port-cli/internal/modules/system_blueprints"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// Options represents export options.
type Options struct {
	OutputPath                    string
	Blueprints                    []string
	Format                        string
	SkipEntities                  bool
	SkipSystemBlueprints          bool // skip _* blueprint schemas and their entities
	SkipSystemBlueprintProperties bool
	IncludeRuleResults            bool // include the _rule_result system blueprint and its entities (excluded by default)
	IncludeResources              []string
	ExcludeBlueprints             []string // deep: exclude blueprint schema + all its resources
	ExcludeBlueprintSchema        []string // shallow: exclude only the blueprint schema, keep resources

	// AutoScopeBlueprints, when true, causes Collect to record which blueprints
	// produced at least one matching entity/scorecard/action into
	// Data.ReferencedBlueprintIDs. It does NOT narrow Data.Blueprints itself —
	// callers that want the narrowed set call FilterBlueprintsToReferenced
	// after combining this signal with any other source of "referenced" (e.g.
	// export.go's separate entity-streaming pass, which never runs inside
	// Collect when SkipEntities is forced true).
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

// Validate validates export options.
func (o *Options) Validate() error {
	if o.OutputPath == "" {
		return fmt.Errorf("output_path is required")
	}

	if o.Format != "" && o.Format != "json" && o.Format != "tar" {
		return fmt.Errorf("format must be 'json' or 'tar'")
	}

	return nil
}

// Data represents collected export data.
type Data struct {
	Blueprints []api.Blueprint
	Entities   []api.Entity
	Scorecards []api.Scorecard
	Actions    []api.Action
	// BlueprintPermissions maps blueprint identifier -> permissions object.
	BlueprintPermissions map[string]api.Permissions
	// ActionPermissions maps action identifier -> permissions object.
	ActionPermissions map[string]api.Permissions
	// PagePermissions maps page identifier -> permissions object.
	PagePermissions map[string]api.Permissions
	Teams           []api.Team
	Users           []api.User
	Folders         []api.Folder
	Pages           []api.Page
	Integrations    []api.Integration
	TimeoutErrors   []string // Blueprints that timed out during export
	Warnings        []string // Non-fatal issues encountered during collection
	// ReferencedBlueprintIDs is populated when Options.AutoScopeBlueprints is
	// true: the set of blueprint identifiers that produced at least one
	// matching entity/scorecard/action during Collect. Always non-nil.
	ReferencedBlueprintIDs map[string]bool
}

// maxConcurrentBlueprints caps how many blueprints are fetched in parallel.
// Without a cap, 100+ blueprints each spawn 3-4 goroutines simultaneously,
// exhausting the rate limit on reads before a single response returns.
const maxConcurrentBlueprints = 10

// Collector collects data from Port API concurrently.
type Collector struct {
	client *api.Client
}

// NewCollector creates a new collector.
func NewCollector(client *api.Client) *Collector {
	return &Collector{
		client: client,
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

// isTimeoutError checks if an error is a timeout error (504 Gateway Timeout).
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// Check for various timeout indicators
	return strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "gateway timeout") ||
		strings.Contains(errStr, "timeout_error") ||
		strings.Contains(errStr, "request was too long") ||
		strings.Contains(errStr, "timeout")
}

// FilterByField filters a slice of map-typed resources, keeping only items
// whose field value appears in the ids set. Returns all items when ids is empty.
func FilterByField[T ~map[string]interface{}](items []T, ids []string, field string) []T {
	if len(ids) == 0 {
		return items
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	var out []T
	for _, item := range items {
		if v, _ := item[field].(string); set[v] {
			out = append(out, item)
		}
	}
	return out
}

// FilterFoldersToAncestors returns only the folders that are ancestors of the
// given pages. It walks up the parent chain from each page's parent folder.
func FilterFoldersToAncestors(folders []api.Folder, pages []api.Page) []api.Folder {
	folderByID := make(map[string]api.Folder, len(folders))
	for _, f := range folders {
		if id, _ := f["identifier"].(string); id != "" {
			folderByID[id] = f
		}
	}

	keep := make(map[string]bool)
	for _, page := range pages {
		parent, _ := page["parent"].(string)
		for parent != "" {
			if keep[parent] {
				break
			}
			if f, ok := folderByID[parent]; ok {
				keep[parent] = true
				parent, _ = f["parent"].(string)
			} else {
				break
			}
		}
	}

	var out []api.Folder
	for _, f := range folders {
		if id, _ := f["identifier"].(string); keep[id] {
			out = append(out, f)
		}
	}
	return out
}

// SelectActionsForResources filters the org-wide /actions response to the
// resource types requested by the caller. Non-automation actions are treated as
// actions; trigger.type=automation records are treated as automations.
func SelectActionsForResources(allActions []api.Action, includeActions, includeAutomations bool, actionIDs []string) []api.Action {
	filtered := FilterByField(allActions, actionIDs, "identifier")
	selected := make([]api.Action, 0, len(filtered))
	for _, action := range filtered {
		if api.IsAutomationAction(action) {
			if includeAutomations {
				selected = append(selected, action)
			}
			continue
		}
		if includeActions {
			selected = append(selected, action)
		}
	}
	return selected
}

// FilterBlueprintsToReferenced narrows blueprints to only those whose
// identifier is present in referenced. Used by AutoScopeBlueprints callers
// once they've finished gathering every signal of "this blueprint is
// referenced" (see Data.ReferencedBlueprintIDs).
func FilterBlueprintsToReferenced(blueprints []api.Blueprint, referenced map[string]bool) []api.Blueprint {
	scoped := []api.Blueprint{}
	for _, bp := range blueprints {
		if id, _ := bp["identifier"].(string); referenced[id] {
			scoped = append(scoped, bp)
		}
	}
	return scoped
}

// Collect collects all data from Port API concurrently.
func (c *Collector) Collect(ctx context.Context, opts Options) (*Data, error) {
	data := &Data{
		Blueprints:             []api.Blueprint{},
		Entities:               []api.Entity{},
		Scorecards:             []api.Scorecard{},
		Actions:                []api.Action{},
		Teams:                  []api.Team{},
		Users:                  []api.User{},
		Folders:                []api.Folder{},
		Pages:                  []api.Page{},
		Integrations:           []api.Integration{},
		TimeoutErrors:          []string{},
		BlueprintPermissions:   make(map[string]api.Permissions),
		ActionPermissions:      make(map[string]api.Permissions),
		PagePermissions:        make(map[string]api.Permissions),
		ReferencedBlueprintIDs: make(map[string]bool),
	}

	// Collect blueprints first (needed for other resources)
	allBlueprints, err := c.client.GetBlueprints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get blueprints: %w", err)
	}

	blueprints := FilterByField(allBlueprints, opts.Blueprints, "identifier")
	excludeDeep := opts.ExcludeBlueprints
	if !opts.IncludeRuleResults {
		excludeDeep = append(excludeDeep, "_rule_result")
	}
	iterBlueprints, dataBlueprints := systemblueprints.ApplyExclusions(
		blueprints,
		excludeDeep,
		opts.ExcludeBlueprintSchema,
		opts.SkipSystemBlueprints,
		opts.SkipSystemBlueprintProperties,
	)
	if shouldCollect("blueprints", opts.IncludeResources) {
		data.Blueprints = dataBlueprints
	}
	blueprints = iterBlueprints

	// Use errgroup for concurrent collection, bounded by semaphore to avoid
	// firing 100+ simultaneous requests (one per blueprint) and exhausting the
	// read-side rate limit before any response arrives.
	g, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(maxConcurrentBlueprints)
	var mu sync.Mutex
	var timeoutErrors []string // Track timeout errors separately

	// Collect entities, scorecards, and actions concurrently per blueprint
	for _, blueprint := range blueprints {
		bp := blueprint
		bpID, ok := bp["identifier"].(string)
		if !ok {
			continue
		}

		skipEntitiesForBP := opts.SkipEntities || (opts.SkipSystemBlueprints && strings.HasPrefix(bpID, "_"))
		if !skipEntitiesForBP && shouldCollect("entities", opts.IncludeResources) {
			if err := sem.Acquire(ctx, 1); err != nil {
				return nil, err
			}
			g.Go(func() error {
				defer sem.Release(1)
				var entities []api.Entity
				err := c.client.ForEachEntity(ctx, bpID, func(batch []api.Entity) error {
					entities = append(entities, batch...)
					return nil
				})
				if err != nil {
					if strings.Contains(err.Error(), "410 Gone") {
						return nil
					}
					return fmt.Errorf("failed to get entities for blueprint %s: %w", bpID, err)
				}

				entities = FilterByField(entities, opts.Entities, "identifier")
				mu.Lock()
				data.Entities = append(data.Entities, entities...)
				if opts.AutoScopeBlueprints && len(entities) > 0 {
					data.ReferencedBlueprintIDs[bpID] = true
				}
				mu.Unlock()
				return nil
			})
		}

		// Collect scorecards
		if shouldCollect("scorecards", opts.IncludeResources) {
			if err := sem.Acquire(ctx, 1); err != nil {
				return nil, err
			}
			g.Go(func() error {
				defer sem.Release(1)
				scorecards, err := c.client.GetScorecards(ctx, bpID)
				if err != nil {
					// Silent skip for expected errors
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

				scorecards = FilterByField(scorecards, opts.Scorecards, "identifier")
				mu.Lock()
				data.Scorecards = append(data.Scorecards, scorecards...)
				if opts.AutoScopeBlueprints && len(scorecards) > 0 {
					data.ReferencedBlueprintIDs[bpID] = true
				}
				mu.Unlock()
				return nil
			})
		}

		// Collect blueprint permissions
		if shouldCollect("blueprint-permissions", opts.IncludeResources) || len(opts.IncludeResources) == 0 {
			bpIDCopy := bpID // capture for goroutine closure
			if err := sem.Acquire(ctx, 1); err != nil {
				return nil, err
			}
			g.Go(func() error {
				defer sem.Release(1)
				perms, err := c.client.GetBlueprintPermissions(ctx, bpIDCopy)
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

	// Collect organization-wide resources
	if !opts.SkipEntities && shouldCollect("teams", opts.IncludeResources) {
		g.Go(func() error {
			teams, err := c.client.GetTeams(ctx)
			if err != nil {
				return fmt.Errorf("failed to get teams: %w", err)
			}

			teams = FilterByField(teams, opts.Teams, "name")
			mu.Lock()
			data.Teams = teams
			mu.Unlock()
			return nil
		})
	}

	// Collect users
	if !opts.SkipEntities && shouldCollect("users", opts.IncludeResources) {
		g.Go(func() error {
			users, err := c.client.GetUsers(ctx)
			if err != nil {
				return fmt.Errorf("failed to get users: %w", err)
			}

			users = FilterByField(users, opts.Users, "email")
			mu.Lock()
			data.Users = users
			mu.Unlock()
			return nil
		})
	}

	// Collect actions and automations from the organization-wide /actions endpoint.
	if shouldCollect("actions", opts.IncludeResources) || shouldCollect("automations", opts.IncludeResources) {
		g.Go(func() error {
			allActions, err := c.client.GetAllActions(ctx)
			if err != nil {
				return fmt.Errorf("failed to get all actions/automations: %w", err)
			}

			selectedActions := SelectActionsForResources(
				allActions,
				shouldCollect("actions", opts.IncludeResources),
				shouldCollect("automations", opts.IncludeResources),
				opts.Actions,
			)
			mu.Lock()
			data.Actions = append(data.Actions, selectedActions...)
			if opts.AutoScopeBlueprints {
				for _, action := range selectedActions {
					if bpID := api.ActionBlueprintID(action); bpID != "" {
						data.ReferencedBlueprintIDs[bpID] = true
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
					aID := actionID // capture for goroutine closure
					g.Go(func() error {
						perms, err := c.client.GetActionPermissions(ctx, aID)
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
			folders, err := c.client.GetFolders(ctx)
			if err != nil {
				return fmt.Errorf("failed to get folders: %w", err)
			}
			pages, err := c.client.GetPages(ctx)
			if err != nil {
				return fmt.Errorf("failed to get pages: %w", err)
			}

			pages = FilterByField(pages, opts.Pages, "identifier")

			if len(opts.Pages) > 0 {
				folders = FilterFoldersToAncestors(folders, pages)
			}

			mu.Lock()
			data.Folders = folders
			data.Pages = pages
			mu.Unlock()

			// Fetch permissions for each page (only filtered pages)
			if shouldCollect("page-permissions", opts.IncludeResources) || len(opts.IncludeResources) == 0 {
				for _, page := range pages {
					pageID, ok := page["identifier"].(string)
					if !ok || pageID == "" {
						continue
					}
					pID := pageID
					g.Go(func() error {
						perms, err := c.client.GetPagePermissions(ctx, pID)
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
			integrations, err := c.client.GetIntegrations(ctx)
			if err != nil {
				return fmt.Errorf("failed to get integrations: %w", err)
			}

			integrations = FilterByField(integrations, opts.Integrations, "installationId")
			mu.Lock()
			data.Integrations = integrations
			mu.Unlock()
			return nil
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Attach timeout errors to data
	data.TimeoutErrors = timeoutErrors

	return data, nil
}

// ApplyBlueprintExclusions returns two filtered slices from all:
//   - iterList: used to iterate for fetching entities/scorecards/actions (deep-excluded removed, schema-only kept)
//   - dataList: written to data.Blueprints for export output (both deep and schema-only excluded)
func ApplyBlueprintExclusions(all []api.Blueprint, excludeDeep, excludeSchema []string) (iterList, dataList []api.Blueprint) {
	if len(excludeDeep) == 0 && len(excludeSchema) == 0 {
		return all, all
	}
	deepSet := make(map[string]bool, len(excludeDeep))
	for _, id := range excludeDeep {
		deepSet[id] = true
	}
	schemaSet := make(map[string]bool, len(excludeSchema))
	for _, id := range excludeSchema {
		schemaSet[id] = true
	}

	for _, bp := range all {
		id, _ := bp["identifier"].(string)
		if deepSet[id] {
			// Deep exclusion: skip in both lists
			continue
		}
		// In iteration list (fetches dependent resources) for everyone not deep-excluded
		iterList = append(iterList, bp)
		if schemaSet[id] {
			// Schema-only exclusion: skip in data (output) list only
			continue
		}
		dataList = append(dataList, bp)
	}
	return iterList, dataList
}
