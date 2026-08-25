package import_module

import (
	"context"
	"sync"
)

// WorkerPool provides bounded concurrency for parallel operations.
// Unlike errgroup.WithContext, individual task failures don't cancel other tasks.
type WorkerPool struct {
	sem chan struct{}
	wg  sync.WaitGroup
	mu  sync.Mutex
}

// NewWorkerPool creates a worker pool with the specified concurrency limit.
func NewWorkerPool(limit int) *WorkerPool {
	if limit <= 0 {
		limit = 1
	}
	return &WorkerPool{
		sem: make(chan struct{}, limit),
	}
}

// Go submits a task to the worker pool.
// The task will run when a worker slot is available.
// Tasks run in separate goroutines and errors are handled by the task itself.
func (p *WorkerPool) Go(task func()) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.sem <- struct{}{}        // Acquire semaphore
		defer func() { <-p.sem }() // Release semaphore
		task()
	}()
}

// GoWithContext submits a task that respects context cancellation.
// Returns immediately if context is already canceled.
func (p *WorkerPool) GoWithContext(ctx context.Context, task func()) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()

		// Try to acquire semaphore, but respect context cancellation
		select {
		case p.sem <- struct{}{}:
			defer func() { <-p.sem }()
			// Check context again after acquiring
			select {
			case <-ctx.Done():
				return
			default:
				task()
			}
		case <-ctx.Done():
			return
		}
	}()
}

// Wait blocks until all submitted tasks complete.
func (p *WorkerPool) Wait() {
	p.wg.Wait()
}

// BatchProcessor processes items in batches with bounded concurrency.
type BatchProcessor[T any] struct {
	pool       *WorkerPool
	mu         sync.Mutex
	results    []BatchResult[T]
	processed  int
	total      int
	onProgress func(processed, total int)
}

// BatchResult holds the result of processing a single item.
type BatchResult[T any] struct {
	Item  T
	Error error
}

// NewBatchProcessor creates a processor for batch operations.
func NewBatchProcessor[T any](concurrency int) *BatchProcessor[T] {
	return &BatchProcessor[T]{
		pool:    NewWorkerPool(concurrency),
		results: make([]BatchResult[T], 0),
	}
}

// SetProgressCallback sets a callback for progress updates.
func (bp *BatchProcessor[T]) SetProgressCallback(cb func(processed, total int)) {
	bp.onProgress = cb
}

// Process processes all items using the provided function.
// Returns results in the order items were processed (not necessarily submission order).
func (bp *BatchProcessor[T]) Process(items []T, fn func(T) error) []BatchResult[T] {
	bp.total = len(items)
	bp.processed = 0
	bp.results = make([]BatchResult[T], 0, len(items))

	for _, item := range items {
		item := item // Capture for goroutine
		bp.pool.Go(func() {
			err := fn(item)
			bp.mu.Lock()
			bp.results = append(bp.results, BatchResult[T]{Item: item, Error: err})
			bp.processed++
			if bp.onProgress != nil {
				bp.onProgress(bp.processed, bp.total)
			}
			bp.mu.Unlock()
		})
	}

	bp.pool.Wait()
	return bp.results
}

// ProcessWithContext processes items with context cancellation support.
func (bp *BatchProcessor[T]) ProcessWithContext(ctx context.Context, items []T, fn func(T) error) []BatchResult[T] {
	bp.total = len(items)
	bp.processed = 0
	bp.results = make([]BatchResult[T], 0, len(items))

	for _, item := range items {
		item := item
		bp.pool.GoWithContext(ctx, func() {
			err := fn(item)
			bp.mu.Lock()
			bp.results = append(bp.results, BatchResult[T]{Item: item, Error: err})
			bp.processed++
			if bp.onProgress != nil {
				bp.onProgress(bp.processed, bp.total)
			}
			bp.mu.Unlock()
		})
	}

	bp.pool.Wait()
	return bp.results
}

// Concurrency limits for different resource types.
// EntityConcurrency=30 saturates Port's ~33 req/s rate limit ceiling
// without burning retries (tested against 131k+ entity imports).
const (
	BlueprintConcurrency = 5
	EntityConcurrency    = 30
	DefaultConcurrency   = 10
	EntityBulkBatchSize  = 20 // max entities per bulk API call
)
