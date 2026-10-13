package import_module

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/port-labs/port-cli/internal/api"
	"github.com/port-labs/port-cli/internal/auth"
	"github.com/port-labs/port-cli/internal/config"
	"github.com/port-labs/port-cli/internal/modules/export"
	systemblueprints "github.com/port-labs/port-cli/internal/modules/system_blueprints"
)

// Module handles importing data to Port.
type Module struct {
	client *api.Client
}

// NewModule creates a new import module.
func NewModule(token *auth.Token, orgConfig *config.OrganizationConfig) *Module {
	client := api.NewClient(api.ClientOpts{
		Token:        token,
		ClientID:     orgConfig.ClientID,
		ClientSecret: orgConfig.ClientSecret,
		APIURL:       orgConfig.APIURL,
		Timeout:      0,
	})
	return &Module{
		client: client,
	}
}

// Options represents import options.
// ProgressCallback is called to report import progress.
// phase is the current phase name, current is the number of items processed, total is the total count.
type ProgressCallback func(phase string, current, total int)

// Options represents import options.
type Options struct {
	InputPath                     string
	DryRun                        bool
	SkipEntities                  bool
	SkipSystemBlueprints          bool // skip _* blueprint schemas and their entities
	SkipSystemBlueprintProperties bool
	IncludeRuleResults            bool // include _rule_result system blueprint entities (included by default)
	IncludeResources              []string
	ExcludeBlueprints             []string // deep: exclude blueprint schema + all its resources
	ExcludeBlueprintSchema        []string // shallow: exclude only the blueprint schema, keep resources
	UsersAsDisabled               bool     // import non-admin users as DISABLED after staging
	Verbose                       bool
	ShowPagesPipeline             bool
	ProgressCallback              ProgressCallback
	LogCallback                   func(string)
	ErrorHandling                 ErrorHandlingOptions
}

// ValidationWarning represents a pre-import validation warning.
type ValidationWarning struct {
	Type    string // "cycle", "missing_dependency", "protected_resource", "orphaned_permission_field"
	Message string
	Details []string
}

// Result represents the result of an import operation.
type Result struct {
	Success                     bool
	Message                     string
	BlueprintsCreated           int
	BlueprintsUpdated           int
	EntitiesCreated             int
	EntitiesUpdated             int
	ScorecardsCreated           int
	ScorecardsUpdated           int
	ActionsCreated              int
	ActionsUpdated              int
	TeamsCreated                int
	TeamsUpdated                int
	UsersCreated                int
	UsersUpdated                int
	PagesCreated                int
	PagesUpdated                int
	IntegrationsUpdated         int
	BlueprintPermissionsUpdated int
	ActionPermissionsUpdated    int
	PagePermissionsUpdated      int
	Errors                      []string
	ErrorsByCategory            map[string][]string // Categorized errors for verbose output
	Warnings                    []ValidationWarning // Pre-import validation warnings
	DiffResult                  *DiffResult
	SidebarPipeline             []string
	// IgnoredRuleResultTargetRelationCount is how many _rule_result relations with type rule_result_target were omitted from API payloads.
	IgnoredRuleResultTargetRelationCount int
	// IgnoredRuleResultTargetRelationKeys lists relation identifiers omitted (sorted, unique).
	IgnoredRuleResultTargetRelationKeys []string
}

type SidebarPipelineOperation struct {
	ResourceType string
	Identifier   string
	Folder       api.Folder
	Page         api.Page
}

type SidebarPipelineStep struct {
	Operations []SidebarPipelineOperation
}

// Execute performs the import operation.
func (m *Module) Execute(ctx context.Context, opts Options) (*Result, error) {
	// Load data
	loader := NewLoader()
	streamEntities := !opts.SkipEntities && shouldImport("entities", opts.IncludeResources)
	var data *export.Data
	var err error
	if streamEntities {
		data, err = NewStreamLoader().LoadDataWithoutEntities(opts.InputPath)
	} else {
		data, err = loader.LoadData(opts.InputPath)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load data: %w", err)
	}

	// Apply blueprint exclusions before diffing/importing
	applyDataExclusion(data, opts.ExcludeBlueprints, opts.ExcludeBlueprintSchema, opts.SkipSystemBlueprints, opts.SkipSystemBlueprintProperties)

	// Validate data
	if err := loader.ValidateData(data, opts.IncludeResources); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Diff validation (always enabled)
	comparer := NewDiffComparer(m.client)
	compareOpts := opts
	if streamEntities {
		compareOpts.SkipEntities = true
	}
	diffResult, err := comparer.Compare(ctx, data, compareOpts)
	if err != nil {
		return nil, fmt.Errorf("diff comparison failed: %w", err)
	}

	// Use diff result to filter data
	data = diffResult.FilterData(data)

	sidebarPipeline := PlanSidebarPipeline(data.Folders, data.Pages)

	// Dry run - show what would happen
	if opts.DryRun {
		result := m.generateDryRunResult(data, diffResult, opts)
		if streamEntities {
			importer := NewImporter(m.client)
			if opts.ProgressCallback != nil {
				importer.SetProgressCallback(opts.ProgressCallback)
			}
			if err := importer.ImportEntitiesFromStream(ctx, opts.InputPath, opts, result, true); err != nil {
				return nil, fmt.Errorf("streaming entity dry run failed: %w", err)
			}
		}
		result.SidebarPipeline = DescribeSidebarPipeline(sidebarPipeline)
		return result, nil
	}

	// Import data using new reliable importer
	importer := NewImporter(m.client)
	if len(sidebarPipeline) > 0 && opts.LogCallback != nil && opts.ShowPagesPipeline {
		opts.LogCallback("Proposed sidebar pipeline:")
		for _, line := range DescribeSidebarPipeline(sidebarPipeline) {
			opts.LogCallback(fmt.Sprintf("  %s", line))
		}
	}
	importOpts := opts
	if streamEntities {
		importOpts.SkipEntities = true
	}
	result, err := importer.Import(ctx, data, importOpts)
	if err != nil {
		return nil, fmt.Errorf("import failed: %w", err)
	}
	if streamEntities {
		if err := importer.ImportEntitiesFromStream(ctx, opts.InputPath, opts, result, false); err != nil {
			return nil, fmt.Errorf("streaming entity import failed: %w", err)
		}
	}

	// Import permissions (blueprint and action permissions depend on resources existing)
	bpUpdated, actionUpdated, pageUpdated, permWarnings := importer.importPermissions(ctx, diffResult)

	// Surface permission sanitization warnings as validation warnings
	for _, w := range permWarnings {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:    "orphaned_permission_field",
			Message: w,
		})
	}

	// Merge any permission errors into result
	result.Errors = importer.errors.ToStringSlice()
	result.BlueprintPermissionsUpdated = bpUpdated
	result.ActionPermissionsUpdated = actionUpdated
	result.PagePermissionsUpdated = pageUpdated

	if len(result.Errors) > 0 {
		result.Success = false
		result.Message = fmt.Sprintf("Import completed with %d error(s)", len(result.Errors))
	} else {
		result.Success = true
		result.Message = "Successfully imported data"
	}
	result.DiffResult = diffResult
	result.SidebarPipeline = DescribeSidebarPipeline(sidebarPipeline)
	return result, nil
}

// generateDryRunResult generates a dry run result with accurate predictions.
func (m *Module) generateDryRunResult(data *export.Data, diffResult *DiffResult, _ Options) *Result {
	if diffResult != nil {
		return &Result{
			Success:                     true,
			Message:                     "Validation passed (dry run - no changes applied)",
			BlueprintsCreated:           len(diffResult.BlueprintsToCreate),
			BlueprintsUpdated:           len(diffResult.BlueprintsToUpdate),
			EntitiesCreated:             len(diffResult.EntitiesToCreate),
			EntitiesUpdated:             len(diffResult.EntitiesToUpdate),
			ScorecardsCreated:           len(diffResult.ScorecardsToCreate),
			ScorecardsUpdated:           len(diffResult.ScorecardsToUpdate),
			ActionsCreated:              len(diffResult.ActionsToCreate),
			ActionsUpdated:              len(diffResult.ActionsToUpdate),
			TeamsCreated:                len(diffResult.TeamsToCreate),
			TeamsUpdated:                len(diffResult.TeamsToUpdate),
			UsersCreated:                len(diffResult.UsersToCreate),
			UsersUpdated:                len(diffResult.UsersToUpdate),
			PagesCreated:                len(diffResult.PagesToCreate),
			PagesUpdated:                len(diffResult.PagesToUpdate),
			IntegrationsUpdated:         len(diffResult.IntegrationsToUpdate),
			BlueprintPermissionsUpdated: len(diffResult.BlueprintPermissions),
			ActionPermissionsUpdated:    len(diffResult.ActionPermissions),
			PagePermissionsUpdated:      len(diffResult.PagePermissions),
			DiffResult:                  diffResult,
		}
	}

	return &Result{
		Success:           true,
		Message:           "Validation passed (dry run - no changes applied)",
		BlueprintsCreated: len(data.Blueprints),
		EntitiesCreated:   len(data.Entities),
	}
}

// Close closes the API client.
func (m *Module) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}

// shouldImport checks if a resource type should be imported.
func shouldImport(resourceType string, includeResources []string) bool {
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

// cleanSystemFields removes system fields that shouldn't be sent to API.
func cleanSystemFields(resource map[string]interface{}, fieldsToRemove []string) map[string]interface{} {
	cleaned := make(map[string]interface{})
	removeSet := make(map[string]bool)
	for _, f := range fieldsToRemove {
		removeSet[f] = true
	}
	for k, v := range resource {
		if !removeSet[k] {
			cleaned[k] = v
		}
	}
	return cleaned
}

// isConflictError checks if an error is a conflict (409) error.
func isConflictError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "409") || strings.Contains(errStr, "Conflict")
}

// IsConflictError checks if an error indicates that the resource already exists.
func IsConflictError(err error) bool {
	return isConflictError(err)
}

func (i *Importer) recordRuleResultIgnoredRelations(ignored []string, result *Result) {
	if len(ignored) == 0 || result == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.ruleResultIgnoreDedupe == nil {
		i.ruleResultIgnoreDedupe = make(map[string]struct{})
	}
	var logKeys []string
	for _, k := range ignored {
		if _, dup := i.ruleResultIgnoreDedupe[k]; dup {
			continue
		}
		i.ruleResultIgnoreDedupe[k] = struct{}{}
		result.IgnoredRuleResultTargetRelationCount++
		result.IgnoredRuleResultTargetRelationKeys = append(result.IgnoredRuleResultTargetRelationKeys, k)
		logKeys = append(logKeys, k)
	}
	if len(logKeys) > 0 && i.log != nil {
		i.log(fmt.Sprintf("Ignored %d _rule_result relation(s) (not sent to API): %s", len(logKeys), strings.Join(logKeys, ", ")))
	}
}

// isProtectedBlueprint checks if a blueprint is protected (entities can't be created).
func isProtectedBlueprint(blueprintID string, includeRuleResults bool) bool {
	if strings.HasPrefix(blueprintID, "_rule") {
		return !includeRuleResults
	}
	return false
}

// blueprintRelatesToInheritedOwnership checks if a blueprint has ANY relation to a blueprint with inherited ownership.
// This is used to skip all entities from such blueprints, since Port will reject them.
func blueprintRelatesToInheritedOwnership(blueprintID string, inheritedOwnershipBPs map[string]bool, relationTargets map[string]map[string]string) bool {
	// Get the relation targets for this blueprint
	bpRelations, ok := relationTargets[blueprintID]
	if !ok {
		return false
	}

	// Check if any relation targets an inherited ownership blueprint
	for _, targetBP := range bpRelations {
		if inheritedOwnershipBPs[targetBP] {
			return true
		}
	}

	return false
}

