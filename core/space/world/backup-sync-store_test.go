package space_world

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// backupSyncStore blocks a durability fence but permits an ordered root swap.
// It preserves the real Volume's leased-root contract for the retention test.
type backupSyncStore struct {
	// StoreOps serves the actual fixture blocks and roots.
	block.StoreOps
	// LeasedRoots keeps PendingStore's real staging and pinning behavior.
	block.LeasedRoots
	// entered reports the first disk-fence wait.
	entered chan struct{}
	// resume releases every simulated disk-fence wait.
	resume <-chan struct{}
	// once closes entered once even if several paths try to fence.
	once sync.Once
}

// wait pauses the simulated fence without retaining any fixture store lock.
func (s *backupSyncStore) wait(ctx context.Context) error {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-s.resume:
		return nil
	}
}

// SetRoots distinguishes an ordered root swap from a synchronous durability fence.
func (s *backupSyncStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if !kvtx.IsOrderedCommitContext(ctx) {
		if err := s.wait(ctx); err != nil {
			return err
		}
	}
	return s.StoreOps.SetRoots(ctx, entries, roots)
}

// Sync exposes the durability wait after the ordered swap has released rootMtx.
func (s *backupSyncStore) Sync(ctx context.Context) (bool, error) {
	if err := s.wait(ctx); err != nil {
		return false, err
	}
	return s.StoreOps.Sync(ctx)
}

// _ is a type assertion.
var _ block.StoreOps = (*backupSyncStore)(nil)
