package provider_local

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// remoteOpenWait bounds how long a read waits for the placed storage backend
// to open before it fails as unavailable.
const remoteOpenWait = time.Minute

// remoteOpenPoll is how often a waiting read checks for the open backend.
const remoteOpenPoll = 50 * time.Millisecond

// getRemoteReads returns the backend store serving reads. While the placement
// names a backend that has not opened yet, as just after a start, it returns
// a store whose reads wait for it to open, so a block eviction dropped from
// the disk reads from the backend instead of failing over to peers.
func (t *bstoreTracker) getRemoteReads(p *placementState) block.StoreOps {
	// Serve the open backend, or none when the store is not placed.
	if remote := t.getRemote(); remote != nil {
		return remote
	}
	if p.backend.Load() == nil {
		return nil
	}
	return &pendingRemote{t: t, p: p}
}

// pendingRemote is the placed backend store before it opens. Each call waits
// up to remoteOpenWait for it, then forwards to it.
type pendingRemote struct {
	t *bstoreTracker
	p *placementState
}

// wait returns the open backend store once it opens.
func (r *pendingRemote) wait(ctx context.Context) (block.StoreOps, error) {
	// Check the backend at each poll until it opens or the wait ends.
	deadline := time.NewTimer(remoteOpenWait)
	defer deadline.Stop()
	poll := time.NewTicker(remoteOpenPoll)
	defer poll.Stop()
	for {
		if remote := r.t.getRemote(); remote != nil {
			return remote, nil
		}
		if r.p.backend.Load() == nil {
			return nil, errors.Wrap(block.ErrUnavailable, "store left its storage backend")
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-deadline.C:
			return nil, errors.Wrap(block.ErrUnavailable, "storage backend did not open")
		case <-poll.C:
		}
	}
}

// GetHashType returns zero: the backend has not opened.
func (r *pendingRemote) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures returns no features: the backend has not opened.
func (r *pendingRemote) GetSupportedFeatures() block.StoreFeature {
	return 0
}

// BeginReadOperation opens a read scope on the backend once it opens.
func (r *pendingRemote) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return nil, nil, err
	}
	return remote.BeginReadOperation(ctx)
}

// PutBlock writes the block to the backend once it opens.
func (r *pendingRemote) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return nil, false, err
	}
	return remote.PutBlock(ctx, data, opts)
}

// PutBlockBatch writes the blocks to the backend once it opens.
func (r *pendingRemote) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return nil, err
	}
	return remote.PutBlockBatch(ctx, entries)
}

// GetBlock reads the block from the backend once it opens.
func (r *pendingRemote) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return nil, false, err
	}
	return remote.GetBlock(ctx, ref)
}

// GetStoredBlock reads the block and its refs from the backend once it opens.
func (r *pendingRemote) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return nil, err
	}
	return remote.GetStoredBlock(ctx, ref)
}

// GetBlockExists checks the backend for the block once it opens.
func (r *pendingRemote) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return false, err
	}
	return remote.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks the backend for the blocks once it opens.
func (r *pendingRemote) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return nil, err
	}
	return remote.GetBlockExistsBatch(ctx, refs)
}

// RmBlock removes the block from the backend once it opens.
func (r *pendingRemote) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	remote, err := r.wait(ctx)
	if err != nil {
		return err
	}
	return remote.RmBlock(ctx, ref)
}

// StatBlock stats the block on the backend once it opens.
func (r *pendingRemote) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return nil, err
	}
	return remote.StatBlock(ctx, ref)
}

// Sync syncs the backend once it opens.
func (r *pendingRemote) Sync(ctx context.Context) (bool, error) {
	remote, err := r.wait(ctx)
	if err != nil {
		return false, err
	}
	return remote.Sync(ctx)
}

// _ is a type assertion.
var _ block.StoreOps = (*pendingRemote)(nil)
