package kvtx

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	hstore "github.com/s4wave/spacewave/db/store"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/hash"
)

func TestStorageStatsSnapshotWakesOnDelete(t *testing.T) {
	t.Parallel()

	deleted := false
	vol := &Volume{
		statsFn: func(context.Context) (*volume.StorageStats, error) {
			if deleted {
				return &volume.StorageStats{}, nil
			}
			return &volume.StorageStats{TotalBytes: 1024, BlockCount: 2}, nil
		},
		deleteFn: func() error {
			deleted = true
			return nil
		},
	}

	stats, waitCh, err := vol.GetStorageStatsSnapshotWithWait(context.Background())
	if err != nil {
		t.Fatalf("GetStorageStatsSnapshotWithWait() error = %v", err)
	}
	if stats.GetTotalBytes() != 1024 || stats.GetBlockCount() != 2 {
		t.Fatalf("stats = %+v, want 1024/2", stats)
	}

	if err := vol.Delete(); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for storage stats delete wakeup")
	}
}

func TestStorageStatsSnapshotWakesOnDirectBlockMutations(t *testing.T) {
	t.Parallel()

	store := &testStatsBlockStore{}
	vol := &Volume{Store: store}

	_, putCh, err := vol.GetStorageStatsSnapshotWithWait(context.Background())
	if err != nil {
		t.Fatalf("GetStorageStatsSnapshotWithWait() error = %v", err)
	}
	_, exists, err := vol.PutBlock(context.Background(), []byte("hello"), nil)
	if err != nil {
		t.Fatalf("PutBlock() error = %v", err)
	}
	if exists {
		t.Fatal("PutBlock() exists = true, want false")
	}
	waitForStorageStatsWake(t, putCh)

	_, rmCh, err := vol.GetStorageStatsSnapshotWithWait(context.Background())
	if err != nil {
		t.Fatalf("GetStorageStatsSnapshotWithWait() error = %v", err)
	}
	if err := vol.RmBlock(context.Background(), &block.BlockRef{}); err != nil {
		t.Fatalf("RmBlock() error = %v", err)
	}
	waitForStorageStatsWake(t, rmCh)

	_, flushCh, err := vol.GetStorageStatsSnapshotWithWait(context.Background())
	if err != nil {
		t.Fatalf("GetStorageStatsSnapshotWithWait() error = %v", err)
	}
	if err := vol.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	waitForStorageStatsWake(t, flushCh)
}

type testStatsBlockStore struct {
	hstore.Store
}

func (s *testStatsBlockStore) Execute(context.Context) error { return nil }

func (s *testStatsBlockStore) GetHashType() hash.HashType { return 0 }

func (s *testStatsBlockStore) GetSupportedFeatures() block.StoreFeature { return 0 }

func (s *testStatsBlockStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

func (s *testStatsBlockStore) PutBlock(
	context.Context,
	[]byte,
	*block.PutOpts,
) (*block.BlockRef, bool, error) {
	return &block.BlockRef{}, false, nil
}

func (s *testStatsBlockStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return nil
}

func (s *testStatsBlockStore) PutBlockBackground(
	context.Context,
	[]byte,
	*block.PutOpts,
) (*block.BlockRef, bool, error) {
	return &block.BlockRef{}, false, nil
}

func (s *testStatsBlockStore) GetBlock(
	context.Context,
	*block.BlockRef,
) ([]byte, bool, error) {
	return nil, false, nil
}

func (s *testStatsBlockStore) GetBlockExists(context.Context, *block.BlockRef) (bool, error) {
	return false, nil
}

func (s *testStatsBlockStore) GetBlockExistsBatch(
	context.Context,
	[]*block.BlockRef,
) ([]bool, error) {
	return nil, nil
}

func (s *testStatsBlockStore) RmBlock(context.Context, *block.BlockRef) error { return nil }

func (s *testStatsBlockStore) StatBlock(context.Context, *block.BlockRef) (*block.BlockStat, error) {
	return nil, nil
}

func (s *testStatsBlockStore) Flush(context.Context) error { return nil }

func (s *testStatsBlockStore) BeginDeferFlush() {}

func (s *testStatsBlockStore) EndDeferFlush(context.Context) error { return nil }

func waitForStorageStatsWake(t *testing.T, waitCh <-chan struct{}) {
	t.Helper()
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for storage stats wakeup")
	}
}
