package kvtx

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
)

// orderedCommits tracks ordered commits that no flush has made durable yet, so
// Sync flushes only when one is pending.
type orderedCommits struct {
	// store flushes ordered commits.
	store kvtx.OrderedCommitStore
	// committed counts completed ordered commits.
	committed atomic.Uint64

	// mu serializes flushes.
	mu sync.Mutex
	// synced is the committed count the last flush covered.
	synced uint64
}

// commit commits tx with write ordering when the transaction supports it.
func (o *orderedCommits) commit(ctx context.Context, tx kvtx.Tx) error {
	otx, ok := tx.(kvtx.OrderedCommitTx)
	if !ok {
		return tx.Commit(ctx)
	}
	if err := otx.CommitOrdered(ctx); err != nil {
		return err
	}
	o.committed.Add(1)
	return nil
}

// sync makes every ordered commit completed before the call durable.
func (o *orderedCommits) sync(ctx context.Context) error {
	committed := o.committed.Load()
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.synced >= committed {
		return nil
	}
	if err := o.store.Sync(ctx); err != nil {
		return err
	}
	o.synced = committed
	return nil
}