// detectInheritedOwnershipBlueprints fetches blueprints and returns:
// 1. A set of blueprint IDs that have inherited ownership enabled
// 2. A map of blueprintID -> relationName -> targetBlueprintID for all blueprints
func (i *Importer) detectInheritedOwnershipBlueprints(ctx context.Context) (map[string]bool, map[string]map[string]string) {
	inheritedOwnership := make(map[string]bool)
	relationTargets := make(map[string]map[string]string)

	blueprints, err := i.client.GetBlueprints(ctx)
	if err != nil {
		// If we can't fetch blueprints, return empty maps and let errors occur naturally
		return inheritedOwnership, relationTargets
	}

	for _, bp := range blueprints {
		id, ok := bp["identifier"].(string)
		if !ok || id == "" {
			continue
		}

		// Check for teamInheritance field with inheritOwnership property
		if teamInheritance, ok := bp["teamInheritance"].(map[string]interface{}); ok {
			if inheritOwnership, ok := teamInheritance["inheritOwnership"].(bool); ok && inheritOwnership {
				inheritedOwnership[id] = true
			}
		}

		// Also check the older/alternative field name
		if inheritOwnershipVal, ok := bp["inheritedOwnership"].(bool); ok && inheritOwnershipVal {
			inheritedOwnership[id] = true
		}

		// Extract relation targets for this blueprint
		if relations, ok := bp["relations"].(map[string]interface{}); ok {
			relationTargets[id] = make(map[string]string)
			for relName, relDef := range relations {
				if relMap, ok := relDef.(map[string]interface{}); ok {
					if target, ok := relMap["target"].(string); ok {
						relationTargets[id][relName] = target
					}
				}
			}
		}
	}

	return inheritedOwnership, relationTargets
}

// Importer handles importing data to Port with proper dependency ordering.
type Importer struct {
	client                 *api.Client
	errors                 *ErrorCollector
	mu                     sync.Mutex
	log                    func(string)
	verbose                bool
	progress               ProgressCallback
	ruleResultIgnoreDedupe map[string]struct{}
}

// NewImporter creates a new importer.
func NewImporter(client *api.Client) *Importer {
	return &Importer{
		client: client,
		errors: NewErrorCollector(),
	}
}

// SetProgressCallback sets the progress callback for the importer.
func (i *Importer) SetProgressCallback(cb ProgressCallback) {
	i.progress = cb
}

// CollectedErrors returns all errors accumulated during the last operation.
func (i *Importer) CollectedErrors() []string {
	return i.errors.ToStringSlice()
}

func (i *Importer) SetLogCallback(cb func(string)) {
	i.log = cb
}

// reportProgress reports progress if a callback is set.
func (i *Importer) reportProgress(phase string, current, total int) {
	if i.progress != nil {
		i.progress(phase, current, total)
	}
}

// Import imports data to Port with proper dependency ordering.
func (i *Importer) Import(ctx context.Context, data *export.Data, opts Options) (*Result, error) {
	// Set progress callback if provided
	if opts.ProgressCallback != nil {
		i.progress = opts.ProgressCallback
	}
	i.verbose = opts.Verbose
	if opts.LogCallback != nil {
		i.log = opts.LogCallback
	}

	result := &Result{
		Errors:           []string{},
		ErrorsByCategory: make(map[string][]string),
		Warnings:         []ValidationWarning{},
	}
	i.ruleResultIgnoreDedupe = make(map[string]struct{})

	// Import blueprints with three-phase approach
	if shouldImport("blueprints", opts.IncludeResources) {
		if err := i.importBlueprints(ctx, data.Blueprints, result, opts.ErrorHandling); err != nil {
			return nil, err
		}
	}

	// Import other resources concurrently (but with bounded concurrency)
	if err := i.importOtherResources(ctx, data, opts, result); err != nil {
		return nil, err
	}

	// Convert collected errors to string slice for backward compatibility
	result.Errors = i.errors.ToStringSlice()

	// Populate errors by category for verbose output
	for _, category := range []ErrorCategory{
		ErrDependency, ErrAuth, ErrBlueprintConfig, ErrValidation,
		ErrSchemaMismatch, ErrRateLimit, ErrNetwork, ErrConflict,
		ErrNotFound, ErrUnknown,
	} {
		categoryErrors := i.errors.GetByCategory(category)
		if len(categoryErrors) > 0 {
			categoryStrings := make([]string, len(categoryErrors))
			for j, e := range categoryErrors {
				categoryStrings[j] = e.Error()
			}
			result.ErrorsByCategory[string(category)] = categoryStrings
		}
	}

	return result, nil
}

// importBlueprints imports blueprints using a multi-phase approach:
// Phase 1: Create non-system blueprints with relations and dependent fields stripped
// Phase 2a: Add relations back to all blueprints
// Phase 2b: Add calculationProperties (self-contained, no cross-blueprint dependencies)
// Phase 2c: Add mirrorProperties (depend on relations existing)
// Phase 2d: Add aggregationProperties (depend on properties existing on OTHER blueprints)
// Phase 3: Update system blueprints
func (i *Importer) importBlueprints(ctx context.Context, blueprints []api.Blueprint, result *Result, errorHandlingOpts ...ErrorHandlingOptions) error {
	var errorHandling ErrorHandlingOptions
	if len(errorHandlingOpts) > 0 {
		errorHandling = errorHandlingOpts[0]
	}
	errorHandling.AddWarning = i.handledErrorWarningCallback(result, errorHandling.AddWarning)
	updater := NewBlueprintUpdater(i.client, errorHandling)

	// Separate system and non-system blueprints
	nonSystemBPs, systemBPs := SeparateSystemBlueprints(blueprints)

	// Build existing blueprints set (system blueprints are assumed to exist)
	existingBPs := make(map[string]bool)
	for _, bp := range systemBPs {
		if id, ok := bp["identifier"].(string); ok {
			existingBPs[id] = true
		}
	}
	// Also add common system blueprints that might not be in export
	for _, id := range CommonSystemBlueprints() {
		existingBPs[id] = true
	}

	// Store each field type separately for ordered updates in Phase 2
	storedRelations := make(map[string]map[string]interface{})
	storedCalcProps := make(map[string]map[string]interface{})
	storedMirrorProps := make(map[string]map[string]interface{})
	storedAggProps := make(map[string]map[string]interface{})
	storedOwnership := make(map[string]map[string]interface{})
	strippedBPs := make([]api.Blueprint, 0, len(nonSystemBPs))

	for _, bp := range nonSystemBPs {
		id, ok := bp["identifier"].(string)
		if !ok || id == "" {
			i.errors.Add(fmt.Errorf("blueprint is missing identifier field, skipping"), "blueprint", "<unknown>")
			continue
		}

		if relations, ok := bp["relations"].(map[string]interface{}); ok && len(relations) > 0 {
			storedRelations[id] = relations
		}

		// Extract and store each dependent field type separately
		if calcProps, ok := bp["calculationProperties"].(map[string]interface{}); ok && len(calcProps) > 0 {
			storedCalcProps[id] = calcProps
		}
		if mirrorProps, ok := bp["mirrorProperties"].(map[string]interface{}); ok && len(mirrorProps) > 0 {
			storedMirrorProps[id] = mirrorProps
		}
		if aggProps, ok := bp["aggregationProperties"].(map[string]interface{}); ok && len(aggProps) > 0 {
			storedAggProps[id] = aggProps
		}
		if ownership, ok := bp["ownership"].(map[string]interface{}); ok && len(ownership) > 0 {
			storedOwnership[id] = ownership
		}

		stripped := StripDependentFields(bp)
		stripped = StripRelations(stripped)
		strippedBPs = append(strippedBPs, stripped)
	}

	// Topological sort
	levels, cyclic := TopologicalSort(strippedBPs, existingBPs)

	// Add warning about cyclic blueprints
	if len(cyclic) > 0 {
		cyclicIDs := make([]string, 0, len(cyclic))
		for _, bp := range cyclic {
			if id, ok := bp["identifier"].(string); ok {
				cyclicIDs = append(cyclicIDs, id)
			}
		}
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:    "cycle",
			Message: fmt.Sprintf("Detected %d blueprints with circular dependencies", len(cyclic)),
			Details: cyclicIDs,
		})
	}

	// Track successfully created blueprints
	successfulBPs := make(map[string]bool)
	for id := range existingBPs {
		successfulBPs[id] = true
	}

	// Phase 1: Create non-system blueprints in dependency order
	pool := NewWorkerPool(BlueprintConcurrency)
	totalBPs := len(FlattenLevels(levels)) + len(cyclic)
	createdCount := 0

	for levelIdx, level := range levels {
		i.reportProgress(fmt.Sprintf("Blueprints (level %d/%d)", levelIdx+1, len(levels)), createdCount, totalBPs)

		var levelMu sync.Mutex
		for _, bp := range level {
			bp := bp
			pool.Go(func() {
				id := bp["identifier"].(string)
				created, updated, err := i.createOrUpdateBlueprint(ctx, bp, result, updater)

				i.mu.Lock()
				if err != nil {
					i.errors.Add(err, "blueprint", id)
				} else {
					if created {
						result.BlueprintsCreated++
					} else if updated {
						result.BlueprintsUpdated++
					}
					levelMu.Lock()
					successfulBPs[id] = true
					levelMu.Unlock()
				}
				createdCount++
				i.mu.Unlock()
			})
		}
		pool.Wait()
	}

	// Handle cyclic blueprints (best effort)
	if len(cyclic) > 0 {
		i.reportProgress("Blueprints (cyclic)", createdCount, totalBPs)
		for _, bp := range cyclic {
			bp := bp
			pool.Go(func() {
				id := bp["identifier"].(string)
				created, updated, err := i.createOrUpdateBlueprint(ctx, bp, result, updater)

				i.mu.Lock()
				if err != nil {
					i.errors.Add(err, "blueprint", id)
				} else {
					if created {
						result.BlueprintsCreated++
					} else if updated {
						result.BlueprintsUpdated++
					}
					successfulBPs[id] = true
				}
				createdCount++
				i.mu.Unlock()
			})
		}
		pool.Wait()
	}

	// Fetch ALL existing blueprints from target for validation
	allExistingBPs := make(map[string]bool)
	for id := range successfulBPs {
		allExistingBPs[id] = true
	}
	targetBlueprints, err := i.client.GetBlueprints(ctx)
	if err == nil {
		for _, bp := range targetBlueprints {
			if id, ok := bp["identifier"].(string); ok && id != "" {
				allExistingBPs[id] = true
			}
		}
	}

	// Phase 2a: Add relations back to all blueprints
	if len(storedRelations) > 0 {
		i.reportProgress("Blueprints (adding relations)", 0, len(storedRelations))
		count := 0
		for id, relations := range storedRelations {
			if !allExistingBPs[id] {
				continue
			}
			id, relations := id, relations
			pool.Go(func() {
				err := i.updateBlueprintFieldsDirect(ctx, id, map[string]interface{}{"relations": relations}, result, updater)
				i.mu.Lock()
				if err != nil {
					i.errors.Add(err, "blueprint", id)
				}
				count++
				i.reportProgress("Blueprints (adding relations)", count, len(storedRelations))
				i.mu.Unlock()
			})
		}
		pool.Wait()
	}

	// Phase 2b: Add calculationProperties (self-contained, no cross-blueprint deps)
	if len(storedCalcProps) > 0 {
		i.reportProgress("Blueprints (adding calculationProperties)", 0, len(storedCalcProps))
		count := 0
		for id, calcProps := range storedCalcProps {
			if !allExistingBPs[id] {
				continue
			}
			id, calcProps := id, calcProps
			pool.Go(func() {
				err := i.updateBlueprintFieldsDirect(ctx, id, map[string]interface{}{"calculationProperties": calcProps}, result, updater)
				i.mu.Lock()
				if err != nil {
					i.errors.Add(err, "blueprint", id)
				}
				count++
				i.reportProgress("Blueprints (adding calculationProperties)", count, len(storedCalcProps))
				i.mu.Unlock()
			})
		}
		pool.Wait()
	}

	// failedMirrorProps collects Phase 2c failures for a second pass after Phase 2d,
	// because some mirror props reference agg props that don't exist until Phase 2d.
	failedMirrorProps := make(map[string]map[string]interface{})
	var failedMirrorMu sync.Mutex

	// Phase 2c: Add mirrorProperties (depend on relations existing)
	if len(storedMirrorProps) > 0 {
		i.reportProgress("Blueprints (adding mirrorProperties)", 0, len(storedMirrorProps))
		count := 0
		for id, mirrorProps := range storedMirrorProps {
			if !allExistingBPs[id] {
				continue
			}
			id, mirrorProps := id, mirrorProps
			pool.Go(func() {
				err := i.updateBlueprintFieldsDirect(ctx, id, map[string]interface{}{"mirrorProperties": mirrorProps}, result, updater)
				if err != nil {
					failedMirrorMu.Lock()
					failedMirrorProps[id] = mirrorProps
					failedMirrorMu.Unlock()
				}
				i.mu.Lock()
				count++
				i.reportProgress("Blueprints (adding mirrorProperties)", count, len(storedMirrorProps))
				i.mu.Unlock()
			})
		}
		pool.Wait()
	}

	// Phase 2d: Add aggregationProperties in topological order so that agg props
	// referencing another blueprint's agg props are applied after their dependencies
	// (e.g. businessApplication.codeQualityBugs must run after component.codeQualityBugs).
	// Failures are retried after Phase 3 (system blueprint updates) because some agg props
	// use path filters through system blueprint relations (e.g. _rule_result._githubBranch)
	// that don't exist until Phase 3 applies the system blueprint schema.
	failedAggProps := make(map[string]map[string]interface{})
	var failedAggMu sync.Mutex

	if len(storedAggProps) > 0 {
		levels := TopologicalSortAggProps(storedAggProps)
		for levelIdx, level := range levels {
			label := fmt.Sprintf("Blueprints (adding aggregationProperties, level %d/%d)", levelIdx+1, len(levels))
			i.reportProgress(label, 0, len(level))
			count := 0
			for _, id := range level {
				if !allExistingBPs[id] {
					continue
				}
				id, aggProps := id, storedAggProps[id]
				pool.Go(func() {
					err := i.updateBlueprintFieldsDirect(ctx, id, map[string]interface{}{"aggregationProperties": aggProps}, result, updater)
					if err != nil {
						failedAggMu.Lock()
						failedAggProps[id] = aggProps
						failedAggMu.Unlock()
					}
					i.mu.Lock()
					count++
					i.reportProgress(label, count, len(level))
					i.mu.Unlock()
				})
			}
			pool.Wait()
		}
	}

	// Phase 2e: Retry mirror properties that failed in Phase 2c. Some mirror props
	// reference aggregation properties on related blueprints that now exist after Phase 2d.
	if len(failedMirrorProps) > 0 {
		i.reportProgress("Blueprints (adding mirrorProperties, pass 2/2)", 0, len(failedMirrorProps))
		count := 0
		for id, mirrorProps := range failedMirrorProps {
			if !allExistingBPs[id] {
				continue
			}
			id, mirrorProps := id, mirrorProps
			pool.Go(func() {
				err := i.updateBlueprintFieldsDirect(ctx, id, map[string]interface{}{"mirrorProperties": mirrorProps}, result, updater)
				i.mu.Lock()
				if err != nil {
					i.errors.Add(err, "blueprint", id)
				}
				count++
				i.reportProgress("Blueprints (adding mirrorProperties, pass 2/2)", count, len(failedMirrorProps))
				i.mu.Unlock()
			})
		}
		pool.Wait()
	}

	// Phase 2e: Add ownership (inherited ownership depends on relations existing)
	if len(storedOwnership) > 0 {
		var ownershipBlueprints []api.Blueprint
		for _, bp := range append(nonSystemBPs, systemBPs...) {
			id, ok := bp["identifier"].(string)
			if !ok || id == "" {
				continue
			}
			if ownership, ok := storedOwnership[id]; ok && len(ownership) > 0 {
				ownershipBlueprints = append(ownershipBlueprints, bp)
			}
		}

		levels, cyclic := TopologicalSortOwnership(ownershipBlueprints)
		totalOwnership := len(FlattenLevels(levels)) + len(cyclic)
		appliedCount := 0

		for levelIdx, level := range levels {
			i.reportProgress(fmt.Sprintf("Blueprints (adding ownership level %d/%d)", levelIdx+1, len(levels)), appliedCount, totalOwnership)
			for _, bp := range level {
				id := bp["identifier"].(string)
				if !allExistingBPs[id] {
					continue
				}
				ownership := storedOwnership[id]
				pool.Go(func() {
					err := i.updateBlueprintFieldsDirect(ctx, id, map[string]interface{}{"ownership": ownership}, result, updater)
					i.mu.Lock()
					if err != nil {
						i.errors.Add(err, "blueprint", id)
					}
					appliedCount++
					i.mu.Unlock()
				})
			}
			pool.Wait()
		}

		if len(cyclic) > 0 {
			i.reportProgress("Blueprints (adding ownership cyclic)", appliedCount, totalOwnership)
			for _, bp := range cyclic {
				id := bp["identifier"].(string)
				if !allExistingBPs[id] {
					continue
				}
				ownership := storedOwnership[id]
				pool.Go(func() {
					err := i.updateBlueprintFieldsDirect(ctx, id, map[string]interface{}{"ownership": ownership}, result, updater)
					i.mu.Lock()
					if err != nil {
						i.errors.Add(err, "blueprint", id)
					}
					appliedCount++
					i.mu.Unlock()
				})
			}
			pool.Wait()
		}
	}

	// Phase 3: Update system blueprints
	if len(systemBPs) > 0 {
		i.reportProgress("System blueprints", 0, len(systemBPs))
		sysCount := 0

		for _, bp := range systemBPs {
			bp := bp
			pool.Go(func() {
				id := bp["identifier"].(string)
				_, updated, err := i.createOrUpdateBlueprint(ctx, bp, result, updater)

				i.mu.Lock()
				if err != nil {
					i.errors.Add(err, "blueprint", id)
				} else if updated {
					result.BlueprintsUpdated++
				}
				sysCount++
				i.reportProgress("System blueprints", sysCount, len(systemBPs))
				i.mu.Unlock()
			})
		}
		pool.Wait()
	}

	// Phase 4: Retry aggregationProperties that failed in Phase 2d. Some agg props
	// reference path filters through system blueprint relations (e.g. _rule_result._githubBranch)
	// that only exist after Phase 3 updates the system blueprint schema.
	if len(failedAggProps) > 0 {
		i.reportProgress("Blueprints (adding aggregationProperties, pass 2/2)", 0, len(failedAggProps))
		count := 0
		for id, aggProps := range failedAggProps {
			if !allExistingBPs[id] {
				continue
			}
			id, aggProps := id, aggProps
			pool.Go(func() {
				err := i.updateBlueprintFieldsDirect(ctx, id, map[string]interface{}{"aggregationProperties": aggProps}, result, updater)
				i.mu.Lock()
				if err != nil {
					i.errors.Add(err, "blueprint", id)
				}
				count++
				i.reportProgress("Blueprints (adding aggregationProperties, pass 2/2)", count, len(failedAggProps))
				i.mu.Unlock()
			})
		}
		pool.Wait()
	}

	if len(result.IgnoredRuleResultTargetRelationKeys) > 0 {
		sort.Strings(result.IgnoredRuleResultTargetRelationKeys)
	}

	return nil
}

