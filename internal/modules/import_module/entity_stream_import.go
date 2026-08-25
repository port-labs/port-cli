package import_module

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/port-labs/port-cli/internal/api"
	entitystream "github.com/port-labs/port-cli/internal/modules/entity_stream"
)

type entityPartition struct {
	Blueprint string
	Path      string
}

type entityPartitionWriter struct {
	file    *os.File
	encoder *json.Encoder
}

type entityPartitions struct {
	dir   string
	files map[string]*entityPartitionWriter
	paths map[string]string
}

func newEntityPartitions() (*entityPartitions, error) {
	dir, err := os.MkdirTemp("", "port-cli-import-entities-*")
	if err != nil {
		return nil, err
	}
	return &entityPartitions{
		dir:   dir,
		files: make(map[string]*entityPartitionWriter),
		paths: make(map[string]string),
	}, nil
}

func (p *entityPartitions) write(entity api.Entity) error {
	bpID, _ := entity["blueprint"].(string)
	if bpID == "" {
		return nil
	}
	writer, ok := p.files[bpID]
	if !ok {
		path := filepath.Join(p.dir, fmt.Sprintf("%s.jsonl", safePartitionName(bpID)))
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		writer = &entityPartitionWriter{file: file, encoder: json.NewEncoder(file)}
		p.files[bpID] = writer
		p.paths[bpID] = path
	}
	if err := writer.encoder.Encode(entity); err != nil {
		return err
	}
	return nil
}

func (p *entityPartitions) close() error {
	var err error
	for _, writer := range p.files {
		if closeErr := writer.file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

func (p *entityPartitions) cleanup() error {
	return os.RemoveAll(p.dir)
}

func (p *entityPartitions) list() []entityPartition {
	result := make([]entityPartition, 0, len(p.paths))
	for bpID, path := range p.paths {
		result = append(result, entityPartition{
			Blueprint: bpID,
			Path:      path,
		})
	}
	return result
}

func safePartitionName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "entities"
	}
	return b.String()
}

func partitionEntities(inputPath string, opts Options) (*entityPartitions, error) {
	partitions, err := newEntityPartitions()
	if err != nil {
		return nil, err
	}
	loader := NewStreamLoader()
	deepSet := make(map[string]bool, len(opts.ExcludeBlueprints))
	for _, id := range opts.ExcludeBlueprints {
		deepSet[id] = true
	}
	err = loader.ForEachEntity(inputPath, func(entity api.Entity) error {
		bpID, _ := entity["blueprint"].(string)
		if bpID == "" {
			return nil
		}
		if opts.SkipSystemBlueprints && strings.HasPrefix(bpID, "_") {
			return nil
		}
		if deepSet[bpID] {
			return nil
		}
		return partitions.write(entity)
	})
	if closeErr := partitions.close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		partitions.cleanup()
		return nil, err
	}
	return partitions, nil
}

func forEachPartitionEntity(ctx context.Context, path string, yield func(api.Entity) error) error {
	return entitystream.ForEachEntity(ctx, entitystream.JSONLPageIterator(path, EntityBulkBatchSize), yield)
}

// EntityImportContext holds target blueprint metadata used by entity imports.
type EntityImportContext struct {
	InheritedOwnershipBlueprints map[string]bool
	BlueprintsToSkip             map[string]bool
}

// NewEntityImportContext prepares the target-side metadata used by blueprint-scoped entity imports.
func (i *Importer) NewEntityImportContext(ctx context.Context) *EntityImportContext {
	inheritedOwnershipBPs, relationTargets := i.detectInheritedOwnershipBlueprints(ctx)
	blueprintsToSkip := make(map[string]bool)
	for bpID := range relationTargets {
		if blueprintRelatesToInheritedOwnership(bpID, inheritedOwnershipBPs, relationTargets) {
			blueprintsToSkip[bpID] = true
		}
	}
	return &EntityImportContext{
		InheritedOwnershipBlueprints: inheritedOwnershipBPs,
		BlueprintsToSkip:             blueprintsToSkip,
	}
}

