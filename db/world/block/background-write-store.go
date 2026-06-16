package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// backgroundWriteStore routes foreground block writes to a self-buffered store's
// background-priority intake.
//
// In the single-writer deferred-durability path a self-buffered store (e.g.
// blockshard) already owns its pending buffer and Sync fence, so world commits
// do not need an in-process BufferedStore. Routing PutBlock to the store's
// background intake makes each commit enqueue to the pending buffer instead of
// blocking on a synchronous per-commit publish; reads still resolve through the
// pending buffer, and Sync fences the buffered writes durable alongside the
// deferred head. The background put returns the computed ref before the write is
// durable, so commit-time ref validation still holds and a real write error
// surfaces at the Sync fence, exactly like the BufferedStore path.
//
// Every other operation, including the Sync barrier and the GC defer-flush
// scope, forwards unchanged to the inner store.
type backgroundWriteStore struct {
	block.StoreOps
}

// newBackgroundWriteStore wraps a self-buffered store so commit writes enqueue
// at background priority.
func newBackgroundWriteStore(inner block.StoreOps) backgroundWriteStore {
	return backgroundWriteStore{StoreOps: inner}
}

// PutBlock enqueues the write at background priority on the inner store.
func (s backgroundWriteStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return s.PutBlockBackground(ctx, data, opts)
}

// BeginDeferFlush forwards the GC defer-flush scope to the inner store.
func (s backgroundWriteStore) BeginDeferFlush() {
	block.BeginDeferFlush(s.StoreOps)
}

// EndDeferFlush forwards closing the GC defer-flush scope to the inner store.
func (s backgroundWriteStore) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, s.StoreOps)
}

// _ is a type assertion
var (
	_ block.StoreOps     = backgroundWriteStore{}
	_ block.DeferFlusher = backgroundWriteStore{}
)
