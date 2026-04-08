//go:build js

package volume_opfs

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/opfs"
	store_kvtx "github.com/aperturerobotics/hydra/store/kvtx"
	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/sirupsen/logrus"
)

func TestOpfsVolumeIntegration(t *testing.T) {
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	ctx := context.Background()
	vol, err := NewOpfs(ctx, logrus.NewEntry(logrus.New()), &Config{
		RootPath:    "test-volume-js-opfs",
		StoreConfig: &store_kvtx.Config{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := vol.Delete(); err != nil {
			t.Error(err)
		}
	}()

	if err := store_test.TestAll(ctx, vol); err != nil {
		t.Fatal(err)
	}

	ref, _, err := vol.PutBlock(ctx, []byte("stats-block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	data, found, err := vol.GetBlock(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(data) != "stats-block" {
		t.Fatalf("GetBlock: found=%v data=%q", found, data)
	}

	stats, err := vol.GetStorageStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.GetBlockCount() != 1 {
		t.Fatalf("BlockCount: got %d want 1", stats.GetBlockCount())
	}
	if stats.GetTotalBytes() != uint64(len(data)) {
		t.Fatalf("TotalBytes: got %d want %d", stats.GetTotalBytes(), len(data))
	}
}
