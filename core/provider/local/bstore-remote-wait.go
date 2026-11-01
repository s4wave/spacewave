package provider_local

import (
	"context"
	"time"

	"github.com/pkg/errors"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// remoteOpenWait bounds how long a read waits for the placed storage backend
// to open before it fails as unavailable.
const remoteOpenWait = time.Minute

// getRemoteReads returns the backend store serving reads. While the placement
// names a backend that has not opened yet, as just after a start, it returns
// a store whose reads wait for it to open, so a block eviction dropped from
// the disk reads from the backend instead of failing over to peers.
func (t *bstoreTracker) getRemoteReads(p *placementState) block.StoreOps {
	// Serve an open, matching backend without taking the notification lock.
	backend := p.backend.Load()
	if backend == nil {
		return nil
	}
	if remote := t.remote.Load(); remote != nil && remote.backend.EqualVT(backend) {
		return remote.store
	}
	return &pendingRemote{t: t, p: p}
}

// setPlacementBackend publishes a placement change to the backend's waiters.
func (t *bstoreTracker) setPlacementBackend(p *placementState, next *account_settings.StorageBackend) {
	t.remoteBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		prev := p.backend.Swap(next)
		if !prev.EqualVT(next) {
			broadcast()
		}
	})
}

// readBackend returns the current backend or its next change channel. A nil
// store and channel mean the mount is unplaced. State and channel are captured
// under the same lock that publishes placement and remote handle changes.
func (t *bstoreTracker) readBackend(p *placementState) (block.StoreOps, <-chan struct{}) {
	// Match the open handle to the placement before subscribing to its change.
	var store block.StoreOps
	var changed <-chan struct{}
	t.remoteBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		backend := p.backend.Load()
		if backend == nil {
			return
		}
		if remote := t.remote.Load(); remote != nil && remote.backend.EqualVT(backend) {
			store = remote.store
			return
		}
		changed = getWaitCh()
	})
	return store, changed
}

// pendingRemote is the placed backend store before it opens. Each call waits
// up to remoteOpenWait for it, then forwards to it.
type pendingRemote struct {
	t *bstoreTracker
	p *placementState
}

// wait returns the open backend store once it opens.
func (r *pendingRemote) wait(ctx context.Context) (block.StoreOps, error) {
	// Keep one deadline across notifications and backend retries.
	deadline := time.NewTimer(remoteOpenWait)
	defer deadline.Stop()
	for {
		// A remount ends this mount's wait even if the caller stays alive.
		select {
		case <-r.p.done:
			return nil, errors.Wrap(block.ErrUnavailable, "block store unmounted")
		default:
		}

		// Capture the backend and its notification without a missed transition.
		remote, changed := r.t.readBackend(r.p)
		if remote != nil {
			return remote, nil
		}
		if changed == nil {
			return nil, errors.Wrap(block.ErrUnavailable, "store left its storage backend")
		}

		// Wait outside the publication lock for the source or caller to change.
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-r.p.done:
			return nil, errors.Wrap(block.ErrUnavailable, "block store unmounted")
		case <-deadline.C:
			return nil, errors.Wrap(block.ErrUnavailable, "storage backend did not open")
		case <-changed:
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