// EntityStreamOptions controls blueprint-scoped entity import from any iterator.
type EntityStreamOptions struct {
	IncludeRuleResults bool
	EntityIDs          []string
	OnEntitySkipped    func(api.Entity)
}

func entityStreamOptionsFromImportOptions(opts Options) EntityStreamOptions {
	return EntityStreamOptions{
		IncludeRuleResults: opts.IncludeRuleResults,
	}
}

func (i *Importer) ImportEntitiesFromStream(ctx context.Context, inputPath string, opts Options, result *Result, dryRun bool) error {
	partitions, err := partitionEntities(inputPath, opts)
	if err != nil {
		return err
	}
	defer partitions.cleanup()

	importCtx := i.NewEntityImportContext(ctx)
	currentSource := entitystream.FromAPI(i.client)

	for _, partition := range partitions.list() {
		iterator := entitystream.JSONLPageIterator(partition.Path, EntityBulkBatchSize)
		if err := i.ImportBlueprintEntities(ctx, partition.Blueprint, iterator, currentSource, entityStreamOptionsFromImportOptions(opts), result, dryRun, importCtx, filepath.Dir(partition.Path)); err != nil {
			return err
		}
	}
	return nil
}

func (i *Importer) importEntityPartition(
	ctx context.Context,
	partition entityPartition,
	opts Options,
	result *Result,
	dryRun bool,
	inheritedOwnershipBPs map[string]bool,
	blueprintsToSkip map[string]bool,
) error {
	importCtx := &EntityImportContext{
		InheritedOwnershipBlueprints: inheritedOwnershipBPs,
		BlueprintsToSkip:             blueprintsToSkip,
	}
	return i.ImportBlueprintEntities(
		ctx,
		partition.Blueprint,
		entitystream.JSONLPageIterator(partition.Path, EntityBulkBatchSize),
		entitystream.FromAPI(i.client),
		entityStreamOptionsFromImportOptions(opts),
		result,
		dryRun,
		importCtx,
		filepath.Dir(partition.Path),
	)
}