func (i *Importer) handledErrorWarningCallback(result *Result, existing func(string)) func(string) {
	return func(message string) {
		i.mu.Lock()
		defer i.mu.Unlock()
		result.Warnings = append(result.Warnings, ValidationWarning{
			Type:    "handled_error",
			Message: message,
		})
		if existing != nil {
			existing(message)
		}
	}
}

// createOrUpdateBlueprint creates or updates a single blueprint.
// Returns (created, updated, error).
func (i *Importer) createOrUpdateBlueprint(ctx context.Context, bp api.Blueprint, result *Result, updater *BlueprintUpdater) (bool, bool, error) {
	id, _ := bp["identifier"].(string)
	sendBP := bp
	if rels, ok := bp["relations"].(map[string]interface{}); ok && len(rels) > 0 {
		kept, ignored := systemblueprints.FilterManagedRelations(id, rels)
		i.recordRuleResultIgnoredRelations(ignored, result)
		if len(ignored) > 0 {
			sendBP = systemblueprints.BlueprintWithRelations(bp, kept)
		}
	}

	_, err := i.client.CreateBlueprint(ctx, sendBP)
	if err == nil {
		return true, false, nil
	}

	if isConflictError(err) {
		var updateErr error
		if systemblueprints.PrefersPatchUpdate(id) {
			updateErr = updater.Update(ctx, id, sendBP, BlueprintUpdatePATCH)
		} else {
			// Fetch existing blueprint and merge to avoid destroying fields
			// (like relations) that were stripped for Phase 1 ordering.
			existing, fetchErr := i.client.GetBlueprint(ctx, id)
			if fetchErr != nil {
				return false, false, fetchErr
			}
			for k, v := range sendBP {
				existing[k] = v
			}
			existing = api.Blueprint(cleanSystemFields(map[string]interface{}(existing),
				[]string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}))
			updateErr = updater.Update(ctx, id, existing, BlueprintUpdatePUT)
		}
		if updateErr != nil {
			return false, false, updateErr
		}
		return false, true, nil
	}

	return false, false, err
}

// updateBlueprintFieldsDirect updates a blueprint by merging in specific fields.
// This fetches the existing blueprint and merges the new fields, properly handling
// nested maps (like adding new properties to existing calculationProperties).
func (i *Importer) updateBlueprintFieldsDirect(ctx context.Context, id string, fields map[string]interface{}, result *Result, updater *BlueprintUpdater) error {
	existing, err := i.client.GetBlueprint(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to fetch blueprint: %w", err)
	}

	for k, v := range fields {
		if k == "relations" {
			if newMap, ok := v.(map[string]interface{}); ok {
				kept, ignored := systemblueprints.FilterManagedRelations(id, newMap)
				i.recordRuleResultIgnoredRelations(ignored, result)
				if len(kept) == 0 {
					continue
				}
				v = kept
			}
		}
		if newMap, ok := v.(map[string]interface{}); ok {
			if existingMap, ok := existing[k].(map[string]interface{}); ok {
				for itemKey, itemVal := range newMap {
					existingMap[itemKey] = itemVal
				}
				existing[k] = existingMap
			} else {
				existing[k] = v
			}
		} else {
			existing[k] = v
		}
	}

	existing = api.Blueprint(cleanSystemFields(map[string]interface{}(existing),
		[]string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}))

	var updateErr error
	if systemblueprints.PrefersPatchUpdate(id) {
		updateErr = updater.Update(ctx, id, existing, BlueprintUpdatePATCH)
	} else {
		updateErr = updater.Update(ctx, id, existing, BlueprintUpdatePUT)
	}
	if updateErr != nil {
		return fmt.Errorf("failed to update blueprint fields: %w", updateErr)
	}

	return nil
}

// importOtherResources imports non-blueprint resources with bounded concurrency.
func (i *Importer) importOtherResources(ctx context.Context, data *export.Data, opts Options, result *Result) error {
	// Import entities
	if !opts.SkipEntities && shouldImport("entities", opts.IncludeResources) {
		if err := i.ImportEntities(ctx, data.Entities, opts.IncludeRuleResults, result); err != nil {
			return err
		}
	}

	// Import other resources concurrently with bounded concurrency
	pool := NewWorkerPool(DefaultConcurrency)

	// Import scorecards
	if shouldImport("scorecards", opts.IncludeResources) {
		i.importScorecards(ctx, data.Scorecards, result, pool)
	}

	// Import actions
	if shouldImport("actions", opts.IncludeResources) || shouldImport("automations", opts.IncludeResources) {
		i.importActions(ctx, data.Actions, result, pool)
	}

	// Import teams
	if !opts.SkipEntities && shouldImport("teams", opts.IncludeResources) {
		i.importTeams(ctx, data.Teams, result, pool)
	}

	// Import users
	if !opts.SkipEntities && shouldImport("users", opts.IncludeResources) {
		i.importUsers(ctx, data.Users, result, opts.UsersAsDisabled)
	}

	// Import integrations
	if shouldImport("integrations", opts.IncludeResources) {
		i.importIntegrations(ctx, data.Integrations, result, pool)
	}

	pool.Wait()

	// Import pages level-by-level in topological `after` order.
	// Sidebar resources are executed through a shared pipeline so folders and pages
	// can depend on each other via `parent` and `after`.
	if shouldImport("pages", opts.IncludeResources) {
		i.importSidebarPipeline(ctx, PlanSidebarPipeline(data.Folders, data.Pages), result)
	}

	return nil
}

