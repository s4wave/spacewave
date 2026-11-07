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
// reads and adoption never reach it. Lacking the pack, being away, being slow
// and returning bad bytes all cost at most one bounded wait, and the remote
// answers.
//
// The pack store checks a block's data against its hash. Its refs have no such
// check, so they rest on the node's chunk sums, which catch a damaged disk. That
// trusts the node as a process: it is the operator's own host behind SSH, and
// the sums would not stop one that lied.
type Reads struct {
	// wait bounds one read from the node.
	wait time.Duration
	// on is the runtime switch.
	on *atomic.Bool
	// client holds what reaches the node, nil until the store mounts its push.
	client *atomic.Pointer[Client]
	// quietUntil is the Unix time in nanoseconds before which reads skip a
	// node that timed out.
	quietUntil atomic.Int64
}

// NewReads reads through client while on is set, waiting at most wait.
func NewReads(wait time.Duration, on *atomic.Bool, client *atomic.Pointer[Client]) *Reads {
	return &Reads{wait: wait, on: on, client: client}
}

// ReadCached reads a range of pack id from the node. It reports false for
// anything but the full range.
func (r *Reads) ReadCached(ctx context.Context, id string, off int64, length int) ([]byte, bool) {
	// Ask only a node that is on, connected and not just timed out, about a
	// range it would serve.
	client := r.client.Load()
	if client == nil || !r.on.Load() || length > maxReadBytes || time.Now().UnixNano() < r.quietUntil.Load() {
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
