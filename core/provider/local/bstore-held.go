package provider_local

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_nodepush "github.com/s4wave/spacewave/db/block/store/nodepush"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

const (
	// heldSpillAfter is how long a block waits in memory for the node's report
	// before it spills to the local disk with a queue entry. A report takes
	// seconds to minutes; one this late means the node lost its disk or its
	// upload stalls.
	heldSpillAfter = 10 * time.Minute
	// heldSpillAway is how long the node may stay away before every held
	// block spills: while it is away, the memory set may hold the only copy
	// the local process can reach. The node holds a flushed block on its
	// disk, so heldSpillAfter alone bounds a node that is up and not
	// reporting, which costs memory, not a copy.
	heldSpillAway = 5 * time.Second
	// heldSpillEvery is how often the spill looks.
	heldSpillEvery = time.Second
	// heldSpillBatch bounds the blocks one spill commit writes.
	heldSpillBatch = 256
	// nodeKeyReadTries is how many times a block read asks the node by key
	// before it reports the block not found, and nodeKeyReadWait the least
	// each try waits. The tries back off 50, 100, 150 ms.
	nodeKeyReadTries = 4
	nodeKeyReadWait  = time.Second
)

// heldReads reads the store's memory set before the local store. A block the
// durable path keeps off the disk is held there, and the local store names it
// without its bytes, so every read must look there first.
type heldReads struct {
	block.StoreOps
	held *atomic.Pointer[block_store_nodepush.Held]
}

// get returns the held value of ref, decoded, or nil.
func (r *heldReads) get(ref *block.BlockRef) (*block.StoredBlock, error) {
	// Find it in the memory set, if one is up.
	held := r.held.Load()
	if held == nil || ref.GetEmpty() {
		return nil, nil
	}
	value, ok := held.Get(packfile.BlockKey(ref.GetHash()))
	if !ok {
		return nil, nil
	}

	// Decode and check it.
	stored, err := block.DecodeBlockObject(value)
	if err != nil {
		return nil, errors.Wrap(err, ref.MarshalString())
	}
	if err := ref.VerifyData(stored.Data, false); err != nil {
		return nil, errors.Wrap(err, ref.MarshalString())
	}
	return stored, nil
}

// BeginReadOperation opens a read scope of the local store that still reads
// the memory set first.
func (r *heldReads) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scope, release, err := r.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &heldReads{StoreOps: scope, held: r.held}, release, nil
}

// GetBlock reads the memory set, then the local store.
func (r *heldReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if stored, err := r.get(ref); err != nil || stored != nil {
		return storedData(stored), stored != nil, err
	}
	return r.StoreOps.GetBlock(ctx, ref)
}

// GetStoredBlock reads the memory set, then the local store.
func (r *heldReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if stored, err := r.get(ref); err != nil || stored != nil {
		return stored, err
	}
	return r.StoreOps.GetStoredBlock(ctx, ref)
}

// GetBlockExists reports a held block, or asks the local store.
func (r *heldReads) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if held := r.held.Load(); held != nil && !ref.GetEmpty() {
		if _, ok := held.Get(packfile.BlockKey(ref.GetHash())); ok {
			return true, nil
		}
	}
	return r.StoreOps.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch reports held blocks, and asks the local store for the
// rest.
func (r *heldReads) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Ask the local store.
	exists, err := r.StoreOps.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, err
	}

	// Add the held ones.
	held := r.held.Load()
	if held == nil {
		return exists, nil
	}
	for i, ref := range refs {
		if !exists[i] && !ref.GetEmpty() {
			_, exists[i] = held.Get(packfile.BlockKey(ref.GetHash()))
		}
	}
	return exists, nil
}

// StatBlock stats a held block, or asks the local store.
func (r *heldReads) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if stored, err := r.get(ref); err != nil || stored != nil {
		if err != nil {
			return nil, err
		}
		return &block.BlockStat{Ref: ref, Size: int64(len(stored.Data))}, nil
	}
	return r.StoreOps.StatBlock(ctx, ref)
}

// nodeKeyReads asks the storage node's pack log by key for a block every
// other read path missed. After a restart the memory set is empty, and a block
// the node flushed but has not yet reported is held only there until its
// report is applied.
type nodeKeyReads struct {
	block.StoreOps
	client *atomic.Pointer[block_store_nodepush.Client]
	wait   time.Duration
}

// BeginReadOperation opens a read scope of the other paths that still asks
// the node last.
func (r *nodeKeyReads) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scope, release, err := r.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &nodeKeyReads{StoreOps: scope, client: r.client, wait: r.wait}, release, nil
}

// GetStoredBlock reads the other paths, then the node. A path that ends in
// an error, such as no peer to ask, still asks the node, and keeps its error
// when the node lacks the block too.
func (r *nodeKeyReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read the other paths.
	stored, err := r.StoreOps.GetStoredBlock(ctx, ref)
	if stored != nil || ref.GetEmpty() {
		return stored, err
	}
	if found := r.readNode(ctx, ref); found != nil {
		return found, nil
	}
	return nil, err
}

// readNode reads ref from the node's pack log by key, or returns nil.
func (r *nodeKeyReads) readNode(ctx context.Context, ref *block.BlockRef) *block.StoredBlock {
	return readNodeBlock(ctx, r.client, r.wait, ref)
}

// nodeBeforePeers asks the storage node by key before the peers, whose read
// waits up to peerReadWait for a session. After a restart in durable mode,
// every block flushed and not yet reported is only in the node's pack log,
// so asking the peers first cost each such read seconds.
func nodeBeforePeers(peers func() block.StoreOps, client *atomic.Pointer[block_store_nodepush.Client], wait time.Duration) func() block.StoreOps {
	return func() block.StoreOps {
		store := peers()
		if store == nil {
			return nil
		}
		return &nodeFirstReads{StoreOps: store, client: client, wait: wait}
	}
}