func (i *Importer) importSidebarPipeline(ctx context.Context, pipeline []SidebarPipelineStep, result *Result) {
	for _, step := range pipeline {
		pool := NewWorkerPool(DefaultConcurrency)
		for _, op := range step.Operations {
			op := op
			pool.Go(func() {
				switch op.ResourceType {
				case "folder":
					folderID := op.Identifier
					postedFolder := CleanFolderForCreate(op.Folder)
					if err := i.client.CreateFolder(ctx, postedFolder); err != nil && !isConflictError(err) {
						i.mu.Lock()
						i.errors.Add(err, "folder", folderID)
						i.mu.Unlock()
						return
					}
					i.logFolderCreateMismatch(ctx, folderID, postedFolder)
				case "page":
					i.importPage(ctx, op.Page, result)
				}
			})
		}
		pool.Wait()
	}
}

// ImportEntities imports entities with two-phase bulk approach.
// Phase 1: bulk upsert all entities with relations stripped.
// Phase 2: bulk upsert entities that have relations (upsert=true, entities exist from Phase 1).
func (i *Importer) ImportEntities(ctx context.Context, entities []api.Entity, includeRuleResults bool, result *Result) error {
	if len(entities) == 0 {
		return nil
	}

	// Fetch blueprints to detect those with inherited ownership and build relation target map
	inheritedOwnershipBPs, relationTargets := i.detectInheritedOwnershipBlueprints(ctx)

	// Build set of blueprints that relate to inherited ownership blueprints
	blueprintsToSkip := make(map[string]bool)
	for bpID := range relationTargets {
		if blueprintRelatesToInheritedOwnership(bpID, inheritedOwnershipBPs, relationTargets) {
			blueprintsToSkip[bpID] = true
		}
	}

	// Filter out entities that:
	// 1. Belong to protected system blueprints
	// 2. Belong to blueprints with inherited ownership
	// 3. Belong to blueprints that have relations to inherited ownership blueprints
	filteredEntities := make([]api.Entity, 0, len(entities))
	protectedSkipped := 0
	inheritedOwnershipSkipped := 0
	for _, entity := range entities {
		blueprintID, _ := entity["blueprint"].(string)
		if isProtectedBlueprint(blueprintID, includeRuleResults) {
			protectedSkipped++
			continue
		}
		if inheritedOwnershipBPs[blueprintID] {
			inheritedOwnershipSkipped++
			continue
		}
		// Check if blueprint has relations to inherited ownership blueprints
		if blueprintsToSkip[blueprintID] {
			inheritedOwnershipSkipped++
			continue
		}
		filteredEntities = append(filteredEntities, entity)
	}

	skippedMsg := ""
	if protectedSkipped > 0 || inheritedOwnershipSkipped > 0 {
		parts := []string{}
		if protectedSkipped > 0 {
			parts = append(parts, fmt.Sprintf("%d protected", protectedSkipped))
		}
		if inheritedOwnershipSkipped > 0 {
			parts = append(parts, fmt.Sprintf("%d inherited-ownership", inheritedOwnershipSkipped))
		}
		skippedMsg = fmt.Sprintf(" (skipped %s)", strings.Join(parts, ", "))
	}

	total := len(filteredEntities)

	// Separate entities with and without relations
	entitiesWithRelations := make([]api.Entity, 0)
	for _, entity := range filteredEntities {
		if HasEntityRelations(entity) {
			entitiesWithRelations = append(entitiesWithRelations, entity)
		}
	}

	// Phase 1: Bulk upsert all entities with relations stripped
	i.reportProgress(fmt.Sprintf("Entities Phase 1%s", skippedMsg), 0, total)
	processedCount := 0
	var progressMu sync.Mutex
	successfulEntities := make(map[string]bool)
	var successMu sync.Mutex

	strippedEntities := make([]api.Entity, 0, len(filteredEntities))
	for _, entity := range filteredEntities {
		bp, ok1 := entity["blueprint"].(string)
		id, ok2 := entity["identifier"].(string)
		if !ok1 || !ok2 || bp == "" || id == "" {
			continue
		}
		strippedEntities = append(strippedEntities, StripEntityRelations(entity))
	}

	i.bulkUpsertEntities(ctx, strippedEntities, false, result, successfulEntities, &successMu, "Entities Phase 1", total, &processedCount, &progressMu)

	// Phase 2: Bulk update entities with relations (upsert=true — known to exist from Phase 1)
	if len(entitiesWithRelations) > 0 {
		phase2Total := len(entitiesWithRelations)
		i.reportProgress("Entities Phase 2 (relations)", 0, phase2Total)
		phase2Count := 0
		var phase2ProgressMu sync.Mutex

		// Filter to entities that succeeded in Phase 1
		successfulWithRelations := make([]api.Entity, 0, len(entitiesWithRelations))
		for _, entity := range entitiesWithRelations {
			blueprintID, _ := entity["blueprint"].(string)
			entityID, _ := entity["identifier"].(string)
			key := fmt.Sprintf("%s:%s", blueprintID, entityID)
			successMu.Lock()
			ok := successfulEntities[key]
			successMu.Unlock()
			if ok {
				successfulWithRelations = append(successfulWithRelations, entity)
			}
		}

		// Pass result=nil to skip double-counting (entities were counted in Phase 1)
		i.bulkUpsertEntities(ctx, successfulWithRelations, true, nil, nil, &successMu, "Entities Phase 2 (relations)", phase2Total, &phase2Count, &phase2ProgressMu)
	}

	return nil
}

// processBulkChunk sends one batch of entities for a single blueprint to the bulk endpoint.
// When upsert=false, 409 conflicts are collected and retried with upsert=true.
// result may be nil to skip created/updated counting (used in Phase 2 to avoid double-counting).
func (i *Importer) processBulkChunk(
	ctx context.Context,
	blueprintID string,
	chunk []api.Entity,
	upsert bool,
	result *Result,
	successfulEntities map[string]bool,
	successMu *sync.Mutex,
	phaseName string,
	total int,
	processedCount *int,
	progressMu *sync.Mutex,
) {
	bulkErrs, err := i.client.BulkUpsertEntities(ctx, blueprintID, chunk, upsert)
	if err != nil {
		i.mu.Lock()
		for _, e := range chunk {
			id, _ := e["identifier"].(string)
			i.errors.Add(err, "entity", id)
		}
		i.mu.Unlock()
		progressMu.Lock()
		*processedCount += len(chunk)
		progressMu.Unlock()
		return
	}

	errByID := make(map[string]api.BulkEntityError, len(bulkErrs))
	for _, be := range bulkErrs {
		errByID[be.Identifier] = be
	}

	created := 0
	var conflicts []api.Entity

	for _, entity := range chunk {
		id, _ := entity["identifier"].(string)
		if bErr, failed := errByID[id]; failed {
			if int(bErr.StatusCode) == 409 && !upsert {
				conflicts = append(conflicts, entity)
			} else {
				i.mu.Lock()
				i.errors.Add(fmt.Errorf("%s", bErr.Message), "entity", id)
				i.mu.Unlock()
			}
		} else {
			created++
			if successfulEntities != nil {
				successMu.Lock()
				successfulEntities[fmt.Sprintf("%s:%s", blueprintID, id)] = true
				successMu.Unlock()
			}
		}
	}

	updated := 0
	if len(conflicts) > 0 {
		retryErrs, retryErr := i.client.BulkUpsertEntities(ctx, blueprintID, conflicts, true)
		if retryErr != nil {
			i.mu.Lock()
			for _, e := range conflicts {
				id, _ := e["identifier"].(string)
				i.errors.Add(retryErr, "entity", id)
			}
			i.mu.Unlock()
		} else {
			retryErrByID := make(map[string]api.BulkEntityError, len(retryErrs))
			for _, re := range retryErrs {
				retryErrByID[re.Identifier] = re
			}
			for _, entity := range conflicts {
				id, _ := entity["identifier"].(string)
				if rErr, failed := retryErrByID[id]; failed {
					i.mu.Lock()
					i.errors.Add(fmt.Errorf("%s", rErr.Message), "entity", id)
					i.mu.Unlock()
				} else {
					updated++
					if successfulEntities != nil {
						successMu.Lock()
						successfulEntities[fmt.Sprintf("%s:%s", blueprintID, id)] = true
						successMu.Unlock()
					}
				}
			}
		}
	}

	if result != nil {
		i.mu.Lock()
		result.EntitiesCreated += created
		result.EntitiesUpdated += updated
		i.mu.Unlock()
	}

	progressMu.Lock()
	*processedCount += len(chunk)
	cur := *processedCount
	progressMu.Unlock()

	if cur%100 == 0 || cur >= total {
		i.reportProgress(phaseName, cur, total)
	}
}

// bulkUpsertEntities sends entities to the bulk endpoint in batches, grouped by blueprint.
// result may be nil to skip counting (used in Phase 2).
func (i *Importer) bulkUpsertEntities(
	ctx context.Context,
	entities []api.Entity,
	upsert bool,
	result *Result,
	successfulEntities map[string]bool,
	successMu *sync.Mutex,
	phaseName string,
	total int,
	processedCount *int,
	progressMu *sync.Mutex,
) {
	byBlueprint := make(map[string][]api.Entity)
	for _, e := range entities {
		bp, _ := e["blueprint"].(string)
		if bp != "" {
			byBlueprint[bp] = append(byBlueprint[bp], e)
		}
	}

	pool := NewWorkerPool(EntityConcurrency)

	for blueprint, bpEnts := range byBlueprint {
		bp := blueprint
		ents := bpEnts
		for start := 0; start < len(ents); start += EntityBulkBatchSize {
			end := start + EntityBulkBatchSize
			if end > len(ents) {
				end = len(ents)
			}
			chunk := ents[start:end]
			pool.Go(func() {
				i.processBulkChunk(ctx, bp, chunk, upsert, result, successfulEntities, successMu, phaseName, total, processedCount, progressMu)
			})
		}
	}

	pool.Wait()
}

