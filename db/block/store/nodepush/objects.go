package block_store_nodepush

import (
	"context"
	"path"
	"sync/atomic"
	"time"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// distrustFor is how long bytes that failed their hash keep reads off the node.
const distrustFor = 10 * time.Minute

// Objects reads ranges through the storage node before the object store below.
// Writes and listings go to the object store alone.
//
// Invariant: the node only ever saves a trip. It lacking a pack, being away or
// being slow costs one bounded wait, and the object store answers. Every block
// read through here is verified against its hash by the pack store, which
// calls DistrustReads if the node's bytes were wrong.
type Objects struct {
	block_store_s3.ObjectStore

	// wait bounds one read from the node.
	wait time.Duration
	// on is the runtime switch; off, reads go to the object store alone.
	on *atomic.Bool
	// client holds what reaches the node, nil until the store mounts its push.
	client *atomic.Pointer[Client]
	// distrustUntil is the Unix time in nanoseconds reads stay off the node.
	distrustUntil atomic.Int64
}

// NewObjects wraps store, waiting at most wait for the node reached through
// client while on is set.
func NewObjects(store block_store_s3.ObjectStore, wait time.Duration, on *atomic.Bool, client *atomic.Pointer[Client]) *Objects {
	return &Objects{ObjectStore: store, wait: wait, on: on, client: client}
}

// DistrustReads keeps reads off the node for a while.
func (o *Objects) DistrustReads() {
	o.distrustUntil.Store(time.Now().Add(distrustFor).UnixNano())
}

// GetObjectRange reads the range from the node, or the object store below.
func (o *Objects) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Ask the node, while it is trusted and connected.
	client := o.client.Load()
	if client != nil && o.on.Load() && time.Now().UnixNano() >= o.distrustUntil.Load() {
		readCtx, cancel := context.WithTimeout(ctx, o.wait)
		data, ok := client.ReadRange(readCtx, path.Base(key), off, length)
		cancel()
		if ok {
			return data, nil
		}
	}

	// Anything else reads the object store.
	return o.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
}

// _ are type assertions
var (
	_ block_store_s3.ObjectStore    = (*Objects)(nil)
	_ block_store_s3.ReadDistruster = (*Objects)(nil)
)
