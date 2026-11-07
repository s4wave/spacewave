package block_store_nodepush

import (
	"context"
	"sync/atomic"
	"time"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// readsQuietFor is how long reads stop waiting for a node that timed out.
const readsQuietFor = 5 * time.Second

// Reads is a pack store's read cache on the storage node.
//
// The node only ever saves a trip to the remote. It answers nothing but the
// bytes of one block, so it can't vouch for the remote: existence checks, index
// reads and adoption never reach it. A pack the node lacks it range-reads from
// the remote beside it, which is near enough to answer well inside the wait.
// Being away, being slow and returning bad bytes all cost at most one bounded
// wait, and the remote answers.
//
// The pack store checks a block's data, which holds its refs, against its hash.
// The refs list stored beside the data is a copy no read path checks, from the
// remote or the node, so a copy on the node carries the same risk as one on the
// remote. The tunnel authenticates the bytes in transit.
type Reads struct {
	// wait bounds one read from the node.
	wait time.Duration
	// on is the node's runtime switch.
	on *atomic.Bool
	// readsOn turns reads off alone while on stays set, or is nil to read
	// whenever on is set.
	readsOn *atomic.Bool
	// client holds what reaches the node, nil until the store mounts its push.
	client *atomic.Pointer[Client]
	// quietUntil is the Unix time in nanoseconds before which reads skip a
	// node that timed out.
	quietUntil atomic.Int64
}

// NewReads reads through client while on and readsOn are set, waiting at most
// wait. A nil readsOn reads whenever on is set.
func NewReads(wait time.Duration, on, readsOn *atomic.Bool, client *atomic.Pointer[Client]) *Reads {
	return &Reads{wait: wait, on: on, readsOn: readsOn, client: client}
}

// enabled reports whether both switches let reads reach the node.
func (r *Reads) enabled() bool {
	return r.on.Load() && (r.readsOn == nil || r.readsOn.Load())
}

// ReadCached reads a range of pack id from the node. It reports false for
// anything but the full range.
func (r *Reads) ReadCached(ctx context.Context, id string, off int64, length int) ([]byte, bool) {
	// Ask only a node that is on, connected and not just timed out, about a
	// range it would serve.
	client := r.client.Load()
	if client == nil || !r.enabled() || length > maxReadBytes || time.Now().UnixNano() < r.quietUntil.Load() {
		return nil, false
	}

	// A timeout quiets the node, so a silent one costs one wait, not one for
	// each block.
	readCtx, cancel := context.WithTimeout(ctx, r.wait)
	defer cancel()
	data, ok := client.ReadRange(readCtx, id, off, length)
	if readCtx.Err() != nil && ctx.Err() == nil {
		r.quietUntil.Store(time.Now().Add(readsQuietFor).UnixNano())
	}

	// An extent is whole or it is nothing.
	return data, ok && len(data) == length
}

// _ is a type assertion
var _ block_store_s3.RangeCache = (*Reads)(nil)