// importScorecards imports scorecards grouped by blueprint.
func (i *Importer) importScorecards(ctx context.Context, scorecards []api.Scorecard, result *Result, pool *WorkerPool) {
	byBlueprint := make(map[string][]api.Scorecard)
	for _, sc := range scorecards {
		bpID, ok1 := sc["blueprintIdentifier"].(string)
		scID, ok2 := sc["identifier"].(string)
		if !ok1 || !ok2 || bpID == "" || scID == "" {
			i.errors.Add(fmt.Errorf("scorecard is missing identifier or blueprintIdentifier field, skipping"), "scorecard", "<unknown>")
			continue
		}
		cleaned := cleanSystemFields(sc, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id", "blueprint", "blueprintIdentifier"})
		byBlueprint[bpID] = append(byBlueprint[bpID], api.Scorecard(cleaned))
	}

	for bpID, scs := range byBlueprint {
		bpID := bpID
		scs := scs
		pool.Go(func() {
			var toMerge []api.Scorecard
			for _, sc := range scs {
				scID := sc["identifier"].(string)
				_, err := i.client.CreateScorecard(ctx, bpID, sc)

				i.mu.Lock()
				if err == nil {
					result.ScorecardsCreated++
				} else if isConflictError(err) {
					toMerge = append(toMerge, sc)
				} else {
					i.errors.Add(err, "scorecard", scID)
				}
				i.mu.Unlock()
			}

			// Port has no PATCH endpoint for individual scorecards, so we
			// fetch the full set, merge in our updates, and bulk PUT.
			if len(toMerge) > 0 {
				existing, fetchErr := i.client.GetScorecards(ctx, bpID)
				if fetchErr != nil {
					i.mu.Lock()
					i.errors.Add(fetchErr, "scorecard", fmt.Sprintf("fetch:%s", bpID))
					i.mu.Unlock()
					return
				}

				mergeSet := make(map[string]api.Scorecard, len(toMerge))
				for _, sc := range toMerge {
					mergeSet[sc["identifier"].(string)] = sc
				}

				merged := make([]api.Scorecard, 0, len(existing))
				for _, ex := range existing {
					exID, _ := ex["identifier"].(string)
					cleaned := cleanSystemFields(ex, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id", "blueprint", "blueprintIdentifier"})
					if replacement, ok := mergeSet[exID]; ok {
						merged = append(merged, replacement)
						delete(mergeSet, exID)
					} else {
						merged = append(merged, api.Scorecard(cleaned))
					}
				}
				for _, sc := range mergeSet {
					merged = append(merged, sc)
				}

				_, putErr := i.client.UpdateScorecards(ctx, bpID, merged)
				i.mu.Lock()
				if putErr != nil {
					i.errors.Add(putErr, "scorecard", fmt.Sprintf("bulk-put:%s", bpID))
				} else {
					result.ScorecardsUpdated += len(toMerge)
				}
				i.mu.Unlock()
			}
		})
	}
}

// importActions imports actions/automations.
func (i *Importer) importActions(ctx context.Context, actions []api.Action, result *Result, pool *WorkerPool) {
	for _, action := range actions {
		action := action
		pool.Go(func() {
			actionID, ok := action["identifier"].(string)
			if !ok || actionID == "" {
				return
			}

			cleaned := cleanSystemFields(action, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"})
			apiAction := api.Automation(cleaned)

			_, err := i.client.CreateAutomation(ctx, apiAction)

			i.mu.Lock()
			if err == nil {
				result.ActionsCreated++
			} else if isConflictError(err) {
				_, updateErr := i.client.UpdateAutomation(ctx, actionID, apiAction)
				if updateErr != nil {
					i.errors.Add(updateErr, "action", actionID)
				} else {
					result.ActionsUpdated++
				}
			} else {
				i.errors.Add(err, "action", actionID)
			}
			i.mu.Unlock()
		})
	}
}

// sanitizeTeamFields removes nil-valued fields from a team map before sending
// to the API. Some fields (e.g. description) exported as null from the source
// org cause invalid_request errors on upsert even though the API stores null
// internally. Omitting the field avoids the validation error.
func sanitizeTeamFields(team api.Team) api.Team {
	result := make(api.Team, len(team))
	for k, v := range team {
		if v != nil {
			result[k] = v
		}
	}
	return result
}

// importTeams imports teams.
func (i *Importer) importTeams(ctx context.Context, teams []api.Team, result *Result, pool *WorkerPool) {
	for _, team := range teams {
		team := team
		pool.Go(func() {
			teamName, ok := team["name"].(string)
			if !ok || teamName == "" {
				return
			}

			sanitized := sanitizeTeamFields(team)
			_, err := i.client.CreateTeam(ctx, sanitized)

			i.mu.Lock()
			if err == nil {
				result.TeamsCreated++
			} else if isConflictError(err) {
				_, updateErr := i.client.UpdateTeam(ctx, teamName, sanitized)
				if updateErr != nil {
					i.errors.Add(updateErr, "team", teamName)
				} else {
					result.TeamsUpdated++
				}
			} else {
				i.errors.Add(err, "team", teamName)
			}
			i.mu.Unlock()
		})
	}
}

// UserBatchSize is the maximum number of _user entities per bulk API call.
const UserBatchSize = 20

// UserStatusForCreate returns the status to set when creating a new user entity.
func UserStatusForCreate(user api.User, usersAsDisabled bool) string {
	if usersAsDisabled {
		userType, _ := user["type"].(string)
		if userType != "ADMIN" {
			return "DISABLED"
		}
	}
	return "STAGED"
}

// UserToEntity converts a User API response to a _user blueprint entity payload.
// Pass statusOverride="" to keep the source status (used for updates).
func UserToEntity(user api.User, statusOverride string) api.Entity {
	email, _ := user["email"].(string)
	firstName, _ := user["firstName"].(string)
	lastName, _ := user["lastName"].(string)

	systemFields := map[string]bool{
		"id": true, "createdAt": true, "updatedAt": true,
		"createdBy": true, "updatedBy": true,
	}
	props := make(map[string]interface{})
	for k, v := range user {
		if !systemFields[k] {
			props[k] = v
		}
	}
	if statusOverride != "" {
		props["status"] = statusOverride
	}

	title := strings.TrimSpace(firstName + " " + lastName)
	if title == "" {
		title = email
	}
	return api.Entity{
		"identifier": email,
		"title":      title,
		"properties": props,
	}
}

// importUsers imports users as _user blueprint entities.
// New users are created with STAGED status (or DISABLED for non-admins when usersAsDisabled is true).
// Existing users are updated with source data as-is.
func (i *Importer) importUsers(ctx context.Context, users []api.User, result *Result, usersAsDisabled bool) {
	// Index by email for conflict resolution
	byEmail := make(map[string]api.User, len(users))
	for _, u := range users {
		if email, ok := u["email"].(string); ok && email != "" {
			byEmail[email] = u
		}
	}

	for start := 0; start < len(users); start += UserBatchSize {
		end := start + UserBatchSize
		if end > len(users) {
			end = len(users)
		}
		batch := users[start:end]

		entities := make([]api.Entity, 0, len(batch))
		for _, u := range batch {
			if email, ok := u["email"].(string); !ok || email == "" {
				continue
			}
			status := UserStatusForCreate(u, usersAsDisabled)
			entities = append(entities, UserToEntity(u, status))
		}
		if len(entities) == 0 {
			continue
		}

		errs, err := i.client.CreateUserEntitiesBulk(ctx, entities, false)
		if err != nil {
			i.mu.Lock()
			for _, e := range entities {
				if email, ok := e["identifier"].(string); ok {
					i.errors.Add(err, "user", email)
				}
			}
			i.mu.Unlock()
			continue
		}

		i.mu.Lock()
		result.UsersCreated += len(entities) - len(errs)
		i.mu.Unlock()

		// Collect conflicting users and re-POST with upsert=true, source data as-is
		var conflictEntities []api.Entity
		var nonConflictErrs []api.BulkEntityError
		for _, be := range errs {
			if int(be.StatusCode) == 409 {
				if orig, ok := byEmail[be.Identifier]; ok {
					conflictEntities = append(conflictEntities, UserToEntity(orig, ""))
				}
			} else {
				nonConflictErrs = append(nonConflictErrs, be)
			}
		}

		for _, be := range nonConflictErrs {
			i.mu.Lock()
			i.errors.Add(fmt.Errorf("%s: %s", be.Error, be.Message), "user", be.Identifier)
			i.mu.Unlock()
		}

		if len(conflictEntities) > 0 {
			updateErrs, updateErr := i.client.CreateUserEntitiesBulk(ctx, conflictEntities, true)
			if updateErr != nil {
				i.mu.Lock()
				for _, e := range conflictEntities {
					if email, ok := e["identifier"].(string); ok {
						i.errors.Add(updateErr, "user", email)
					}
				}
				i.mu.Unlock()
			} else {
				i.mu.Lock()
				result.UsersUpdated += len(conflictEntities) - len(updateErrs)
				i.mu.Unlock()
				for _, be := range updateErrs {
					i.mu.Lock()
					i.errors.Add(fmt.Errorf("%s: %s", be.Error, be.Message), "user", be.Identifier)
					i.mu.Unlock()
				}
			}
		}
	}
}

// isSidebarParentNotFound returns true when Port rejects a page because its parent
// sidebar item does not exist in the target organisation.
func isSidebarParentNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Sidebar item")
}

// IsSidebarParentNotFound is the exported form for use by the migrate package.
func IsSidebarParentNotFound(err error) bool {
	return isSidebarParentNotFound(err)
}

// IsAfterItemNotInParent returns true when Port rejects page creation because the
// `after` sibling item doesn't exist inside the specified parent folder.
func IsAfterItemNotInParent(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "after_item_not_in_parent") ||
		strings.Contains(err.Error(), "is not in the parent folder") ||
		strings.Contains(err.Error(), "Sidebar item with after")
}

// IsAgentIdentifierError returns true when the Port API rejects a request because
// a widget is missing the required agentIdentifier field.
func IsAgentIdentifierError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "agentIdentifier")
}

// isAdditionalPropertyError returns true when Port rejects a request because a field
// is not allowed for that page type (e.g. sidebar/requiredQueryParams on entity pages).
func isAdditionalPropertyError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "must NOT have additional properties")
}

var additionalPropertyPattern = regexp.MustCompile(`additional property: (?:\\\")?([^"\\]+)(?:\\\")?`)

func extractAdditionalProperty(err error) string {
	if err == nil {
		return ""
	}
	matches := additionalPropertyPattern.FindStringSubmatch(err.Error())
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

// IsAdditionalPropertyError is the exported form for use by the migrate package.
func IsAdditionalPropertyError(err error) bool {
	return isAdditionalPropertyError(err)
}

// isInvalidPermissionsError returns true when the Port API rejects a permissions
// PATCH because the payload references relations or properties that don't exist
// on the target blueprint (e.g., orphaned scorecard relations on _rule_result).
func isInvalidPermissionsError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "invalid_permissions")
}

// IsInvalidPermissionsError is the exported form for use by the migrate package.
func IsInvalidPermissionsError(err error) bool {
	return isInvalidPermissionsError(err)
}

// invalidPermBodyPattern extracts the JSON body from the API error string.
// The error format is: "API request to ... failed: 422 ... Body: {JSON}"
var invalidPermBodyPattern = regexp.MustCompile(`(?s)Body: (\{.*\})`)

// ParseInvalidPermissionFields extracts the invalidRelations and
// invalidProperties arrays from an invalid_permissions API error.
// Returns nil slices when the error is not parseable or not an
// invalid_permissions error.
func ParseInvalidPermissionFields(err error) (relations, properties []string) {
	if err == nil {
		return nil, nil
	}
	matches := invalidPermBodyPattern.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return nil, nil
	}
	var body struct {
		Error   string `json:"error"`
		Details struct {
			InvalidRelations  []string `json:"invalidRelations"`
			InvalidProperties []string `json:"invalidProperties"`
		} `json:"details"`
	}
	if json.Unmarshal([]byte(matches[1]), &body) != nil {
		return nil, nil
	}
	if body.Error != "invalid_permissions" {
		return nil, nil
	}
	return body.Details.InvalidRelations, body.Details.InvalidProperties
}

// SanitizePermissions returns a deep copy of perms with the named relation
// and property keys removed. Invalid relations are stripped from top-level
// keys and from entities.updateRelations; invalid properties are stripped
// from top-level keys and from entities.updateProperties.
func SanitizePermissions(perms api.Permissions, invalidRelations, invalidProperties []string) api.Permissions {
	relSet := make(map[string]bool, len(invalidRelations))
	for _, r := range invalidRelations {
		relSet[r] = true
	}
	propSet := make(map[string]bool, len(invalidProperties))
	for _, p := range invalidProperties {
		propSet[p] = true
	}

	// Deep-copy and strip top-level keys
	cleaned := make(api.Permissions, len(perms))
	for k, v := range perms {
		if relSet[k] || propSet[k] {
			continue
		}
		cleaned[k] = v
	}

	// Strip nested relation/property keys inside entities.updateRelations
	// and entities.updateProperties where the API actually validates them.
	entities, ok := cleaned["entities"].(map[string]interface{})
	if !ok {
		return cleaned
	}
	entitiesCopy := make(map[string]interface{}, len(entities))
	for k, v := range entities {
		entitiesCopy[k] = v
	}

	if ur, ok := entitiesCopy["updateRelations"].(map[string]interface{}); ok && len(relSet) > 0 {
		urCopy := make(map[string]interface{}, len(ur))
		for k, v := range ur {
			if !relSet[k] {
				urCopy[k] = v
			}
		}
		entitiesCopy["updateRelations"] = urCopy
	}

	if up, ok := entitiesCopy["updateProperties"].(map[string]interface{}); ok && len(propSet) > 0 {
		upCopy := make(map[string]interface{}, len(up))
		for k, v := range up {
			if !propSet[k] {
				upCopy[k] = v
			}
		}
		entitiesCopy["updateProperties"] = upCopy
	}

	cleaned["entities"] = entitiesCopy
	return cleaned
}

// actionAuditFields are the audit/internal fields that must be stripped before
// sending an action or automation to the Port API.
var actionAuditFields = []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}

// CleanActionForCreate returns a copy of the action with audit fields removed.
func CleanActionForCreate(action api.Action) api.Action {
	return api.Action(cleanSystemFields(map[string]interface{}(action), actionAuditFields))
}

// CleanPageForCreateNoNav is like CleanPageForCreate but also strips navigation
// fields (after, sidebar, parent, section, requiredQueryParams). Used as a fallback
// when the target org is missing the sidebar parent.
func CleanPageForCreateNoNav(page api.Page) api.Page {
	strip := append(pageMetaFields, pageNavFields...)
	cleaned := cleanSystemFields(map[string]interface{}(page), strip)
	if widgets, ok := cleaned["widgets"].([]interface{}); ok {
		cleaned["widgets"] = cleanWidgetsRecursive(widgets)
	}
	return api.Page(cleaned)
}