// ImportBlueprintEntities imports one blueprint's desired entities from a page iterator.
func (i *Importer) ImportBlueprintEntities(
	ctx context.Context,
	blueprintID string,
	desired entitystream.PageIterator,
	currentSource entitystream.BlueprintEntitySource,
	opts EntityStreamOptions,
	result *Result,
	dryRun bool,
	importCtx *EntityImportContext,
	tempDir string,
) error {
	if blueprintID == "" {
		return nil
	}
	if importCtx == nil {
		importCtx = i.NewEntityImportContext(ctx)
	}
	currentMap, err := entitystream.CurrentMap(ctx, currentSource, blueprintID)
	if err != nil {
		if strings.Contains(err.Error(), "410 Gone") {
			currentMap = make(map[string]api.Entity)
		} else {
			return err
		}
	}

	cleanupTempDir := false
	if tempDir == "" {
		tempDir, err = os.MkdirTemp("", "port-cli-import-entities-*")
		if err != nil {
			return err
		}
		cleanupTempDir = true
	}
	if cleanupTempDir {
		defer os.RemoveAll(tempDir)
	}

	changedFile, err := os.CreateTemp(tempDir, safePartitionName(blueprintID)+"-changed-*.jsonl")
	if err != nil {
		return err
	}
	changedPath := changedFile.Name()
	defer os.Remove(changedPath)
	changedEncoder := json.NewEncoder(changedFile)
	changedCount := 0
	entityIDFilter := stringSet(opts.EntityIDs)

	err = entitystream.ForEachEntity(ctx, desired, func(entity api.Entity) error {
		bpID, _ := entity["blueprint"].(string)
		entityID, _ := entity["identifier"].(string)
		if bpID == "" || entityID == "" {
			return nil
		}
		if bpID != blueprintID {
			return nil
		}
		if len(entityIDFilter) > 0 && !entityIDFilter[entityID] {
			return nil
		}
		if isProtectedBlueprint(bpID, opts.IncludeRuleResults) || importCtx.InheritedOwnershipBlueprints[bpID] || importCtx.BlueprintsToSkip[bpID] {
			return nil
		}
		currentEntity, exists := currentMap[entityID]
		if exists && resourcesEqual(entity, currentEntity, []string{"createdBy", "updatedBy", "createdAt", "updatedAt", "id"}) {
			if opts.OnEntitySkipped != nil {
				opts.OnEntitySkipped(entity)
			}
			return nil
		}
		if dryRun {
			if exists {
				result.EntitiesUpdated++
			} else {
				result.EntitiesCreated++
			}
			return nil
		}
		changedCount++
		return changedEncoder.Encode(entity)
	})
	if closeErr := changedFile.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if dryRun || changedCount == 0 {
		return nil
	}

	i.reportProgress("Entities Phase 1", 0, changedCount)
	processedCount := 0
	var progressMu sync.Mutex
	successfulEntities := make(map[string]bool)
	var successMu sync.Mutex
	if err := i.processChangedEntityFile(ctx, changedPath, false, result, successfulEntities, &successMu, "Entities Phase 1", changedCount, &processedCount, &progressMu); err != nil {
		return err
	}

	relationCount, err := countSuccessfulRelationEntities(ctx, changedPath, successfulEntities, &successMu)
	if err != nil {
		return err
	}
	if relationCount == 0 {
		return nil
	}
	i.reportProgress("Entities Phase 2 (relations)", 0, relationCount)
	phase2Count := 0
	var phase2ProgressMu sync.Mutex
	return i.processChangedEntityFile(ctx, changedPath, true, nil, successfulEntities, &successMu, "Entities Phase 2 (relations)", relationCount, &phase2Count, &phase2ProgressMu)
}

func stringSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func (i *Importer) processChangedEntityFile(
	ctx context.Context,
	path string,
	withRelations bool,
	result *Result,
	successfulEntities map[string]bool,
	successMu *sync.Mutex,
	phaseName string,
	total int,
	processedCount *int,
	progressMu *sync.Mutex,
) error {
	batches := make(map[string][]api.Entity)
	flush := func(bpID string) {
		chunk := batches[bpID]
		if len(chunk) == 0 {
			return
		}
		i.processBulkChunk(ctx, bpID, chunk, withRelations, result, successfulEntities, successMu, phaseName, total, processedCount, progressMu)
		batches[bpID] = nil
	}
	err := forEachPartitionEntity(ctx, path, func(entity api.Entity) error {
		bpID, _ := entity["blueprint"].(string)
		entityID, _ := entity["identifier"].(string)
		if bpID == "" || entityID == "" {
			return nil
		}
		if withRelations {
			successMu.Lock()
			success := successfulEntities[fmt.Sprintf("%s:%s", bpID, entityID)]
			successMu.Unlock()
			if !success || !HasEntityRelations(entity) {
				return nil
			}
		} else {
			entity = StripEntityRelations(entity)
		}
		batches[bpID] = append(batches[bpID], entity)
		if len(batches[bpID]) >= EntityBulkBatchSize {
			flush(bpID)
		}
		return nil
	})
	for bpID := range batches {
		flush(bpID)
	}
	return err
}

func countSuccessfulRelationEntities(ctx context.Context, path string, successfulEntities map[string]bool, successMu *sync.Mutex) (int, error) {
	count := 0
	err := forEachPartitionEntity(ctx, path, func(entity api.Entity) error {
		if !HasEntityRelations(entity) {
			return nil
		}
		bpID, _ := entity["blueprint"].(string)
		entityID, _ := entity["identifier"].(string)
		successMu.Lock()
		success := successfulEntities[fmt.Sprintf("%s:%s", bpID, entityID)]
		successMu.Unlock()
		if success {
			count++
		}
		return nil
	})
	return count, err
}
