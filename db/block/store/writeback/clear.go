package block_store_writeback

import (
	"sync/atomic"
	"time"
)

// Bounds of the transactions that clear an uploaded batch's markers, as
// ClearBatch describes.
//
// A clear transaction is background work: it takes the store's writer only
// when no foreground writer waits, and on a busy store it waits a quarter of
// a second or more for each turn. The turn then clears what the hold admits,
// so the hold, not the cost of a block, sets the clear's rate: a 20 ms hold
// cleared about 45 blocks for each 0.28 s wait on a loaded production store,
// about 175 blocks a second, below the rate the World writes them.
const (
	// clearHoldDefault is the hold of one clear transaction by default.
	clearHoldDefault = 100 * time.Millisecond
	// clearHoldMax is the most a clear transaction may hold the writer
	// whatever ClearBatch sets, so a mistyped knob cannot hold a player's
	// write behind it for seconds.
	clearHoldMax = 500 * time.Millisecond
	// clearBlocksDefault bounds the entries one clear transaction removes
	// by default, and the entries one prefetch warms ahead of the
	// transactions that remove them. An upload batch to a pack store carries
	// over a hundred thousand blocks; cleared in one transaction, it held
	// the store's writer for a minute, and every write of the store, a
	// player's included, waited behind it.
	clearBlocksDefault = 4096
	// clearBlocksMax is the most entries ClearBatch may set.
	clearBlocksMax = 65536
)

// ClearBatch bounds the transactions that clear the markers of uploaded
// blocks, in every store of the process, as SetClearBatch sets them. A zero
// field keeps its default.
type ClearBatch struct {
	// Hold is how long one clear transaction holds the store's writer once
	// it removed one entry, 100 ms by default and 500 ms at most. A slow
	// disk shortens the transaction rather than lengthening the foreground
	// writers' wait. The clear commits there and queues for the writer
	// again, behind every foreground writer that arrived meanwhile. A
	// foreground writer that arrives during a clear waits for up to the hold,
	// plus the block in flight and the commit.
	Hold time.Duration
	// MaxBlocks is the most entries one clear transaction removes, and the
	// entries one prefetch warms before the transactions that remove them,
	// 4096 by default and 65536 at most.
	MaxBlocks int
}

// clearBatchConfig holds the bounds SetClearBatch set, nil for the defaults.
var clearBatchConfig atomic.Pointer[ClearBatch]

// SetClearBatch sets the bounds of the marker clears of every store in the
// process. A clear running reads the hold at its next transaction and the
// block bound at its next prefetch window. A zero or negative field keeps its
// default, and one past its most is lowered to it.
func SetClearBatch(b ClearBatch) {
	b = b.normalize()
	clearBatchConfig.Store(&b)
}

// loadClearBatch returns the bounds in force.
func loadClearBatch() ClearBatch {
	if b := clearBatchConfig.Load(); b != nil {
		return *b
	}
	return ClearBatch{}.normalize()
}

// normalize fills the defaults of the zero or negative fields and lowers each
// past its most.
func (b ClearBatch) normalize() ClearBatch {
	// Fill the defaults, then cap each field at its most.
	if b.Hold <= 0 {
		b.Hold = clearHoldDefault
	}
	if b.MaxBlocks <= 0 {
		b.MaxBlocks = clearBlocksDefault
	}
	b.Hold = min(b.Hold, clearHoldMax)
	b.MaxBlocks = min(b.MaxBlocks, clearBlocksMax)
	return b
}
