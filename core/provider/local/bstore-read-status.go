package provider_local

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// ReadStatus counts a placed store's block reads by where they were served:
// the local store, or the storage backend, whose pack store tells the storage
// node's answers from the bucket's. A read a fill cache served counts in
// neither; the cache counts its own. It also counts the peer reads by outcome
// and the blocks they filled.
type ReadStatus struct {
	// Local counts the blocks the local store served.
	Local uint64
	// Backend counts the open backend's reads by source, zero when the
	// backend is no pack store. It starts again from zero when the backend
	// reopens.
	Backend block_store_s3.ReadCounts
	// PeerServed counts the peer reads a peer served the block to.
	PeerServed uint64
	// PeerNotHeld counts the peer reads every peer asked answered that it
	// lacks the block.
	PeerNotHeld uint64
	// PeerFailed counts the peer reads that failed, outlived the peer read
	// bound or were canceled.
	PeerFailed uint64
	// PeerFilled counts the blocks a peer served that filled the local store.
	// A block served without its refs fills later, counted in
	// FallbackStatus.Filled instead.
	PeerFilled uint64
}

// readCounter is a backend that counts its block reads by source.
type readCounter interface {
	// ReadCounts returns the reads by source.
	ReadCounts() block_store_s3.ReadCounts
}

// readStatus returns the placement's read counts.
func (p *placementState) readStatus() ReadStatus {
	st := ReadStatus{
		Local:       p.localHits.Load(),
		PeerServed:  p.peerReads.served.Load(),
		PeerNotHeld: p.peerReads.notHeld.Load(),
		PeerFailed:  p.peerReads.failed.Load(),
		PeerFilled:  p.peerReads.filled.Load(),
	}
	if remote := p.remote.Load(); remote != nil {
		if counter, ok := remote.store.(readCounter); ok {
			st.Backend = counter.ReadCounts()
		}
	}
	return st
}

// hitCounter counts the blocks a store serves, in its read scopes too.
type hitCounter struct {
	block.StoreOps
	// hits counts the blocks served.
	hits *atomic.Uint64
}

// BeginReadOperation opens a read scope that counts its hits too.
func (c *hitCounter) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := c.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &hitCounter{StoreOps: scoped, hits: c.hits}, release, nil
}

// GetBlock reads the block and counts a hit.
func (c *hitCounter) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := c.StoreOps.GetBlock(ctx, ref)
	if found {
		c.hits.Add(1)
	}
	return data, found, err
}

// GetStoredBlock reads the block with its refs and counts a hit.
func (c *hitCounter) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := c.StoreOps.GetStoredBlock(ctx, ref)
	if stored != nil {
		c.hits.Add(1)
	}
	return stored, err
}

// _ is a type assertion
var (
	_ block.StoreOps = (*hitCounter)(nil)
	_ readCounter    = (*block_store_s3.PackStore)(nil)
)