// nodeFirstReads reads the node by key, then the store it wraps.
type nodeFirstReads struct {
	block.StoreOps
	client *atomic.Pointer[block_store_nodepush.Client]
	wait   time.Duration
}

// BeginReadOperation opens a read scope of the wrapped store that still asks
// the node first.
func (r *nodeFirstReads) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scope, release, err := r.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &nodeFirstReads{StoreOps: scope, client: r.client, wait: r.wait}, release, nil
}

// GetStoredBlock asks the node, then the wrapped store.
func (r *nodeFirstReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if found := readNodeBlock(ctx, r.client, r.wait, ref); found != nil {
		return found, nil
	}
	return r.StoreOps.GetStoredBlock(ctx, ref)
}

// GetBlock asks the node, then the wrapped store.
func (r *nodeFirstReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if found := readNodeBlock(ctx, r.client, r.wait, ref); found != nil {
		return found.Data, true, nil
	}
	return r.StoreOps.GetBlock(ctx, ref)
}

// readNodeBlock reads ref from the node's pack log by key within wait, or
// returns nil.
func readNodeBlock(ctx context.Context, client *atomic.Pointer[block_store_nodepush.Client], wait time.Duration, ref *block.BlockRef) *block.StoredBlock {
	// Only a mounted push can ask.
	if ref.GetEmpty() {
		return nil
	}

	// Ask the node, again after a miss: a node with no free read slot, or
	// one slow to answer, misses a block it holds.
	key := packfile.BlockKey(ref.GetHash())
	var value []byte
	for try := range nodeKeyReadTries {
		c := client.Load()
		if c == nil {
			return nil
		}
		rctx, cancel := context.WithTimeout(ctx, max(wait, nodeKeyReadWait))
		data, ok := c.ReadBlock(rctx, key)
		cancel()
		if ok {
			value = data
			break
		}
		if try == nodeKeyReadTries-1 || ctx.Err() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(try+1) * 50 * time.Millisecond):
		}
	}

	// Check what it sends; a bad answer is a miss.
	stored, err := block.DecodeBlockObject(value)
	if err != nil || ref.VerifyData(stored.Data, false) != nil {
		return nil
	}
	return stored
}

// GetBlock reads as GetStoredBlock does.
func (r *nodeKeyReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := r.GetStoredBlock(ctx, ref)
	return storedData(stored), stored != nil, err
}

// storedData returns stored's data, nil for a nil block.
func storedData(stored *block.StoredBlock) []byte {
	if stored == nil {
		return nil
	}
	return stored.Data
}

// runSpill writes to the local disk, with queue entries, the held blocks whose
// report is late, every held block while the node is away or the durable
// switch is off, and drops them from held once the local store holds them
// durably. It runs behind foreground writers until ctx ends.
func runSpill(ctx context.Context, le *logrus.Entry, held *block_store_nodepush.Held, wb *block_store_writeback.Store, durable *atomic.Bool, connected func() bool) {
	// Write behind foreground writers, every heldSpillEvery.
	ctx = kvtx.WithBackgroundWriteContext(ctx)
	ticker := time.NewTicker(heldSpillEvery)
	defer ticker.Stop()
	awaySince := time.Time{}
	for {
		// Wait for the next look.
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Pick what spills: everything once the node is away too long or the
		// switch is off, else the late ones.
		now := time.Now()
		if connected() {
			awaySince = time.Time{}
		} else if awaySince.IsZero() {
			awaySince = now
		}
		before := now.Add(-heldSpillAfter)
		if !durable.Load() || (!awaySince.IsZero() && now.Sub(awaySince) > heldSpillAway) {
			before = now
		}

		// Spill them in batches.
		for ctx.Err() == nil {
			keys, values := held.HeldSince(before, heldSpillBatch)
			if len(keys) == 0 {
				break
			}
			if err := spill(ctx, wb, keys, values); err != nil {
				le.WithError(err).Warn("unable to spill held blocks to the local disk, trying again")
				break
			}
			held.Drop(keys)
		}
	}
}

// spill writes the held values locally with their queue entries and flushes
// them.
func spill(ctx context.Context, wb *block_store_writeback.Store, keys, values [][]byte) error {
	// Rebuild the entries the publications held.
	entries := make([]*block.PutBatchEntry, len(keys))
	for i, key := range keys {
		h, err := packfile.ParseBlockKey(key)
		if err != nil {
			return err
		}
		stored, err := block.DecodeBlockObject(values[i])
		if err != nil {
			return err
		}
		entries[i] = &block.PutBatchEntry{Ref: block.NewBlockRef(h.Clone()), Data: stored.Data, Refs: stored.Refs, RefsKnown: stored.RefsKnown}
	}

	// Write and mark them, then flush.
	if _, err := wb.PutBlockBatch(ctx, entries); err != nil {
		return err
	}
	_, err := wb.Sync(ctx)
	return err
}

// peerSource returns the store's peer reads, which a store whose pushed
// blocks skip its disk reads after asking the node by key: with no peer
// session, a peer read waits for one, so the node must come first.
func (t *bstoreTracker) peerSource(le *logrus.Entry, bucketID string) func() block.StoreOps {
	// A store with no peers to ask reads none.
	conf := t.nodePushConfig()
	if conf != nil && conf.NoPeers {
		return func() block.StoreOps { return nil }
	}

	// One whose pushed blocks skip its disk asks the node first.
	peers := newPeerMissSource(le, func() block.StoreOps { return t.a.getP2PStore(bucketID) })
	if conf != nil && conf.Durable != nil {
		return nodeBeforePeers(peers, &t.nodeClient, conf.ReadWait)
	}
	return peers
}
