package kvtx

import (
	"context"
	"testing"
	"time"

	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/volume"
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

	store := store_kvtx.NewKVTx(
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		nil,
	)
	vol := &Volume{Store: store}

	_, putCh, err := vol.GetStorageStatsSnapshotWithWait(context.Background())
	if err != nil {
		t.Fatalf("GetStorageStatsSnapshotWithWait() error = %v", err)
	}
	ref, exists, err := vol.PutBlock(context.Background(), []byte("hello"), nil)
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
	if err := vol.RmBlock(context.Background(), ref); err != nil {
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

func waitForStorageStatsWake(t *testing.T, waitCh <-chan struct{}) {
	t.Helper()
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for storage stats wakeup")
	}
}
