package entity_stream

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/port-labs/port-cli/internal/api"
)

const defaultJSONLBatchSize = 100

// PageIterator yields bounded pages of entities from any source.
type PageIterator func(ctx context.Context, yield func([]api.Entity) error) error

// BlueprintEntitySource returns entity pages for one blueprint at a time.
type BlueprintEntitySource interface {
	ForEachEntity(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error
}

// BlueprintEntitySourceFunc adapts a function into a BlueprintEntitySource.
type BlueprintEntitySourceFunc func(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error

func (f BlueprintEntitySourceFunc) ForEachEntity(ctx context.Context, blueprintID string, yield func([]api.Entity) error) error {
	return f(ctx, blueprintID, yield)
}

// FromAPI wraps any API client-like type that exposes ForEachEntity.
func FromAPI(client BlueprintEntitySource) BlueprintEntitySource {
	return client
}

// BlueprintIterator returns a page iterator scoped to one blueprint.
func BlueprintIterator(source BlueprintEntitySource, blueprintID string) PageIterator {
	return func(ctx context.Context, yield func([]api.Entity) error) error {
		return source.ForEachEntity(ctx, blueprintID, yield)
	}
}

// EntityIterator adapts an item callback source into a bounded page iterator.
func EntityIterator(batchSize int, iter func(func(api.Entity) error) error) PageIterator {
	if batchSize <= 0 {
		batchSize = defaultJSONLBatchSize
	}
	return func(ctx context.Context, yield func([]api.Entity) error) error {
		batch := make([]api.Entity, 0, batchSize)
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			if err := yield(batch); err != nil {
				return err
			}
			batch = make([]api.Entity, 0, batchSize)
			return nil
		}
		if err := iter(func(entity api.Entity) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			batch = append(batch, entity)
			if len(batch) >= batchSize {
				return flush()
			}
			return nil
		}); err != nil {
			return err
		}
		return flush()
	}
}

// JSONLPageIterator reads newline-delimited JSON entities in bounded pages.
func JSONLPageIterator(path string, batchSize int) PageIterator {
	return EntityIterator(batchSize, func(yield func(api.Entity) error) error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		dec := json.NewDecoder(bufio.NewReader(file))
		for {
			var entity api.Entity
			if err := dec.Decode(&entity); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			if err := yield(entity); err != nil {
				return err
			}
		}
	})
}

// ForEachEntity calls yield for every entity from iter.
func ForEachEntity(ctx context.Context, iter PageIterator, yield func(api.Entity) error) error {
	return iter(ctx, func(page []api.Entity) error {
		for _, entity := range page {
			if err := yield(entity); err != nil {
				return err
			}
		}
		return nil
	})
}

// CurrentMap builds the current target map for exactly one blueprint.
func CurrentMap(ctx context.Context, source BlueprintEntitySource, blueprintID string) (map[string]api.Entity, error) {
	currentMap := make(map[string]api.Entity)
	err := source.ForEachEntity(ctx, blueprintID, func(currentEntities []api.Entity) error {
		for _, entity := range currentEntities {
			id, _ := entity["identifier"].(string)
			if id != "" {
				currentMap[id] = entity
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get current entities for blueprint %s: %w", blueprintID, err)
	}
	return currentMap, nil
}
