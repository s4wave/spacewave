package kvtx

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
)

// BlockDomain is a key-value store that keeps blocks beside its records, such
// as payloads in a log, and serves them through its transactions. Each write
// transaction implements BlockTx.
type BlockDomain interface {
	kvtx.Store
	// GetBlockStore returns the block store outside any transaction.
	GetBlockStore() block.StoreOps
	// Maintenance reclaims space the removed blocks held.
	Maintenance(ctx context.Context) error
}

// BlockTx is a write transaction of a BlockDomain. Its block puts and removes
// commit and discard with its records.
type BlockTx interface {
	kvtx.Tx
	// GetBlockStore returns the transaction's block store.
	GetBlockStore() block.StoreOps
	// WroteBlocks reports whether the transaction put or removed a block.
	WroteBlocks() bool
}

// physicalTx is one write transaction of the Volume's store with the block
// store and reference graph that commit and discard with it.
type physicalTx struct {
	// tx is the store transaction.
	tx kvtx.Tx
	// store wraps tx and records whether a record changed.
	store *kvtx.TxStore
	// blocks serves the blocks in tx and records the blocks tx removes.
	blocks *removalRecorder
	// rg is the reference graph in tx.
	rg *block_gc.RefGraph
}

// openPhysical opens a write transaction of the Volume's store. A BlockDomain
// serves the blocks through the transaction; any other store keeps them as
// records. The caller discards tx.
func (v *Volume) openPhysical(ctx context.Context) (*physicalTx, error) {
	// Open the transaction and its record view.
	tx, err := v.kvtxStore.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	p := &physicalTx{tx: tx, store: kvtx.NewTxStore(tx)}
	p.rg = openRefGraph(p.store)

	// Serve blocks from the domain's transaction, or as records. Either view
	// reads tx's uncommitted writes, so neither marks reads durable.
	var blocks block.StoreOps
	if btx, ok := tx.(BlockTx); ok {
		blocks = btx.GetBlockStore()
	} else {
		blocks = block_store_kvtx.NewKVTxBlock(v.kvKey, p.store, v.GetHashType(), v.atomicHashGet)
	}
	p.blocks = &removalRecorder{StoreOps: blocks}
	return p, nil
}

// discard ends the transaction, then reports the blocks it removed to the
// decoded-block cache. Callers defer it, so a committed transaction reports
// its removals after the commit; a discarded one reports removals that never
// happened, which costs only misses.
func (p *physicalTx) discard() {
	p.tx.Discard()
	for _, ref := range p.blocks.take() {
		block.InvalidateRemovedBlock(ref)
	}
}

// removalRecorder serves a physical transaction's blocks and records each
// block the transaction removes: the sweeps, the block passes, the deferred
// edits and the publication tombstones all remove through it.
type removalRecorder struct {
	block.StoreOps
	// mtx guards removed.
	mtx sync.Mutex
	// removed lists the removed blocks.
	removed []*block.BlockRef
}

// RmBlock records and removes the block.
func (r *removalRecorder) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	r.record(ref)
	return r.StoreOps.RmBlock(ctx, ref)
}

// PutBlockBatch records the batch's tombstones and writes the batch.
func (r *removalRecorder) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	for _, entry := range entries {
		if entry != nil && entry.Tombstone {
			r.record(entry.Ref)
		}
	}
	return r.StoreOps.PutBlockBatch(ctx, entries)
}

// BeginDeferFlush forwards the GC defer-flush scope to the inner store.
func (r *removalRecorder) BeginDeferFlush() {
	block.BeginDeferFlush(r.StoreOps)
}

// EndDeferFlush forwards the GC defer-flush scope to the inner store.
func (r *removalRecorder) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, r.StoreOps)
}

// record appends ref to the removed blocks.
func (r *removalRecorder) record(ref *block.BlockRef) {
	r.mtx.Lock()
	r.removed = append(r.removed, ref)
	r.mtx.Unlock()
}

// take returns and clears the removed blocks.
func (r *removalRecorder) take() []*block.BlockRef {
	// Swap the list out under the lock.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	removed := r.removed
	r.removed = nil
	return removed
}

// wrote reports whether the transaction changed a record or a block.
func (p *physicalTx) wrote() bool {
	btx, ok := p.tx.(BlockTx)
	return p.store.Wrote() || (ok && btx.WroteBlocks())
}