// MergeWidgetAgentIdentifiers copies agentIdentifier values from existing
// widgets into new widgets so that Port's required-field validation passes.
func MergeWidgetAgentIdentifiers(newWidgets, existingWidgets []interface{}) []interface{} {
	return mergeWidgetAgentIdentifiers(newWidgets, existingWidgets)
}

// SortPagesByAfterDeps is the exported version of sortPagesByAfterDeps for use by migrate.
func SortPagesByAfterDeps(pages []api.Page) []api.Page {
	return sortPagesByAfterDeps(pages)
}

func SortFoldersByAfterLevels(folders []api.Folder) [][]api.Folder {
	pipeline := PlanSidebarPipeline(folders, nil)
	levels := make([][]api.Folder, 0, len(pipeline))
	for _, step := range pipeline {
		level := make([]api.Folder, 0, len(step.Operations))
		for _, op := range step.Operations {
			if op.ResourceType == "folder" {
				level = append(level, op.Folder)
			}
		}
		if len(level) > 0 {
			levels = append(levels, level)
		}
	}
	return levels
}

func PlanSidebarPipeline(folders []api.Folder, pages []api.Page) []SidebarPipelineStep {
	opsByID := make(map[string]SidebarPipelineOperation, len(folders)+len(pages))
	inDegree := make(map[string]int, len(folders)+len(pages))
	dependents := make(map[string][]string, len(folders)+len(pages))

	for _, folder := range folders {
		id, _ := folder["identifier"].(string)
		if id == "" {
			continue
		}
		opsByID[id] = SidebarPipelineOperation{
			ResourceType: "folder",
			Identifier:   id,
			Folder:       folder,
		}
		inDegree[id] = 0
	}
	for _, page := range pages {
		id, _ := page["identifier"].(string)
		if id == "" {
			continue
		}
		opsByID[id] = SidebarPipelineOperation{
			ResourceType: "page",
			Identifier:   id,
			Page:         page,
		}
		inDegree[id] = 0
	}

	addDependency := func(id, dep string) {
		if id == "" || dep == "" || id == dep {
			return
		}
		if _, exists := opsByID[dep]; !exists {
			return
		}
		inDegree[id]++
		dependents[dep] = append(dependents[dep], id)
	}

	for _, folder := range folders {
		id, _ := folder["identifier"].(string)
		if id == "" {
			continue
		}
		deps := make(map[string]bool)
		if parent, _ := folder["parent"].(string); parent != "" {
			deps[parent] = true
		}
		if after, _ := folder["after"].(string); after != "" {
			deps[after] = true
		}
		for dep := range deps {
			addDependency(id, dep)
		}
	}
	for _, page := range pages {
		id, _ := page["identifier"].(string)
		if id == "" {
			continue
		}
		deps := make(map[string]bool)
		if parent, _ := page["parent"].(string); parent != "" {
			deps[parent] = true
		}
		if after, _ := page["after"].(string); after != "" {
			deps[after] = true
		}
		for dep := range deps {
			addDependency(id, dep)
		}
	}

	remaining := make(map[string]bool, len(opsByID))
	for id := range opsByID {
		remaining[id] = true
	}

	var steps []SidebarPipelineStep
	for len(remaining) > 0 {
		readyIDs := make([]string, 0, len(remaining))
		for id := range remaining {
			if inDegree[id] == 0 {
				readyIDs = append(readyIDs, id)
			}
		}
		if len(readyIDs) == 0 {
			for id := range remaining {
				readyIDs = append(readyIDs, id)
			}
		}
		sort.Strings(readyIDs)

		step := SidebarPipelineStep{
			Operations: make([]SidebarPipelineOperation, 0, len(readyIDs)),
		}
		for _, id := range readyIDs {
			step.Operations = append(step.Operations, opsByID[id])
		}
		steps = append(steps, step)

		for _, id := range readyIDs {
			delete(remaining, id)
			for _, dep := range dependents[id] {
				inDegree[dep]--
			}
		}
	}

	return steps
}

func DescribeSidebarPipeline(steps []SidebarPipelineStep) []string {
	lines := make([]string, 0, len(steps))
	for idx, step := range steps {
		var folders []string
		var pages []string
		for _, op := range step.Operations {
			switch op.ResourceType {
			case "folder":
				folders = append(folders, op.Identifier)
			case "page":
				pages = append(pages, op.Identifier)
			}
		}
		sort.Strings(folders)
		sort.Strings(pages)

		parts := make([]string, 0, 2)
		if len(folders) > 0 {
			parts = append(parts, fmt.Sprintf("folders [%s]", strings.Join(folders, ", ")))
		}
		if len(pages) > 0 {
			parts = append(parts, fmt.Sprintf("pages [%s]", strings.Join(pages, ", ")))
		}
		lines = append(lines, fmt.Sprintf("Step %d: %s", idx+1, strings.Join(parts, " | ")))
	}
	return lines
}

// sortPagesByAfterDeps returns pages sorted so that if page B has after=A and A is
// also in the list, A comes before B.
func sortPagesByAfterDeps(pages []api.Page) []api.Page {
	pageSet := make(map[string]bool, len(pages))
	for _, p := range pages {
		if id, ok := p["identifier"].(string); ok {
			pageSet[id] = true
		}
	}

	result := make([]api.Page, 0, len(pages))
	placed := make(map[string]bool, len(pages))
	remaining := make([]api.Page, len(pages))
	copy(remaining, pages)

	for len(remaining) > 0 {
		added := 0
		var next []api.Page
		for _, p := range remaining {
			after, _ := p["after"].(string)
			if !pageSet[after] || placed[after] {
				result = append(result, p)
				if id, ok := p["identifier"].(string); ok {
					placed[id] = true
				}
				added++
			} else {
				next = append(next, p)
			}
		}
		remaining = next
		if added == 0 {
			result = append(result, remaining...)
			break
		}
	}
	return result
}

func CleanFolderForCreate(folder api.Folder) api.Folder {
	cleaned := make(api.Folder)
	for _, key := range []string{"identifier", "title", "after", "parent"} {
		if value, ok := folder[key]; ok && value != nil {
			cleaned[key] = value
		}
	}
	return cleaned
}

// sortPagesByAfterLevels groups pages into levels where all pages within a level
// have no after-dependencies on each other. Pages in the same level can be
// processed concurrently; levels must be processed sequentially.
func sortPagesByAfterLevels(pages []api.Page) [][]api.Page {
	pageSet := make(map[string]bool, len(pages))
	pageByID := make(map[string]api.Page, len(pages))
	for _, p := range pages {
		if id, ok := p["identifier"].(string); ok && id != "" {
			pageSet[id] = true
			pageByID[id] = p
		}
	}

	inDegree := make(map[string]int, len(pages))
	dependents := make(map[string][]string, len(pages))
	for _, p := range pages {
		id, _ := p["identifier"].(string)
		if id == "" {
			continue
		}
		inDegree[id] = 0
	}
	for _, p := range pages {
		id, _ := p["identifier"].(string)
		after, _ := p["after"].(string)
		if id == "" || after == "" || !pageSet[after] {
			continue
		}
		inDegree[id]++
		dependents[after] = append(dependents[after], id)
	}

	remaining := make(map[string]bool, len(pages))
	for id := range inDegree {
		remaining[id] = true
	}

	var levels [][]api.Page
	for len(remaining) > 0 {
		var level []api.Page
		for id := range remaining {
			if inDegree[id] == 0 {
				level = append(level, pageByID[id])
			}
		}
		if len(level) == 0 {
			// Cycle — flush all remaining to break deadlock.
			for id := range remaining {
				level = append(level, pageByID[id])
			}
		}
		for _, p := range level {
			id, _ := p["identifier"].(string)
			delete(remaining, id)
			for _, dep := range dependents[id] {
				inDegree[dep]--
			}
		}
		levels = append(levels, level)
	}
	return levels
}

// importPages imports pages in topological `after` order.
// Pages are grouped into levels: all pages in a level are independent of each
// other (no after-dependency between them) and can run concurrently. Levels are
// processed sequentially so that `after` targets are always present before their
// dependents. This avoids race conditions without a separate second pass.
func (i *Importer) importPages(ctx context.Context, pages []api.Page, result *Result) {
	levels := sortPagesByAfterLevels(pages)
	for _, level := range levels {
		pool := NewWorkerPool(DefaultConcurrency)
		for _, page := range level {
			page := page
			pool.Go(func() {
				i.importPage(ctx, page, result)
			})
		}
		pool.Wait()
	}
}

// importPage imports a single page.
func (i *Importer) importPage(ctx context.Context, page api.Page, result *Result) {
	pageID, ok := page["identifier"].(string)
	if !ok || pageID == "" {
		return
	}

	// metaFields are always stripped (audit metadata, internal IDs).
	metaFields := []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id", "protected"}
	// navFields control sidebar placement.
	navFields := []string{"after", "section", "sidebar", "parent", "requiredQueryParams"}

	// buildPage strips the given extra fields and recursively cleans widget metadata.
	buildPage := func(extra []string) api.Page {
		strip := append(metaFields, extra...)
		cleaned := cleanSystemFields(page, strip)
		if widgets, ok := cleaned["widgets"].([]interface{}); ok {
			cleaned["widgets"] = cleanWidgetsRecursive(widgets)
		}
		return api.Page(cleaned)
	}

	// pageForCreate keeps `type` and sidebar placement fields, but strips
	// requiredQueryParams because Port rejects it for some page types on create.
	pageForCreate := buildPage([]string{"requiredQueryParams"})
	// pageForUpdate keeps navigation fields (including `after`) so Port places the
	// page in the correct sidebar position. `type` is stripped because the PATCH
	// endpoint rejects it. Null string nav fields are stripped to avoid clearing
	// existing values in the target.
	pageForUpdate := buildPage([]string{"type", "requiredQueryParams", "sidebar"})
	for _, field := range navFields {
		if v, exists := pageForUpdate[field]; exists && v == nil {
			delete(pageForUpdate, field)
		}
	}
	var (
		createPosted api.Page
		createdPage  api.Page
		err          error
	)

	createPosted = pageForCreate
	createdPage, err = i.client.CreatePage(ctx, createPosted)

	needsUpdate := false
	i.mu.Lock()
	if err == nil {
		result.PagesCreated++
		i.mu.Unlock()
		i.logPageCreateMismatch(ctx, pageID, pageForCreate, createPosted, createdPage)
		return
	} else if IsAfterItemNotInParent(err) || extractAdditionalProperty(err) != "" {
		createPosted, createdPage, retryErr := i.retryCreatePageWithNarrowFallbacks(ctx, pageForCreate, err)
		if retryErr == nil {
			result.PagesCreated++
			i.mu.Unlock()
			i.logPageCreateMismatch(ctx, pageID, pageForCreate, createPosted, createdPage)
			return
		} else if isConflictError(retryErr) {
			needsUpdate = true
		}
	} else if isConflictError(err) {
		needsUpdate = true
	} else if strings.Contains(err.Error(), "agentIdentifier") {
		// Create failed with agentIdentifier — check if the page already exists.
		existingPage, fetchErr := i.client.GetPage(ctx, pageID)
		if fetchErr == nil && existingPage != nil {
			pageWithoutWidgets := make(api.Page)
			for k, v := range pageForUpdate {
				if k != "widgets" {
					pageWithoutWidgets[k] = v
				}
			}
			_, updateErr := i.client.UpdatePage(ctx, pageID, pageWithoutWidgets)
			if updateErr != nil {
				i.errors.Add(updateErr, "page", pageID)
			} else {
				result.PagesUpdated++
			}
		} else {
			i.errors.Add(err, "page", pageID)
		}
	} else {
		i.errors.Add(err, "page", pageID)
	}

	if needsUpdate {
		// Fetch existing page to preserve fields like agentIdentifier.
		existingPage, fetchErr := i.client.GetPage(ctx, pageID)
		if fetchErr == nil && existingPage != nil {
			if existingWidgets, ok := existingPage["widgets"].([]interface{}); ok {
				if newWidgets, ok := pageForUpdate["widgets"].([]interface{}); ok {
					pageForUpdate["widgets"] = mergeWidgetAgentIdentifiers(newWidgets, existingWidgets)
				}
			}
		}

		_, updateErr := i.client.UpdatePage(ctx, pageID, pageForUpdate)
		if updateErr != nil {
			if IsAfterItemNotInParent(updateErr) || extractAdditionalProperty(updateErr) != "" {
				_, retryErr := i.retryUpdatePageWithNarrowFallbacks(ctx, pageID, pageForUpdate, updateErr)
				if retryErr != nil {
					i.errors.Add(retryErr, "page", pageID)
				} else {
					result.PagesUpdated++
				}
			} else if strings.Contains(updateErr.Error(), "agentIdentifier") {
				// Fetch existing page to merge agentIdentifiers from its widgets, then retry.
				if existingPage, fetchErr := i.client.GetPage(ctx, pageID); fetchErr == nil && existingPage != nil {
					if existingWidgets, ok := existingPage["widgets"].([]interface{}); ok {
						if newWidgets, ok := pageForUpdate["widgets"].([]interface{}); ok {
							pageForUpdate["widgets"] = mergeWidgetAgentIdentifiers(newWidgets, existingWidgets)
						}
					}
				}
				_, retryErr := i.client.UpdatePage(ctx, pageID, pageForUpdate)
				if retryErr != nil {
					// Last resort: update without widgets.
					pageWithoutWidgets := make(api.Page)
					for k, v := range pageForUpdate {
						if k != "widgets" {
							pageWithoutWidgets[k] = v
						}
					}
					_, lastErr := i.client.UpdatePage(ctx, pageID, pageWithoutWidgets)
					if lastErr != nil {
						i.errors.Add(lastErr, "page", pageID)
					} else {
						result.PagesUpdated++
					}
				} else {
					result.PagesUpdated++
				}
			} else {
				i.errors.Add(updateErr, "page", pageID)
			}
		} else {
			result.PagesUpdated++
		}
	}
	i.mu.Unlock()
}

