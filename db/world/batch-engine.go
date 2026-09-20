package world

import (
	"context"

	"github.com/pkg/errors"
)

// BatchEngine composes sequential transaction clients into one atomic write.
// Only calls carrying Run's context join the batch. Other callers continue to
// use the underlying Engine. A batch and its transaction handles must not escape
// Run or be used concurrently. Publish external effects after Run succeeds.
type BatchEngine struct {
	// Engine supplies storage and transactions outside an active batch.
	Engine
}

// NewBatchEngine wraps an engine without opening a transaction.
func NewBatchEngine(engine Engine) *BatchEngine { return &BatchEngine{Engine: engine} }

// Run executes sequential clients against one lazily opened write transaction.
// Any discarded write aborts the entire batch, even if its caller ignores the
// error. An empty batch performs no storage operation. Run never retries fn.
func (e *BatchEngine) Run(ctx context.Context, fn func(context.Context) error) error {
	// Nested batches share their enclosing transaction and completion boundary.
	if ctx.Value(e) != nil {
		return fn(ctx)
	}
	batch := &batchTransaction{}
	defer func() {
		batch.closed = true
		if batch.tx != nil {
			batch.tx.Discard()
		}
	}()

	// Apply the batch before committing its complete state once.
	if err := fn(context.WithValue(ctx, e, batch)); err != nil {
		return err
	}
	if batch.aborted {
		return errors.New("World batch contains a discarded write")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if batch.tx == nil {
		return nil
	}
	return batch.tx.Commit(ctx)
}

// NewTransaction borrows the active batch, or opens an independent transaction.
// Borrowed Commit accepts changes into the batch; only Run makes them durable.
func (e *BatchEngine) NewTransaction(ctx context.Context, write bool) (Tx, error) {
	// Context identity keeps unrelated users of the engine independent.
	batch, _ := ctx.Value(e).(*batchTransaction)
	if batch == nil {
		return e.Engine.NewTransaction(ctx, write)
	}
	if batch.closed || batch.aborted {
		return nil, errors.New("World batch is closed or aborted")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// One write snapshot supplies read-your-writes across all joined clients.
	if batch.tx == nil {
		var err error
		batch.tx, err = e.Engine.NewTransaction(ctx, true)
		if err != nil {
			return nil, err
		}
	}
	return &batchTransactionHandle{WorldState: batch.tx, batch: batch, write: write}, nil
}

// Sync cannot establish durability inside an uncommitted batch.
func (e *BatchEngine) Sync(ctx context.Context) (bool, error) {
	if ctx.Value(e) != nil {
		return false, errors.New("World batch must complete before Sync")
	}
	return e.Engine.Sync(ctx)
}

var _ Engine = (*BatchEngine)(nil)
