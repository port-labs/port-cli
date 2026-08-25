package entity_stream

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/port-labs/port-cli/internal/api"
)

type fakeBlueprintSource struct {
	calls []string
	err   error
	pages map[string][][]api.Entity
}

func (f *fakeBlueprintSource) ForEachEntity(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error {
	f.calls = append(f.calls, blueprintID)
	if f.err != nil {
		return f.err
	}
	for _, page := range f.pages[blueprintID] {
		if err := yield(page); err != nil {
			return err
		}
	}
	return nil
}

func TestFromAPIReturnsProvidedSource(t *testing.T) {
	source := &fakeBlueprintSource{}

	if got := FromAPI(source); got != source {
		t.Fatalf("FromAPI should return the provided source")
	}
}

func TestBlueprintEntitySourceFuncDelegates(t *testing.T) {
	called := false
	source := BlueprintEntitySourceFunc(func(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error {
		called = true
		if blueprintID != "service" {
			t.Fatalf("expected service blueprint, got %q", blueprintID)
		}
		return yield([]api.Entity{{"identifier": "svc-1", "blueprint": "service"}})
	})

	var got []api.Entity
	err := source.ForEachEntity(context.Background(), "service", func(page []api.Entity) error {
		got = append(got, page...)
		return nil
	})
	if err != nil {
		t.Fatalf("ForEachEntity error: %v", err)
	}
	if !called {
		t.Fatal("expected delegated function to be called")
	}
	if len(got) != 1 || got[0]["identifier"] != "svc-1" {
		t.Fatalf("unexpected yielded entities: %v", got)
	}
}

func TestBlueprintIteratorYieldsAllPages(t *testing.T) {
	source := &fakeBlueprintSource{
		pages: map[string][][]api.Entity{
			"service": {
				{{"identifier": "svc-1", "blueprint": "service"}},
				{{"identifier": "svc-2", "blueprint": "service"}},
			},
		},
	}

	var got []string
	err := ForEachEntity(context.Background(), BlueprintIterator(source, "service"), func(entity api.Entity) error {
		got = append(got, entity["identifier"].(string))
		return nil
	})
	if err != nil {
		t.Fatalf("ForEachEntity error: %v", err)
	}

	if !reflect.DeepEqual(got, []string{"svc-1", "svc-2"}) {
		t.Fatalf("unexpected identifiers: %v", got)
	}
	if !reflect.DeepEqual(source.calls, []string{"service"}) {
		t.Fatalf("expected one service call, got %v", source.calls)
	}
}

func TestBlueprintIteratorPropagatesSourceError(t *testing.T) {
	wantErr := errors.New("source failed")
	source := &fakeBlueprintSource{err: wantErr}

	err := BlueprintIterator(source, "service")(context.Background(), func(page []api.Entity) error {
		t.Fatalf("yield should not be called after source error: %v", page)
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected source error, got %v", err)
	}
}

func TestBlueprintIteratorPropagatesYieldError(t *testing.T) {
	wantErr := errors.New("yield failed")
	source := &fakeBlueprintSource{
		pages: map[string][][]api.Entity{
			"service": {{{"identifier": "svc-1", "blueprint": "service"}}},
		},
	}

	err := BlueprintIterator(source, "service")(context.Background(), func(page []api.Entity) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected yield error, got %v", err)
	}
}

func TestEntityIteratorUsesDefaultBatchSizeAndFlushesFinalBatch(t *testing.T) {
	var entities []api.Entity
	for i := 0; i < defaultJSONLBatchSize+1; i++ {
		entities = append(entities, api.Entity{"identifier": i, "blueprint": "service"})
	}

	var pageSizes []int
	err := EntityIterator(0, func(yield func(api.Entity) error) error {
		for _, entity := range entities {
			if err := yield(entity); err != nil {
				return err
			}
		}
		return nil
	})(context.Background(), func(page []api.Entity) error {
		pageSizes = append(pageSizes, len(page))
		return nil
	})
	if err != nil {
		t.Fatalf("EntityIterator error: %v", err)
	}

	if !reflect.DeepEqual(pageSizes, []int{defaultJSONLBatchSize, 1}) {
		t.Fatalf("unexpected page sizes: %v", pageSizes)
	}
}

func TestEntityIteratorDoesNotYieldEmptyPages(t *testing.T) {
	yielded := false
	err := EntityIterator(2, func(yield func(api.Entity) error) error {
		return nil
	})(context.Background(), func(page []api.Entity) error {
		yielded = true
		return nil
	})
	if err != nil {
		t.Fatalf("EntityIterator error: %v", err)
	}
	if yielded {
		t.Fatal("expected no yield for empty iterator")
	}
}

func TestEntityIteratorPropagatesIteratorError(t *testing.T) {
	wantErr := errors.New("iterator failed")

	err := EntityIterator(2, func(yield func(api.Entity) error) error {
		return wantErr
	})(context.Background(), func(page []api.Entity) error {
		t.Fatalf("yield should not be called after iterator error: %v", page)
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected iterator error, got %v", err)
	}
}

func TestEntityIteratorPropagatesYieldError(t *testing.T) {
	wantErr := errors.New("yield failed")

	err := EntityIterator(1, func(yield func(api.Entity) error) error {
		return yield(api.Entity{"identifier": "svc-1", "blueprint": "service"})
	})(context.Background(), func(page []api.Entity) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected yield error, got %v", err)
	}
}

func TestEntityIteratorRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := EntityIterator(1, func(yield func(api.Entity) error) error {
		return yield(api.Entity{"identifier": "svc-1", "blueprint": "service"})
	})(ctx, func(page []api.Entity) error {
		t.Fatalf("yield should not be called after cancellation: %v", page)
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestJSONLPageIteratorYieldsBoundedPages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entities.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jsonl: %v", err)
	}
	enc := json.NewEncoder(file)
	for _, entity := range []api.Entity{
		{"identifier": "svc-1", "blueprint": "service"},
		{"identifier": "svc-2", "blueprint": "service"},
		{"identifier": "svc-3", "blueprint": "service"},
	} {
		if err := enc.Encode(entity); err != nil {
			t.Fatalf("encode jsonl: %v", err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close jsonl: %v", err)
	}

	var pageSizes []int
	var got []string
	err = JSONLPageIterator(path, 2)(context.Background(), func(page []api.Entity) error {
		pageSizes = append(pageSizes, len(page))
		for _, entity := range page {
			got = append(got, entity["identifier"].(string))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("JSONLPageIterator error: %v", err)
	}

	if !reflect.DeepEqual(pageSizes, []int{2, 1}) {
		t.Fatalf("unexpected page sizes: %v", pageSizes)
	}
	if !reflect.DeepEqual(got, []string{"svc-1", "svc-2", "svc-3"}) {
		t.Fatalf("unexpected identifiers: %v", got)
	}
}

func TestJSONLPageIteratorReturnsOpenError(t *testing.T) {
	err := JSONLPageIterator(filepath.Join(t.TempDir(), "missing.jsonl"), 2)(context.Background(), func(page []api.Entity) error {
		t.Fatalf("yield should not be called for missing file: %v", page)
		return nil
	})
	if err == nil {
		t.Fatal("expected missing file error")
	}
}

func TestJSONLPageIteratorReturnsDecodeError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entities.jsonl")
	if err := os.WriteFile(path, []byte(`{"identifier":"svc-1"`+"\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	err := JSONLPageIterator(path, 2)(context.Background(), func(page []api.Entity) error {
		t.Fatalf("yield should not be called for invalid JSON: %v", page)
		return nil
	})
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestJSONLPageIteratorPropagatesYieldError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entities.jsonl")
	if err := os.WriteFile(path, []byte(`{"identifier":"svc-1","blueprint":"service"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
	wantErr := errors.New("yield failed")

	err := JSONLPageIterator(path, 1)(context.Background(), func(page []api.Entity) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected yield error, got %v", err)
	}
}

func TestForEachEntityPropagatesYieldError(t *testing.T) {
	wantErr := errors.New("entity yield failed")

	err := ForEachEntity(context.Background(), func(ctx context.Context, yield func([]api.Entity) error) error {
		return yield([]api.Entity{
			{"identifier": "svc-1", "blueprint": "service"},
			{"identifier": "svc-2", "blueprint": "service"},
		})
	}, func(entity api.Entity) error {
		if entity["identifier"] == "svc-2" {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected entity yield error, got %v", err)
	}
}

func TestCurrentMapBuildsOneBlueprintMap(t *testing.T) {
	source := &fakeBlueprintSource{
		pages: map[string][][]api.Entity{
			"service": {
				{
					{"identifier": "svc-1", "blueprint": "service"},
					{"identifier": "svc-2", "blueprint": "service"},
				},
			},
			"package": {
				{{"identifier": "pkg-1", "blueprint": "package"}},
			},
		},
	}

	current, err := CurrentMap(context.Background(), source, "service")
	if err != nil {
		t.Fatalf("CurrentMap error: %v", err)
	}

	if len(current) != 2 {
		t.Fatalf("expected 2 current entities, got %d", len(current))
	}
	if _, ok := current["pkg-1"]; ok {
		t.Fatalf("current map included another blueprint entity")
	}
	if !reflect.DeepEqual(source.calls, []string{"service"}) {
		t.Fatalf("expected one service call, got %v", source.calls)
	}
}

func TestCurrentMapIgnoresBlankIdentifiersAndOverwritesDuplicates(t *testing.T) {
	source := &fakeBlueprintSource{
		pages: map[string][][]api.Entity{
			"service": {
				{
					{"identifier": "", "blueprint": "service"},
					{"identifier": "svc-1", "blueprint": "service", "title": "old"},
				},
				{
					{"identifier": "svc-1", "blueprint": "service", "title": "new"},
				},
			},
		},
	}

	current, err := CurrentMap(context.Background(), source, "service")
	if err != nil {
		t.Fatalf("CurrentMap error: %v", err)
	}

	if len(current) != 1 {
		t.Fatalf("expected only non-empty identifier entry, got %d", len(current))
	}
	if current["svc-1"]["title"] != "new" {
		t.Fatalf("expected duplicate identifier to be overwritten by last value, got %v", current["svc-1"])
	}
}

func TestCurrentMapWrapsSourceErrorWithBlueprint(t *testing.T) {
	source := &fakeBlueprintSource{err: errors.New("source failed")}

	_, err := CurrentMap(context.Background(), source, "service")
	if err == nil {
		t.Fatal("expected CurrentMap error")
	}
	if !strings.Contains(err.Error(), "service") || !strings.Contains(err.Error(), "source failed") {
		t.Fatalf("expected wrapped blueprint error, got %v", err)
	}
}