func (i *Importer) logPageCreateMismatch(ctx context.Context, pageID string, intended api.Page, posted api.Page, created api.Page) {
	if !i.verbose {
		return
	}
	actualPage, err := i.client.GetPage(ctx, pageID)
	if err == nil && actualPage != nil {
		created = actualPage
	}

	normalizedIntended := normalizePageForLog(intended)
	normalizedPosted := normalizePageForLog(posted)
	normalizedCreated := normalizePageForLog(created)

	if subsetEqual(normalizedPosted, normalizedCreated) && subsetEqual(normalizedIntended, normalizedCreated) {
		return
	}

	lines := []string{
		fmt.Sprintf("Page create mismatch for %s", pageID),
	}
	if !subsetEqual(normalizedIntended, normalizedPosted) {
		lines = append(lines, fmt.Sprintf("  intended: %s", mustJSON(normalizedIntended)))
	}
	lines = append(lines,
		fmt.Sprintf("  posted: %s", mustJSON(normalizedPosted)),
		fmt.Sprintf("  created: %s", mustJSON(normalizedCreated)),
	)
	i.logLines(lines)
}

func (i *Importer) logFolderCreateMismatch(ctx context.Context, folderID string, posted api.Folder) {
	if !i.verbose {
		return
	}
	folders, err := i.client.GetFolders(ctx)
	if err != nil {
		i.logLines([]string{
			fmt.Sprintf("Folder create mismatch check failed for %s", folderID),
			fmt.Sprintf("  posted: %s", mustJSON(normalizeFolderForLog(posted))),
			fmt.Sprintf("  error: %v", err),
		})
		return
	}

	var actual api.Folder
	for _, folder := range folders {
		if identifier, _ := folder["identifier"].(string); identifier == folderID {
			actual = folder
			break
		}
	}
	if actual == nil {
		i.logLines([]string{
			fmt.Sprintf("Folder create mismatch for %s", folderID),
			fmt.Sprintf("  posted: %s", mustJSON(normalizeFolderForLog(posted))),
			"  created: null",
		})
		return
	}

	normalizedPosted := normalizeFolderForLog(posted)
	normalizedActual := normalizeFolderForLog(actual)
	if subsetEqual(normalizedPosted, normalizedActual) {
		return
	}

	i.logLines([]string{
		fmt.Sprintf("Folder create mismatch for %s", folderID),
		fmt.Sprintf("  posted: %s", mustJSON(normalizedPosted)),
		fmt.Sprintf("  created: %s", mustJSON(normalizedActual)),
	})
}

