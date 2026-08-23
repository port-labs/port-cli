package import_module

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
	entitystream "github.com/port-labs/port-cli/internal/modules/entity_stream"
)

func TestPartitionEntities_FiltersByBlueprintOptions(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "export.json")
	content := `{
  "entities": [
    {"identifier":"svc-1","blueprint":"service"},
    {"identifier":"hidden-1","blueprint":"hidden"},
    {"identifier":"user-1","blueprint":"_user"}
  ]
}`
	if err := os.WriteFile(inputPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write export: %v", err)
	}

	partitions, err := partitionEntities(inputPath, Options{
		SkipSystemBlueprints: true,
		ExcludeBlueprints:    []string{"hidden"},
	})
	if err != nil {
		t.Fatalf("partitionEntities error: %v", err)
	}
	defer partitions.cleanup()

	list := partitions.list()
	if len(list) != 1 {
		t.Fatalf("expected 1 partition, got %d: %v", len(list), list)
	}
	if list[0].Blueprint != "service" {
		t.Fatalf("expected service partition, got %q", list[0].Blueprint)
	}

	var entities []api.Entity
	if err := forEachPartitionEntity(context.Background(), list[0].Path, func(entity api.Entity) error {
		entities = append(entities, entity)
		return nil
	}); err != nil {
		t.Fatalf("read partition entities: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}
	if entities[0]["identifier"] != "svc-1" {
		t.Fatalf("unexpected entity: %v", entities[0])
	}
}

func TestForEachPartitionEntityRespectsContextCancellation(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "entities.jsonl")
	if err := os.WriteFile(inputPath, []byte(`{"identifier":"svc-1","blueprint":"service"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := forEachPartitionEntity(ctx, inputPath, func(entity api.Entity) error {
		t.Fatalf("yield should not be called after cancellation: %v", entity)
		return nil
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestImportBlueprintEntities_DryRunUsesInjectedSourcesAndFilters(t *testing.T) {
	importer := NewImporter(api.NewClient(api.ClientOpts{}))
	currentSource := entitystream.BlueprintEntitySourceFunc(func(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error {
		if blueprintID != "service" {
			t.Fatalf("expected service current lookup, got %q", blueprintID)
		}
		return yield([]api.Entity{{
			"identifier": "svc-existing",
			"blueprint":  "service",
			"properties": map[string]interface{}{"tier": "gold"},
		}})
	})
	desired := entitystream.EntityIterator(1, func(yield func(api.Entity) error) error {
		for _, entity := range []api.Entity{
			{"identifier": "svc-new", "blueprint": "service"},
			{"identifier": "svc-existing", "blueprint": "service", "properties": map[string]interface{}{"tier": "gold"}},
			{"identifier": "svc-filtered", "blueprint": "service"},
			{"identifier": "pkg-1", "blueprint": "package"},
		} {
			if err := yield(entity); err != nil {
				return err
			}
		}
		return nil
	})

	result := &Result{}
	skipped := 0
	err := importer.ImportBlueprintEntities(
		context.Background(),
		"service",
		desired,
		currentSource,
		EntityStreamOptions{
			EntityIDs: []string{"svc-new", "svc-existing"},
			OnEntitySkipped: func(api.Entity) {
				skipped++
			},
		},
		result,
		true,
		&EntityImportContext{},
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("ImportBlueprintEntities error: %v", err)
	}

	if result.EntitiesCreated != 1 {
		t.Fatalf("expected one created entity, got %d", result.EntitiesCreated)
	}
	if result.EntitiesUpdated != 0 {
		t.Fatalf("expected no updated entities, got %d", result.EntitiesUpdated)
	}
	if skipped != 1 {
		t.Fatalf("expected one skipped entity, got %d", skipped)
	}
}

func TestImportBlueprintEntities_CurrentSource410TreatsTargetAsEmpty(t *testing.T) {
	importer := NewImporter(api.NewClient(api.ClientOpts{}))
	currentSource := entitystream.BlueprintEntitySourceFunc(func(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error {
		return errors.New("API request failed: 410 Gone")
	})
	desired := entitystream.EntityIterator(1, func(yield func(api.Entity) error) error {
		return yield(api.Entity{"identifier": "svc-1", "blueprint": "service"})
	})

	result := &Result{}
	err := importer.ImportBlueprintEntities(
		context.Background(),
		"service",
		desired,
		currentSource,
		EntityStreamOptions{},
		result,
		true,
		&EntityImportContext{},
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("ImportBlueprintEntities error: %v", err)
	}
	if result.EntitiesCreated != 1 {
		t.Fatalf("expected entity to be treated as create after target 410, got %d creates", result.EntitiesCreated)
	}
}

func TestImportBlueprintEntities_ReturnsCurrentSourceError(t *testing.T) {
	importer := NewImporter(api.NewClient(api.ClientOpts{}))
	currentSource := entitystream.BlueprintEntitySourceFunc(func(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error {
		return errors.New("target unavailable")
	})
	desired := entitystream.EntityIterator(1, func(yield func(api.Entity) error) error {
		t.Fatal("desired iterator should not be consumed when current source fails")
		return nil
	})

	err := importer.ImportBlueprintEntities(
		context.Background(),
		"service",
		desired,
		currentSource,
		EntityStreamOptions{},
		&Result{},
		true,
		&EntityImportContext{},
		t.TempDir(),
	)
	if err == nil {
		t.Fatal("expected current source error")
	}
	if !strings.Contains(err.Error(), "service") || !strings.Contains(err.Error(), "target unavailable") {
		t.Fatalf("expected wrapped current source error, got %v", err)
	}
}

func TestImportBlueprintEntities_ReturnsDesiredIteratorError(t *testing.T) {
	importer := NewImporter(api.NewClient(api.ClientOpts{}))
	currentSource := entitystream.BlueprintEntitySourceFunc(func(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error {
		return nil
	})
	wantErr := errors.New("desired failed")
	desired := entitystream.PageIterator(func(ctx context.Context, yield func([]api.Entity) error) error {
		return wantErr
	})

	err := importer.ImportBlueprintEntities(
		context.Background(),
		"service",
		desired,
		currentSource,
		EntityStreamOptions{},
		&Result{},
		false,
		&EntityImportContext{},
		t.TempDir(),
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected desired iterator error, got %v", err)
	}
}
