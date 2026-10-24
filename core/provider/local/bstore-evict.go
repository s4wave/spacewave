package provider_local

import (
	"context"
	"sync"
	"sync/atomic"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/net/hash"
)

// readSetMax bounds the blocks a read set records between eviction passes.
// Reads past it go unrecorded, so their blocks age as if unread.
const readSetMax = 1 << 20

// readSet records the blocks read from the local store since the last
// eviction pass, which the next pass keeps.
type readSet struct {
	// enabled is set while the placement has a resident budget, before the
	// budget reaches the store, so reads after it are recorded.
	enabled atomic.Bool
	// mtx guards hashes.
	mtx sync.Mutex
	// hashes holds the raw hashes of the blocks read.
	hashes map[string]struct{}
}

// add records a read of the block with hash h.
func (r *readSet) add(h *hash.Hash) {
	// Record nothing without a budget.
	if !r.enabled.Load() {
		return
	}

	// Record the hash while the set has room.
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if r.hashes == nil {
		r.hashes = make(map[string]struct{})
	}
	if len(r.hashes) < readSetMax {
		r.hashes[string(h.GetHash())] = struct{}{}
	}
}

// take starts a new set and returns the recorded one as a keep test.
func (r *readSet) take() func(*hash.Hash) bool {
	// Swap in a new set.
	r.mtx.Lock()
	hashes := r.hashes
	r.hashes = nil
	r.mtx.Unlock()

	// Keep the blocks the old set recorded.
	return func(h *hash.Hash) bool {
		_, ok := hashes[string(h.GetHash())]
		return ok
	}
}

// readRecorder records the blocks the local store serves to reads.
type readRecorder struct {
	block.StoreOps
	// reads records the blocks served.
	reads *readSet
}

// GetBlock reads the block and records a hit.
func (r *readRecorder) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := r.StoreOps.GetBlock(ctx, ref)
	if found {
		r.reads.add(ref.GetHash())
	}
	return data, found, err
}

// GetStoredBlock reads the block with its refs and records a hit.
func (r *readRecorder) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := r.StoreOps.GetStoredBlock(ctx, ref)
	if stored != nil {
		r.reads.add(ref.GetHash())
	}
	return stored, err
}

// setPolicy applies the cache settings of the placement. Reads are recorded
// from before the store sees a resident budget, so no read the budget pass
// should keep goes unrecorded.
func (p *placementState) setPolicy(cache *account_settings.BlockStoreCache) {
	policy := uploadPolicy(cache)
	p.reads.enabled.Store(policy.ResidentHigh > 0)
	p.wb.SetPolicy(policy)
}

// runEviction keeps the store's resident bytes under its placement's budget.
// When the resident bytes cross the high mark, a pass drops the uploaded
// blocks not read since the last pass, oldest queued first, down to the low
// mark. Uploads and writes change the status, which starts the next check.
func (p *placementState) runEviction(ctx context.Context) error {
	for {
		// Run a pass while the store is past its budget.
		status, changed := p.wb.GetStatus()
		if want := status.EvictBytes(); want > 0 {
			if _, err := p.evict(ctx, want); err != nil {
				return err
			}
		}

		// Wait for the next status change.
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-changed:
		}
	}
}

// evict drops up to maxBytes of uploaded local values that the backend holds,
// oldest queued first, keeping the blocks read since the last pass. A storage
// move holds passes off through the gate. Returns a zero result without a
// storage backend.
func (p *placementState) evict(ctx context.Context, maxBytes int64) (block_store_writeback.EvictResult, error) {
	// Wait out a storage move.
	var result block_store_writeback.EvictResult
	release, err := p.evictGate.Lock(ctx)
	if err != nil {
		return result, err
	}
	defer release()

	// Evict to the open backend.
	remote := p.remote.Load()
	if remote == nil {
		return result, nil
	}
	return p.wb.Evict(ctx, remote.store, maxBytes, p.reads.take())
}