func (i *Importer) logLines(lines []string) {
	if i.log == nil {
		return
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	for _, line := range lines {
		i.log(line)
	}
}

func normalizePageForLog(page api.Page) api.Page {
	if page == nil {
		return nil
	}
	normalized := make(api.Page)
	for _, key := range []string{"identifier", "title", "type", "after", "parent", "section", "sidebar", "showInSidebar", "requiredQueryParams"} {
		if value, ok := page[key]; ok && value != nil {
			normalized[key] = value
		}
	}
	return normalized
}

func clonePage(page api.Page) api.Page {
	if page == nil {
		return nil
	}

	cloned := make(api.Page, len(page))
	for key, value := range page {
		cloned[key] = value
	}
	return cloned
}

func (i *Importer) retryCreatePageWithNarrowFallbacks(ctx context.Context, base api.Page, initialErr error) (api.Page, api.Page, error) {
	candidate := clonePage(base)
	currentErr := initialErr

	for {
		nextCandidate, changed := removeSingleFailingPageField(candidate, currentErr)
		if !changed {
			return candidate, nil, currentErr
		}

		createdPage, retryErr := i.client.CreatePage(ctx, nextCandidate)
		if retryErr == nil {
			return nextCandidate, createdPage, nil
		}
		if isConflictError(retryErr) {
			return nextCandidate, nil, retryErr
		}

		candidate = nextCandidate
		currentErr = retryErr
	}
}

func (i *Importer) retryUpdatePageWithNarrowFallbacks(ctx context.Context, pageID string, base api.Page, initialErr error) (api.Page, error) {
	candidate := clonePage(base)
	currentErr := initialErr

	for {
		nextCandidate, changed := removeSingleFailingPageField(candidate, currentErr)
		if !changed {
			return candidate, currentErr
		}

		updatedPage, retryErr := i.client.UpdatePage(ctx, pageID, nextCandidate)
		if retryErr == nil {
			return updatedPage, nil
		}

		candidate = nextCandidate
		currentErr = retryErr
	}
}

func removeSingleFailingPageField(page api.Page, err error) (api.Page, bool) {
	candidate := clonePage(page)

	if IsAfterItemNotInParent(err) {
		// Explicitly null out `after` so the PATCH clears any existing invalid
		// value in the target, rather than leaving it unchanged by omission.
		candidate["after"] = nil
		return candidate, true
	}

	if invalidProperty := extractAdditionalProperty(err); invalidProperty != "" {
		if _, exists := candidate[invalidProperty]; exists {
			delete(candidate, invalidProperty)
			return candidate, true
		}
	}

	return page, false
}

func normalizeFolderForLog(folder api.Folder) api.Folder {
	if folder == nil {
		return nil
	}

	normalized := make(api.Folder)
	for _, key := range []string{"identifier", "title", "after", "parent", "sidebar", "section", "showInSidebar"} {
		if value, ok := folder[key]; ok && value != nil {
			normalized[key] = value
		}
	}
	return normalized
}

func subsetEqual(expected, actual interface{}) bool {
	switch actualTyped := actual.(type) {
	case api.Page:
		return subsetEqual(expected, map[string]interface{}(actualTyped))
	case api.Folder:
		return subsetEqual(expected, map[string]interface{}(actualTyped))
	}

	switch expectedTyped := expected.(type) {
	case map[string]interface{}:
		actualTyped, ok := actual.(map[string]interface{})
		if !ok {
			return false
		}
		for key, expectedValue := range expectedTyped {
			actualValue, exists := actualTyped[key]
			if !exists || !subsetEqual(expectedValue, actualValue) {
				return false
			}
		}
		return true
	case api.Page:
		return subsetEqual(map[string]interface{}(expectedTyped), actual)
	case api.Folder:
		return subsetEqual(map[string]interface{}(expectedTyped), actual)
	case []interface{}:
		actualTyped, ok := actual.([]interface{})
		if !ok || len(expectedTyped) != len(actualTyped) {
			return false
		}
		for idx := range expectedTyped {
			if !subsetEqual(expectedTyped[idx], actualTyped[idx]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(expected, actual)
	}
}

func mustJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

// pageMetaFields are the audit/internal fields always stripped before sending a page to Port.
var pageMetaFields = []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id", "protected"}

// pageNavFields control sidebar placement; Port rejects them when the referenced parent
// doesn't exist in the target org.
var pageNavFields = []string{"after", "section", "sidebar", "parent", "requiredQueryParams"}

// CleanPageForCreate returns a copy of page with audit/internal fields removed.
// Sidebar placement fields are preserved, but requiredQueryParams is stripped
// because Port rejects it for some page types on create.
func CleanPageForCreate(page api.Page) api.Page {
	strip := append(pageMetaFields, "requiredQueryParams")
	cleaned := cleanSystemFields(map[string]interface{}(page), strip)
	if widgets, ok := cleaned["widgets"].([]interface{}); ok {
		cleaned["widgets"] = cleanWidgetsRecursive(widgets)
	}
	return api.Page(cleaned)
}

// CleanPageForUpdate returns a copy of page with audit/internal fields and `type`
// removed. Navigation fields are kept so Port can move the page to the correct
// sidebar position, except requiredQueryParams and sidebar which are stripped by
// default because Port rejects them for some page types on update. Nav fields
// that are nil/null are also stripped — sending null would clear the page's
// existing navigation context in Port.
func CleanPageForUpdate(page api.Page) api.Page {
	strip := append(pageMetaFields, "type", "requiredQueryParams", "sidebar")
	cleaned := cleanSystemFields(map[string]interface{}(page), strip)
	for _, field := range pageNavFields {
		if v, exists := cleaned[field]; exists && v == nil {
			delete(cleaned, field)
		}
	}
	if widgets, ok := cleaned["widgets"].([]interface{}); ok {
		cleaned["widgets"] = cleanWidgetsRecursive(widgets)
	}
	return api.Page(cleaned)
}

// CleanPageForUpdateNoNav is the fallback for CleanPageForUpdate when Port rejects
// the update because the parent page doesn't exist in the target org.
func CleanPageForUpdateNoNav(page api.Page) api.Page {
	strip := append(pageMetaFields, append(pageNavFields, "type")...)
	cleaned := cleanSystemFields(map[string]interface{}(page), strip)
	if widgets, ok := cleaned["widgets"].([]interface{}); ok {
		cleaned["widgets"] = cleanWidgetsRecursive(widgets)
	}
	return api.Page(cleaned)
}

// cleanWidgetsRecursive removes system fields from widgets and their nested widgets.
// It also fixes widget configurations that would cause validation errors.
func cleanWidgetsRecursive(widgets []interface{}) []interface{} {
	systemFields := map[string]bool{
		"createdBy": true, "updatedBy": true, "createdAt": true, "updatedAt": true,
	}

	result := make([]interface{}, 0, len(widgets))
	for _, w := range widgets {
		widget, ok := w.(map[string]interface{})
		if !ok {
			result = append(result, w)
			continue
		}

		// Clean system fields from this widget
		cleaned := make(map[string]interface{})
		for k, v := range widget {
			if systemFields[k] {
				continue
			}
			// Recursively clean nested widgets
			if k == "widgets" {
				if nestedWidgets, ok := v.([]interface{}); ok {
					cleaned[k] = cleanWidgetsRecursive(nestedWidgets)
					continue
				}
			}
			// Recursively clean groups (which contain widgets)
			if k == "groups" {
				if groups, ok := v.([]interface{}); ok {
					cleanedGroups := make([]interface{}, 0, len(groups))
					for _, g := range groups {
						if group, ok := g.(map[string]interface{}); ok {
							cleanedGroup := make(map[string]interface{})
							for gk, gv := range group {
								if gk == "widgets" {
									if groupWidgets, ok := gv.([]interface{}); ok {
										cleanedGroup[gk] = cleanWidgetsRecursive(groupWidgets)
										continue
									}
								}
								cleanedGroup[gk] = gv
							}
							cleanedGroups = append(cleanedGroups, cleanedGroup)
						} else {
							cleanedGroups = append(cleanedGroups, g)
						}
					}
					cleaned[k] = cleanedGroups
					continue
				}
			}
			cleaned[k] = v
		}

		// Fix table-entities-explorer widgets that have dataset but no blueprint
		// The API requires either a blueprint property or a blueprint rule in the dataset
		widgetType, _ := cleaned["type"].(string)
		if widgetType == "table-entities-explorer" {
			_, hasBlueprint := cleaned["blueprint"]
			_, hasDataset := cleaned["dataset"]
			if hasDataset && !hasBlueprint {
				// Add empty blueprint to indicate cross-blueprint dataset query
				cleaned["blueprint"] = ""
			}
		}

		result = append(result, cleaned)
	}
	return result
}

// mergeWidgetAgentIdentifiers copies agentIdentifier from existing widgets to new widgets.
// This is needed because the API now requires agentIdentifier on certain widget types,
// but exported data may not have it.
func mergeWidgetAgentIdentifiers(newWidgets, existingWidgets []interface{}) []interface{} {
	// Build a map of existing widgets by ID for quick lookup
	existingByID := make(map[string]map[string]interface{})
	for _, w := range existingWidgets {
		if widget, ok := w.(map[string]interface{}); ok {
			if id, ok := widget["id"].(string); ok && id != "" {
				existingByID[id] = widget
			}
		}
	}

	result := make([]interface{}, 0, len(newWidgets))
	for idx, w := range newWidgets {
		widget, ok := w.(map[string]interface{})
		if !ok {
			result = append(result, w)
			continue
		}

		// Try to find matching existing widget by ID
		var existingWidget map[string]interface{}
		if id, ok := widget["id"].(string); ok && id != "" {
			existingWidget = existingByID[id]
		}
		// Fallback to index-based matching if no ID match
		if existingWidget == nil && idx < len(existingWidgets) {
			if ew, ok := existingWidgets[idx].(map[string]interface{}); ok {
				existingWidget = ew
			}
		}

		// Copy agentIdentifier from existing widget if present and not in new widget
		if existingWidget != nil {
			if agentID, ok := existingWidget["agentIdentifier"]; ok {
				if _, hasAgentID := widget["agentIdentifier"]; !hasAgentID {
					widget["agentIdentifier"] = agentID
				}
			}
		}

		// Recursively merge nested widgets
		if newNestedWidgets, ok := widget["widgets"].([]interface{}); ok {
			var existingNestedWidgets []interface{}
			if existingWidget != nil {
				existingNestedWidgets, _ = existingWidget["widgets"].([]interface{})
			}
			if existingNestedWidgets != nil {
				widget["widgets"] = mergeWidgetAgentIdentifiers(newNestedWidgets, existingNestedWidgets)
			}
		}

		// Recursively merge groups
		if newGroups, ok := widget["groups"].([]interface{}); ok {
			var existingGroups []interface{}
			if existingWidget != nil {
				existingGroups, _ = existingWidget["groups"].([]interface{})
			}
			if existingGroups != nil {
				widget["groups"] = mergeGroupAgentIdentifiers(newGroups, existingGroups)
			}
		}

		result = append(result, widget)
	}
	return result
}

// mergeGroupAgentIdentifiers merges agentIdentifier for widgets within groups.
func mergeGroupAgentIdentifiers(newGroups, existingGroups []interface{}) []interface{} {
	// Build a map of existing groups by title for matching
	existingByTitle := make(map[string]map[string]interface{})
	for _, g := range existingGroups {
		if group, ok := g.(map[string]interface{}); ok {
			if title, ok := group["title"].(string); ok && title != "" {
				existingByTitle[title] = group
			}
		}
	}

	result := make([]interface{}, 0, len(newGroups))
	for idx, g := range newGroups {
		group, ok := g.(map[string]interface{})
		if !ok {
			result = append(result, g)
			continue
		}

		// Try to find matching existing group by title
		var existingGroup map[string]interface{}
		if title, ok := group["title"].(string); ok && title != "" {
			existingGroup = existingByTitle[title]
		}
		// Fallback to index-based matching
		if existingGroup == nil && idx < len(existingGroups) {
			if eg, ok := existingGroups[idx].(map[string]interface{}); ok {
				existingGroup = eg
			}
		}

		// Recursively merge widgets within the group
		if newWidgets, ok := group["widgets"].([]interface{}); ok {
			var existingWidgets []interface{}
			if existingGroup != nil {
				existingWidgets, _ = existingGroup["widgets"].([]interface{})
			}
			if existingWidgets != nil {
				group["widgets"] = mergeWidgetAgentIdentifiers(newWidgets, existingWidgets)
			}
		}

		result = append(result, group)
	}
	return result
}

// importIntegrations imports integrations (update config only).
func (i *Importer) importIntegrations(ctx context.Context, integrations []api.Integration, result *Result, pool *WorkerPool) {
	for _, integration := range integrations {
		integration := integration
		pool.Go(func() {
			integrationID, ok := integration["identifier"].(string)
			if !ok || integrationID == "" {
				i.errors.Add(fmt.Errorf("integration is missing identifier field, skipping"), "integration", "<unknown>")
				return
			}

			// The integration config endpoint expects {"config": {...}} wrapper
			config, ok := integration["config"].(map[string]interface{})
			if !ok || config == nil {
				// No config to update — report so the user knows this integration was skipped
				i.errors.Add(fmt.Errorf("integration has no config field to update, skipping"), "integration", integrationID)
				return
			}

			// Wrap the config in the expected format
			payload := map[string]interface{}{
				"config": config,
			}

			_, err := i.client.UpdateIntegrationConfig(ctx, integrationID, payload)

			i.mu.Lock()
			if err != nil {
				i.errors.Add(err, "integration", integrationID)
			} else {
				result.IntegrationsUpdated++
			}
			i.mu.Unlock()
		})
	}
}

// importPermissions applies blueprint and action permission changes from a DiffResult.
// Permissions are applied after all other resources have been imported so that the
// underlying blueprints, actions, and pages are guaranteed to exist.
// When the API rejects a payload due to orphaned relations or properties (422
// invalid_permissions), the offending keys are stripped and the request is retried.
// Returns the counts of successfully updated permissions and any sanitization warnings.
func (i *Importer) importPermissions(ctx context.Context, diff *DiffResult) (bpUpdated, actionUpdated, pageUpdated int, warnings []string) {
	if diff == nil {
		return
	}

	// Import blueprint permissions
	for _, change := range diff.BlueprintPermissions {
		perms := change.Permissions
		_, err := i.client.UpdateBlueprintPermissions(ctx, change.Identifier, perms)
		if err != nil && isInvalidPermissionsError(err) {
			relations, properties := ParseInvalidPermissionFields(err)
			if len(relations) > 0 || len(properties) > 0 {
				warnings = append(warnings, fmt.Sprintf("Stripped orphaned fields from %s permissions: relations=%v properties=%v", change.Identifier, relations, properties))
				perms = SanitizePermissions(perms, relations, properties)
				_, err = i.client.UpdateBlueprintPermissions(ctx, change.Identifier, perms)
			}
		}
		if err != nil {
			i.errors.Add(fmt.Errorf("failed to update blueprint permissions for %s: %w", change.Identifier, err), "blueprint_permissions", change.Identifier)
		} else {
			bpUpdated++
		}
	}

	// Import action permissions
	for _, change := range diff.ActionPermissions {
		perms := change.Permissions
		_, err := i.client.UpdateActionPermissions(ctx, change.Identifier, perms)
		if err != nil && isInvalidPermissionsError(err) {
			relations, properties := ParseInvalidPermissionFields(err)
			if len(relations) > 0 || len(properties) > 0 {
				warnings = append(warnings, fmt.Sprintf("Stripped orphaned fields from %s action permissions: relations=%v properties=%v", change.Identifier, relations, properties))
				perms = SanitizePermissions(perms, relations, properties)
				_, err = i.client.UpdateActionPermissions(ctx, change.Identifier, perms)
			}
		}
		if err != nil {
			i.errors.Add(fmt.Errorf("failed to update action permissions for %s: %w", change.Identifier, err), "action_permissions", change.Identifier)
		} else {
			actionUpdated++
		}
	}

	// Import page permissions
	for _, change := range diff.PagePermissions {
		perms := change.Permissions
		_, err := i.client.UpdatePagePermissions(ctx, change.Identifier, perms)
		if err != nil && isInvalidPermissionsError(err) {
			relations, properties := ParseInvalidPermissionFields(err)
			if len(relations) > 0 || len(properties) > 0 {
				warnings = append(warnings, fmt.Sprintf("Stripped orphaned fields from %s page permissions: relations=%v properties=%v", change.Identifier, relations, properties))
				perms = SanitizePermissions(perms, relations, properties)
				_, err = i.client.UpdatePagePermissions(ctx, change.Identifier, perms)
			}
		}
		if err != nil {
			i.errors.Add(fmt.Errorf("failed to update page permissions for %s: %w", change.Identifier, err), "page_permissions", change.Identifier)
		} else {
			pageUpdated++
		}
	}
	return
}

// applyDataExclusion filters data in-place before diffing/importing.
// excludeDeep removes the blueprint schema AND all its entities/scorecards/actions.
// excludeSchema removes only the blueprint schema; resources for that blueprint are kept.
func applyDataExclusion(data *export.Data, excludeDeep, excludeSchema []string, skipSystemBlueprints bool, skipSystemBlueprintProperties bool) {
	// Pre-pass: remove system blueprint schemas and their entities (shallow skip).
	// Scorecards, actions, and permissions are kept.
	if skipSystemBlueprints {
		filteredBPs := data.Blueprints[:0:0]
		for _, bp := range data.Blueprints {
			id, _ := bp["identifier"].(string)
			if strings.HasPrefix(id, "_") {
				if !skipSystemBlueprintProperties {
					if patch := systemblueprints.CustomPatch(bp); patch != nil {
						filteredBPs = append(filteredBPs, patch)
					}
				}
				continue
			}
			filteredBPs = append(filteredBPs, bp)
		}
		data.Blueprints = filteredBPs

		filteredEnts := data.Entities[:0:0]
		for _, e := range data.Entities {
			bpID, _ := e["blueprint"].(string)
			if strings.HasPrefix(bpID, "_") {
				continue
			}
			filteredEnts = append(filteredEnts, e)
		}
		data.Entities = filteredEnts
	}

	if len(excludeDeep) == 0 && len(excludeSchema) == 0 {
		return
	}
	deepSet := make(map[string]bool, len(excludeDeep))
	for _, id := range excludeDeep {
		deepSet[id] = true
	}
	schemaSet := make(map[string]bool, len(excludeSchema))
	for _, id := range excludeSchema {
		schemaSet[id] = true
	}

	// Filter blueprints (both deep and schema-only remove the blueprint record)
	filtered := data.Blueprints[:0:0]
	for _, bp := range data.Blueprints {
		id, _ := bp["identifier"].(string)
		if deepSet[id] || schemaSet[id] {
			continue
		}
		filtered = append(filtered, bp)
	}
	data.Blueprints = filtered

	// Filter entities — only deep exclusion removes them
	filteredEntities := data.Entities[:0:0]
	for _, e := range data.Entities {
		bpID, _ := e["blueprint"].(string)
		if deepSet[bpID] {
			continue
		}
		filteredEntities = append(filteredEntities, e)
	}
	data.Entities = filteredEntities

	// Filter scorecards — only deep exclusion removes them
	filteredScorecards := data.Scorecards[:0:0]
	for _, sc := range data.Scorecards {
		bpID, _ := sc["blueprintIdentifier"].(string)
		if deepSet[bpID] {
			continue
		}
		filteredScorecards = append(filteredScorecards, sc)
	}
	data.Scorecards = filteredScorecards

	// Track action IDs for deep-excluded blueprints so we can clean up ActionPermissions
	excludedActionIDs := make(map[string]bool)
	for _, a := range data.Actions {
		bpID, _ := a["blueprint"].(string)
		if deepSet[bpID] {
			if actionID, _ := a["identifier"].(string); actionID != "" {
				excludedActionIDs[actionID] = true
			}
		}
	}

	// Filter actions — only deep exclusion removes them
	filteredActions := data.Actions[:0:0]
	for _, a := range data.Actions {
		bpID, _ := a["blueprint"].(string)
		if deepSet[bpID] {
			continue
		}
		filteredActions = append(filteredActions, a)
	}
	data.Actions = filteredActions

	// Clean up blueprint permissions for deep exclusions
	for id := range deepSet {
		delete(data.BlueprintPermissions, id)
	}

	// Clean up action permissions for deep exclusions
	if data.ActionPermissions != nil {
		for id := range excludedActionIDs {
			delete(data.ActionPermissions, id)
		}
	}
}
